// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infracluster

import (
	goctx "context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	// VcCredsSecretName is the credential secret that stores the VM operator service provider user credentials.
	VcCredsSecretName = "wcp-vmop-sa-vc-auth" //nolint:gosec
)

type infraClusterProvider interface {
	UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error
	ResetVcClient(ctx goctx.Context)
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerName      = "infracluster"
		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controllerName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	var provider infraClusterProvider
	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		provider = ctx.VMProviderA2
	} else {
		provider = ctx.VMProvider
	}

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.Namespace, // Aka lib.GetVmOpNamespaceFromEnv()
		provider,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	err = addCredSecretWatch(mgr, c, ctx.SyncPeriod, r.vmOpNamespace)
	if err != nil {
		return err
	}

	err = addWcpClusterCMWatch(mgr, c, ctx.SyncPeriod)
	if err != nil {
		return err
	}

	return nil
}

func addCredSecretWatch(mgr manager.Manager, c controller.Controller, syncPeriod time.Duration, ns string) error {
	nsCache, err := pkgmgr.NewNamespaceCache(mgr, &syncPeriod, ns)
	if err != nil {
		return err
	}

	return c.Watch(source.NewKindWithCache(&corev1.Secret{}, nsCache), &handler.EnqueueRequestForObject{},
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

func addWcpClusterCMWatch(mgr manager.Manager, c controller.Controller, syncPeriod time.Duration) error {
	nsCache, err := pkgmgr.NewNamespaceCache(mgr, &syncPeriod, WcpClusterConfigMapNamespace)
	if err != nil {
		return err
	}

	return c.Watch(source.NewKindWithCache(&corev1.ConfigMap{}, nsCache), &handler.EnqueueRequestForObject{},
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

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmOpNamespace string,
	provider infraClusterProvider) *Reconciler {
	return &Reconciler{
		Client:        client,
		Logger:        logger,
		Recorder:      recorder,
		vmOpNamespace: vmOpNamespace,
		provider:      provider,
	}
}

type Reconciler struct {
	client.Client
	Logger        logr.Logger
	Recorder      record.Recorder
	vmOpNamespace string
	provider      infraClusterProvider
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (ctrl.Result, error) {
	// This is totally wrong and we should break this controller apart so we're not
	// watching different types.

	if req.Name == VcCredsSecretName && req.Namespace == r.vmOpNamespace {
		r.reconcileVcCreds(ctx, req)
		return ctrl.Result{}, nil
	}

	if req.Name == WcpClusterConfigMapName && req.Namespace == WcpClusterConfigMapNamespace {
		return ctrl.Result{}, r.reconcileWcpClusterConfig(ctx, req)
	}

	r.Logger.Error(nil, "Reconciling unexpected object", "req", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileVcCreds(ctx goctx.Context, req ctrl.Request) {
	r.Logger.Info("Reconciling updated VM Operator credentials", "secret", req.NamespacedName)
	r.provider.ResetVcClient(ctx)
}

func (r *Reconciler) reconcileWcpClusterConfig(ctx goctx.Context, req ctrl.Request) error {
	r.Logger.V(4).Info("Reconciling WCP Cluster Config", "configMap", req.NamespacedName)

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
