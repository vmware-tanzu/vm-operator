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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName = "contentsource.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.ContentSource{}
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
		Owns(&vmopv1alpha1.ContentLibraryProvider{}).
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

// Reconciler reconciles a ContentSource object.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
}

// CreateImages creates a set of VirtualMachineImages in a best effort manner.
func (r *Reconciler) CreateImages(ctx goctx.Context, images []vmopv1alpha1.VirtualMachineImage) error {
	var retErr error
	for _, image := range images {
		img := image
		r.Logger.V(4).Info("Creating VirtualMachineImage", "name", img.Name)
		if err := r.Create(ctx, &img); err != nil {
			if apiErrors.IsAlreadyExists(err) {
				r.Logger.V(4).Info("VirtualMachineImage already exists, generating a new name",
					"name", img.Name, "ContentProvider name", img.Spec.ProviderRef.Name)
				// If this image already exists, it happens when there's a vm image with duplicate name from a different
				// content source. Then we use GenerateName and try to create it again.
				img.Name = ""
				img.GenerateName = img.Status.ImageName + "-"
				err = r.Create(ctx, &img)
			}
			if err != nil {
				retErr = err
				r.Logger.Error(err, "failed to create VirtualMachineImage", "name", img.Name)
				continue
			}
		}

		// Update status sub resource for the VirtualMachineImage.
		if err := r.Status().Update(ctx, &img); err != nil {
			retErr = err
			r.Logger.Error(err, "failed to update status sub resource for image", "name", img.Name)
		}
	}

	return retErr
}

// DeleteImages deletes a set of VirtualMachineImages in a best effort manner.
func (r *Reconciler) DeleteImages(ctx goctx.Context, images []vmopv1alpha1.VirtualMachineImage) error {
	var retErr error
	for _, image := range images {
		img := image
		r.Logger.V(4).Info("Deleting image", "name", img.Name)
		if err := r.Delete(ctx, &img); err != nil {
			retErr = err
			r.Logger.Error(err, "failed to delete VirtualMachineImage", "name", img.Name)
		}
	}

	return retErr
}

// UpdateImages updates a set of VirtualMachineImages in a best effort manner.
func (r *Reconciler) UpdateImages(ctx goctx.Context, images []vmopv1alpha1.VirtualMachineImage) error {
	var retErr error
	for _, image := range images {
		img := image
		r.Logger.V(4).Info("Updating image", "name", img.Name)
		if err := r.Update(ctx, &img); err != nil {
			retErr = err
			r.Logger.Error(err, "failed to update VirtualMachineImage", "name", img.Name)
			continue
		}

		// Update status sub resource for the VirtualMachineImage.
		if err := r.Status().Update(ctx, &img); err != nil {
			retErr = err
			r.Logger.Error(err, "failed to update status sub resource for image", "name", img.Name)
		}
	}

	return retErr
}

