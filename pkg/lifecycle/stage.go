// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package lifecycle implements the VM Operator side of the blocking lifecycle
// hooks contract with the Lifecycle Operator. It is a leaf package so that both
// the VirtualMachine controller and the vSphere provider can call it without
// importing each other.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
)

const (
	// StageCreate is the stage reached before a VM is created in vSphere.
	StageCreate = "Create"

	// StagePowerStateChange is the stage reached before a power operation is
	// applied.
	StagePowerStateChange = "PowerStateChange"

	// StageDelete is the stage reached before a VM is deleted from vSphere.
	StageDelete = "Delete"

	// StageResourceDelete is the stage reached before the VM's Kubernetes
	// finalizer is removed.
	StageResourceDelete = "ResourceDelete"

	// TargetGroup is the API group of VM Operator's lifecycle target.
	TargetGroup = "vmoperator.vmware.com"

	// TargetKind is the kind of VM Operator's lifecycle target.
	TargetKind = "VirtualMachine"

	// TargetGroupLabel identifies a lifecycle object's target API group.
	TargetGroupLabel = "lifecycle.vcfa.vmware.com/target-group"

	// TargetKindLabel identifies a lifecycle object's target kind.
	TargetKindLabel = "lifecycle.vcfa.vmware.com/target-kind"

	// HooksReadyConditionType is the condition type the Lifecycle Operator sets
	// on a stage's status entry once every hook for that stage is done.
	HooksReadyConditionType = "HooksReady"

	// HooksReadyReasonHookFailed is the HooksReady reason the Lifecycle
	// Operator sets when at least one hook failed.
	HooksReadyReasonHookFailed = "HookFailed"

	// lifecycleStateFinalizer is VM Operator's finalizer on a LifecycleState. It
	// is removed only after the ResourceDelete stage resolves.
	lifecycleStateFinalizer = "lifecycle.vcfa.vmware.com/vm-operator-state"

	conditionType = vmopv1.VirtualMachineConditionLifecycleHookReady
)

// ErrHooksBlocked is returned by a call site that must stop because a stage is
// blocked on hooks. It is not an error condition and does not requeue.
var ErrHooksBlocked = pkgerr.NoRequeueNoErr("waiting for lifecycle hooks")

// IsHooksBlocked returns true if err is, or wraps, ErrHooksBlocked.
func IsHooksBlocked(err error) bool {
	return errors.Is(err, ErrHooksBlocked)
}

// Result is the outcome of evaluating a stage.
type Result struct {
	// Proceed is true if the guarded step may run.
	Proceed bool
}

// GetOrCreateLifecycleState returns the VM's LifecycleState. If the VM has none
// and LifecycleSubscribedStages lists at least one stage for VirtualMachine, it
// creates one with every listed stage at (false, false). It returns nil if no
// stage is hooked or the capability is off.
//
// A found LifecycleState that lacks VM Operator's finalizer is adopted by
// adding it.
func GetOrCreateLifecycleState(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (*lifecyclev1.LifecycleState, error) {

	if !pkgcfg.FromContext(ctx).Features.LifecycleHooks {
		return nil, nil
	}

	state, err := getState(ctx, k8sClient, vm)
	if err != nil {
		return nil, err
	}
	if state != nil {
		if state.DeletionTimestamp.IsZero() &&
			!controllerutil.ContainsFinalizer(state, lifecycleStateFinalizer) {

			base := state.DeepCopy()
			controllerutil.AddFinalizer(state, lifecycleStateFinalizer)
			if err := patchState(ctx, k8sClient, state, base); err != nil {
				return nil, fmt.Errorf(
					"failed to add finalizer to LifecycleState %s/%s: %w",
					state.Namespace, state.Name, err)
			}
		}
		return state, nil
	}

	var subscribed lifecyclev1.LifecycleSubscribedStages
	subscribedKey := ctrlclient.ObjectKey{
		Namespace: vm.Namespace,
		Name:      lifecyclev1.SubscribedStagesName(TargetGroup, TargetKind),
	}
	if err := k8sClient.Get(ctx, subscribedKey, &subscribed); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"failed to get LifecycleSubscribedStages %s: %w",
			subscribedKey, err)
	}
	if len(subscribed.Spec.Stages) == 0 {
		return nil, nil
	}

	stages := make([]lifecyclev1.StageSpec, 0, len(subscribed.Spec.Stages))
	for _, name := range subscribed.Spec.Stages {
		stages = append(stages, lifecyclev1.StageSpec{Name: name})
	}

	state = &lifecyclev1.LifecycleState{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vm.Namespace,
			Name:      lifecyclev1.LifecycleStateName(TargetKind, vm.UID),
			Labels: map[string]string{
				TargetGroupLabel: TargetGroup,
				TargetKindLabel:  TargetKind,
			},
			Finalizers: []string{lifecycleStateFinalizer},
		},
		Spec: lifecyclev1.LifecycleStateSpec{Stages: stages},
	}
	if err := controllerutil.SetControllerReference(
		vm,
		state,
		k8sClient.Scheme(),
		controllerutil.WithBlockOwnerDeletion(false)); err != nil {

		return nil, fmt.Errorf(
			"failed to set owner reference on LifecycleState %s/%s: %w",
			state.Namespace, state.Name, err)
	}

	if err := k8sClient.Create(ctx, state); err != nil {
		switch {
		case apierrors.IsAlreadyExists(err):
			// Another creator won or the cache is behind. Never treat this as
			// "no hook".
			return nil, pkgerr.RequeueError{
				After:   time.Second,
				Message: "LifecycleState exists but is not yet cached",
			}
		case apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause):
			// Accepted race: the namespace is already being deleted.
			return nil, nil
		}
		return nil, fmt.Errorf(
			"failed to create LifecycleState %s/%s: %w",
			state.Namespace, state.Name, err)
	}

	return state, nil
}

