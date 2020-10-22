// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource

import (
	goCtx "context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	controllerName = "ContentSource"
	finalizerName  = "contentsource.vmoperator.vmware.com"
)

type ContentSourceReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	vmProvider vmprovider.VirtualMachineProviderInterface
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager) reconcile.Reconciler {
	return &ContentSourceReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName(controllerName),
		Scheme:     mgr.GetScheme(),
		vmProvider: ctx.VmProvider,
	}
}

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	return add(mgr, newReconciler(ctx, mgr))
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch ContentSource type resource
	err = c.Watch(
		&source.Kind{Type: &vmopv1alpha1.ContentSource{}},
		&handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch ContentLibraryProvider resource and enqueue request to the owner ContentSource.
	err = c.Watch(
		&source.Kind{Type: &vmopv1alpha1.ContentLibraryProvider{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &vmopv1alpha1.ContentSource{}})
	if err != nil {
		return err
	}

	return nil
}

// GetContentProvider fetches the content provider object from the API server
func GetContentProvider(ctx goCtx.Context, c client.Client, ref vmopv1alpha1.ContentProviderReference) (*vmopv1alpha1.ContentLibraryProvider, error) {
	contentLibrary := &vmopv1alpha1.ContentLibraryProvider{}
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, contentLibrary); err != nil {
		return nil, fmt.Errorf("failed to retrieve object from API server. kind: %v, namespace: %v, name: %v",
			ref.Kind, ref.Namespace, ref.Name)
	}

	return contentLibrary, nil
}

// reconcileProviderRef reconciles a ContentSource's provider reference. Verifies that the content provider pointed by
// the provider ref exists on the infrastructure.
func (r *ContentSourceReconciler) reconcileProviderRef(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) error {
	logger := r.Log.WithValues("contentSourceName", contentSource.Name)

	logger.Info("Reconciling content provider reference")

	defer func() {
		logger.Info("Finished reconciling content provider reference")
	}()

	// ProviderRef should always be set.
	// AKP: This will move to a content provider interface so we are not calling into vmprovider from a content source
	providerRef := contentSource.Spec.ProviderRef
	if providerRef.Kind == "ContentLibraryProvider" {
		contentLibrary, err := GetContentProvider(ctx, r.Client, contentSource.Spec.ProviderRef)
		if err != nil {
			return err
		}

		logger = r.Log.WithValues("contentSourceName", contentSource.Name, "contentLibraryName", contentLibrary.Name,
			"contentLibraryUUID", contentLibrary.Spec.UUID)

		logger.V(4).Info("Content library provider found")

		beforeObj := contentLibrary.DeepCopy()

		// Set an ownerref to the ContentSource
		ownerRef := metav1.OwnerReference{
			APIVersion: contentSource.APIVersion,
			Kind:       contentSource.Kind,
			Name:       contentSource.Name,
			UID:        contentSource.UID,
		}

		contentLibrary.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

		exists, err := r.vmProvider.DoesContentLibraryExist(ctx, contentLibrary)
		if err != nil {
			logger.Error(err, "error in checking if a content library exists")
			return err
		}

		if !exists {
			return errors.New("ContentLibrary does not exist")
		}

		// We can refactor some of the image syncing logic from VM image controller to here. When a CL is created, deleted and
		// on the regular sync period, we can sync the images here. We can also have a goroutine which checks for vSphere events
		// related to the content library pointed by the content source.

		// Update the object if needed
		if !reflect.DeepEqual(beforeObj, contentLibrary) {
			if err := r.Update(ctx, contentLibrary); err != nil {
				return errors.Wrapf(err, "error updating the content library resource")
			}
		}
	}

	return nil
}

// reconcileDeleteProviderRef reconciles a delete for a provider reference. Currently, no op.
func (r *ContentSourceReconciler) reconcileDeleteProviderRef(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) error {
	logger := r.Log.WithValues("contentSourceName", contentSource.Name)

	providerRef := contentSource.Spec.ProviderRef

	logger.Info("Reconciling delete for a content provider", "contentProviderName", providerRef.Name,
		"contentProviderKind", providerRef.Kind, "contentProviderAPIVersion", providerRef.APIVersion)

	// Call into the content provider to clean up any state, if needed. We can delete the VM images from the CL here.
	// For now, VM image controller's SyncImage will do the job for us.

	return nil
}

// reconcile reconciles a content source. Calls into the provider to reconcile the content provider.
func (r *ContentSourceReconciler) reconcile(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) (reconcile.Result, error) {
	logger := r.Log.WithValues("name", contentSource.Name)

	logger.Info("Reconciling ContentSource CreateOrUpdate")

	// Add our finalizer so we can clean up any state when the object is deleted
	if !lib.ContainsString(contentSource.ObjectMeta.Finalizers, finalizerName) {
		contentSource.ObjectMeta.Finalizers = append(contentSource.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, contentSource); err != nil {
			return reconcile.Result{}, nil
		}
	}

	if err := r.reconcileProviderRef(ctx, contentSource); err != nil {
		logger.Error(err, "error in reconciling the provider ref")
		return reconcile.Result{}, err
	}

	logger.Info("Finished reconciling ContentSource")
	return reconcile.Result{}, nil
}

// reconcileDelete reconciles a content source delete. We use a finalizer here to clean up any state if needed.
func (r *ContentSourceReconciler) reconcileDelete(ctx goCtx.Context, contentSource *vmopv1alpha1.ContentSource) (reconcile.Result, error) {
	logger := r.Log.WithValues("name", contentSource.Name)

	defer func() {
		logger.Info("Finished Reconciling ContentSource Deletion")
	}()

	logger.Info("Reconciling ContentSource deletion")

	if lib.ContainsString(contentSource.ObjectMeta.Finalizers, finalizerName) {
		if err := r.reconcileDeleteProviderRef(ctx, contentSource); err != nil {
			logger.Error(err, "error when reconciling the provider ref")
			return reconcile.Result{}, nil
		}

		contentSource.ObjectMeta.Finalizers = lib.RemoveString(contentSource.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, contentSource); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsourcebindings,verbs=get;list;watch

func (r *ContentSourceReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.Log.Info("Received reconcile request", "name", request.Name)

	ctx := goCtx.Background()
	instance := &vmopv1alpha1.ContentSource{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Handle deletion reconciliation loop.
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance)
	}

	return r.reconcile(ctx, instance)
}
