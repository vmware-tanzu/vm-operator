// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	goctx "context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	metrics "github.com/vmware-tanzu/vm-operator/pkg/metrics2"
	patch "github.com/vmware-tanzu/vm-operator/pkg/patch2"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName     = "virtualmachinepublishrequest.vmoperator.vmware.com"
	TaskDescriptionID = "com.vmware.ovfs.LibraryItem.capture"

	// waitForTaskTimeout represents the timeout to wait for task existence in task manager.
	// When calling `CreateOVF` API, there is no guarantee that we can get its task info due to
	// - this request is not made to VC, e.g. VC credentials rotate.
	// - VC receives this request, but fails to submit this task.
	// - content library service hasn't processed far enough to submit and register this task. (Most common case)
	// Wait for 30 seconds to eliminate the last case in a best-effort manner.
	waitForTaskTimeout = 30 * time.Second

	clItemPrefix          = "clibitem-"
	ItemParseErrorMessage = "Failed to get the uploaded item ID. This error is unrecoverable." +
		" Please create a new VirtualMachinePublishRequest to retry VM publishing."

	// ItemDescriptionRegexString is used to filter the VMPub UID from the content library item description.
	ItemDescriptionRegexString = "virtualmachinepublishrequest\\.vmoperator\\.vmware\\.com: ([a-z0-9-]*)"
)

