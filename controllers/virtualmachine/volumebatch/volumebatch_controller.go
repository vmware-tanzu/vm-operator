// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	controllerName = "volumebatch"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controllerName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("volumebatch"),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
		LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, &vmopv1.VirtualMachine{}, mgr.GetScheme()),
	})
	if err != nil {
		return err
	}

	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&vmopv1.VirtualMachine{},
		&handler.TypedEnqueueRequestForObject[*vmopv1.VirtualMachine]{},
	)); err != nil {
		return fmt.Errorf("failed to start VirtualMachine watch: %w", err)
	}

	// TODO (Oracle RAC): Update any comment or log message that includes
	// CnsNodeVmBatchAttachment to new format once the API format is changed.
	// Watch for changes to CnsNodeVmBatchAttachment, and enqueue a
	// request to the owner VirtualMachine.
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&cnsv1alpha1.CnsNodeVmBatchAttachment{},
		handler.TypedEnqueueRequestForOwner[*cnsv1alpha1.CnsNodeVmBatchAttachment](
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&vmopv1.VirtualMachine{},
			handler.OnlyControllerOwner(),
		),
	)); err != nil {
		return fmt.Errorf("failed to start CnsNodeVmBatchAttachment watch: %w", err)
	}

	return nil
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
		logger:     logger,
		recorder:   recorder,
		VMProvider: vmProvider,
	}
}

var _ reconcile.Reconciler = &Reconciler{}

type Reconciler struct {
	client.Client
	Context    context.Context
	logger     logr.Logger
	recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmbatchattachments,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmbatchattachments/status,verbs=get;list

// Reconcile reconciles a VirtualMachine object and processes the volumes for batch attachment.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, request.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	volCtx := &pkgctx.VolumeContext{
		Context: ctx,
		Logger:  pkglog.FromContextOrDefault(ctx),
		VM:      vm,
	}

	if metav1.HasAnnotation(vm.ObjectMeta, vmopv1.PauseAnnotation) {
		volCtx.Logger.Info("Skipping reconciliation since VirtualMachine contains the pause annotation")
		return ctrl.Result{}, nil
	}

	// If the VM has a pause reconcile label key, Skip volume reconciliation.
	if val, ok := vm.Labels[vmopv1.PausedVMLabelKey]; ok {
		volCtx.Logger.Info("Skipping reconciliation because a pause operation has been initiated on this VirtualMachine.",
			"pausedBy", val)
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", volCtx, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, vm); err != nil {
			if reterr == nil {
				reterr = err
			}
			volCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vm.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.ReconcileDelete(volCtx)
	}

	if err := r.ReconcileNormal(volCtx); err != nil {
		return ctrl.Result{}, err
	}

	// TODO: In case of instance storage volumes, we need to make
	// sure we queue the reconcile if any of the PVCs are not bound.

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VolumeContext) error {
	ctx.Logger.Info("Reconciling VirtualMachine for batch volume processing")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachine for batch volume processing")
	}()

	// Reconcile instance storage volumes
	if pkgcfg.FromContext(ctx).Features.InstanceStorage {
		ready, err := r.reconcileInstanceStoragePVCs(ctx)
		if err != nil || !ready {
			return err
		}
	}

	if ctx.VM.Status.BiosUUID == "" {
		// CNS requires the BiosUUID to match up the attachment request with the VM.
		if len(ctx.VM.Spec.Volumes) != 0 {
			ctx.Logger.Info("VM Status does not yet have BiosUUID. Deferring volume attachment")
		}
		return nil
	}

	// Get existing batch attachment for this VM
	batchAttachment, err := r.getBatchAttachmentForVM(ctx)
	if err != nil {
		return fmt.Errorf("error getting existing CnsNodeVmBatchAttachment for VM: %w", err)
	}

	// Only need to validate the hardware for once during the first time of
	// creating the batchAttachment.
	if batchAttachment == nil && len(ctx.VM.Spec.Volumes) > 0 {
		if err := r.validateHardwareVersion(ctx); err != nil {
			return fmt.Errorf("hardware version validation failed: %w", err)
		}
	}

	// Process volumes and create/update batch attachment
	if err := r.processBatchAttachment(ctx, batchAttachment); err != nil {
		return fmt.Errorf("error processing CnsNodeVmBatchAttachment: %w", err)
	}

	return nil
}

