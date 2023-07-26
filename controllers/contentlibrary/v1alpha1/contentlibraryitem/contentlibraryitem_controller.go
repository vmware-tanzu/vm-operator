// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem

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
		clItemType     = &imgregv1a1.ContentLibraryItem{}
		clItemTypeName = reflect.TypeOf(clItemType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(clItemTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(clItemTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(clItemType).
		// We do not set Owns(VirtualMachineImage) here as we call SetControllerReference()
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

// Reconciler reconciles an IaaS Image Registry Service's ContentLibraryItem object
// by creating/updating the corresponding VM-Service's VirtualMachineImage resource.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
	Metrics    *metrics.ContentLibraryItemMetrics
}

// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraryitems,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraryitems/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := r.Logger.WithValues("clItemName", req.Name, "namespace", req.Namespace)
	logger.Info("Reconciling ContentLibraryItem")

	clItem := &imgregv1a1.ContentLibraryItem{}
	if err := r.Get(ctx, req.NamespacedName, clItem); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmiName, nameErr := utils.GetImageFieldNameFromItem(clItem.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported ContentLibraryItem name, skip reconciling")
		return ctrl.Result{}, nil
	}
	logger = logger.WithValues("vmiName", vmiName)

	clItemCtx := &context.ContentLibraryItemContext{
		Context:      ctx,
		Logger:       logger,
		CLItem:       clItem,
		ImageObjName: vmiName,
	}

	if !clItem.DeletionTimestamp.IsZero() {
		err := r.ReconcileDelete(clItemCtx)
		return ctrl.Result{}, err
	}

	// Create or update the VirtualMachineImage resource accordingly.
	err := r.ReconcileNormal(clItemCtx)
	return ctrl.Result{}, err
}

// ReconcileDelete reconciles a deletion for a ContentLibraryItem resource.
func (r *Reconciler) ReconcileDelete(ctx *context.ContentLibraryItemContext) error {
	if controllerutil.ContainsFinalizer(ctx.CLItem, utils.ContentLibraryItemVmopFinalizer) {
		r.Metrics.DeleteMetrics(ctx.Logger, ctx.ImageObjName, ctx.CLItem.Namespace)
		controllerutil.RemoveFinalizer(ctx.CLItem, utils.ContentLibraryItemVmopFinalizer)
		return r.Update(ctx, ctx.CLItem)
	}

	return nil
}

// ReconcileNormal reconciles a ContentLibraryItem resource by creating or
// updating the corresponding VirtualMachineImage resource.
func (r *Reconciler) ReconcileNormal(ctx *context.ContentLibraryItemContext) error {
	if !controllerutil.ContainsFinalizer(ctx.CLItem, utils.ContentLibraryItemVmopFinalizer) {
		// The finalizer must be present before proceeding in order to ensure ReconcileDelete() will be called.
		// Return immediately after here to update the object and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.CLItem, utils.ContentLibraryItemVmopFinalizer)
		return r.Update(ctx, ctx.CLItem)
	}

	// Do not set additional fields here as they will be overwritten in CreateOrPatch below.
	vmi := &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ctx.ImageObjName,
			Namespace: ctx.CLItem.Namespace,
		},
	}
	ctx.VMI = vmi

	var didSync bool
	var syncErr error
	var savedStatus *vmopv1.VirtualMachineImageStatus

	opRes, createOrPatchErr := controllerutil.CreateOrPatch(ctx, r.Client, vmi, func() error {
		defer func() {
			savedStatus = vmi.Status.DeepCopy()
		}()

		if err := r.setUpVMIFromCLItem(ctx); err != nil {
			ctx.Logger.Error(err, "Failed to set up VirtualMachineImage from ContentLibraryItem")
			return err
		}

		// Check if the item is ready and skip the image content sync if not.
		if !utils.IsItemReady(ctx.CLItem.Status.Conditions) {
			conditions.MarkFalse(vmi,
				vmopv1.VirtualMachineImageProviderReadyCondition,
				vmopv1.VirtualMachineImageProviderNotReadyReason,
				vmopv1.ConditionSeverityError,
				"Provider item is not in ready condition",
			)
			ctx.Logger.Info("ContentLibraryItem is not ready yet, skipping image content sync")
		} else {
			conditions.MarkTrue(vmi, vmopv1.VirtualMachineImageProviderReadyCondition)
			syncErr = r.syncImageContent(ctx)
			didSync = true
		}

		// Do not return syncErr here as we still want to patch the updated fields we get above.
		return nil
	})

	ctx.Logger = ctx.Logger.WithValues("operationResult", opRes)

	// Registry metrics based on the corresponding error captured.
	defer func() {
		r.Metrics.RegisterVMIResourceResolve(ctx.Logger, vmi.Name, vmi.Namespace, createOrPatchErr == nil)
		r.Metrics.RegisterVMIContentSync(ctx.Logger, vmi.Name, vmi.Namespace, (didSync && syncErr == nil))
	}()

	if createOrPatchErr != nil {
		ctx.Logger.Error(createOrPatchErr, "failed to create or patch VirtualMachineImage resource")
		return createOrPatchErr
	}

	// CreateOrPatch/CreateOrUpdate doesn't patch sub-resource for creation.
	if opRes == controllerutil.OperationResultCreated {
		vmi.Status = *savedStatus
		if createOrPatchErr = r.Status().Update(ctx, vmi); createOrPatchErr != nil {
			ctx.Logger.Error(createOrPatchErr, "Failed to update VirtualMachineImage status")
			return createOrPatchErr
		}
	}

	if syncErr != nil {
		ctx.Logger.Error(syncErr, "Failed to sync VirtualMachineImage to the latest content version")
		return syncErr
	}

	ctx.Logger.Info("Successfully reconciled VirtualMachineImage", "contentVersion", savedStatus.ContentVersion)
	return nil
}

