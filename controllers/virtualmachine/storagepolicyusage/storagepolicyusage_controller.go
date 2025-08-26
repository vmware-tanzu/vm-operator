// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyusage

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerName      = "storagepolicyusage"
		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controllerName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	// Index the VM's spec.storageClass field to make it easy to list VMs in a
	// namespace by the field.
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.VirtualMachine{},
		"spec.storageClass",
		func(rawObj client.Object) []string {
			vm := rawObj.(*vmopv1.VirtualMachine)
			return []string{vm.Spec.StorageClass}
		}); err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:     r,
		LogConstructor: pkgutil.ControllerLogConstructor(controllerNameShort, &spqv1.StoragePolicyUsage{}, mgr.GetScheme()),
	})
	if err != nil {
		return err
	}

	return c.Watch(source.Channel(
		spqutil.FromContext(ctx),
		&handler.EnqueueRequestForObject{}))
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a StoragePolicyUsage object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots/status,verbs=get
// +kubebuilder:rbac:groups="",resources=resourcequotas;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas,verbs=get;list;watch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas/status,verbs=get
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	return ctrl.Result{}, r.ReconcileNormal(
		ctx,
		req.Namespace,
		req.Name)
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	namespace string,
	scName string) error {

	var list vmopv1.VirtualMachineList
	if err := r.Client.List(
		ctx,
		&list,
		client.InNamespace(namespace),
		client.MatchingFields{"spec.storageClass": scName},
		//
		// !!! WARNING !!!
		//
		// The use of the UnsafeDisableDeepCopy option improves
		// performance by skipping a CPU-intensive operation.
		// However, it also means any writes to the returned
		// objects will directly impact the cache. Therefore,
		// please be aware of this when doing anything with the
		// object(s) that are the result of this operation.
		//
		client.UnsafeDisableDeepCopy); err != nil {

		return fmt.Errorf(
			"failed to list VMs in namespace %s: %w", namespace, err)
	}

	var errs []error
	if err := r.ReconcileSPUForVM(ctx, namespace, scName, list.Items); err != nil {
		errs = append(errs, err)
	}
	if pkgcfg.FromContext(ctx).Features.VMSnapshots {
		if err := r.ReconcileSPUForVMSnapshot(ctx, namespace, scName, list.Items); err != nil {
			errs = append(errs, err)
		}
	}

	return apierrorsutil.NewAggregate(errs)
}

func (r *Reconciler) ReconcileSPUForVM(
	ctx context.Context,
	namespace string,
	scName string,
	vms []vmopv1.VirtualMachine) error {

	objKey := client.ObjectKey{
		Namespace: namespace,
		Name:      spqutil.StoragePolicyUsageNameForVM(scName),
	}

	// Make sure the StoragePolicyUsage document exists.
	var spu spqv1.StoragePolicyUsage
	if err := r.Client.Get(ctx, objKey, &spu); err != nil {
		// Even if the SPU is not found, log the error to indicate that
		// the function was enqueued but could not proceed.
		return fmt.Errorf(
			"failed to get StoragePolicyUsage %s: %w", objKey, err)
	}

	var (
		totalUsed     resource.Quantity
		totalReserved resource.Quantity
	)

	for i := range vms {
		vm := vms[i]

		if !vm.DeletionTimestamp.IsZero() {
			// Ignore VMs that are being deleted.
			continue
		}

		if vm.Status.Storage == nil ||
			!conditions.IsTrue(&vm, vmopv1.VirtualMachineConditionCreated) {

			// The VM is not yet fully created or is not yet reporting its
			// storage information. Therefore, report the VM's potential
			// capacity as its Reserved value.
			if err := reportReserved(
				ctx, r.Client, vm, &totalReserved); err != nil {

				return fmt.Errorf(
					"failed to report reserved capacity for %q: %w",
					vm.NamespacedName(), err)
			}
		} else {
			// The VM is created and is reporting storage usage information.
			// Therefore, report the VM's actual usage as its Used value.
			reportUsed(vm, &totalUsed)
		}
	}

	// Get the StoragePolicyUsage resource again to ensure it is up-to-date.
	if err := r.Client.Get(ctx, objKey, &spu); err != nil {
		return fmt.Errorf(
			"failed to get StoragePolicyUsage %s: %w", objKey, err)
	}

	objPatch := client.MergeFrom(spu.DeepCopy())

	if spu.Status.ResourceTypeLevelQuotaUsage == nil {
		spu.Status.ResourceTypeLevelQuotaUsage = &spqv1.QuotaUsageDetails{}
	}
	spu.Status.ResourceTypeLevelQuotaUsage.Reserved = &totalReserved
	spu.Status.ResourceTypeLevelQuotaUsage.Used = &totalUsed

	if err := r.Client.Status().Patch(ctx, &spu, objPatch); err != nil {
		return fmt.Errorf(
			"failed to patch StoragePolicyUsage %s: %w", objKey, err)
	}

	return nil
}

