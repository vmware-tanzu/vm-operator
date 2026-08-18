// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigpolicy

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util/configpolicysync"
)

const (
	// zoneByClusterMoIDIndex is the field index key used to look up Zones by
	// one of the cluster MoIDs in spec.managedVMs.clusterMoIDs, so a
	// ConfigTarget watch event can find every Zone whose sync result depends
	// on that ConfigTarget.
	zoneByClusterMoIDIndex = "spec.managedVMs.clusterMoIDs"

	// policyByZoneIndex is the field index key used to look up
	// VirtualMachineConfigPolicy objects by spec.zone, so a Zone (directly)
	// or ConfigTarget (via zoneByClusterMoIDIndex) watch event can find
	// every policy that references it.
	policyByZoneIndex = "spec.zone"
)

const (
	// SyncDisabledReason is the Ready condition reason used when
	// spec.syncMode=Disabled, so reconciliation intentionally does not
	// touch spec.
	SyncDisabledReason = "SyncDisabled"

	// ZoneNotFoundReason is the Ready condition reason used when
	// spec.zone does not resolve to an existing Zone. This is the
	// controller-side complement to the admission webhook's spec.zone
	// check: the webhook rejects a policy created or updated with a bad
	// reference; this handles a Zone deleted after the policy already
	// exists.
	ZoneNotFoundReason = "ZoneNotFound"

	// ConfigTargetNotFoundReason is the Ready condition reason used when
	// the zone has no cluster MoIDs to sync from.
	ConfigTargetNotFoundReason = "ConfigTargetNotFound"

	// ConfigTargetNotReadyReason is the Ready condition reason used when
	// one or more of the zone's cluster MoIDs do not resolve to a Ready
	// ConfigTarget. spec is left untouched in this case: a ConfigTarget
	// with Ready!=True has not finished populating its status, and merging
	// its zero-valued fields would publish a false capability denial rather
	// than "unknown," since a zero maximum on a numeric field means "not
	// supported" per VirtualMachineConfigPolicySpec's doc comments.
	ConfigTargetNotReadyReason = "ConfigTargetNotReady"

	// InvalidRangeReason is the Ready condition reason used when a
	// cluster's reported capability has narrowed below an existing,
	// tenant-managed Min on one or more of the policy's range fields.
	// configpolicysync.Merge leaves each such field unchanged rather than
	// publishing a Min > Max range for it, but every other
	// ConfigTarget-derived field -- including other range fields -- still
	// converges normally, so this reason does not mean spec as a whole is
	// untouched.
	InvalidRangeReason = "InvalidRange"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vimv1.VirtualMachineConfigPolicy{}
		controlledTypeName = reflect.TypeFor[vimv1.VirtualMachineConfigPolicy]().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorder(controllerNameShort)))

	err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vimv1.VirtualMachineConfigPolicy{},
		policyByZoneIndex,
		func(rawObj client.Object) []string {
			policy := rawObj.(*vimv1.VirtualMachineConfigPolicy) //nolint:forcetypeassert

			// A Disabled policy never syncs, so it has no reason to be
			// woken by a Zone/ConfigTarget event -- omitting it here keeps
			// zoneToPolicyMapper/configTargetToPolicyMapper's List calls
			// from enqueuing (and patching status.observedGeneration on)
			// every Disabled policy in the namespace on every capability
			// change. A policy transitioning to/from Disabled still
			// reconciles immediately via For(controlledType) on its own
			// update, so nothing is lost.
			if policy.Spec.Zone == "" || policy.Spec.SyncMode == vimv1.VirtualMachineConfigPolicySyncModeDisabled {
				return nil
			}

			return []string{policy.Spec.Zone}
		})
	if err != nil {
		return fmt.Errorf("failed to index VirtualMachineConfigPolicy by spec.zone: %w", err)
	}

	err = mgr.GetFieldIndexer().IndexField(
		ctx,
		&topologyv1.Zone{},
		zoneByClusterMoIDIndex,
		func(rawObj client.Object) []string {
			zone := rawObj.(*topologyv1.Zone) //nolint:forcetypeassert
			return zone.Spec.ManagedVMs.ClusterMoIDs
		})
	if err != nil {
		return fmt.Errorf("failed to index Zone by spec.managedVMs.clusterMoIDs: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Watches(
			&topologyv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(zoneToPolicyMapper(r.Client))).
		Watches(
			&vimv1.ConfigTarget{},
			handler.EnqueueRequestsFromMapFunc(configTargetToPolicyMapper(r.Client))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 0),
			SkipNameValidation:      SkipNameValidation,
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

// zoneToPolicyMapper returns reconcile requests for every
// VirtualMachineConfigPolicy in the Zone's namespace whose spec.zone
// references it, so a Zone change (e.g. its cluster MoIDs) is reflected on
// the next reconcile.
func zoneToPolicyMapper(c client.Client) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		zone, ok := o.(*topologyv1.Zone)
		if !ok {
			return nil
		}

		return policiesReferencingZone(ctx, c, zone.Namespace, zone.Name)
	}
}

