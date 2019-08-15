/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineclass

import (
	"context"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new VirtualMachineClass Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileVirtualMachineClass{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualmachineclass-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: common.GetMaxReconcileNum()})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachineClass
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachineClass{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineClass{}

// ReconcileVirtualMachineClass reconciles a VirtualMachineClass object
type ReconcileVirtualMachineClass struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a VirtualMachineClass object and makes changes based on the state read
// and what is in the VirtualMachineClass.Spec
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses/status,verbs=get;update;patch
func (r *ReconcileVirtualMachineClass) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the VirtualMachineClass instance
	instance := &vmoperatorv1alpha1.VirtualMachineClass{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
