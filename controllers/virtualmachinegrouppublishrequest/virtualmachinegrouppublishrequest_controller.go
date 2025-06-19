// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegrouppublishrequest

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	finalizerName = "vmoperator.vmware.com/virtualmachinegrouppublishrequest"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineGroupPublishRequest{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)
	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		mgr.GetAPIReader(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)
	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(r)
}

// Reconciler reconciles a VirtualMachineGroupePublishRequest object.
type Reconciler struct {
	client.Client
	Context    context.Context
	apiReader  client.Reader
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	apiReader client.Reader,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		apiReader:  apiReader,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// Reconcile reconciles a VirtualMachineGroupPublishRequest object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vmGroupPublishReq := &vmopv1.VirtualMachineGroupPublishRequest{}
	if err := r.apiReader.Get(ctx, req.NamespacedName, vmGroupPublishReq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmGroupPublishCtx := &pkgctx.VirtualMachineGroupPublishRequestContext{
		Context:               ctx,
		Logger:                ctrl.Log.WithName("VirtualMachineGroupPublishRequest").WithValues("name", req.NamespacedName),
		VMGroupPublishRequest: vmGroupPublishReq,
	}
	patchHelper, err := patch.NewHelper(vmGroupPublishReq, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s/%s: %w", vmGroupPublishReq.Namespace, vmGroupPublishReq.Name, err)
	}

	// the patch is skipped when the VirtualMachinePublishRequest.Status().Update
	// is called during publishVirtualMachine().
	defer func() {
		if vmGroupPublishCtx.SkipPatch {
			return
		}

		if err := patchHelper.Patch(ctx, vmGroupPublishReq); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmGroupPublishCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmGroupPublishReq.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmGroupPublishCtx)
	}

	return r.ReconcileNormal(vmGroupPublishCtx)
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) (ctrl.Result, error) {
	ctx.Logger.Info("Reconciling VirtualMachineGroupPublishRequest Deletion")

	if controllerutil.ContainsFinalizer(ctx.VMGroupPublishRequest, finalizerName) {
		controllerutil.RemoveFinalizer(ctx.VMGroupPublishRequest, finalizerName)
		ctx.Logger.Info("Removed finalizer for VirtualMachineGroupPublishRequest")
	}
	ctx.Logger.Info("Finished Reconciling VirtualMachineGroupPublishRequest Deletion")
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) (_ ctrl.Result, reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachinePublishRequest")
	vmGroupPublishReq := ctx.VMGroupPublishRequest

	if !controllerutil.ContainsFinalizer(vmGroupPublishReq, finalizerName) {
		controllerutil.AddFinalizer(vmGroupPublishReq, finalizerName)
		return ctrl.Result{}, reterr
	}

	if vmGroupPublishReq.Status.StartTime.IsZero() {
		vmGroupPublishReq.Status.StartTime = metav1.Now()
	}

	r.updateSourceAndTargetRef(ctx)

	if err := r.reconcileMembers(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile group members")
		conditions.MarkFalse(ctx.VMGroupPublishRequest, vmopv1.VirtualMachineGroupPublishRequestConditionMembersOwnerRefReady, "ReconcileMembersError", "%v", err)
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(ctx.VMGroupPublishRequest, vmopv1.VirtualMachineGroupPublishRequestConditionMembersOwnerRefReady)
	return ctrl.Result{}, nil
}

// This is dependent on https://github.com/vmware-tanzu/vm-operator/pull/954/ to
// introduce VM Group.
// reconcileMembers reconciles group members by checking if their group name is
// set to the current VM Group's name and adding an owner reference on them.
func (r *Reconciler) reconcileMembers(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) error {
	ownerRef := []metav1.OwnerReference{
		{
			APIVersion: ctx.VMGroupPublishRequest.APIVersion,
			Kind:       ctx.VMGroupPublishRequest.Kind,
			Name:       ctx.VMGroupPublishRequest.Name,
			UID:        ctx.VMGroupPublishRequest.UID,
		},
	}

	var errs []error
	nsName := ctx.VMGroupPublishRequest.Namespace
	vmGroupName := ctx.VMGroupPublishRequest.Spec.Source.Name
	vmGroup := &vmopv1.VirtualMachineGroup{}
	vmGroupKey := client.ObjectKey{Namespace: nsName, Name: vmGroupName}
	if err := r.Get(ctx, vmGroupKey, vmGroup); err != nil {
		return err
	}

	for _, member := range vmGroup.Spec.Members {
		switch member.Kind {
		case "VirtualMachine":
			vmPubName := member.Name + "-image"
			vmPub := &vmopv1.VirtualMachinePublishRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmPubName,
					Namespace: nsName,
					OwnerReferences: ownerRef,
				},
			}
			if err := r.Create(ctx, vmPub); err != nil {
				errs = append(errs, err)
				continue
			}
		case "VirtualMachineGroup":
			//TODO
		}
	}

	return apierrorsutil.NewAggregate(errs)
}

func (r *Reconciler) updateSourceAndTargetRef(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) {
	vmGroupPubReq := ctx.VMGroupPublishRequest

	if vmGroupPubReq.Status.SourceRef == nil {
		vmGroupName := vmGroupPubReq.Spec.Source.Name
		if vmGroupName == "" {
			// set default source VM Group to this VirtualMachineGroupPublishRequest's name.
			vmGroupName = vmGroupPubReq.Name
		}
		vmGroupPubReq.Status.SourceRef = &vmopv1.VirtualMachineGroupPublishRequestSource{
			Name: vmGroupName,
		}
	}

	if vmGroupPubReq.Status.TargetRef == nil {
		vmGroupPubReq.Status.TargetRef = &vmopv1.VirtualMachineGroupPublishRequestTarget{
			Description: vmGroupPubReq.Spec.Target.Description,
			Location:    vmGroupPubReq.Spec.Target.Location,
		}
	}
}
