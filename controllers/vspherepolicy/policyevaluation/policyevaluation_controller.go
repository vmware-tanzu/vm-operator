// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policyevaluation

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
			LogConstructor: pkglog.ControllerLogConstructor(
				controllerNameShort,
				controlledType,
				mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client ctrlclient.Client,
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
	ctrlclient.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=policyevaluations,verbs=create;get;list;watch;update;patch
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=policyevaluations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=computepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=computepolicies/status,verbs=get
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tagpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tagpolicies/status,verbs=get

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj vspherepolv1.PolicyEvaluation
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
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

const (
	failMandatoryPolicies = "failed to reconcile mandatory policies"
	failExplicitPolicies  = "failed to reconcile explicit policies"
)

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) (ctrl.Result, error) {

	if controllerutil.AddFinalizer(obj, Finalizer) {
		// Ensure the finalizer is present before reconciling further.
		return ctrl.Result{}, nil
	}

	// Clear existing policies before reconciliation.
	obj.Status.Policies = nil

	if err := r.reconcileMandatoryPolicies(ctx, obj); err != nil {
		conditions.MarkError(
			obj,
			vspherepolv1.ReadyConditionType,
			failMandatoryPolicies,
			err)
		return ctrl.Result{}, fmt.Errorf(
			failMandatoryPolicies+": %w", err)
	}

	if err := r.reconcileExplicitPolicies(ctx, obj); err != nil {
		conditions.MarkError(
			obj,
			vspherepolv1.ReadyConditionType,
			failExplicitPolicies,
			err)
		return ctrl.Result{}, fmt.Errorf(
			failExplicitPolicies+": %w", err)
	}

	// Update the observed generation.
	obj.Status.ObservedGeneration = obj.Generation

	// Update the ready condition.
	conditions.MarkTrue(obj, vspherepolv1.ReadyConditionType)

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileMandatoryPolicies(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) error {

	if err := r.reconcileMandatoryComputePolicies(ctx, obj); err != nil {
		return fmt.Errorf(
			"failed to reconcile mandatory compute policies: %w", err)
	}

	return nil
}

func (r *Reconciler) reconcileMandatoryComputePolicies(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation) error {

	var list vspherepolv1.ComputePolicyList
	if err := r.Client.List(
		ctx,
		&list,
		ctrlclient.InNamespace(obj.Namespace)); err != nil {

		return fmt.Errorf("failed to list compute policies: %w", err)
	}

	for _, p := range list.Items {
		// Only mandatory policies should be automatically applied.
		if p.Spec.EnforcementMode != vspherepolv1.PolicyEnforcementModeMandatory {
			continue
		}

		matches, err := matchesPolicy(obj, p)
		if err != nil {
			return err
		}

		if matches {
			if err := r.addComputePolicy(ctx, obj, p); err != nil {
				return err
			}
		}
	}

	return nil
}

func matchesPolicy(
	obj *vspherepolv1.PolicyEvaluation,
	pol vspherepolv1.ComputePolicy) (bool, error) {

	if pol.Spec.Match == nil {
		return true, nil
	}

	return evaluateMatchSpec(obj, pol.Spec.Match)
}

func evaluateMatchSpec(
	obj *vspherepolv1.PolicyEvaluation,
	matchSpec *vspherepolv1.MatchSpec) (bool, error) {

	if matchSpec == nil {
		return true, nil
	}

	workloadMatch, err := evaluateWorkloadMatch(obj, matchSpec)
	if err != nil {
		return false, err
	}

	imageMatch, err := evaluateImageMatch(obj, matchSpec)
	if err != nil {
		return false, err
	}

	directMatches := workloadMatch && imageMatch

	// If there are no nested Match specs, return the direct matches.
	if len(matchSpec.Match) == 0 {
		return directMatches, nil
	}

	// Evaluate nested MatchSpec objects using the specified boolean operation.
	var nestedMatches bool
	switch matchSpec.Op {
	case vspherepolv1.BooleanOpOr:
		// OR operation: any nested match must be true
		nestedMatches = false
		for _, nestedMatch := range matchSpec.Match {
			if ok, err := evaluateMatchSpec(obj, &nestedMatch); err != nil {
				return false, err
			} else if ok {
				nestedMatches = true
				break
			}
		}
	default:
		// AND operation (default): all nested matches must be true.
		nestedMatches = true
		for _, nestedMatch := range matchSpec.Match {
			if ok, err := evaluateMatchSpec(obj, &nestedMatch); err != nil {
				return false, err
			} else if !ok {
				nestedMatches = false
				break
			}
		}
	}

	// The direct matches (workload + image) are always AND'd with the nested
	// results.
	return directMatches && nestedMatches, nil
}

func evaluateWorkloadMatch(
	obj *vspherepolv1.PolicyEvaluation,
	matchSpec *vspherepolv1.MatchSpec) (bool, error) {

	if matchSpec.Workload == nil {
		return true, nil
	}

	if ok, err := evaluateGuestMatch(
		obj,
		matchSpec.Workload.Guest); err != nil || !ok {

		return ok, err
	}

	if ok, err := evaluateWorkloadLabelsMatch(
		obj,
		matchSpec.Workload.Labels); err != nil || !ok {

		return ok, err
	}

	return true, nil
}

func evaluateImageMatch(
	obj *vspherepolv1.PolicyEvaluation,
	matchSpec *vspherepolv1.MatchSpec) (bool, error) {

	if matchSpec.Image == nil {
		return true, nil
	}

	if ok, err := evaluateImageNameMatch(
		obj,
		matchSpec.Image.Name); err != nil || !ok {

		return ok, err
	}

	if ok, err := evaluateImageLabelsMatch(
		obj,
		matchSpec.Image.Labels); err != nil || !ok {

		return ok, err
	}

	return true, nil
}

func evaluateGuestMatch(
	obj *vspherepolv1.PolicyEvaluation,
	guestSpec *vspherepolv1.MatchGuestSpec) (bool, error) {

	if guestSpec == nil {
		return true, nil
	}

	if guestSpec.GuestID != nil {
		// Policy requires a specific guest ID match.

		if obj.Spec.Workload == nil || obj.Spec.Workload.Guest == nil {
			// The policy eval object does not include guest info, which means
			// this is not a match.
			return false, nil
		}

		if ok, err := matchesString(
			obj.Spec.Workload.Guest.GuestID,
			guestSpec.GuestID); err != nil || !ok {

			return ok, err
		}
	}

	if guestSpec.GuestFamily != nil {
		// Policy requires a specific guest family match.

		if obj.Spec.Workload == nil || obj.Spec.Workload.Guest == nil {
			// The policy eval object does not include guest info, which means
			// this is not a match.
			return false, nil
		}

		if ok, err := matchesGuestFamily(
			obj.Spec.Workload.Guest.GuestFamily,
			guestSpec.GuestFamily); err != nil || !ok {

			return ok, err
		}
	}

	return true, nil
}

func evaluateWorkloadLabelsMatch(
	obj *vspherepolv1.PolicyEvaluation,
	labelRequirements []metav1.LabelSelectorRequirement) (bool, error) {

	if len(labelRequirements) == 0 {
		return true, nil
	}

	if obj.Spec.Workload == nil {
		return false, nil
	}

	selector := metav1.LabelSelector{
		MatchLabels:      nil,
		MatchExpressions: labelRequirements,
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return false, fmt.Errorf("failed to match workload labels: %w", err)
	}

	return labelSelector.Matches(labels.Set(obj.Spec.Workload.Labels)), nil
}

func evaluateImageLabelsMatch(
	obj *vspherepolv1.PolicyEvaluation,
	labelRequirements []metav1.LabelSelectorRequirement) (bool, error) {

	if len(labelRequirements) == 0 {
		return true, nil
	}

	if obj.Spec.Image == nil {
		return false, nil
	}

	selector := metav1.LabelSelector{
		MatchLabels:      nil,
		MatchExpressions: labelRequirements,
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return false, fmt.Errorf("failed to match image labels: %w", err)
	}

	return labelSelector.Matches(labels.Set(obj.Spec.Image.Labels)), nil
}

func evaluateImageNameMatch(
	obj *vspherepolv1.PolicyEvaluation,
	nameSpec *vspherepolv1.StringMatcherSpec) (bool, error) {

	if nameSpec == nil {
		return true, nil
	}

	if obj.Spec.Image == nil {
		return false, nil
	}

	return matchesString(obj.Spec.Image.Name, nameSpec)
}

func matchesString(
	actual string,
	matcher *vspherepolv1.StringMatcherSpec) (bool, error) {

	if matcher == nil {
		return true, nil
	}

	switch matcher.Op {
	case vspherepolv1.ValueSelectorOpEqual, "":
		return actual == matcher.Value, nil
	case vspherepolv1.ValueSelectorOpNotEqual:
		return actual != matcher.Value, nil
	case vspherepolv1.ValueSelectorOpHasPrefix:
		return strings.HasPrefix(actual, matcher.Value), nil
	case vspherepolv1.ValueSelectorOpNotHasPrefix:
		return !strings.HasPrefix(actual, matcher.Value), nil
	case vspherepolv1.ValueSelectorOpHasSuffix:
		return strings.HasSuffix(actual, matcher.Value), nil
	case vspherepolv1.ValueSelectorOpNotHasSuffix:
		return !strings.HasSuffix(actual, matcher.Value), nil
	case vspherepolv1.ValueSelectorOpContains:
		return strings.Contains(actual, matcher.Value), nil
	case vspherepolv1.ValueSelectorOpNotContains:
		return !strings.Contains(actual, matcher.Value), nil
	case vspherepolv1.ValueSelectorOpMatch:
		return regexp.MatchString(matcher.Value, actual)
	case vspherepolv1.ValueSelectorOpNotMatch:
		ok, err := regexp.MatchString(matcher.Value, actual)
		return !ok, err
	default:
		return false, nil
	}
}

func matchesGuestFamily(
	actual vspherepolv1.GuestFamilyType,
	matcher *vspherepolv1.GuestFamilyMatcherSpec) (bool, error) {

	var sm *vspherepolv1.StringMatcherSpec
	if matcher != nil {
		sm = &vspherepolv1.StringMatcherSpec{
			Op:    matcher.Op,
			Value: string(matcher.Value),
		}
	}

	return matchesString(string(actual), sm)
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
		key = ctrlclient.ObjectKey{
			Namespace: obj.Namespace,
			Name:      ref.Name,
		}
	)

	if err := r.Client.Get(ctx, key, &pol); err != nil {
		return fmt.Errorf("failed to get compute policy: %w", err)
	}

	matches, err := matchesPolicy(obj, pol)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("compute policy %q does not match", pol.Name)
	}

	return r.addComputePolicy(ctx, obj, pol)
}