// getBatchAttachmentForVM returns the CnsNodeVmBatchAttachment resource for the
// VM. We assume that the name of the resource matches the name of the VM.
// Returns nil if no CNSNodeVMBatchAttachment resource exists for the VM.
func (r *Reconciler) getBatchAttachmentForVM(ctx *pkgctx.VolumeContext) (*cnsv1alpha1.CnsNodeVmBatchAttachment, error) {
	attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{}

	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      pkgutil.CNSBatchAttachmentNameForVolume(ctx.VM.Name),
		Namespace: ctx.VM.Namespace,
	}, attachment); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to find CnsNodeVmBatchAttachment: %w", err)
		}

		return nil, nil
	}

	// Ensure that the attachment is owned by the VM.
	if !metav1.IsControlledBy(attachment, ctx.VM) {
		return nil, fmt.Errorf("CnsNodeVmBatchAttachment %s has a different controlling owner",
			attachment.Name)
	}

	return attachment, nil
}

func (r *Reconciler) processBatchAttachment(
	ctx *pkgctx.VolumeContext,
	existingAttachment *cnsv1alpha1.CnsNodeVmBatchAttachment) error {

	// Filter volumes that have PVC source
	pvcVolumes := make([]vmopv1.VirtualMachineVolume, 0)
	for _, vol := range ctx.VM.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			pvcVolumes = append(pvcVolumes, vol)
		}
	}

	if len(pvcVolumes) == 0 {
		// No PVC volumes to process
		if existingAttachment != nil {
			ctx.Logger.Info("Delete existing CnsNodeVmBatchAttachment",
				"batchAttachment", existingAttachment.Name,
				"namespace", existingAttachment.Namespace)
			err := r.Client.Delete(ctx, existingAttachment)
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete CnsNodeVmBatchAttachment: %w", err)
			}
		}
		return nil
	}

	curVolumeAttachSpecMap := make(map[string]cnsv1alpha1.VolumeSpec)
	if existingAttachment != nil {
		for _, volSpec := range existingAttachment.Spec.Volumes {
			curVolumeAttachSpecMap[volSpec.Name] = volSpec
		}
	}
	var createErrs []error
	for _, vol := range pvcVolumes {
		if err := r.handlePVCWithWFFC(ctx, vol, curVolumeAttachSpecMap); err != nil {
			createErrs = append(createErrs, err)
		}
	}

	// Return early if the WFFC handling has error.
	// TODO (Oracle RAC): Do we reflect this as part of volume status for better
	// visibility to the user? Looks like old logic doesn't do it.
	if len(createErrs) > 0 {
		return apierrorsutil.NewAggregate(createErrs)
	}

	// Use robust error handling for volume processing
	if err := r.processVolumesWithErrorHandling(ctx, pvcVolumes); err != nil {
		ctx.Logger.Error(err, "Failed to process volumes with error handling")
		// Fall back to standard processing
	}

	// Create or update batch attachment
	if existingAttachment == nil {
		return r.createBatchAttachment(ctx, pvcVolumes)
	}

	return r.updateBatchAttachment(ctx, existingAttachment, pvcVolumes)
}

