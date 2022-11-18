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

	// TODO: . Watch VirtualMachineImage resources.
	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(r)
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
	}
}

// Reconciler reconciles a VirtualMachinePublishRequest object.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
}

func requeueDelay(_ *context.VirtualMachinePublishRequestContext) time.Duration {
	// TODO: update requeue delay based on publish request conditions
	return time.Minute
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
			targetItemName = fmt.Sprintf("%s-image", vmPubReq.Name)
		}

		vmPubReq.Status.TargetRef = &vmopv1alpha1.VirtualMachinePublishRequestTarget{
			Item: vmopv1alpha1.VirtualMachinePublishRequestTargetItem{
				Name: targetItemName,
			},
			Location: vmPubReq.Spec.Target.Location,
		}
	}
}

// checkIsSourceValid function checks if the source VM is valid.
// It is invalid if the VM k8s resource doesn't exist, or is not in Created phase.
func (r *Reconciler) checkIsSourceValid(ctx *context.VirtualMachinePublishRequestContext) error {
	vmPubReq := ctx.VMPublishRequest
	vm := &vmopv1alpha1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmPubReq.Status.SourceRef.Name, Namespace: vmPubReq.Namespace}
	err := r.Get(ctx, objKey, vm)
	if err != nil {
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		if apiErrors.IsNotFound(err) {
			vmPubReq.MarkSourceValid(corev1.ConditionFalse, "SourceVmNotExist", err.Error())
			err = nil
		}
		return err
	}
	ctx.VM = vm

	// TODO: . Add more check.
	// VM is in created phase and .Status.UniqueID is not empty.
	vmPubReq.MarkSourceValid(corev1.ConditionTrue)
	return nil
}

// checkIsTargetValid checks if the target item is valid.
// It is invalid if the content library doesn't exist, an item with the same name in the CL exists,
// or another vmpub request with the same name is in progress.
func (r *Reconciler) checkIsTargetValid(ctx *context.VirtualMachinePublishRequestContext) error {
	vmPubReq := ctx.VMPublishRequest
	targetLocationName := vmPubReq.Spec.Target.Location.Name
	contentLibrary := &imgregv1a1.ContentLibrary{}
	objKey := client.ObjectKey{Name: targetLocationName, Namespace: vmPubReq.Namespace}
	if err := r.Get(ctx, objKey, contentLibrary); err != nil {
		ctx.Logger.Error(err, "failed to get ContentLibrary", "cl", objKey)
		if apiErrors.IsNotFound(err) {
			vmPubReq.MarkTargetValid(corev1.ConditionFalse, "TargetContentLibraryNotExist", err.Error())
			err = nil
		}
		return err
	}
	ctx.ContentLibrary = contentLibrary

	// TODO: . Add more check.
	vmPubReq.MarkTargetValid(corev1.ConditionTrue)
	return nil
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

// checkPublishRequestStatus gets VM Publish task from task manager and checks its status.
// - If this task is queued/running, then we should return early.
// - task succeeded, update related conditions. (Uploaded/ImageAvailable)
// - task failed, clear previous status and retry the operation.
// Return true if a publish request should be sent, otherwise return false.
func (r *Reconciler) checkPublishRequestStatus(ctx *context.VirtualMachinePublishRequestContext) (bool, error) {
	if ctx.VMPublishRequest.Status.Attempts == 0 {
		// no VM publish task has been attempted. return immediately.
		return true, nil
	}

	actID := getPublishRequestActID(ctx.VMPublishRequest)
	logger := ctx.Logger.WithValues("actID", actID, "descriptionID", TaskDescriptionID)

	task, err := r.getPublishRequestTask(ctx)
	if err != nil {
		// Failed to get task from taskManager, return early and retry in next reconcile loop.
		return false, err
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
			// CreateOvf API failed to submit this task for some reason. In this case, retry VM publish and return error.
			return true, fmt.Errorf("failed to create task: %s, last attempt time: %s",
				TaskDescriptionID, ctx.VMPublishRequest.Status.LastAttemptTime.String())
		}

		// CreateOvf request has been sent but the task com.vmware.ovfs.LibraryItem.capture hasn't been
		// submitted to the task manager yet. retry taskManager query at later reconcile.
		return false, nil
	}

	switch task.State {
	case vimtypes.TaskInfoStateQueued:
		logger.V(5).Info("VM Publish task is queued but hasn't started")
		return false, nil
	case vimtypes.TaskInfoStateRunning:
		// CreateOVF is still in progress
		logger.V(5).Info("VM Publish is still in progress", "progress", task.Progress)
		return false, nil
	case vimtypes.TaskInfoStateSuccess:
		// Publish request succeeds. requeue
		// TODO: . Update conditions.
		// TODO: . Watch ContentLibraryItem resources and create vm image resources.
		logger.Info("VM Publish succeeded", "result", task.Result)
		return false, nil
	case vimtypes.TaskInfoStateError:
		err = fmt.Errorf("VM publish failed, err: %v", task.Error)
		logger.Error(err, "VM Publish failed, will retry this operation")
		return true, err
	}
	return true, nil
}

