// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	finalizerName = "vmoperator.vmware.com/virtualmachinegroup"
	vmKind        = "VirtualMachine"
	vmgKind       = "VirtualMachineGroup"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineGroup{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Watches(&vmopv1.VirtualMachineGroup{},
			handler.EnqueueRequestsFromMapFunc(vmGroupToParentGroupMapperFn())).
		// TODO(sai): Add a mapper function to enqueue requests for VM.Spec.GroupName changes.
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
		}).
		Complete(r)
}

// vmGroupToParentGroupMapperFn returns a mapper function that enqueues
// reconcile requests for VirtualMachineGroup when another VirtualMachineGroup's
// Spec.GroupName pointing to it changes.
func vmGroupToParentGroupMapperFn() handler.MapFunc {
	return func(_ context.Context, o client.Object) []reconcile.Request {
		vmGroup, ok := o.(*vmopv1.VirtualMachineGroup)
		if !ok {
			panic(fmt.Sprintf("Expected a VirtualMachineGroup, but got a %T", o))
		}

		var requests []reconcile.Request

		if vmGroup.Spec.GroupName != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: vmGroup.Namespace,
					Name:      vmGroup.Spec.GroupName,
				},
			})
		}

		return requests
	}
}

// NewReconciler returns a new reconciler for VirtualMachineGroup objects.
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

// Reconciler reconciles a VirtualMachineGroup object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;update;patch

// Reconcile reconciles a VirtualMachineGroup object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = record.WithContext(ctx, r.Recorder)

	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := r.Get(ctx, req.NamespacedName, vmGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger := ctrl.Log.WithName("VirtualMachineGroup").WithValues("name", req.NamespacedName)

	vmGroupCtx := &pkgctx.VirtualMachineGroupContext{
		Context: ctx,
		Logger:  logger,
		VMGroup: vmGroup,
	}

	patchHelper, err := patch.NewHelper(vmGroup, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for %s: %w",
			req.NamespacedName, err,
		)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, vmGroup); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmGroupCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmGroup.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmGroupCtx)
	}

	return r.ReconcileNormal(vmGroupCtx)
}

func (r *Reconciler) ReconcileDelete(
	ctx *pkgctx.VirtualMachineGroupContext) (ctrl.Result, error) {

	ctx.Logger.Info("Reconciling VirtualMachineGroup Deletion")

	if controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.RemoveFinalizer(ctx.VMGroup, finalizerName)
	}

	ctx.Logger.Info("Finished Reconciling VirtualMachineGroup Deletion")
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineGroupContext) (
	_ ctrl.Result, reterr error) {

	if !controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.AddFinalizer(ctx.VMGroup, finalizerName)
		return ctrl.Result{}, nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineGroup")

	defer func(beforeVMGroupStatus *vmopv1.VirtualMachineGroupStatus) {
		if !apiequality.Semantic.DeepEqual(beforeVMGroupStatus, &ctx.VMGroup.Status) {
			ctx.Logger.Info("Finished Reconciling VirtualMachineGroup with updates to the CR",
				"createdTime", ctx.VMGroup.CreationTimestamp,
				"currentTime", time.Now().Format(time.RFC3339))
		} else {
			ctx.Logger.Info("Finished Reconciling VirtualMachineGroup")
		}
	}(ctx.VMGroup.Status.DeepCopy())

	defer func() {
		setReadyCondition(ctx, reterr)
	}()

	if reterr = r.reconcileMembers(ctx); reterr != nil {
		ctx.Logger.Error(reterr, "Failed to reconcile group members")
		return ctrl.Result{}, reterr
	}

	if reterr = r.reconcilePlacement(ctx); reterr != nil {
		ctx.Logger.Error(reterr, "Failed to reconcile group placement")
		return ctrl.Result{}, reterr
	}

	return ctrl.Result{}, nil
}

