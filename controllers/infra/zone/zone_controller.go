// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &topologyv1.Zone{}
		controlledTypeName = reflect.TypeFor[topologyv1.Zone]().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorder(controllerNameShort)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 0),
			SkipNameValidation:      SkipNameValidation,
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {
	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Finalizer is the finalizer placed on Zone objects by VM Operator.
const Finalizer = "vmoperator.vmware.com/zone-finalizer"

// Reconciler reconciles a StoragePolicyQuota object.
type Reconciler struct {
	client.Client

	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigpolicies,verbs=get;list;watch;create;update;patch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = watcher.JoinContext(ctx, r.Context)

	var obj topologyv1.Zone

	err := r.Get(ctx, req.NamespacedName, &obj)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		pErr := patchHelper.Patch(ctx, &obj)
		if pErr != nil {
			if reterr == nil {
				reterr = pErr
			} else {
				reterr = fmt.Errorf("%w,%w", pErr, reterr)
			}
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(ctx, &obj)
	}

	return r.ReconcileNormal(ctx, &obj)
}

func (r *Reconciler) ReconcileDelete(
	ctx context.Context,
	obj *topologyv1.Zone) (ctrl.Result, error) {
	if val := obj.Spec.ManagedVMs.FolderMoID; val != "" {
		err := watcher.Remove(
			ctx,
			vimtypes.ManagedObjectReference{
				Type:  "Folder",
				Value: val,
			},
			fmt.Sprintf("%s/%s", obj.Namespace, obj.Name))

		if err != nil && !errors.Is(err, watcher.ErrAsyncSignalDisabled) {
			// We don't ignore watcher.ErrNoWatcher here to interlock with the vm watcher
			// service that is in the process of restarting the watcher. This does mean
			// that if watcher cannot start like because of invalid VC creds the finalizer
			// won't be removed.
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(obj, Finalizer)

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *topologyv1.Zone) (ctrl.Result, error) {
	if controllerutil.AddFinalizer(obj, Finalizer) {
		// Ensure the finalizer is present before watching this zone.
		return ctrl.Result{}, nil
	}

	if val := obj.Spec.ManagedVMs.FolderMoID; val != "" {
		err := watcher.Add(
			ctx,
			vimtypes.ManagedObjectReference{
				Type:  "Folder",
				Value: val,
			},
			fmt.Sprintf("%s/%s", obj.Namespace, obj.Name))

		if err != nil && !errors.Is(err, watcher.ErrAsyncSignalDisabled) {
			return ctrl.Result{}, err
		}
	}

	if pkgcfg.FromContext(ctx).Features.VirtualMachineConfigPolicy {
		err := r.reconcileConfigTargets(ctx, obj)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.reconcileVMConfigPolicy(ctx, obj)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// clusterMoIDsForZone returns the ClusterComputeResource managed object IDs
// of the AvailabilityZone the given Zone is derived from. Zone and
// AvailabilityZone share the same name.
func (r *Reconciler) clusterMoIDsForZone(
	ctx context.Context,
	zone *topologyv1.Zone) ([]string, error) {
	az, err := topology.GetAvailabilityZone(ctx, r.Client, zone.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get availability zone %s: %w", zone.Name, err)
	}

	if len(az.Spec.ClusterComputeResourceMoIDs) > 0 {
		return az.Spec.ClusterComputeResourceMoIDs, nil
	}

	if az.Spec.ClusterComputeResourceMoId != "" {
		return []string{az.Spec.ClusterComputeResourceMoId}, nil
	}

	return nil, nil
}

// reconcileConfigTargets ensures a cluster-scoped ConfigTarget exists for each
// ClusterComputeResource MoID of the zone's availability zone.
func (r *Reconciler) reconcileConfigTargets(
	ctx context.Context,
	zone *topologyv1.Zone) error {
	clusterMoIDs, err := r.clusterMoIDsForZone(ctx, zone)
	if err != nil {
		return err
	}

	for _, clusterMoID := range clusterMoIDs {
		ct := &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterMoID,
			},
		}

		_, err := controllerutil.CreateOrPatch(ctx, r.Client, ct, func() error {
			if ct.UID == "" {
				ct.Spec.ID = vimv1.ManagedObjectID{ID: clusterMoID}
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to reconcile ConfigTarget %s: %w", clusterMoID, err)
		}
	}

	return nil
}

// reconcileVMConfigPolicy ensures a namespace-scoped VirtualMachineConfigPolicy
// exists for the zone. The syncMode is set only on creation to allow manual
// overrides to persist across reconciles.
func (r *Reconciler) reconcileVMConfigPolicy(
	ctx context.Context,
	zone *topologyv1.Zone) error {
	policy := &vimv1.VirtualMachineConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zone.Name,
			Namespace: zone.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, policy, func() error {
		policy.Spec.Zone = zone.Name
		if policy.Spec.SyncMode == "" {
			// Set the default only when the object is new; the kubebuilder
			// defaulting webhook ensures SyncMode is never empty on an existing
			// object, so this preserves any value a user has configured.
			policy.Spec.SyncMode = vimv1.VirtualMachineConfigPolicySyncModeConfigTarget
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile VirtualMachineConfigPolicy %s/%s: %w",
			zone.Namespace, zone.Name, err)
	}

	return nil
}