func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachinePublishRequestContext) (_ ctrl.Result, reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachinePublishRequest")
	vmPublishReq := ctx.VMPublishRequest

	skipPatch := false
	patchHelper, err := patch.NewHelper(vmPublishReq, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", fmt.Sprintf("%s/%s", vmPublishReq.Namespace, vmPublishReq.Name))
	}

	defer func() {
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

	if vmPublishReq.IsComplete() {
		// TODO: . fulfill the logic when VM publish request is completed.
		ctx.Logger.Info("VM Publish completed", "time", vmPublishReq.Status.CompletionTime)
		return ctrl.Result{}, nil
	}

	if vmPublishReq.Status.StartTime.IsZero() {
		vmPublishReq.Status.StartTime = metav1.Now()
	}

	r.updateSourceAndTargetRef(ctx)

	shouldPublish, err := r.checkPublishRequestStatus(ctx)
	reterr = err
	if !shouldPublish {
		return ctrl.Result{}, reterr
	}

	// Check if the source and target is valid
	if err := r.checkIsSourceValid(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.checkIsTargetValid(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if vmPublishReq.IsSourceValid() && vmPublishReq.IsTargetValid() {
		vmPublishReq.Status.Attempts++
		vmPublishReq.Status.LastAttemptTime = metav1.Now()
		skipPatch = true

		// Update VirtualMachinePublishRequest object to avoid conflict.
		// API server will reject the Update request if updating to a stale object, so that we can always
		// set the correct number of attempts in the status to avoid running into a situation where we end
		// up sending multiple publish requests with the same actID.
		//
		// Patch a stale object won't fail even when the resourceVersion doesn't match. In this case, we
		// may set .status.attempts to a same number multiple times using Patch.
		// Use Update instead and skip Patch in the defer function.
		if err = r.Client.Status().Update(ctx, vmPublishReq); err != nil {
			ctx.Logger.Error(err, "update failed")
			return ctrl.Result{}, err
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
	}

	return ctrl.Result{RequeueAfter: requeueDelay(ctx)}, reterr
}

func (r *Reconciler) ReconcileDelete(_ *context.VirtualMachinePublishRequestContext) (ctrl.Result, error) {
	// no op
	return ctrl.Result{}, nil
}

// getPublishRequestActID returns the activation ID we pass down to the content library vAPI.
// Append .status.attempts to the vmpublishrequest uuid to generate an activation ID.
// VC doesn't prevent duplicate activation IDs for different requests, but we can have problems
// when doing the query, i.e. not returned all the tasks.
// So we need a unique string, which is also under track to be the actID for a later query.
func getPublishRequestActID(vmPub *vmopv1alpha1.VirtualMachinePublishRequest) string {
	return fmt.Sprintf("%s-%d", string(vmPub.UID), vmPub.Status.Attempts)
}
