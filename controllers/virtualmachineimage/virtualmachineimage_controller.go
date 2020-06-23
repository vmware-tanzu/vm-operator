// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage

import (
	goctx "context"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	ControllerName = "virtualmachineimage-controller"

	// Set an initial timer to aggressively discover images until there is a single successful complete sync.
	InitialImageDiscoveryFrequency = 1 * time.Minute

	// Set this timer pop sufficiently long to avoid aggressively loading VC with content discovery tasks
	// This 10 minute timeout tradeoffs discovery responsiveness for VC loading and task history pollution
	// The initial sync of content on startup will ensure that images are eagerly synced
	ContinuousImageDiscoveryFrequency = 10 * time.Minute
)

type VirtualMachineImageDiscovererOptions struct {
	initialDiscoveryFrequency    time.Duration
	continuousDiscoveryFrequency time.Duration
}

type VirtualMachineImageDiscoverer struct {
	client     client.Client
	log        logr.Logger
	vmProvider vmprovider.VirtualMachineProviderInterface
	options    VirtualMachineImageDiscovererOptions
}

func NewVirtualMachineImageDiscoverer(
	ctx *context.ControllerManagerContext,
	client client.Client,
	options VirtualMachineImageDiscovererOptions) *VirtualMachineImageDiscoverer {

	return &VirtualMachineImageDiscoverer{
		client:     client,
		log:        ctx.Logger.WithName("VirtualMachineImageDiscover"),
		vmProvider: ctx.VmProvider,
		options:    options,
	}
}

// Attempt to create a set of images.
func (d *VirtualMachineImageDiscoverer) createImages(ctx goctx.Context, images []vmoperatorv1alpha1.VirtualMachineImage) error {

	for _, image := range images {
		img := image
		d.log.V(4).Info("Creating image", "name", img.Name)
		err := d.client.Create(ctx, &img)
		if err != nil {
			d.log.V(5).Info("failed to create image", "name", img.Name, "error", err)
		}
	}

	return nil
}

// Attempt to delete a set of images.  Fail fast on first failure
func (d *VirtualMachineImageDiscoverer) deleteImages(ctx goctx.Context, images []vmoperatorv1alpha1.VirtualMachineImage) error {

	for _, image := range images {
		img := image
		d.log.V(4).Info("Deleting image", "name", img.Name)
		err := d.client.Delete(ctx, &img)
		if err != nil {
			return errors.Wrapf(err, "failed to delete image %s", img.Name)
		}
	}

	return nil
}

// Difference two lists of VirtualMachineImages producing 3 lists: images that have been added to "right", images that
// have been removed in "right", and images that have been updated in "right".
func (d *VirtualMachineImageDiscoverer) diffImages(left []vmoperatorv1alpha1.VirtualMachineImage, right []vmoperatorv1alpha1.VirtualMachineImage) (
	added []vmoperatorv1alpha1.VirtualMachineImage,
	removed []vmoperatorv1alpha1.VirtualMachineImage,
	updated []vmoperatorv1alpha1.VirtualMachineImage) {

	leftMap := make(map[string]bool)
	rightMap := make(map[string]bool)

	for _, item := range left {
		leftMap[item.Name] = true
	}

	for _, item := range right {
		rightMap[item.Name] = true
	}

	// Difference
	for _, l := range left {
		_, ok := rightMap[l.Name]
		if !ok {
			// Identify removed items
			d.log.V(4).Info("Removing Image", "name", l.Name)
			removed = append(removed, l)
		} else {
			// Note, this adds every existing image to the updated list without actually performing a deep comparison.
			d.log.V(4).Info("Updating Image", "name", l.Name)
			updated = append(updated, l)
		}
	}

	// Identify added items
	for _, r := range right {
		if _, ok := leftMap[r.Name]; !ok {
			d.log.V(4).Info("Adding Image", "name", r.Name)
			added = append(added, r)
		}
	}

	return added, removed, updated
}

