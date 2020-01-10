/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage

import (
	"context"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
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

var log = logf.Log.WithName(ControllerName)

type VirtualMachineImageDiscovererOptions struct {
	initialDiscoveryFrequency    time.Duration
	continuousDiscoveryFrequency time.Duration
}

type VirtualMachineImageDiscoverer struct {
	client     client.Client
	vmprovider vmprovider.VirtualMachineProviderInterface
	options    VirtualMachineImageDiscovererOptions
}

func newVirtualMachineImageDiscoverer(client client.Client, vmprovider vmprovider.VirtualMachineProviderInterface,
	options VirtualMachineImageDiscovererOptions) *VirtualMachineImageDiscoverer {
	return &VirtualMachineImageDiscoverer{client: client, vmprovider: vmprovider, options: options}
}

// Attempt to create a set of images.  Fail fast on first failure
func (d *VirtualMachineImageDiscoverer) createImages(ctx context.Context, images []vmoperatorv1alpha1.VirtualMachineImage) error {
	log.V(4).Info("create")

	for _, image := range images {
		img := image
		log.V(4).Info("Creating image", "name", img.Name)
		err := d.client.Create(ctx, &img)
		if err != nil {
			return errors.Wrapf(err, "failed to create image %s", img.Name)
		}
	}

	return nil
}

// Attempt to delete a set of images.  Fail fast on first failure
func (d *VirtualMachineImageDiscoverer) deleteImages(ctx context.Context, images []vmoperatorv1alpha1.VirtualMachineImage) error {
	log.V(4).Info("delete")

	for _, image := range images {
		img := image
		log.V(4).Info("Deleting image", "name", img.Name)
		err := d.client.Delete(ctx, &img)
		if err != nil {
			return errors.Wrapf(err, "failed to delete image %s", img.Name)
		}
	}

	return nil
}

// Difference two lists of VirtualMachineImages producing 3 lists: images that have been added to "right, images that
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
			log.V(4).Info("Removing Image", "name", l.Name)
			removed = append(removed, l)
		} else {
			// Note, this adds every existing image to the updated list without actually performing a deep comparison.
			log.V(4).Info("Updating Image", "name", l.Name)
			updated = append(updated, l)
		}
	}

	// Identify added items
	for _, r := range right {
		if _, ok := leftMap[r.Name]; !ok {
			log.V(4).Info("Adding", "name", r.Name)
			added = append(added, r)
		}
	}

	return added, removed, updated
}

func (d *VirtualMachineImageDiscoverer) differenceImages(ctx context.Context) (error, []vmoperatorv1alpha1.VirtualMachineImage, []vmoperatorv1alpha1.VirtualMachineImage) {
	log.V(4).Info("Differencing images")

	// List the existing images from both the vm provider backend and the Kubernetes control plane (etcd).
	// Difference this list and update the k8s control plane to reflect the state of the image inventory from
	// the backend.
	k8sManagedImageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
	err := d.client.List(ctx, &client.ListOptions{}, k8sManagedImageList)
	if err != nil {
		return errors.Wrap(err, "failed to list images from control plane"), nil, nil
	}

	k8sManagedImages := k8sManagedImageList.Items

	// List the cluster-scoped VirtualMachineImages.
	providerManagedImages, err := d.vmprovider.ListVirtualMachineImages(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to list images from backend"), nil, nil
	}

	var convertedImages []vmoperatorv1alpha1.VirtualMachineImage
	for _, image := range providerManagedImages {
		img := image
		convertedImages = append(convertedImages, *img)
	}

	// Difference the kubernetes images with the provider images
	// TODO: Ignore updated for now
	added, removed, _ := d.diffImages(k8sManagedImages, convertedImages)
	log.V(4).Info("Differenced", "added", added, "removed", removed)

	return nil, added, removed
}

