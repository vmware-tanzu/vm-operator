// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
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
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		mgr.GetAPIReader(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorder(controllerNameShort)),
		ctx.VMProvider,
	)

	c := ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Watches(&vmopv1.VirtualMachineGroup{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.GroupToMembersMapperFn(ctx, r.Client, vmgKind))).
		Watches(&vmopv1.VirtualMachineGroup{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.MemberToGroupMapperFn(ctx))).
		Watches(&vmopv1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.MemberToGroupMapperFn(ctx)))

	if pkgcfg.FromContext(ctx).Features.VSpherePolicies {
		c.Watches(&vspherepolv1.PolicyEvaluation{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.PolicyEvalToVMToVMGroupMapperFunc(ctx, r.Client)))
	}

	c.WithOptions(controller.Options{
		MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, ctx.MaxConcurrentReconciles),
		LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
	})

	return c.Complete(r)
}

// NewReconciler returns a new reconciler for VirtualMachineGroup objects.
func NewReconciler(
	ctx context.Context,
	client client.Client,
	apiReader client.Reader,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		APIReader:  apiReader,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles a VirtualMachineGroup object.
type Reconciler struct {
	client.Client
	APIReader  client.Reader
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
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

	vmGroup, err := r.ensurePlacementStatusUpToDate(ctx, vmGroup)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmGroupCtx := &pkgctx.VirtualMachineGroupContext{
		Context: ctx,
		Logger:  pkglog.FromContextOrDefault(ctx),
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

	return pkgerr.ResultFromError(r.ReconcileNormal(vmGroupCtx))
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

func (r *Reconciler) ReconcileNormal(
	ctx *pkgctx.VirtualMachineGroupContext) (reterr error) {

	if !controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.AddFinalizer(ctx.VMGroup, finalizerName)
		return nil
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
		// A requeue is a normal wait, not an error for the Ready condition.
		if pkgerr.IsRequeueError(reterr) {
			setReadyCondition(ctx, nil)
		} else {
			setReadyCondition(ctx, reterr)
		}
	}()

	// Reconcile spec.groupName first as it's required for other reconciles
	// (e.g. placement and boot order delays) if this group is a nested group.
	if err := r.reconcileGroupName(ctx); err != nil {
		reterr = fmt.Errorf("failed to reconcile group name: %w", err)
		return reterr
	}

	// A requeue from the members, while a power-off waits for the time set
	// by the parent group, does not stop the placement.
	var requeueErr error
	if err := r.reconcileMembers(ctx); err != nil {
		if !pkgerr.IsRequeueError(err) {
			reterr = fmt.Errorf("failed to reconcile group members: %w", err)
			return reterr
		}
		requeueErr = err
	}

	if err := r.reconcilePlacement(ctx); err != nil {
		reterr = fmt.Errorf("failed to reconcile group placement: %w", err)
		return reterr
	}

	return requeueErr
}

// reconcileGroupName reconciles the group.spec.groupName field.
func (r *Reconciler) reconcileGroupName(
	ctx *pkgctx.VirtualMachineGroupContext) error {

	// Call UpdateGroupLinkedCondition even if the group name is empty to delete
	// that condition if it existed previously.
	if err := vmopv1util.UpdateGroupLinkedCondition(
		ctx,
		ctx.VMGroup,
		r.Client,
	); err != nil {
		return fmt.Errorf("failed to update group linked condition: %w", err)
	}

	// Return an error if the group linked condition is explicitly set to false.
	if conditions.IsFalse(
		ctx.VMGroup,
		vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
	) {
		return fmt.Errorf("group is not linked to its parent group")
	}

	return nil
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

	applyPowerStateTime, err := getApplyPowerStateTime(
		ctx, updatePowerState, lastUpdateAnnoTime)
	if err != nil {
		return err
	}

	// Power-off releases one boot order group at a time, from the last to the
	// first, once the previous boot order group has powered off.
	powerOff := updatePowerState &&
		ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOff

	var (
		release    powerOffRelease
		releaseErr error
	)
	if powerOff {
		release, releaseErr = getPowerOffRelease(ctx, r.Client, applyPowerStateTime)
		if releaseErr != nil {
			// Still reconcile the members, without releasing any of them.
			release = powerOffRelease{index: len(ctx.VMGroup.Spec.BootOrder)}
		}
	}

	var (
		memberStatuses = []vmopv1.VirtualMachineGroupMemberStatus{}
		memberErrs     = []error{}
	)

	// Determine the order of boot order group iteration based on the desired
	// power state. Power-on iterates forward using PowerOnDelay; power-off
	// iterates in reverse.
	bootOrderIndices := make([]int, len(ctx.VMGroup.Spec.BootOrder))
	for i := range bootOrderIndices {
		bootOrderIndices[i] = i
	}
	if ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOff {
		slices.Reverse(bootOrderIndices)
	}

	for _, i := range bootOrderIndices {
		bootOrder := ctx.VMGroup.Spec.BootOrder[i]

		var op memberPowerOp
		switch {
		case powerOff:
			op = release.memberPowerOp(i)
		case updatePowerState:
			if ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn &&
				bootOrder.PowerOnDelay != nil {
				applyPowerStateTime = applyPowerStateTime.Add(bootOrder.PowerOnDelay.Duration)
			}
			op = memberPowerOp{action: memberPowerSync, applyTime: applyPowerStateTime}
		}

		for _, member := range bootOrder.Members {
			key := member.Kind + "/" + member.Name

			// Check if we have an existing status to update, or create new one.
			var ms *vmopv1.VirtualMachineGroupMemberStatus
			if s, ok := existingStatuses[key]; ok {
				ms = s.DeepCopy()
			} else {
				ms = &vmopv1.VirtualMachineGroupMemberStatus{
					Name: member.Name,
					Kind: member.Kind,
				}
			}

			if err := r.reconcileMember(ctx, member, ms, op); err != nil {
				memberErrs = append(memberErrs, err)
			}

			memberStatuses = append(memberStatuses, *ms)
		}
	}

	if releaseErr != nil {
		memberErrs = append(memberErrs, releaseErr)
	}

	// Only update the last updated power state time in status if no errors.
	// This ensures the requeue continues to apply the group power state.
	// A power-off also stays pending until every boot order group has
	// powered off.
	if updatePowerState && len(memberErrs) == 0 && (!powerOff || release.done()) {
		if powerOff {
			ctx.Logger.Info("All boot order groups are powered off")
		}
		ctx.VMGroup.Status.LastUpdatedPowerStateTime = &metav1.Time{
			Time: time.Now().UTC(),
		}
	}

	ctx.VMGroup.Status.Members = memberStatuses

	err = aggregateOrNoRequeue(memberErrs)

	// Requeue when the release is held for the time set by the parent group.
	// Other errors take precedence as they are retried with a backoff.
	if powerOff && release.holdFor > 0 &&
		(err == nil || pkgerr.IsNoRequeueError(err)) {
		return pkgerr.RequeueError{After: release.holdFor}
	}

	return err
}

// getApplyPowerStateTime returns the group's apply power state change time
// that may be set from its parent group for power-on or power-off delay. If
// it is not set, this group's power state was being changed directly (not
// inherited from a parent), and the last updated annotation timestamp is
// returned as the base time for calculating members' delays.
func getApplyPowerStateTime(
	ctx *pkgctx.VirtualMachineGroupContext,
	updatePowerState bool,
	lastUpdateAnnoTime time.Time) (time.Time, error) {

	if updatePowerState && (ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn ||
		ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOff) {
		if v := ctx.VMGroup.Annotations[constants.ApplyPowerStateTimeAnnotation]; v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				ctx.Logger.Error(err, "Failed to parse time from annotation",
					"annotationKey", constants.ApplyPowerStateTimeAnnotation,
					"annotationValue", v)
				return time.Time{}, err
			}
			return t, nil
		}
	}

	return lastUpdateAnnoTime, nil
}

