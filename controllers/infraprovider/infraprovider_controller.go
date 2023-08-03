// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infraprovider

import (
	goctx "context"
	"reflect"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

type infraProvider interface {
	ComputeCPUMinFrequency(ctx goctx.Context) error
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &corev1.Node{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()
	)

	var provider infraProvider
	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		provider = ctx.VMProviderA2
	} else {
		provider = ctx.VMProvider
	}

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		provider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithEventFilter(
			// Queue a reconcile request when a new node is added or removed so we can calculate the minimum CPU frequency.
			// If a new node is added to the vSphere cluster, we rely on the next sync.
			predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return true
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return true
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return false
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return false
				},
			},
		).Complete(r)
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	provider infraProvider) *Reconciler {
	return &Reconciler{
		Client:   client,
		Logger:   logger,
		provider: provider,
	}
}

type Reconciler struct {
	client.Client
	Logger   logr.Logger
	provider infraProvider
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.Info("Received reconcile request", "namespace", req.Namespace, "name", req.Name)

	// Update the minimum CPU frequency. This frequency is used to populate the resource allocation
	// fields in the ConfigSpec for cloning the VM.
	if err := r.provider.ComputeCPUMinFrequency(ctx); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