// getContentProviderManagedImages fetches the VM images from a given content provider
func (d *VirtualMachineImageDiscoverer) getImagesFromContentProvider(ctx goctx.Context,
	contentSource vmoperatorv1alpha1.ContentSource) ([]*vmoperatorv1alpha1.VirtualMachineImage, error) {
	providerRef := contentSource.Spec.ProviderRef

	// Currently, the only supported content provider is content library, so we directly fetch the object in a ContentLibraryProvider resource.
	// Once we support multiple content providers, this will be modified to fetch the resource based on the provider ref kind.
	contentLibrary := vmoperatorv1alpha1.ContentLibraryProvider{}
	if err := d.client.Get(ctx, types.NamespacedName{Name: providerRef.Name, Namespace: providerRef.Namespace}, &contentLibrary); err != nil {
		return nil, err
	}
	d.log.V(4).Info("listing images from content library", "contentLibraryName", contentLibrary.Name, "contentLibraryUUID", contentLibrary.Spec.UUID)

	return d.vmProvider.ListVirtualMachineImagesFromContentLibrary(ctx, contentLibrary)
}

func (d *VirtualMachineImageDiscoverer) differenceImages(ctx goctx.Context) (error, []vmoperatorv1alpha1.VirtualMachineImage, []vmoperatorv1alpha1.VirtualMachineImage) {
	d.log.V(4).Info("Differencing images")

	// List the existing images from both the vm provider backend and the Kubernetes control plane (etcd).
	// Difference this list and update the k8s control plane to reflect the state of the image inventory from
	// the backend.
	k8sManagedImageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
	err := d.client.List(ctx, k8sManagedImageList)
	if err != nil {
		return errors.Wrap(err, "failed to list images from control plane"), nil, nil
	}

	k8sManagedImages := k8sManagedImageList.Items

	// List the cluster-scoped VirtualMachineImages
	// If we have ContentSource custom resource available
	//  - calls into the content provider to list VM images from each ContentSourceProvider
	// else
	//  - calls into the provider to list VM images from the content source configured by the ConfigMap.

	var providerManagedImages []*vmoperatorv1alpha1.VirtualMachineImage
	if os.Getenv("FSS_WCP_VMSERVICE") == "true" {
		contentSourceList := &vmoperatorv1alpha1.ContentSourceList{}
		err = d.client.List(ctx, contentSourceList)
		if err != nil {
			return errors.Wrap(err, "failed to list content sources from control plane"), nil, nil
		}

		for _, contentSource := range contentSourceList.Items {
			images, err := d.getImagesFromContentProvider(ctx, contentSource)
			if err != nil {
				return nil, nil, nil
			}

			providerManagedImages = append(providerManagedImages, images...)
		}
	} else {
		providerManagedImages, err = d.vmProvider.ListVirtualMachineImages(ctx, "")
		if err != nil {
			// If the VM provider cannot reach the configured content library (e.g. when a library is deleted), it returns http
			// not found error. We log and return empty image list here so the existing VM image templates can be cleaned up from the API server.

			if !lib.IsNotFoundError(err) {
				return errors.Wrap(err, "failed to list images from backend"), nil, nil
			}

			d.log.Error(err, "VM provider failed to find the Content Library")
		}

	}

	var convertedImages []vmoperatorv1alpha1.VirtualMachineImage
	for _, image := range providerManagedImages {
		img := image
		convertedImages = append(convertedImages, *img)
	}

	// Difference the kubernetes images with the provider images
	// TODO: Ignore updated for now
	added, removed, _ := d.diffImages(k8sManagedImages, convertedImages)
	d.log.V(4).Info("Differenced", "added", added, "removed", removed)

	return nil, added, removed
}

func (d *VirtualMachineImageDiscoverer) SyncImages() error {
	ctx := goctx.Background()
	err, added, removed := d.differenceImages(ctx)
	if err != nil {
		d.log.Error(err, "failed to difference images")
		return err
	}

	err = d.createImages(ctx, added)
	if err != nil {
		d.log.Error(err, "failed to create all images")
		return err
	}

	err = d.deleteImages(ctx, removed)
	if err != nil {
		d.log.Error(err, "failed to delete all images")
		return err
	}

	return nil
}

