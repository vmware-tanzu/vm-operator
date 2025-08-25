// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policyevaluation

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vspherepolv1.PolicyEvaluation{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf(
			"%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong = fmt.Sprintf(
			"%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			LogConstructor: pkgutil.ControllerLogConstructor(
				controllerNameShort,
				controlledType,
				mgr.GetScheme()),
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

// Finalizer is the finalizer placed on objects by VM Operator.
const Finalizer = "vmoperator.vmware.com/policy-evaluation-finalizer"

// Reconciler reconciles a PolicyEvaluation object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=placement.vmware.com,resources=policyevaluations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=placement.vmware.com,resources=policyevaluations/status,verbs=get
// +kubebuilder:rbac:groups=placement.vmware.com,resources=computepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=placement.vmware.com,resources=computepolicies/status,verbs=get
// +kubebuilder:rbac:groups=placement.vmware.com,resources=tagpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=placement.vmware.com,resources=tagpolicies/status,verbs=get

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj vspherepolv1.PolicyEvaluation
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, &obj); err != nil {
			if reterr == nil {
				reterr = err
			} else {
				reterr = fmt.Errorf("%w,%w", err, reterr)
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
	obj *vspherepolv1.PolicyEvaluation) (ctrl.Result, error) {

	controllerutil.RemoveFinalizer(obj, Finalizer)

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) (ctrl.Result, error) {

	if controllerutil.AddFinalizer(obj, Finalizer) {
		// Ensure the finalizer is present before reconciling further.
		return ctrl.Result{}, nil
	}

	// Clear existing policies before reconciliation.
	obj.Status.Policies = nil

	if err := r.reconcileMatchingPolicies(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to reconcile compute policies: %w", err)
	}

	if err := r.reconcileExplicitPolicies(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to reconcile explicit policies: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileMatchingPolicies(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) error {

	if err := r.reconcileMatchingComputePolicies(ctx, obj); err != nil {
		return fmt.Errorf("failed to reconcile matching compute policy: %w", err)
	}

	return nil
}

func (r *Reconciler) reconcileMatchingComputePolicies(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) error {

	var list vspherepolv1.ComputePolicyList
	if err := r.Client.List(ctx, &list); err != nil {
		return fmt.Errorf("failed to list compute policies: %w", err)
	}

	for _, p := range list.Items {
		if matchesPolicy(obj, p) {
			if err := r.addComputePolicy(ctx, obj, p); err != nil {
				return err
			}
		}
	}

	return nil
}

func matchesPolicy(
	obj *vspherepolv1.PolicyEvaluation,
	pol vspherepolv1.ComputePolicy) bool {

	if pol.Spec.Match == nil {
		return true
	}

	return evaluateMatchSpec(obj, pol.Spec.Match)
}

func evaluateMatchSpec(
	obj *vspherepolv1.PolicyEvaluation,
	matchSpec *vspherepolv1.MatchSpec) bool {

	if matchSpec == nil {
		return true
	}

	// Evaluate workload and image matches (these are always AND'd together).
	workloadMatch := evaluateWorkloadMatch(obj, matchSpec)
	imageMatch := evaluateImageMatch(obj, matchSpec)
	directMatches := workloadMatch && imageMatch

	// If there are no nested Match specs, return the direct matches.
	if len(matchSpec.Match) == 0 {
		return directMatches
	}

	// Evaluate nested MatchSpec objects using the specified boolean operation.
	var nestedMatches bool
	switch matchSpec.Op {
	case vspherepolv1.MatchesBooleanOr:
		// OR operation: any nested match must be true
		nestedMatches = false
		for _, nestedMatch := range matchSpec.Match {
			if evaluateMatchSpec(obj, &nestedMatch) {
				nestedMatches = true
				break
			}
		}
	default:
		// AND operation (default): all nested matches must be true.
		nestedMatches = true
		for _, nestedMatch := range matchSpec.Match {
			if !evaluateMatchSpec(obj, &nestedMatch) {
				nestedMatches = false
				break
			}
		}
	}

	// The direct matches (workload + image) are always AND'd with the nested
	// results.
	return directMatches && nestedMatches
}

func evaluateWorkloadMatch(
	obj *vspherepolv1.PolicyEvaluation,
	matchSpec *vspherepolv1.MatchSpec) bool {

	if matchSpec.Workload == nil {
		return true
	}

	return evaluateGuestMatch(obj, matchSpec.Workload.Guest) &&
		evaluateWorkloadLabelsMatch(obj, matchSpec.Workload.Labels)
}

func evaluateImageMatch(
	obj *vspherepolv1.PolicyEvaluation,
	matchSpec *vspherepolv1.MatchSpec) bool {

	if matchSpec.Image == nil {
		return true
	}

	return evaluateImageNameMatch(obj, matchSpec.Image.Name) &&
		evaluateImageLabelsMatch(obj, matchSpec.Image.Labels)
}

func evaluateGuestMatch(
	obj *vspherepolv1.PolicyEvaluation,
	guestSpec *vspherepolv1.MatchGuestSpec) bool {

	if guestSpec == nil {
		return true
	}

	if guestSpec.GuestID != nil {
		// Policy requires a specific guest ID match.

		if obj.Spec.Workload == nil || obj.Spec.Workload.Guest == nil {
			// The policy eval object does not include guest info, which means
			// this is not a match.
			return false
		}

		if !matchesString(obj.Spec.Workload.Guest.GuestID, guestSpec.GuestID) {
			return false
		}
	}

	if guestSpec.GuestFamily != nil {
		// Policy requires a specific guest family match.

		if obj.Spec.Workload == nil || obj.Spec.Workload.Guest == nil {
			// The policy eval object does not include guest info, which means
			// this is not a match.
			return false
		}

		if !matchesGuestFamily(
			obj.Spec.Workload.Guest.GuestFamily,
			guestSpec.GuestFamily) {

			return false
		}
	}

	return true
}

func evaluateWorkloadLabelsMatch(
	obj *vspherepolv1.PolicyEvaluation,
	labelRequirements []metav1.LabelSelectorRequirement) bool {

	if len(labelRequirements) == 0 {
		return true
	}

	if obj.Spec.Workload == nil {
		return false
	}

	selector := metav1.LabelSelector{
		MatchLabels:      nil,
		MatchExpressions: labelRequirements,
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return false
	}

	return labelSelector.Matches(labels.Set(obj.Spec.Workload.Labels))
}

func evaluateImageLabelsMatch(
	obj *vspherepolv1.PolicyEvaluation,
	labelRequirements []metav1.LabelSelectorRequirement) bool {

	if len(labelRequirements) == 0 {
		return true
	}

	if obj.Spec.Image == nil {
		return false
	}

	selector := metav1.LabelSelector{
		MatchLabels:      nil,
		MatchExpressions: labelRequirements,
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return false
	}

	return labelSelector.Matches(labels.Set(obj.Spec.Image.Labels))
}

func evaluateImageNameMatch(
	obj *vspherepolv1.PolicyEvaluation,
	nameSpec *vspherepolv1.StringMatcherSpec) bool {

	if nameSpec == nil {
		return true
	}

	if obj.Spec.Image == nil {
		return false
	}

	return matchesString(obj.Spec.Image.Name, nameSpec)
}

func matchesString(
	actual string,
	matcher *vspherepolv1.StringMatcherSpec) bool {

	if matcher == nil {
		return true
	}

	switch matcher.Op {
	case vspherepolv1.ValueSelectorOpEqual, "":
		return actual == matcher.Value
	case vspherepolv1.ValueSelectorOpNotEqual:
		return actual != matcher.Value
	default:
		return false
	}
}

func matchesGuestFamily(
	actual vspherepolv1.GuestFamilyType,
	matcher *vspherepolv1.GuestFamilyMatcherSpec) bool {

	if matcher == nil {
		return true
	}

	switch matcher.Op {
	case vspherepolv1.ValueSelectorOpEqual, "":
		return actual == matcher.Value
	case vspherepolv1.ValueSelectorOpNotEqual:
		return actual != matcher.Value
	default:
		return false
	}
}

const computePolicyKind = "ComputePolicy"

func (r *Reconciler) reconcileExplicitPolicies(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) error {

	for _, ref := range obj.Spec.Policies {
		switch ref.Kind {
		case computePolicyKind:
			if err := r.addComputePolicyRef(ctx, obj, ref); err != nil {
				return fmt.Errorf(
					"failed to add explicit compute policy %s: %w",
					ref.Name, err)
			}
		default:
			// Log and skip unknown policy kinds
			r.Logger.Info("skipping unknown policy kind",
				"kind", ref.Kind,
				"name", ref.Name)
		}
	}

	return nil
}

func (r *Reconciler) addComputePolicyRef(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation,
	ref vspherepolv1.LocalObjectRef) error {

	var (
		pol vspherepolv1.ComputePolicy
		key = client.ObjectKey{
			Namespace: obj.Namespace,
			Name:      ref.Name,
		}
	)

	if err := r.Client.Get(ctx, key, &pol); err != nil {
		return fmt.Errorf("failed to get compute policy: %w", err)
	}

	return r.addComputePolicy(ctx, obj, pol)
}

func (r *Reconciler) addComputePolicy(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation,
	pol vspherepolv1.ComputePolicy) error {

	// Check if this policy is already in the results to avoid duplicates
	for _, p := range obj.Status.Policies {
		if p.Name == pol.Name && p.Kind == computePolicyKind {
			// Policy already exists, skip adding it again.
			return nil
		}
	}

	var tags []string
	for _, tpn := range pol.Spec.Tags {
		var (
			tp  vspherepolv1.TagPolicy
			tpk = client.ObjectKey{
				Namespace: obj.Namespace,
				Name:      tpn,
			}
		)
		if err := r.Client.Get(ctx, tpk, &tp); err != nil {
			return fmt.Errorf("failed to get tag policy %q: %w", tpn, err)
		}
		tags = append(tags, tp.Spec.Tags...)
	}

	obj.Status.Policies = append(
		obj.Status.Policies,
		vspherepolv1.PolicyEvaluationResult{
			Name: pol.Name,
			Kind: computePolicyKind,
			Tags: tags,
		},
	)

	return nil
}
