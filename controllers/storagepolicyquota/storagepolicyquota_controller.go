// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
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
		ctx.Namespace,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithLogConstructor(pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme())).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	podNamespace string) *Reconciler {

	return &Reconciler{
		Context:      ctx,
		Client:       client,
		Logger:       logger,
		Recorder:     recorder,
		PodNamespace: podNamespace,
	}
}

// Reconciler reconciles a StoragePolicyQuota object.
type Reconciler struct {
	client.Client
	Context      context.Context
	Logger       logr.Logger
	Recorder     record.Recorder
	PodNamespace string
}

//
// Please note, the delete permissions are required on storagepolicyquotas
// because it is set as the ControllerOwnerReference for the created
// StoragePolicyUsage resource. If a controller owner reference does not have
// delete on the owner, then a 422 (unprocessable entity) is returned.
//

// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj spqv1.StoragePolicyQuota
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure the GVK for the object is synced back into the object since
	// the object's APIVersion and Kind fields may be used later.
	kubeutil.MustSyncGVKToObject(&obj, r.Scheme())

	if !obj.DeletionTimestamp.IsZero() {
		// No-op
		return ctrl.Result{}, nil
	}

	if err := r.ReconcileNormal(ctx, &obj); err != nil {
		pkgutil.FromContextOrDefault(ctx).Error(err, "Failed to ReconcileNormal StoragePolicyQuota")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	spq *spqv1.StoragePolicyQuota) error {

	caBundle, err := spqutil.GetWebhookCABundle(ctx, r.Client)
	if err != nil {
		return err
	}

	// Get the list of storage classes for the provided policy quota.
	objs, err := spqutil.GetStorageClassesForPolicyQuota(
		ctx,
		r.Client,
		spq)
	if err != nil {
		return err
	}

	// Create the StoragePolicyUsage resources for below kinds:
	// - VirtualMachine
	// - VirtualMachineSnapshot
	resourceKinds := []struct {
		Kind     string
		NameFunc func(string) string
	}{
		{
			Kind:     spqutil.VirtualMachineKind,
			NameFunc: spqutil.StoragePolicyUsageNameForVM,
		},
		{
			Kind:     spqutil.VirtualMachineSnapshotKind,
			NameFunc: spqutil.StoragePolicyUsageNameForVMSnapshot,
		},
	}

	var errs []error
	for i := range objs {
		for _, resourceKind := range resourceKinds {
			spu := spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spq.Namespace,
					Name:      resourceKind.NameFunc(objs[i].Name),
				},
			}

			fn := func() error {
				if err := ctrlutil.SetControllerReference(
					spq, &spu, r.Scheme()); err != nil {

					return err
				}

				spu.Spec.StorageClassName = objs[i].Name
				spu.Spec.StoragePolicyId = spq.Spec.StoragePolicyId
				spu.Spec.ResourceAPIgroup = ptr.To(vmopv1.GroupVersion.Group)
				spu.Spec.ResourceKind = resourceKind.Kind
				spu.Spec.ResourceExtensionName = spqutil.StoragePolicyQuotaExtensionName
				spu.Spec.ResourceExtensionNamespace = r.PodNamespace
				spu.Spec.CABundle = caBundle

				return nil
			}

			if _, err := ctrlutil.CreateOrPatch(
				ctx,
				r.Client,
				&spu,
				fn); err != nil {

				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}