// IsImageOwnedByContentLibrary checks whether a VirtualMachineImage is owned by a content library with given UUID.
func IsImageOwnedByContentLibrary(img vmopv1alpha1.VirtualMachineImage, clUUID string) bool {
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
func GetVMImageName(img vmopv1alpha1.VirtualMachineImage) string {
	name := img.Status.ImageName
	// This happens if a vm image is created before duplicate vm image name is supported.
	if name == "" {
		name = img.Name
	}
	return name
}

// DiffImages difference two lists of VirtualMachineImages producing 3 lists: images that have been added
// to "providerImages", images that have been removed in "providerImages", and images that have been updated in "providerImages".
// It only differences VirtualMachineImages that belong to the content library with name clUUID.
func (r *Reconciler) DiffImages(
	clUUID string,
	k8sImages []vmopv1alpha1.VirtualMachineImage,
	providerImages []vmopv1alpha1.VirtualMachineImage) (
	added []vmopv1alpha1.VirtualMachineImage,
	removed []vmopv1alpha1.VirtualMachineImage,
	updated []vmopv1alpha1.VirtualMachineImage) {
	// k8sImagesInCLMap describes k8s images that are in the cluster and belong to the content library with UUID clUUID.
	// providerImagesMap describes images returned by the content library provider whose UUID is clUUID.
	k8sImagesInCLMap := make(map[string]struct{}, len(k8sImages))
	providerImagesMap := make(map[string]int, len(providerImages))

	for i, item := range providerImages {
		providerImagesMap[item.Status.ImageName] = i
	}

	// TODO: Use imageID as the key in the map.
	// In order to support duplicate vm image names in the cluster, we change the name format of an image.
	// We'd like to preserve existing images to avoid any potential problems caused by this changes after upgrade.
	// So we need a common field to match the existing k8s managed images and provider images.
	// It makes more sense to use image ID to identify an image. However, for those images created before
	// duplicate name is supported, .spec.imageID is empty and .status.uuid is deprecated,
	// there are no easy ways to get the image ID. Use image name instead as the key for now.
	// Difference
	for _, image := range k8sImages {
		// Only process the VirtualMachineImage resources that are owned by the content source.
		if !IsImageOwnedByContentLibrary(image, clUUID) {
			continue
		}
		name := GetVMImageName(image)
		k8sImagesInCLMap[name] = struct{}{}

		i, ok := providerImagesMap[name]
		if !ok {
			// Identify removed items
			removed = append(removed, image)
		} else {
			// Image already exists on the API server.
			beforeUpdate := image.DeepCopy()
			// Identify updated items.
			image.Annotations = providerImages[i].Annotations
			image.OwnerReferences = providerImages[i].OwnerReferences
			image.Spec = providerImages[i].Spec
			image.Status = providerImages[i].Status

			if !equality.Semantic.DeepEqual(image, *beforeUpdate) {
				updated = append(updated, image)
			}
		}
	}

	// Identify added items
	for _, i := range providerImages {
		providerImageName := i.Status.ImageName
		if _, ok := k8sImagesInCLMap[providerImageName]; !ok {
			added = append(added, i)
		}
	}

	return added, removed, updated
}

// GetImagesFromContentProvider fetches the VM images from a given content provider. Also sets the owner ref in the images.
func (r *Reconciler) GetImagesFromContentProvider(
	ctx goctx.Context,
	contentSource vmopv1alpha1.ContentSource,
	existingImages []vmopv1alpha1.VirtualMachineImage) ([]vmopv1alpha1.VirtualMachineImage, error) {
	providerRef := contentSource.Spec.ProviderRef

	// Currently, the only supported content provider is content library, so we assume that the providerRef
	// is of ContentLibraryProvider kind.
	clProvider := vmopv1alpha1.ContentLibraryProvider{}
	if err := r.Get(ctx, client.ObjectKey{Name: providerRef.Name, Namespace: providerRef.Namespace}, &clProvider); err != nil {
		return nil, err
	}

	clOwnerRef := metav1.OwnerReference{
		APIVersion: clProvider.APIVersion,
		Kind:       clProvider.Kind,
		Name:       clProvider.Name,
		UID:        clProvider.UID,
	}

	logger := r.Logger.WithValues("clProviderName", clProvider.Name, "clProviderUUID", clProvider.Spec.UUID)
	logger.V(4).Info("listing images from content library")

	currentCLImages := map[string]vmopv1alpha1.VirtualMachineImage{}
	for _, image := range existingImages {
		imageID := image.Spec.ImageID
		if imageID == "" {
			// This occurs during upgrade from not supporting to supporting duplicate vm image names.
			// We need to update all existing VirtualMachineImage objects.
			continue
		}
		// For a vm image missing clOwnerRef, it doesn't matter that IsImageOwnedByContentLibrary returns true.
		// It always has empty imageID, so that it won't reach this line.
		if IsImageOwnedByContentLibrary(image, clProvider.Name) {
			currentCLImages[imageID] = image
		}
	}

	images, err := r.VMProvider.ListVirtualMachineImagesFromContentLibrary(ctx, clProvider, currentCLImages)
	if err != nil {
		logger.Error(err, "error listing images from provider")
		return nil, err
	}

	convertedImages := make([]vmopv1alpha1.VirtualMachineImage, 0)
	for _, img := range images {
		img.OwnerReferences = []metav1.OwnerReference{clOwnerRef}
		img.Spec.ProviderRef = providerRef
		convertedImages = append(convertedImages, *img)
	}

	return convertedImages, nil
}

func (r *Reconciler) DifferenceImages(ctx goctx.Context,
	contentSource *vmopv1alpha1.ContentSource) ([]vmopv1alpha1.VirtualMachineImage, []vmopv1alpha1.VirtualMachineImage, []vmopv1alpha1.VirtualMachineImage, error) {
	r.Logger.V(4).Info("Differencing images")

	// List the existing images from both the vm provider backend and the Kubernetes control plane (etcd).
	// Difference this list and update the k8s control plane to reflect the state of the image inventory from
	// the backend.
	k8sManagedImageList := &vmopv1alpha1.VirtualMachineImageList{}
	if err := r.List(ctx, k8sManagedImageList); err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to list VirtualMachineImages from control plane")
	}

	k8sManagedImages := k8sManagedImageList.Items

	// Best effort to list VirtualMachineImages from this content sources.
	providerManagedImages, err := r.GetImagesFromContentProvider(ctx, *contentSource, k8sManagedImages)
	if err != nil {
		r.Logger.Error(err, "Error listing VirtualMachineImages from the content provider", "contentSourceName", contentSource.Name)
		return nil, nil, nil, err
	}

	// Difference the kubernetes images with the provider images
	added, removed, updated := r.DiffImages(contentSource.Spec.ProviderRef.Name, k8sManagedImages, providerManagedImages)
	r.Logger.V(4).Info("Differenced", "added", added, "removed", removed, "updated", updated)

	return added, removed, updated, nil
}

