// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability

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
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &corev1.ConfigMap{}
		controllerName      = "capability-configmap"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	cache, err := pkgmgr.NewNamespacedCacheForObject(
		mgr,
		&ctx.SyncPeriod,
		controlledType,
		capabilities.WCPClusterCapabilitiesConfigMapObjKey.Namespace)
	if err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		cache,
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	// This controller is also run on the non-leaders (webhooks) pods too
	// so capabilities updates are reflected there.
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 1,
		NeedLeaderElection:      ptr.To(false),
	})
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(
		cache,
		controlledType,
		&handler.TypedEnqueueRequestForObject[*corev1.ConfigMap]{},
		predicate.TypedFuncs[*corev1.ConfigMap]{
			CreateFunc: func(e event.TypedCreateEvent[*corev1.ConfigMap]) bool {
				return e.Object.Name == capabilities.WCPClusterCapabilitiesConfigMapObjKey.Name
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
				return e.ObjectOld.Name == capabilities.WCPClusterCapabilitiesConfigMapObjKey.Name
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.ConfigMap]) bool {
				return false
			},
			GenericFunc: func(e event.TypedGenericEvent[*corev1.ConfigMap]) bool {
				return false
			},
		},
		kubeutil.TypedResourceVersionChangedPredicate[*corev1.ConfigMap]{},
	))
}

func NewReconciler(
	ctx context.Context,
	cache client.Reader,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Cache:    cache,
		Logger:   logger,
		Recorder: recorder,
	}
}

type Reconciler struct {
	Context  context.Context
	Cache    client.Reader
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if req.NamespacedName == capabilities.WCPClusterCapabilitiesConfigMapObjKey {
		return ctrl.Result{}, r.reconcileWcpClusterCapabilitiesConfig(ctx, req)
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileWcpClusterCapabilitiesConfig(ctx context.Context, req ctrl.Request) error {
	oldFeatures := pkgcfg.FromContext(ctx).Features
	r.Logger.Info("Reconciling WCP Cluster Capabilities Config", "configMap", req.NamespacedName,
		"isAsyncSVUpgrade", oldFeatures.SVAsyncUpgrade)

	cm := &corev1.ConfigMap{}
	if err := r.Cache.Get(ctx, req.NamespacedName, cm); err != nil {
		return client.IgnoreNotFound(err)
	}

	capabilities.UpdateCapabilitiesFeatures(ctx, cm.Data)
	if newFeatures := pkgcfg.FromContext(ctx).Features; oldFeatures != newFeatures {
		r.Logger.Info("Updated features from capabilities", "old", oldFeatures, "new", newFeatures)
	}

	return nil
}