func (r *Reconciler) createBatchAttachment(
	ctx *pkgctx.VolumeContext,
	volumes []vmopv1.VirtualMachineVolume) error {

	attachmentName := pkgutil.CNSBatchAttachmentNameForVolume(ctx.VM.Name)

	volumeSpecs, err := r.buildVolumeSpecs(volumes)
	if err != nil {
		return fmt.Errorf("failed to build volume specs: %w", err)
	}

	// Validate the volume specs before creating the batch attachment
	if err := r.validateVolumeSpecs(ctx, volumeSpecs); err != nil {
		return fmt.Errorf("volume spec validation failed: %w", err)
	}

	attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      attachmentName,
			Namespace: ctx.VM.Namespace,
		},
		Spec: cnsv1alpha1.CnsNodeVmBatchAttachmentSpec{
			NodeUUID: ctx.VM.Status.BiosUUID,
			Volumes:  volumeSpecs,
		},
	}

	if err := controllerutil.SetControllerReference(ctx.VM, attachment, r.Client.Scheme()); err != nil {
		return fmt.Errorf("cannot set controller reference on CnsNodeVmBatchAttachment: %w", err)
	}

	if err := r.Create(ctx, attachment); err != nil {
		return fmt.Errorf("cannot create CnsNodeVmBatchAttachment: %w", err)
	}

	ctx.Logger.Info("Created CnsNodeVmBatchAttachment", "attachment", attachmentName)
	return nil
}

func (r *Reconciler) updateBatchAttachment(
	ctx *pkgctx.VolumeContext,
	existing *cnsv1alpha1.CnsNodeVmBatchAttachment,
	volumes []vmopv1.VirtualMachineVolume) error {

	volumeSpecs, err := r.buildVolumeSpecs(volumes)
	if err != nil {
		return fmt.Errorf("failed to build volume specs: %w", err)
	}

	// TODO (Oracle RAC): this should not just be called when the spec differs.
	// Check if update is needed.
	if r.volumeSpecsEqual(existing.Spec.Volumes, volumeSpecs) {
		// No update needed, just update VM status from existing attachment
		return r.updateVMStatusFromBatchAttachment(ctx, existing)
	}

	// Update the batch attachment with new volume specs
	existing.Spec.Volumes = volumeSpecs
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("cannot update CnsNodeVmBatchAttachment: %w", err)
	}

	ctx.Logger.Info("Updated CnsNodeVmBatchAttachment", "attachment", existing.Name)
	return nil
}

// buildVolumeSpecs builds a volume spec that will be used to create
// the CnsNodeVmBatchAttachment object.
func (r *Reconciler) buildVolumeSpecs(
	volumes []vmopv1.VirtualMachineVolume) ([]cnsv1alpha1.VolumeSpec, error) {

	volumeSpecs := make([]cnsv1alpha1.VolumeSpec, 0, len(volumes))

	for _, vol := range volumes {
		pvcSpec := vol.PersistentVolumeClaim

		// Map VM volume spec to CNS batch attachment spec
		cnsVolumeSpec := cnsv1alpha1.VolumeSpec{
			Name: vol.Name,
			PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimSpec{
				ClaimName: pvcSpec.ClaimName,
			},
		}

		// Apply application type presets first
		// Ideally, this would already have been mutated by the webhook, but just handle that here anyway.
		if err := r.applyApplicationTypePresets(pvcSpec, &cnsVolumeSpec); err != nil {
			return nil, fmt.Errorf("failed to apply application type presets for volume %s: %w", vol.Name, err)
		}

		// Map disk mode (can override application type presets)
		if pvcSpec.DiskMode != "" {
			switch pvcSpec.DiskMode {
			case vmopv1.VolumeDiskModePersistent:
				cnsVolumeSpec.PersistentVolumeClaim.DiskMode = cnsv1alpha1.Persistent
			case vmopv1.VolumeDiskModeIndependentPersistent:
				cnsVolumeSpec.PersistentVolumeClaim.DiskMode = cnsv1alpha1.IndependentPersistent
			default:
				return nil, fmt.Errorf("unsupported disk mode: %s for volume %s", pvcSpec.DiskMode, vol.Name)
			}
		}

		// Map sharing mode (can override application type presets)
		if pvcSpec.SharingMode != "" {
			switch pvcSpec.SharingMode {
			case vmopv1.VolumeSharingModeNone:
				cnsVolumeSpec.PersistentVolumeClaim.SharingMode = cnsv1alpha1.SharingNone
			case vmopv1.VolumeSharingModeMultiWriter:
				cnsVolumeSpec.PersistentVolumeClaim.SharingMode = cnsv1alpha1.SharingMultiWriter
			default:
				return nil, fmt.Errorf("unsupported sharing mode: %s for volume %s", pvcSpec.SharingMode, vol.Name)
			}
		}

		// Map controller key (combination of controller type and bus number)
		if pvcSpec.ControllerType != "" || pvcSpec.ControllerBusNumber != nil {
			controllerKey := pkgutil.BuildControllerKey(pvcSpec.ControllerType, pvcSpec.ControllerBusNumber)
			cnsVolumeSpec.PersistentVolumeClaim.ControllerKey = controllerKey
		}

		// Map unit number
		if pvcSpec.UnitNumber != nil {
			cnsVolumeSpec.PersistentVolumeClaim.UnitNumber = strconv.Itoa(int(*pvcSpec.UnitNumber))
		}

		volumeSpecs = append(volumeSpecs, cnsVolumeSpec)
	}

	return volumeSpecs, nil
}

