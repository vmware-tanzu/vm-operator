// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package providerconfigmap implements a controller that is used to reconcile
// the `ContentSource` changes in the provider ConfigMap. The `ContentSource`
// key in the provider ConfigMap represents the TKG content library associated
// with VM operator. This controller detects any create/update ops to the TKG CL
// associations and creates the necessary Custom Resources so VM operator can
// discover VM images from the configured content library.

package providerconfigmap

import (
	goctx "context"
	"time"

	v1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

// This label is used to differentiate a TKG ContentSource from a VM service ContentSource.
const (
	TKGContentSourceLabelKey   = "ContentSourceType"
	TKGContentSourceLabelValue = "TKGContentSource"
	UserWorkloadNamespaceLabel = "vSphereClusterID"
)

// AddToManager adds the ConfigMap controller to the manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	controllerName := "provider-configmap"

	r := NewReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		ctrl.Log.WithName("controllers").WithName(controllerName),
		ctx.VmProvider,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	err = addConfigMapWatch(mgr, c, ctx.SyncPeriod, ctx.Namespace)
	if err != nil {
		return err
	}

	return nil
}

func addConfigMapWatch(mgr manager.Manager, c controller.Controller, syncPeriod time.Duration, ns string) error {
	nsCache, err := pkgmgr.NewNamespaceCache(mgr, &syncPeriod, ns)
	if err != nil {
		return err
	}

	return c.Watch(source.NewKindWithCache(&v1.ConfigMap{}, nsCache), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return e.Meta.GetName() == vsphere.ProviderConfigMapName &&
					e.Object.(*v1.ConfigMap).Data[vsphere.ContentSourceKey] != ""
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.MetaOld.GetName() == vsphere.ProviderConfigMapName &&
					e.ObjectOld.(*v1.ConfigMap).Data[vsphere.ContentSourceKey] !=
						e.ObjectNew.(*v1.ConfigMap).Data[vsphere.ContentSourceKey]
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		},
	)
}

func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	logger logr.Logger,
	vmProvider vmprovider.VirtualMachineProviderInterface) *ConfigMapReconciler {

	return &ConfigMapReconciler{
		Client:     client,
		scheme:     scheme,
		Logger:     logger,
		vmProvider: vmProvider,
	}
}

type ConfigMapReconciler struct {
	client.Client
	scheme     *runtime.Scheme
	Logger     logr.Logger
	vmProvider vmprovider.VirtualMachineProviderInterface
}

func (r *ConfigMapReconciler) CreateOrUpdateContentSourceResources(ctx goctx.Context, clUUID string) error {
	r.Logger.Info("Creating ContentLibraryProvider and ContentSource for TKG content library", "contentLibraryUUID", clUUID)

	clProvider := &vmopv1alpha1.ContentLibraryProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: clUUID,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, clProvider, func() error {
		clProvider.Spec = vmopv1alpha1.ContentLibraryProviderSpec{
			UUID: clUUID,
		}

		return nil
	}); err != nil {
		r.Logger.Error(err, "error creating/updating the ContentLibraryProvider resource", "clProvider", clProvider)
		return err
	}

	cs := &vmopv1alpha1.ContentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: clUUID,
		},
	}

	gvk, err := apiutil.GVKForObject(clProvider, r.scheme)
	if err != nil {
		r.Logger.Error(err, "error extracting the scheme from the ContentLibraryProvider")
		return err
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cs, func() error {
		// Existing labels will be overwritten. Fine for now since we don't have any labels on this resource and it is immutable for developers.
		cs.ObjectMeta.Labels = map[string]string{
			TKGContentSourceLabelKey: TKGContentSourceLabelValue,
		}
		cs.Spec = vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       clProvider.Name,
			},
		}

		return nil
	}); err != nil {
		r.Logger.Error(err, "error creating/updating the ContentSource resource", "contentSource", cs)
		return err
	}

	r.Logger.Info("Created ContentLibraryProvider and ContentSource for TKG content library", "contentLibraryUUID", clUUID)
	return nil
}

