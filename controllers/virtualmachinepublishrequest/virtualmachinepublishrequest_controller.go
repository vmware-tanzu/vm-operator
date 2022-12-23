// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinepublishrequest

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	TaskDescriptionID = "com.vmware.ovfs.LibraryItem.capture"

	// waitForTaskTimeout represents the timeout to wait for task existence in task manager.
	// When calling `CreateOVF` API, there is no guarantee that we can get its task info due to
	// - this request is not made to VC, e.g. VC credentials rotate.
	// - VC receives this request, but fails to submit this task.
	// - content library service hasn't processed far enough to submit and register this task. (Most common case)
	// Wait for 30 seconds to eliminate the last case in a best-effort manner.
	waitForTaskTimeout = 30 * time.Second

	clibItemPrefix = "clibitem-"
)

var (
	// DefaultRequeueDelaySeconds represents the default requeue delay seconds when reconcile.
	DefaultRequeueDelaySeconds = 60
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.VirtualMachinePublishRequest{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Watches(&source.Kind{Type: &vmopv1alpha1.VirtualMachineImage{}},
			handler.EnqueueRequestsFromMapFunc(vmiToVMPubMapperFn(ctx, r.Client))).
		Complete(r)
}

// vmiToVMPubMapperFn returns a mapper function that can be used to queue reconcile request
// for the VirtualMachinePublishRequests in response to an event on the VirtualMachineImage resource.
// Note: Only when WCP_VM_Image_Registry FSS is enabled, this controller will be added to the controller manager.
// In this case, the VirtualMachineImage is a namespace scoped resource.
func vmiToVMPubMapperFn(ctx *context.ControllerManagerContext, c client.Client) func(o client.Object) []reconcile.Request {
	// For a given VirtualMachineImage, return reconcile requests
	// for those VirtualMachinePublishRequests with corresponding VirtualMachinesImage as the target item.
	return func(o client.Object) []reconcile.Request {
		vmi := o.(*vmopv1alpha1.VirtualMachineImage)
		logger := ctx.Logger.WithValues("name", vmi.Name, "namespace", vmi.Namespace)

		logger.V(4).Info("Reconciling all VirtualMachinePublishRequests referencing a target item name same " +
			"with this VirtualMachineImage display name")

		vmPubList := &vmopv1alpha1.VirtualMachinePublishRequestList{}
		if err := c.List(ctx, vmPubList, client.InNamespace(vmi.Namespace)); err != nil {
			logger.Error(err, "Failed to list VirtualMachinePublishRequests for reconciliation due to VirtualMachineImage watch")
			return nil
		}

		// Populate reconcile requests for vmpubs
		var reconcileRequests []reconcile.Request
		for _, vmPub := range vmPubList.Items {
			if vmPub.Status.TargetRef == nil {
				continue
			}

			if vmPub.Status.TargetRef.Item.Name == vmi.Status.ImageName {
				key := client.ObjectKey{Namespace: vmPub.Namespace, Name: vmPub.Name}
				reconcileRequests = append(reconcileRequests, reconcile.Request{NamespacedName: key})
			}
		}

		logger.V(4).Info("Returning VirtualMachinePublishRequest reconcile requests due to VirtualMachineImage watch",
			"requests", reconcileRequests)
		return reconcileRequests
	}
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
		Metrics:    metrics.NewVMPublishMetrics(),
	}
}

// Reconciler reconciles a VirtualMachinePublishRequest object.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
	Metrics    *metrics.VMPublishMetrics
}

func requeueDelay(ctx *context.VirtualMachinePublishRequestContext) time.Duration {
	vmPubReq := ctx.VMPublishRequest
	// Skip checking other conditions.
	// If ImageAvailable is true, Uploaded must be true. This also marks Condition Complete to true,
	// we will never reach this function.
	if vmPubReq.IsUploaded() {
		return 10 * time.Second
	}

	// VM Publish request is still in progress, requeue after one minute.
	return time.Duration(DefaultRequeueDelaySeconds) * time.Second
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get;list;watch
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries/status,verbs=get;

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	vmPublishReq := &vmopv1alpha1.VirtualMachinePublishRequest{}
	err := r.Get(ctx, req.NamespacedName, vmPublishReq)
	if err != nil {
		// Delete registered metrics if the resource is not found.
		if apiErrors.IsNotFound(err) {
			r.Metrics.DeleteMetrics(r.Logger, req.Name, req.Namespace)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmPublishCtx := &context.VirtualMachinePublishRequestContext{
		Context:          ctx,
		Logger:           ctrl.Log.WithName("VirtualMachinePublishRequest").WithValues("name", req.NamespacedName),
		VMPublishRequest: vmPublishReq,
	}

	if !vmPublishReq.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmPublishCtx)
	}

	return r.ReconcileNormal(vmPublishCtx)
}

