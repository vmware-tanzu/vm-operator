// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegrouppublishrequest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	finalizerName         = "vmoperator.vmware.com/virtualmachinegrouppublishrequest"
	undefinedSpecErrorMsg = "spec.%s is undefined"
	invalidSpecErrorMsg   = "webhooks failed to mutate/validate spec. please delete and create again."
	delTTLMsg             = "%s vm group publish request due to TTL expired"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineGroupPublishRequest{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)
	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		mgr.GetAPIReader(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)
	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Owns(&vmopv1.VirtualMachinePublishRequest{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
			LogConstructor: pkgutil.ControllerLogConstructor(
				controllerNameShort,
				controlledType,
				mgr.GetScheme()),
		}).
		Complete(r)
}

// Reconciler reconciles a VirtualMachineGroupPublishRequest object.
type Reconciler struct {
	client.Client
	Context    context.Context
	apiReader  client.Reader
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	apiReader client.Reader,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		apiReader:  apiReader,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list

// Reconcile reconciles a VirtualMachineGroupPublishRequest object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vmGroupPublishRequest := &vmopv1.VirtualMachineGroupPublishRequest{}
	if err := r.Get(ctx, req.NamespacedName, vmGroupPublishRequest); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmGroupPublishRequestCtx := &pkgctx.VirtualMachineGroupPublishRequestContext{
		Context:               ctx,
		Logger:                pkgutil.FromContextOrDefault(ctx),
		VMGroupPublishRequest: vmGroupPublishRequest,
	}
	patchHelper, err := patch.NewHelper(vmGroupPublishRequest, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for %s/%s: %w",
			vmGroupPublishRequest.Namespace,
			vmGroupPublishRequest.Name,
			err)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, vmGroupPublishRequest); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmGroupPublishRequestCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmGroupPublishRequest.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmGroupPublishRequestCtx)
	}

	return pkgerr.ResultFromError(r.ReconcileNormal(vmGroupPublishRequestCtx))
}