var (
	itemDescriptionReg = regexp.MustCompile(ItemDescriptionRegexString)
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachinePublishRequest{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		mgr.GetAPIReader(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProviderA2,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Watches(&source.Kind{Type: &vmopv1.VirtualMachineImage{}},
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
		vmi := o.(*vmopv1.VirtualMachineImage)
		logger := ctx.Logger.WithValues("name", vmi.Name, "namespace", vmi.Namespace)

		logger.V(4).Info("Reconciling all VirtualMachinePublishRequests referencing a target item name same " +
			"with this VirtualMachineImage display name")

		vmPubList := &vmopv1.VirtualMachinePublishRequestList{}
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

			if vmPub.Status.TargetRef.Item.Name == vmi.Status.Name {
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
	apiReader client.Reader,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterfaceA2) *Reconciler {

	return &Reconciler{
		Client:     client,
		apiReader:  apiReader,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
		Metrics:    metrics.NewVMPublishMetrics(),
	}
}

// Reconciler reconciles a VirtualMachinePublishRequest object.
type Reconciler struct {
	client.Client
	apiReader  client.Reader
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterfaceA2
	Metrics    *metrics.VMPublishMetrics
}

func requeueResult(ctx *context.VirtualMachinePublishRequestContextA2) ctrl.Result {
	vmPubReq := ctx.VMPublishRequest

	// no need to requeue to trigger another reconcile if:
	// - target item already exists
	// - failed to parse item ID from a successful task result
	if conditions.GetReason(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionTargetValid) == vmopv1.TargetItemAlreadyExistsReason {
		return ctrl.Result{}
	}

	if conditions.GetReason(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionUploaded) == vmopv1.UploadItemIDInvalidReason {
		return ctrl.Result{}
	}

	// In case the item is uploaded but VMI is not available, or,
	// the export task is not submitted to the vCenter task manager,
	// requeue after a short wait time since we expect these issues to be resolved quickly.
	if conditions.GetReason(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionUploaded) == vmopv1.UploadTaskNotStartedReason ||
		conditions.IsTrue(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionUploaded) {
		return ctrl.Result{RequeueAfter: 10 * time.Second}
	}

	// Skip checking ImageAvailable.
	// If ImageAvailable is true, Uploaded must be true. This also marks Condition Complete to true,
	// we will never reach this function.

	// For other cases, requeue after 60 seconds,
	// including VM Publish request is still in progress: queued/running
	return ctrl.Result{RequeueAfter: 60 * time.Second}
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get;list;watch
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries/status,verbs=get;

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	vmPublishReq := &vmopv1.VirtualMachinePublishRequest{}

	// Get the VirtualMachinePublishRequest directly from the API server - bypassing the cache of the
	// regular client - to avoid potentially stale objects from cache. We rely on the up-to-date Status
	// when sending publish VM requests during reconciliation.
	// Update() of stale object will be rejected by API server and result in unnecessary reconciles.
	err := r.apiReader.Get(ctx, req.NamespacedName, vmPublishReq)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmPublishCtx := &context.VirtualMachinePublishRequestContextA2{
		Context:          ctx,
		Logger:           ctrl.Log.WithName("VirtualMachinePublishRequest").WithValues("name", req.NamespacedName),
		VMPublishRequest: vmPublishReq,
	}

	patchHelper, err := patch.NewHelper(vmPublishReq, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", fmt.Sprintf("%s/%s", vmPublishReq.Namespace, vmPublishReq.Name))
	}

	// the patch is skipped when the VirtualMachinePublishRequest.Status().Update
	// is called during publishVirtualMachine().
	defer func() {
		if vmPublishCtx.SkipPatch {
			return
		}

		if err := patchHelper.Patch(ctx, vmPublishReq); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmPublishCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmPublishReq.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmPublishCtx)
	}

	return r.ReconcileNormal(vmPublishCtx)
}

func (r *Reconciler) updateSourceAndTargetRef(ctx *context.VirtualMachinePublishRequestContextA2) {
	vmPubReq := ctx.VMPublishRequest

	if vmPubReq.Status.SourceRef == nil {
		vmName := vmPubReq.Spec.Source.Name
		if vmName == "" {
			// set default source VM to this VirtualMachinePublishRequest's name.
			vmName = vmPubReq.Name
		}
		vmPubReq.Status.SourceRef = &vmopv1.VirtualMachinePublishRequestSource{
			Name: vmName,
		}
	}

	if vmPubReq.Status.TargetRef == nil {
		targetItemName := vmPubReq.Spec.Target.Item.Name
		if targetItemName == "" {
			targetItemName = fmt.Sprintf("%s-image", vmPubReq.Status.SourceRef.Name)
		}

		vmPubReq.Status.TargetRef = &vmopv1.VirtualMachinePublishRequestTarget{
			Item: vmopv1.VirtualMachinePublishRequestTargetItem{
				Name:        targetItemName,
				Description: vmPubReq.Spec.Target.Item.Description,
			},
			Location: vmPubReq.Spec.Target.Location,
		}
	}
}

// publishVirtualMachine checks if source VM and target is valid. Publish a VM if all requirements are met.
func (r *Reconciler) publishVirtualMachine(ctx *context.VirtualMachinePublishRequestContextA2) error {
	vmPublishReq := ctx.VMPublishRequest
	// Check if the source and target is valid
	if err := r.checkIsSourceValid(ctx); err != nil {
		ctx.Logger.Error(err, "failed to check if source is valid")
		return err
	}

	if err := r.checkIsTargetValid(ctx); err != nil {
		ctx.Logger.Error(err, "failed to check if target is valid")
		return err
	}

	// In case we mark Upload condition to true when checking target, return early.
	if conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionUploaded) {
		return nil
	}

	if conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionSourceValid) &&
		conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionTargetValid) {

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
		ctx.SkipPatch = true
		if err := r.Client.Status().Update(ctx, vmPublishReq); err != nil {
			ctx.Logger.Error(err, "update VirtualMachinePublishRequest status failed")
			return err
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
		return nil
	}

	return nil
}

func (r *Reconciler) removeVMPubResourceFromCluster(ctx *context.VirtualMachinePublishRequestContextA2) (requeueAfter time.Duration,
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
func (r *Reconciler) checkIsSourceValid(ctx *context.VirtualMachinePublishRequestContextA2) error {
	vmPubReq := ctx.VMPublishRequest
	vm := &vmopv1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmPubReq.Status.SourceRef.Name, Namespace: vmPubReq.Namespace}
	err := r.Get(ctx, objKey, vm)
	if err != nil {
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		if apiErrors.IsNotFound(err) {
			conditions.MarkFalse(vmPubReq,
				vmopv1.VirtualMachinePublishRequestConditionSourceValid,
				vmopv1.SourceVirtualMachineNotExistReason,
				err.Error())
		}
		return err
	}
	ctx.VM = vm

	if vm.Status.UniqueID == "" {
		err = errors.New("VM hasn't been created and has no uniqueID")
		conditions.MarkFalse(vmPubReq,
			vmopv1.VirtualMachinePublishRequestConditionSourceValid,
			vmopv1.SourceVirtualMachineNotCreatedReason,
			err.Error())
		return err
	}

	conditions.MarkTrue(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionSourceValid)
	return nil
}

// checkIsTargetValid checks if the target item is valid.
// It is invalid if the content library doesn't exist, an item with the same name in the CL exists.
func (r *Reconciler) checkIsTargetValid(ctx *context.VirtualMachinePublishRequestContextA2) error {
	vmPubReq := ctx.VMPublishRequest
	contentLibrary := &imgregv1a1.ContentLibrary{}
	targetLocationName := vmPubReq.Spec.Target.Location.Name
	targetItemName := vmPubReq.Status.TargetRef.Item.Name
	objKey := client.ObjectKey{Name: targetLocationName, Namespace: vmPubReq.Namespace}
	if err := r.Get(ctx, objKey, contentLibrary); err != nil {
		ctx.Logger.Error(err, "failed to get ContentLibrary", "cl", objKey)
		if apiErrors.IsNotFound(err) {
			conditions.MarkFalse(vmPubReq,
				vmopv1.VirtualMachinePublishRequestConditionTargetValid,
				vmopv1.TargetContentLibraryNotExistReason,
				err.Error())
		}
		return err
	}

	if !contentLibrary.Spec.Writable {
		err := fmt.Errorf("target location %s is not writable", contentLibrary.Status.Name)
		conditions.MarkFalse(vmPubReq,
			vmopv1.VirtualMachinePublishRequestConditionTargetValid,
			vmopv1.TargetContentLibraryNotWritableReason,
			err.Error())
		return err
	}

	// Check if the content library is ready.
	isReady := false
	for _, condition := range contentLibrary.Status.Conditions {
		if condition.Type == imgregv1a1.ReadyCondition {
			isReady = condition.Status == corev1.ConditionTrue
			break
		}
	}

	if !isReady {
		err := fmt.Errorf("target location %s is not ready", contentLibrary.Status.Name)
		conditions.MarkFalse(vmPubReq,
			vmopv1.VirtualMachinePublishRequestConditionTargetValid,
			vmopv1.TargetContentLibraryNotReadyReason,
			err.Error())
		return err
	}

	ctx.ContentLibrary = contentLibrary
	item, err := r.VMProvider.GetItemFromLibraryByName(ctx, string(contentLibrary.Spec.UUID), targetItemName)
	if err != nil {
		ctx.Logger.Error(err, "failed to find item", "cl", objKey, "item name", targetItemName)
		return err
	}

	if item != nil {
		ctx.Logger.Info("target item already exists in the content library",
			"cl", objKey, "item name", targetItemName)
		// If item already exists in the content library and attempt is not zero,
		// check if it is created from this VirtualMachinePublishRequest by getting its description.
		// If VC forgets the task, or the task hadn't proceeded far enough to be submitted to VC
		// within 30 seconds window, this check can help us find if there is a prior task succeeded.
		// Ideally we are unlikely to see this.
		if vmPubReq.Status.Attempts > 0 {
			if r.isItemCorrelatedWithVMPub(ctx, item) {
				ctx.Logger.Info("existing target item is published by this VMPubReq")
				conditions.MarkTrue(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionTargetValid)
				conditions.MarkTrue(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionUploaded)
				ctx.ItemID = item.ID
				return nil
			}
		}

		// If duplicate item name exists, give up at this point.
		// no need to requeue to cause another reconcile. return nil.
		conditions.MarkFalse(vmPubReq,
			vmopv1.VirtualMachinePublishRequestConditionTargetValid,
			vmopv1.TargetItemAlreadyExistsReason,
			fmt.Sprintf("item with name %s already exists in the content library %s", targetItemName,
				contentLibrary.Status.Name))
		return nil
	}

	conditions.MarkTrue(vmPubReq, vmopv1.VirtualMachinePublishRequestConditionTargetValid)
	return nil
}

// checkIsImageAvailable checks if the published VirtualMachineImage resource is available in the cluster.
func (r *Reconciler) checkIsImageAvailable(ctx *context.VirtualMachinePublishRequestContextA2) error {
	if !conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionUploaded) {
		return nil
	}

	if conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionImageAvailable) {
		return nil
	}

	if ctx.ItemID == "" {
		id, err := r.getUploadedItemID(ctx)
		if err != nil {
			ctx.Logger.Error(err, "failed to get uploaded item UUID")
			return err
		}
		ctx.ItemID = id
	}

	vmiList := &vmopv1.VirtualMachineImageList{}
	if err := r.List(ctx, vmiList, client.InNamespace(ctx.VMPublishRequest.Namespace)); err != nil {
		ctx.Logger.Error(err, "failed to list VirtualMachineImage")
		return err
	}

	found := false
	for _, vmi := range vmiList.Items {
		if vmi.Status.ProviderItemID == ctx.ItemID {
			found = true
			ctx.VMPublishRequest.Status.ImageName = vmi.Name
			conditions.MarkTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionImageAvailable)
			ctx.Logger.Info("VirtualMachineImage is available", "vmiName", vmi.Name)
			break
		}
	}

	if !found {
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionImageAvailable,
			vmopv1.TargetVirtualMachineImageNotFoundReason,
			"VirtualMachineImage not found")
	}

	return nil
}

