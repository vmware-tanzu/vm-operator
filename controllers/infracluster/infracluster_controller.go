/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infracluster

import (
	goctx "context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vmware-tanzu/vm-operator/pkg/context"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	controllerName = "infracluster-controller"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	return add(mgr, newReconciler(ctx, mgr))
}

func newReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileInfraClusterProvider{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("InfraCluster"),
		Scheme:     mgr.GetScheme(),
		vmProvider: ctx.VmProvider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Predicate functions to determine when a reconcile should be triggered for the "Watch"ed resources.
	// We detect if a resource has been modified by comparing the ResourceVersion of old and new resources.
	vmOpServiceAccountSecretPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isVmOpServiceAccountCredSecret(e.Meta.GetName(), e.Meta.GetNamespace())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !isVmOpServiceAccountCredSecret(e.MetaOld.GetName(), e.MetaOld.GetNamespace()) {
				return false
			}

			predicateInstance := predicate.ResourceVersionChangedPredicate{}
			return predicateInstance.Update(e)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	wcpClusterConfigMapPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isWcpClusterConfigMap(e.Meta.GetName(), e.Meta.GetNamespace())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !isWcpClusterConfigMap(e.MetaOld.GetName(), e.MetaOld.GetNamespace()) {
				return false
			}

			predicateInstance := predicate.ResourceVersionChangedPredicate{}
			return predicateInstance.Update(e)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// Watch for ConfigMap and Secret resource types.
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, wcpClusterConfigMapPredicate)
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, vmOpServiceAccountSecretPredicate)
}

type ReconcileInfraClusterProvider struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	vmProvider vmprovider.VirtualMachineProviderInterface
}

// Reconcile clears all the vCenter sessions when a VCPNID is updated in wcp-cluster-config ConfigMap or a Service Account Secret is rotated.
//
// PNID, aka Primary Network Identifier, is the unique vCenter server name given to the vCenter server during deployment.
// Generally, this will be the FQDN of the machine if the IP is resolvable; otherwise, IP is taken as PNID.
//
// The VMOP config map is installed with initial PNID at the time of the Supervisor enable workflow. How the wcpsvc communicates
// the PNID changes to components running on API master (like schedExt/imageSvc etc) is by updating a global kube-system/wcp-cluster-config
// ConfigMap. This update happens automatically as a part of the wcpsvc controller. Components like schedExt watch this configmap and
// update the information internally. VMOP, like other controllers, listens on the changes in the global configmap, refresh its internal data,
// and cleans up any connected vCenter sessions.
//
// The vSphere VM provider registration would create a singleton VM provider object with SessionManger to hold a per-namespace session map with
// each session establishes a client connection with vCenter. The per-namespace client would turn out to be a per-manager client in the future
// to reduce the in-house management connections to vCenter.
//
// This controller refreshes the VMOP config map and clears active sessions upon receiving create or update event on kube-system/wcp-cluster-config.
//
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *ReconcileInfraClusterProvider) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.Log.Info("Received reconcile request", "namespace", request.Namespace, "name", request.Name)
	ctx := goctx.Background()

	// The reconcile request can be either a VCPNID update or a VM operator secret rotation. We check on the namespacedName to differentiate the two.
	// This is an anti pattern. Usually a controller should only reconcile one object type.
	// If the namespaced names of ConfigMap or Secret change, this code needs to be updated.
	if isVmOpServiceAccountCredSecret(request.Name, request.Namespace) {
		r.Log.Info("VM operator secret has been updated. Going to invalidate session cache for all namespaces")
		r.vmProvider.UpdateVmOpSACredSecret(ctx)
		return reconcile.Result{}, nil
	}

	err := r.reconcileVcPNID(ctx, request)

	return reconcile.Result{}, err
}

func (r *ReconcileInfraClusterProvider) reconcileVcPNID(ctx goctx.Context, request reconcile.Request) error {
	r.Log.V(4).Info("Reconciling VC PNID")
	instance := &corev1.ConfigMap{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}

	return r.vmProvider.UpdateVcPNID(ctx, instance)
}

func isVmOpServiceAccountCredSecret(secretName, secretNamespace string) bool {
	vmOpNamespace, err := lib.GetVmOpNamespaceFromEnv()
	return err == nil && vmOpNamespace == secretNamespace && secretName == vsphere.VmOpSecretName
}

func isWcpClusterConfigMap(configMapName, configMapNamespace string) bool {
	return configMapName == vsphere.WcpClusterConfigMapName && configMapNamespace == vsphere.WcpClusterConfigMapNamespace
}
