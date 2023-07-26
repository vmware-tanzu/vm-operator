// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		cclItemType     = &imgregv1a1.ClusterContentLibraryItem{}
		cclItemTypeName = reflect.TypeOf(cclItemType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(cclItemTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(cclItemTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(cclItemType).
		// We do not set Owns(ClusterVirtualMachineImage) here as we call SetControllerReference()
		// when creating such resources in the reconciling process below.
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
		Metrics:    metrics.NewContentLibraryItemMetrics(),
	}
}

// Reconciler reconciles an IaaS Image Registry Service's ClusterContentLibraryItem object
// by creating/updating the corresponding VM-Service's ClusterVirtualMachineImage resource.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
	Metrics    *metrics.ContentLibraryItemMetrics
}

// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=clustercontentlibraryitems,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=clustercontentlibraryitems/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := r.Logger.WithValues("cclItemName", req.Name)
	logger.Info("Reconciling ClusterContentLibraryItem")

	cclItem := &imgregv1a1.ClusterContentLibraryItem{}
	if err := r.Get(ctx, req.NamespacedName, cclItem); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cvmiName, nameErr := utils.GetImageFieldNameFromItem(cclItem.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported ClusterContentLibraryItem name, skip reconciling")
		return ctrl.Result{}, nil
	}
	logger = logger.WithValues("cvmiName", cvmiName)

	cclItemCtx := &context.ClusterContentLibraryItemContext{
		Context:      ctx,
		Logger:       logger,
		CCLItem:      cclItem,
		ImageObjName: cvmiName,
	}

	if !cclItem.DeletionTimestamp.IsZero() {
		err := r.ReconcileDelete(cclItemCtx)
		return ctrl.Result{}, err
	}

	// Create or update the ClusterVirtualMachineImage resource accordingly.
	err := r.ReconcileNormal(cclItemCtx)
	return ctrl.Result{}, err
}

// ReconcileDelete reconciles a deletion for a ClusterContentLibraryItem resource.
func (r *Reconciler) ReconcileDelete(ctx *context.ClusterContentLibraryItemContext) error {
	if controllerutil.ContainsFinalizer(ctx.CCLItem, utils.ClusterContentLibraryItemVmopFinalizer) {
		r.Metrics.DeleteMetrics(ctx.Logger, ctx.ImageObjName, "")
		controllerutil.RemoveFinalizer(ctx.CCLItem, utils.ClusterContentLibraryItemVmopFinalizer)
		return r.Update(ctx, ctx.CCLItem)
	}

	return nil
}

// ReconcileNormal reconciles a ClusterContentLibraryItem resource by creating or
// updating the corresponding ClusterVirtualMachineImage resource.
func (r *Reconciler) ReconcileNormal(ctx *context.ClusterContentLibraryItemContext) error {
	if !controllerutil.ContainsFinalizer(ctx.CCLItem, utils.ClusterContentLibraryItemVmopFinalizer) {
		// The finalizer must be present before proceeding in order to ensure ReconcileDelete() will be called.
		// Return immediately after here to update the object and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.CCLItem, utils.ClusterContentLibraryItemVmopFinalizer)
		return r.Update(ctx, ctx.CCLItem)
	}

	// Do not set additional fields here as they will be overwritten in CreateOrPatch below.
	cvmi := &vmopv1.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: ctx.ImageObjName,
		},
	}
	ctx.CVMI = cvmi

	var didSync bool
	var syncErr error
	var savedStatus *vmopv1.VirtualMachineImageStatus

	opRes, createOrPatchErr := controllerutil.CreateOrPatch(ctx, r.Client, cvmi, func() error {
		defer func() {
			savedStatus = cvmi.Status.DeepCopy()
		}()

		if err := r.setUpCVMIFromCCLItem(ctx); err != nil {
			ctx.Logger.Error(err, "Failed to set up ClusterVirtualMachineImage from ClusterContentLibraryItem")
			return err
		}

		// Check if the item is ready and skip the image content sync if not.
		if !utils.IsItemReady(ctx.CCLItem.Status.Conditions) {
			conditions.MarkFalse(cvmi,
				vmopv1.VirtualMachineImageProviderReadyCondition,
				vmopv1.VirtualMachineImageProviderNotReadyReason,
				vmopv1.ConditionSeverityError,
				"Provider item is not in ready condition",
			)
			ctx.Logger.Info("ClusterContentLibraryItem is not ready yet, skipping image content sync")
		} else {
			conditions.MarkTrue(cvmi, vmopv1.VirtualMachineImageProviderReadyCondition)
			syncErr = r.syncImageContent(ctx)
			didSync = true
		}

		// Do not return syncErr here as we still want to patch the updated fields we get above.
		return nil
	})

	ctx.Logger = ctx.Logger.WithValues("operationResult", opRes)

	// Registry metrics based on the corresponding error captured.
	defer func() {
		r.Metrics.RegisterVMIResourceResolve(ctx.Logger, cvmi.Name, "", createOrPatchErr == nil)
		r.Metrics.RegisterVMIContentSync(ctx.Logger, cvmi.Name, "", (didSync && syncErr == nil))
	}()

	if createOrPatchErr != nil {
		ctx.Logger.Error(createOrPatchErr, "Failed to create or patch ClusterVirtualMachineImage resource")
		return createOrPatchErr
	}

	// CreateOrPatch/CreateOrUpdate doesn't patch sub-resource for creation.
	if opRes == controllerutil.OperationResultCreated {
		cvmi.Status = *savedStatus
		if createOrPatchErr = r.Status().Update(ctx, cvmi); createOrPatchErr != nil {
			ctx.Logger.Error(createOrPatchErr, "Failed to update ClusterVirtualMachineImage status")
			return createOrPatchErr
		}
	}

	if syncErr != nil {
		ctx.Logger.Error(syncErr, "Failed to sync ClusterVirtualMachineImage to the latest content version")
		return syncErr
	}

	ctx.Logger.Info("Successfully reconciled ClusterVirtualMachineImage", "contentVersion", savedStatus.ContentVersion)
	return nil
}

