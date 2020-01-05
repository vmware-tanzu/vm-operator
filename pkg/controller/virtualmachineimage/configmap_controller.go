/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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

	"github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	OpDeleteVMOpConfig = "DeleteVMOpConfigMap"

	ConfigMapControllerName = "configmapconfig-controller"
)

// Add creates a new VMOperator ConfigMap Controller and adds it to the Manager with default RBAC. The Manager will set fields
// on the Controller and Start it when the Manager is Started.
func AddConfigMapController(mgr manager.Manager) error {
	return addController(mgr, cmReconciler(mgr))
}

// cmReconciler returns a new reconcile.Reconciler
func cmReconciler(mgr manager.Manager) reconcile.Reconciler {
	vmProvider := vmprovider.GetVmProviderOrDie()
	imageDiscoverer := NewVirtualMachineImageDiscoverer(mgr.GetClient(), vmProvider, VirtualMachineImageDiscovererOptions{})

	return &ReconcileVMOpConfigMap{
		Client:          mgr.GetClient(),
		scheme:          mgr.GetScheme(),
		imageDiscoverer: imageDiscoverer,
		vmProvider:      vmProvider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func addController(mgr manager.Manager, r reconcile.Reconciler) error {
	log.Info("Adding ConfigMap Reconciler")
	// Create a new controller.  We only need one reconciler to do the job.
	c, err := controller.New(ConfigMapControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return err
	}

	//Predicate such that a newly created VMOP ConfigMap or changes to the Content Source triggers a Reconcile.
	vmOperatorConfigPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isVmOperatorConfigResource(e.Meta.GetName(), e.Meta.GetNamespace())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isVmOperatorConfigResource(e.MetaNew.GetName(), e.MetaNew.GetNamespace()) &&
				isVmOperatorConfigContentSourceChanged(e.MetaOld, e.MetaNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if isVmOperatorConfigResource(e.Meta.GetName(), e.Meta.GetNamespace()) {
				log.Info("ConfigMap Deleted", "Name", vsphere.VSphereConfigMapName)
				record.EmitEvent(e.Object, OpDeleteVMOpConfig, &err, false)
			}
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// Watch for changes to VM Operator's ConfigMap for CL source changes
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, vmOperatorConfigPredicate)
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVMOpConfigMap{}

type ReconcileVMOpConfigMap struct {
	client.Client
	scheme          *runtime.Scheme
	imageDiscoverer *VirtualMachineImageDiscoverer
	vmProvider      vmprovider.VirtualMachineProviderInterface
}

func (r *ReconcileVMOpConfigMap) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// A newly created VMOP ConfigMap or changes to the Content Source
	// needs to clear the namespace session cache, so that all subsequent requests will create a new session and
	// repopulate the cache.
	r.vmProvider.UpdateVmOpConfigMap(context.TODO())
	//Sync new set of images
	//TODO: Merge thread contexts to sync images with a watch on source.Channel in the vmimage-controller
	err := r.imageDiscoverer.SyncImages()
	if err != nil {
		log.Error(err, "failed to Sync Images after Content Source changed")
	}
	return reconcile.Result{}, nil
}

func isVmOperatorConfigResource(name, namespace string) bool {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	return err == nil && name == vsphere.VSphereConfigMapName &&
		namespace == vmopNamespace
}

func isVmOperatorConfigContentSourceChanged(old, updated metav1.Object) bool {
	oldObj := old.(*corev1.ConfigMap)
	newObj := updated.(*corev1.ConfigMap)

	oldContentSource := oldObj.Data[vsphere.ContentSourceKey]
	newContentSource := newObj.Data[vsphere.ContentSourceKey]

	return oldContentSource != newContentSource
}
