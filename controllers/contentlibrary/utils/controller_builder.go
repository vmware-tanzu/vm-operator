// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	imgutil "github.com/vmware-tanzu/vm-operator/pkg/util/image"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

func AddToManager(
	ctx *pkgctx.ControllerManagerContext,
	mgr manager.Manager,
	obj client.Object) error {

	var (
		controlledItemType     = obj
		controlledItemTypeName = reflect.TypeOf(controlledItemType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledItemTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledItemTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
		controlledItemTypeName,
	)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(controlledItemType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
			SkipNameValidation:      SkipNameValidation,
		})

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		builder = builder.Watches(
			&vmopv1.VirtualMachineImageCache{},
			handler.EnqueueRequestsFromMapFunc(
				vmopv1util.VirtualMachineImageCacheToItemMapper(
					ctx,
					r.Logger.WithName("VirtualMachineImageCacheToItemMapper"),
					r.Client,
					imgregv1a1.GroupVersion,
					controlledItemTypeName),
			))
	}

	return builder.Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface,
	kind string) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
		Metrics:    metrics.NewContentLibraryItemMetrics(),
		Kind:       kind,
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
	Kind       string
}

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = ovfcache.JoinContext(ctx, r.Context)

	logger := ctrl.Log.WithName(r.Kind).WithValues("name", req.String())

	var (
		obj    client.Object
		spec   *imgregv1a1.ContentLibraryItemSpec
		status *imgregv1a1.ContentLibraryItemStatus
	)

	if req.Namespace != "" {
		var o imgregv1a1.ContentLibraryItem
		if err := r.Get(ctx, req.NamespacedName, &o); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		obj, spec, status = &o, &o.Spec, &o.Status
	} else {
		var o imgregv1a1.ClusterContentLibraryItem
		if err := r.Get(ctx, req.NamespacedName, &o); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		obj, spec, status = &o, &o.Spec, &o.Status
	}

	vmiName, nameErr := GetImageFieldNameFromItem(req.Name)
	if nameErr != nil {
		logger.Error(nameErr, "Unsupported library item name, skip reconciling")
		return ctrl.Result{}, nil
	}
	logger = logger.WithValues("vmiName", vmiName)

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for %s: %w", req.NamespacedName, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, obj); err != nil {
			if reterr == nil {
				reterr = err
			}
			logger.Error(err, "patch failed")
		}
	}()

	if !obj.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, r.ReconcileDelete(ctx, logger, obj, vmiName)
	}

	// Create or update the VirtualMachineImage resource accordingly.
	return ctrl.Result{}, r.ReconcileNormal(
		ctx,
		logger,
		obj,
		spec,
		status,
		vmiName)
}

// ReconcileDelete reconciles a deletion for a ContentLibraryItem resource.
func (r *Reconciler) ReconcileDelete(
	ctx context.Context,
	logger logr.Logger,
	obj client.Object,
	vmiName string) error {

	finalizer, depFinalizer := GetAppropriateFinalizers(obj)
	if !controllerutil.ContainsFinalizer(obj, finalizer) &&
		!controllerutil.ContainsFinalizer(obj, depFinalizer) {

		return nil
	}

	r.Metrics.DeleteMetrics(logger, vmiName, obj.GetNamespace())
	controllerutil.RemoveFinalizer(obj, finalizer)
	controllerutil.RemoveFinalizer(obj, depFinalizer)

	return nil
}