// Blocking function to drive image discovery and differencing
func (d *VirtualMachineImageDiscoverer) Start(stopChan <-chan struct{}, doneChan chan struct{}) {
	d.log.Info("Starting VirtualMachineImageDiscoverer",
		"initial discovery frequency", d.options.initialDiscoveryFrequency,
		"continuous discovery frequency", d.options.continuousDiscoveryFrequency)

	// Drive "aggressive" discovery until there is a fully successful initial sync of the images.  We do this so
	// that there is some initial content seeded into the k8s control plane.
	d.log.Info("Performing an initial image discovery")
	initSyncDone := make(chan bool)
	ticker := time.NewTicker(d.options.initialDiscoveryFrequency)
	go func() {
		for {
			select {
			case <-stopChan:
				d.log.Info("Stop channel event")
				ticker.Stop()
				close(initSyncDone)
				return
			case t := <-ticker.C:
				d.log.V(4).Info("Tick at", "time", t)
				err := d.SyncImages()

				// If no error was received, assume a successful sync and move to a continuous discovery frequency
				if err == nil {
					ticker.Stop()
					close(initSyncDone)
					return
				}
			}
		}
	}()

	// Block until the initial discovery is complete
	<-initSyncDone
	d.log.Info("Initial image discovery completed successfully. Graduating to the continuous image discovery phase.")

	// Shift to a continuous discovery frequency (presumably at a longer interval) in order to discovery new content
	// published on the order of days.
	ticker = time.NewTicker(d.options.continuousDiscoveryFrequency)
	go func() {
		for {
			select {
			case <-stopChan:
				d.log.Info("Stop channel event")
				ticker.Stop()
				close(doneChan)
				return
			case t := <-ticker.C:
				d.log.V(4).Info("Tick at", "time", t)
				_ = d.SyncImages()
			}
		}
	}()
}

func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	options := VirtualMachineImageDiscovererOptions{
		initialDiscoveryFrequency:    InitialImageDiscoveryFrequency,
		continuousDiscoveryFrequency: ContinuousImageDiscoveryFrequency,
	}
	return AddWithOptions(ctx, mgr, options)
}

func AddWithOptions(ctx *context.ControllerManagerContext, mgr manager.Manager, options VirtualMachineImageDiscovererOptions) error {
	r := newReconciler(ctx, mgr, options)

	// Add the reconciler explicitly as runnable in order to receive a Start() event
	err := mgr.Add(r.(*ReconcileVirtualMachineImage))
	if err != nil {
		return err
	}

	return add(mgr, r)
}

func newReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager, options VirtualMachineImageDiscovererOptions) reconcile.Reconciler {
	imageDiscoverer := NewVirtualMachineImageDiscoverer(ctx, mgr.GetClient(), options)

	return &ReconcileVirtualMachineImage{
		Client:          mgr.GetClient(),
		log:             ctrl.Log.WithName("controllers").WithName("VirtualMachineImages"),
		scheme:          mgr.GetScheme(),
		imageDiscoverer: imageDiscoverer,
		vmProvider:      ctx.VmProvider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&vmoperatorv1alpha1.VirtualMachineImage{}).
		Complete(r)
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineImage{}

// ReconcileVirtualMachineImage reconciles a VirtualMachineImage object
type ReconcileVirtualMachineImage struct {
	client.Client
	log             logr.Logger
	scheme          *runtime.Scheme
	imageDiscoverer *VirtualMachineImageDiscoverer
	vmProvider      vmprovider.VirtualMachineProviderInterface
}

func (r *ReconcileVirtualMachineImage) Start(stopChan <-chan struct{}) error {
	r.log.Info("Starting VirtualMachineImage Reconciler")
	doneChan := make(chan struct{})
	r.imageDiscoverer.Start(stopChan, doneChan)
	<-doneChan
	r.log.Info("Stopping VirtualMachineImage Reconciler")
	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;update;patch

// At the moment, this Reconcile is a noop.
func (r *ReconcileVirtualMachineImage) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	r.log.V(4).Info("Reconcile VirtualMachineImage ", "namespace", request.Namespace, "name", request.Name)

	instance := &vmoperatorv1alpha1.VirtualMachineImage{}
	err := r.Get(goctx.TODO(), request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
