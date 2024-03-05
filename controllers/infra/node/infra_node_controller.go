// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	goctx "context"
	"fmt"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

type provider interface {
	ComputeCPUMinFrequency(ctx goctx.Context) error
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerName      = "infra-node"
		controlledType      = &corev1.Node{}
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProviderA2,
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(controlledType).
		WithEventFilter(
			// Queue a reconcile request when a new node is added or removed
			// so we can calculate the minimum CPU frequency.
			// If a new node is added to the vSphere cluster, we rely on the
			// next sync.
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
	ctx goctx.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	provider provider) *Reconciler {
	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
		provider: provider,
	}
}

type Reconciler struct {
	client.Client
	Context  goctx.Context
	Logger   logr.Logger
	Recorder record.Recorder
	provider provider
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgconfig.JoinContext(ctx, r.Context)

	r.Logger.Info("Received reconcile request", "namespace", req.Namespace, "name", req.Name)

	// Update the minimum CPU frequency. This frequency is used to populate the
	// resource allocation fields in the ConfigSpec for cloning the VM.
	if err := r.provider.ComputeCPUMinFrequency(ctx); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
