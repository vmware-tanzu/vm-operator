// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyusage

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
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

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	c, err := controller.New(
		controllerName, mgr, controller.Options{Reconciler: r})
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
		spqutil.StoragePolicyUsageName(req.Name))
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	namespace, storagePolicyUsageName string) error {

	objKey := client.ObjectKey{
		Namespace: namespace,
		Name:      storagePolicyUsageName,
	}

	// Make sure the StoragePolicyUsage document exists.
	var obj spqv1.StoragePolicyUsage
	if err := r.Client.Get(ctx, objKey, &obj); err != nil {

		// Even if the SPU is not found, log the error to indicate that
		// the function was enqueued but could not proceed.
		return fmt.Errorf(
			"failed to get StoragePolicyUsage %s: %w", objKey, err)
	}

	var list vmopv1.VirtualMachineList
	if err := r.Client.List(
		ctx,
		&list,
		client.InNamespace(namespace)); err != nil {

		return fmt.Errorf(
			"failed to list VMs in namespace %s: %w", namespace, err)
	}

	var (
		totalUsed     resource.Quantity
		totalReserved resource.Quantity
	)

	for i := range list.Items {
		vm := list.Items[i]

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
	if err := r.Client.Get(ctx, objKey, &obj); err != nil {
		return fmt.Errorf(
			"failed to get StoragePolicyUsage %s: %w", objKey, err)
	}

	objPatch := client.MergeFrom(obj.DeepCopy())

	if obj.Status.ResourceTypeLevelQuotaUsage == nil {
		obj.Status.ResourceTypeLevelQuotaUsage = &spqv1.QuotaUsageDetails{}
	}
	obj.Status.ResourceTypeLevelQuotaUsage.Reserved = &totalReserved
	obj.Status.ResourceTypeLevelQuotaUsage.Used = &totalUsed

	if err := r.Client.Status().Patch(ctx, &obj, objPatch); err != nil {
		return fmt.Errorf(
			"failed to patch StoragePolicyUsage %s: %w", objKey, err)
	}

	return nil
}

func reportUsed(
	vm vmopv1.VirtualMachine,
	total *resource.Quantity) {

	// The VM is created, and therefore the VM's actual usage should be
	// reported as its Used value.
	if vm.Status.Storage.Unshared != nil {
		total.Add(*vm.Status.Storage.Unshared)
	}

	// Subtract the PVC usage/limit from the total used value as PVCs
	// are counted separately.
	for j := range vm.Status.Volumes {
		v := vm.Status.Volumes[j]
		if v.Type == vmopv1.VirtualMachineStorageDiskTypeManaged {
			if v.Used != nil {
				total.Sub(*v.Used)
			}
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
