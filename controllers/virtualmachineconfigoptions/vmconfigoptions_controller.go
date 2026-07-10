// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigoptions

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
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
	// Finalizer is the finalizer placed on VirtualMachineConfigOptions objects
	// by VM Operator.
	Finalizer = "vmoperator.vmware.com/virtualmachineconfigoptions"

	// ConfigTargetKind is the owner-reference Kind used to identify a
	// ConfigTarget among a VirtualMachineConfigOptions' owner references.
	ConfigTargetKind = "ConfigTarget"

	// ConfigTargetNotFoundReason is the Ready condition reason used while a
	// VirtualMachineConfigOptions has no ConfigTarget owner reference yet.
	ConfigTargetNotFoundReason = "ConfigTargetNotFound"

	// QueryFailedReason is the Ready condition reason used when resolving the
	// owning cluster or querying QueryConfigOptionEx fails.
	QueryFailedReason = "QueryFailed"

	// NotFoundReason is the Ready condition reason used when
	// QueryConfigOptionEx reports no config option for the requested
	// hardware version.
	NotFoundReason = "NotFound"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var (
	SkipNameValidation *bool

	// nonAlnumHyphenRE matches any character that is not a lowercase letter,
	// digit, or hyphen.
	nonAlnumHyphenRE = regexp.MustCompile(`[^a-z0-9-]+`)
)

var _ = SkipNameValidation

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vimv1.VirtualMachineConfigOptions{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorder(controllerNameShort)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 1),
			SkipNameValidation:      SkipNameValidation,
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

// NewReconciler returns a new Reconciler for VirtualMachineConfigOptions.
func NewReconciler(
	ctx context.Context,
	c client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     c,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles a VirtualMachineConfigOptions object.
type Reconciler struct {
	client.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets,verbs=get;list;watch
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineguestoptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineguestoptions/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj vimv1.VirtualMachineConfigOptions
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
			} else {
				reterr = fmt.Errorf("%w,%w", err, reterr)
			}
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(ctx, &obj)
	}
	return r.ReconcileNormal(ctx, &obj)
}

// ReconcileDelete handles deletion of a VirtualMachineConfigOptions object.
func (r *Reconciler) ReconcileDelete(
	ctx context.Context,
	obj *vimv1.VirtualMachineConfigOptions) (ctrl.Result, error) {

	controllerutil.RemoveFinalizer(obj, Finalizer)
	return ctrl.Result{}, nil
}

// ReconcileNormal reconciles a VirtualMachineConfigOptions object during normal
// (non-delete) operations.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *vimv1.VirtualMachineConfigOptions) (ctrl.Result, error) {

	if controllerutil.AddFinalizer(obj, Finalizer) {
		// Return immediately to persist the finalizer before proceeding.
		return ctrl.Result{}, nil
	}

	clusterMoID, err := r.findClusterMoID(ctx, obj)
	if err != nil {
		pkgcond.MarkError(obj, vimv1.ReadyConditionType, QueryFailedReason, err)
		return ctrl.Result{}, err
	}
	if clusterMoID == "" {
		// No ConfigTarget owner found yet; surface it on the Ready condition
		// and requeue after a short delay.
		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, ConfigTargetNotFoundReason,
			"no %s owner reference found yet", ConfigTargetKind)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	configOption, err := r.VMProvider.QueryConfigOptionEx(ctx, clusterMoID, obj.Spec.HardwareVersion)
	if err != nil {
		pkgcond.MarkError(obj, vimv1.ReadyConditionType, QueryFailedReason, err)
		return ctrl.Result{}, fmt.Errorf("failed to query config option ex: %w", err)
	}

	if configOption == nil {
		pkgcond.MarkFalse(obj, vimv1.ReadyConditionType, NotFoundReason,
			"config option not found for hardware version %s", obj.Spec.HardwareVersion)
		obj.Status.ObservedGeneration = obj.Generation
		return ctrl.Result{}, nil
	}

	applyStatus(obj, configOption)

	if err := r.fanOutGuestOptions(ctx, obj.Spec.HardwareVersion, configOption.GuestOSDescriptor); err != nil {
		return ctrl.Result{}, err
	}

	pkgcond.MarkTrue(obj, vimv1.ReadyConditionType)
	obj.Status.ObservedGeneration = obj.Generation
	return ctrl.Result{}, nil
}