// ReconcileStage evaluates one stage checkpoint against the VM's LifecycleState
// and reports whether the guarded step may proceed. A nil state means no hook.
//
// Only vm.Status.Conditions is mutated; the caller persists it. A write to
// state is an optimistic-lock patch that leaves state holding the new
// resourceVersion, and a conflict is returned as a plain error for the caller's
// requeue to retry.
func ReconcileStage(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	state *lifecyclev1.LifecycleState,
	stageName string) (Result, error) {

	if !pkgcfg.FromContext(ctx).Features.LifecycleHooks || state == nil {
		return Result{Proceed: true}, nil
	}

	idx := stageIndex(state, stageName)
	if idx < 0 {
		return Result{Proceed: true}, nil
	}

	var (
		spec   = state.Spec.Stages[idx]
		status = findStageStatus(state, stageName)
		ready  *metav1.Condition
	)
	if status != nil {
		ready = apimeta.FindStatusCondition(
			status.Conditions, HooksReadyConditionType)
	}

	if !spec.WorkflowPaused {
		if status == nil {
			// Ready to hold.
			base := state.DeepCopy()
			state.Spec.Stages[idx].WorkflowPaused = true
			state.Spec.Stages[idx].WorkflowResumed = false
			if err := patchState(ctx, k8sClient, state, base); err != nil {
				return Result{}, fmt.Errorf(
					"failed to pause stage %s: %w", stageName, err)
			}
			markWaiting(vm, stageName, nil)
			return Result{}, nil
		}

		// The previous hold is still being cleared; pausing now would reuse its
		// results.
		if isHookFailedDone(ready) {
			markRetrying(vm, stageName, ready)
		} else {
			markWaiting(vm, stageName, nil)
		}
		return Result{}, nil
	}

	if ready == nil || ready.Status != metav1.ConditionTrue {
		markWaiting(vm, stageName, ready)
		return Result{}, nil
	}

	failed := isHookFailedDone(ready)
	proceed := !failed

	base := state.DeepCopy()
	state.Spec.Stages[idx].WorkflowPaused = false
	state.Spec.Stages[idx].WorkflowResumed = true
	if proceed && stageName == StageResourceDelete {
		// Release in the same write: a separate patch would conflict with the
		// Lifecycle Operator removing the status entry in response to this one.
		// Also release any other stage still paused, such as Delete when the
		// vSphere VM disappeared out of band.
		releasePaused(state)
		controllerutil.RemoveFinalizer(state, lifecycleStateFinalizer)
	}
	if err := patchState(ctx, k8sClient, state, base); err != nil {
		return Result{}, fmt.Errorf(
			"failed to release stage %s: %w", stageName, err)
	}

	if failed {
		markRetrying(vm, stageName, ready)
		return Result{}, nil
	}
	setCondition(vm, metav1.ConditionTrue, stageName, ready.Message)
	return Result{Proceed: true}, nil
}