func (r *Reconciler) ReconcileDelete(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext) (ctrl.Result, error) {
	ctx.Logger.Info("Reconciling VirtualMachineGroupPublishRequest Deletion")

	if controllerutil.ContainsFinalizer(ctx.VMGroupPublishRequest, finalizerName) {
		ctx.Logger.Info("Removing owned VirtualMachinePublishRequests")
		if err := r.deleteVMPublishRequests(ctx); err != nil {
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(ctx.VMGroupPublishRequest, finalizerName)
		ctx.Logger.Info("Removed finalizer for VirtualMachineGroupPublishRequest")
	}
	ctx.Logger.V(4).Info("Finished Reconciling VirtualMachineGroupPublishRequest Deletion")
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) error {
	ctx.Logger.Info("Reconciling VirtualMachineGroupPublishRequest")
	vmGroupPublishReq := ctx.VMGroupPublishRequest

	if controllerutil.AddFinalizer(vmGroupPublishReq, finalizerName) {
		ctx.Logger.Info("Added finalizer for VirtualMachineGroupPublishRequest")
		return nil
	}

	if conditions.IsTrue(vmGroupPublishReq, vmopv1.VirtualMachineGroupPublishRequestConditionComplete) {
		return r.reconcileSpecTTL(ctx)
	}

	if err := r.verifySpec(vmGroupPublishReq); err != nil {
		return err
	}

	return r.reconcileStatus(ctx)
}

func (r *Reconciler) reconcileStatus(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) error {
	if ctx.VMGroupPublishRequest.Status.StartTime.IsZero() {
		ctx.VMGroupPublishRequest.Status.StartTime = metav1.NewTime(time.Now())
	}

	existReqsMap, pendingReqsSet, completedReqsSet, err := r.getVMPublishRequests(ctx)
	if err != nil {
		return err
	}

	r.reconcileStatusImages(ctx, existReqsMap)

	desiredReqsSet := sets.New(ctx.VMGroupPublishRequest.Spec.VirtualMachines...)
	if err := r.reconcileNewPublishRequests(ctx, sets.KeySet(existReqsMap), desiredReqsSet); err != nil {
		return err
	}
	return r.reconcileStatusCompletedCondition(ctx, pendingReqsSet, completedReqsSet, desiredReqsSet)
}

// getVMPublishRequests returns a map of VMGroupPublishRequests own by the VirtualMachineGroupPublishRequest
// along with a set of pending VMGroupPublishRequest and a set of completed VMGroupPublishRequest.
// The key for those objects is the source VirtualMachine name.
func (r *Reconciler) getVMPublishRequests(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext) (
	map[string]vmopv1.VirtualMachinePublishRequest,
	sets.Set[string],
	sets.Set[string],
	error) {

	reqs := &vmopv1.VirtualMachinePublishRequestList{}
	// retrieve from API server directly to avoid duplicate resource creation in case that the newly
	// created vm publish request has not made it back into the client's cache
	if err := r.apiReader.List(ctx,
		reqs,
		client.InNamespace(ctx.VMGroupPublishRequest.Namespace),
		client.MatchingLabels{
			vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ctx.VMGroupPublishRequest.Name,
		}); err != nil {
		ctx.Logger.Error(err, "failed to list VirtualMachinePublishRequest")
		return nil, nil, nil, err
	}

	existReqsMap := make(map[string]vmopv1.VirtualMachinePublishRequest)
	pendingReqsSet := sets.Set[string]{}
	completedReqsSet := sets.Set[string]{}
	for _, req := range reqs.Items {
		if metav1.IsControlledBy(&req, ctx.VMGroupPublishRequest) {
			existReqsMap[req.Spec.Source.Name] = req
			if req.Status.Ready {
				completedReqsSet.Insert(req.Spec.Source.Name)
			} else {
				pendingReqsSet.Insert(req.Spec.Source.Name)
			}
		}
	}

	return existReqsMap, pendingReqsSet, completedReqsSet, nil
}

// reconcileStatusImages updates status's image status by taking a map of VMGroupPublishRequests
// own by the VirtualMachineGroupPublishRequest and converting them into a sorted list of
// VirtualMachineGroupPublishRequestImageStatus. Sorting prevents reconciling when list is unchanged.
func (r *Reconciler) reconcileStatusImages(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext,
	existReqsMap map[string]vmopv1.VirtualMachinePublishRequest) {

	imageStatuses := make([]vmopv1.VirtualMachineGroupPublishRequestImageStatus, 0, len(existReqsMap))
	for _, req := range existReqsMap {
		imageStatuses = append(imageStatuses, vmopv1.VirtualMachineGroupPublishRequestImageStatus{
			Source:             req.Spec.Source.Name,
			PublishRequestName: req.Name,
			ImageName:          req.Status.ImageName,
			Conditions:         req.Status.Conditions,
		})
	}

	ctx.VMGroupPublishRequest.Status.Images = slices.SortedFunc(slices.Values(imageStatuses),
		func(a, b vmopv1.VirtualMachineGroupPublishRequestImageStatus) int {
			return strings.Compare(a.Source, b.Source)
		})
}

// reconcileNewPublishRequests compares a set of exist requests and a set of expected requests and
// creates new requests if they are missing in the exist requests set.
// It returns nil ONLY when no new publish requests were created, in other words, the set of exist requests
// and the set of expected requests are the same.
func (r *Reconciler) reconcileNewPublishRequests(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext,
	existReqsSet, desiredReqsSet sets.Set[string]) error {

	newReqsSet := desiredReqsSet.Difference(existReqsSet)
	if newReqsSet.Len() == 0 {
		return nil
	}

	vmGroupPubReqGvk, err := apiutil.GVKForObject(ctx.VMGroupPublishRequest, r.Scheme())
	if err != nil {
		return err
	}

	vmAPIVersion := vmopv1.GroupVersion.String()
	clAPIVersion := imgregv1a1.GroupVersion.String()
	for sourceVM := range newReqsSet {
		// TTLSecondsAfterFinished is intentionally unset to avoid vm publish request to be
		// deleted after completion.
		if err := r.Create(ctx, &vmopv1.VirtualMachinePublishRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ctx.VMGroupPublishRequest.Namespace,
				GenerateName: ctx.VMGroupPublishRequest.Name + "-",
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(ctx.VMGroupPublishRequest, vmGroupPubReqGvk),
				},
				Labels: map[string]string{
					vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ctx.VMGroupPublishRequest.Name,
				},
			},
			Spec: vmopv1.VirtualMachinePublishRequestSpec{
				Source: vmopv1.VirtualMachinePublishRequestSource{
					Name:       sourceVM,
					Kind:       "VirtualMachine",
					APIVersion: vmAPIVersion,
				},
				Target: vmopv1.VirtualMachinePublishRequestTarget{
					Location: vmopv1.VirtualMachinePublishRequestTargetLocation{
						Name:       ctx.VMGroupPublishRequest.Spec.Target,
						Kind:       "ContentLibrary",
						APIVersion: clAPIVersion,
					},
					Item: vmopv1.VirtualMachinePublishRequestTargetItem{
						Name: ctx.VMGroupPublishRequest.Name + "-" + sourceVM,
					},
				},
			},
		}); err != nil {
			return err
		}
	}

	return nil
}

