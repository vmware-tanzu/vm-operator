// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &spqv1.StoragePolicyQuota{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a StoragePolicyQuota object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

//
// Please note, the delete permissions are required on storagepolicyquotas
// because it is set as the ControllerOwnerReference for the created
// StoragePolicyUsage resource. If a controller owner reference does not have
// delete on the owner, then a 422 (unprocessable entity) is returned.
//

// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas/status,verbs=get
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj spqv1.StoragePolicyQuota
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger := ctrl.Log.WithName("StoragePolicyQuota").WithValues("name", req.NamespacedName)

	// Ensure the GVK for the object is synced back into the object since
	// the object's APIVersion and Kind fields may be used later.
	kubeutil.MustSyncGVKToObject(&obj, r.Scheme())

	if !obj.DeletionTimestamp.IsZero() {
		// Noop.
		return ctrl.Result{}, nil
	}

	if err := r.ReconcileNormal(ctx, logger, obj); err != nil {
		logger.Error(err, "Failed to reconcile StoragePolicyQuota")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	logger logr.Logger,
	src spqv1.StoragePolicyQuota) error {

	dst := spqv1.StoragePolicyUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeutil.StoragePolicyUsageNameFromQuotaName(src.Name),
			Namespace: src.Namespace,
		},
	}

	fn := func() error {
		if err := ctrlutil.SetControllerReference(
			&src, &dst, r.Scheme()); err != nil {

			return err
		}

		dst.Spec.StorageClassName = src.Name
		dst.Spec.StoragePolicyId = src.Spec.StoragePolicyId
		dst.Spec.ResourceAPIgroup = ptr.To(spqv1.GroupVersion.Group)
		dst.Spec.ResourceKind = "StoragePolicyQuota"
		dst.Spec.ResourceExtensionName = "vmservice.cns.vsphere.vmware.com"

		return nil
	}

	_, err := ctrlutil.CreateOrPatch(ctx, r.Client, &dst, fn)

	return err
}
