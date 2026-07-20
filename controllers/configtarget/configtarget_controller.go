// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configtarget

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/configtarget"
)

const (
	// ClusterNotFoundReason is the Ready condition reason used when the
	// ConfigTarget's metadata.name does not resolve to a vSphere cluster.
	ClusterNotFoundReason = "ClusterNotFound"

	// QueryConfigTargetFailedReason is the Ready condition reason used when
	// EnvironmentBrowser.QueryConfigTarget returns a transient error.
	QueryConfigTargetFailedReason = "QueryConfigTargetFailed"

	// QueryConfigOptionDescriptorFailedReason is the Ready condition reason
	// used when EnvironmentBrowser.QueryConfigOptionDescriptor returns a
	// transient error.
	QueryConfigOptionDescriptorFailedReason = "QueryConfigOptionDescriptorFailed"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vimv1.ConfigTarget{}
		controlledTypeName = reflect.TypeFor[vimv1.ConfigTarget]().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorder(controllerNameShort)),
		ctx.VMProvider)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		// VirtualMachineConfigOptions objects may be co-owned by more than
		// one ConfigTarget (see reconcileConfigOptions), so every owner
		// reference -- not just the sole controller reference -- must
		// trigger a re-reconcile of this ConfigTarget.
		Owns(&vimv1.VirtualMachineConfigOptions{}, builder.MatchEveryOwner).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 0),
			SkipNameValidation:      SkipNameValidation,
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {
	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles the cluster-scope path of a ConfigTarget object.
type Reconciler struct {
	client.Client

	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	logger := pkglog.FromContextOrDefault(ctx)
	logger = logger.WithName(req.Name)
	ctx = logr.NewContext(ctx, logger)

	var obj vimv1.ConfigTarget

	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", req, err)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, &obj); err != nil {
			if reterr == nil {
				reterr = err
			}

			logger.Error(err, "patch failed")
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.ReconcileNormal(ctx, &obj)
}

// ReconcileNormal reconciles the cluster-scope path of a ConfigTarget:
// QueryConfigTarget populates status, QueryConfigOptionDescriptor fans out
// VirtualMachineConfigOptions objects, and any hardware version no longer
// reported is garbage-collected.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *vimv1.ConfigTarget) error {
	logger := pkglog.FromContextOrDefault(ctx)

	clusterMoID := obj.Name

	configTarget, configOptionDescriptors, err := r.VMProvider.GetVirtualMachineConfigTarget(ctx, clusterMoID)
	if err != nil {
		reason := QueryConfigTargetFailedReason

		var notFound *vimtypes.ManagedObjectNotFound
		if _, ok := fault.As(err, &notFound); ok {
			reason = ClusterNotFoundReason
		}

		pkgcond.MarkError(obj, vimv1.ReadyConditionType, reason, err)

		return fmt.Errorf("failed to get virtual machine config target: %w", err)
	}

	configtarget.PopulateStatus(obj, configTarget, configOptionDescriptors)

	if err = r.reconcileConfigOptions(ctx, obj, configOptionDescriptors); err != nil {
		pkgcond.MarkError(obj, vimv1.ReadyConditionType, QueryConfigOptionDescriptorFailedReason, err)
		return fmt.Errorf("failed to reconcile config options: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	pkgcond.MarkTrue(obj, vimv1.ReadyConditionType)

	logger.V(4).Info("Reconciled ConfigTarget", "clusterMoID", clusterMoID)

	return nil
}

// reconcileConfigOptions creates or patches a VirtualMachineConfigOptions
// object per hardware version key returned by QueryConfigOptionDescriptor,
// adding obj's (non-controller) owner reference to each one, then
// garbage-collects any obj owns that are no longer reported.
//
// A plain (non-controller) owner reference is used, rather than
// SetControllerReference, because the same hardware-version-keyed
// VirtualMachineConfigOptions object may be legitimately co-owned by
// multiple ConfigTarget objects from different clusters that happen to
// support the same hardware version. That fan-in means more than one
// reconcile can write to the same object's owner references, so the patch
// uses client.MergeFromWithOptimisticLock: the API server rejects a
// concurrent write with a conflict error -- no custom retry loop is needed
// to detect that; the error is simply returned, and controller-runtime's own
// requeue re-fetches and re-applies on the next attempt.
func (r *Reconciler) reconcileConfigOptions(
	ctx context.Context,
	obj *vimv1.ConfigTarget,
	descriptors []vimtypes.VirtualMachineConfigOptionDescriptor) error {
	liveKeys := sets.New[string]()

	for _, d := range descriptors {
		if d.Key == "" {
			continue
		}

		liveKeys.Insert(d.Key)
		key := d.Key

		vmco := &vimv1.VirtualMachineConfigOptions{
			ObjectMeta: metav1.ObjectMeta{Name: key},
		}

		if err := r.Get(ctx, client.ObjectKey{Name: key}, vmco); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get VirtualMachineConfigOptions %q: %w", key, err)
			}
			vmco.Spec.HardwareVersion = key
			if err := controllerutil.SetOwnerReference(obj, vmco, r.Scheme()); err != nil {
				return fmt.Errorf("failed to set owner reference on VirtualMachineConfigOptions %q: %w", key, err)
			}
			if err := r.Create(ctx, vmco); err != nil {
				return fmt.Errorf("failed to create VirtualMachineConfigOptions %q: %w", key, err)
			}
			continue
		}

		// The same hardware-version-keyed object can be co-owned by
		// multiple ConfigTargets, so each writer upserts its own owner
		// reference into a shared list. The optimistic lock makes a racing
		// writer's patch fail with a conflict instead of dropping an owner
		// reference; skip the write when nothing changed.
		base := vmco.DeepCopy()
		vmco.Spec.HardwareVersion = key
		if err := controllerutil.SetOwnerReference(obj, vmco, r.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on VirtualMachineConfigOptions %q: %w", key, err)
		}
		if apiequality.Semantic.DeepEqual(base.Spec, vmco.Spec) &&
			apiequality.Semantic.DeepEqual(base.OwnerReferences, vmco.OwnerReferences) {
			continue
		}
		if err := r.Patch(ctx, vmco, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
			return fmt.Errorf("failed to patch VirtualMachineConfigOptions %q: %w", key, err)
		}
	}

	return r.garbageCollectConfigOptions(ctx, obj, liveKeys)
}

