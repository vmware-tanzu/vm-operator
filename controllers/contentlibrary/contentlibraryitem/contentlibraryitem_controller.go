// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
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

// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraryitems,verbs=get;list;watch
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraryitems/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	r.Logger.Info("Reconciling ContentLibraryItem", "CLItemName", req.NamespacedName)

	clItem := &imgregv1a1.ContentLibraryItem{}
	if err := r.Get(ctx, req.NamespacedName, clItem); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !clItem.DeletionTimestamp.IsZero() {
		err := r.ReconcileDelete(clItem)
		return ctrl.Result{}, err
	}

	// Create or update the VirtualMachineImage resource accordingly.
	err := r.ReconcileNormal(ctx, clItem)
	return ctrl.Result{}, err
}

// ReconcileDelete reconciles a deletion for a ContentLibraryItem resource.
func (r *Reconciler) ReconcileDelete(clItem *imgregv1a1.ContentLibraryItem) error {
	logger := r.Logger.WithValues("clItemName", clItem.Name, "namespace", clItem.Namespace)

	vmiName, nameErr := utils.GetImageFieldNameFromItem(clItem.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported ContentLibraryItem, skip reconciling")
		return nil
	}

	r.Metrics.DeleteMetrics(logger, vmiName, clItem.Namespace)

	// NoOp. The VirtualMachineImages are automatically deleted because of OwnerReference.

	return nil
}

// ReconcileNormal reconciles a ContentLibraryItem resource by creating or
// updating the corresponding VirtualMachineImage resource.
func (r *Reconciler) ReconcileNormal(ctx goctx.Context, clItem *imgregv1a1.ContentLibraryItem) error {
	logger := r.Logger.WithValues("clItemName", clItem.Name, "namespace", clItem.Namespace)

	vmiName, nameErr := utils.GetImageFieldNameFromItem(clItem.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported ContentLibraryItem, skip reconciling")
		return nil
	}

	vmi := &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: clItem.Namespace,
		},
	}

	logger = logger.WithValues("vmiName", vmi.Name, "vmiNamespace", vmi.Namespace)

	clItemSecurityCompliance := clItem.Status.SecurityCompliance

	// If the ContentLibraryItem's security compliance field is not set or is false, delete the vmi if already exists
	if clItemSecurityCompliance == nil || !*clItemSecurityCompliance {
		logger.Info("ContentLibraryItem's security status is not true, deleting corresponding vmi if exists")
		if err := r.Client.Delete(ctx, vmi); err != nil {
			if k8serrors.IsNotFound(err) {
				logger.Info("Corresponding vmi not found, skipping delete")
				return nil
			}
			return err
		}

		r.Metrics.DeleteMetrics(logger, vmi.Name, vmi.Namespace)
		logger.Info("Deleted Corresponding vmi")
		return nil
	}

	var syncErr error
	var savedStatus *vmopv1.VirtualMachineImageStatus

	opRes, createOrPatchErr := controllerutil.CreateOrPatch(ctx, r.Client, vmi, func() error {
		defer func() {
			savedStatus = vmi.Status.DeepCopy()
		}()

		if utils.CheckItemReadyCondition(clItem.Status.Conditions) {
			conditions.MarkTrue(vmi, vmopv1.VirtualMachineImageProviderReadyCondition)
		} else {
			conditions.MarkFalse(vmi,
				vmopv1.VirtualMachineImageProviderReadyCondition,
				vmopv1.VirtualMachineImageProviderNotReadyReason,
				vmopv1.ConditionSeverityError,
				"Provider item is not in ready condition",
			)
			logger.Info("ContentLibraryItem is not ready yet, skipping further reconciliation")
			return nil
		}

		if err := r.setUpVMIFromCLItem(vmi, clItem); err != nil {
			logger.Error(err, "failed to set up VirtualMachineImage from ContentLibraryItem")
			return err
		}

		syncErr = r.syncImageContent(ctx, vmi, clItem)
		// Do not return syncErr here as we still want to patch the updated fields we get above.
		return nil
	})

	// Registry metrics based on the corresponding error captured.
	defer func() {
		r.Metrics.RegisterVMIResourceResolve(logger, vmi.Name, vmi.Namespace, createOrPatchErr == nil)
		r.Metrics.RegisterVMIContentSync(logger, vmi.Name, vmi.Namespace, syncErr == nil)
	}()

	logger = logger.WithValues("operationResult", opRes)
	if createOrPatchErr != nil {
		logger.Error(createOrPatchErr, "failed to create or patch VirtualMachineImage resource")
		return createOrPatchErr
	}

	// CreateOrPatch/CreateOrUpdate doesn't patch sub-resource for creation.
	if opRes == controllerutil.OperationResultCreated {
		vmi.Status = *savedStatus
		if createOrPatchErr = r.Status().Update(ctx, vmi); createOrPatchErr != nil {
			logger.Error(createOrPatchErr, "failed to update VirtualMachineImage status")
			return createOrPatchErr
		}
	}

	if syncErr != nil {
		logger.Error(syncErr, "failed to sync VirtualMachineImage to the latest content version")
		return syncErr
	}

	logger.Info("Successfully reconciled VirtualMachineImage", "contentVersion", savedStatus.ContentVersion)
	return nil
}

// setUpVMIFromCLItem sets up the VirtualMachineImage fields that
// are retrievable from the given ContentLibraryItem resource.
func (r *Reconciler) setUpVMIFromCLItem(vmi *vmopv1.VirtualMachineImage,
	clItem *imgregv1a1.ContentLibraryItem) error {
	if err := controllerutil.SetControllerReference(clItem, vmi, r.Scheme()); err != nil {
		return err
	}

	// Do not initialize the Spec or Status directly as it might overwrite the existing fields.
	vmi.Spec.Type = string(clItem.Status.Type)
	vmi.Spec.ImageID = clItem.Spec.UUID
	vmi.Spec.ProviderRef = vmopv1.ContentProviderReference{
		APIVersion: clItem.APIVersion,
		Kind:       clItem.Kind,
		Name:       clItem.Name,
	}
	vmi.Status.ImageName = clItem.Status.Name
	vmi.Status.ContentLibraryRef = &corev1.TypedLocalObjectReference{
		APIGroup: &imgregv1a1.GroupVersion.Group,
		Kind:     utils.ContentLibraryKind,
		Name:     clItem.Status.ContentLibraryRef.Name,
	}

	return nil
}

// syncImageContent syncs the VirtualMachineImage content from the provider.
// It skips syncing if the image content is already up-to-date.
func (r *Reconciler) syncImageContent(ctx goctx.Context,
	vmi *vmopv1.VirtualMachineImage, clItem *imgregv1a1.ContentLibraryItem) error {
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
