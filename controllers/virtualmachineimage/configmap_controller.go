// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage

import (
	goctx "context"
	"reflect"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	OpDeleteVMOpConfig      = "DeleteVMOpConfigMap"
	ConfigMapControllerName = "configmapconfig-controller"
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
	var (
		controlledType     = &corev1.ConfigMap{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()
	)

	r := NewCMReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		ctx.Namespace,
		ctx.VmProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithEventFilter(
			predicate.Funcs{
				// Queue a reconcile if the ConfigMap is created with a non empty ContentSource key.
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Meta.GetName() == vsphere.ProviderConfigMapName &&
						e.Meta.GetNamespace() == r.vmOpNamespace &&
						e.Object.(*corev1.ConfigMap).Data[vsphere.ContentSourceKey] != ""
				},
				// Queue a reconcile if the ContentSource key has been updated.
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.MetaOld.GetName() == vsphere.ProviderConfigMapName &&
						e.MetaOld.GetNamespace() == r.vmOpNamespace &&
						e.ObjectOld.(*corev1.ConfigMap).Data[vsphere.ContentSourceKey] !=
							e.ObjectNew.(*corev1.ConfigMap).Data[vsphere.ContentSourceKey]
				},
				// Ignore a CM deletion since without a provider CM, VM operator cannot function anyway.
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return false
				},
			},
		).Complete(r)
}

func NewCMReconciler(
	client client.Client,
	logger logr.Logger,
	vmOpNamespace string,
	vmProvider vmprovider.VirtualMachineProviderInterface) *ConfigMapReconciler {

	return &ConfigMapReconciler{
		Client:        client,
		Logger:        logger,
		vmOpNamespace: vmOpNamespace,
		vmProvider:    vmProvider,
	}
}

type ConfigMapReconciler struct {
	client.Client
	Logger        logr.Logger
	vmOpNamespace string
	vmProvider    vmprovider.VirtualMachineProviderInterface
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

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cs, func() error {
		cs.Spec = vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				APIVersion: clProvider.APIVersion,
				Kind:       clProvider.Kind,
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

	clUUID, ok := cm.Data[vsphere.ContentSourceKey]
	if !ok || clUUID == "" {
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