// reconcileMembers reconciles all current members of the group and updates
// the group's Status.Members accordingly.
func (r *Reconciler) reconcileMembers(
	ctx *pkgctx.VirtualMachineGroupContext) error {

	existingStatuses := make(
		map[string]*vmopv1.VirtualMachineGroupMemberStatus,
		len(ctx.VMGroup.Status.Members),
	)

	for i := range ctx.VMGroup.Status.Members {
		ms := &ctx.VMGroup.Status.Members[i]
		key := ms.Kind + "/" + ms.Name
		existingStatuses[key] = ms
	}

	updatePowerState, lastUpdateAnnoTime, err := shouldUpdatePowerState(ctx)
	if err != nil {
		return err
	}

	// Get the group's apply power state change time that may be set from its
	// parent group for power-on delay.
	var applyPowerOnTime time.Time
	if updatePowerState && ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn {
		if v := ctx.VMGroup.Annotations[constants.ApplyPowerStateTimeAnnotation]; v != "" {
			applyPowerOnTime, err = time.Parse(time.RFC3339Nano, v)
			if err != nil {
				ctx.Logger.Error(err, "Failed to parse time from annotation",
					"annotationKey", constants.ApplyPowerStateTimeAnnotation,
					"annotationValue", v)
				return err
			}
		}
	}

	// If applyPowerOnTime is zero, this group's power state was being changed
	// directly (not inherited from a parent). Use the last updated annotation
	// timestamp as the base time for calculating members' power-on delays.
	if applyPowerOnTime.IsZero() {
		applyPowerOnTime = lastUpdateAnnoTime
	}

	var (
		memberStatuses = make([]vmopv1.VirtualMachineGroupMemberStatus, 0, len(ctx.VMGroup.Spec.Members))
		memberErrs     = make([]error, 0, len(ctx.VMGroup.Spec.Members))
	)

	for _, member := range ctx.VMGroup.Spec.Members {
		key := member.Kind + "/" + member.Name

		// Check if we have an existing status to update, or create a new one.
		var ms *vmopv1.VirtualMachineGroupMemberStatus
		if s, ok := existingStatuses[key]; ok {
			ms = s.DeepCopy()
		} else {
			ms = &vmopv1.VirtualMachineGroupMemberStatus{
				Name: member.Name,
				Kind: member.Kind,
			}
		}

		switch member.Kind {
		case vmKind:
			if err := r.reconcileVMMember(ctx, member, ms, updatePowerState, &applyPowerOnTime); err != nil {
				memberErrs = append(memberErrs, err)
			}
		case vmgKind:
			if err := r.reconcileVMGMember(ctx, member, ms, updatePowerState, &applyPowerOnTime); err != nil {
				memberErrs = append(memberErrs, err)
			}
		}

		memberStatuses = append(memberStatuses, *ms)
	}

	if len(memberErrs) == 0 {
		// Only update the last updated power state time in status if no errors.
		// This ensures the requeue continues to apply the group power state.
		ctx.VMGroup.Status.LastUpdatedPowerStateTime = &metav1.Time{
			Time: time.Now().UTC(),
		}
	}

	ctx.VMGroup.Status.Members = memberStatuses
	return apierrorsutil.NewAggregate(memberErrs)
}

// reconcileVMMember handles reconciliation of a VirtualMachine member.
func (r *Reconciler) reconcileVMMember(
	ctx *pkgctx.VirtualMachineGroupContext,
	member vmopv1.GroupMember,
	ms *vmopv1.VirtualMachineGroupMemberStatus,
	updatePowerState bool,
	applyPowerOnTime *time.Time,
) error {
	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: ctx.VMGroup.Namespace, Name: member.Name}, vm); err != nil {
		ctx.Logger.Error(err, "Failed to get group member",
			"kind", member.Kind,
			"name", member.Name,
		)
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"GetError",
			err,
		)
		return err
	}

	if vm.Spec.GroupName != ctx.VMGroup.Name {
		err := fmt.Errorf("member has a different group name: %s",
			vm.Spec.GroupName)
		ctx.Logger.Error(err, "Invalid group name for member",
			"kind", member.Kind,
			"name", member.Name,
		)
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"InvalidGroupName",
			err,
		)
		return err
	}

	patch := client.MergeFrom(vm.DeepCopy())

	// Set owner reference to the parent group.
	if err := controllerutil.SetControllerReference(ctx.VMGroup, vm, r.Scheme()); err != nil {
		ctx.Logger.Error(err, "Failed to set owner reference to group member",
			"kind", member.Kind,
			"name", member.Name,
		)
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"SetOwnerRefError",
			err,
		)
		return err
	}

	if vm.Status.PowerState == ctx.VMGroup.Spec.PowerState {
		// VM is already synced with the group's power state.
		conditions.MarkTrue(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
		)
	} else if updatePowerState {
		// Mark the VM member status condition as false if the VM's power state
		// doesn't currently match the group's power state. After it's updated,
		// it will trigger a new reconciliation of the parent group.
		conditions.MarkFalse(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			"Pending",
			"",
		)
		if d := member.PowerOnDelay; d != nil {
			*applyPowerOnTime = applyPowerOnTime.Add(d.Duration)
		}
		updateMemberPowerState(*ctx.VMGroup, vm, *applyPowerOnTime)
	}

	if err := r.Patch(ctx, vm, patch); err != nil {
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"OwnerRefPatchError",
			err,
		)
		if updatePowerState {
			conditions.MarkError(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"PowerStatePatchError",
				err,
			)
		}
		return fmt.Errorf("failed to patch group member, kind=%s, name=%s: %w",
			member.Kind, member.Name, err)
	}

	conditions.MarkTrue(
		ms,
		vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
	)

	return nil
}

