// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegrouppublishrequest

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
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
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
			LogConstructor: pkgutil.ControllerLogConstructor(
				controllerNameShort,
				controlledType,
				mgr.GetScheme()),
		}).
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

	obj := &vmopv1.VirtualMachineGroupPublishRequest{}
	if err := r.apiReader.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	objCtx := &pkgctx.VirtualMachineGroupPublishRequestContext{
		Context: ctx,
		Logger:  pkgutil.FromContextOrDefault(ctx),
		Obj:     obj,
	}
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for %s/%s: %w",
			obj.Namespace,
			obj.Name, err)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, obj); err != nil {
			if reterr == nil {
				reterr = err
			}
			objCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(objCtx)
	}

	return r.ReconcileNormal(objCtx)
}

func (r *Reconciler) ReconcileDelete(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext) (ctrl.Result, error) {

	ctx.Logger.Info("Reconciling VirtualMachineGroupPublishRequest Deletion")

	if controllerutil.ContainsFinalizer(ctx.Obj, finalizerName) {
		controllerutil.RemoveFinalizer(ctx.Obj, finalizerName)
		ctx.Logger.Info("Removed finalizer for VirtualMachineGroupPublishRequest")
	}
	ctx.Logger.V(4).Info("Finished Reconciling VirtualMachineGroupPublishRequest Deletion")
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext) (_ ctrl.Result, reterr error) {

	ctx.Logger.Info("Reconciling VirtualMachinePublishRequest")
	vmGroupPublishReq := ctx.Obj

	if !controllerutil.ContainsFinalizer(vmGroupPublishReq, finalizerName) {
		controllerutil.AddFinalizer(vmGroupPublishReq, finalizerName)
		return ctrl.Result{}, reterr
	}

	return ctrl.Result{}, nil
}