func (r *Reconciler) updateSourceAndTargetRef(ctx *context.VirtualMachinePublishRequestContext) {
	vmPubReq := ctx.VMPublishRequest
	if vmPubReq.Status.SourceRef == nil {
		vmName := vmPubReq.Spec.Source.Name
		if vmName == "" {
			// set default source VM to this VirtualMachinePublishRequest's name.
			vmName = vmPubReq.Name
		}
		vmPubReq.Status.SourceRef = &vmopv1alpha1.VirtualMachinePublishRequestSource{
			Name: vmName,
		}
	}

	if vmPubReq.Status.TargetRef == nil {
		targetItemName := vmPubReq.Spec.Target.Item.Name
		if targetItemName == "" {
			targetItemName = fmt.Sprintf("%s-image", vmPubReq.Status.SourceRef.Name)
		}

		vmPubReq.Status.TargetRef = &vmopv1alpha1.VirtualMachinePublishRequestTarget{
			Item: vmopv1alpha1.VirtualMachinePublishRequestTargetItem{
				Name:        targetItemName,
				Description: vmPubReq.Spec.Target.Item.Description,
			},
			Location: vmPubReq.Spec.Target.Location,
		}
	}
}

// publishVirtualMachine checks if source VM and target is valid. Publish a VM if all requirements are met.
// Returns
// - A boolean value represents if the following Patch should be skipped.
// - error.
func (r *Reconciler) publishVirtualMachine(ctx *context.VirtualMachinePublishRequestContext) (bool, error) {
	vmPublishReq := ctx.VMPublishRequest
	// Check if the source and target is valid
	if err := r.checkIsSourceValid(ctx); err != nil {
		ctx.Logger.Error(err, "failed to check if source is valid")
		return false, err
	}

	if err := r.checkIsTargetValid(ctx); err != nil {
		ctx.Logger.Error(err, "failed to check if target is valid")
		return false, err
	}

	if vmPublishReq.IsSourceValid() && vmPublishReq.IsTargetValid() {
		vmPublishReq.Status.Attempts++
		vmPublishReq.Status.LastAttemptTime = metav1.Now()

		// Update VirtualMachinePublishRequest object to avoid conflict.
		// API server will reject the Update request if updating to a stale object, so that we can always
		// set the correct number of attempts in the status to avoid running into a situation where we end
		// up sending multiple publish requests with the same actID.
		//
		// Patch a stale object won't fail even when the resourceVersion doesn't match. In this case, we
		// may set .status.attempts to a same number multiple times using Patch.
		// Use Update instead and skip Patch in the ReconcileNormal defer function.
		if err := r.Client.Status().Update(ctx, vmPublishReq); err != nil {
			ctx.Logger.Error(err, "update VirtualMachinePublishRequest status failed")
			return true, err
		}

		go func() {
			actID := getPublishRequestActID(vmPublishReq)
			itemID, pubErr := r.VMProvider.PublishVirtualMachine(ctx, ctx.VM, vmPublishReq, ctx.ContentLibrary, actID)
			if pubErr != nil {
				ctx.Logger.Error(pubErr, "failed to publish VM")
			} else {
				ctx.Logger.Info("created an OVF from VM", "itemID", itemID)
			}
			r.Recorder.EmitEvent(vmPublishReq, "Publish", pubErr, false)
		}()
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) removeVMPubResourceFromCluster(ctx *context.VirtualMachinePublishRequestContext) (requeueAfter time.Duration,
	deleted bool, err error) {
	vmPublishReq := ctx.VMPublishRequest
	ttlSecondsAfterFinished := vmPublishReq.Spec.TTLSecondsAfterFinished
	if ttlSecondsAfterFinished == nil {
		// Skip auto clean up
		return
	}

	if *ttlSecondsAfterFinished > 0 {
		completeTime := vmPublishReq.Status.CompletionTime.Time
		if time.Since(completeTime) < time.Duration(*ttlSecondsAfterFinished)*time.Second {
			targetTime := completeTime.Add(time.Duration(*ttlSecondsAfterFinished) * time.Second)
			return time.Until(targetTime), false, nil
		}
	}

	// TTLSecondsAfterFinished elapsed, delete the resource
	ctx.Logger.Info("deleting VM Publish Request")
	deleted = true
	if err = r.Delete(ctx, vmPublishReq); err != nil {
		deleted = false
		ctx.Logger.Error(err, "failed to delete vm publish requests")
	}

	return
}

// checkIsSourceValid function checks if the source VM is valid. It is invalid if the VM k8s resource
// doesn't exist, or is not in Created phase.
func (r *Reconciler) checkIsSourceValid(ctx *context.VirtualMachinePublishRequestContext) error {
	vmPubReq := ctx.VMPublishRequest
	vm := &vmopv1alpha1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmPubReq.Status.SourceRef.Name, Namespace: vmPubReq.Namespace}
	err := r.Get(ctx, objKey, vm)
	if err != nil {
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		if apiErrors.IsNotFound(err) {
			// If not found the source VM, no need to requeue to cause another reconcile. Set error to nil.
			// Ideally this won't happen because we have a validation webhook to check source VM existence.
			errMsg := fmt.Sprintf("%s, a new VirtualMachinePublishRequest is needed to continue publishing.", err.Error())
			vmPubReq.MarkSourceValid(corev1.ConditionFalse, vmopv1alpha1.SourceVirtualMachineNotExistReason, errMsg)
			err = nil
		}
		return err
	}
	ctx.VM = vm

	if vm.Status.UniqueID == "" {
		err = fmt.Errorf("VM hasn't been created, phase: %s, unique ID: %s", vm.Status.Phase, vm.Status.UniqueID)
		vmPubReq.MarkSourceValid(corev1.ConditionFalse, vmopv1alpha1.SourceVirtualMachineNotCreatedReason, err.Error())
		return err
	}

	vmPubReq.MarkSourceValid(corev1.ConditionTrue)
	return nil
}