// CreateContentSourceBindings creates ContentSourceBindings in all the user workload namespaces for the configured TKG ContentSource.
func (r *ConfigMapReconciler) CreateContentSourceBindings(ctx goctx.Context, clUUID string) error {
	nsList := &v1.NamespaceList{}
	// Presence of the UserWorkloadNamespaceLabel label indicates that a namespace is a user namespace (and not a reserved one). We use
	// this filtration to create ContentSourceBindings for TKG content source in user namespaces.
	if err := r.List(ctx, nsList, client.HasLabels{UserWorkloadNamespaceLabel}); err != nil {
		r.Logger.Error(err, "error listing user workload namespaces")
		return err
	}

	cs := &vmopv1alpha1.ContentSource{}
	if err := r.Get(ctx, client.ObjectKey{Name: clUUID}, cs); err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(cs, r.scheme)
	if err != nil {
		r.Logger.Error(err, "error extracting the scheme from the ContentSource")
		return err
	}

	for _, ns := range nsList.Items {
		r.Logger.Info("Creating ContentSourceBinding for TKG content library in namespace", "contentLibraryUUID", clUUID, "namespace", ns.Name)
		csBinding := &vmopv1alpha1.ContentSourceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clUUID,
				Namespace: ns.Name,
			},
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, csBinding, func() error {
			// Set OwnerRef to the ContentSource so the bindings get cleaned up when the ContentSource is deleted.
			if err := controllerutil.SetOwnerReference(cs, csBinding, r.scheme); err != nil {
				return err
			}

			csBinding.ContentSourceRef = vmopv1alpha1.ContentSourceReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       clUUID,
			}

			return nil
		}); err != nil {
			r.Logger.Error(err, "error creating/updating the ContentSourceBinding resource", "contentSourceBinding", csBinding, "namespace", ns.Name)
			return err
		}
	}

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsourcebindings,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

func (r *ConfigMapReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := goctx.Background()

	cm := &v1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.ReconcileNormal(ctx, cm); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ConfigMapReconciler) ReconcileNormal(ctx goctx.Context, cm *v1.ConfigMap) error {
	r.Logger.Info("Reconciling VM provider ConfigMap", "name", cm.Name, "namespace", cm.Namespace)

	// Filter out the ContentSources that should not exist
	csList := &vmopv1alpha1.ContentSourceList{}
	labels := map[string]string{TKGContentSourceLabelKey: TKGContentSourceLabelValue}

	if err := r.List(ctx, csList, client.MatchingLabels(labels)); err != nil {
		r.Logger.Error(err, "Error in listing ContentSources")
		return err
	}

	// Assume that the ContentSource name is the content library UUID.
	clUUID := cm.Data[vsphere.ContentSourceKey]
	for _, cs := range csList.Items {
		contentSource := cs
		if contentSource.Name != clUUID {
			if err := r.Delete(ctx, &contentSource); err != nil {
				if !apiErrors.IsNotFound(err) {
					r.Logger.Error(err, "Error in deleting the ContentSource resource", "contentSourceName", contentSource.Name)
					return err
				}
			}
		}
	}

	if clUUID == "" {
		r.Logger.V(4).Info("ContentSource key not found/unset in provider ConfigMap. No op reconcile",
			"configMapNamespace", cm.Namespace, "configMapName", cm.Name)
		return nil
	}

	// Ensure that the ContentSource and ContentLibraryProviders exist and are up to date.
	if err := r.CreateOrUpdateContentSourceResources(ctx, clUUID); err != nil {
		r.Logger.Error(err, "failed to create resource from the ConfigMap")
		return err
	}

	// Ensure that all workload namespaces have access to the TKG ContentSource by creating ContentSourceBindings.
	if err := r.CreateContentSourceBindings(ctx, clUUID); err != nil {
		r.Logger.Error(err, "failed to create ContentSourceBindings in user workload namespaces")
		return err
	}

	r.Logger.Info("Finished reconciling VM provider ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
	return nil
}