// reconcileVMGMember handles reconciliation of a VirtualMachineGroup member.
func (r *Reconciler) reconcileVMGMember(
	ctx *pkgctx.VirtualMachineGroupContext,
	member vmopv1.GroupMember,
	ms *vmopv1.VirtualMachineGroupMemberStatus,
	updatePowerState bool,
	nextPowerOnTime *time.Time,
) error {
	vmg := &vmopv1.VirtualMachineGroup{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: ctx.VMGroup.Namespace, Name: member.Name}, vmg); err != nil {
		ctx.Logger.Error(err, "Failed to get group member",
			"kind", member.Kind,
			"name", member.Name,
		)
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"GetError",
			err,
		)
		return err
	}

	if vmg.Spec.GroupName != ctx.VMGroup.Name {
		err := fmt.Errorf("member has a different group name: %s",
			vmg.Spec.GroupName)
		ctx.Logger.Error(err, "Invalid group name for member",
			"kind", member.Kind,
			"name", member.Name,
			"groupName", vmg.Spec.GroupName,
		)
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"InvalidGroupName",
			err,
		)
		return err
	}

	patch := client.MergeFrom(vmg.DeepCopy())

	// Set owner reference to the parent group.
	if err := controllerutil.SetControllerReference(ctx.VMGroup, vmg, r.Scheme()); err != nil {
		ctx.Logger.Error(err, "Failed to set owner reference to group member",
			"kind", member.Kind,
			"name", member.Name,
		)
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"SetOwnerRefError",
			err,
		)
		return err
	}

	if updatePowerState {
		if d := member.PowerOnDelay; d != nil {
			*nextPowerOnTime = nextPowerOnTime.Add(d.Duration)
		}
		updateMemberPowerState(*ctx.VMGroup, vmg, *nextPowerOnTime)
	}

	if err := r.Patch(ctx, vmg, patch); err != nil {
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"OwnerRefPatchError",
			err,
		)
		return fmt.Errorf("failed to patch group member, kind=%s, name=%s: %w",
			member.Kind, member.Name, err)
	}

	conditions.MarkTrue(
		ms,
		vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
	)

	// Pass down the member's ready condition to member status.
	conditions.SetMirror(ms, vmopv1.ReadyConditionType, vmg)

	return nil
}

// TODO(sai): Implement placement logic for all unplaced VMs in the group.
func (r *Reconciler) reconcilePlacement(
	ctx *pkgctx.VirtualMachineGroupContext) error {

	return nil
}

// shouldUpdatePowerState returns true if the group's power state is set and
// the last updated power state time in annotation is after the status.
// It also returns the last updated power state time of the group in annotation.
func shouldUpdatePowerState(
	ctx *pkgctx.VirtualMachineGroupContext) (bool, time.Time, error) {

	if ctx.VMGroup.Spec.PowerState == "" {
		ctx.Logger.V(4).Info("Group's Spec.PowerState is not set, skipping")
		return false, time.Time{}, nil
	}

	var (
		lastUpdateAnnotation time.Time
		err                  error
	)

	if val := ctx.VMGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]; val != "" {
		lastUpdateAnnotation, err = time.Parse(time.RFC3339Nano, val)
		if err != nil {
			ctx.Logger.Error(err, "Failed to parse last updated power state time",
				"annotationKey", constants.LastUpdatedPowerStateTimeAnnotation,
				"annotationValue", val)
			return false, time.Time{}, err
		}
	}

	if t := ctx.VMGroup.Status.LastUpdatedPowerStateTime; t != nil && t.After(lastUpdateAnnotation) {
		ctx.Logger.V(4).Info("Last updated power state time in status is after the annotation, skipping")
		return false, lastUpdateAnnotation, nil
	}

	return true, lastUpdateAnnotation, nil
}