// reconcileStatusCompletedCondition sets vm group publish request's completed condition and completion time
// once all desired vm publish requests are completed.
func (r *Reconciler) reconcileStatusCompletedCondition(
	ctx *pkgctx.VirtualMachineGroupPublishRequestContext,
	pendingReqsSet, completedReqsSet, desiredReqsSet sets.Set[string]) error {

	if completedReqsSet.Equal(desiredReqsSet) {
		conditions.MarkTrue(ctx.VMGroupPublishRequest, vmopv1.VirtualMachineGroupPublishRequestConditionComplete)
		ctx.VMGroupPublishRequest.Status.CompletionTime = metav1.Now()
		ctx.Logger.Info("VirtualMachineGroupPublishRequest completed successfully")
		// update to condition will trigger a reconcile to process TTL value
		return nil
	}

	conditions.MarkFalse(
		ctx.VMGroupPublishRequest,
		vmopv1.VirtualMachinePublishRequestConditionComplete,
		vmopv1.VirtualMachineGroupPublishRequestConditionReasonPending,
		"waiting %d more vm publish requests to be completed",
		pendingReqsSet.Len())

	return nil
}

// verifySpec ensures spec's source, target, and virtualMachines are set up and validated properly by the webhooks.
// It sets the completed condition to false with message asking user to re-create the group request.
func (r *Reconciler) verifySpec(vmGroupPublishReq *vmopv1.VirtualMachineGroupPublishRequest) error {
	var errs []error
	if vmGroupPublishReq.Spec.Source == "" {
		errs = append(errs, fmt.Errorf(undefinedSpecErrorMsg, "source"))
	}
	if vmGroupPublishReq.Spec.Target == "" {
		errs = append(errs, fmt.Errorf(undefinedSpecErrorMsg, "target"))
	}
	if len(vmGroupPublishReq.Spec.VirtualMachines) == 0 {
		errs = append(errs, fmt.Errorf(undefinedSpecErrorMsg, "virtualMachines"))
	}

	if len(errs) > 0 {
		// in the rare case that webhooks were down when request was created
		// set condition to failure
		conditions.MarkError(vmGroupPublishReq,
			vmopv1.VirtualMachineGroupPublishRequestConditionComplete,
			invalidSpecErrorMsg,
			errors.Join(errs...))

		return pkgerr.NoRequeueError{Message: invalidSpecErrorMsg}
	}

	return nil
}

// reconcileSpecTTL deletes the group publish request and its child vm publish requests once the TTL expires.
func (r *Reconciler) reconcileSpecTTL(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) error {
	vmGroupPubReq := ctx.VMGroupPublishRequest

	// skip deletions when ttl is nil
	if vmGroupPubReq.Spec.TTLSecondsAfterFinished == nil {
		return nil
	}

	// calculate expiration time based on the completion time and the expected ttl.
	// when ttl is zero, the expiration completion time is same as completion time which triggers immediate deletion.
	expirationTime := vmGroupPubReq.Status.CompletionTime.Time.Add(time.Duration(*vmGroupPubReq.Spec.TTLSecondsAfterFinished) * time.Second)
	if time.Now().Before(expirationTime) {
		return pkgerr.RequeueError{After: time.Until(expirationTime)}
	}

	ctx.Logger.Info(fmt.Sprintf(delTTLMsg, "deleting"))
	if err := r.Delete(ctx, vmGroupPubReq); client.IgnoreNotFound(err) != nil {
		ctx.Logger.Error(err, fmt.Sprintf(delTTLMsg, "failed to delete"))
		return err
	}
	ctx.Logger.Info(fmt.Sprintf(delTTLMsg, "deleted"))
	return nil
}

// deleteVMPublishRequests deletes vm publish requests owned by this group publish request.
// It is called in ReconcileDelete() but not strictly necessarily since it's owned VirtualMachinePublishRequests have
// OwnerReference set to it and the k8s GC will clean up them as well if VirtualMachineGroupPublishRequest is deleted.
// The explicit delete helps speeding up the cleanup process.
func (r *Reconciler) deleteVMPublishRequests(ctx *pkgctx.VirtualMachineGroupPublishRequestContext) error {
	reqs, _, _, err := r.getVMPublishRequests(ctx)
	if err != nil {
		return err
	}

	for _, req := range reqs {
		if err := r.Delete(ctx, &req); client.IgnoreNotFound(err) != nil {
			ctx.Logger.Error(err, "failed to delete vm publish request", "name", req.Name)
			return err
		}
	}
	return nil
}
