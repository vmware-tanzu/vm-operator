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

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmpublish"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.VirtualMachinePublishRequest{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	vmpubCatalog := vmpublish.NewCatalog()
	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
		vmpubCatalog,
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
	vmProvider vmprovider.VirtualMachineProviderInterface,
	vmCatalog vmpublish.Catalog) *Reconciler {

	return &Reconciler{
		Client:       client,
		Logger:       logger,
		Recorder:     recorder,
		VMProvider:   vmProvider,
		VMPubCatalog: vmCatalog,
	}
}

// Reconciler reconciles a VirtualMachinePublishRequest object.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface

	VMPubCatalog vmpublish.Catalog
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

	patchHelper, err := patch.NewHelper(vmPublishReq, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", req.NamespacedName)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, vmPublishReq); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmPublishCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmPublishReq.DeletionTimestamp.IsZero() {
		err = r.ReconcileDelete(vmPublishCtx)
		return ctrl.Result{}, err
	}

	if err := r.ReconcileNormal(vmPublishCtx); err != nil {
		vmPublishCtx.Logger.Error(err, "failed to reconcile VirtualMachinePublishRequest")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueDelay(vmPublishCtx)}, nil
}

// checkIsSourceValid function checks if the source VM is valid. It is invalid if the VM k8s resource
// doesn't exist, or is not in Created phase.
func (r *Reconciler) checkIsSourceValid(ctx *context.VirtualMachinePublishRequestContext) error {
	vmPubReq := ctx.VMPublishRequest
	vm := &vmopv1alpha1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmPubReq.Spec.Source.Name, Namespace: vmPubReq.Namespace}
	err := r.Get(ctx, objKey, vm)
	if err != nil {
		r.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
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
	contentLibrary := &imgregv1a1.ContentLibrary{}
	targetName := vmPubReq.Spec.Target.Location.Name
	objKey := client.ObjectKey{Name: targetName, Namespace: vmPubReq.Namespace}
	if err := r.Get(ctx, objKey, contentLibrary); err != nil {
		r.Logger.Error(err, "failed to get ContentLibrary", "cl", objKey)
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

// checkPublishRequestProcessStatus gets VM Publish task from task manager and checks its status.
// - If this task is queued/running, then we should return early.
// - task succeeded, update related conditions. (Uploaded/ImageAvailable)
// - task failed, clear previous status and retry the operation.
func (r *Reconciler) checkPublishRequestProcessStatus(ctx *context.VirtualMachinePublishRequestContext) bool {
	// TODO: 	// With actid, we can query Task manager to track the task status.
	// Implement this when content library service team fixes the bug.
	exist, res := r.VMPubCatalog.GetPubRequestState(string(ctx.VMPublishRequest.UID))
	if !exist {
		return false
	}

	switch res.State {
	case vmpublish.StateRunning:
		// CreateOVF is still in progress
		r.Logger.V(5).Info("VM Publish is still in progress")
		return true
	case vmpublish.StateSuccess:
		// Publish request succeeds. requeue
		// TODO: . Update conditions.
		// TODO: . Watch ContentLibraryItem resources and create vm image resources.
		r.Logger.V(5).Info("VM Publish succeeded")
		return true
	case vmpublish.StateError:
		r.Logger.V(5).Info("VM Publish failed. Retry this operation")
		// clear from map, and retry.
		r.VMPubCatalog.RemoveFromPubRequestResult(string(ctx.VMPublishRequest.UID))
	}
	return false
}

func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachinePublishRequestContext) (reterr error) {
	r.Logger.Info("Reconciling VirtualMachinePublishRequest")
	vmPublishReq := ctx.VMPublishRequest
	if vmPublishReq.IsComplete() {
		// TODO: . fulfill the logic when VM publish request is completed.
		r.Logger.Info("VM Publish completed", "time", vmPublishReq.Status.CompletionTime)
		return nil
	}

	if vmPublishReq.Status.StartTime.IsZero() {
		vmPublishReq.Status.StartTime = metav1.Now()
	}

	if r.checkPublishRequestProcessStatus(ctx) {
		return nil
	}

	// Check if the source and target is valid
	if err := r.checkIsSourceValid(ctx); err != nil {
		return err
	}

	if err := r.checkIsTargetValid(ctx); err != nil {
		return err
	}

	if vmPublishReq.IsSourceValid() && vmPublishReq.IsTargetValid() {
		// We need to check status before we actually update the request status. If the status is already running/success/error,
		// we should return early. Since this is only a tmp workaround. Leave it as it is and will be fixed in .
		// TODO: .
		r.VMPubCatalog.UpdateVMPubRequestResult(string(vmPublishReq.UID), vmpublish.StateRunning, "")
		go func() {
			itemID, pubErr := r.VMProvider.PublishVirtualMachine(ctx, ctx.VM, vmPublishReq, ctx.ContentLibrary)
			if pubErr != nil {
				r.Logger.Error(pubErr, "failed to publish VM")
				r.VMPubCatalog.UpdateVMPubRequestResult(string(vmPublishReq.UID), vmpublish.StateError, "")
			} else {
				r.VMPubCatalog.UpdateVMPubRequestResult(string(vmPublishReq.UID), vmpublish.StateSuccess, itemID)
			}
		}()
	}

	return nil
}

func (r *Reconciler) ReconcileDelete(ctx *context.VirtualMachinePublishRequestContext) error {
	// no op
	return nil
}