func (r *Reconciler) addComputePolicy(
	ctx context.Context,
	obj *vspherepolv1.PolicyEvaluation,
	pol vspherepolv1.ComputePolicy) error {

	var tags []string
	for _, tpn := range pol.Spec.Tags {
		var (
			tp  vspherepolv1.TagPolicy
			tpk = ctrlclient.ObjectKey{
				Namespace: obj.Namespace,
				Name:      tpn,
			}
		)
		if err := r.Client.Get(ctx, tpk, &tp); err != nil {
			return fmt.Errorf("failed to get tag policy %q: %w", tpn, err)
		}
		tags = append(tags, tp.Spec.Tags...)
	}

	// Check if this policy is already in the results to avoid duplicates.
	if slices.ContainsFunc(
		obj.Status.Policies,
		func(p vspherepolv1.PolicyEvaluationResult) bool {
			return p.Name == pol.Name && p.Kind == computePolicyKind
		}) {

		// Policy already exists, skip adding it again.
		return nil
	}

	obj.Status.Policies = append(
		obj.Status.Policies,
		vspherepolv1.PolicyEvaluationResult{
			APIVersion: vspherepolv1.GroupVersion.String(),
			Kind:       computePolicyKind,
			Name:       pol.Name,
			Generation: pol.Generation,
			Tags:       tags,
		},
	)

	return nil
}
