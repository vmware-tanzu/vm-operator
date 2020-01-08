/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infracluster

import (
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	ControllerName = "infracluster-controller"
)

var log = logf.Log.WithName(ControllerName)

// Add creates a new InfraClusterProvider Controller and adds it to the Manager with default RBAC. The Manager will set fields
// on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	// Get provider registered in the manager's main()
	provider := vmprovider.GetVmProviderOrDie()

	return &ReconcileInfraClusterProvider{
		Client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		vmProvider: provider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return err
	}

	// As the value of PNID change is notified through the global config map, the configmap create or update are the feasible
	// candidates to find out the changed PNID value. The event of configmap addition or update needs identifying source configmap
	// and finding out the real change of the PNID.
	wcpClusterConfigPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isWcpClusterConfigResource(e.Meta)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isWcpClusterConfigResource(e.MetaNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// Watch for changes to WCP Cluster's ConfigMap for PNID changes
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, wcpClusterConfigPredicate)
	if err != nil {
		return err
	}

	// Predicate functions for VM operator service account credential secret
	vmOpCredsSecretPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isVmOpServiceAccountCredSecret(e.Meta.GetName(), e.Meta.GetNamespace())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isVmOpServiceAccountCredSecret(e.MetaNew.GetName(), e.MetaNew.GetNamespace())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, vmOpCredsSecretPredicate)
}

type ReconcileInfraClusterProvider struct {
	client.Client
	scheme     *runtime.Scheme
	vmProvider vmprovider.VirtualMachineProviderInterface
}

// Reconcile clears all the vCenter sessions and update the VMOP config map with the new PNID.
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
// +kubebuilder:rbac:groups=v1,resources=configmaps,verbs=get;watch
func (r *ReconcileInfraClusterProvider) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("Received reconcile request", "namespace", request.Namespace, "name", request.Name)
	ctx := context.Background()

	// The reconcile request can be either a VCPNID update or a VM operator secret rotation. We check on the namespacedName to differentiate the two.
	// This is an anti pattern. Usually a controller should only reconcile one object type.
	// If the namespaced names of ConfigMap or Secret change, this code needs to be updated.
	if isVmOpServiceAccountCredSecret(request.Name, request.Namespace) {
		r.vmProvider.UpdateVmOpSACredSecret(ctx)
		return reconcile.Result{}, nil
	}

	err := r.reconcileVcPNID(ctx, request)

	return reconcile.Result{}, err
}

func (r *ReconcileInfraClusterProvider) reconcileVcPNID(ctx context.Context, request reconcile.Request) error {
	log.V(4).Info("Reconciling VC PNID")
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

func isVmOpServiceAccountCredSecret(name, namespace string) bool {
	vmopNamespace, vmopNamespaceExists := os.LookupEnv(vsphere.VmopNamespaceEnv)
	return vmopNamespaceExists && vmopNamespace == namespace && name == vsphere.VmOpSecretName
}

func isWcpClusterConfigResource(obj metav1.Object) bool {
	return obj.GetName() == vsphere.WcpClusterConfigMapName && obj.GetNamespace() == vsphere.WcpClusterConfigMapNamespace
}