// memberPowerAction describes what is done to a group member's power state.
type memberPowerAction int

const (
	// memberPowerNone leaves the member's power state unchanged.
	memberPowerNone memberPowerAction = iota

	// memberPowerSync applies the group's power state to the member.
	memberPowerSync

	// memberPowerOff releases the member for a power-off. The apply power
	// state time is only set when the member's power state changes, so that
	// a later reconcile does not move it.
	memberPowerOff

	// memberPowerQueued leaves the member's power state unchanged while it
	// waits for an earlier boot order group to power off.
	memberPowerQueued
)

// memberPowerOp describes how a group member's power state is reconciled.
type memberPowerOp struct {
	action memberPowerAction

	// applyTime is the apply power state time set on the member. When zero
	// for a power-off, any existing apply power state time is removed.
	applyTime time.Time
}

// powerOffRelease describes the boot order group released by a power-off.
type powerOffRelease struct {
	// index is the index in Spec.BootOrder of the boot order group to
	// release, or -1 when every boot order group has powered off.
	index int

	// holdFor is greater than zero when the group waits for the apply power
	// state time set by its parent group. No boot order group is released.
	holdFor time.Duration

	// applyTime is the apply power state time set on the released members.
	applyTime time.Time
}

func (p powerOffRelease) done() bool {
	return p.index < 0
}

