// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName = "contentsource.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.ContentSource{}
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

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Owns(&vmopv1.ContentLibraryProvider{}).
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
		CSMetrics:  metrics.NewContentSourceMetrics(),
	}
}

// Reconciler reconciles a ContentSource object.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
	CSMetrics  *metrics.ContentSourceMetrics
}

// CreateImage creates thr VirtualMachineImage. If a VirtualMachineImage with the same name alreay exists,
// use GenerateName to give this image a new name.
func (r *Reconciler) CreateImage(ctx goctx.Context, image vmopv1.VirtualMachineImage) error {
	// Preserve the status information. This is due to a change in
	// controller-runtime's change to Create/Update calls that nils out
	// empty fields. The copy here is to reintroduce the status later when
	// performing the Update call below. This is because the call a few
	// lines from here, r.Create(ctx, &img), will cause img.Status to
	// be unset.
	imgStatus := image.Status.DeepCopy()

	r.Logger.V(4).Info("Creating VirtualMachineImage", "name", image.Name)
	if err := r.Create(ctx, &image); err != nil {
		if apiErrors.IsAlreadyExists(err) {
			r.Logger.V(4).Info("VirtualMachineImage already exists, generating a new name",
				"name", image.Name, "ContentProvider name", image.Spec.ProviderRef.Name)
			// If this image already exists, it happens when there's a vm image with duplicate name from a different
			// content source. Then we use GenerateName and try to create it again.
			image.Name = ""
			image.GenerateName = image.Status.ImageName + "-"
			err = r.Create(ctx, &image)
		}
		if err != nil {
			r.Logger.Error(err, "failed to create VirtualMachineImage", "name", image.Name)
			return err
		}
	}

	// Update status sub resource for the VirtualMachineImage.
	// Reintroduce the status from imgWithStatus.
	imgStatus.DeepCopyInto(&image.Status)
	if err := r.Status().Update(ctx, &image); err != nil {
		r.Logger.Error(err, "failed to update status sub resource for image", "name", image.Name)
		return err
	}

	return nil
}

// DeleteImage deletes a VirtualMachineImage. Ignore the NotFound error.
func (r *Reconciler) DeleteImage(ctx goctx.Context, image vmopv1.VirtualMachineImage) error {
	r.Logger.V(4).Info("Deleting image", "name", image.Name)
	if err := client.IgnoreNotFound(r.Delete(ctx, &image)); err != nil {
		r.Logger.Error(err, "failed to delete VirtualMachineImage", "name", image.Name)
		return err
	}

	return nil
}

// UpdateImage checks a VirtualMachineImage non-status and status resource and update it if needed.
func (r *Reconciler) UpdateImage(ctx goctx.Context, currentImage, expectedImage vmopv1.VirtualMachineImage) error {
	r.Logger.V(4).Info("Updating image", "name", expectedImage.Name)

	beforeUpdate := currentImage.DeepCopy()
	currentImage.Annotations = expectedImage.Annotations
	currentImage.OwnerReferences = expectedImage.OwnerReferences
	currentImage.Spec = expectedImage.Spec

	// Check VirtualMachineImage non-status.
	if !equality.Semantic.DeepEqual(currentImage.ObjectMeta, beforeUpdate.ObjectMeta) ||
		!equality.Semantic.DeepEqual(currentImage.Spec, beforeUpdate.Spec) {
		if err := r.Update(ctx, &currentImage); err != nil {
			r.Logger.Error(err, "failed to update VirtualMachineImage", "name", expectedImage.Name)
			return err
		}
	}

	// Check status sub resource.
	currentImage.Status = expectedImage.Status
	if !equality.Semantic.DeepEqual(currentImage.Status, beforeUpdate.Status) {
		// Update status sub resource for the VirtualMachineImage.
		if err := r.Status().Update(ctx, &currentImage); err != nil {
			r.Logger.Error(err, "failed to update status sub resource for image", "name", expectedImage.Name)
			return err
		}
	}

	return nil
}

// IsImageOwnedByContentLibrary checks whether a VirtualMachineImage is owned by a content library with given UUID.
func IsImageOwnedByContentLibrary(img vmopv1.VirtualMachineImage, clUUID string) bool {
	// If an image has empty OwnerRefs, then this image is created before VM Service.
	// This case only happens during upgrade from Non-VM service to Non-VM service, or
	// Non-VM service to VM service.
	// In this case, we only have one content library before VM Service,
	// so it's safe to think this image is owned by the given content library.
	emptyCLOwnerRef := true
	for _, o := range img.OwnerReferences {
		if o.Kind == "ContentLibraryProvider" {
			emptyCLOwnerRef = false
			if o.Name == clUUID {
				return true
			}
		}
	}
	return emptyCLOwnerRef
}

// GetVMImageName returns the display name of the image defined in the template.
// Note that this is different from the name of the VirtualMachineImage Kubernetes object.
func GetVMImageName(img vmopv1.VirtualMachineImage) string {
	name := img.Status.ImageName
	// This happens if a vm image is created before duplicate vm image name is supported.
	if name == "" {
		name = img.Name
	}
	return name
}

