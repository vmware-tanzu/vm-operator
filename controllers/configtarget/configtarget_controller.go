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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

// bytesPerMB is used to convert vSphere's MB-denominated memory fields into
// the byte-denominated resource.Quantity fields used by ConfigTargetStatus.
const bytesPerMB = 1024 * 1024

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

		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, reason, "%v", err)

		return err
	}

	populateStatus(obj, configTarget)

	if err = r.reconcileConfigOptions(ctx, obj, configOptionDescriptors); err != nil {
		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, QueryConfigOptionDescriptorFailedReason, "%v", err)
		return err
	}

	obj.Status.ObservedGeneration = obj.Generation
	pkgcond.MarkTrue(obj, vimv1.ReadyConditionType)

	logger.V(4).Info("Reconciled ConfigTarget", "clusterMoID", clusterMoID)

	return nil
}

// populateStatus maps the vSphere QueryConfigTarget result onto the
// ConfigTarget's status. Per this sub-task's scope, only numeric limits,
// memory quantities, and security flags are populated; all
// ConfigTargetDevices fields (including SR-IOV) and the per-host-derived
// hardware version fields are left unset — see VMSVC-3759.
func populateStatus(obj *vimv1.ConfigTarget, ct *vimtypes.ConfigTarget) {
	if ct == nil {
		return
	}

	obj.Status.NumCPUs = ct.NumCpus
	obj.Status.NumCPUCores = ct.NumCpuCores
	obj.Status.NumNumaNodes = ct.NumNumaNodes
	obj.Status.MaxCPUsPerVM = ct.MaxCpusPerHost
	obj.Status.MaxSimultaneousThreads = ct.MaxSimultaneousThreads
	obj.Status.SMCPresent = ct.SmcPresent

	if ct.MaxMemMBOptimalPerf > 0 {
		obj.Status.MaxMemOptimalPerf = resource.NewQuantity(int64(ct.MaxMemMBOptimalPerf)*bytesPerMB, resource.BinarySI)
	} else {
		obj.Status.MaxMemOptimalPerf = nil
	}

	if ct.SupportedMaxMemMB > 0 {
		obj.Status.SupportedMaxMem = resource.NewQuantity(int64(ct.SupportedMaxMemMB)*bytesPerMB, resource.BinarySI)
	} else {
		obj.Status.SupportedMaxMem = nil
	}

	if ct.AvailablePersistentMemoryReservationMB > 0 {
		obj.Status.AvailablePersistentMemoryReservation = resource.NewQuantity(
			ct.AvailablePersistentMemoryReservationMB*bytesPerMB, resource.BinarySI)
	} else {
		obj.Status.AvailablePersistentMemoryReservation = nil
	}

	obj.Status.SEVSupported = ct.SevSupported != nil && *ct.SevSupported
	obj.Status.SEVSNPSupported = ct.SevSnpSupported != nil && *ct.SevSnpSupported
	obj.Status.TDXSupported = ct.TdxSupported != nil && *ct.TdxSupported
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
// support the same hardware version.
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

		vmco := &vimv1.VirtualMachineConfigOptions{
			ObjectMeta: metav1.ObjectMeta{Name: d.Key},
		}

		_, err := controllerutil.CreateOrPatch(ctx, r.Client, vmco, func() error {
			vmco.Spec.HardwareVersion = d.Key
			return controllerutil.SetOwnerReference(obj, vmco, r.Scheme())
		})
		if err != nil {
			return fmt.Errorf("failed to create or patch VirtualMachineConfigOptions %q: %w", d.Key, err)
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
			return err
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
	// The caller has already confirmed obj owns vmco, so a single remaining
	// owner reference is obj's own: the object can be deleted outright
	// without first mutating its owner references.
	if len(vmco.OwnerReferences) == 1 {
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
