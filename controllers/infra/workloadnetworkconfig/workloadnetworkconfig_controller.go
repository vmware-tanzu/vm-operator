// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package workloadnetworkconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgexit "github.com/vmware-tanzu/vm-operator/pkg/exit"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	netsetutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/networksettings"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &netopv1alpha1.WorkloadNetworkConfiguration{}
		controllerName      = "workload-network-config"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		record.New(mgr.GetEventRecorder(controllerNameShort)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 1),
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj ctrlclient.Object) bool {
			return obj.GetName() == netsetutil.WorkloadNetworkConfigurationName
		})).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client ctrlclient.Client,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Recorder: recorder,
	}
}

// Reconciler watches WorkloadNetworkConfiguration and restarts the pod when
// the set of declared network providers changes.
type Reconciler struct {
	Context  context.Context
	Client   ctrlclient.Client
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=netoperator.vmware.com,resources=workloadnetworkconfigurations,verbs=get;list;watch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (ctrl.Result, error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)
	providerTypes := netsetutil.GetClusterSupportedProviderTypesFromConfig(ctx)

	logger := pkglog.FromContextOrDefault(ctx)
	logger.Info("Reconciling WorkloadNetworkConfiguration",
		"currentProviderTypes", providerTypes)

	var wnc netopv1alpha1.WorkloadNetworkConfiguration
	if err := r.Client.Get(ctx, req.NamespacedName, &wnc); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
	}

	newProviderTypes, err := netsetutil.GetClusterSupportedProviderTypesFromWNC(&wnc)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to determine new network provider types: %w", err)
	}

	// The order in the WNC does not matter for us so compare sets.
	if sets.New(providerTypes...).Equal(sets.New(newProviderTypes...)) {
		return ctrl.Result{}, nil
	}

	reason := fmt.Sprintf(
		"network providers have changed: %v -> %v",
		providerTypes, newProviderTypes)

	if err := pkgexit.Restart(ctx, r.Client, reason); err != nil {
		logger.Error(err, "Failed to exit due to network provider change")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
