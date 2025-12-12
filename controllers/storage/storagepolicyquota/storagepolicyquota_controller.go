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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
		WithLogConstructor(pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme())).
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

	logger := pkglog.FromContextOrDefault(ctx)

	if err := r.ReconcileNormal(ctx, logger, &obj); err != nil {
		logger.Error(err, "Failed to ReconcileNormal StoragePolicyQuota")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

type resourceKindInfo struct {
	kind               string
	nameFunc           func(string) string
	quotaExtensionName string
	// Only create StoragePolicyUsage for enabled resource kinds.
	enabled bool
	// If yes, check if quotaExtensionName is correct, otherwise delete the
	// SPU.
	requireExtensionNameCheck bool
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	logger logr.Logger,
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
	// - VirtualMachine.
	// - VirtualMachineSnapshot.
	resourceKinds := []resourceKindInfo{
		{
			kind:               spqutil.VirtualMachineKind,
			nameFunc:           spqutil.StoragePolicyUsageNameForVM,
			quotaExtensionName: spqutil.StoragePolicyQuotaVMExtensionName,
			enabled:            true,
		},
		{
			kind:                      spqutil.VirtualMachineSnapshotKind,
			nameFunc:                  spqutil.StoragePolicyUsageNameForVMSnapshot,
			quotaExtensionName:        spqutil.StoragePolicyQuotaVMSnapshotExtensionName,
			enabled:                   pkgcfg.FromContext(ctx).Features.VMSnapshots,
			requireExtensionNameCheck: pkgcfg.FromContext(ctx).Features.VMSnapshots,
		},
	}

	var errs []error
	for i := range objs {
		objectName := objs[i].Name
		for _, resourceKind := range resourceKinds {
			if !resourceKind.enabled {
				logger.V(4).Info("Skip creating storage policy usage for resource kind",
					"kind", resourceKind.kind)
				continue
			}

			// Handle Upgrade Workaround (Delete-Then-Create if name is wrong).
			// This should just be a one time thing during upgrade.
			if err := r.handleIncorrectSPUResourceExtensionName(
				ctx,
				spq.Namespace,
				resourceKind,
				objectName); err != nil {

				errs = append(errs, err)
				continue
			}

			// Create or Patch the SPU.
			if err := r.createOrPatchSPU(
				ctx,
				spq,
				resourceKind,
				objectName,
				caBundle); err != nil {

				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

// handleIncorrectSPUResourceExtensionName checks for and deletes an SPU if its
// ResourceExtensionName is incorrect due to an upgrade scenario
// (workaround for immutability).
func (r *Reconciler) handleIncorrectSPUResourceExtensionName(
	ctx context.Context,
	namespace string,
	resourceKind resourceKindInfo,
	objectName string) error {

	if !resourceKind.requireExtensionNameCheck {
		return nil
	}

	spu := spqv1.StoragePolicyUsage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      resourceKind.nameFunc(objectName),
		},
	}

	if err := r.Client.Get(
		ctx,
		client.ObjectKey{Namespace: spu.Namespace, Name: spu.Name},
		&spu); err != nil {

		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to fetch SPU %s: %w", spu.Name, err)
		}
		// SPU not found, nothing to delete.
		return nil
	}

	// Check if the current SPU has the wrong name (due to old VMOP version)
	if resourceKind.quotaExtensionName != spu.Spec.ResourceExtensionName {
		pkglog.FromContextOrDefault(ctx).Info(
			"SPU found with wrong quotaExtensionName, deleting",
			"spuNamespace", spu.Namespace,
			"spuName", spu.Name,
			"expected", resourceKind.quotaExtensionName,
			"found", spu.Spec.ResourceExtensionName)

		if err := r.Client.Delete(ctx, &spu); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete SPU %s: %w", spu.Name, err)
			}
		}
		// SPU not found, nothing to delete.
	}
	return nil
}

// createOrPatchSPU ensures the StoragePolicyUsage exists and has the correct spec.
func (r *Reconciler) createOrPatchSPU(
	ctx context.Context,
	spq *spqv1.StoragePolicyQuota,
	resourceKind resourceKindInfo,
	objectName string,
	caBundle []byte) error {

	spu := spqv1.StoragePolicyUsage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: spq.Namespace,
			Name:      resourceKind.nameFunc(objectName),
		},
	}

	mutator := func() error {
		if err := ctrlutil.SetControllerReference(spq, &spu, r.Scheme()); err != nil {
			return err
		}

		spu.Spec.StorageClassName = objectName
		spu.Spec.StoragePolicyId = spq.Spec.StoragePolicyId
		spu.Spec.ResourceAPIgroup = ptr.To(vmopv1.GroupVersion.Group)
		spu.Spec.ResourceKind = resourceKind.kind
		spu.Spec.ResourceExtensionName = resourceKind.quotaExtensionName
		spu.Spec.ResourceExtensionNamespace = r.PodNamespace
		spu.Spec.CABundle = caBundle

		return nil
	}

	if _, err := ctrlutil.CreateOrPatch(ctx, r.Client, &spu, mutator); err != nil {
		return fmt.Errorf("failed to CreateOrPatch SPU %s: %w", spu.Name, err)
	}
	return nil
}