// memberPowerOp returns how to reconcile the members of the boot order group
// at the given index in Spec.BootOrder.
func (p powerOffRelease) memberPowerOp(i int) memberPowerOp {
	switch {
	case p.holdFor > 0 || i < p.index:
		return memberPowerOp{action: memberPowerQueued}
	case i == p.index:
		return memberPowerOp{action: memberPowerOff, applyTime: p.applyTime}
	default:
		// The boot order group already powered off.
		return memberPowerOp{action: memberPowerOff}
	}
}

// getPowerOffRelease returns the boot order group to release for a power-off.
// The first boot order group that has not powered off, in reverse order, is
// released. It waits for its PowerOffDelay from the time it is released,
// except for the last boot order group, which has no earlier group to wait
// for. Nothing is released until the given apply power state time set by a
// parent group has passed.
func getPowerOffRelease(
	ctx *pkgctx.VirtualMachineGroupContext,
	c client.Reader,
	parentApplyTime time.Time) (powerOffRelease, error) {

	idx, err := vmopv1util.FirstIncompletePowerOffBootOrder(ctx, c, ctx.VMGroup)
	if err != nil {
		return powerOffRelease{}, fmt.Errorf(
			"failed to get boot order group to power off: %w", err)
	}

	release := powerOffRelease{index: idx}
	if release.done() {
		return release, nil
	}

	now := time.Now().UTC()

	if parentApplyTime.After(now) {
		release.holdFor = parentApplyTime.Sub(now)
		ctx.Logger.V(4).Info("Waiting for parent group to power off",
			"applyPowerStateTime", parentApplyTime)
		return release, nil
	}

	bootOrders := ctx.VMGroup.Spec.BootOrder
	if delay := bootOrders[idx].PowerOffDelay; idx != len(bootOrders)-1 &&
		delay != nil && delay.Duration > 0 {
		release.applyTime = now.Add(delay.Duration)
	}

	ctx.Logger.V(4).Info("Boot order group to power off",
		"bootOrderIndex", idx,
		"applyPowerStateTime", release.applyTime)

	return release, nil
}

