// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability

import (
	"context"
	"fmt"
	"strconv"

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
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

const (
	// WCPClusterCapabilitiesConfigMapName is the name of the wcp-cluster-capabilities ConfigMap.
	WCPClusterCapabilitiesConfigMapName = "wcp-cluster-capabilities"

	// WCPClusterCapabilitiesNamespace is the namespace of the wcp-cluster-capabilities
	// ConfigMap.
	WCPClusterCapabilitiesNamespace = "kube-system"

	// TKGMultipleCLCapabilityKey is the name of capability key defined in wcp-cluster-capabilities ConfigMap.
	TKGMultipleCLCapabilityKey = "MultipleCL_For_TKG_Supported"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType      = &corev1.ConfigMap{}
		controllerName      = "capability-configmap"
		controllerNameShort = fmt.Sprintf("%s-controller", controllerName)
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
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
		WCPClusterCapabilitiesNamespace)
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(
		cache,
		controlledType,
		&handler.TypedEnqueueRequestForObject[*corev1.ConfigMap]{},
		predicate.TypedFuncs[*corev1.ConfigMap]{
			CreateFunc: func(e event.TypedCreateEvent[*corev1.ConfigMap]) bool {
				return e.Object.GetName() == WCPClusterCapabilitiesConfigMapName
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.ConfigMap]) bool {
				return e.ObjectOld.GetName() == WCPClusterCapabilitiesConfigMapName
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
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {
	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

type Reconciler struct {
	client.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if req.Name == WCPClusterCapabilitiesConfigMapName && req.Namespace == WCPClusterCapabilitiesNamespace {
		return ctrl.Result{}, r.reconcileWcpClusterCapabilitiesConfig(ctx, req)
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileWcpClusterCapabilitiesConfig(ctx context.Context, req ctrl.Request) error {
	r.Logger.Info("Reconciling WCP Cluster Capabilities Config", "configMap", req.NamespacedName)

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		return client.IgnoreNotFound(err)
	}

	// The SetContext call impacts the configuration available to contexts throughout the process,
	// not just *this* context or its children. Please refer to the pkg/config package for more information
	// on SetContext and its behavior.
	pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
		config.Features.TKGMultipleCL = isTKGMultipleCLSupported(cm.Data)
	})

	return nil
}

// isTKGMultipleCLSupported returns if MultipleCL_For_TKG_Supported is enabled or not in the
// wcp-cluster-capabilities ConfigMap.
func isTKGMultipleCLSupported(data map[string]string) bool {
	ok, _ := strconv.ParseBool(data[TKGMultipleCLCapabilityKey])
	return ok
}