// checkIsTargetValid checks if the target item is valid.
// It is invalid if the content library doesn't exist, an item with the same name in the CL exists,
// or another vmpub request with the same name is in progress.
func (r *Reconciler) checkIsTargetValid(ctx *context.VirtualMachinePublishRequestContext) error {
	vmPubReq := ctx.VMPublishRequest
	contentLibrary := &imgregv1a1.ContentLibrary{}
	targetLocationName := vmPubReq.Spec.Target.Location.Name
	targetItemName := vmPubReq.Status.TargetRef.Item.Name
	objKey := client.ObjectKey{Name: targetLocationName, Namespace: vmPubReq.Namespace}
	if err := r.Get(ctx, objKey, contentLibrary); err != nil {
		ctx.Logger.Error(err, "failed to get ContentLibrary", "cl", objKey)
		if apiErrors.IsNotFound(err) {
			// If not found the target CL, no need to requeue to cause another reconcile. Set error to nil.
			// Ideally this won't happen because we have a validation webhook to check target CL existence.
			errMsg := fmt.Sprintf("%s, a new VirtualMachinePublishRequest is needed to continue publishing.", err.Error())
			vmPubReq.MarkTargetValid(corev1.ConditionFalse, vmopv1alpha1.TargetContentLibraryNotExistReason, errMsg)
			err = nil
		}
		return err
	}

	if !contentLibrary.Spec.Writable {
		err := fmt.Errorf("target location %s is not writable", contentLibrary.Status.Name)
		vmPubReq.MarkTargetValid(corev1.ConditionFalse, vmopv1alpha1.TargetContentLibraryNotWritableReason,
			err.Error())
		return err
	}

	ctx.ContentLibrary = contentLibrary
	exist, err := r.VMProvider.DoesItemExistInContentLibrary(ctx, contentLibrary, targetItemName)
	if err != nil {
		ctx.Logger.Error(err, "failed to find item", "cl", objKey, "item name", targetItemName)
		return err
	}
	if exist {
		err = fmt.Errorf("item with name %s already exists in the content library %s", targetItemName, contentLibrary.Status.Name)
		vmPubReq.MarkTargetValid(corev1.ConditionFalse, vmopv1alpha1.TargetItemAlreadyExistsReason, err.Error())
		return err
	}

	vmPubReq.MarkTargetValid(corev1.ConditionTrue)
	return nil
}

