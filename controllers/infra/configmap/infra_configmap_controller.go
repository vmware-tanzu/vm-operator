// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configmap

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
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
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
		WcpClusterConfigMapNamespace)
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(
		cache,
		controlledType,
		&handler.TypedEnqueueRequestForObject[*corev1.ConfigMap]{},
		predicate.TypedFuncs[*corev1.ConfigMap]{
			CreateFunc: func(e event.TypedCreateEvent[*corev1.ConfigMap]) bool {
				return e.Object.GetName() == WcpClusterConfigMapName
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
				return e.ObjectOld.GetName() == WcpClusterConfigMapName
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

type provider interface {
	UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error
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

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if req.Name == WcpClusterConfigMapName && req.Namespace == WcpClusterConfigMapNamespace {
		return ctrl.Result{}, r.reconcileWcpClusterConfig(ctx, req)
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileWcpClusterConfig(ctx context.Context, req ctrl.Request) error {
	r.Logger.Info("Reconciling WCP Cluster Config", "configMap", req.NamespacedName)

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		return client.IgnoreNotFound(err)
	}

	clusterConfig, err := ParseWcpClusterConfig(cm.Data)
	if err != nil {
		r.Logger.Error(err, "error in parsing the WCP cluster config")
		// No point of retrying until the object is updated.
		return nil
	}

	return r.provider.UpdateVcPNID(ctx, clusterConfig.VcPNID, clusterConfig.VcPort)
}