// SyncImages syncs images from the given content sources.
func (r *Reconciler) SyncImages(ctx goctx.Context, contentSource *vmopv1alpha1.ContentSource) error {
	added, removed, updated, err := r.DifferenceImages(ctx, contentSource)
	if err != nil {
		r.Logger.Error(err, "failed to difference images")
		return err
	}

	// Best effort to sync VirtualMachineImage resources between provider and API server.
	// DeleteImages should be called before CreateImages, in case that removed list and added list have duplicate
	// vm images.
	deleteErr := r.DeleteImages(ctx, removed)
	if deleteErr != nil {
		r.Logger.Error(deleteErr, "failed to delete VirtualMachineImages")
	}

	createErr := r.CreateImages(ctx, added)
	if createErr != nil {
		r.Logger.Error(createErr, "failed to create VirtualMachineImages")
	}

	updateErr := r.UpdateImages(ctx, updated)
	if updateErr != nil {
		r.Logger.Error(updateErr, "failed to update VirtualMachineImages")
	}

	if createErr != nil || updateErr != nil || deleteErr != nil {
		return fmt.Errorf("error syncing VirtualMachineImage resources between provider and API server")
	}

	return nil
}

// ReconcileProviderRef reconciles a ContentSource's provider reference. Verifies that the content provider pointed by
// the provider ref exists on the infrastructure.
func (r *Reconciler) ReconcileProviderRef(ctx goctx.Context, contentSource *vmopv1alpha1.ContentSource) error {
	logger := r.Logger.WithValues("contentSourceName", contentSource.Name)

	logger.Info("Reconciling content provider reference")

	defer func() {
		r.Logger.Info("Finished reconciling content provider reference", "contentSourceName", contentSource.Name)
	}()

	// ProviderRef should always be set.
	providerRef := contentSource.Spec.ProviderRef
	if providerRef.Kind != "ContentLibraryProvider" {
		logger.Info("Unknown provider. Only ContentLibraryProvider is supported", "providerRefName", providerRef.Name, "providerRefKind", providerRef.Kind)
		return nil
	}

	contentLibrary := &vmopv1alpha1.ContentLibraryProvider{}
	if err := r.Get(ctx, client.ObjectKey{Name: providerRef.Name, Namespace: providerRef.Namespace}, contentLibrary); err != nil {
		logger.Error(err, "failed to get ContentLibraryProvider resource", "providerRef", providerRef)
		return err
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
			return err
		}
	}

	return nil
}

// ReconcileDeleteProviderRef reconciles a delete for a provider reference. Currently, no op.
func (r *Reconciler) ReconcileDeleteProviderRef(ctx goctx.Context, contentSource *vmopv1alpha1.ContentSource) error {
	logger := r.Logger.WithValues("contentSourceName", contentSource.Name)

	providerRef := contentSource.Spec.ProviderRef

	logger.V(4).Info("Reconciling delete for a content provider", "contentProviderName", providerRef.Name,
		"contentProviderKind", providerRef.Kind, "contentProviderAPIVersion", providerRef.APIVersion)

	// NoOp. The VirtualMachineImages are automatically deleted because of OwnerReference.

	return nil
}

// ReconcileNormal reconciles a content source. Calls into the provider to reconcile the content provider.
func (r *Reconciler) ReconcileNormal(ctx goctx.Context, contentSource *vmopv1alpha1.ContentSource) error {
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
	if err := r.ReconcileProviderRef(ctx, contentSource); err != nil {
		logger.Error(err, "error in reconciling the provider ref")
		return err
	}

	if err := r.SyncImages(ctx, contentSource); err != nil {
		logger.Error(err, "Error in syncing image from the content provider")
		return err
	}

	logger.Info("Finished reconciling ContentSource")
	return nil
}

// ReconcileDelete reconciles a content source delete. We use a finalizer here to clean up any state if needed.
func (r *Reconciler) ReconcileDelete(ctx goctx.Context, contentSource *vmopv1alpha1.ContentSource) error {
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

func (r *Reconciler) Reconcile(ctx goctx.Context, request ctrl.Request) (ctrl.Result, error) {
	r.Logger.Info("Received reconcile request", "name", request.Name)

	instance := &vmopv1alpha1.ContentSource{}
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