// setUpCVMIFromCCLItem sets up the ClusterVirtualMachineImage fields that
// are retrievable from the given ClusterContentLibraryItem resource.
func (r *Reconciler) setUpCVMIFromCCLItem(ctx *context.ClusterContentLibraryItemContext) error {
	cclItem := ctx.CCLItem
	cvmi := ctx.CVMI
	if err := controllerutil.SetControllerReference(cclItem, cvmi, r.Scheme()); err != nil {
		return err
	}

	if cvmi.Labels == nil {
		cvmi.Labels = make(map[string]string)
	}

	// Only watch for service type labels from ClusterContentLibraryItem
	for label := range cclItem.Labels {
		if strings.HasPrefix(label, "type.services.vmware.com/") {
			cvmi.Labels[label] = ""
		}
	}

	// Do not initialize the Spec or Status directly as it might overwrite the existing fields.
	cvmi.Spec.Type = string(cclItem.Status.Type)
	cvmi.Spec.ImageID = string(cclItem.Spec.UUID)
	cvmi.Spec.ProviderRef = vmopv1.ContentProviderReference{
		APIVersion: cclItem.APIVersion,
		Kind:       cclItem.Kind,
		Name:       cclItem.Name,
	}

	cvmi.Status.ImageName = cclItem.Status.Name
	if cclItem.Status.ContentLibraryRef != nil {
		cvmi.Status.ContentLibraryRef = &corev1.TypedLocalObjectReference{
			APIGroup: &imgregv1a1.GroupVersion.Group,
			Kind:     cclItem.Status.ContentLibraryRef.Kind,
			Name:     cclItem.Status.ContentLibraryRef.Name,
		}
	}

	// Update image condition based on the security compliance of the provider item.
	cclItemSecurityCompliance := ctx.CCLItem.Status.SecurityCompliance
	if cclItemSecurityCompliance == nil || !*cclItemSecurityCompliance {
		conditions.MarkFalse(cvmi,
			vmopv1.VirtualMachineImageProviderSecurityComplianceCondition,
			vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason,
			vmopv1.ConditionSeverityError,
			"Provider item is not security compliant",
		)
	} else {
		conditions.MarkTrue(cvmi, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
	}

	return nil
}

// syncImageContent syncs the ClusterVirtualMachineImage content from the provider.
// It skips syncing if the image content is already up-to-date.
func (r *Reconciler) syncImageContent(ctx *context.ClusterContentLibraryItemContext) error {
	cclItem := ctx.CCLItem
	cvmi := ctx.CVMI
	latestVersion := cclItem.Status.ContentVersion
	if cvmi.Status.ContentVersion == latestVersion {
		return nil
	}

	err := r.VMProvider.SyncVirtualMachineImage(ctx, cclItem, cvmi)
	if err != nil {
		conditions.MarkFalse(cvmi,
			vmopv1.VirtualMachineImageSyncedCondition,
			vmopv1.VirtualMachineImageNotSyncedReason,
			vmopv1.ConditionSeverityError,
			"Failed to sync to the latest content version from provider")
	} else {
		conditions.MarkTrue(cvmi, vmopv1.VirtualMachineImageSyncedCondition)
		cvmi.Status.ContentVersion = latestVersion
	}

	r.Recorder.EmitEvent(cvmi, "Update", err, false)
	return err
}
