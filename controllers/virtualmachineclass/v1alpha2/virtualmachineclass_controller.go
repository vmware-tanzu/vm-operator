// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	patch "github.com/vmware-tanzu/vm-operator/pkg/patch2"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineClass{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {
	return &Reconciler{
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a VirtualMachineClass object.
type Reconciler struct {
	client.Client
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	vmClass := &vmopv1.VirtualMachineClass{}
	if err := r.Get(ctx, req.NamespacedName, vmClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmClassCtx := &context.VirtualMachineClassContextA2{
		Context: ctx,
		Logger:  ctrl.Log.WithName("VirtualMachineClass").WithValues("name", req.Name),
		VMClass: vmClass,
	}

	patchHelper, err := patch.NewHelper(vmClass, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", vmClassCtx.String(), err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, vmClass); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmClassCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmClass.DeletionTimestamp.IsZero() {
		// Noop.
		return ctrl.Result{}, nil
	}

	if err := r.ReconcileNormal(vmClassCtx); err != nil {
		vmClassCtx.Logger.Error(err, "Failed to reconcile VirtualMachineClass")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(vmClassCtx *context.VirtualMachineClassContextA2) error {
	// Implicitly always ready until we actually check. Don't worry about the conditions for now.
	vmClassCtx.VMClass.Status.Ready = true
	return nil
}