// reconcileMember reconciles a group member and updates the member's status.
func (r *Reconciler) reconcileMember(
	ctx *pkgctx.VirtualMachineGroupContext,
	member vmopv1.GroupMember,
	ms *vmopv1.VirtualMachineGroupMemberStatus,
	op memberPowerOp,
) error {

	var obj vmopv1util.VirtualMachineOrGroup
	switch member.Kind {
	case vmKind:
		obj = &vmopv1.VirtualMachine{}
	case vmgKind:
		obj = &vmopv1.VirtualMachineGroup{}
	}

	memberKindAndName := member.Kind + "/" + member.Name

	if err := r.Get(ctx, client.ObjectKey{
		Namespace: ctx.VMGroup.Namespace,
		Name:      member.Name,
	}, obj); err != nil {

		if !apierrors.IsNotFound(err) {
			conditions.MarkError(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				"Error",
				err,
			)
			return err
		}

		conditions.MarkFalse(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"NotFound",
			"",
		)
		// Do not requeue as the group will be reconciled when the member is
		// created with the correct group name. Same as below for NotMember.
		return pkgerr.NoRequeueError{
			Message: fmt.Sprintf("member %q not found", memberKindAndName),
		}
	}

	if groupName := obj.GetGroupName(); groupName != ctx.VMGroup.Name {
		var groupNameErr error
		if groupName == "" {
			groupNameErr = fmt.Errorf("member %q has no group name",
				memberKindAndName)
		} else {
			groupNameErr = fmt.Errorf("member %q has different group name: %q",
				memberKindAndName,
				groupName)
		}

		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"NotMember",
			groupNameErr,
		)
		return pkgerr.NoRequeueError{Message: groupNameErr.Error()}
	}

	patch := client.MergeFrom(obj.DeepCopyObject().(vmopv1util.VirtualMachineOrGroup))

	if err := controllerutil.SetOwnerReference(
		ctx.VMGroup,
		obj,
		r.Scheme(),
	); err != nil {
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"SetOwnerRefError",
			err,
		)
		return fmt.Errorf("failed to set owner reference to member %q: %w",
			memberKindAndName, err)
	}

	if member.Kind == vmKind {
		vm := obj.(*vmopv1.VirtualMachine)

		vmStatusPowerState := obj.GetPowerState()

		if vmStatusPowerState == "" {
			ms.PowerState = nil
		} else {
			ms.PowerState = &vmStatusPowerState
		}

		if ctx.VMGroup.Spec.PowerState == "" {
			conditions.Delete(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			)
			goto patchMember
		}

		if vmStatusPowerState == ctx.VMGroup.Spec.PowerState {
			conditions.MarkTrue(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			)
			goto patchMember
		}

		vmSpecMatchGroup := vm.Spec.PowerState == ctx.VMGroup.Spec.PowerState
		switch {
		case op.action == memberPowerQueued && !vmSpecMatchGroup:
			// VM waits for an earlier boot order group to power off.
			conditions.MarkFalse(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"Queued",
				"",
			)
		case op.action != memberPowerNone || vmSpecMatchGroup:
			// VM power state is syncing with group or pending status update.
			conditions.MarkFalse(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"Pending",
				"",
			)
		default:
			// VM power state was changed directly outside the group.
			conditions.MarkFalse(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"NotSynced",
				"",
			)
		}
	}

	if member.Kind == vmgKind {
		if err := r.reconcileGroupMemberPowerOff(ctx, member, obj.(*vmopv1.VirtualMachineGroup), ms, op); err != nil {
			return err
		}
	}

	switch op.action {
	case memberPowerSync:
		// Update power state specs for both member types (VMs and VM Groups).
		updateMemberPowerState(*ctx.VMGroup, obj, op.applyTime)
	case memberPowerOff:
		if updateMemberPowerOff(*ctx.VMGroup, obj, op.applyTime) {
			ctx.Logger.Info("Released group member to power off",
				"member", memberKindAndName,
				"applyPowerStateTime", op.applyTime)
		}
	case memberPowerNone, memberPowerQueued:
	}

patchMember:
	if err := r.Patch(ctx, obj, patch); err != nil {
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"OwnerRefPatchError",
			err,
		)
		if op.action == memberPowerSync || op.action == memberPowerOff {
			conditions.MarkError(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"PowerStatePatchError",
				err,
			)
		}
		return fmt.Errorf("failed to patch group member %q: %w",
			memberKindAndName, err)
	}

	conditions.MarkTrue(
		ms,
		vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
	)

	// Pass down the Ready condition to member status for the group kind member.
	if member.Kind == vmgKind {
		conditions.SetMirror(ms, vmopv1.ReadyConditionType, obj)
	}

	return nil
}

// reconcileGroupMemberPowerOff updates the PowerStateSynced condition of a
// VirtualMachineGroup member during a power-off. The condition is only set
// during a power-off, as a group has no observed power state. When a released
// child group has not powered off and its last power state request is closed,
// for example because one of its members was powered on afterwards, a new
// request is started on the child group so that it powers off its members
// again.
func (r *Reconciler) reconcileGroupMemberPowerOff(
	ctx *pkgctx.VirtualMachineGroupContext,
	member vmopv1.GroupMember,
	child *vmopv1.VirtualMachineGroup,
	ms *vmopv1.VirtualMachineGroupMemberStatus,
	op memberPowerOp) error {

	if op.action != memberPowerOff && op.action != memberPowerQueued {
		conditions.Delete(ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)
		return nil
	}

	if op.action == memberPowerQueued &&
		child.Spec.PowerState != ctx.VMGroup.Spec.PowerState {
		conditions.MarkFalse(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			"Queued",
			"",
		)
		return nil
	}

	ok, reason, err := vmopv1util.IsGroupMemberPoweredOff(
		ctx,
		r.Client,
		ctx.VMGroup.Namespace,
		ctx.VMGroup.Name,
		member,
		ctx.VMGroup.Spec.PowerOffMode)
	if err != nil {
		conditions.MarkError(
			ms,
			vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			"Error",
			err,
		)
		return err
	}

	if ok {
		conditions.MarkTrue(ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)
		return nil
	}

	conditions.MarkFalse(
		ms,
		vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
		"Pending",
		"%s",
		reason,
	)

	if op.action == memberPowerOff &&
		child.Spec.PowerState == ctx.VMGroup.Spec.PowerState &&
		reopenPowerStateRequest(child) {
		ctx.Logger.Info("Started a new power state request on child group",
			"member", member.Kind+"/"+member.Name,
			"reason", reason)
	}

	return nil
}

