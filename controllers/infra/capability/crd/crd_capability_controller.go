// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

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

	"github.com/vmware-tanzu/vm-operator/controllers/infra/capability/exit"
	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			NeedLeaderElection:      ptr.To(false),
		}).
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

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj capv1.Capabilities
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
	}

	if capabilities.UpdateCapabilitiesFeatures(ctx, obj) {
		r.Logger.Info("killing pod due to changed capabilities")
		exit.Exit()
	}

	return ctrl.Result{}, nil
}
