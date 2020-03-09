// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

/*
import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1a2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gcmv1 "gitlab.eng.vmware.com/core-build/guest-cluster-controller/apis/gcm/v1alpha1"
	"gitlab.eng.vmware.com/core-build/guest-cluster-controller/controllers/util/remote"
	"gitlab.eng.vmware.com/core-build/guest-cluster-controller/pkg/context"
	"gitlab.eng.vmware.com/core-build/guest-cluster-controller/pkg/record"
)

const (
	clusterNotReadyRequeueTime = time.Minute * 2
)
*/

// Reconciler is a base type for builder's reconcilers
type Reconciler interface{}

// NewReconcilerFunc is a base type for functions that return a reconciler
type NewReconcilerFunc func(client.Client) Reconciler

// VirtualMachineReconciler is used to create a new Controller
type VirtualMachineReconciler interface {
	Reconciler

	// ReconcileNormal is called when the object is not scheduled for deletion.
	ReconcileNormal(*context.VirtualMachineContext) (reconcile.Result, error)
}

// VirtualMachineOperatorReconcilerWithDelete is a Reconciler that implements
// explicit destruction logic.
type VirtualMachineReconcilerWithDelete interface {
	VirtualMachineReconciler

	// ReconcileDelete is called when the object is scheduled for deletion.
	ReconcileDelete(*context.VirtualMachineContext) (reconcile.Result, error)
}