// reopenPowerStateRequest starts a new power state request on a group whose
// last request is closed, so that it applies its power state to its members
// again. It returns true if a new request was started.
func reopenPowerStateRequest(group *vmopv1.VirtualMachineGroup) bool {
	closed := group.Status.LastUpdatedPowerStateTime
	if closed == nil {
		return false
	}
	if v := group.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]; v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil && !closed.After(t) {
			// The last request is still pending.
			return false
		}
	}

	if group.Annotations == nil {
		group.Annotations = make(map[string]string)
	}
	group.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] =
		time.Now().UTC().Format(time.RFC3339Nano)

	return true
}

// reconcilePlacement reconciles and updates the placement status of
// the members of the group and its children groups.
//
// It does so by collecting all the members across all groups that
// need to be placed and by calling into the VM provider to get
// placement recommendations from DRS. Group member statuses are then
// updated with the placement recommendations. VMs members that
// already exist are skipped from placement.
func (r *Reconciler) reconcilePlacement(
	ctx *pkgctx.VirtualMachineGroupContext) error {

	if ctx.VMGroup.Spec.GroupName != "" {
		// Placement is only done on the group root.
		return nil
	}

	// Initialize groupPatches map to collect all child group patches that need
	// to update their member placement status.
	groupPatches := make(map[*vmopv1.VirtualMachineGroup]client.Patch)

	groupPlacements, err := r.getPlacementMembers(ctx, ctx.VMGroup, groupPatches)
	if err != nil {
		return fmt.Errorf("failed to get all placement members: %w", err)
	}

	if len(groupPlacements) == 0 {
		pkglog.FromContextOrDefault(ctx).V(4).Info("No group members need placement")
	} else if err := r.VMProvider.PlaceVirtualMachineGroup(ctx, ctx.VMGroup, groupPlacements); err != nil {
		return fmt.Errorf("failed to place group members: %w", err)
	}

	// Patch all child groups regardless of whether they have members that need
	// placement to ensure their member placement conditions are always updated.
	var errs []error
	for group, patch := range groupPatches {
		if ctx.VMGroup.Name == group.Name {
			// Let the outer patch helper update the root group.
			continue
		}
		if err := r.Status().Patch(ctx, group, patch); err != nil {
			errs = append(errs, fmt.Errorf(
				"failed to patch placement status for child group %q: %w",
				group.Name, err))
		}
	}

	return errors.Join(errs...)
}

func (r *Reconciler) getPlacementMembers(
	ctx context.Context,
	vmGroup *vmopv1.VirtualMachineGroup,
	groupPatches map[*vmopv1.VirtualMachineGroup]client.Patch,
) ([]providers.VMGroupPlacement, error) {

	var groupPlacements []providers.VMGroupPlacement
	groupPlacement := providers.VMGroupPlacement{
		VMGroup: vmGroup,
	}

	for _, bootOrder := range vmGroup.Spec.BootOrder {
		for _, member := range bootOrder.Members {
			switch member.Kind {
			case vmKind:
				if vm, err := r.getVMForPlacement(ctx, vmGroup, member.Name); err != nil {
					return nil, err
				} else if vm != nil {
					groupPlacement.VMMembers = append(groupPlacement.VMMembers, vm)
				}

			case vmgKind:
				groupMemberPlacements, err := r.getGroupsForPlacement(ctx, vmGroup, member.Name, groupPatches)
				if err != nil {
					return nil, err
				}

				groupPlacements = append(groupPlacements, groupMemberPlacements...)
			}
		}
	}

	if len(groupPlacement.VMMembers) > 0 {
		// This group has VMs that actually need to be placed.
		groupPlacements = append(groupPlacements, groupPlacement)
	}

	return groupPlacements, nil
}