func (r *Reconciler) ReconcileSPUForVMSnapshot(
	ctx context.Context,
	namespace string,
	scName string,
	vms []vmopv1.VirtualMachine) error {

	objKey := client.ObjectKey{
		Namespace: namespace,
		Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(scName),
	}

	// Make sure the StoragePolicyUsage document exists.
	var spu spqv1.StoragePolicyUsage
	if err := r.Client.Get(ctx, objKey, &spu); err != nil {
		return fmt.Errorf(
			"failed to get StoragePolicyUsage %s: %w", objKey, err)
	}

	var (
		totalUsed     resource.Quantity
		totalReserved resource.Quantity
	)

	vmMap := make(map[string]vmopv1.VirtualMachine)
	for _, vm := range vms {
		vmMap[vm.Name] = vm
	}

	// TODO (lubron): Use pagination to fetch VirtualMachineSnapshots in batches and update
	// the StoragePolicyUsage incrementally, so we avoid overloading memory.
	// Can't think of a good way to index the VMSnapshots by storage class, since a VMSnapshot
	// Can have disks with different storage classes.
	vmSnapshotList := &vmopv1.VirtualMachineSnapshotList{}
	if err := r.Client.List(ctx,
		vmSnapshotList,
		client.InNamespace(namespace),
		//
		// !!! WARNING !!!
		//
		// The use of the UnsafeDisableDeepCopy option improves
		// performance by skipping a CPU-intensive operation.
		// However, it also means any writes to the returned
		// objects will directly impact the cache. Therefore,
		// please be aware of this when doing anything with the
		// object(s) that are the result of this operation.
		//
		client.UnsafeDisableDeepCopy); err != nil {
		return fmt.Errorf(
			"failed to list VirtualMachineSnapshots in namespace %s: %w", namespace, err)
	}

	for _, vmSnapshot := range vmSnapshotList.Items {
		// Update the usage only when the CSI has marked the VMSnapshot with volume sync completed.
		if vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] ==
			constants.CSIVSphereVolumeSyncAnnotationValueCompleted {
			// Report the snapshot's used capacity.
			if err := r.reportUsedForSnapshot(
				ctx, vmSnapshot, &totalUsed, vmMap); err != nil {
				return fmt.Errorf(
					"failed to report used capacity for %q: %w",
					vmSnapshot.NamespacedName(), err)
			}
		} else {
			// Report the snapshot's reserved capacity at worst case.
			if err := r.reportReservedForSnapshot(
				ctx, vmSnapshot, &totalReserved, spu.Spec.StorageClassName); err != nil {

				return fmt.Errorf(
					"failed to report reserved capacity for %q: %w",
					vmSnapshot.NamespacedName(), err)
			}
		}
	}

	// Get the StoragePolicyUsage resource again to ensure it is up-to-date.
	if err := r.Client.Get(ctx, objKey, &spu); err != nil {
		return fmt.Errorf(
			"failed to get StoragePolicyUsage %s: %w", objKey, err)
	}

	objPatch := client.MergeFrom(spu.DeepCopy())

	if spu.Status.ResourceTypeLevelQuotaUsage == nil {
		spu.Status.ResourceTypeLevelQuotaUsage = &spqv1.QuotaUsageDetails{}
	}
	spu.Status.ResourceTypeLevelQuotaUsage.Reserved = &totalReserved
	spu.Status.ResourceTypeLevelQuotaUsage.Used = &totalUsed

	if err := r.Client.Status().Patch(ctx, &spu, objPatch); err != nil {
		return fmt.Errorf(
			"failed to patch StoragePolicyUsage %s: %w", objKey, err)
	}

	return nil
}

