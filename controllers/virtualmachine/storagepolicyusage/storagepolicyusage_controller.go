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
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
		LogConstructor: pkglog.ControllerLogConstructor(controllerNameShort, &spqv1.StoragePolicyUsage{}, mgr.GetScheme()),
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
		vm := &vms[i]

		if !vm.DeletionTimestamp.IsZero() {
			// Ignore VMs that are being deleted.
			continue
		}

		if vm.Status.Storage == nil ||
			!conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated) {

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

	// Record VMs that are being deleted so we could skip reporting their
	// snapshot requested/used capacity in SPU.
	// NOTE: we are not using a Set here because we also need to know whether
	// the snapshot belongs to the VM which are not sharing the same StorageClass
	// as the current SPU. It's possible when calculating requested capacity since
	// VM could has different SC while its PVCs might have same SC.
	var vmDeletionState *map[string]bool
	for _, vmSnapshot := range vmSnapshotList.Items {

		if !vmSnapshot.ObjectMeta.DeletionTimestamp.IsZero() {
			// If the snapshot is being deleted, skip including its
			// requested/used capacity.
			continue
		}

		// Initialize vmDeletionState only once.
		if vmDeletionState == nil {
			tmp := make(map[string]bool, len(vms))
			for _, vm := range vms {
				tmp[vm.Name] = !vm.ObjectMeta.DeletionTimestamp.IsZero()
			}
			vmDeletionState = &tmp
		}

		// Update the usage only when the CSI has marked the VMSnapshot with
		// volume sync completed.
		if vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] ==
			constants.CSIVSphereVolumeSyncAnnotationValueCompleted {

			// Report the snapshot's used capacity.
			r.reportUsedForSnapshot(
				ctx, &vmSnapshot, &totalUsed, *vmDeletionState)
		} else {

			// Report the snapshot's reserved capacity in the worst-case scenario.
			r.reportReservedForSnapshot(
				ctx,
				&vmSnapshot,
				&totalReserved,
				spu.Spec.StorageClassName,
				*vmDeletionState)
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
	vm *vmopv1.VirtualMachine,
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
	vm *vmopv1.VirtualMachine,
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
		imgKey   = client.ObjectKey{Name: imgName}
		imgDisks []vmopv1.VirtualMachineImageDiskInfo
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
		imgDisks = img.Status.Disks
	case "ClusterVirtualMachineImage":
		var img vmopv1.ClusterVirtualMachineImage
		if err := k8sClient.Get(ctx, imgKey, &img); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get %s %s: %w", imgKind, imgKey, err)
			}
			return nil
		}
		imgDisks = img.Status.Disks
	}

	for i := range imgDisks {
		if v := imgDisks[i].Limit; v != nil {
			total.Add(*v)
		}
	}

	return nil
}

// reportReservedForSnapshot reports the reserved capacity for a snapshot.
func (r *Reconciler) reportReservedForSnapshot(
	ctx context.Context,
	vmSnapshot *vmopv1.VirtualMachineSnapshot,
	total *resource.Quantity,
	storageClass string,
	vmDeletionState map[string]bool) {

	if vmSnapshot.Spec.VMName == "" {
		// This shouldn't happen though since our validation webhhook guards this.
		// Check anyway.
		return
	}

	// 'ok' could be false, which means VM itself has different StorageClass
	// than the SPU, but its PVCs might share the same SC as this SPU.
	// So we still want to include this snapshot's requested capacity in the SPU.
	//
	// But we are not doing a Get() call here to check whether the owner VM (which
	// is not part of the vmDeletionState map) has been deleted or not, because:
	// 1. This is a rare case, this means after Snapshot is created, before CSI
	//    has done the volume synce, the owner VM is deleted.
	// 2. VMSnapshot controller will enqueue another SPU event if the Snapshot
	//    is GCed after VM is deleted. And the SPU's requested capacity will be
	//    corrected in later reconcile immediately.
	if beingDeleted, ok := vmDeletionState[vmSnapshot.Spec.VMName]; ok && beingDeleted {
		// The owner VM is being deleted, skip calculating
		// capacity for this snapshot.
		return
	}

	logger := pkglog.FromContextOrDefault(ctx)
	// It's possible that the snapshot's requested capacity is not updated yet,
	// so we skip calculating.
	if vmSnapshot.Status.Storage == nil || vmSnapshot.Status.Storage.Requested == nil {
		logger.V(5).Info("snapshot has no requested capacity reported by controller yet, "+
			"skip calculating reserved capacity for it", "snapshot", vmSnapshot.Name)
		return
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
}

// reportUsedForSnapshot reports the used capacity for a snapshot.
// Get the snapshot size from the VMSnapshot's status.Used.Total.
func (r *Reconciler) reportUsedForSnapshot(
	ctx context.Context,
	vmSnapshot *vmopv1.VirtualMachineSnapshot,
	total *resource.Quantity,
	vmDeletionState map[string]bool) {

	if vmSnapshot.Spec.VMName == "" {
		// This shouldn't happen though since our validation webhhook guards this.
		// Check anyway.
		return
	}

	beingDeleted, ok := vmDeletionState[vmSnapshot.Spec.VMName]
	if !ok {
		// The VMs listed are filtered by the SPU's StorageClass, if the
		// snapshot's owner VM is not in this list, that means its StorageClass
		// doesn't match SPU's StorageClass. We skip reporting its used capacity
		// to this SPU.
		// We don't care about VM that has different StorageClass even though
		// they might have PVC that has same StorageClass. Since we don't include
		// PVC's usage in VMSnapshot usage.
		return
	}

	if beingDeleted {
		// The owner VM is being deleted, that means this snapshot will be cleaned
		// up soon. Skip reporting the used capacity of this Snapshot.
		return
	}

	// There might be chance that VMSnapshot's marked as CSIVolumeSync completed but VMSnapshot controller
	// hasn't updated VMSnapshot with used capacity yet. Wait for next reconcile to update the SPU.
	if vmSnapshot.Status.Storage == nil || vmSnapshot.Status.Storage.Used == nil {
		logger := pkglog.FromContextOrDefault(ctx)
		logger.V(5).Info("snapshot has no used capacity reported by controller yet, "+
			"skip calculating used capacity for it", "snapshot", vmSnapshot.Name)
		return
	}

	total.Add(*vmSnapshot.Status.Storage.Used)
}
