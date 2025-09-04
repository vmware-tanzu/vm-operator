// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	pkgexit "github.com/vmware-tanzu/vm-operator/pkg/exit"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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
		capabilities.ConfigMapKey.Namespace)
	if err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		cache,
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	// This controller is also run on the non-leaders (webhooks) pods too
	// so capabilities updates are reflected there.
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 1,
		LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
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
				return e.Object.Name == capabilities.ConfigMapKey.Name
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
				return e.ObjectOld.Name == capabilities.ConfigMapKey.Name
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.ConfigMap]) bool {
				return false
			},
		},
		kubeutil.TypedResourceVersionChangedPredicate[*corev1.ConfigMap]{},
	))
}

func NewReconciler(
	ctx context.Context,
	client ctrlclient.Client,
	reader ctrlclient.Reader,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Reader:   reader,
		Logger:   logger,
		Recorder: recorder,
	}
}

type Reconciler struct {
	Context  context.Context
	Client   ctrlclient.Client
	Reader   ctrlclient.Reader
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (ctrl.Result, error) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.Info("Reconciling capabilities")

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj corev1.ConfigMap
	if err := r.Reader.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
	}

	if diff, ok := capabilities.WouldUpdateCapabilitiesFeatures(ctx, obj); ok {
		if err := pkgexit.Restart(
			ctx,
			r.Client,
			fmt.Sprintf("capabilities have changed: %s", diff)); err != nil {

			logger.Error(err, "Failed to exit due to capability change")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