// updateMemberPowerState updates all the required power state fields on a given
// member object.
func updateMemberPowerState(
	group vmopv1.VirtualMachineGroup,
	member client.Object,
	nextPowerOnTime time.Time,
) {

	switch obj := member.(type) {
	case *vmopv1.VirtualMachine:
		obj.Spec.PowerState = group.Spec.PowerState
		obj.Spec.PowerOffMode = group.Spec.PowerOffMode
		obj.Spec.SuspendMode = group.Spec.SuspendMode

		if obj.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn {
			if obj.Annotations == nil {
				obj.Annotations = make(map[string]string)
			}
			obj.Annotations[constants.ApplyPowerStateTimeAnnotation] = nextPowerOnTime.Format(time.RFC3339Nano)
		}

	case *vmopv1.VirtualMachineGroup:
		obj.Spec.PowerState = group.Spec.PowerState
		obj.Spec.PowerOffMode = group.Spec.PowerOffMode
		obj.Spec.SuspendMode = group.Spec.SuspendMode

		if obj.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn {
			if obj.Annotations == nil {
				obj.Annotations = make(map[string]string)
			}
			obj.Annotations[constants.ApplyPowerStateTimeAnnotation] = nextPowerOnTime.Format(time.RFC3339Nano)
		}
	}
}

// setReadyCondition sets the group's Ready condition to True if there are no
// errors and all the group's members have all their expected conditions ready.
func setReadyCondition(ctx *pkgctx.VirtualMachineGroupContext, err error) {

	if err == nil && len(ctx.VMGroup.Status.Members) == 0 {
		conditions.MarkTrue(ctx.VMGroup, vmopv1.ReadyConditionType)
		return
	}

	var (
		notReadyCount  int
		failureReasons []string
	)

	for _, member := range ctx.VMGroup.Status.Members {
		if ready, reason := isMemberReady(member); !ready {
			notReadyCount++
			failureReasons = append(failureReasons, reason)
		}
	}

	if notReadyCount > 0 {
		// At least one of the members is not ready, set the group's Ready
		// condition to False with the specific member's failure reason.
		msg := fmt.Sprintf("%d of %d members not ready",
			notReadyCount, len(ctx.VMGroup.Status.Members))
		if len(failureReasons) > 0 {
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(failureReasons, ";"))
		}
		conditions.MarkFalse(
			ctx.VMGroup,
			vmopv1.ReadyConditionType,
			"MembersNotReady",
			"%s",
			msg,
		)
		return
	}

	// Members have all their expected conditions ready, set the group's Ready
	// condition to True if there are no errors.
	if err == nil {
		conditions.MarkTrue(ctx.VMGroup, vmopv1.ReadyConditionType)
	} else {
		conditions.MarkError(
			ctx.VMGroup,
			vmopv1.ReadyConditionType,
			"Error",
			err,
		)
	}
}

// isMemberReady checks if a member has all expected conditions set to true.
func isMemberReady(ms vmopv1.VirtualMachineGroupMemberStatus) (bool, string) {
	// Define expected conditions based on member kind.
	var expectedConditions []string
	switch ms.Kind {
	case vmKind:
		expectedConditions = []string{
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			// TODO(sai): Uncomment this once the placement is implemented.
			// vmopv1.VirtualMachineConditionPlacementReady,
		}
	case vmgKind:
		expectedConditions = []string{
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			vmopv1.ReadyConditionType,
		}
	}

	conditionMap := make(map[string]*metav1.Condition)
	for _, condition := range ms.Conditions {
		conditionMap[condition.Type] = &condition
	}

	memberKey := fmt.Sprintf("%s/%s", ms.Kind, ms.Name)
	for _, expectedType := range expectedConditions {
		condition, exists := conditionMap[expectedType]
		if !exists {
			return false, fmt.Sprintf("%s missing condition %s",
				memberKey, expectedType)
		}

		if condition.Status != metav1.ConditionTrue {
			return false, fmt.Sprintf("%s condition %s is %s",
				memberKey, expectedType, condition.Status)
		}
	}

	return true, ""
}
