// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	finalizerName           = "vmoperator.vmware.com/virtualmachinesetresourcepolicy"
	deprecatedFinalizerName = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineSetResourcePolicy{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Watches(
			&topologyv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(zoneToNamespaceVMSRP(mgr.GetClient()))).
		WithLogConstructor(pkgutil.ControllerLogConstructor(controlledTypeName, controlledType, mgr.GetScheme())).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {
	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		VMProvider: vmProvider,
	}
}

func zoneToNamespaceVMSRP(
	c client.Client) func(context.Context, client.Object) []reconcile.Request {

	return func(ctx context.Context, o client.Object) []reconcile.Request {
		zone := o.(*topologyv1.Zone)

		list := vmopv1.VirtualMachineSetResourcePolicyList{}
		if err := c.List(ctx, &list, client.InNamespace(zone.Namespace)); err != nil {
			return nil
		}

		var reconcileRequests []reconcile.Request
		for i := range list.Items {
			reconcileRequests = append(reconcileRequests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
		return reconcileRequests
	}
}

// Reconciler reconciles a VirtualMachineSetResourcePolicy object.
type Reconciler struct {
	Context context.Context
	client.Client
	Logger     logr.Logger
	VMProvider providers.VirtualMachineProviderInterface
}

// ReconcileNormal reconciles a VirtualMachineSetResourcePolicy.
func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineSetResourcePolicyContext) error {
	if !controllerutil.ContainsFinalizer(ctx.ResourcePolicy, finalizerName) {
		// If the object has the deprecated finalizer, remove it.
		if updated := controllerutil.RemoveFinalizer(ctx.ResourcePolicy, deprecatedFinalizerName); updated {
			ctx.Logger.V(5).Info("Removed deprecated finalizer", "finalizerName", deprecatedFinalizerName)
		}

		// Return here so the VirtualMachineSetResourcePolicy can be patched immediately. This ensures that
		// the resource policies are cleaned up properly when they are deleted.
		controllerutil.AddFinalizer(ctx.ResourcePolicy, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachineSetResourcePolicy")
	}()

	if err := r.VMProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, ctx.ResourcePolicy); err != nil {
		ctx.Logger.Error(err, "Provider failed to reconcile VirtualMachineSetResourcePolicy")
		return err
	}

	return nil
}

// deleteResourcePolicy deletes a VirtualMachineSetResourcePolicy resource.
func (r *Reconciler) deleteResourcePolicy(ctx *pkgctx.VirtualMachineSetResourcePolicyContext) error {
	resourcePolicy := ctx.ResourcePolicy

	// Skip deleting a VirtualMachineSetResourcePolicy if it is referenced by a VirtualMachine.
	vmsInNamespace := &vmopv1.VirtualMachineList{}
	if err := r.List(ctx, vmsInNamespace, client.InNamespace(resourcePolicy.Namespace)); err != nil {
		ctx.Logger.Error(err, "Failed to list VMs in namespace", "namespace", resourcePolicy.Namespace)
		return err
	}

	for _, vm := range vmsInNamespace.Items {
		if reserved := vm.Spec.Reserved; reserved != nil && reserved.ResourcePolicyName == resourcePolicy.Name {
			return fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'",
				vm.NamespacedName(), resourcePolicy.NamespacedName())
		}
	}

	ctx.Logger.V(4).Info("Attempting to delete VirtualMachineSetResourcePolicy")
	if err := r.VMProvider.DeleteVirtualMachineSetResourcePolicy(ctx, resourcePolicy); err != nil {
		ctx.Logger.Error(err, "error in deleting VirtualMachineSetResourcePolicy")
		return err
	}
	ctx.Logger.Info("Deleted VirtualMachineSetResourcePolicy successfully")

	return nil
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineSetResourcePolicyContext) error {
	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy Deletion")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachineSetResourcePolicy Deletion")
	}()

	if controllerutil.ContainsFinalizer(ctx.ResourcePolicy, finalizerName) ||
		controllerutil.ContainsFinalizer(ctx.ResourcePolicy, deprecatedFinalizerName) {
		if err := r.deleteResourcePolicy(ctx); err != nil {
			return err
		}

		controllerutil.RemoveFinalizer(ctx.ResourcePolicy, finalizerName)
		controllerutil.RemoveFinalizer(ctx.ResourcePolicy, deprecatedFinalizerName)
	}

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	rp := &vmopv1.VirtualMachineSetResourcePolicy{}
	if err := r.Get(ctx, req.NamespacedName, rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rpCtx := &pkgctx.VirtualMachineSetResourcePolicyContext{
		Context:        ctx,
		Logger:         pkgutil.FromContextOrDefault(ctx),
		ResourcePolicy: rp,
	}

	patchHelper, err := patch.NewHelper(rp, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", rpCtx, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, rp); err != nil {
			if reterr == nil {
				reterr = err
			}
			rpCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !rp.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.ReconcileDelete(rpCtx)
	}

	return ctrl.Result{}, r.ReconcileNormal(rpCtx)
}