func (r *Reconciler) getVMForPlacement(
	ctx context.Context,
	vmGroup *vmopv1.VirtualMachineGroup,
	vmName string) (vm *vmopv1.VirtualMachine, err error) {

	_, memberStatus := findMemberStatus(vmName, vmKind, vmGroup.Status.Members)
	if memberStatus == nil {
		return nil, fmt.Errorf("VM %q is not in group member status", vmName)
	}

	var (
		alreadyPlaced     bool
		alreadyPlacedZone string
	)

	defer func() {
		if err != nil {
			conditions.MarkError(
				memberStatus,
				vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
				"Error",
				err,
			)
			return
		}

		if vm != nil {
			// Let the provider update PlacementReady condition for this VM.
			return
		}

		// Set PlacementReady condition to True if the VM is already placed.
		// For VMs with zone label override, VMG will receive reconciliation
		// requests once they're actually placed and update the condition then.
		if alreadyPlaced {
			conditions.Set(memberStatus, &metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
				Status: metav1.ConditionTrue,
				Reason: "AlreadyPlaced",

				//
				// This is an API contract with VKS to surface the zone of the
				// already placed VM in this condition, so do not change.
				//
				Message: alreadyPlacedZone,
			})
		}
	}()

	if !conditions.IsTrue(memberStatus, vmopv1.VirtualMachineGroupMemberConditionGroupLinked) {
		return nil, fmt.Errorf("VM %q is not linked for group %q", vmName, vmGroup.Name)
	}

	vm = &vmopv1.VirtualMachine{}
	if err = r.Get(ctx, client.ObjectKey{Name: vmName, Namespace: vmGroup.Namespace}, vm); err != nil {
		return nil, fmt.Errorf("failed to get group member VM %q: %w", vmName, err)
	}

	if gn := vm.Spec.GroupName; gn != vmGroup.Name {
		return nil, fmt.Errorf("VM %q is assigned to group %q instead of expected %q", vmName, gn, vmGroup.Name)
	}

	// Skip if the group already has placement condition ready true for this VM.
	// Need to check the UID in case the VM is recreated with the same name and
	// without being removed from the group (could have stale placement status).
	if vm.GetUID() == memberStatus.UID && conditions.IsTrue(
		memberStatus,
		vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
	) {
		pkglog.FromContextOrDefault(ctx).V(4).Info(
			"Group already has placement condition ready for VM, skipping",
			"vmName", vmName,
			"vmUID", vm.GetUID(),
		)
		return nil, nil
	}

	// Group status doesn't have placement condition ready for this VM (UID).
	// Check if the VM is already placed or will be placed outside the group.

	memberStatus.UID = vm.GetUID()

	if vm.Status.UniqueID != "" {
		alreadyPlaced = true
		alreadyPlacedZone = vm.Status.Zone

		pkglog.FromContextOrDefault(ctx).V(4).Info(
			"VM has uniqueID, skipping group placement",
			"vmName", vmName,
			"uniqueID", vm.Status.UniqueID,
			"zone", vm.Status.Zone,
		)
		return nil, nil
	}

	if zoneName := vm.Labels[corev1.LabelTopologyZone]; zoneName != "" {
		isVksNodeVM := kubeutil.HasCAPILabels(vm.Labels)
		// A zone-labeled VM bypasses group placement to honor its zone override
		// when VMAffinityDuringExecution is disabled, or when it is a VKS node
		// (CAPI-managed) which must stay pinned to its assigned zone.
		if !pkgcfg.FromContext(ctx).Features.VMAffinityDuringExecution ||
			isVksNodeVM {
			pkglog.FromContextOrDefault(ctx).V(4).Info(
				"VM has explicit zone label, skipping group placement",
				"vmName", vmName,
				"zoneName", zoneName,
			)
			return nil, nil
		}

		// Feature enabled + non-VKS: fall through so the zone-labeled VM still
		// participates in group placement for DRS host recommendations.
		pkglog.FromContextOrDefault(ctx).V(4).Info(
			"VM has explicit zone label but participates in group placement "+
				"(VMAffinityDuringExecution enabled, non-VKS)",
			"vmName", vmName,
			"zoneName", zoneName,
		)
	}

	// VMG doesn't have a PlacementReady condition true for this VM (UID).
	// VM is not already placed, and either has no zone label override or is a
	// non-VKS zone-labeled VM participating in group placement for VM-VM
	// Affinity/Anti-Affinity at host level within the zone.
	// Return this VM to get it placed by the group.
	return vm, nil
}