// configTargetToPolicyMapper returns reconcile requests for every
// VirtualMachineConfigPolicy whose zone lists the ConfigTarget's cluster
// MoID (the ConfigTarget's name) among spec.managedVMs.clusterMoIDs, so a
// ConfigTarget status change is reflected within one reconcile, per the
// spec's requirement that config policy stays in sync with the ConfigTarget.
func configTargetToPolicyMapper(c client.Client) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		ct, ok := o.(*vimv1.ConfigTarget)
		if !ok {
			return nil
		}

		var zones topologyv1.ZoneList

		err := c.List(ctx, &zones, client.MatchingFields{zoneByClusterMoIDIndex: ct.Name})
		if err != nil {
			return nil
		}

		var requests []reconcile.Request

		for i := range zones.Items {
			zone := &zones.Items[i]
			requests = append(requests, policiesReferencingZone(ctx, c, zone.Namespace, zone.Name)...)
		}

		return requests
	}
}

// policiesReferencingZone lists every VirtualMachineConfigPolicy in
// namespace whose spec.zone equals zoneName.
func policiesReferencingZone(
	ctx context.Context,
	c client.Client,
	namespace, zoneName string) []reconcile.Request {
	var policies vimv1.VirtualMachineConfigPolicyList

	err := c.List(ctx, &policies,
		client.InNamespace(namespace),
		client.MatchingFields{policyByZoneIndex: zoneName})
	if err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(policies.Items))
	for i := range policies.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&policies.Items[i])}
	}

	return requests
}

// NewReconciler returns a new Reconciler for VirtualMachineConfigPolicy.
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