// ReconcileNormal reconciles a ContentLibraryItem resource by creating or
// updating the corresponding VirtualMachineImage resource.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	logger logr.Logger,
	cliObj client.Object,
	cliSpec *imgregv1a1.ContentLibraryItemSpec,
	cliStatus *imgregv1a1.ContentLibraryItemStatus,
	vmiName string) error {

	finalizer, depFinalizer := GetAppropriateFinalizers(cliObj)
	if !controllerutil.ContainsFinalizer(cliObj, finalizer) {

		// If the object has the deprecated finalizer, remove it.
		if controllerutil.RemoveFinalizer(
			cliObj,
			depFinalizer) {

			logger.V(5).Info(
				"Removed deprecated finalizer",
				"finalizerName", depFinalizer)
		}

		// The finalizer must be present before proceeding in order to ensure
		// ReconcileDelete() will be called. Return immediately after here to
		// update the object and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(cliObj, finalizer)

		return nil
	}

	var (
		vmiObj    client.Object
		vmiSpec   *vmopv1.VirtualMachineImageSpec
		vmiStatus *vmopv1.VirtualMachineImageStatus
		vmiKind   string
	)

	if ns := cliObj.GetNamespace(); ns != "" {
		o := vmopv1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmiName,
				Namespace: ns,
			},
		}
		vmiKind = "VirtualMachineImage"
		vmiObj, vmiSpec, vmiStatus = &o, &o.Spec, &o.Status
	} else {
		o := vmopv1.ClusterVirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: vmiName,
			},
		}
		vmiKind = "ClusterVirtualMachineImage"
		vmiObj, vmiSpec, vmiStatus = &o, &o.Spec, &o.Status
	}

	logger = logger.WithValues("vmiKind", vmiKind)

	var (
		didSync     bool
		syncErr     error
		savedStatus *vmopv1.VirtualMachineImageStatus
	)

	opRes, copErr := controllerutil.CreateOrPatch(
		ctx,
		r.Client,
		vmiObj,
		func() error {

			defer func() {
				savedStatus = vmiStatus.DeepCopy()
			}()

			if err := r.setUpVMIFromCLItem(
				ctx,
				cliObj,
				*cliSpec,
				*cliStatus,
				vmiObj,
				vmiSpec,
				vmiStatus); err != nil {

				logger.Error(err, "Failed to setup image from library item")
				return err
			}

			// Update image condition based on the security compliance of the
			// underlying library item.
			clItemSecurityCompliance := cliStatus.SecurityCompliance
			if clItemSecurityCompliance == nil || !*clItemSecurityCompliance {
				pkgcnd.MarkFalse(
					vmiStatus,
					vmopv1.ReadyConditionType,
					vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason,
					"Provider item is not security compliant",
				)
				// Since we want to persist a False condition if the CCL Item is
				// not security compliant.
				return nil
			}

			// Check if the item is ready and skip the image content sync if
			// not.
			if !IsItemReady(cliStatus.Conditions) {
				pkgcnd.MarkFalse(
					vmiStatus,
					vmopv1.ReadyConditionType,
					vmopv1.VirtualMachineImageProviderNotReadyReason,
					"Provider item is not in ready condition",
				)
				logger.Info(
					"Skipping image content sync",
					"reason", "library item not ready")
				return nil
			}

			// If the sync is successful then the VMI resource is ready.
			if syncErr = r.syncImageContent(
				ctx,
				cliObj,
				*cliStatus,
				vmiObj,
				vmiStatus); syncErr == nil {

				pkgcnd.MarkTrue(vmiStatus, vmopv1.ReadyConditionType)
			}

			didSync = true

			// Do not return syncErr here as we still want to patch the updated
			// fields we get above.
			return nil
		})

	logger = logger.WithValues("operationResult", opRes)

	// Registry metrics based on the corresponding error captured.
	defer func() {
		r.Metrics.RegisterVMIResourceResolve(
			logger,
			vmiObj.GetName(),
			vmiObj.GetNamespace(),
			copErr == nil)
		r.Metrics.RegisterVMIContentSync(logger,
			vmiObj.GetName(),
			vmiObj.GetNamespace(),
			didSync && syncErr == nil)
	}()

	if copErr != nil {
		logger.Error(copErr, "failed to create or patch image")
		return copErr
	}

	// CreateOrPatch/CreateOrUpdate doesn't patch sub-resource for creation.
	if opRes == controllerutil.OperationResultCreated {
		*vmiStatus = *savedStatus
		if copErr = r.Status().Update(ctx, vmiObj); copErr != nil {
			logger.Error(copErr, "Failed to update image status")
			return copErr
		}
	}

	if syncErr != nil {
		logger.Error(
			syncErr,
			"Failed to sync image to the latest content version")
		return syncErr
	}

	logger.Info(
		"Successfully reconciled library item",
		"contentVersion", savedStatus.ProviderContentVersion)
	return nil
}

