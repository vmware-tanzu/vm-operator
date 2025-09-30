// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Watches(&vmopv1.VirtualMachineGroup{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.GroupToMembersMapperFn(ctx, r.Client, vmgKind))).
		Watches(&vmopv1.VirtualMachineGroup{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.MemberToGroupMapperFn(ctx))).
		Watches(&vmopv1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(vmopv1util.MemberToGroupMapperFn(ctx))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

// NewReconciler returns a new reconciler for VirtualMachineGroup objects.
func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles a VirtualMachineGroup object.
type Reconciler struct {
	client.Client
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
		setReadyCondition(ctx, reterr)
	}()

	// Reconcile spec.groupName first as it's required for other reconciles
	// (e.g. placement and boot order delays) if this group is a nested group.
	if err := r.reconcileGroupName(ctx); err != nil {
		reterr = fmt.Errorf("failed to reconcile group name: %w", err)
		return reterr
	}

	if err := r.reconcileMembers(ctx); err != nil {
		reterr = fmt.Errorf("failed to reconcile group members: %w", err)
		return reterr
	}

	if err := r.reconcilePlacement(ctx); err != nil {
		reterr = fmt.Errorf("failed to reconcile group placement: %w", err)
		return reterr
	}

	return nil
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
		memberStatuses = []vmopv1.VirtualMachineGroupMemberStatus{}
		memberErrs     = []error{}
	)

	for _, bootOrder := range ctx.VMGroup.Spec.BootOrder {
		if ctx.VMGroup.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn &&
			bootOrder.PowerOnDelay != nil {
			applyPowerOnTime = applyPowerOnTime.Add(bootOrder.PowerOnDelay.Duration)
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

			if err := r.reconcileMember(
				ctx, member, ms, updatePowerState, applyPowerOnTime,
			); err != nil {
				memberErrs = append(memberErrs, err)
			}

			memberStatuses = append(memberStatuses, *ms)
		}
	}

	if updatePowerState && len(memberErrs) == 0 {
		// Only update the last updated power state time in status if no errors.
		// This ensures the requeue continues to apply the group power state.
		ctx.VMGroup.Status.LastUpdatedPowerStateTime = &metav1.Time{
			Time: time.Now().UTC(),
		}
	}

	ctx.VMGroup.Status.Members = memberStatuses

	return aggregateOrNoRequeue(memberErrs)
}

// reconcileMember reconciles a group member and updates the member's status.
func (r *Reconciler) reconcileMember(
	ctx *pkgctx.VirtualMachineGroupContext,
	member vmopv1.GroupMember,
	ms *vmopv1.VirtualMachineGroupMemberStatus,
	updatePowerState bool,
	applyPowerOnTime time.Time,
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
		if updatePowerState || vmSpecMatchGroup {
			// VM power state is syncing with group or pending status update.
			conditions.MarkFalse(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"Pending",
				"",
			)
		} else {
			// VM power state was changed directly outside the group.
			conditions.MarkFalse(
				ms,
				vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				"NotSynced",
				"",
			)
		}
	}

	if updatePowerState {
		// Update power state specs for both member types (VMs and VM Groups).
		updateMemberPowerState(*ctx.VMGroup, obj, applyPowerOnTime)
	}

patchMember:
	if err := r.Patch(ctx, obj, patch); err != nil {
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
		pkglog.FromContextOrDefault(ctx).V(5).Info("No group members need placement")
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

	defer func() {
		if err != nil {
			conditions.MarkError(
				memberStatus,
				vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
				"Error",
				err)
		} else if vm == nil {
			// If both error and vm are nil, it means the VM is already placed,
			// or will be placed outside the group. Update the PlacementReady
			// condition to True to be able to set the group as ready.
			conditions.MarkTrue(
				memberStatus,
				vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
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
	if vm.GetUID() == memberStatus.UID &&
		conditions.IsTrue(memberStatus, vmopv1.VirtualMachineGroupMemberConditionPlacementReady) {
		pkglog.FromContextOrDefault(ctx).V(5).Info(
			"Group already has placement condition ready for VM, skipping",
			"vmName", vmName,
			"vmUID", vm.GetUID(),
		)
		return nil, nil
	}

	memberStatus.UID = vm.GetUID()

	// If the VM has uniqueID set, then we don't need to do placement
	// for it.
	//
	// Preexisting VMs would already be reconciled and thus will have
	// MemberLinked condition set. Therefore, will not be part of
	// placement. But on the off chance that the condition doesn't
	// exist, we still check for existing VMs explicitly.
	if vm.Status.UniqueID != "" {
		pkglog.FromContextOrDefault(ctx).V(5).Info(
			"VM has uniqueID, skipping group placement",
			"vmName", vmName,
			"uniqueID", vm.Status.UniqueID,
		)
		return nil, nil
	}

	// If the VM has an explicit zone label, skip group placement to respect the
	// zone override.
	if zoneName := vm.Labels[corev1.LabelTopologyZone]; zoneName != "" {
		pkglog.FromContextOrDefault(ctx).V(5).Info(
			"VM has explicit zone label, skipping group placement",
			"vmName", vmName,
			"zoneName", zoneName,
		)
		return nil, nil
	}

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

	// Initialize patch for this child group if it will need status updates.
	if _, exists := groupPatches[vmGroup]; !exists {
		groupPatches[vmGroup] = client.MergeFrom(vmGroup.DeepCopy())
	}

	// TODO: Detect cycles
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
	applyPowerOnTime time.Time,
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
			obj.Annotations[constants.ApplyPowerStateTimeAnnotation] = applyPowerOnTime.Format(time.RFC3339Nano)
		}

	case *vmopv1.VirtualMachineGroup:
		obj.Spec.PowerState = group.Spec.PowerState
		obj.Spec.PowerOffMode = group.Spec.PowerOffMode
		obj.Spec.SuspendMode = group.Spec.SuspendMode

		if obj.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn {
			if obj.Annotations == nil {
				obj.Annotations = make(map[string]string)
			}
			obj.Annotations[constants.ApplyPowerStateTimeAnnotation] = applyPowerOnTime.Format(time.RFC3339Nano)
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
			// TODO(sai): Uncomment this once the placement is implemented.
			// vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
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