// AbandonStage releases a paused stage whose action will no longer run,
// whatever HooksReady says, and sets the condition to True. It is a no-op if
// state is nil, the stage is not paused, or the capability is off.
func AbandonStage(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	state *lifecyclev1.LifecycleState,
	stageName string) error {

	if !pkgcfg.FromContext(ctx).Features.LifecycleHooks || state == nil {
		return nil
	}

	idx := stageIndex(state, stageName)
	if idx < 0 || !state.Spec.Stages[idx].WorkflowPaused {
		return nil
	}

	base := state.DeepCopy()
	state.Spec.Stages[idx].WorkflowPaused = false
	state.Spec.Stages[idx].WorkflowResumed = true
	if err := patchState(ctx, k8sClient, state, base); err != nil {
		return fmt.Errorf("failed to abandon stage %s: %w", stageName, err)
	}

	setCondition(vm, metav1.ConditionTrue, stageName, "")
	return nil
}

// MarkUnknown records that the stage's hooks could not be evaluated. Nothing
// is written for a conflict, which the caller's requeue retries.
func MarkUnknown(
	vm *vmopv1.VirtualMachine,
	stageName string,
	err error) {

	if err == nil || apierrors.IsConflict(err) {
		return
	}
	setCondition(
		vm,
		metav1.ConditionUnknown,
		stageName,
		fmt.Sprintf("Could not evaluate %s hooks: %v", stageName, err))
}

// ReleaseLifecycleState removes VM Operator's finalizer from the VM's
// LifecycleState, and releases any stage still paused in the same write. It
// never deletes the object.
//
// With the capability off, the object is read through apiReader so that no
// informer is started; this keeps an object created while the capability was on
// from blocking VM and namespace deletion.
func ReleaseLifecycleState(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	apiReader ctrlclient.Reader,
	vm *vmopv1.VirtualMachine,
	state *lifecyclev1.LifecycleState) error {

	if !pkgcfg.FromContext(ctx).Features.LifecycleHooks {
		var err error
		state, err = getState(ctx, apiReader, vm)
		if err != nil {
			if apimeta.IsNoMatchError(err) {
				return nil
			}
			return err
		}
	}

	if state == nil ||
		!controllerutil.ContainsFinalizer(state, lifecycleStateFinalizer) {

		return nil
	}

	base := state.DeepCopy()
	releasePaused(state)
	controllerutil.RemoveFinalizer(state, lifecycleStateFinalizer)
	if err := patchState(ctx, k8sClient, state, base); err != nil {
		return fmt.Errorf(
			"failed to release LifecycleState %s/%s: %w",
			state.Namespace, state.Name, err)
	}
	return nil
}

// getState returns the VM's LifecycleState, or nil if it does not exist. A
// found object that is not bound to this VM, or that is terminating without VM
// Operator's finalizer, results in an error so that its stages are never read
// and the VM is retried until the garbage collector removes it.
func getState(
	ctx context.Context,
	reader ctrlclient.Reader,
	vm *vmopv1.VirtualMachine) (*lifecyclev1.LifecycleState, error) {

	var (
		state lifecyclev1.LifecycleState
		key   = ctrlclient.ObjectKey{
			Namespace: vm.Namespace,
			Name:      lifecyclev1.LifecycleStateName(TargetKind, vm.UID),
		}
	)
	if err := reader.Get(ctx, key, &state); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get LifecycleState %s: %w", key, err)
	}

	if !isBoundTo(&state, vm) {
		return nil, fmt.Errorf(
			"LifecycleState %s is not bound to VirtualMachine %s/%s (uid %s)",
			key, vm.Namespace, vm.Name, vm.UID)
	}
	if !state.DeletionTimestamp.IsZero() &&
		!controllerutil.ContainsFinalizer(&state, lifecycleStateFinalizer) {

		return nil, fmt.Errorf(
			"LifecycleState %s is terminating without finalizer %s",
			key, lifecycleStateFinalizer)
	}
	return &state, nil
}

// isBoundTo returns true if the state carries the VM's target labels and has
// exactly one controller owner reference, which matches the VM's group, kind,
// name and UID.
func isBoundTo(
	state *lifecyclev1.LifecycleState,
	vm *vmopv1.VirtualMachine) bool {

	if state.Labels[TargetGroupLabel] != TargetGroup ||
		state.Labels[TargetKindLabel] != TargetKind {

		return false
	}

	var owner *metav1.OwnerReference
	for i := range state.OwnerReferences {
		r := &state.OwnerReferences[i]
		if r.Controller == nil || !*r.Controller {
			continue
		}
		if owner != nil {
			return false
		}
		owner = r
	}
	if owner == nil {
		return false
	}

	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return false
	}
	return gv.Group == TargetGroup &&
		owner.Kind == TargetKind &&
		owner.Name == vm.Name &&
		owner.UID == vm.UID
}