func (r *Reconciler) getGroupsForPlacement(
	ctx context.Context,
	parentVMGroup *vmopv1.VirtualMachineGroup,
	groupName string,
	groupPatches map[*vmopv1.VirtualMachineGroup]client.Patch,
) ([]providers.VMGroupPlacement, error) {

	if _, member := findMemberStatus(groupName, vmgKind, parentVMGroup.Status.Members); member != nil {
		if !conditions.IsTrue(member, vmopv1.VirtualMachineGroupMemberConditionGroupLinked) {
			return nil, fmt.Errorf("VM Group %q is not linked for parent group %q", groupName, parentVMGroup.Name)
		}
	} else {
		return nil, fmt.Errorf("VM Group %q is not in parent group %q member status", groupName, parentVMGroup.Name)
	}

	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := r.Get(ctx, client.ObjectKey{Name: groupName, Namespace: parentVMGroup.Namespace}, vmGroup); err != nil {
		return nil, fmt.Errorf("failed to get group member group %s: %w", groupName, err)
	}

	if _, exists := groupPatches[vmGroup]; exists {
		return nil, fmt.Errorf("cycle detected in group hierarchy: %q", groupName)
	}

	vmGroup, err := r.ensurePlacementStatusUpToDate(ctx, vmGroup)
	if err != nil {
		return nil, err
	}

	// Initialize patch for this child group to update its member placement status if needed.
	groupPatches[vmGroup] = client.MergeFrom(vmGroup.DeepCopy())

	return r.getPlacementMembers(ctx, vmGroup, groupPatches)
}

func findMemberStatus(
	name, kind string,
	members []vmopv1.VirtualMachineGroupMemberStatus) (int, *vmopv1.VirtualMachineGroupMemberStatus) { //nolint:unparam

	for idx := range members {
		member := &members[idx]
		if member.Name == name && member.Kind == kind {
			return idx, member
		}
	}

	return -1, nil
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

	if t := ctx.VMGroup.Status.LastUpdatedPowerStateTime; t != nil &&
		t.After(lastUpdateAnnotation) {
		ctx.Logger.V(4).Info(
			"Last updated time in status is after the annotation, skipping")
		return false, lastUpdateAnnotation, nil
	}

	return true, lastUpdateAnnotation, nil
}

// updateMemberPowerState updates all the required power state fields on a given
// member object.
func updateMemberPowerState(
	group vmopv1.VirtualMachineGroup,
	member client.Object,
	applyPowerStateTime time.Time,
) {

	setMemberPowerSpec(group, member)

	if group.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn ||
		group.Spec.PowerState == vmopv1.VirtualMachinePowerStateOff {
		setApplyPowerStateTime(member, applyPowerStateTime)
	}
}

// updateMemberPowerOff updates the power state fields on a member released by
// a power-off. The apply power state time is only set when the member's
// desired power state differs from the group's, so that a later reconcile
// does not move it. When applyTime is zero, any existing apply power state
// time is removed. It returns true if the member's desired power state was
// changed.
func updateMemberPowerOff(
	group vmopv1.VirtualMachineGroup,
	member client.Object,
	applyTime time.Time,
) bool {

	released := memberSpecPowerState(member) != group.Spec.PowerState

	setMemberPowerSpec(group, member)

	if released {
		setApplyPowerStateTime(member, applyTime)
	}

	return released
}

// memberSpecPowerState returns the desired power state of a member.
func memberSpecPowerState(member client.Object) vmopv1.VirtualMachinePowerState {
	switch obj := member.(type) {
	case *vmopv1.VirtualMachine:
		return obj.Spec.PowerState
	case *vmopv1.VirtualMachineGroup:
		return obj.Spec.PowerState
	}
	return ""
}

// setMemberPowerSpec sets the group's power state and power modes on a member.
func setMemberPowerSpec(group vmopv1.VirtualMachineGroup, member client.Object) {
	switch obj := member.(type) {
	case *vmopv1.VirtualMachine:
		obj.Spec.PowerState = group.Spec.PowerState
		obj.Spec.PowerOffMode = group.Spec.PowerOffMode
		obj.Spec.SuspendMode = group.Spec.SuspendMode
	case *vmopv1.VirtualMachineGroup:
		obj.Spec.PowerState = group.Spec.PowerState
		obj.Spec.PowerOffMode = group.Spec.PowerOffMode
		obj.Spec.SuspendMode = group.Spec.SuspendMode
	}
}

