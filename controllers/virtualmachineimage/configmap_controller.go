// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage

import (
	goctx "context"
	"time"

	corev1 "k8s.io/api/core/v1"
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
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

// AddToManager adds the ConfigMap controller and VirtualMachineImage controller to the manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if err := addVMImageControllerToManager(ctx, mgr); err != nil {
		return err
	}

	return addConfigMapControllerToManager(ctx, mgr)
}

// addConfigMapControllerToManager adds the ConfigMap controller to manager.
func addConfigMapControllerToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	controllerName := "virtualmachineimage-configmap"

	r := NewCMReconciler(
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

	return c.Watch(source.NewKindWithCache(&corev1.ConfigMap{}, nsCache), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return e.Meta.GetName() == vsphere.ProviderConfigMapName &&
					e.Object.(*corev1.ConfigMap).Data[vsphere.ContentSourceKey] != ""
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.MetaOld.GetName() == vsphere.ProviderConfigMapName &&
					e.ObjectOld.(*corev1.ConfigMap).Data[vsphere.ContentSourceKey] !=
						e.ObjectNew.(*corev1.ConfigMap).Data[vsphere.ContentSourceKey]
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		},
		predicate.ResourceVersionChangedPredicate{},
	)
}

func NewCMReconciler(
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

func (r *ConfigMapReconciler) CreateContentSourceResources(ctx goctx.Context, clUUID string) error {
	r.Logger.Info("Creating ContentLibraryProvider and ContentSource resource for content library", "contentLibraryUUID", clUUID)
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
		r.Logger.Error(err, "error creating the ContentLibraryProvider resource", "clProvider", clProvider)
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
		cs.Spec = vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       clProvider.Name,
			},
		}

		return nil
	}); err != nil {
		r.Logger.Error(err, "error creating the ContentSource resource", "contentSource", cs)
		return err
	}

	r.Logger.Info("Created ContentLibraryProvider and ContentSource resource for content library", "contentLibraryUUID", clUUID)
	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources,verbs=get;list;create;update;delete;deletecollection

func (r *ConfigMapReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := goctx.Background()

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// If the WCP_VMService FSS is enabled, we do not use the provider ConfigMap for content discovery.
	if lib.IsVMServiceFSSEnabled() {
		return ctrl.Result{}, nil
	}

	if err := r.ReconcileNormal(ctx, cm); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ConfigMapReconciler) ReconcileNormal(ctx goctx.Context, cm *corev1.ConfigMap) error {
	r.Logger.Info("Reconciling VM provider ConfigMap", "name", cm.Name, "namespace", cm.Namespace)

	if err := r.DeleteAllOf(ctx, &vmopv1alpha1.ContentSource{}); err != nil {
		r.Logger.Error(err, "error in deleting the ContentSource resources")
		return err
	}

	clUUID := cm.Data[vsphere.ContentSourceKey]
	if clUUID == "" {
		r.Logger.V(4).Info("ContentSource key not found/unset in provider ConfigMap. No op reconcile",
			"configMapNamespace", cm.Namespace, "configMapName", cm.Name)
		return nil
	}

	if err := r.CreateContentSourceResources(ctx, clUUID); err != nil {
		r.Logger.Error(err, "failed to create resource from the ConfigMap")
		return err
	}

	r.Logger.Info("Finished reconciling VM provider ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
	return nil
}