// checkIsImageAvailable checks if the published VirtualMachineImage resource is available in the cluster.
func (r *Reconciler) checkIsImageAvailable(ctx *context.VirtualMachinePublishRequestContext, itemID string) error {
	if !ctx.VMPublishRequest.IsUploaded() {
		return nil
	}

	if itemID == "" {
		id, err := r.getUploadedItemID(ctx)
		if err != nil {
			ctx.Logger.Error(err, "failed to get uploaded item UUID")
			return err
		}
		itemID = id
	}

	vmiList := &vmopv1alpha1.VirtualMachineImageList{}
	if err := r.Client.List(ctx, vmiList, client.InNamespace(ctx.VMPublishRequest.Namespace)); err != nil {
		ctx.Logger.Error(err, "failed to list VirtualMachineImage")
		return err
	}

	found := false
	for _, vmi := range vmiList.Items {
		if vmi.Spec.ImageID == itemID {
			found = true
			ctx.VMPublishRequest.Status.ImageName = vmi.Name
			ctx.VMPublishRequest.MarkImageAvailable(corev1.ConditionTrue)
			ctx.Logger.Info("VirtualMachineImage is available", "vmiName", vmi.Name)
			break
		}
	}

	if !found {
		ctx.VMPublishRequest.MarkImageAvailable(corev1.ConditionFalse,
			vmopv1alpha1.TargetVirtualMachineImageNotFoundReason, "VirtualMachineImage not found")
	}

	return nil
}

// checkIsComplete checks if condition Complete can be marked to true.
// The condition's status is set to true only when all other conditions present on the resource have a truthy status.
func (r *Reconciler) checkIsComplete(ctx *context.VirtualMachinePublishRequestContext) bool {
	if ctx.VMPublishRequest.IsComplete() {
		return true
	}

	if !ctx.VMPublishRequest.IsUploaded() {
		ctx.VMPublishRequest.MarkComplete(corev1.ConditionFalse, vmopv1alpha1.HasNotBeenUploadedReason,
			"item hasn't been uploaded yet.")
		return false
	}

	if !ctx.VMPublishRequest.IsImageAvailable() {
		ctx.VMPublishRequest.MarkComplete(corev1.ConditionFalse, vmopv1alpha1.ImageUnavailableReason,
			"VirtualMachineImage is not available")
		return false
	}

	ctx.VMPublishRequest.MarkComplete(corev1.ConditionTrue)
	ctx.VMPublishRequest.Status.Ready = true
	ctx.VMPublishRequest.Status.CompletionTime = metav1.Now()
	ctx.Logger.Info("VM publish request completed", "time", ctx.VMPublishRequest.Status.CompletionTime)

	return true
}

// getPublishRequestTask gets task with description id com.vmware.ovfs.LibraryItem.capture and specified actid in taskManager.
func (r *Reconciler) getPublishRequestTask(ctx *context.VirtualMachinePublishRequestContext) (*vimtypes.TaskInfo, error) {
	actID := getPublishRequestActID(ctx.VMPublishRequest)
	logger := ctx.Logger.WithValues("activationID", actID)

	tasks, err := r.VMProvider.GetTasksByActID(ctx, actID)
	if err != nil {
		logger.Error(err, "failed to get task")
		return nil, err
	}

	if len(tasks) == 0 {
		logger.V(5).Info("task doesn't exist", "actID", actID, "descriptionID", TaskDescriptionID)
		return nil, nil
	}

	// Return the first task.
	// We would never send multiple CreateOvf requests with the same actID,
	// so that we should never get multiple tasks.
	publishTask := &tasks[0]
	if publishTask.DescriptionId != TaskDescriptionID {
		err = fmt.Errorf("failed to find expected task %s, found %s instead", TaskDescriptionID, publishTask.DescriptionId)
		logger.Error(err, "task doesn't exist")
		return nil, err
	}

	return publishTask, nil
}