// checkIsComplete checks if condition Complete can be marked to true.
// The condition's status is set to true only when all other conditions present on the resource have a truthy status.
func (r *Reconciler) checkIsComplete(ctx *context.VirtualMachinePublishRequestContextA2) bool {
	if conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionComplete) {
		return true
	}

	if !conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionUploaded) {
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionComplete,
			vmopv1.HasNotBeenUploadedReason,
			"item hasn't been uploaded yet")
		return false
	}

	if !conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionImageAvailable) {
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionComplete,
			vmopv1.ImageUnavailableReason,
			"VirtualMachineImage is not available")
		return false
	}

	conditions.MarkTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionComplete)
	ctx.VMPublishRequest.Status.Ready = true
	ctx.VMPublishRequest.Status.CompletionTime = metav1.Now()
	ctx.Logger.Info("VM publish request completed", "time", ctx.VMPublishRequest.Status.CompletionTime)

	return true
}

// getPublishRequestTask gets task with description id com.vmware.ovfs.LibraryItem.capture and specified actid in taskManager.
func (r *Reconciler) getPublishRequestTask(ctx *context.VirtualMachinePublishRequestContextA2) (*vimtypes.TaskInfo, error) {
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

// checkPubReqStatusAndShouldRepublish checks the publish request task status, mark Uploaded condition
// and check if we should re-publish this VM.
// Returns
// - if a following publish task is needed (true if a publish request should be sent, otherwise return false.),
// - error
// It filters VM Publish task from VC task manager by activation ID and checks its status.
// - If this task is queued/running, then we should mark Uploaded to false and no need to retry VM publish.
// - task succeeded, mark Uploaded to true and return the uploaded item ID.
// - task failed, mark Uploaded to false and retry the operation.
func (r *Reconciler) checkPubReqStatusAndShouldRepublish(ctx *context.VirtualMachinePublishRequestContextA2) (bool, error) {
	if ctx.VMPublishRequest.Status.Attempts == 0 {
		// No VM publish task has been attempted. return immediately.
		return true, nil
	}

	if conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionUploaded) {
		return false, nil
	}

	actID := getPublishRequestActID(ctx.VMPublishRequest)
	logger := ctx.Logger.WithValues("actID", actID, "descriptionID", TaskDescriptionID)

	task, err := r.getPublishRequestTask(ctx)
	if err != nil {
		// Failed to get task from taskManager, return early and retry in next reconcile loop.
		return false, err
	}

	// If we can not find a relevant task, probably due to:
	// - Reason 1: VM operator fails to send the request this request is not made to VC,
	// e.g. VC credentials rotate. Error returned.
	// - Reason 2: VM operator is able to create the client and send the request, VC receives this request,
	// but fails to submit and register this task in the VC task manager. Error returned.
	// - Reason 3: content library service hasn't processed far enough to submit and register this task,
	// but will eventually succeed. (Most common)
	// - Reason 4: task is removed from VC. The default task retention in vpxd is set to 30 days, we can ignore such case.
	//
	// Ideally we should immediately return error for Reason 1 & 2. But we can't tell why we can't find that task for
	// sure since we don't track the CreateOVF error. Meanwhile, reason 1 & 2 are rare and won't be a big problem
	// if not schedule reconcile requests immediately.
	//
	// Wait for the configured timeout before retrying VM publish.
	// This is done so that the controller doesn't schedule reconcile requests immediately in case 3,
	// and we are not submitting requests over and over when this publish task is actually running.
	if task == nil {
		if time.Since(ctx.VMPublishRequest.Status.LastAttemptTime.Time) > waitForTaskTimeout {
			// CreateOvf API failed to submit this task for some reason. In this case, retry VM publish.
			ctx.Logger.Info("failed to create task, retry publishing this VM",
				"taskName", TaskDescriptionID,
				"lastAttemptTime", ctx.VMPublishRequest.Status.LastAttemptTime.String())
			return true, nil
		}

		// CreateOvf request has been sent but the task com.vmware.ovfs.LibraryItem.capture hasn't been
		// submitted to the task manager yet. retry taskManager query at later reconcile.
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionUploaded,
			vmopv1.UploadTaskNotStartedReason,
			"VM Publish task hasn't started.")
		return false, nil
	}

	switch task.State {
	case vimtypes.TaskInfoStateQueued:
		logger.V(5).Info("VM Publish task is queued but hasn't started")
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionUploaded,
			vmopv1.UploadTaskQueuedReason,
			"VM Publish task is queued.")
		return false, nil
	case vimtypes.TaskInfoStateRunning:
		// CreateOVF is still in progress
		// TODO: Add task Progress info.
		logger.V(5).Info("VM Publish is still in progress", "progress", task.Progress)
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionUploaded,
			vmopv1.UploadingReason,
			"Uploading item to content library.")
		return false, nil
	case vimtypes.TaskInfoStateSuccess:
		// Publish request succeeds. Update Uploaded condition.
		logger.Info("VM Publish succeeded", "result", task.Result)
		r.processUploadedItem(ctx, task)
		return false, nil
	case vimtypes.TaskInfoStateError:
		errMsg := "failed to publish source VM"
		if task.Error != nil {
			errMsg = task.Error.LocalizedMessage
		}

		logger.Error(err, "VM Publish failed, will retry this operation",
			"actID", task.ActivationId, "descriptionID", task.DescriptionId)
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionUploaded,
			vmopv1.UploadFailureReason,
			errMsg)
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) processUploadedItem(ctx *context.VirtualMachinePublishRequestContextA2, task *vimtypes.TaskInfo) {
	itemID, err := parseItemIDFromTaskResult(task.Result)
	if err != nil {
		// Don't return err here because the task result won't be updated, the error will persist.
		// Ideally, this should never happen.
		ctx.Logger.Error(err, "failed to get uploaded item UUID")
		conditions.MarkFalse(ctx.VMPublishRequest,
			vmopv1.VirtualMachinePublishRequestConditionUploaded,
			vmopv1.UploadItemIDInvalidReason,
			ItemParseErrorMessage)
		r.Recorder.Warn(ctx.VMPublishRequest, "PublishFailure", ItemParseErrorMessage)
		return
	}

	ctx.ItemID = itemID
	conditions.MarkTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionUploaded)
}

