// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
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

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (ctrl.Result, error) {
	vmClass := &vmopv1.VirtualMachineClass{}
	err := r.Get(ctx, req.NamespacedName, vmClass)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	vmClassCtx := &context.VirtualMachineClassContext{
		Context: ctx,
		Logger:  ctrl.Log.WithName("VirtualMachineClass").WithValues("name", req.Name),
		VMClass: vmClass,
	}

	if err := r.ReconcileNormal(vmClassCtx); err != nil {
		vmClassCtx.Logger.Error(err, "Failed to reconcile VirtualMachineClass")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(_ *context.VirtualMachineClassContext) error {
	// NoOp.
	return nil
}