func reportUsed(
	vm vmopv1.VirtualMachine,
	total *resource.Quantity) {

	if s := vm.Status.Storage; s != nil {
		if s.Total != nil {
			total.Add(*s.Total)
		}
	}
}

func reportReserved(
	ctx context.Context,
	k8sClient client.Client,
	vm vmopv1.VirtualMachine,
	total *resource.Quantity) error {

	var (
		imgKind string
		imgName string
	)

	if ref := vm.Spec.Image; ref != nil {
		imgKind = ref.Kind
		imgName = ref.Name
	}

	if imgKind == "" || imgName == "" {
		// Only report reserved capacity if the image ref has been specified.
		return nil
	}

	var (
		imgKey    = client.ObjectKey{Name: imgName}
		imgStatus vmopv1.VirtualMachineImageStatus
	)

	switch imgKind {
	case "VirtualMachineImage":
		imgKey.Namespace = vm.Namespace
		var img vmopv1.VirtualMachineImage
		if err := k8sClient.Get(ctx, imgKey, &img); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get %s %s: %w", imgKind, imgKey, err)
			}
			return nil
		}
		imgStatus = img.Status
	case "ClusterVirtualMachineImage":
		var img vmopv1.ClusterVirtualMachineImage
		if err := k8sClient.Get(ctx, imgKey, &img); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get %s %s: %w", imgKind, imgKey, err)
			}
			return nil
		}
		imgStatus = img.Status
	}

	for i := range imgStatus.Disks {
		if v := imgStatus.Disks[i].Capacity; v != nil {
			total.Add(*v)
		}
	}

	return nil
}

// reportReservedForSnapshot reports the reserved capacity for a snapshot.
func (r *Reconciler) reportReservedForSnapshot(
	ctx context.Context,
	vmSnapshot vmopv1.VirtualMachineSnapshot,
	total *resource.Quantity,
	storageClass string) error {

	if vmSnapshot.Spec.VMRef == nil {
		return fmt.Errorf("vmRef is not set")
	}

	logger := pkgutil.FromContextOrDefault(ctx)

	// It's possible that the snapshot's requested capacity is not updated yet,
	// so we skip the calculation for now
	if vmSnapshot.Status.Storage == nil || vmSnapshot.Status.Storage.Requested == nil {
		logger.V(5).Info("snapshot has no requested capacity reported by controller yet, "+
			"skip calculating reserved capacity for it", "snapshot", vmSnapshot.Name)
		return nil
	}
	// Loop through all the requested capacities and add the requested capacity
	// with the same storage class as the SPU's storage class.
	for _, requested := range vmSnapshot.Status.Storage.Requested {
		if requested.StorageClass == storageClass {
			logger.V(5).Info("adding snapshot's requested capacity to total reserved capacity",
				"snapshot", vmSnapshot.Name, "requested", requested.Total)
			total.Add(*requested.Total)
			break
		}
	}

	return nil
}

// reportUsedForSnapshot reports the used capacity for a snapshot.
// Get the snapshot size from the VMSnapshot's status.Used.Total.
func (r *Reconciler) reportUsedForSnapshot(
	ctx context.Context,
	vmSnapshot vmopv1.VirtualMachineSnapshot,
	total *resource.Quantity,
	vmMap map[string]vmopv1.VirtualMachine) error {

	if vmSnapshot.Spec.VMRef == nil {
		return fmt.Errorf("vmRef is not set")
	}

	logger := pkgutil.FromContextOrDefault(ctx)

	if _, ok := vmMap[vmSnapshot.Spec.VMRef.Name]; !ok {
		logger.V(5).Info("VM not found in vmMap, which means the VM's storage class is "+
			"different from the SPU's storage class, skip calculating used capacity for it",
			"vm", vmSnapshot.Spec.VMRef.Name)
		return nil
	}

	// There might be chance that VMSnapshot's marked as CSIVolumeSync completed but VMSnapshot controller
	// hasn't updated VMSnapshot with used capacity yet.
	// Wait for next reconcile to update the SPU.
	if vmSnapshot.Status.Storage == nil || vmSnapshot.Status.Storage.Used == nil {
		logger.V(5).Info("snapshot has no used capacity reported by controller yet, "+
			"skip calculating used capacity for it", "snapshot", vmSnapshot.Name)
		return nil
	}

	total.Add(*vmSnapshot.Status.Storage.Used)

	return nil
}