// setApplyPowerStateTime sets the apply power state time annotation on a
// member, or removes it when t is zero.
func setApplyPowerStateTime(member client.Object, t time.Time) {
	annotations := member.GetAnnotations()
	if t.IsZero() {
		delete(annotations, constants.ApplyPowerStateTimeAnnotation)
	} else {
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[constants.ApplyPowerStateTimeAnnotation] = t.Format(time.RFC3339Nano)
	}
	member.SetAnnotations(annotations)
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
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(failureReasons, "; "))
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

// isMemberReady checks if a member has all expected conditions set to true and
// no existing conditions are set to false.
func isMemberReady(ms vmopv1.VirtualMachineGroupMemberStatus) (bool, string) {
	// Define expected conditions based on member kind.
	var expectedConditions []string
	switch ms.Kind {
	case vmKind:
		expectedConditions = []string{
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
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

	// Check if all the expected conditions exist and set to true.
	for _, expectedType := range expectedConditions {
		if c, ok := conditionMap[expectedType]; !ok {
			return false, fmt.Sprintf("%s missing condition %s",
				memberKey, expectedType)
		} else if c.Status != metav1.ConditionTrue {
			return false, fmt.Sprintf("%s condition %s is %s",
				memberKey, expectedType, c.Status)
		}
	}

	// Check if any existing conditions are set to false.
	for _, condition := range ms.Conditions {
		if condition.Status != metav1.ConditionTrue {
			return false, fmt.Sprintf("%s condition %s is %s",
				memberKey, condition.Type, condition.Status)
		}
	}

	return true, ""
}

// aggregateOrNoRequeue aggregates the given errors and returns a NoRequeueError
// if all the errors are NoRequeueError.
func aggregateOrNoRequeue(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	allNoRequeue := true
	for _, err := range errs {
		if !pkgerr.IsNoRequeueError(err) {
			allNoRequeue = false
			break
		}
	}

	agg := apierrorsutil.NewAggregate(errs)

	if allNoRequeue {
		return pkgerr.NoRequeueError{
			Message: agg.Error(),
		}
	}

	return agg
}

// ensurePlacementStatusUpToDate checks if all members defined in spec have the
// PlacementReady condition true in status.members. If any member doesn't have
// the PlacementReady condition true, it uses the API reader to get a fresh copy
// of the group object to avoid stale placement status.
// Returns the original group object if all VM-kind members are already placed.
// Otherwise, returns a fresh group object from the API reader directly.
func (r *Reconciler) ensurePlacementStatusUpToDate(
	ctx context.Context,
	vmGroup *vmopv1.VirtualMachineGroup) (*vmopv1.VirtualMachineGroup, error) {

	// Return early if the group has no members defined in the spec.
	if len(vmGroup.Spec.BootOrder) == 0 {
		return vmGroup, nil
	}

	// Build a map of all VM-kind members from the group's spec.
	specVMMembers := make(map[string]struct{})
	for _, bootOrder := range vmGroup.Spec.BootOrder {
		for _, member := range bootOrder.Members {
			if member.Kind == vmKind {
				specVMMembers[member.Name] = struct{}{}
			}
		}
	}

	// Build a map of all VM-kind members from the group's status.
	// Record whether the member has the PlacementReady condition true.
	statusVMMembers := make(map[string]bool, len(vmGroup.Status.Members))
	for _, memberStatus := range vmGroup.Status.Members {
		if memberStatus.Kind == vmKind {
			statusVMMembers[memberStatus.Name] = conditions.IsTrue(
				&memberStatus,
				vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
			)
		}
	}

	// Check if all spec members have PlacementReady condition true in status.
	allPlacementReady := true
	for vmMemberName := range specVMMembers {
		if !statusVMMembers[vmMemberName] {
			allPlacementReady = false
			break
		}
	}

	// If all members have PlacementReady condition true, no need to refresh.
	if allPlacementReady {
		return vmGroup, nil
	}

	// Use api reader to get a fresh copy to avoid stale placement status.
	freshGroup := &vmopv1.VirtualMachineGroup{}
	if err := r.APIReader.Get(ctx, client.ObjectKey{
		Namespace: vmGroup.Namespace,
		Name:      vmGroup.Name,
	}, freshGroup); err != nil {
		return nil, fmt.Errorf("failed to get group from API reader: %w", err)
	}

	return freshGroup, nil
}