// garbageCollectConfigOptions removes obj's owner reference from any
// VirtualMachineConfigOptions object it owns whose name is not in liveKeys.
// The object itself is deleted only once that removal leaves it with no
// remaining owner references, so a hardware-version key shared by another
// live ConfigTarget is left untouched.
func (r *Reconciler) garbageCollectConfigOptions(
	ctx context.Context,
	obj *vimv1.ConfigTarget,
	liveKeys sets.Set[string]) error {
	var list vimv1.VirtualMachineConfigOptionsList

	if err := r.List(ctx, &list); err != nil {
		return fmt.Errorf("failed to list VirtualMachineConfigOptions: %w", err)
	}

	for i := range list.Items {
		vmco := &list.Items[i]

		if liveKeys.Has(vmco.Name) {
			continue
		}

		owned, err := controllerutil.HasOwnerReference(vmco.OwnerReferences, obj, r.Scheme())
		if err != nil {
			return fmt.Errorf("failed to check owner reference on VirtualMachineConfigOptions %q: %w", vmco.Name, err)
		}

		if !owned {
			continue
		}

		if err := r.removeOwnerRefAndDeleteIfOrphaned(ctx, obj, vmco); err != nil {
			return fmt.Errorf("failed to remove owner reference and delete if orphaned: %w", err)
		}
	}

	return nil
}

// removeOwnerRefAndDeleteIfOrphaned removes obj's owner reference from vmco
// and, if that leaves vmco with no remaining owner references, deletes it.
// If other ConfigTarget objects still own vmco, only the owner reference is
// removed and vmco itself is left in place.
func (r *Reconciler) removeOwnerRefAndDeleteIfOrphaned(
	ctx context.Context,
	obj *vimv1.ConfigTarget,
	vmco *vimv1.VirtualMachineConfigOptions) error {
	// If obj is the sole owner reference, the object can be deleted outright
	// without first mutating its owner references.
	if len(vmco.OwnerReferences) == 1 && vmco.OwnerReferences[0].UID == obj.GetUID() {
		if err := r.Delete(ctx, vmco); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete orphaned VirtualMachineConfigOptions %q: %w", vmco.Name, err)
		}

		return nil
	}

	if err := controllerutil.RemoveOwnerReference(obj, vmco, r.Scheme()); err != nil {
		return fmt.Errorf("failed to remove owner reference from VirtualMachineConfigOptions %q: %w", vmco.Name, err)
	}

	if err := r.Update(ctx, vmco); err != nil {
		return fmt.Errorf("failed to patch VirtualMachineConfigOptions %q after removing owner reference: %w", vmco.Name, err)
	}

	return nil
}