// checkPublishRequestStatus checks the publish request task status and mark Uploaded condition.
// Returns
// - if a following publish task is needed (true if a publish request should be sent, otherwise return false.),
// - the uploaded item UUID
// - error
// It filters VM Publish task from VC task manager by activation ID and checks its status.
// - If this task is queued/running, then we should mark Uploaded to false and no need to retry VM publish.
// - task succeeded, mark Uploaded to true and return the uploaded item ID.
// - task failed, mark Uploaded to false and retry the operation.
func (r *Reconciler) checkPublishRequestStatus(ctx *context.VirtualMachinePublishRequestContext) (string, bool, error) {
	if ctx.VMPublishRequest.Status.Attempts == 0 {
		// no VM publish task has been attempted. return immediately.
		return "", true, nil
	}

	if ctx.VMPublishRequest.IsUploaded() {
		return "", false, nil
	}

	actID := getPublishRequestActID(ctx.VMPublishRequest)
	logger := ctx.Logger.WithValues("actID", actID, "descriptionID", TaskDescriptionID)

	task, err := r.getPublishRequestTask(ctx)
	if err != nil {
		// Failed to get task from taskManager, return early and retry in next reconcile loop.
		return "", false, err
	}

	// If we can not find a relevant task, probably due to:
	// - Reason 1: this request is not made to VC, e.g. VC credentials rotate. Error returned.
	// - Reason 2: VC receives this request, but fails to submit this task. Error returned.
	// - Reason 3: content library service hasn't processed far enough to submit and register this task,
	// but will eventually succeed. (Most common)
	//
	// Ideally we should immediately return error for Reason 1 & 2. But we can't tell why we can't find that task for
	// sure since we don't track the CreateOVF error. Meanwhile, reason 1 & 2 are rare and won't be a big problem
	// if not schedule reconcile requests immediately.
	//
	// Wait for the configured timeout before returning an error.
	// This is done so that the controller doesn't schedule reconcile requests immediately and goes into exponential
	// backoff in case 3, and we are not submitting requests over and over when this publish task is actually running.
	if task == nil {
		if time.Since(ctx.VMPublishRequest.Status.LastAttemptTime.Time) > waitForTaskTimeout {
			// CreateOvf API failed to submit this task for some reason. In this case, retry VM publish.
			// Also return error here to goes into exponential backoff in case it is not transient.
			return "", true, fmt.Errorf("failed to create task: %s, last attempt time: %s",
				TaskDescriptionID, ctx.VMPublishRequest.Status.LastAttemptTime.String())
		}

		// CreateOvf request has been sent but the task com.vmware.ovfs.LibraryItem.capture hasn't been
		// submitted to the task manager yet. retry taskManager query at later reconcile.
		return "", false, nil
	}

	switch task.State {
	case vimtypes.TaskInfoStateQueued:
		logger.V(5).Info("VM Publish task is queued but hasn't started")
		ctx.VMPublishRequest.MarkUploaded(corev1.ConditionFalse, vmopv1alpha1.UploadTaskQueuedReason, "VM Publish task is queued.")
		return "", false, nil
	case vimtypes.TaskInfoStateRunning:
		// CreateOVF is still in progress
		// TODO: VCLCO-2927 Add task Progress info.
		logger.V(5).Info("VM Publish is still in progress", "progress", task.Progress)
		ctx.VMPublishRequest.MarkUploaded(corev1.ConditionFalse, vmopv1alpha1.UploadingReason, "Uploading item to content library.")
		return "", false, nil
	case vimtypes.TaskInfoStateSuccess:
		// Publish request succeeds. Update Uploaded condition.
		ctx.VMPublishRequest.MarkUploaded(corev1.ConditionTrue)
		logger.Info("VM Publish succeeded", "result", task.Result)
		itemID, err := parseItemIDFromTaskResult(task.Result)
		if err != nil {
			// Only log this error. We can retry this operation when checking if VMI is available.
			logger.Error(err, "failed to get uploaded item UUID")
		}
		return itemID, false, nil
	case vimtypes.TaskInfoStateError:
		errMsg := "failed to publish source VM"
		if task.Error != nil {
			errMsg = task.Error.LocalizedMessage
		}

		logger.Error(err, "VM Publish failed, will retry this operation", "actID",
			task.ActivationId, "descriptionID", task.DescriptionId)
		ctx.VMPublishRequest.MarkUploaded(corev1.ConditionFalse, vmopv1alpha1.UploadFailureReason, errMsg)
		return "", true, fmt.Errorf(errMsg)
	}

	return "", true, nil
}

// getUploadedItemID returns the uploaded content library item ID.
func (r *Reconciler) getUploadedItemID(ctx *context.VirtualMachinePublishRequestContext) (string, error) {
	task, err := r.getPublishRequestTask(ctx)
	if err != nil {
		return "", err
	}

	if task == nil {
		return "", fmt.Errorf("VM publish task doesn't exist in the task manager, actID: %s",
			getPublishRequestActID(ctx.VMPublishRequest))
	}

	return parseItemIDFromTaskResult(task.Result)
}

