// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource

import (
	goCtx "context"
	"fmt"
	"reflect"
	"sort"
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
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
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
		ctx.VmProvider,
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
	vmProvider vmprovider.VirtualMachineProviderInterface) *ContentSourceReconciler {

	return &ContentSourceReconciler{
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VmProvider: vmProvider,
	}
}

// ContentSourceReconciler reconciles a ContentSource object
type ContentSourceReconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VmProvider vmprovider.VirtualMachineProviderInterface
}

// Best effort attempt to create a set of VirtualMachineImages resources on the API server.
func (r *ContentSourceReconciler) CreateImages(ctx goCtx.Context, images []vmopv1alpha1.VirtualMachineImage) error {
	var retErr error
	for _, image := range images {
		img := image
		r.Logger.V(4).Info("Creating VirtualMachineImage", "name", img.Name)
		if err := r.Create(ctx, &img); err != nil {

			// Ignore VirtualMachineImage if it already exists. This can happen if we have a duplicate image name in the
			// content library. We sort the ContentSources by the oldest to latest one, so the VirtualMachineImage from
			// the first library is not overwritten.
			if !apiErrors.IsAlreadyExists(err) {
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

// Best effort attempt to delete a set of VirtualMachineImages resources from the API server.
func (r *ContentSourceReconciler) DeleteImages(ctx goCtx.Context, images []vmopv1alpha1.VirtualMachineImage) error {
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

// Best effort attempt to update a set of VirtualMachineImages resources on the API server.
func (r *ContentSourceReconciler) UpdateImages(ctx goCtx.Context, images []vmopv1alpha1.VirtualMachineImage) error {
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

// GetContentLibraryFromOwnerRefs returns the ContentLibraryProvider name from the list of OwnerRefs.
func GetContentLibraryNameFromOwnerRefs(ownerRefs []metav1.OwnerReference) string {
	for _, o := range ownerRefs {
		if o.Kind == "ContentLibraryProvider" {
			return o.Name
		}
	}
	return ""
}

// Difference two lists of VirtualMachineImages producing 3 lists: images that have been added to "right", images that
// have been removed in "right", and images that have been updated in "right".
func (r *ContentSourceReconciler) DiffImages(left []vmopv1alpha1.VirtualMachineImage, right []vmopv1alpha1.VirtualMachineImage) (
	added []vmopv1alpha1.VirtualMachineImage,
	removed []vmopv1alpha1.VirtualMachineImage,
	updated []vmopv1alpha1.VirtualMachineImage) {

	leftMap := make(map[string]int, len(left))
	rightMap := make(map[string]int, len(right))

	for i, item := range left {
		leftMap[item.Name] = i
	}

	for i, item := range right {
		rightMap[item.Name] = i
	}

	// Difference
	for _, l := range left {
		i, ok := rightMap[l.Name]
		if !ok {
			// Identify removed items
			removed = append(removed, l)
		} else {
			// Image already exists on the API server.
			// Two scenarios here:
			// - Image updated on the provider in the same library.
			// - Image with the same name uploaded in a different library.
			// Since the OwnerRef points to the content library, use that to decide whether it is a duplicate image from another content library.
			leftCL := GetContentLibraryNameFromOwnerRefs(l.OwnerReferences)
			rightCL := GetContentLibraryNameFromOwnerRefs(right[i].OwnerReferences)
			// Images that were created before 7.0 U2 will not have the OwnerReference. We update those so the OwnerReference is added.
			// The empty string check will only matter for non VM Service to VM Service, or non VM Service to non VMService upgrades.
			// Thus, we do not have to worry about the scenario where there can be same images in different libraries (since we will
			// only have one CL before VM Service).
			if leftCL != "" && leftCL != rightCL {
				// Should this be an event?
				r.Logger.Error(nil, "A VirtualMachineImage by this name has already been created from another content library",
					"imageName", right[i].Name, "syncingFromContentLibrary", rightCL, "existsInContentLibrary", leftCL)
				continue
			}

			beforeUpdate := l.DeepCopy()
			// Identify updated items. We only care about OwnerReference and Spec update.
			l.Annotations = right[i].Annotations
			l.OwnerReferences = right[i].OwnerReferences
			l.Spec = right[i].Spec
			l.Status = right[i].Status

			if !equality.Semantic.DeepEqual(l, *beforeUpdate) {
				updated = append(updated, l)
			}
		}
	}

	// Identify added items
	for _, ri := range right {
		if _, ok := leftMap[ri.Name]; !ok {
			added = append(added, ri)
		}
	}

	return added, removed, updated
}

// GetContentProviderManagedImages fetches the VM images from a given content provider. Also sets the owner ref in the images.
func (r *ContentSourceReconciler) GetImagesFromContentProvider(ctx goCtx.Context,
	contentSource vmopv1alpha1.ContentSource) ([]*vmopv1alpha1.VirtualMachineImage, error) {
	providerRef := contentSource.Spec.ProviderRef

	// Currently, the only supported content provider is content library, so we assume that the providerRef is of ContentLibraryProvider kind.
	clProvider := vmopv1alpha1.ContentLibraryProvider{}
	if err := r.Get(ctx, client.ObjectKey{Name: providerRef.Name, Namespace: providerRef.Namespace}, &clProvider); err != nil {
		return nil, err
	}
	r.Logger.V(4).Info("listing images from content library", "clProviderName", clProvider.Name, "clProviderUUID", clProvider.Spec.UUID)

	images, err := r.VmProvider.ListVirtualMachineImagesFromContentLibrary(ctx, clProvider)
	if err != nil {
		r.Logger.Error(err, "error listing images from provider", "contentLibraryUUID", clProvider.Spec.UUID)
		return nil, err
	}

	for _, img := range images {
		img.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: clProvider.APIVersion,
			Kind:       clProvider.Kind,
			Name:       clProvider.Name,
			UID:        clProvider.UID,
		}}
	}

	return images, nil
}

func (r *ContentSourceReconciler) DifferenceImages(ctx goCtx.Context) (error, []vmopv1alpha1.VirtualMachineImage, []vmopv1alpha1.VirtualMachineImage, []vmopv1alpha1.VirtualMachineImage) {
	r.Logger.V(4).Info("Differencing images")

	// List the existing images from both the vm provider backend and the Kubernetes control plane (etcd).
	// Difference this list and update the k8s control plane to reflect the state of the image inventory from
	// the backend.
	k8sManagedImageList := &vmopv1alpha1.VirtualMachineImageList{}
	if err := r.List(ctx, k8sManagedImageList); err != nil {
		return errors.Wrap(err, "failed to list VirtualMachineImages from control plane"), nil, nil, nil
	}

	k8sManagedImages := k8sManagedImageList.Items

	// List the cluster-scoped VirtualMachineImages from the content sources configured by the ContentSource resources.
	contentSourceList := &vmopv1alpha1.ContentSourceList{}
	if err := r.List(ctx, contentSourceList); err != nil {
		return errors.Wrap(err, "failed to list ContentSources from control plane"), nil, nil, nil
	}

	// Sort the ContentSources by latest to oldest CreationTimestamp so the VirtualMachineImage from the oldest ContentSource takes precedence.
	sort.Sort(SortableContentSources(contentSourceList.Items))

	// Best effort to list VirtualMachineImages from all content sources.
	var providerManagedImages []*vmopv1alpha1.VirtualMachineImage
	for _, contentSource := range contentSourceList.Items {
		images, err := r.GetImagesFromContentProvider(ctx, contentSource)
		if err != nil {
			if lib.IsNotFoundError(err) {
				r.Logger.Error(err, "content library not found on provider", "contentSourceName", contentSource.Name)
				continue
			}
			r.Logger.Error(err, "Error listing VirtualMachineImages from the content provider", "contentSourceName", contentSource.Name)
			return err, nil, nil, nil
		}

		providerManagedImages = append(providerManagedImages, images...)
	}

	var convertedImages []vmopv1alpha1.VirtualMachineImage
	for _, img := range providerManagedImages {
		convertedImages = append(convertedImages, *img)
	}

	// Difference the kubernetes images with the provider images
	added, removed, updated := r.DiffImages(k8sManagedImages, convertedImages)
	r.Logger.V(4).Info("Differenced", "added", added, "removed", removed, "updated", updated)

	return nil, added, removed, updated
}

// SyncImages syncs images from all the content sources installed.
func (r *ContentSourceReconciler) SyncImages(ctx goCtx.Context) error {
	err, added, removed, updated := r.DifferenceImages(ctx)
	if err != nil {
		r.Logger.Error(err, "failed to difference images")
		return err
	}

	// Best effort to sync VirtualMachineImage resources between provider and API server.
	createErr := r.CreateImages(ctx, added)
	if createErr != nil {
		r.Logger.Error(createErr, "failed to create VirtualMachineImages")
	}

	updateErr := r.UpdateImages(ctx, updated)
	if updateErr != nil {
		r.Logger.Error(updateErr, "failed to update VirtualMachineImages")
	}

	deleteErr := r.DeleteImages(ctx, removed)
	if deleteErr != nil {
		r.Logger.Error(deleteErr, "failed to delete VirtualMachineImages")
	}

	if createErr != nil || updateErr != nil || deleteErr != nil {
		return fmt.Errorf("Error in syncing VirtualMachineImage resources between provider and API server")
	}

	return nil
}

// GetContentProvider fetches the ContentLibraryProvider object from the API server.
func GetContentProvider(ctx goCtx.Context, c client.Client, ref vmopv1alpha1.ContentProviderReference) (*vmopv1alpha1.ContentLibraryProvider, error) {
	contentLibrary := &vmopv1alpha1.ContentLibraryProvider{}
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, contentLibrary); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve object from API server. kind: %v, namespace: %v, name: %v",
			ref.Kind, ref.Namespace, ref.Name)
	}

	return contentLibrary, nil
}

// ReconcileProviderRef reconciles a ContentSource's provider reference. Verifies that the content provider pointed by
// the provider ref exists on the infrastructure.
func (r *ContentSourceReconciler) ReconcileProviderRef(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) error {
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
		logger.Error(err, "failed to get ContentLibraryProvider resource", "providerRefName", providerRef.Name, "providerNamespace", providerRef.Namespace, "providerRefKind", providerRef.Kind)
		return err
	}

	logger = logger.WithValues("contentLibraryName", contentLibrary.Name, "contentLibraryUUID", contentLibrary.Spec.UUID)

	logger.V(4).Info("ContentLibraryProvider backing the ContentSource found")

	beforeObj := contentLibrary.DeepCopy()

	isController := true
	// Set an ownerref to the ContentSource
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
func (r *ContentSourceReconciler) ReconcileDeleteProviderRef(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) error {
	logger := r.Logger.WithValues("contentSourceName", contentSource.Name)

	providerRef := contentSource.Spec.ProviderRef

	logger.V(4).Info("Reconciling delete for a content provider", "contentProviderName", providerRef.Name,
		"contentProviderKind", providerRef.Kind, "contentProviderAPIVersion", providerRef.APIVersion)

	// NoOp. The VirtualMachineImages are automatically deleted because of OwnerReference.

	return nil
}

// ReconcileNormal reconciles a content source. Calls into the provider to reconcile the content provider.
func (r *ContentSourceReconciler) ReconcileNormal(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) error {
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

	if err := r.SyncImages(ctx); err != nil {
		logger.Error(err, "Error in syncing image from the content provider")
		return err
	}

	logger.Info("Finished reconciling ContentSource")
	return nil
}

// ReconcileDelete reconciles a content source delete. We use a finalizer here to clean up any state if needed.
func (r *ContentSourceReconciler) ReconcileDelete(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) error {
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

func (r *ContentSourceReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	r.Logger.Info("Received reconcile request", "name", request.Name)

	ctx := goCtx.Background()
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