// setUpVMIFromCLItem sets up the VirtualMachineImage fields that
// are retrievable from the given ContentLibraryItem resource.
func (r *Reconciler) setUpVMIFromCLItem(
	ctx context.Context,
	cliObj client.Object,
	cliSpec imgregv1a1.ContentLibraryItemSpec,
	cliStatus imgregv1a1.ContentLibraryItemStatus,
	vmiObj client.Object,
	vmiSpec *vmopv1.VirtualMachineImageSpec,
	vmiStatus *vmopv1.VirtualMachineImageStatus) error {

	if err := controllerutil.SetControllerReference(
		cliObj,
		vmiObj,
		r.Scheme()); err != nil {

		return err
	}

	cliGVK := cliObj.GetObjectKind().GroupVersionKind()

	vmiSpec.ProviderRef = &common.LocalObjectRef{
		APIVersion: cliGVK.GroupVersion().String(),
		Kind:       cliGVK.Kind,
		Name:       cliObj.GetName(),
	}

	if cliObj.GetNamespace() == "" {
		var (
			cliLabels      = cliObj.GetLabels()
			vmiLabels      = vmiObj.GetLabels()
			labelKeyPrefix = TKGServiceTypeLabelKeyPrefix
		)

		if pkgcfg.FromContext(ctx).Features.TKGMultipleCL {
			labelKeyPrefix = MultipleCLServiceTypeLabelKeyPrefix

			// Remove any labels on the VMI object that have a matching
			// prefix do not also exist on the CLI resource.
			for k := range vmiLabels {
				if strings.HasPrefix(k, labelKeyPrefix) {
					if _, ok := cliLabels[k]; !ok {
						delete(vmiLabels, k)
					}
				}
			}
		}

		// Copy the labels from the CLI object that match the given prefix
		// to the VMI resource.
		for k := range cliLabels {
			if strings.HasPrefix(k, labelKeyPrefix) {
				if vmiLabels == nil {
					vmiLabels = map[string]string{}
				}
				vmiLabels[k] = ""
			}
		}

		vmiObj.SetLabels(vmiLabels)
	}

	vmiStatus.Name = cliStatus.Name
	vmiStatus.ProviderItemID = string(cliSpec.UUID)
	vmiStatus.Type = string(cliStatus.Type)

	return AddContentLibraryRefToAnnotation(
		vmiObj, cliStatus.ContentLibraryRef)
}

// syncImageContent syncs the VirtualMachineImage content from the provider.
// It skips syncing if the image content is already up-to-date.
func (r *Reconciler) syncImageContent(
	ctx context.Context,
	cliObj client.Object,
	cliStatus imgregv1a1.ContentLibraryItemStatus,
	vmiObj client.Object,
	vmiStatus *vmopv1.VirtualMachineImageStatus) error {

	latestVersion := cliStatus.ContentVersion
	if vmiStatus.ProviderContentVersion == latestVersion {
		// Hack: populate Disks fields during version upgrade.
		if len(vmiStatus.Disks) != 0 {
			return nil
		}
	}

	err := r.VMProvider.SyncVirtualMachineImage(ctx, cliObj, vmiObj)
	if err != nil {
		if !pkgerr.WatchVMICacheIfNotReady(err, cliObj) {
			pkgcnd.MarkFalse(
				vmiStatus,
				vmopv1.ReadyConditionType,
				vmopv1.VirtualMachineImageNotSyncedReason,
				"Failed to sync to the latest content version from provider")
		}
	} else {
		vmiStatus.ProviderContentVersion = latestVersion

		// Now that the VMI is synced, the CLI no longer needs to be reconciled
		// when the VMI Cache object is updated.
		labels := cliObj.GetLabels()
		delete(labels, pkgconst.VMICacheLabelKey)
		cliObj.SetLabels(labels)
	}

	// Sync the image's type, OS information and capabilities to the resource's
	// labels to make it easier for clients to search for images based on type,
	// OS info or image capabilities.
	imgutil.SyncStatusToLabels(vmiObj, *vmiStatus)

	r.Recorder.EmitEvent(vmiObj, "Update", err, false)
	return err
}

// GetAppropriateFinalizers returns the finalizers for this type of object.
func GetAppropriateFinalizers(obj client.Object) (string, string) {
	if obj.GetNamespace() != "" {
		return CLItemFinalizer, DeprecatedCLItemFinalizer
	}
	return CCLItemFinalizer, DeprecatedCCLItemFinalizer
}
