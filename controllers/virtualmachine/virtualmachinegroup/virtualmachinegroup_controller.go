// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	finalizerName = "vmoperator.vmware.com/virtualmachinegroup"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineGroup{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
		}).
		Complete(r)
}

// NewReconciler returns a new reconciler for VirtualMachineGroup objects.
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

// Reconciler reconciles a VirtualMachineGroup object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch

// Reconcile reconciles a VirtualMachineGroup object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = record.WithContext(ctx, r.Recorder)

	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := r.Get(ctx, req.NamespacedName, vmGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger := ctrl.Log.WithName("VirtualMachineGroup").WithValues("name", req.NamespacedName)

	vmGroupCtx := &pkgctx.VirtualMachineGroupContext{
		Context: ctx,
		Logger:  logger,
		VMGroup: vmGroup,
	}

	patchHelper, err := patch.NewHelper(vmGroup, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", req.NamespacedName, err)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, vmGroup); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmGroupCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmGroup.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmGroupCtx)
	}

	return r.ReconcileNormal(vmGroupCtx)
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineGroupContext) (ctrl.Result, error) {
	ctx.Logger.Info("Reconciling VirtualMachineGroup Deletion")

	if controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.RemoveFinalizer(ctx.VMGroup, finalizerName)
	}

	ctx.Logger.Info("Finished Reconciling VirtualMachineGroup Deletion")
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineGroupContext) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.AddFinalizer(ctx.VMGroup, finalizerName)
		return ctrl.Result{}, nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineGroup")

	defer func(beforeVMGroupStatus *vmopv1.VirtualMachineGroupStatus) {
		if !apiequality.Semantic.DeepEqual(beforeVMGroupStatus, &ctx.VMGroup.Status) {
			ctx.Logger.Info("Finished Reconciling VirtualMachineGroup with updates to the CR",
				"createdTime", ctx.VMGroup.CreationTimestamp, "currentTime", time.Now().Format(time.RFC3339))
		} else {
			ctx.Logger.Info("Finished Reconciling VirtualMachineGroup")
		}
	}(ctx.VMGroup.Status.DeepCopy())

	if err := r.reconcileMembers(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile members")
		return ctrl.Result{}, err
	}

	if err := r.reconcilePlacement(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile placement")
		return ctrl.Result{}, err
	}

	if err := r.reconcilePowerState(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile power state")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileMembers reconciles group members by setting their owner references.
func (r *Reconciler) reconcileMembers(ctx *pkgctx.VirtualMachineGroupContext) error {
	ownerRef := metav1.OwnerReference{
		APIVersion: ctx.VMGroup.APIVersion,
		Kind:       ctx.VMGroup.Kind,
		Name:       ctx.VMGroup.Name,
		UID:        ctx.VMGroup.UID,
	}

	for _, member := range ctx.VMGroup.Spec.Members {
		switch member.Kind {
		case "VirtualMachine":
			vm := &vmopv1.VirtualMachine{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: ctx.VMGroup.Namespace, Name: member.Name}, vm); err != nil {
				return err
			}

			patch := client.MergeFrom(vm.DeepCopy())
			vm.OwnerReferences = append(vm.OwnerReferences, ownerRef)
			if err := r.Patch(ctx, vm, patch); err != nil {
				return err
			}

		case "VirtualMachineGroup":
			group := &vmopv1.VirtualMachineGroup{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: ctx.VMGroup.Namespace, Name: member.Name}, group); err != nil {
				return err
			}

			patch := client.MergeFrom(group.DeepCopy())
			group.OwnerReferences = append(group.OwnerReferences, ownerRef)
			if err := r.Patch(ctx, group, patch); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unsupported member kind: %s", member.Kind)
		}
	}

	return nil
}

// TODO(sai): Implement placement logic for all unplaced VMs in the group.
func (r *Reconciler) reconcilePlacement(ctx *pkgctx.VirtualMachineGroupContext) error {
	return nil
}

// TODO(sai): Implement power state logic for all group members.
func (r *Reconciler) reconcilePowerState(ctx *pkgctx.VirtualMachineGroupContext) error {
	return nil
}
