// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigoptions

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
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
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

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
		pkgcond.Set(obj, pkgcond.FalseCondition(
			vimv1.ReadyConditionType,
			"QueryFailed",
			"%v", err,
		))
		return ctrl.Result{}, err
	}
	if clusterMoID == "" {
		// No ConfigTarget owner found yet; requeue after a short delay.
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	configOption, err := r.VMProvider.QueryConfigOptionEx(ctx, clusterMoID, obj.Spec.HardwareVersion)
	if err != nil {
		pkgcond.Set(obj, pkgcond.FalseCondition(
			vimv1.ReadyConditionType,
			"QueryFailed",
			"%v", err,
		))
		return ctrl.Result{}, fmt.Errorf("failed to query config option ex: %w", err)
	}

	if configOption == nil {
		pkgcond.Set(obj, pkgcond.FalseCondition(
			vimv1.ReadyConditionType,
			"NotFound",
			"config option not found for hardware version %s", obj.Spec.HardwareVersion,
		))
		obj.Status.ObservedGeneration = obj.Generation
		return ctrl.Result{}, nil
	}

	if err := r.applyStatus(ctx, obj, configOption); err != nil {
		return ctrl.Result{}, err
	}

	pkgcond.Set(obj, pkgcond.TrueCondition(vimv1.ReadyConditionType))
	obj.Status.ObservedGeneration = obj.Generation
	return ctrl.Result{}, nil
}

// findClusterMoID looks up the cluster managed object ID from the owner
// references of the given VirtualMachineConfigOptions. It returns an empty
// string (with no error) when no ConfigTarget owner is found.
func (r *Reconciler) findClusterMoID(
	ctx context.Context,
	obj *vimv1.VirtualMachineConfigOptions) (string, error) {

	for _, ref := range obj.OwnerReferences {
		if ref.Kind == "ConfigTarget" {
			var ct vimv1.ConfigTarget
			if err := r.Client.Get(ctx, client.ObjectKey{Name: ref.Name}, &ct); err != nil {
				return "", fmt.Errorf("failed to get ConfigTarget %s: %w", ref.Name, err)
			}
			return ct.Spec.ID.ID, nil
		}
	}
	return "", nil
}

// applyStatus maps configOption fields onto obj.Status and fans out child
// VirtualMachineGuestOptions objects for each guest OS descriptor.
func (r *Reconciler) applyStatus(
	ctx context.Context,
	obj *vimv1.VirtualMachineConfigOptions,
	configOption *vimtypes.VirtualMachineConfigOption) error {

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

	for i := range configOption.GuestOSDescriptor {
		desc := configOption.GuestOSDescriptor[i]
		if err := r.reconcileGuestOptions(ctx, obj.Spec.HardwareVersion, desc); err != nil {
			return err
		}
	}

	return nil
}

// reconcileGuestOptions creates or updates a VirtualMachineGuestOptions object
// for the given guest OS descriptor.
func (r *Reconciler) reconcileGuestOptions(
	ctx context.Context,
	hardwareVersion string,
	desc vimtypes.GuestOsDescriptor) error {

	name := toGuestOptionsName(desc.Id)

	desired := &vimv1.VirtualMachineGuestOptions{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if _, err := controllerutil.CreateOrPatch(ctx, r.Client, desired, func() error {
		desired.Spec.ID = vimv1.VirtualMachineGuestOSIdentifier(desc.Id)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create or patch VirtualMachineGuestOptions %s: %w", name, err)
	}

	// Build the hardware-version status entry.
	hwStatus := buildHardwareVersionStatus(hardwareVersion, desc)

	// Fetch the latest version so the status patch is applied on a current base.
	current := &vimv1.VirtualMachineGuestOptions{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: name}, current); err != nil {
		return fmt.Errorf("failed to get VirtualMachineGuestOptions %s for status patch: %w", name, err)
	}

	updated := current.DeepCopy()
	updated.Status.FullName = desc.FullName
	updated.Status.Family = vimv1.VirtualMachineGuestOSFamily(desc.Family)

	// Upsert the hardware version entry in the listMap.
	found := false
	for i := range updated.Status.HardwareVersions {
		if updated.Status.HardwareVersions[i].HardwareVersion == hardwareVersion {
			updated.Status.HardwareVersions[i] = hwStatus
			found = true
			break
		}
	}
	if !found {
		updated.Status.HardwareVersions = append(updated.Status.HardwareVersions, hwStatus)
	}

	if err := r.Client.Status().Patch(ctx, updated, client.MergeFrom(current)); err != nil {
		return fmt.Errorf("failed to patch status for VirtualMachineGuestOptions %s: %w", name, err)
	}

	return nil
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
		SupportLevel:                vimv1.SupportLevel(desc.SupportLevel),
	}

	s.SmcRequired = desc.SmcRequired
	s.SmcRecommended = desc.SmcRecommended
	s.UsbRecommended = desc.UsbRecommended
	s.SupportsVMI = desc.SupportsVMI
	s.SupportedForCreate = desc.SupportedForCreate

	if desc.SupportedMaxMemMB > 0 {
		q := resource.NewMilliQuantity(int64(desc.SupportedMaxMemMB)*1024*1024, resource.BinarySI)
		s.SupportedMaxMem = q
	}
	if desc.SupportedMinMemMB > 0 {
		q := resource.NewMilliQuantity(int64(desc.SupportedMinMemMB)*1024*1024, resource.BinarySI)
		s.SupportedMinMem = q
	}
	if desc.RecommendedMemMB > 0 {
		q := resource.NewMilliQuantity(int64(desc.RecommendedMemMB)*1024*1024, resource.BinarySI)
		s.RecommendedMem = q
	}

	return s
}

// nonAlnumHyphenRE matches any character that is not a lowercase letter,
// digit, or hyphen.
var nonAlnumHyphenRE = regexp.MustCompile(`[^a-z0-9-]+`)

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