// releasePaused sets every paused stage to released, (false, true).
func releasePaused(state *lifecyclev1.LifecycleState) {
	for i := range state.Spec.Stages {
		if state.Spec.Stages[i].WorkflowPaused {
			state.Spec.Stages[i].WorkflowPaused = false
			state.Spec.Stages[i].WorkflowResumed = true
		}
	}
}

func patchState(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	state, base *lifecyclev1.LifecycleState) error {

	return k8sClient.Patch(
		ctx,
		state,
		ctrlclient.MergeFromWithOptions(
			base, ctrlclient.MergeFromWithOptimisticLock{}))
}

func stageIndex(state *lifecyclev1.LifecycleState, stageName string) int {
	return slices.IndexFunc(
		state.Spec.Stages,
		func(s lifecyclev1.StageSpec) bool { return s.Name == stageName })
}

func findStageStatus(
	state *lifecyclev1.LifecycleState,
	stageName string) *lifecyclev1.StageStatus {

	for i := range state.Status.Stages {
		if state.Status.Stages[i].Name == stageName {
			return &state.Status.Stages[i]
		}
	}
	return nil
}

// isHookFailedDone returns true if HooksReady is True with reason HookFailed:
// every hook finished and at least one failed.
func isHookFailedDone(ready *metav1.Condition) bool {
	return ready != nil &&
		ready.Status == metav1.ConditionTrue &&
		ready.Reason == HooksReadyReasonHookFailed
}

func markWaiting(
	vm *vmopv1.VirtualMachine,
	stageName string,
	ready *metav1.Condition) {

	var msg string
	switch {
	case ready == nil:
		msg = fmt.Sprintf("Waiting for %s hooks", stageName)
	case ready.Reason == HooksReadyReasonHookFailed:
		msg = fmt.Sprintf(
			"%s hook failed, waiting for remaining hooks%s",
			stageName, suffix(ready.Message))
	default:
		msg = fmt.Sprintf("Waiting for %s hooks%s", stageName, suffix(ready.Message))
	}
	setCondition(vm, metav1.ConditionFalse, stageName, msg)
}

func markRetrying(
	vm *vmopv1.VirtualMachine,
	stageName string,
	ready *metav1.Condition) {

	setCondition(
		vm,
		metav1.ConditionFalse,
		stageName,
		fmt.Sprintf(
			"%s hook failed, retrying%s", stageName, suffix(ready.Message)))
}

func suffix(msg string) string {
	if msg == "" {
		return ""
	}
	return ": " + msg
}

func setCondition(
	vm *vmopv1.VirtualMachine,
	status metav1.ConditionStatus,
	stageName, message string) {

	conditions.Set(vm, &metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  stageName + "Stage",
		Message: message,
	})
}

// StatusChangedPredicate filters LifecycleState events down to the ones that
// carry information VM Operator lacks: creates by the Lifecycle Operator (those
// without VM Operator's finalizer, for adoption), deletes, and changes to
// status. It drops VM Operator's own creates and spec writes.
type StatusChangedPredicate struct {
	predicate.TypedFuncs[ctrlclient.Object]
}

// Create implements predicate.Predicate.
func (StatusChangedPredicate) Create(
	e event.TypedCreateEvent[ctrlclient.Object]) bool {

	state, ok := e.Object.(*lifecyclev1.LifecycleState)
	if !ok {
		return false
	}
	return !controllerutil.ContainsFinalizer(state, lifecycleStateFinalizer)
}

// Generic implements predicate.Predicate.
func (StatusChangedPredicate) Generic(
	event.TypedGenericEvent[ctrlclient.Object]) bool {

	return false
}

// Delete implements predicate.Predicate.
func (StatusChangedPredicate) Delete(
	event.TypedDeleteEvent[ctrlclient.Object]) bool {

	return true
}

// Update implements predicate.Predicate.
func (StatusChangedPredicate) Update(
	e event.TypedUpdateEvent[ctrlclient.Object]) bool {

	oldState, ok := e.ObjectOld.(*lifecyclev1.LifecycleState)
	if !ok {
		return false
	}
	newState, ok := e.ObjectNew.(*lifecyclev1.LifecycleState)
	if !ok {
		return false
	}
	return !apiequality.Semantic.DeepEqual(oldState.Status, newState.Status)
}