func (r *Reconciler) applyApplicationTypePresets(
	pvcSpec *vmopv1.PersistentVolumeClaimVolumeSource,
	cnsSpec *cnsv1alpha1.VolumeSpec) error {

	switch pvcSpec.ApplicationType {
	case vmopv1.VolumeApplicationTypeOracleRAC:
		// OracleRAC preset: diskMode=IndependentPersistent, sharingMode=MultiWriter
		cnsSpec.PersistentVolumeClaim.DiskMode = cnsv1alpha1.IndependentPersistent
		cnsSpec.PersistentVolumeClaim.SharingMode = cnsv1alpha1.SharingMultiWriter

	case vmopv1.VolumeApplicationTypeMicrosoftWSFC:
		// MicrosoftWSFC preset: diskMode=IndependentPersistent
		// Note: Controller sharing mode requirements will be handled by CNS controller
		cnsSpec.PersistentVolumeClaim.DiskMode = cnsv1alpha1.IndependentPersistent

	case "":
		// No application type specified, use defaults
		break

	default:
		return fmt.Errorf("unsupported application type: %s", pvcSpec.ApplicationType)
	}

	return nil
}

func (r *Reconciler) volumeSpecsEqual(existing, desired []cnsv1alpha1.VolumeSpec) bool {

	if len(existing) != len(desired) {
		return false
	}

	existingMap := make(map[string]cnsv1alpha1.VolumeSpec)
	for _, vol := range existing {
		existingMap[vol.Name] = vol
	}

	for _, desiredVol := range desired {
		existingVol, exists := existingMap[desiredVol.Name]
		if !exists {
			return false
		}

		if !apiequality.Semantic.DeepEqual(existingVol, desiredVol) {
			return false
		}
	}

	return true
}

func (r *Reconciler) updateVMStatusFromBatchAttachment(
	ctx *pkgctx.VolumeContext,
	attachment *cnsv1alpha1.CnsNodeVmBatchAttachment) error {

	// Update VM.Status.Volumes based on the batch attachment status
	volumeStatuses := make([]vmopv1.VirtualMachineVolumeStatus, 0, len(ctx.VM.Status.Volumes))

	// Keep existing non-managed volumes (Classic type)
	for _, vol := range ctx.VM.Status.Volumes {
		if vol.Type == vmopv1.VolumeTypeClassic {
			volumeStatuses = append(volumeStatuses, vol)
		}
	}

	// Add managed volumes from batch attachment status
	for _, volStatus := range attachment.Status.VolumeStatus {
		vmVolStatus := vmopv1.VirtualMachineVolumeStatus{
			Name:     volStatus.Name,
			Type:     vmopv1.VolumeTypeManaged,
			Attached: volStatus.PersistentVolumeClaim.Attached,
			DiskUUID: volStatus.PersistentVolumeClaim.Diskuuid,
			Error:    volStatus.PersistentVolumeClaim.Error,
		}

		// Add PVC capacity information
		if err := r.updateVolumeStatusWithPVCInfo(ctx, volStatus.PersistentVolumeClaim.ClaimName, &vmVolStatus); err != nil {
			ctx.Logger.Error(err, "Failed to update PVC capacity info for volume",
				"volume", volStatus.Name, "pvc", volStatus.PersistentVolumeClaim.ClaimName)
		}

		// Sort the volume statuses to ensure consistent ordering.
		vmopv1.SortVirtualMachineVolumeStatuses(volumeStatuses)

		volumeStatuses = append(volumeStatuses, vmVolStatus)
	}

	ctx.VM.Status.Volumes = volumeStatuses
	return nil
}