// Reconciler reconciles a VirtualMachineConfigPolicy's spec with the
// capabilities reported by the ConfigTarget(s) behind its zone.
type Reconciler struct {
	client.Client

	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets,verbs=get;list;watch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	logger := pkglog.FromContextOrDefault(ctx)
	logger = logger.WithName(req.String())
	ctx = logr.NewContext(ctx, logger)

	var obj vimv1.VirtualMachineConfigPolicy

	err := r.Get(ctx, req.NamespacedName, &obj)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	base := obj.DeepCopy()

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", req, err)
	}

	defer func() {
		err := patchHelper.Patch(ctx, &obj)
		if err != nil {
			if reterr == nil {
				reterr = err
			}

			logger.Error(err, "patch failed")
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.ReconcileNormal(ctx, base, &obj)
}

// ReconcileNormal reconciles obj's spec against the ConfigTarget(s) behind
// its zone when spec.syncMode=ConfigTarget, and otherwise leaves spec
// untouched. base is obj's state as read, before this reconcile's changes;
// it is used to detect a no-op spec write so an unchanged sync does not
// bump resourceVersion and re-trigger this controller's own watch.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	base *vimv1.VirtualMachineConfigPolicy,
	obj *vimv1.VirtualMachineConfigPolicy) error {
	logger := pkglog.FromContextOrDefault(ctx)

	if obj.Spec.SyncMode == vimv1.VirtualMachineConfigPolicySyncModeDisabled {
		pkgcond.Set(obj, &metav1.Condition{
			Type:   vimv1.ReadyConditionType,
			Status: metav1.ConditionTrue,
			Reason: SyncDisabledReason,
		})
		obj.Status.ObservedGeneration = obj.Generation

		return nil
	}

	zone, err := topology.GetZone(ctx, r.Client, obj.Spec.Zone, obj.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, ZoneNotFoundReason,
				"zone %q not found", obj.Spec.Zone)
			obj.Status.ObservedGeneration = obj.Generation

			return nil
		}

		pkgcond.MarkError(obj, vimv1.ReadyConditionType, ZoneNotFoundReason, err)

		return fmt.Errorf("failed to get zone %q: %w", obj.Spec.Zone, err)
	}

	if len(zone.Spec.ManagedVMs.ClusterMoIDs) == 0 {
		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, ConfigTargetNotFoundReason,
			"zone %q has no cluster to sync from", zone.Name)
		obj.Status.ObservedGeneration = obj.Generation

		return nil
	}

	targets, notReady, err := r.getConfigTargets(ctx, zone.Spec.ManagedVMs.ClusterMoIDs)
	if err != nil {
		pkgcond.MarkError(obj, vimv1.ReadyConditionType, ConfigTargetNotReadyReason, err)
		return fmt.Errorf("failed to get config targets for zone %q: %w", zone.Name, err)
	}

	if len(notReady) > 0 {
		// Require every cluster behind the zone to have a Ready
		// ConfigTarget before syncing: merging a subset would compute an
		// intersection over fewer clusters than the zone actually spans,
		// which is a strictly wider (more permissive) result than the true
		// intersection -- the opposite of the safe direction for data that
		// feeds capability enforcement.
		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, ConfigTargetNotReadyReason,
			"ConfigTarget(s) not ready for cluster(s) %v", notReady)
		obj.Status.ObservedGeneration = obj.Generation

		return nil
	}

	mergedSpec, err := configpolicysync.Merge(obj.Spec, targets...)

	if !apiequality.Semantic.DeepEqual(base.Spec, mergedSpec) {
		obj.Spec = mergedSpec
	}

	obj.Status.ObservedGeneration = obj.Generation

	if err != nil {
		// Merge still converged every field it could; only the field(s)
		// named in err were left unchanged. Apply the spec above as usual
		// and only use Ready=False to surface the conflict.
		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, InvalidRangeReason, "%v", err)

		return nil
	}

	pkgcond.MarkTrue(obj, vimv1.ReadyConditionType)

	logger.V(4).Info("Reconciled VirtualMachineConfigPolicy", "zone", zone.Name, "clusters", len(targets))

	return nil
}

// getConfigTargets returns the ConfigTarget status of every clusterMoID that
// resolves to a Ready ConfigTarget, and the subset of clusterMoIDs that do
// not (missing, or present but Ready!=True). Every clusterMoID behind the
// zone must be Ready before the caller may sync: a ConfigTarget that has not
// finished populating its status still reports zero-valued numeric fields,
// and a zero maximum is a real capability denial per
// VirtualMachineConfigPolicySpec's field docs, not "no data yet." Merging a
// not-yet-Ready target's status, or silently dropping a missing one from a
// multi-cluster zone's intersection, would therefore compute a result no
// narrower than -- and possibly wider (more permissive) than -- the true
// intersection across every cluster the zone spans.
func (r *Reconciler) getConfigTargets(
	ctx context.Context,
	clusterMoIDs []string) (targets []vimv1.ConfigTargetStatus, notReady []string, err error) {
	targets = make([]vimv1.ConfigTargetStatus, 0, len(clusterMoIDs))

	for _, clusterMoID := range clusterMoIDs {
		var ct vimv1.ConfigTarget

		getErr := r.Get(ctx, client.ObjectKey{Name: clusterMoID}, &ct)
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				notReady = append(notReady, clusterMoID)
				continue
			}

			return nil, nil, fmt.Errorf("failed to get ConfigTarget %q: %w", clusterMoID, getErr)
		}

		if !pkgcond.IsTrue(&ct, vimv1.ReadyConditionType) {
			notReady = append(notReady, clusterMoID)
			continue
		}

		targets = append(targets, ct.Status)
	}

	return targets, notReady, nil
}
