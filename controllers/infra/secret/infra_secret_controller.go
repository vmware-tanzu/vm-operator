// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

type provider interface {
	UpdateVcCreds(ctx context.Context, data map[string][]byte) error
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &corev1.Secret{}
		controllerName      = "infra-secret"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	vcCredsKey := ctrlclient.ObjectKey{
		Name:      pkgcfg.FromContext(ctx).VCCredsSecretName,
		Namespace: ctx.Namespace,
	}

	// This controller only watches Secrets in the pod namespace.
	cache, err := pkgmgr.NewNamespacedCacheForObject(
		mgr,
		&ctx.SyncPeriod,
		controlledType,
		vcCredsKey.Namespace)
	if err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		cache,
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
		vcCredsKey,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 1,
	})
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(
		cache,
		controlledType,
		&handler.TypedEnqueueRequestForObject[*corev1.Secret]{},
		predicate.TypedFuncs[*corev1.Secret]{
			CreateFunc: func(e event.TypedCreateEvent[*corev1.Secret]) bool {
				return e.Object.GetName() == vcCredsKey.Name
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Secret]) bool {
				return e.ObjectOld.GetName() == vcCredsKey.Name
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Secret]) bool {
				return false
			},
			GenericFunc: func(e event.TypedGenericEvent[*corev1.Secret]) bool {
				return false
			},
		},
		kubeutil.TypedResourceVersionChangedPredicate[*corev1.Secret]{},
	))
}

func NewReconciler(
	ctx context.Context,
	secretReader ctrlclient.Reader,
	logger logr.Logger,
	recorder record.Recorder,
	provider provider,
	vcCredsKey ctrlclient.ObjectKey) *Reconciler {

	return &Reconciler{
		Context:      ctx,
		SecretReader: secretReader,
		Logger:       logger,
		Recorder:     recorder,
		provider:     provider,
		vcCredsKey:   vcCredsKey,
	}
}

type Reconciler struct {
	Context      context.Context
	SecretReader ctrlclient.Reader
	Logger       logr.Logger
	Recorder     record.Recorder
	provider     provider
	vcCredsKey   ctrlclient.ObjectKey
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if req.NamespacedName == r.vcCredsKey {
		return ctrl.Result{}, r.reconcileVcCreds(ctx)
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileVcCreds(ctx context.Context) error {
	secret := corev1.Secret{}
	if err := r.SecretReader.Get(ctx, r.vcCredsKey, &secret); err != nil {
		return ctrlclient.IgnoreNotFound(err)
	}

	r.Logger.Info("Reconciling updated VM Operator credentials", "secret", r.vcCredsKey)
	return r.provider.UpdateVcCreds(ctx, secret.Data)
}