// getPublishRequestActID returns the activation ID we pass down to the content library vAPI.
// Append .status.attempts to the vmpublishrequest uuid to generate an activation ID.
// VC doesn't prevent duplicate activation IDs for different requests, but we can have problems
// when doing the query, i.e. not returned all the tasks.
// So we need a unique string, which is also under track to be the actID for a later query.
func getPublishRequestActID(vmPub *vmopv1alpha1.VirtualMachinePublishRequest) string {
	return fmt.Sprintf("%s-%d", string(vmPub.UID), vmPub.Status.Attempts)
}

// parseItemIDFromTaskResult returns the content library item UUID from the task result.
func parseItemIDFromTaskResult(result vimtypes.AnyType) (string, error) {
	clItem, ok := result.(vimtypes.ManagedObjectReference)
	if !ok {
		return "", fmt.Errorf("failed to cast task result to ManagedObjectReference")
	}

	if clItem.Type != "ContentLibraryItem" {
		return "", fmt.Errorf("expect task result type to be ContentLibraryItem, get %s instead", clItem.Type)
	}

	if len(clItem.Value) <= len(clibItemPrefix) {
		return "", fmt.Errorf("content library item UUID is invalid")
	}
	return clItem.Value[len(clibItemPrefix):], nil
}

func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachinePublishRequestContext) (_ ctrl.Result, reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachinePublishRequest")
	vmPublishReq := ctx.VMPublishRequest

	// Register VM publish request metrics based on the reconcile result.
	var isComplete, isDeleted bool
	defer func() {
		if isDeleted {
			r.Metrics.DeleteMetrics(ctx.Logger, vmPublishReq.Name, vmPublishReq.Namespace)
			return
		}

		var res metrics.PublishResult
		switch {
		case isComplete:
			res = metrics.PublishSucceeded
		case reterr != nil:
			res = metrics.PublishFailed
		default:
			res = metrics.PublishInProgress
		}

		r.Metrics.RegisterVMPublishRequest(r.Logger, vmPublishReq.Name, vmPublishReq.Namespace, res)
	}()

	// In case the .spec.ttlSecondsAfterFinished is not set, we can return early and no need to do any reconcile.
	if isComplete = vmPublishReq.IsComplete(); isComplete {
		requeueAfter, deleted, err := r.removeVMPubResourceFromCluster(ctx)
		isDeleted = deleted
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	skipPatch := false
	patchHelper, err := patch.NewHelper(vmPublishReq, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", fmt.Sprintf("%s/%s", vmPublishReq.Namespace, vmPublishReq.Name))
	}

	var savedErr error
	defer func() {
		if reterr == nil && savedErr != nil {
			reterr = savedErr
		}

		if skipPatch {
			return
		}

		if err := patchHelper.Patch(ctx, vmPublishReq); err != nil {
			if reterr == nil {
				reterr = err
			}
			ctx.Logger.Error(err, "patch failed")
		}
	}()

	if vmPublishReq.Status.StartTime.IsZero() {
		vmPublishReq.Status.StartTime = metav1.Now()
	}

	r.updateSourceAndTargetRef(ctx)

	itemID, shouldPublish, err := r.checkPublishRequestStatus(ctx)
	if err != nil {
		// Save this error and return it in defer func().
		// If there is something wrong with VC, i.e. we can never find a task from VC task manager, this can help
		// fall into exponential backoff instead of sending publish VM requests over and over.
		savedErr = err
	}

	if shouldPublish {
		skipPatch, err = r.publishVirtualMachine(ctx)
		if err != nil {
			ctx.Logger.Error(err, "failed to publish VirtualMachine")
			return ctrl.Result{}, errors.Wrapf(err, "failed to publish VirtualMachine")
		}
		return ctrl.Result{RequeueAfter: requeueDelay(ctx)}, nil
	}

	if err = r.checkIsImageAvailable(ctx, itemID); err != nil {
		return ctrl.Result{}, err
	}

	if isComplete = r.checkIsComplete(ctx); isComplete {
		// remove VirtualMachinePublishRequest from the cluster if ttlSecondsAfterFinished is set.
		requeueAfter, deleted, err := r.removeVMPubResourceFromCluster(ctx)
		if deleted {
			skipPatch = true
		}
		isDeleted = deleted
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	return ctrl.Result{RequeueAfter: requeueDelay(ctx)}, nil
}

func (r *Reconciler) ReconcileDelete(ctx *context.VirtualMachinePublishRequestContext) (ctrl.Result, error) {
	r.Metrics.DeleteMetrics(ctx.Logger, ctx.VMPublishRequest.Name, ctx.VMPublishRequest.Namespace)
	// no op
	return ctrl.Result{}, nil
}
