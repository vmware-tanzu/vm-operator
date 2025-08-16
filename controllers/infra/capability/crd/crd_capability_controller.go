// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgexit "github.com/vmware-tanzu/vm-operator/pkg/exit"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &capv1.Capabilities{}
		controllerName      = "capability-crd"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			LogConstructor:          pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.TypedCreateEvent[ctrlclient.Object]) bool {
				return true
			},
			UpdateFunc: func(e event.TypedUpdateEvent[ctrlclient.Object]) bool {
				return true
			},
			DeleteFunc: func(e event.TypedDeleteEvent[ctrlclient.Object]) bool {
				return false
			},
		}).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client ctrlclient.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

type Reconciler struct {
	Context  context.Context
	Client   ctrlclient.Client
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=iaas.vmware.com,resources=capabilities,verbs=get;list;watch
// +kubebuilder:rbac:groups=iaas.vmware.com,resources=capabilities/status,verbs=get

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (ctrl.Result, error) {

	logger := pkgutil.FromContextOrDefault(ctx)
	logger.Info("Reconciling capabilities")

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj capv1.Capabilities
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
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
