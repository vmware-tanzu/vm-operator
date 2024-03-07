// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	goctx "context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &corev1.ConfigMap{}
		controllerName      = "infra-configmap"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.Namespace,
		ctx.VMProviderA2,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	cache, err := pkgmgr.NewNamespacedCacheForObject(
		mgr,
		&ctx.SyncPeriod,
		controlledType,
		WcpClusterConfigMapNamespace)
	if err != nil {
		return err
	}

	return c.Watch(
		source.Kind(cache, controlledType),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return e.Object.GetName() == WcpClusterConfigMapName
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetName() == WcpClusterConfigMapName
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

type provider interface {
	UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error
}

func NewReconciler(
	ctx goctx.Context,
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
	Context       goctx.Context
	Logger        logr.Logger
	Recorder      record.Recorder
	vmOpNamespace string
	provider      provider
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgconfig.JoinContext(ctx, r.Context)

	if req.Name == WcpClusterConfigMapName && req.Namespace == WcpClusterConfigMapNamespace {
		return ctrl.Result{}, r.reconcileWcpClusterConfig(ctx, req)
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileWcpClusterConfig(ctx goctx.Context, req ctrl.Request) error {
	r.Logger.Info("Reconciling WCP Cluster Config", "configMap", req.NamespacedName)

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		if !apiErrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	clusterConfig, err := ParseWcpClusterConfig(cm.Data)
	if err != nil {
		r.Logger.Error(err, "error in parsing the WCP cluster config")
		// No point of retrying until the object is updated.
		return nil
	}

	return r.provider.UpdateVcPNID(ctx, clusterConfig.VcPNID, clusterConfig.VcPort)
}