func (r *Reconciler) updateVolumeStatusWithPVCInfo(
	ctx *pkgctx.VolumeContext,
	pvcName string, status *vmopv1.VirtualMachineVolumeStatus) error {

	// This is similar to the existing updateVolumeStatusWithLimitAndRequest function
	// but adapted for the batch controller

	pvc := &corev1.PersistentVolumeClaim{}
	pvcKey := client.ObjectKey{
		Namespace: ctx.VM.Namespace,
		Name:      pvcName,
	}

	if err := r.Get(ctx, pvcKey, pvc); err != nil {
		return err
	}

	if v, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		// Use the request if it exists.
		status.Requested = &v
	}

	if v, ok := pvc.Spec.Resources.Limits[corev1.ResourceStorage]; ok {
		// Use the limit if it exists.
		status.Limit = &v
	} else {
		// Otherwise use the requested capacity.
		status.Limit = status.Requested
	}

	return nil
}

func (r *Reconciler) ReconcileDelete(_ *pkgctx.VolumeContext) error {
	// Do nothing here since we depend on the Garbage Collector to do the
	// deletion of the dependent CNSNodeVMBatchAttachment objects when their
	// owning VM is deleted.
	// We require the Volume provider to handle the situation where the VM is
	// deleted before the volumes are detached & removed.

	return nil
}

// reconcileInstanceStoragePVCs handles instance storage PVC lifecycle management.
// This provides feature parity with the v1 controller's instance storage support.
func (r *Reconciler) reconcileInstanceStoragePVCs(_ *pkgctx.VolumeContext) (bool, error) {
	// TODO: Implement instance storage PVC reconciliation
	// This method should:
	// - Create missing instance storage PVCs
	// - Handle PVC binding and placement
	// - Manage instance storage annotations
	// - Handle placement failures and cleanup

	return true, nil
}

// handlePVCWithWFFC handles PVCs with WaitForFirstConsumer binding mode.
// This ensures proper node selection for storage classes requiring it.
func (r *Reconciler) handlePVCWithWFFC(
	ctx *pkgctx.VolumeContext,
	volume vmopv1.VirtualMachineVolume,
	curVolumeAttachSpecMap map[string]cnsv1alpha1.VolumeSpec,
) error {

	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.InstanceVolumeClaim != nil {
		return nil
	}

	if s, ok := curVolumeAttachSpecMap[volume.PersistentVolumeClaim.ClaimName]; ok &&
		volume.PersistentVolumeClaim.ClaimName == s.PersistentVolumeClaim.ClaimName {
		// Skip the volumes that's already in BatchAttachment spec and still
		// points to the same PVC.
		return nil
	}

	pvc := corev1.PersistentVolumeClaim{}
	pvcKey := client.ObjectKey{
		Namespace: ctx.VM.Namespace,
		Name:      volume.PersistentVolumeClaim.ClaimName,
	}

	if err := r.Get(ctx, pvcKey, &pvc); err != nil {
		return fmt.Errorf("cannot get PVC: %w", err)
	}

	if pvc.Status.Phase == corev1.ClaimBound {
		// Regardless of the StorageClass binding mode there is nothing to done if already bound.
		return nil
	}

	if pvc.Annotations[storagehelpers.AnnSelectedNode] != "" {
		// Once set, this annotation cannot really be changed so just keep going.
		return nil
	}

	scName := pvc.Spec.StorageClassName
	if scName == nil {
		return fmt.Errorf("PVC %s does not have StorageClassName set", pvc.Name)
	} else if *scName == "" {
		return nil
	}

	sc := &storagev1.StorageClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: *scName}, sc); err != nil {
		return fmt.Errorf("cannot get StorageClass for PVC %s: %w", pvc.Name, err)
	}

	if mode := sc.VolumeBindingMode; mode == nil ||
		*mode != storagev1.VolumeBindingWaitForFirstConsumer {
		return nil
	}

	if !pkgcfg.FromContext(ctx).Features.VMWaitForFirstConsumerPVC {
		return errors.New("PVC with WFFC storage class support is not enabled")
	}

	zoneName := ctx.VM.Status.Zone
	if zoneName == "" {
		// Fallback to the label value if Status hasn't been updated yet.
		zoneName = ctx.VM.Labels[corev1.LabelTopologyZone]
		if zoneName == "" {
			return errors.New("VM does not have Zone set")
		}
	}

	if pvc.Annotations == nil {
		pvc.Annotations = map[string]string{}
	}
	pvc.Annotations[constants.CNSSelectedNodeIsZoneAnnotationKey] = "true"
	pvc.Annotations[storagehelpers.AnnSelectedNode] = zoneName

	if err := r.Client.Update(ctx, &pvc); err != nil {
		return fmt.Errorf("cannot update PVC to add selected-node annotation: %w", err)
	}

	return nil
}