/*

// NewController returns a new controller for responding to updates for a ManagedCluster
// and/or a CAPI Cluster once the guest cluster's API server is available.
func NewController(
	ctx *context.ControllerManagerContext,
	mgr manager.Manager,
	controllerName string,
	reconciler GuestClusterReconciler) (ctrl.Controller, error) {

	var (
		controllerNameShort = controllerName
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	// Build the controller context.
	controllerContext := &context.ControllerContext{
		ControllerManagerContext: ctx,
		Name:                     controllerNameShort,
		Recorder:                 record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		Logger:                   ctx.Logger.WithName(controllerNameShort),
	}

	// Initialize a new controller.
	controller, err := ctrl.New(
		controllerName, mgr,
		ctrl.Options{
			Reconciler: &guestClusterReconciler{
				ControllerContext: controllerContext,
				reconciler:        reconciler,
			},
		})
	if err != nil {
		return nil, err
	}

	// Define the types used in the watch.
	managedCluster := &gcmv1.ManagedCluster{}

	// When watching a ManagedCluster, we only care about Create and Update events.
	managedClusterPredicates := predicate.Funcs{
		// We must watch Create events. There are corner cases where a controller can restart against a
		// ManagedCluster that has yet to be updated
		CreateFunc: func(event.CreateEvent) bool { return true },
		// Reconcile on update. Don't test for equality as the DefaultSyncTime reconciles happen through this path
		UpdateFunc:  func(e event.UpdateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}

	// Watch for update events for ManagedCluster resources.
	controllerContext.Logger.Info("Adding watch on ManagedCluster objects")

	if err := controller.Watch(
		&source.Kind{Type: managedCluster},
		&handler.EnqueueRequestForObject{},
		managedClusterPredicates); err != nil {

		return nil, errors.Wrap(err, "failed to watch ManagedCluster")
	}

	// Watch for update events for CAPI v1alpha2 Cluster resources.
	controllerContext.Logger.Info("Adding watch on CAPI v1alpha2 Cluster resources")
	if err := watchV1Alpha2Cluster(controllerContext, controller); err != nil {
		return nil, errors.Wrap(err, "failed to watch CAPI v1alpha2 Cluster")
	}

	return controller, nil
}

type guestClusterReconciler struct {
	*context.ControllerContext
	reconciler GuestClusterReconciler
}

func (r *guestClusterReconciler) Reconcile(req reconcile.Request) (_ reconcile.Result, reterr error) {
	r.ControllerContext.Logger.V(4).Info("Starting Reconcile")

	// Get the managed cluster for this request.
	managedCluster := &gcmv1.ManagedCluster{}
	managedClusterKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := r.Client.Get(r, managedClusterKey, managedCluster); err != nil {
		if apierrors.IsNotFound(err) {
			r.Logger.Info("Cluster not found, won't reconcile", "cluster", managedClusterKey)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Create the patch helper.
	patchHelper, err := patch.NewHelper(managedCluster, r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(
			err,
			"failed to init patch helper for %s %s/%s",
			managedCluster.GroupVersionKind(),
			managedCluster.Namespace,
			managedCluster.Name)
	}

	// Create the cluster context for this request.
	clusterContext := &context.ClusterContext{
		ControllerContext: r.ControllerContext,
		Cluster:           managedCluster,
		Logger:            r.Logger.WithName(req.Namespace).WithName(req.Name),
		PatchHelper:       patchHelper,
	}

	// Always issue a patch when exiting this function so changes to the
	// resource are patched back to the API server.
	defer func() {
		if err := clusterContext.Patch(); err != nil {
			if reterr == nil {
				reterr = err
			} else {
				clusterContext.Logger.Error(err, "patch failed", "cluster", clusterContext.String())
			}
		}
	}()

	// This type of controller doesn't care about delete events.
	if !managedCluster.DeletionTimestamp.IsZero() {
		if reconciler, ok := r.reconciler.(GuestClusterReconcilerWithDelete); ok {
			return reconciler.ReconcileDelete(clusterContext)
		}
		return reconcile.Result{}, nil
	}

	// We cannot proceed until we are able to access the target cluster. Until
	// then just return a no-op and wait for the next sync. This will occur when
	// the Cluster's status is updated with a reference to the secret that has
	// the Kubeconfig data used to access the target cluster.
	guestClient, err := remote.NewClusterClient(clusterContext, clusterContext.Client, clusterContext.Cluster.Namespace,
		clusterContext.Cluster.Name, clusterContext.GuestClusterClientTimeout)
	if err != nil {
		clusterContext.Logger.Info("The control plane is not ready yet")
		return reconcile.Result{RequeueAfter: clusterNotReadyRequeueTime}, nil
	}

	// All controllers should wait until the PSP are created and bind successfully by DefaultPSP controller.
	for _, requiredComponent := range r.reconciler.RequiredComponents() {
		switch requiredComponent {
		case DefaultPSP:
			// Do not reconcile until the default PSP are created.
			if clusterContext.Cluster.Status.Addons.PSP == nil || !clusterContext.Cluster.Status.Addons.PSP.IsApplied() {
				clusterContext.Logger.Info("Skipping reconcile until ManagedCluster.Status.Addons.PSP.Applied is true",
					"cluster", clusterContext.String())
				// No need to requeue because the change of Cluster.Status.Addons.PSP.Applied will trigger reconcile.
				return reconcile.Result{}, nil
			}
		}
	}

	// Defer to the Reconciler for reconciling a non-delete event.
	return r.reconciler.ReconcileNormal(&context.GuestClusterContext{
		ClusterContext: clusterContext,
		GuestClient:    guestClient,
	})
}

func watchCAPICluster(ctx *context.ControllerContext, controller ctrl.Controller, cluster runtime.Object) error {
	managedCluster := &gcmv1.ManagedCluster{}

	// When watching a Cluster, we only care about Update events.
	// Should not be necessary to watch Create events since we watch ManagedCluster Create and
	//   there are no circumstances in which a ManagedCluster and Cluster can be created separately
	clusterPredicates := predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			for _, ownerRef := range e.MetaNew.GetOwnerReferences() {
				if ownerRef.Name == e.MetaNew.GetName() {
					// We don't have this check in ManagedCluster Update ensuring that every DefaultTimeSync
					// we're guaranteed to have at least one Reconcile.
					// If we didn't have this check here, we'd get at least two Reconciles every DefaultTimeSync
					return e.MetaOld.GetResourceVersion() != e.MetaNew.GetResourceVersion()
				}
			}
			return false
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}

	// Watch for updates to the CAPI Cluster.
	if err := controller.Watch(
		&source.Kind{Type: cluster},
		&handler.EnqueueRequestForOwner{IsController: true, OwnerType: managedCluster},
		clusterPredicates); err != nil {

		return err
	}

	return nil
}

func watchV1Alpha2Cluster(ctx *context.ControllerContext, controller ctrl.Controller) error {
	return watchCAPICluster(ctx, controller, &clusterv1a2.Cluster{})
}
*/
