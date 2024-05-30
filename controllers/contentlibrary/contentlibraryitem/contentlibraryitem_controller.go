// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	imgutil "github.com/vmware-tanzu/vm-operator/pkg/util/image"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		clItemType     = &imgregv1a1.ContentLibraryItem{}
		clItemTypeName = reflect.TypeOf(clItemType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(clItemTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
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
		Metrics:    metrics.NewContentLibraryItemMetrics(),
	}
}

// Reconciler reconciles an IaaS Image Registry Service's ContentLibraryItem object
// by creating/updating the corresponding VM-Service's VirtualMachineImage resource.
type Reconciler struct {
	client.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
	Metrics    *metrics.ContentLibraryItemMetrics
}

// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraryitems,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraryitems/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

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

	clItemCtx := &pkgctx.ContentLibraryItemContext{
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
func (r *Reconciler) ReconcileDelete(ctx *pkgctx.ContentLibraryItemContext) error {
	if controllerutil.ContainsFinalizer(ctx.CLItem, utils.CLItemFinalizer) ||
		controllerutil.ContainsFinalizer(ctx.CLItem, utils.DeprecatedCLItemFinalizer) {

		r.Metrics.DeleteMetrics(ctx.Logger, ctx.ImageObjName, ctx.CLItem.Namespace)
		controllerutil.RemoveFinalizer(ctx.CLItem, utils.CLItemFinalizer)
		controllerutil.RemoveFinalizer(ctx.CLItem, utils.DeprecatedCLItemFinalizer)
		return r.Update(ctx, ctx.CLItem)
	}

	return nil
}

// ReconcileNormal reconciles a ContentLibraryItem resource by creating or
// updating the corresponding VirtualMachineImage resource.
func (r *Reconciler) ReconcileNormal(ctx *pkgctx.ContentLibraryItemContext) error {
	if !controllerutil.ContainsFinalizer(ctx.CLItem, utils.CLItemFinalizer) {

		// If the object has the deprecated finalizer, remove it.
		if updated := controllerutil.RemoveFinalizer(ctx.CLItem, utils.DeprecatedCLItemFinalizer); updated {
			ctx.Logger.V(5).Info("Removed deprecated finalizer", "finalizerName", utils.DeprecatedCLItemFinalizer)
		}

		// The finalizer must be present before proceeding in order to ensure ReconcileDelete() will be called.
		// Return immediately after here to update the object and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.CLItem, utils.CLItemFinalizer)
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
		// Update image condition based on the security compliance of the provider item.
		clItemSecurityCompliance := ctx.CLItem.Status.SecurityCompliance
		if clItemSecurityCompliance == nil || !*clItemSecurityCompliance {
			conditions.MarkFalse(vmi,
				vmopv1.ReadyConditionType,
				vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason,
				"Provider item is not security compliant",
			)
			// Since we want to persist a False condition if the CCL Item is
			// not security compliant.
			return nil
		}

		// Check if the item is ready and skip the image content sync if not.
		if !utils.IsItemReady(ctx.CLItem.Status.Conditions) {
			conditions.MarkFalse(vmi,
				vmopv1.ReadyConditionType,
				vmopv1.VirtualMachineImageProviderNotReadyReason,
				"Provider item is not in ready condition",
			)
			ctx.Logger.Info("ContentLibraryItem is not ready yet, skipping image content sync")
			return nil
		}

		syncErr = r.syncImageContent(ctx)
		if syncErr == nil {
			// In this block, we have confirmed that all the three sub-conditions constituting this
			// Ready condition are true, hence mark it as true.
			conditions.MarkTrue(vmi, vmopv1.ReadyConditionType)
		}
		didSync = true

		// Do not return syncErr here as we still want to patch the updated fields we get above.
		return nil
	})

	ctx.Logger = ctx.Logger.WithValues("operationResult", opRes)

	// Registry metrics based on the corresponding error captured.
	defer func() {
		r.Metrics.RegisterVMIResourceResolve(ctx.Logger, vmi.Name, vmi.Namespace, createOrPatchErr == nil)
		r.Metrics.RegisterVMIContentSync(ctx.Logger, vmi.Name, vmi.Namespace, didSync && syncErr == nil)
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

	ctx.Logger.Info("Successfully reconciled VirtualMachineImage", "contentVersion", savedStatus.ProviderContentVersion)
	return nil
}

// setUpVMIFromCLItem sets up the VirtualMachineImage fields that
// are retrievable from the given ContentLibraryItem resource.
func (r *Reconciler) setUpVMIFromCLItem(ctx *pkgctx.ContentLibraryItemContext) error {
	clItem := ctx.CLItem
	vmi := ctx.VMI
	if err := controllerutil.SetControllerReference(clItem, vmi, r.Scheme()); err != nil {
		return err
	}

	vmi.Spec.ProviderRef = &common.LocalObjectRef{
		APIVersion: clItem.APIVersion,
		Kind:       clItem.Kind,
		Name:       clItem.Name,
	}
	vmi.Status.Name = clItem.Status.Name
	vmi.Status.ProviderItemID = string(clItem.Spec.UUID)

	return utils.AddContentLibraryRefToAnnotation(vmi, ctx.CLItem.Status.ContentLibraryRef)
}

// syncImageContent syncs the VirtualMachineImage content from the provider.
// It skips syncing if the image content is already up-to-date.
func (r *Reconciler) syncImageContent(ctx *pkgctx.ContentLibraryItemContext) error {
	clItem := ctx.CLItem
	vmi := ctx.VMI
	latestVersion := clItem.Status.ContentVersion
	if vmi.Status.ProviderContentVersion == latestVersion {
		return nil
	}

	err := r.VMProvider.SyncVirtualMachineImage(ctx, clItem, vmi)
	if err != nil {
		conditions.MarkFalse(vmi,
			vmopv1.ReadyConditionType,
			vmopv1.VirtualMachineImageNotSyncedReason,
			"Failed to sync to the latest content version from provider")
	} else {
		vmi.Status.ProviderContentVersion = latestVersion
	}

	// Sync the image's OS information and capabilities to the resource's labels
	// to make it easier for clients to search for images based on OS info and
	// image capabilities.
	imgutil.SyncStatusToLabels(vmi, vmi.Status)

	r.Recorder.EmitEvent(vmi, "Update", err, false)
	return err
}