func (r *Reconciler) ProcessItemFromContentLibrary(ctx goctx.Context,
	logger logr.Logger,
	clProvider *vmopv1.ContentLibraryProvider,
	itemID string, currentCLImages map[string]vmopv1.VirtualMachineImage) (reterr error) {
	logger.V(4).Info("Processing image item", "itemID", itemID)

	providerImage, err := r.VMProvider.GetVirtualMachineImageFromContentLibrary(ctx, clProvider, itemID, currentCLImages)
	if err != nil {
		logger.Error(err, "failed to get VirtualMachineImage from content library")
		return err
	}

	if providerImage != nil {
		clOwnerRef := metav1.OwnerReference{
			APIVersion: clProvider.APIVersion,
			Kind:       clProvider.Kind,
			Name:       clProvider.Name,
			UID:        clProvider.UID,
		}
		providerImage.OwnerReferences = []metav1.OwnerReference{clOwnerRef}
		providerImage.Spec.ProviderRef = vmopv1.ContentProviderReference{
			APIVersion: clProvider.APIVersion,
			Kind:       clProvider.Kind,
			Name:       clProvider.Name,
			Namespace:  clProvider.Namespace,
		}

		defer func() {
			r.CSMetrics.RegisterVMImageCreateOrUpdate(logger, *providerImage, reterr == nil)
		}()

		if currentImage, ok := currentCLImages[providerImage.Status.ImageName]; !ok {
			// Create this VM Image
			if err := r.CreateImage(ctx, *providerImage); err != nil {
				return err
			}
		} else {
			// After processing all the items from the CL, we delete all remaining images in this map.
			// Remove from the currentCLImages to avoid deletion.
			delete(currentCLImages, providerImage.Status.ImageName)

			// Image already exists on the API server. Update it.
			if err := r.UpdateImage(ctx, currentImage, *providerImage); err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncImagesFromContentProvider fetches the VM images from a given content provider. Also sets the owner ref in the images.
func (r *Reconciler) SyncImagesFromContentProvider(
	ctx goctx.Context, clProvider *vmopv1.ContentLibraryProvider) error {
	logger := r.Logger.WithValues("clProviderName", clProvider.Name, "clProviderUUID", clProvider.Spec.UUID)
	logger.V(4).Info("listing images from content library")

	// List the existing images from the supervisor cluster.
	k8sManagedImageList := &vmopv1.VirtualMachineImageList{}
	if err := r.List(ctx, k8sManagedImageList); err != nil {
		return errors.Wrap(err, "failed to list VirtualMachineImages from control plane")
	}

	k8sManagedImages := k8sManagedImageList.Items

	// Use ImageName as te key instead of ImageID.
	// In order to support duplicate vm image names in the cluster, we change the name format of an image.
	// We'd like to preserve existing images to avoid any potential problems caused by this changes after upgrade.
	// So we need a common field to match the existing k8s managed images and provider images.
	// Also, use imageName as the key can help us identify a VMImage which has been renamed.
	currentCLImages := map[string]vmopv1.VirtualMachineImage{}
	for _, image := range k8sManagedImages {
		// Only process the VirtualMachineImage resources that are owned by the content source.
		if !IsImageOwnedByContentLibrary(image, clProvider.Name) {
			continue
		}

		imageName := GetVMImageName(image)
		currentCLImages[imageName] = image
	}

	libItemList, err := r.VMProvider.ListItemsFromContentLibrary(ctx, clProvider)
	if err != nil {
		return err
	}

	retErrs := make([]error, 0)
	for _, item := range libItemList {
		err := r.ProcessItemFromContentLibrary(ctx, logger, clProvider, item, currentCLImages)
		if err != nil {
			retErrs = append(retErrs, err)
			continue
		}
	}

	if len(retErrs) > 0 {
		// For now, assume they are transient errors and let the controller reconcile it.
		// Don't delete Images here. Otherwise, for VM images with duplicate names, they will have new generated names
		// once the transient errors are gone.
		return k8serrors.NewAggregate(retErrs)
	}

	// Remaining images in the currentCLImages map were deleted from the CL.
	// Delete them from the cluster.
	for _, currentImage := range currentCLImages {
		err := r.DeleteImage(ctx, currentImage)
		if err != nil {
			retErrs = append(retErrs, err)
		}
		r.CSMetrics.RegisterVMImageDelete(logger, currentImage, err == nil)
	}

	return k8serrors.NewAggregate(retErrs)
}

// ReconcileProviderRef reconciles a ContentSource's provider reference. Verifies that the content provider pointed by
// the provider ref exists on the infrastructure.
func (r *Reconciler) ReconcileProviderRef(ctx goctx.Context,
	contentSource *vmopv1.ContentSource) (*vmopv1.ContentLibraryProvider, error) {
	logger := r.Logger.WithValues("contentSourceName", contentSource.Name)

	logger.Info("Reconciling content provider reference")

	defer func() {
		r.Logger.Info("Finished reconciling content provider reference", "contentSourceName", contentSource.Name)
	}()

	// ProviderRef should always be set.
	providerRef := contentSource.Spec.ProviderRef
	if providerRef.Kind != "ContentLibraryProvider" {
		logger.Info("Unknown provider. Only ContentLibraryProvider is supported", "providerRefName", providerRef.Name, "providerRefKind", providerRef.Kind)
		return nil, nil
	}

	contentLibrary := &vmopv1.ContentLibraryProvider{}
	if err := r.Get(ctx, client.ObjectKey{Name: providerRef.Name, Namespace: providerRef.Namespace}, contentLibrary); err != nil {
		logger.Error(err, "failed to get ContentLibraryProvider resource", "providerRef", providerRef)
		return nil, err
	}

	logger = logger.WithValues("contentLibraryName", contentLibrary.Name, "contentLibraryUUID", contentLibrary.Spec.UUID)
	logger.V(4).Info("ContentLibraryProvider backing the ContentSource found")

	beforeObj := contentLibrary.DeepCopy()

	isController := true
	// Set an ownerRef to the ContentSource
	ownerRef := metav1.OwnerReference{
		APIVersion: contentSource.APIVersion,
		Kind:       contentSource.Kind,
		Name:       contentSource.Name,
		UID:        contentSource.UID,
		Controller: &isController,
	}

	contentLibrary.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	if !equality.Semantic.DeepEqual(beforeObj, contentLibrary) {
		if err := r.Update(ctx, contentLibrary); err != nil {
			logger.Error(err, "error updating the ContentLibraryProvider")
			return nil, err
		}
	}

	return contentLibrary, nil
}

// ReconcileDeleteProviderRef reconciles a delete for a provider reference. Currently, no op.
func (r *Reconciler) ReconcileDeleteProviderRef(ctx goctx.Context, contentSource *vmopv1.ContentSource) error {
	logger := r.Logger.WithValues("contentSourceName", contentSource.Name)

	providerRef := contentSource.Spec.ProviderRef

	logger.V(4).Info("Reconciling delete for a content provider", "contentProviderName", providerRef.Name,
		"contentProviderKind", providerRef.Kind, "contentProviderAPIVersion", providerRef.APIVersion)

	r.CSMetrics.DeleteMetrics(logger, providerRef)

	// NoOp. The VirtualMachineImages are automatically deleted because of OwnerReference.

	return nil
}

// ReconcileNormal reconciles a content source. Calls into the provider to reconcile the content provider.
func (r *Reconciler) ReconcileNormal(ctx goctx.Context, contentSource *vmopv1.ContentSource) error {
	logger := r.Logger.WithValues("name", contentSource.Name)

	logger.Info("Reconciling ContentSource CreateOrUpdate")

	if !controllerutil.ContainsFinalizer(contentSource, finalizerName) {
		controllerutil.AddFinalizer(contentSource, finalizerName)
		if err := r.Update(ctx, contentSource); err != nil {
			return err
		}
	}

	// TODO: If a ContentSource is deleted before we can add an OwnerReference to the ContentLibraryProvider,
	// we will have an orphan ContentLibraryProvider resource in the cluster.
	clProvider, err := r.ReconcileProviderRef(ctx, contentSource)
	if err != nil {
		logger.Error(err, "error in reconciling the provider ref")
		return err
	}

	// Currently, the only supported content provider is content library, so we assume that the providerRef
	// is of ContentLibraryProvider kind.
	if err := r.SyncImagesFromContentProvider(ctx, clProvider); err != nil {
		logger.Error(err, "Error in syncing image from the content provider")
		r.Recorder.EmitEvent(clProvider, "SyncImages", err, true)
		return err
	}

	logger.Info("Finished reconciling ContentSource")
	return nil
}

// ReconcileDelete reconciles a content source delete. We use a finalizer here to clean up any state if needed.
func (r *Reconciler) ReconcileDelete(ctx goctx.Context, contentSource *vmopv1.ContentSource) error {
	logger := r.Logger.WithValues("name", contentSource.Name)

	defer func() {
		logger.Info("Finished Reconciling ContentSource Deletion")
	}()

	logger.Info("Reconciling ContentSource deletion")

	if controllerutil.ContainsFinalizer(contentSource, finalizerName) {
		if err := r.ReconcileDeleteProviderRef(ctx, contentSource); err != nil {
			logger.Error(err, "error when reconciling the provider ref")
			return err
		}

		controllerutil.RemoveFinalizer(contentSource, finalizerName)
		if err := r.Update(ctx, contentSource); err != nil {
			return err
		}
	}

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsourcebindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, request ctrl.Request) (ctrl.Result, error) {
	r.Logger.Info("Received reconcile request", "name", request.Name)

	instance := &vmopv1.ContentSource{}
	if err := r.Get(ctx, request.NamespacedName, instance); err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		err := r.ReconcileDelete(ctx, instance)
		return ctrl.Result{}, err
	}

	if err := r.ReconcileNormal(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