// Blocking function to drive image discovery and differencing
func (d *VirtualMachineImageDiscoverer) Start(stopChan <-chan struct{}, doneChan chan struct{}) {
	log.Info("Starting VirtualMachineImageDiscoverer",
		"initial discovery frequency", d.options.initialDiscoveryFrequency,
		"continuous discovery frequency", d.options.continuousDiscoveryFrequency)

	syncImages := func() error {
		ctx := context.Background()
		err, added, removed := d.differenceImages(ctx)
		if err != nil {
			log.Error(err, "failed to difference images")
			return err
		}

		err = d.createImages(ctx, added)
		if err != nil {
			log.Error(err, "failed to create all images")
			return err
		}

		err = d.deleteImages(ctx, removed)
		if err != nil {
			log.Error(err, "failed to delete all images")
			return err
		}

		return nil
	}

	// Drive "aggressive" discovery until there is a fully successful initial sync of the images.  We do this so
	// that there is some initial content seeded into the k8s control plane.
	log.Info("Performing an initial image discovery")
	initSyncDone := make(chan bool)
	ticker := time.NewTicker(d.options.initialDiscoveryFrequency)
	go func() {
		for {
			select {
			case <-stopChan:
				log.Info("Stop channel event")
				ticker.Stop()
				close(initSyncDone)
				return
			case t := <-ticker.C:
				log.V(4).Info("Tick at", "time", t)
				err := syncImages()

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
	log.Info("Initial image discovery completed successfully. Graduating to the continuous image discovery phase.")

	// Shift to a continuous discovery frequency (presumably at a longer interval) in order to discovery new content
	// published on the order of days.
	ticker = time.NewTicker(d.options.continuousDiscoveryFrequency)
	go func() {
		for {
			select {
			case <-stopChan:
				log.Info("Stop channel event")
				ticker.Stop()
				close(doneChan)
				return
			case t := <-ticker.C:
				log.V(4).Info("Tick at", "time", t)
				_ = syncImages()
			}
		}
	}()
}

// Add creates a new VirtualMachineImage Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	options := VirtualMachineImageDiscovererOptions{
		initialDiscoveryFrequency:    InitialImageDiscoveryFrequency,
		continuousDiscoveryFrequency: ContinuousImageDiscoveryFrequency,
	}
	return AddWithOptions(mgr, options)
}

func AddWithOptions(mgr manager.Manager, options VirtualMachineImageDiscovererOptions) error {

	r := newReconciler(mgr, options)
	ir := r.(*ReconcileVirtualMachineImage)

	// Add the reconciler explicitly as runnable in order to receive a Start() event
	error := mgr.Add(ir)
	if error != nil {
		return error
	}

	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, options VirtualMachineImageDiscovererOptions) reconcile.Reconciler {
	vmProvider := vmprovider.GetVmProviderOrDie()
	imageDiscoverer := newVirtualMachineImageDiscoverer(mgr.GetClient(), vmProvider, options)

	return &ReconcileVirtualMachineImage{
		Client:          mgr.GetClient(),
		scheme:          mgr.GetScheme(),
		imageDiscoverer: imageDiscoverer,
		vmProvider:      vmProvider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	log.Info("Adding VirtualMachineImage Reconciler")

	// Create a new controller.  We only need one reconciler to do the job.
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachineImage
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachineImage{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineImage{}

// ReconcileVirtualMachineImage reconciles a VirtualMachineImage object
type ReconcileVirtualMachineImage struct {
	client.Client
	scheme          *runtime.Scheme
	imageDiscoverer *VirtualMachineImageDiscoverer
	vmProvider      vmprovider.VirtualMachineProviderInterface
}

func (r *ReconcileVirtualMachineImage) Start(stopChan <-chan struct{}) error {
	log.Info("Starting VirtualMachineImage Reconciler")
	doneChan := make(chan struct{})
	r.imageDiscoverer.Start(stopChan, doneChan)
	<-doneChan
	log.Info("Stopping VirtualMachineImage Reconciler")
	return nil
}

// Reconcile reads that state of the cluster for a VirtualMachineImage object and makes changes based on the state read
// and what is in the VirtualMachineImage.Spec.

// At the moment, this Reconcile is a noop.
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;update;patch
func (r *ReconcileVirtualMachineImage) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.V(4).Info("Reconcile VirtualMachineImage ")
	// Fetch the VirtualMachineImage instance
	instance := &vmoperatorv1alpha1.VirtualMachineImage{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