// findClusterMoID looks up a cluster managed object ID from the owner
// references of the given VirtualMachineConfigOptions. It returns an empty
// string (with no error) when no ConfigTarget owner is found.
//
// A hardware-version-keyed VirtualMachineConfigOptions may legitimately be
// co-owned by more than one ConfigTarget (see
// controllers/configtarget's reconcileConfigOptions), since multiple
// clusters can report supporting the same hardware version. vSphere's own
// EnvironmentBrowser.QueryConfigOptionEx does not aggregate across hosts in
// this situation either: it resolves to exactly one host's real, internally
// consistent ConfigOption, chosen deterministically as the host running the
// lowest product version among those supporting the requested key. We have
// no comparable per-cluster signal to replicate that "most conservative"
// selection today (that would need something like each ConfigTarget's ESXi
// version, which this design deliberately does not expose -- see the
// retired HostSystem CRD). So, among co-owners, this picks the
// lexicographically smallest cluster MoID: an arbitrary but stable and
// reproducible tie-break, not a proxy for "most conservative".
//
// TODO(vmop-3964): Replace this tie-break with a real conservative-selection
// signal (e.g. minimum ESXi build across co-owning ConfigTargets) and record
// the selected cluster in VirtualMachineConfigOptions.Status for debugging.
func (r *Reconciler) findClusterMoID(
	ctx context.Context,
	obj *vimv1.VirtualMachineConfigOptions) (string, error) {

	clusterMoIDs := make([]string, 0, len(obj.OwnerReferences))
	for _, ref := range obj.OwnerReferences {
		if ref.Kind != ConfigTargetKind {
			continue
		}
		var ct vimv1.ConfigTarget
		if err := r.Client.Get(ctx, client.ObjectKey{Name: ref.Name}, &ct); err != nil {
			return "", fmt.Errorf("failed to get ConfigTarget %s: %w", ref.Name, err)
		}
		clusterMoIDs = append(clusterMoIDs, ct.Spec.ID.ID)
	}

	if len(clusterMoIDs) == 0 {
		return "", nil
	}

	sort.Strings(clusterMoIDs)
	return clusterMoIDs[0], nil
}

// applyStatus maps configOption fields onto obj.Status. It has no side
// effects beyond obj itself -- fanning out child VirtualMachineGuestOptions
// objects is the separate fanOutGuestOptions method.
func applyStatus(
	obj *vimv1.VirtualMachineConfigOptions,
	configOption *vimtypes.VirtualMachineConfigOption) {

	obj.Status.Description = configOption.Description
	obj.Status.GuestOSDefaultIndex = configOption.GuestOSDefaultIndex
	obj.Status.SupportedMonitorTypes = configOption.SupportedMonitorType
	obj.Status.SupportedOvfEnvironmentTransports = configOption.SupportedOvfEnvironmentTransport
	obj.Status.SupportedOvfInstallTransports = configOption.SupportedOvfInstallTransport

	guestIDs := make([]vimv1.VirtualMachineGuestOSIdentifier, 0, len(configOption.GuestOSDescriptor))
	for _, desc := range configOption.GuestOSDescriptor {
		guestIDs = append(guestIDs, vimv1.VirtualMachineGuestOSIdentifier(desc.Id))
	}
	obj.Status.GuestOSIdentifiers = guestIDs
}

