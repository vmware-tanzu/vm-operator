// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"

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

	// Set up field index for CnsNodeVmAttachment by NodeUUID to efficiently query legacy attachments
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&cnsv1alpha1.CnsNodeVmAttachment{},
		"spec.nodeuuid",
		func(rawObj client.Object) []string {
			attachment := rawObj.(*cnsv1alpha1.CnsNodeVmAttachment)
			return []string{attachment.Spec.NodeUUID}
		}); err != nil {
		return err
	}

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
	// CnsNodeVMBatchAttachment to new format once the API format is changed.
	// Watch for changes to CnsNodeVMBatchAttachment, and enqueue a
	// request to the owner VirtualMachine.
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&cnsv1alpha1.CnsNodeVMBatchAttachment{},
		handler.TypedEnqueueRequestForOwner[*cnsv1alpha1.CnsNodeVMBatchAttachment](
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&vmopv1.VirtualMachine{},
			handler.OnlyControllerOwner(),
		),
	)); err != nil {
		return fmt.Errorf("failed to start CnsNodeVMBatchAttachment watch: %w", err)
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
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments,verbs=delete;get;list;watch

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
		volCtx.Logger.Info("Skipping reconciliation because a pause operation "+
			"has been initiated on this VirtualMachine.",
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
		return pkgerr.ResultFromError(err)
	}

	// TODO: In case of instance storage volumes, we need to make
	// sure we queue the reconcile if any of the PVCs are not bound.

	return ctrl.Result{}, nil
}

func errOrNoRequeueErr(err1, err2 error) error {
	if err1 == nil {
		return err2
	}

	if err2 == nil {
		return err1
	}

	mergedErr := fmt.Errorf("%w, %w", err1, err2)

	if pkgerr.IsNoRequeueError(err1) && pkgerr.IsNoRequeueError(err2) {
		return pkgerr.NoRequeueError{
			Message: mergedErr.Error(),
			DoNotErr: pkgerr.IsNoRequeueNoError(err1) &&
				pkgerr.IsNoRequeueNoError(err2),
		}
	}

	return mergedErr
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

	if ctx.VM.Status.InstanceUUID == "" {
		// CSI requires the InstanceUUID to match up the batch
		// attachment request with the VM.
		if len(ctx.VM.Spec.Volumes) != 0 {
			ctx.Logger.Info("VM Status does not yet have InstanceUUID. Deferring volume attachment")
		}
		return nil
	}

	if ctx.VM.Status.BiosUUID == "" {
		// CSI requires the BiosUUID to match up the legacy attachment
		// request with the VM.
		if len(ctx.VM.Spec.Volumes) != 0 {
			ctx.Logger.Info("VM Status does not yet have BiosUUID. Deferring volume attachment")
		}
		return nil
	}

	// Get existing batch attachment for this VM
	batchAttachment, err := r.getBatchAttachmentForVM(ctx)
	if err != nil {
		return fmt.Errorf("error getting existing CnsNodeVMBatchAttachment for VM: %w", err)
	}

	// Only need to validate the hardware for once during the first time of
	// creating the batchAttachment.
	if batchAttachment == nil && len(ctx.VM.Spec.Volumes) > 0 {
		if err := r.validateHardwareVersion(ctx); err != nil {
			return fmt.Errorf("hardware version validation failed: %w", err)
		}
	}

	legacyAttachments, err := pkgutil.GetCnsNodeVMAttachmentsForVM(ctx, r.Client, ctx.VM)
	if err != nil {
		return fmt.Errorf(
			"error getting existing CnsNodeVmAttachments for VM: %w", err)
	}

	// Get legacy CnsNodeVmAttachments for this VM. We need to handle
	// detachments via this resource for the brownfield VMs.
	attachmentsToDelete := r.attachmentsToDelete(ctx, legacyAttachments)

	// Delete attachments for this VM that exist but are not currently referenced in the Spec.
	deleteErr := r.deleteOrphanedAttachments(ctx, attachmentsToDelete)
	if deleteErr != nil {
		ctx.Logger.Error(deleteErr, "Error deleting orphaned CnsNodeVmAttachments")
		// Keep going to the create/update processing below.
		//
		// This is the to maintain the behavior of the existing
		// volume controller. In batch processing, we will skip
		// any volume that has a corresponding attachment, so we
		// should not land in a situation where the attachment is
		// tracked by both legacy and batch methods.
	}

	// Process volumes and create/update batch attachment
	processErr := r.processBatchAttachment(ctx, batchAttachment, legacyAttachments)
	if processErr != nil {
		ctx.Logger.Error(processErr, "Error processing CnsNodeVmBatchAttachments")
		// Keep going to return aggregated error below.
	}

	return errOrNoRequeueErr(deleteErr, processErr)
}