// processVolumesWithErrorHandling processes volumes with robust error handling.
// This ensures partial failures don't prevent other volumes from being processed.
func (r *Reconciler) processVolumesWithErrorHandling(_ *pkgctx.VolumeContext, _ []vmopv1.VirtualMachineVolume) error {
	// TODO: Implement robust volume processing with error aggregation
	// This method should:
	// - Process each volume independently
	// - Collect and aggregate errors from individual volume processing
	// - Continue processing remaining volumes on partial failures
	// - Provide detailed error information for troubleshooting
	// - Filter volumes that are not bound, or have an error.

	// TODO: The prior volume controller only allowed one volume to
	// be attached when the VM was being powered on to maintain a
	// consistent ordering since services like VKS may specify the
	// disks in their cloud-config, so the ordering is super critical.

	return nil
}

// validateHardwareVersion validates that the VM hardware version supports requested features.
// This ensures compatibility between VM hardware version and volume features.
func (r *Reconciler) validateHardwareVersion(ctx *pkgctx.VolumeContext) error {
	hardwareVersion, err := r.VMProvider.GetVirtualMachineHardwareVersion(ctx, ctx.VM)
	if err != nil {
		return fmt.Errorf("failed to get VM hardware version: %w", err)
	}

	// If hardware version is 0, which means we failed to parse the version
	// from VM, then just assume that it is above minimal requirement.
	if hardwareVersion.IsValid() && hardwareVersion < pkgconst.MinSupportedHWVersionForPVC {
		retErr := fmt.Errorf("vm has an unsupported "+
			"hardware version %d for PersistentVolumes. "+
			"Minimum supported hardware version %d",
			hardwareVersion, pkgconst.MinSupportedHWVersionForPVC)
		r.recorder.EmitEvent(ctx.VM, "VolumeAttachment", retErr, true)
		return retErr
	}

	return nil
}

// validateVolumeSpecs validates that the volume specs are valid for attachment.
// This method validates controller capacity, unit number conflicts, and other constraints.
// It requires access to the VM's ConfigInfo from vSphere to validate against current controller
// configuration and existing disk attachments.
func (r *Reconciler) validateVolumeSpecs(_ *pkgctx.VolumeContext, _ []cnsv1alpha1.VolumeSpec) error {
	// TODO: Implement validation logic
	// This method will validate:
	// - Controller unit number availability (ensure unit numbers don't exceed controller capacity)
	// - No duplicate unit numbers within the same controller
	// - No conflicts with existing attached volumes
	// - Valid controller type and bus number combinations
	// - Controller capacity limits based on controller type

	return nil
}