// getUploadedItemID returns the uploaded content library item ID.
func (r *Reconciler) getUploadedItemID(ctx *context.VirtualMachinePublishRequestContextA2) (string, error) {
	task, err := r.getPublishRequestTask(ctx)
	if err != nil {
		return "", err
	}

	if task == nil || task.State != vimtypes.TaskInfoStateSuccess {
		// It's rare but still likely that the latest publish task failed due to duplication
		// when any previous task succeeded. (If 30 seconds waitForTaskTimeout is not enough in edge cases).
		// In this case, we need to check item descriptions to match this vmPub and get item ID.
		return r.findCorrelatedItemIDByName(ctx)
	}

	return parseItemIDFromTaskResult(task.Result)
}

// isItemCorrelatedWithVMPub checks if the item is correlated with this VM Publish request
// by checking the item description. We add the vmPub uuid to the target item description when publishing.
//
// Currently, there is no easy way to link published item to the VM publish request
// other than activation ID.
// However, if we lose track of the activation ID (a previous task hasn't started within 30 seconds
// waiting time window but a new task with a new actID is already triggered),
// we'll get stuck if any previous task succeeds because all tasks afterwards
// would fail due to item duplication error.
func (r *Reconciler) isItemCorrelatedWithVMPub(ctx *context.VirtualMachinePublishRequestContextA2,
	item *library.Item) bool {
	if item.Description != nil {
		descriptions := itemDescriptionReg.FindStringSubmatch(*item.Description)
		if len(descriptions) > 1 && descriptions[1] == string(ctx.VMPublishRequest.UID) {
			return true
		}
	}

	return false
}