// getBatchAttachmentForVM returns the CnsNodeVMBatchAttachment resource for the
// VM. We assume that the name of the resource matches the name of the VM.
// Returns nil if no CNSNodeVMBatchAttachment resource exists for the VM.
func (r *Reconciler) getBatchAttachmentForVM(
	ctx *pkgctx.VolumeContext,
) (*cnsv1alpha1.CnsNodeVMBatchAttachment, error) {

	attachment := &cnsv1alpha1.CnsNodeVMBatchAttachment{}

	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      pkgutil.CNSBatchAttachmentNameForVM(ctx.VM.Name),
		Namespace: ctx.VM.Namespace,
	}, attachment); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to find CnsNodeVMBatchAttachment: %w", err)
		}

		return nil, nil
	}

	// Ensure that the attachment is owned by the VM.
	if !metav1.IsControlledBy(attachment, ctx.VM) {
		return nil, fmt.Errorf("CnsNodeVMBatchAttachment %s has a different controlling owner",
			attachment.Name)
	}

	return attachment, nil
}

func (r *Reconciler) processBatchAttachment(
	ctx *pkgctx.VolumeContext,
	existingAttachment *cnsv1alpha1.CnsNodeVMBatchAttachment,
	legacyAttachments map[string]cnsv1alpha1.CnsNodeVmAttachment) error {

	// Filter volumes that have PVC source and are not already tracked
	// by legacy CnsNodeVmAttachment
	pvcVolumes := make([]vmopv1.VirtualMachineVolume, 0)
	for _, vol := range ctx.VM.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			// Check if this volume is already managed by a legacy
			// CnsNodeVmAttachment. It is safe to rely on the
			// attachment name for the volume since that's a contract
			// with CSI.
			attachmentNameForVol := pkgutil.CNSAttachmentNameForVolume(ctx.VM.Name, vol.Name)
			if legacyAttachment, ok := legacyAttachments[attachmentNameForVol]; ok {
				// Only skip the volume if the legacy attachment's PVC name matches
				// the current volume's PVC name. If they don't match, the legacy
				// attachment is stale and will be deleted, so we should include
				// this volume in the batch.
				if legacyAttachment.Spec.VolumeName == vol.PersistentVolumeClaim.ClaimName {
					ctx.Logger.V(4).Info("skipping volume since it is tracked by a CnsNodeVmAttachment",
						"volume", vol.Name)
					continue
				}
			}

			// Only include greenfield volumes that are not tracked by
			// legacy CnsNodeVmAttachment
			pvcVolumes = append(pvcVolumes, vol)
		}
	}

	if len(pvcVolumes) == 0 {
		// No PVC volumes to process
		if existingAttachment != nil {
			ctx.Logger.Info("Delete existing CnsNodeVMBatchAttachment",
				"batchAttachment", existingAttachment.Name,
				"namespace", existingAttachment.Namespace)
			err := r.Client.Delete(ctx, existingAttachment)
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete CnsNodeVMBatchAttachment: %w", err)
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

	var (
		retErr           error
		toBeBuiltPvcVols = make([]vmopv1.VirtualMachineVolume, 0)
	)

	for _, vol := range pvcVolumes {
		if err := r.handlePVCWithWFFC(ctx, vol, curVolumeAttachSpecMap); err != nil {
			retErr = errOrNoRequeueErr(retErr, err)
		} else {
			toBeBuiltPvcVols = append(toBeBuiltPvcVols, vol)
		}
	}

	volumeSpecs, err := r.buildVolumeSpecs(toBeBuiltPvcVols, ctx.VM.Status.Hardware)
	if err != nil {
		retErr = errOrNoRequeueErr(retErr, err)
	}

	// Create or update batch attachment
	if err := r.CreateOrUpdateBatchAttachment(
		ctx, existingAttachment, volumeSpecs); err != nil {
		retErr = errOrNoRequeueErr(retErr, err)
	}

	return retErr
}

// CreateOrUpdateBatchAttachment handles the creation or update of
// CnsNodeVMBatchAttachment.
func (r *Reconciler) CreateOrUpdateBatchAttachment(
	ctx *pkgctx.VolumeContext,
	existingBatchAttachment *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volumeSpecs []cnsv1alpha1.VolumeSpec) error {

	// Validate the volume specs before attempting to create/update
	if err := r.validateVolumeSpecs(ctx, volumeSpecs); err != nil {
		return fmt.Errorf("volume spec validation failed: %w", err)
	}

	vm := ctx.VM
	attachmentName := pkgutil.CNSBatchAttachmentNameForVM(vm.Name)
	batchAttachment := &cnsv1alpha1.CnsNodeVMBatchAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      attachmentName,
			Namespace: vm.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(
		ctx,
		r.Client,
		batchAttachment,
		func() error {
			if err := controllerutil.SetControllerReference(
				vm, batchAttachment, r.Client.Scheme(),
			); err != nil {
				return fmt.Errorf("failed to set controller reference "+
					"on CnsNodeVMBatchAttachment: %w", err)
			}

			// Update the Spec with the desired volumeSpecs
			batchAttachment.Spec = cnsv1alpha1.CnsNodeVMBatchAttachmentSpec{
				NodeUUID: vm.Status.InstanceUUID,
				Volumes:  volumeSpecs,
			}

			return nil
		})

	if err != nil {
		return fmt.Errorf("failed to create or patch CnsNodeVMBatchAttachment %s: %w",
			attachmentName, err)
	}

	switch operationResult {
	case controllerutil.OperationResultCreated:
		ctx.Logger.Info("Created CnsNodeVMBatchAttachment",
			"attachment", attachmentName)
	case controllerutil.OperationResultUpdated:
		ctx.Logger.Info("Updated CnsNodeVMBatchAttachment",
			"attachment", attachmentName)
	}

	// Update VM.status.volumes after BatchAttachment is created or updated.
	r.updateVMVolumeStatus(ctx, existingBatchAttachment, volumeSpecs)

	return nil
}

// buildVolumeSpecs builds a volume spec that will be used to create
// the CnsNodeVMBatchAttachment object.
func (r *Reconciler) buildVolumeSpecs(
	volumes []vmopv1.VirtualMachineVolume,
	hardware *vmopv1.VirtualMachineHardwareStatus,
) ([]cnsv1alpha1.VolumeSpec, error) {

	// Return nil and wait for next reconcile when the status is updated if
	// hardware is nil Or noops when there is no volumes to be attached.
	if hardware == nil || len(volumes) == 0 {
		return nil, nil
	}

	var (
		buildErrMsg   = "failed to build volume specs:"
		volumeSpecs   = make([]cnsv1alpha1.VolumeSpec, 0, len(volumes))
		ctrlDevKeyMap = make(map[pkgutil.ControllerID]int32)
		retErr        error
	)

	for _, ctrlStatus := range hardware.Controllers {
		ctrlDevKeyMap[pkgutil.ControllerID{
			ControllerType: ctrlStatus.Type,
			BusNumber:      ctrlStatus.BusNumber,
		}] = ctrlStatus.DeviceKey
	}

	for _, vol := range volumes {

		pvcSpec := vol.PersistentVolumeClaim

		// The validating webhook should have verified it already.
		// It returns NoRequeueError because we do not want to keep reconciling
		// volume with incorrect spec unless the spec is fixed.
		if pvcSpec.ControllerBusNumber == nil {
			retErr = errOrNoRequeueErr(retErr, pkgerr.NoRequeueError{Message: fmt.Sprintf(
				"%s volume %q is missing controller bus number", buildErrMsg, vol.Name)})
			continue
		}

		ctrlDevKey, ok := ctrlDevKeyMap[pkgutil.ControllerID{
			ControllerType: pvcSpec.ControllerType,
			BusNumber:      *pvcSpec.ControllerBusNumber,
		}]
		if !ok {
			retErr = errOrNoRequeueErr(retErr, pkgerr.NoRequeueError{Message: fmt.Sprintf(
				"%s wating for the device controller %q %q to be created for volume %q",
				buildErrMsg, pvcSpec.ControllerType, *pvcSpec.ControllerBusNumber, vol.Name)})
			continue
		}

		// Map VM volume spec to CNS batch attachment spec
		cnsVolumeSpec := cnsv1alpha1.VolumeSpec{
			Name: vol.Name,
			PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimSpec{
				ClaimName:     pvcSpec.ClaimName,
				ControllerKey: ptr.To(ctrlDevKey),
			},
		}

		if pvcSpec.UnitNumber != nil {
			cnsVolumeSpec.PersistentVolumeClaim.UnitNumber = pvcSpec.UnitNumber
		}

		// Apply application type presets first
		// Ideally, this would already have been mutated by the webhook, but just handle that here anyway.
		if err := r.applyApplicationTypePresets(pvcSpec, &cnsVolumeSpec); err != nil {

			retErr = errOrNoRequeueErr(retErr, pkgerr.NoRequeueError{Message: fmt.Errorf(
				"%s failed to apply application type presets for volume %s: %w",
				buildErrMsg, vol.Name, err).Error()})

			continue
		}

		// Map disk mode (can override application type presets)
		switch pvcSpec.DiskMode {
		case vmopv1.VolumeDiskModePersistent:
			cnsVolumeSpec.PersistentVolumeClaim.DiskMode = cnsv1alpha1.Persistent
		case vmopv1.VolumeDiskModeIndependentPersistent:
			cnsVolumeSpec.PersistentVolumeClaim.DiskMode = cnsv1alpha1.IndependentPersistent
		default:
			retErr = errOrNoRequeueErr(retErr, pkgerr.NoRequeueError{
				Message: fmt.Sprintf("%s unsupported disk mode: %s for volume %s",
					buildErrMsg, pvcSpec.DiskMode, vol.Name)})
			continue
		}

		// Map sharing mode (can override application type presets)
		switch pvcSpec.SharingMode {
		case vmopv1.VolumeSharingModeNone:
			cnsVolumeSpec.PersistentVolumeClaim.SharingMode = cnsv1alpha1.SharingNone
		case vmopv1.VolumeSharingModeMultiWriter:
			cnsVolumeSpec.PersistentVolumeClaim.SharingMode = cnsv1alpha1.SharingMultiWriter
		default:
			retErr = errOrNoRequeueErr(retErr, pkgerr.NoRequeueError{
				Message: fmt.Sprintf("%s unsupported sharing mode: %s for volume %s",
					buildErrMsg, pvcSpec.DiskMode, vol.Name)})
			continue
		}

		volumeSpecs = append(volumeSpecs, cnsVolumeSpec)
	}

	return volumeSpecs, retErr
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

// updateVMVolumeStatus updates managed volumes from VM.status.volumes list
// with data extracted from existingBatchAttachment.status.volumeStatus
// and volumeSpecs.
// If there is existing volume status in batchAttachment.status and volume's PVC hasn't been
// changed, then just construct a detailed vmVolStatus using those info.
// Otherwise just add a basic status.
// In the end add both managed volumes and unmanaged volumes to vm.status.volumes
// and sort this array based on diskUUID.
func (r *Reconciler) updateVMVolumeStatus(
	ctx *pkgctx.VolumeContext,
	existingAttachment *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volumeSpecs []cnsv1alpha1.VolumeSpec) {

	// Update VM.Status.Volumes based on the batch attachment status
	volumeStatuses := make([]vmopv1.VirtualMachineVolumeStatus, 0, len(ctx.VM.Status.Volumes))

	existingAttachVolStatus := make(map[string]cnsv1alpha1.VolumeStatus)
	if existingAttachment != nil {
		for _, volStatus := range existingAttachment.Status.VolumeStatus {
			existingAttachVolStatus[volStatus.Name] = volStatus
		}
	}

	existingVMManagedVolStatus := map[string]vmopv1.VirtualMachineVolumeStatus{}
	for i := range ctx.VM.Status.Volumes {
		vol := ctx.VM.Status.Volumes[i]
		if vol.Type != vmopv1.VolumeTypeClassic {
			existingVMManagedVolStatus[vol.Name] = vol
		}
	}

	for _, vol := range volumeSpecs {
		// By default just add a basic status.
		vmVolStatus := vmopv1.VirtualMachineVolumeStatus{
			Name: vol.Name,
			Type: vmopv1.VolumeTypeManaged,
		}

		// If the batchAttachment.status already has this volume, and its
		// PVC hasn't been changed, get its detailed info from vm.status.vol
		// and batchAttachment.status.vol.
		if volStatus, ok := existingAttachVolStatus[vol.Name]; ok &&
			vol.PersistentVolumeClaim.ClaimName == volStatus.PersistentVolumeClaim.ClaimName {

			vmVolStatus = vmopv1.VirtualMachineVolumeStatus{
				Name:     volStatus.Name,
				Type:     vmopv1.VolumeTypeManaged,
				Attached: volStatus.PersistentVolumeClaim.Attached,
				DiskUUID: volStatus.PersistentVolumeClaim.DiskUUID,
				Error:    pkgutil.SanitizeCNSErrorMessage(volStatus.PersistentVolumeClaim.Error),
				Used:     existingVMManagedVolStatus[vol.Name].Used,
				Crypto:   existingVMManagedVolStatus[vol.Name].Crypto,
			}

			// Add PVC capacity information
			if err := r.updateVolumeStatusWithPVCInfo(
				ctx,
				volStatus.PersistentVolumeClaim.ClaimName,
				&vmVolStatus); err != nil {
				ctx.Logger.Error(err, "failed to get volume status limit")
			}
		}

		volumeStatuses = append(volumeStatuses, vmVolStatus)
	}

	// Remove any managed volumes from the existing status.
	ctx.VM.Status.Volumes = slices.DeleteFunc(ctx.VM.Status.Volumes,
		func(e vmopv1.VirtualMachineVolumeStatus) bool {
			return e.Type != vmopv1.VolumeTypeClassic
		})

	ctx.VM.Status.Volumes = append(ctx.VM.Status.Volumes, volumeStatuses...)

	// Sort the volume statuses to ensure consistent ordering.
	vmopv1.SortVirtualMachineVolumeStatuses(ctx.VM.Status.Volumes)
}

func (r *Reconciler) updateVolumeStatusWithPVCInfo(
	ctx *pkgctx.VolumeContext,
	pvcName string, status *vmopv1.VirtualMachineVolumeStatus) error {

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

func (r *Reconciler) deleteOrphanedAttachments(
	ctx *pkgctx.VolumeContext,
	attachments []cnsv1alpha1.CnsNodeVmAttachment) error {

	var retErr error

	for i := range attachments {
		attachment := attachments[i]

		if !attachment.DeletionTimestamp.IsZero() {
			continue
		}

		ctx.Logger.V(2).Info("Deleting orphaned CnsNodeVmAttachment", "attachment", attachment.Name)
		if err := r.Delete(ctx, &attachment); err != nil {
			if !apierrors.IsNotFound(err) {
				retErr = errOrNoRequeueErr(retErr, err)
			}
		}
	}

	return retErr
}

func (r *Reconciler) attachmentsToDelete(
	ctx *pkgctx.VolumeContext,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment) []cnsv1alpha1.CnsNodeVmAttachment {

	if len(attachments) == 0 {
		return nil
	}

	expectedAttachments := make(map[string]string, len(ctx.VM.Spec.Volumes))
	for _, volume := range ctx.VM.Spec.Volumes {
		// Only process CNS volumes here.
		if volume.PersistentVolumeClaim != nil {
			attachmentName := pkgutil.CNSAttachmentNameForVolume(ctx.VM.Name, volume.Name)
			expectedAttachments[attachmentName] = volume.PersistentVolumeClaim.ClaimName
		}
	}

	// From the existing attachment map, determine which ones shouldn't exist anymore from
	// the volumes Spec.
	var attachmentsToDelete []cnsv1alpha1.CnsNodeVmAttachment
	for _, attachment := range attachments {
		if claimName, exists := expectedAttachments[attachment.Name]; !exists || claimName != attachment.Spec.VolumeName {
			attachmentsToDelete = append(attachmentsToDelete, attachment)
		}
	}

	return attachmentsToDelete
}