// fanOutGuestOptions creates or updates a VirtualMachineGuestOptions object
// for each guest OS descriptor. Every descriptor is attempted even if an
// earlier one fails -- each is an independent, idempotent upsert, so a
// failure on one must not block progress on the others. Errors from all
// descriptors are joined and returned together; any descriptor left
// unconverged this pass is retried, along with the rest, on the next
// reconcile triggered by that error.
func (r *Reconciler) fanOutGuestOptions(
	ctx context.Context,
	hardwareVersion string,
	descriptors []vimtypes.GuestOsDescriptor) error {

	var errs []error
	for i := range descriptors {
		if err := r.reconcileGuestOptions(ctx, hardwareVersion, descriptors[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// reconcileGuestOptions creates or updates the VirtualMachineGuestOptions
// object for the given guest OS descriptor.
//
// This object is shared: every VirtualMachineConfigOptions (one per hardware
// version) that reports support for this guest OS upserts its own entry into
// Status.HardwareVersions. That fan-in means more than one hardware version's
// reconcile can race to update the same object. Writes therefore use
// client.MergeFromWithOptimisticLock, so a concurrent writer's patch fails
// with a conflict instead of silently dropping an entry; the error is simply
// returned, and controller-runtime's own requeue re-fetches and re-applies on
// the next attempt. Status is a subresource, so it is persisted separately via
// Status().Patch. Each write is skipped when nothing changed, to avoid bumping
// ResourceVersion and re-triggering watchers on an otherwise idle reconcile.
func (r *Reconciler) reconcileGuestOptions(
	ctx context.Context,
	hardwareVersion string,
	desc vimtypes.GuestOsDescriptor) error {

	name := toGuestOptionsName(desc.Id)
	hwStatus := buildHardwareVersionStatus(hardwareVersion, desc)

	obj := &vimv1.VirtualMachineGuestOptions{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}

	if err := r.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get VirtualMachineGuestOptions %s: %w", name, err)
		}
		obj.Spec.ID = vimv1.VirtualMachineGuestOSIdentifier(desc.Id)
		if err := r.Create(ctx, obj); err != nil {
			return fmt.Errorf("failed to create VirtualMachineGuestOptions %s: %w", name, err)
		}
		obj.Status.FullName = desc.FullName
		obj.Status.Family.FromVimType(desc.Family)
		obj.Status.HardwareVersions = upsertHardwareVersionStatus(obj.Status.HardwareVersions, hwStatus)
		if err := r.Status().Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to update status for VirtualMachineGuestOptions %s: %w", name, err)
		}
		return nil
	}

	// Patch the spec toward desired, skipping the write when nothing changed
	// so an idle reconcile does not bump ResourceVersion and re-trigger
	// watchers.
	specBase := obj.DeepCopy()
	obj.Spec.ID = vimv1.VirtualMachineGuestOSIdentifier(desc.Id)
	if !apiequality.Semantic.DeepEqual(specBase.Spec, obj.Spec) {
		if err := r.Patch(ctx, obj, client.MergeFromWithOptions(specBase, client.MergeFromWithOptimisticLock{})); err != nil {
			return fmt.Errorf("failed to patch VirtualMachineGuestOptions %s: %w", name, err)
		}
	}

	// Upsert this hardware version's entry into the shared status list. The
	// optimistic lock makes a racing writer's patch fail with a conflict
	// rather than silently dropping an entry; skip the write when nothing
	// changed.
	statusBase := obj.DeepCopy()
	obj.Status.FullName = desc.FullName
	obj.Status.Family.FromVimType(desc.Family)
	obj.Status.HardwareVersions = upsertHardwareVersionStatus(obj.Status.HardwareVersions, hwStatus)
	if !apiequality.Semantic.DeepEqual(statusBase.Status, obj.Status) {
		if err := r.Status().Patch(ctx, obj, client.MergeFromWithOptions(statusBase, client.MergeFromWithOptimisticLock{})); err != nil {
			return fmt.Errorf("failed to patch status for VirtualMachineGuestOptions %s: %w", name, err)
		}
	}

	return nil
}

// upsertHardwareVersionStatus returns versions with hwStatus inserted,
// replacing any existing entry for the same hardware version.
func upsertHardwareVersionStatus(
	versions []vimv1.VirtualMachineGuestOptionsHardwareVersionStatus,
	hwStatus vimv1.VirtualMachineGuestOptionsHardwareVersionStatus,
) []vimv1.VirtualMachineGuestOptionsHardwareVersionStatus {

	for i := range versions {
		if versions[i].HardwareVersion == hwStatus.HardwareVersion {
			versions[i] = hwStatus
			return versions
		}
	}
	return append(versions, hwStatus)
}

// buildHardwareVersionStatus builds a
// VirtualMachineGuestOptionsHardwareVersionStatus from a GuestOsDescriptor.
func buildHardwareVersionStatus(
	hardwareVersion string,
	desc vimtypes.GuestOsDescriptor) vimv1.VirtualMachineGuestOptionsHardwareVersionStatus {

	s := vimv1.VirtualMachineGuestOptionsHardwareVersionStatus{
		HardwareVersion:             hardwareVersion,
		SupportedMaxCPUs:            desc.SupportedMaxCPUs,
		NumSupportedPhysicalSockets: desc.NumSupportedPhysicalSockets,
		NumSupportedCoresPerSocket:  desc.NumSupportedCoresPerSocket,
		RecommendedColorDepth:       desc.RecommendedColorDepth,
		SupportedNumDisks:           desc.SupportedNumDisks,
		RecommendedDiskController:   vimv1.VirtualControllerType(desc.RecommendedDiskController),
		RecommendedSCSIController:   vimv1.VirtualControllerType(desc.RecommendedSCSIController),
		RecommendedCdromController:  vimv1.VirtualControllerType(desc.RecommendedCdromController),
		RecommendedEthernetCard:     vimv1.EthernetCardType(desc.RecommendedEthernetCard),
		SupportsWakeOnLan:           desc.SupportsWakeOnLan,
		SmcRequired:                 desc.SmcRequired,
		SmcRecommended:              desc.SmcRecommended,
		UsbRecommended:              desc.UsbRecommended,
		SupportsVMI:                 desc.SupportsVMI,
		SupportedForCreate:          desc.SupportedForCreate,
	}
	s.SupportLevel.FromVimType(desc.SupportLevel)

	if desc.SupportedMaxMemMB > 0 {
		q := resource.NewQuantity(int64(desc.SupportedMaxMemMB)*1024*1024, resource.BinarySI)
		s.SupportedMaxMem = q
	}
	if desc.SupportedMinMemMB > 0 {
		q := resource.NewQuantity(int64(desc.SupportedMinMemMB)*1024*1024, resource.BinarySI)
		s.SupportedMinMem = q
	}
	if desc.RecommendedMemMB > 0 {
		q := resource.NewQuantity(int64(desc.RecommendedMemMB)*1024*1024, resource.BinarySI)
		s.RecommendedMem = q
	}

	return s
}

// toGuestOptionsName converts a guest OS identifier into a DNS-subdomain-safe
// name for the corresponding VirtualMachineGuestOptions object.
func toGuestOptionsName(guestID string) string {
	s := strings.ToLower(guestID)
	s = nonAlnumHyphenRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	return s
}