// setUpVMIFromCLItem sets up the VirtualMachineImage fields that
// are retrievable from the given ContentLibraryItem resource.
func (r *Reconciler) setUpVMIFromCLItem(ctx *context.ContentLibraryItemContext) error {
	clItem := ctx.CLItem
	vmi := ctx.VMI
	if err := controllerutil.SetControllerReference(clItem, vmi, r.Scheme()); err != nil {
		return err
	}

	// Do not initialize the Spec or Status directly as it might overwrite the existing fields.
	vmi.Spec.Type = string(clItem.Status.Type)
	vmi.Spec.ImageID = string(clItem.Spec.UUID)
	vmi.Spec.ProviderRef = vmopv1.ContentProviderReference{
		APIVersion: clItem.APIVersion,
		Kind:       clItem.Kind,
		Name:       clItem.Name,
	}
	vmi.Status.ImageName = clItem.Status.Name
	if clItem.Status.ContentLibraryRef != nil {
		vmi.Status.ContentLibraryRef = &corev1.TypedLocalObjectReference{
			APIGroup: &imgregv1a1.GroupVersion.Group,
			Kind:     clItem.Status.ContentLibraryRef.Kind,
			Name:     clItem.Status.ContentLibraryRef.Name,
		}
	}

	// Update image condition based on the security compliance of the provider item.
	clItemSecurityCompliance := ctx.CLItem.Status.SecurityCompliance
	if clItemSecurityCompliance == nil || !*clItemSecurityCompliance {
		conditions.MarkFalse(vmi,
			vmopv1.VirtualMachineImageProviderSecurityComplianceCondition,
			vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason,
			vmopv1.ConditionSeverityError,
			"Provider item is not security compliant",
		)
	} else {
		conditions.MarkTrue(vmi, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
	}

	return nil
}

// syncImageContent syncs the VirtualMachineImage content from the provider.
// It skips syncing if the image content is already up-to-date.
func (r *Reconciler) syncImageContent(ctx *context.ContentLibraryItemContext) error {
	clItem := ctx.CLItem
	vmi := ctx.VMI
	latestVersion := clItem.Status.ContentVersion
	if vmi.Status.ContentVersion == latestVersion {
		return nil
	}

	err := r.VMProvider.SyncVirtualMachineImage(ctx, clItem, vmi)
	if err != nil {
		conditions.MarkFalse(vmi,
			vmopv1.VirtualMachineImageSyncedCondition,
			vmopv1.VirtualMachineImageNotSyncedReason,
			vmopv1.ConditionSeverityError,
			"Failed to sync to the latest content version from provider")
	} else {
		conditions.MarkTrue(vmi, vmopv1.VirtualMachineImageSyncedCondition)
		vmi.Status.ContentVersion = latestVersion
	}

	r.Recorder.EmitEvent(vmi, "Update", err, false)
	return err
}