// findCorrelatedItemIDByName finds the published item ID in the VC by target item name.
func (r *Reconciler) findCorrelatedItemIDByName(ctx *context.VirtualMachinePublishRequestContextA2) (string, error) {
	targetItemName := ctx.VMPublishRequest.Status.TargetRef.Item.Name
	// We only get ContentLibrary when checking TargetValid condition,
	// so this ctx.ContentLibrary can be nil in other cases.
	// Usually we won't fall into this corner cases, don't put this ContentLibrary Get in Reconcile
	// to avoid unnecessary reads.
	if ctx.ContentLibrary == nil {
		contentLibrary := &imgregv1a1.ContentLibrary{}
		targetLocationName := ctx.VMPublishRequest.Spec.Target.Location.Name
		objKey := client.ObjectKey{Name: targetLocationName, Namespace: ctx.VMPublishRequest.Namespace}
		if err := r.Get(ctx, objKey, contentLibrary); err != nil {
			ctx.Logger.Error(err, "failed to get ContentLibrary", "cl", objKey)
			return "", err
		}
		ctx.ContentLibrary = contentLibrary
	}

	item, err := r.VMProvider.GetItemFromLibraryByName(ctx, string(ctx.ContentLibrary.Spec.UUID), targetItemName)
	if err != nil {
		ctx.Logger.Error(err, "failed to find item from VC by its name", "item name", targetItemName)
		return "", err
	}

	if item != nil {
		// Item already exists in the content library, check if it is created from
		// this VirtualMachinePublishRequest from its description.
		// If VC forgets the task, or the task hadn't proceeded far enough to be submitted to VC
		// within 30 seconds window, this check can help us find if there is a prior task succeeded.
		// Ideally we are unlikely to see this.
		if r.isItemCorrelatedWithVMPub(ctx, item) {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("no item with name %s exists in the VC", targetItemName)
}

// updatePublishedItemDescription updates item description, which removes vmPub UUID from it.
func (r *Reconciler) updatePublishedItemDescription(ctx *context.VirtualMachinePublishRequestContextA2) error {
	if !conditions.IsTrue(ctx.VMPublishRequest, vmopv1.VirtualMachinePublishRequestConditionImageAvailable) {
		return nil
	}

	if ctx.ItemID == "" {
		id, err := r.getUploadedItemID(ctx)
		if err != nil {
			ctx.Logger.Error(err, "failed to get uploaded item UUID")
			return err
		}
		ctx.ItemID = id
	}

	if err := r.VMProvider.UpdateContentLibraryItem(ctx, ctx.ItemID,
		ctx.VMPublishRequest.Status.TargetRef.Item.Name,
		&ctx.VMPublishRequest.Spec.Target.Item.Description); err != nil {
		ctx.Logger.Error(err, "failed to update item description", "itemID", ctx.ItemID,
			"itemName", ctx.VMPublishRequest.Status.TargetRef.Item.Name, "itemDescription",
			ctx.VMPublishRequest.Spec.Target.Item.Description)
		return err
	}
	return nil
}

// getPublishRequestActID returns the activation ID we pass down to the content library vAPI.
// Append .status.attempts to the vmpublishrequest uuid to generate an activation ID.
// VC doesn't prevent duplicate activation IDs for different requests, but we can have problems
// when doing the query, i.e. not returned all the tasks.
// So we need a unique string, which is also under track to be the actID for a later query.
func getPublishRequestActID(vmPub *vmopv1.VirtualMachinePublishRequest) string {
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

	if !strings.HasPrefix(clItem.Value, clItemPrefix) {
		return "", fmt.Errorf("content library item UUID is invalid, value: %s", clItem.Value)
	}
	return clItem.Value[len(clItemPrefix):], nil
}

func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachinePublishRequestContextA2) (_ ctrl.Result, reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachinePublishRequest")
	vmPublishReq := ctx.VMPublishRequest

	if !controllerutil.ContainsFinalizer(vmPublishReq, finalizerName) {
		// The finalizer must be present before proceeding in order to ensure that the VirtualMachinePublishRequest will
		// be cleaned up. Return immediately after here to let the patcher helper update the
		// object, and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(vmPublishReq, finalizerName)
		return
	}

	// Register VM publish request metrics based on the reconcile result.
	var isComplete, isDeleted bool
	defer func() {
		if isDeleted {
			// If the vmPub is deleted, return immediately.
			// We don't need to call DeleteMetrics here, we will run this in ReconcileDelete().
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
	isComplete = conditions.IsTrue(vmPublishReq, vmopv1.VirtualMachinePublishRequestConditionComplete)
	if isComplete {
		requeueAfter, deleted, err := r.removeVMPubResourceFromCluster(ctx)
		isDeleted = deleted
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	if vmPublishReq.Status.StartTime.IsZero() {
		vmPublishReq.Status.StartTime = metav1.Now()
	}

	r.updateSourceAndTargetRef(ctx)

	shouldPublish, err := r.checkPubReqStatusAndShouldRepublish(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	if shouldPublish {
		err := r.publishVirtualMachine(ctx)
		if err != nil {
			ctx.Logger.Error(err, "failed to publish VirtualMachine")
			return ctrl.Result{}, errors.Wrapf(err, "failed to publish VirtualMachine")
		}
		return requeueResult(ctx), nil
	}

	if err := r.checkIsImageAvailable(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// Update published item description to remove vmPub UUID from it.
	// Update the description after all conditions except Complete is set,
	// otherwise we may lose track of the item if this update fails.
	if err := r.updatePublishedItemDescription(ctx); err != nil {
		r.Recorder.EmitEvent(vmPublishReq, "Publish", err, true)
		return ctrl.Result{}, err
	}

	if isComplete = r.checkIsComplete(ctx); isComplete {
		// remove VirtualMachinePublishRequest from the cluster if ttlSecondsAfterFinished is set.
		requeueAfter, deleted, err := r.removeVMPubResourceFromCluster(ctx)
		isDeleted = deleted
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	return requeueResult(ctx), nil
}

func (r *Reconciler) ReconcileDelete(ctx *context.VirtualMachinePublishRequestContextA2) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(ctx.VMPublishRequest, finalizerName) {
		r.Metrics.DeleteMetrics(ctx.Logger, ctx.VMPublishRequest.Name, ctx.VMPublishRequest.Namespace)
		controllerutil.RemoveFinalizer(ctx.VMPublishRequest, finalizerName)
	}

	return ctrl.Result{}, nil
}
