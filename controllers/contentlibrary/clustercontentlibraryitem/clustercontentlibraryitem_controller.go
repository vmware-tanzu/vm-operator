// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

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

// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=clustercontentlibraryitems,verbs=get;list;watch
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=clustercontentlibraryitems/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	cclItem := &imgregv1a1.ClusterContentLibraryItem{}
	if err := r.Get(ctx, req.NamespacedName, cclItem); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cclItem.DeletionTimestamp.IsZero() {
		err := r.ReconcileDelete(cclItem)
		return ctrl.Result{}, err
	}

	// Create or update the ClusterVirtualMachineImage resource accordingly.
	err := r.ReconcileNormal(ctx, cclItem)
	return ctrl.Result{}, err
}

// ReconcileDelete reconciles a deletion for a ClusterContentLibraryItem resource.
func (r *Reconciler) ReconcileDelete(cclItem *imgregv1a1.ClusterContentLibraryItem) error {
	logger := r.Logger.WithValues("cclItemName", cclItem.Name)

	cvmiName, nameErr := utils.GetImageFieldNameFromItem(cclItem.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported ClusterContentLibraryItem, skip reconciling")
		return nil
	}

	r.Metrics.DeleteMetrics(logger, cvmiName, "")

	// NoOp. The ClusterVirtualMachineImages are automatically deleted because of OwnerReference.

	return nil
}

// ReconcileNormal reconciles a ClusterContentLibraryItem resource by creating or
// updating the corresponding ClusterVirtualMachineImage resource.
func (r *Reconciler) ReconcileNormal(ctx goctx.Context, cclItem *imgregv1a1.ClusterContentLibraryItem) error {
	logger := r.Logger.WithValues("cclItemName", cclItem.Name)

	cvmiName, nameErr := utils.GetImageFieldNameFromItem(cclItem.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported ClusterContentLibraryItem, skip reconciling")
		return nil
	}

	cvmi := &vmopv1a1.ClusterVirtualMachineImage{}
	cvmi.Name = cvmiName
	logger = logger.WithValues("cvmiName", cvmi.Name)

	cclItemSecurityCompliance := cclItem.Status.SecurityCompliance

	// If the ClusterContentLibraryItem's security compliance field is not set or is false, delete corresponding cvmi if exists
	if cclItemSecurityCompliance == nil || !*cclItemSecurityCompliance {
		logger.Info("ClusterContentLibraryItem's security compliance is not true, deleting corresponding cvmi if exists")
		if err := r.Client.Delete(ctx, cvmi); err != nil {
			if k8serrors.IsNotFound(err) {
				logger.Info("Corresponding cvmi not found, skipping delete")
				return nil
			}
			return err
		}

		r.Metrics.DeleteMetrics(logger, cvmi.Name, "")
		logger.Info("Deleted corresponding cvmi")
		return nil
	}

	var syncErr error
	var savedStatus *vmopv1a1.VirtualMachineImageStatus

	opRes, createOrPatchErr := controllerutil.CreateOrPatch(ctx, r.Client, cvmi, func() error {
		defer func() {
			savedStatus = cvmi.Status.DeepCopy()
		}()

		if utils.CheckItemReadyCondition(cclItem.Status.Conditions) {
			conditions.MarkTrue(cvmi, vmopv1a1.VirtualMachineImageProviderReadyCondition)
		} else {
			conditions.MarkFalse(cvmi,
				vmopv1a1.VirtualMachineImageProviderReadyCondition,
				vmopv1a1.VirtualMachineImageProviderNotReadyReason,
				vmopv1a1.ConditionSeverityError,
				"Provider item is not in ready condition",
			)
			logger.Info("ClusterContentLibraryItem is not ready yet, skipping further reconciliation")
			return nil
		}

		if err := r.setUpCVMIFromCCLItem(cvmi, cclItem); err != nil {
			logger.Error(err, "Failed to set up ClusterVirtualMachineImage from ClusterContentLibraryItem")
			return err
		}

		syncErr = r.syncImageContent(ctx, cvmi, cclItem)
		// Do not return syncErr here as we still want to patch the updated fields we get above.
		return nil
	})

	logger = logger.WithValues("operationResult", opRes)
	if createOrPatchErr != nil {
		logger.Error(createOrPatchErr, "Failed to create or patch ClusterVirtualMachineImage resource")
		return createOrPatchErr
	}

	// Registry metrics based on the corresponding error captured.
	defer func() {
		r.Metrics.RegisterVMIResourceResolve(logger, cvmi.Name, "", createOrPatchErr == nil)
		r.Metrics.RegisterVMIContentSync(logger, cvmi.Name, "", syncErr == nil)
	}()

	// CreateOrPatch/CreateOrUpdate doesn't patch sub-resource for creation.
	if opRes == controllerutil.OperationResultCreated {
		cvmi.Status = *savedStatus
		if createOrPatchErr = r.Status().Update(ctx, cvmi); createOrPatchErr != nil {
			logger.Error(createOrPatchErr, "failed to update ClusterVirtualMachineImage status")
			return createOrPatchErr
		}
	}

	if syncErr != nil {
		logger.Error(syncErr, "Failed to sync ClusterVirtualMachineImage to the latest content version")
		return syncErr
	}

	logger.Info("Successfully reconciled ClusterVirtualMachineImage", "contentVersion", savedStatus.ContentVersion)
	return nil
}

// setUpCVMIFromCCLItem sets up the ClusterVirtualMachineImage fields that
// are retrievable from the given ClusterContentLibraryItem resource.
func (r *Reconciler) setUpCVMIFromCCLItem(cvmi *vmopv1a1.ClusterVirtualMachineImage,
	cclItem *imgregv1a1.ClusterContentLibraryItem) error {
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
	cvmi.Spec.ImageID = cclItem.Spec.UUID
	cvmi.Spec.ProviderRef = vmopv1a1.ContentProviderReference{
		APIVersion: cclItem.APIVersion,
		Kind:       cclItem.Kind,
		Name:       cclItem.Name,
	}

	cvmi.Status.ImageName = cclItem.Status.Name
	cvmi.Status.ContentLibraryRef = &corev1.TypedLocalObjectReference{
		APIGroup: &imgregv1a1.GroupVersion.Group,
		Kind:     utils.ClusterContentLibraryKind,
		Name:     cclItem.Status.ClusterContentLibraryRef,
	}

	return nil
}

// syncImageContent syncs the ClusterVirtualMachineImage content from the provider.
// It skips syncing if the image content is already up-to-date.
func (r *Reconciler) syncImageContent(ctx goctx.Context,
	cvmi *vmopv1a1.ClusterVirtualMachineImage, cclItem *imgregv1a1.ClusterContentLibraryItem) error {
	latestVersion := cclItem.Status.ContentVersion
	if cvmi.Status.ContentVersion == latestVersion {
		return nil
	}

	err := r.VMProvider.SyncVirtualMachineImage(ctx, cclItem.Spec.UUID, cvmi)
	if err != nil {
		conditions.MarkFalse(cvmi,
			vmopv1a1.VirtualMachineImageSyncedCondition,
			vmopv1a1.VirtualMachineImageNotSyncedReason,
			vmopv1a1.ConditionSeverityError,
			"Failed to sync to the latest content version from provider")
	} else {
		conditions.MarkTrue(cvmi, vmopv1a1.VirtualMachineImageSyncedCondition)
		cvmi.Status.ContentVersion = latestVersion
	}

	r.Recorder.EmitEvent(cvmi, "Update", err, false)
	return err
}
