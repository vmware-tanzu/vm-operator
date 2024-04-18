// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	// VcCredsSecretName is the credential secret that stores the VM operator service provider user credentials.
	VcCredsSecretName = "wcp-vmop-sa-vc-auth" //nolint:gosec
)

type provider interface {
	ResetVcClient(ctx context.Context)
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &corev1.Secret{}
		controllerName      = "infra-secret"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.Namespace,
		ctx.VMProvider,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	cache, err := pkgmgr.NewNamespacedCacheForObject(
		mgr,
		&ctx.SyncPeriod,
		controlledType,
		r.vmOpNamespace)
	if err != nil {
		return err
	}

	return c.Watch(
		source.Kind(cache, controlledType),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return e.Object.GetName() == VcCredsSecretName
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetName() == VcCredsSecretName
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		},
		predicate.ResourceVersionChangedPredicate{},
	)

}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmOpNamespace string,
	provider provider) *Reconciler {
	return &Reconciler{
		Context:       ctx,
		Client:        client,
		Logger:        logger,
		Recorder:      recorder,
		vmOpNamespace: vmOpNamespace,
		provider:      provider,
	}
}

type Reconciler struct {
	client.Client
	Context       context.Context
	Logger        logr.Logger
	Recorder      record.Recorder
	vmOpNamespace string
	provider      provider
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if req.Name == VcCredsSecretName && req.Namespace == r.vmOpNamespace {
		r.reconcileVcCreds(ctx, req)
		return ctrl.Result{}, nil
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileVcCreds(ctx context.Context, req ctrl.Request) {
	r.Logger.Info("Reconciling updated VM Operator credentials", "secret", req.NamespacedName)
	r.provider.ResetVcClient(ctx)
}
