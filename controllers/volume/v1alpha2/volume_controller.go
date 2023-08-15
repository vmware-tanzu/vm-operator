// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	goctx "context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	patch "github.com/vmware-tanzu/vm-operator/pkg/patch2"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
)

const (
	AttributeFirstClassDiskUUID = "diskUUID"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerName      = "volume"
		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controllerName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("volume"),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProviderA2,
	)

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachine.
	err = c.Watch(&source.Kind{Type: &vmopv1.VirtualMachine{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes for CnsNodeVmAttachment, and enqueue VirtualMachine which is the owner of CnsNodeVmAttachment.
	err = c.Watch(&source.Kind{Type: &cnsv1alpha1.CnsNodeVmAttachment{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &vmopv1.VirtualMachine{},
		IsController: true,
	})
	if err != nil {
		return err
	}

	// Watch for changes for PersistentVolumeClaim, and enqueue VirtualMachine which is the owner of PersistentVolumeClaim.
	err = c.Watch(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &vmopv1.VirtualMachine{},
		IsController: true,
	})
	if err != nil {
		return err
	}

	return nil
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterfaceA2) *Reconciler {
	return &Reconciler{
		Client:     client,
		logger:     logger,
		recorder:   recorder,
		VMProvider: vmProvider,
	}
}

var _ reconcile.Reconciler = &Reconciler{}

type Reconciler struct {
	client.Client
	logger     logr.Logger
	recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterfaceA2
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments/status,verbs=get;list
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=create;delete;get;list;watch;patch;update

// Reconcile reconciles a VirtualMachine object and processes the volumes for attach/detach.
// Longer term, this should be folded back into the VirtualMachine controller, but exists as
// a separate controller to ensure volume attachments are processed promptly, since the VM
// controller can block for a long time, consuming all of the workers.
func (r *Reconciler) Reconcile(ctx goctx.Context, request ctrl.Request) (_ ctrl.Result, reterr error) {
	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, request.NamespacedName, vm); err != nil {
		if apiErrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	volCtx := &context.VolumeContextA2{
		Context:                   ctx,
		Logger:                    ctrl.Log.WithName("Volumes").WithValues("name", vm.NamespacedName()),
		VM:                        vm,
		InstanceStorageFSSEnabled: lib.IsInstanceStorageFSSEnabled(),
	}

	// If the VM has a pause reconcile annotation, it is being restored on vCenter. Return here so our reconcile
	// does not replace the VM being restored on the vCenter inventory.
	//
	// Do not requeue the reconcile here since removing the pause annotation will trigger a reconcile anyway.
	if _, ok := volCtx.VM.Annotations[vmopv1.PauseAnnotation]; ok {
		volCtx.Logger.Info("Skipping reconcile since Pause annotation is set on the VM")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", volCtx.String())
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
	return r.reconcileResult(volCtx), nil
}

func (r *Reconciler) reconcileResult(ctx *context.VolumeContextA2) ctrl.Result {
	if ctx.InstanceStorageFSSEnabled {
		// Requeue the request if all instance storage PVCs are not bound.
		_, pvcsBound := ctx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]
		if IsConfiguredA2(ctx.VM) && !pvcsBound {
			return ctrl.Result{RequeueAfter: lib.GetInstanceStorageRequeueDelay()}
		}
	}

	return ctrl.Result{}
}

func (r *Reconciler) ReconcileDelete(_ *context.VolumeContextA2) error {
	// Do nothing here since we depend on the Garbage Collector to do the deletion of the
	// dependent CNSNodeVMAttachment objects when their owning VM is deleted.
	// We require the Volume provider to handle the situation where the VM is deleted before
	// the volumes are detached & removed.
	return nil
}

func (r *Reconciler) ReconcileNormal(ctx *context.VolumeContextA2) error {
	ctx.Logger.Info("Reconciling VirtualMachine for processing volumes")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachine for processing volumes")
	}()

	if ctx.InstanceStorageFSSEnabled {
		ready, err := r.reconcileInstanceStoragePVCs(ctx)
		if err != nil || !ready {
			return err
		}
	}

	if ctx.VM.Status.BiosUUID == "" {
		// CSI requires the BiosUUID to match up the attachment request with the VM. Defer here
		// until it is set by the VirtualMachine controller.
		ctx.Logger.Info("VM Status does not yet have BiosUUID. Deferring volume attachment")
		return nil
	}

	attachments, err := r.getAttachmentsForVM(ctx)
	if err != nil {
		ctx.Logger.Error(err, "Error getting existing CnsNodeVmAttachments for VM")
		return err
	}

	attachmentsToDelete := r.attachmentsToDelete(ctx, attachments)

	// Delete attachments for this VM that exist but are not currently referenced in the Spec.
	deleteErr := r.deleteOrphanedAttachments(ctx, attachmentsToDelete)
	if deleteErr != nil {
		ctx.Logger.Error(deleteErr, "Error deleting orphaned CnsNodeVmAttachments")
		// Keep going to the create/update processing below.
	}

	// Process attachments, creating when needed, and updating the VM Status Volumes.
	processErr := r.processAttachments(ctx, attachments, attachmentsToDelete)
	if processErr != nil {
		ctx.Logger.Error(processErr, "Error processing CnsNodeVmAttachments")
		// Keep going to return aggregated error below.
	}

	return k8serrors.NewAggregate([]error{deleteErr, processErr})
}

func (r *Reconciler) reconcileInstanceStoragePVCs(ctx *context.VolumeContextA2) (bool, error) {
	// NOTE: We could check for InstanceStoragePVCsBoundAnnotationKey here and short circuit
	// all of this. Might leave stale PVCs though. Need to think more: instance storage is
	// this odd quasi reconcilable thing.

	// If the VM Spec doesn't have any instance storage volumes, there is nothing for us to do.
	// We do not support removing - or changing really - this type of volume.
	isVolumes := FilterVolumesA2(ctx.VM)
	if len(isVolumes) == 0 {
		return true, nil
	}

	pvcList, getErrs := r.getInstanceStoragePVCs(ctx, isVolumes)
	if getErrs != nil {
		return false, k8serrors.NewAggregate(getErrs)
	}

	expectedVolumesMap := map[string]struct{}{}
	for _, vol := range isVolumes {
		expectedVolumesMap[vol.Name] = struct{}{}
	}

	var (
		stalePVCs  []client.ObjectKey
		createErrs []error
	)
	existingVolumesMap := map[string]struct{}{}
	failedVolumesMap := map[string]struct{}{}
	boundCount := 0
	selectedNode := ctx.VM.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey]
	createPVCs := len(selectedNode) > 0

	for _, pvc := range pvcList {
		pvc := pvc

		if !pvc.DeletionTimestamp.IsZero() {
			// Ignore PVC that is being deleted. Likely this is from a previous failed
			// placement and CSI hasn't fully cleaned up yet (a finalizer is still present).
			// NOTE: Don't add this to existingVolumesMap[], so we'll try to create in case
			// our cache is stale.
			continue
		}

		if !metav1.IsControlledBy(&pvc, ctx.VM) {
			// This PVC's OwnerRef doesn't match with VM resource UUID. This shouldn't happen
			// since PVCs are always created with OwnerRef as well as Controller watch filters
			// out non instance storage PVCs. Ignore it.
			continue
		}

		existingVolumesMap[pvc.Name] = struct{}{}

		pvcNode, exists := pvc.Annotations[constants.KubernetesSelectedNodeAnnotationKey]
		if !exists || pvcNode != selectedNode {
			// This PVC is ours but NOT on our selected node. Likely, placement previously failed
			// and we're trying again on a different node.
			// NOTE: This includes even when selectedNode is "". Bias for full cleanup.
			stalePVCs = append(stalePVCs, client.ObjectKeyFromObject(&pvc))
			continue
		}

		if instanceStoragePVCFailed(&pvc) {
			// This PVC is ours but has failed. This instance storage placement is doomed.
			failedVolumesMap[pvc.Name] = struct{}{}
			continue
		}

		if pvc.Status.Phase != corev1.ClaimBound {
			// CSI is still processing this PVC.
			continue
		}

		// This PVC is successfully bound to our selected host, and is ready for attachment.
		boundCount++
	}

	placementFailed := len(failedVolumesMap) > 0
	if placementFailed {
		// Need to start placement over. PVCs successfully realized are recreated or
		// retailed depending on the next host selection.
		return false, r.instanceStoragePlacementFailed(ctx, failedVolumesMap)
	}

	deleteErrs := r.deleteInstanceStoragePVCs(ctx, stalePVCs)
	if createPVCs {
		createErrs = r.createMissingInstanceStoragePVCs(ctx, isVolumes, existingVolumesMap, selectedNode)
	}

	fullyBound := boundCount == len(isVolumes)
	if fullyBound {
		// All of our instance storage volumes are bound. This is our final state.
		ctx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = lib.TrueString
	}

	// There are some implicit relationship between these values. Like there should have been
	// nothing missing to be created if we were fully bound. There is a case where some or all
	// PVCs are successfully created but not found.
	// Returns
	//   1. (false, nil) if some or all PVCs not bound and all or some PVCs created.
	//   2. (false, err) if some or all PVCs not bound and error occurs while deleting or creating PVCs.
	//   3. (true, nil) if all PVCs are bound.
	return fullyBound, k8serrors.NewAggregate(append(deleteErrs, createErrs...))
}

func instanceStoragePVCFailed(pvc *corev1.PersistentVolumeClaim) bool {
	errAnn := pvc.Annotations[constants.InstanceStoragePVPlacementErrorAnnotationKey]
	if strings.HasPrefix(errAnn, constants.InstanceStoragePVPlacementErrorPrefix) &&
		time.Since(pvc.CreationTimestamp.Time) >= lib.GetInstanceStoragePVPlacementFailedTTL() {
		// This triggers delete PVCs operation - Delay it by 5m (default) so that the system is
		// not over loaded with repeated create/delete PVCs.
		// NOTE: There is no limitation of CSI on the rate of create/delete PVCs. With this delay,
		// there is a better chance of successful instance storage VM creation after a delay.
		// At the moment there is no logic to anti-affinitize the VM to the ESX Host that just failed,
		// there is a very high chance that the VM will keep landing on the same host. This will lead
		// to a wasteful tight loop of failed attempts to bring up the instance VM.
		return true
	}

	return false
}

func (r *Reconciler) instanceStoragePlacementFailed(
	ctx *context.VolumeContextA2,
	failedVolumesMap map[string]struct{}) error {

	// Tell the VM controller that it needs to compute placement again.
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeAnnotationKey)
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeMOIDAnnotationKey)

	objKeys := make([]client.ObjectKey, 0, len(failedVolumesMap))
	for volName := range failedVolumesMap {
		objKeys = append(objKeys, client.ObjectKey{Name: volName, Namespace: ctx.VM.Namespace})
	}
	deleteErrs := r.deleteInstanceStoragePVCs(ctx, objKeys)

	return k8serrors.NewAggregate(deleteErrs)
}

func (r *Reconciler) createMissingInstanceStoragePVCs(
	ctx *context.VolumeContextA2,
	isVolumes []vmopv1.VirtualMachineVolume,
	existingVolumesMap map[string]struct{},
	selectedNode string) []error {

	var createErrs []error

	for _, vol := range isVolumes {
		if _, exists := existingVolumesMap[vol.Name]; !exists {
			createErrs = append(createErrs, r.createInstanceStoragePVC(ctx, vol, selectedNode))
		}
	}

	return createErrs
}

func (r *Reconciler) createInstanceStoragePVC(
	ctx *context.VolumeContextA2,
	volume vmopv1.VirtualMachineVolume,
	selectedNode string) error {

	claim := volume.PersistentVolumeClaim.InstanceVolumeClaim

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volume.PersistentVolumeClaim.ClaimName,
			Namespace: ctx.VM.Namespace,
			Labels:    map[string]string{constants.InstanceStorageLabelKey: lib.TrueString},
			Annotations: map[string]string{
				constants.KubernetesSelectedNodeAnnotationKey: selectedNode,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &claim.StorageClass,
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: claim.Size,
				},
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}

	if err := controllerutil.SetControllerReference(ctx.VM, pvc, r.Client.Scheme()); err != nil {
		// This is an unexpected error.
		return errors.Wrap(err, "Cannot set controller reference on PersistentVolumeClaim")
	}

	// We merely consider creating non-existing PVCs in reconcileInstanceStoragePVCs flow.
	// We specifically don't need of CreateOrUpdate / CreateOrPatch.
	if err := r.Create(ctx, pvc); err != nil {
		if instancestorage.IsInsufficientQuota(err) {
			r.recorder.EmitEvent(ctx.VM, "Create", err, true)
		}
		return err
	}

	return nil
}

func (r *Reconciler) getInstanceStoragePVCs(
	ctx *context.VolumeContextA2,
	volumes []vmopv1.VirtualMachineVolume) ([]corev1.PersistentVolumeClaim, []error) {

	var errs []error
	pvcList := make([]corev1.PersistentVolumeClaim, 0)

	for _, vol := range volumes {
		objKey := client.ObjectKey{
			Namespace: ctx.VM.Namespace,
			Name:      vol.PersistentVolumeClaim.ClaimName,
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, objKey, pvc); client.IgnoreNotFound(err) != nil {
			errs = append(errs, err)
			continue
		}
		pvcList = append(pvcList, *pvc)
	}

	return pvcList, errs
}

func (r *Reconciler) deleteInstanceStoragePVCs(
	ctx *context.VolumeContextA2,
	objKeys []client.ObjectKey) []error {

	var errs []error

	for _, objKey := range objKeys {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      objKey.Name,
				Namespace: objKey.Namespace,
			},
		}

		if err := r.Delete(ctx, pvc); client.IgnoreNotFound(err) != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// Return the existing CnsNodeVmAttachments that are for this VM.
func (r *Reconciler) getAttachmentsForVM(ctx *context.VolumeContextA2) (map[string]cnsv1alpha1.CnsNodeVmAttachment, error) {
	// We need to filter the attachments for the ones for this VM. There are a few ways we can do this:
	//  - Look at the OwnerRefs for this VM. Note that we'd need to compare by the UUID, not the name,
	//    to handle the situation we the VM is deleted and recreated before the GC deletes any prior
	//    attachments.
	//  - Match the attachment NodeUUID to the VM BiosUUID.
	//
	// We use the NodeUUID option here. Note that we could speed this by adding an indexer on the
	// NodeUUID field, but expect the number of attachments to be manageable. Note that doing List
	// is safer here, as it will discover otherwise orphaned attachments (previous code used the
	// Volumes in the VM Status as the source of truth).

	list := &cnsv1alpha1.CnsNodeVmAttachmentList{}
	if err := r.Client.List(ctx, list, client.InNamespace(ctx.VM.Namespace)); err != nil {
		return nil, errors.Wrap(err, "failed to list CnsNodeVmAttachments")
	}

	attachments := map[string]cnsv1alpha1.CnsNodeVmAttachment{}
	for _, attachment := range list.Items {
		if attachment.Spec.NodeUUID == ctx.VM.Status.BiosUUID {
			attachments[attachment.Name] = attachment
		}
	}

	return attachments, nil
}

func (r *Reconciler) processAttachments(
	ctx *context.VolumeContextA2,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment,
	orphanedAttachments []cnsv1alpha1.CnsNodeVmAttachment) error {

	var volumeStatus []vmopv1.VirtualMachineVolumeStatus
	var createErrs []error
	var hasPendingAttachment bool

	// When creating a VM, try to attach the volumes in the VM Spec.Volumes order since that is a reasonable
	// expectation and the customization like cloud-init may assume that order. There isn't quite a good way
	// to determine from here if the VM is being created so use the power state to infer it. This is mostly
	// best-effort, and a hack in the current world since the CnsNodeVmAttachment really should not exist in
	// the first place.
	onlyAllowOnePendingAttachment := ctx.VM.Status.PowerState == "" || ctx.VM.Status.PowerState == vmopv1.VirtualMachinePowerStateOff

	for _, volume := range ctx.VM.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// Don't process VsphereVolumes here. Note that we don't have Volume status
			// for Vsphere volumes, so there is nothing to preserve here.
			continue
		}

		attachmentName := CNSAttachmentNameForVolume(ctx.VM.Name, volume.Name)
		if attachment, ok := attachments[attachmentName]; ok {
			// The attachment for this volume already exists. Note it might make sense to ensure
			// that the existing attachment matches what we expect (so like do an Update if needed)
			// but the old code didn't and let's match that behavior until we need to do otherwise.
			// Also, the CNS attachment controller doesn't reconcile Spec changes once the volume
			// is attached.
			volumeStatus = append(volumeStatus, attachmentToVolumeStatus(volume.Name, attachment))
			hasPendingAttachment = hasPendingAttachment || !attachment.Status.Attached
			continue
		}

		// If we're allowing only one pending attachment, we cannot create the next CnsNodeVmAttachment
		// until the previous ones are attached. This is really only effective when the VM is first being
		// created, since the volumes could be added anywhere or the same ones reordered in the Spec.Volumes.
		if onlyAllowOnePendingAttachment && hasPendingAttachment {
			// Do not create another CnsNodeVmAttachment while one is already pending, but continue
			// so we build up the Volume Status for any existing volumes.
			continue
		}

		// If VM hardware version doesn't meet minimal requirement, don't create CNS attachment.
		// We only fetch the hardware version when this is the first time to create the CNS attachment.
		//
		// This HW version check block is along with removing VMI hardware version check from VM validation webhook.
		// We used to deny requests if a PVC is specified in the VM spec while VMI hardware version is not supported.
		// In this case, no attachments can be created if VM hardware version doesn't meet the requirement.
		// So if a VM has volume attached, we can safely assume that it has passed the hardware version check.
		if len(volumeStatus) == 0 && len(attachments) == 0 {
			hardwareVersion, err := r.VMProvider.GetVirtualMachineHardwareVersion(ctx, ctx.VM)
			if err != nil {
				return errors.Wrapf(err, "failed to get VM hardware version")
			}

			// If hardware version is 0, which means we failed to parse the version from VM, then just assume that it
			// is above minimal requirement.
			if hardwareVersion != 0 && hardwareVersion < constants.MinSupportedHWVersionForPVC {
				retErr := fmt.Errorf("VirtualMachine has an unsupported "+
					"hardware version %d for PersistentVolumes. Minimum supported hardware version %d",
					hardwareVersion, constants.MinSupportedHWVersionForPVC)
				r.recorder.EmitEvent(ctx.VM, "VolumeAttachment", retErr, true)
				return retErr
			}
		}

		if err := r.createCNSAttachment(ctx, attachmentName, volume); err != nil {
			createErrs = append(createErrs, errors.Wrap(err, "Cannot create CnsNodeVmAttachment"))
		} else {
			// Add a placeholder Status entry for this volume. We'll populate it fully on a later
			// reconcile after the CNS attachment controller updates it.
			volumeStatus = append(volumeStatus, vmopv1.VirtualMachineVolumeStatus{Name: volume.Name})
		}

		// Always true even if the creation failed above to try to keep volumes attached in order.
		hasPendingAttachment = true
	}

	// Fix up the Volume Status so that attachments that are no longer referenced in the Spec but
	// still exist are included in the Status. This is more than a little odd.
	volumeStatus = append(volumeStatus, r.preserveOrphanedAttachmentStatus(ctx, orphanedAttachments)...)

	// This is how the previous code sorted, but IMO keeping in Spec order makes more sense.
	sort.Slice(volumeStatus, func(i, j int) bool {
		return volumeStatus[i].DiskUUID < volumeStatus[j].DiskUUID
	})
	ctx.VM.Status.Volumes = volumeStatus

	return k8serrors.NewAggregate(createErrs)
}

func (r *Reconciler) createCNSAttachment(
	ctx *context.VolumeContextA2,
	attachmentName string,
	volume vmopv1.VirtualMachineVolume) error {

	attachment := &cnsv1alpha1.CnsNodeVmAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      attachmentName,
			Namespace: ctx.VM.Namespace,
		},
		Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
			NodeUUID:   ctx.VM.Status.BiosUUID,
			VolumeName: volume.PersistentVolumeClaim.ClaimName,
		},
	}

	if err := controllerutil.SetControllerReference(ctx.VM, attachment, r.Client.Scheme()); err != nil {
		// This is an unexpected error.
		return errors.Wrap(err, "Cannot set controller reference on CnsNodeVmAttachment")
	}

	if err := r.Create(ctx, attachment); err != nil {
		if apiErrors.IsAlreadyExists(err) {
			return r.createCNSAttachmentButAlreadyExists(ctx, attachmentName)
		}
		return errors.Wrap(err, "Cannot create CnsNodeVmAttachment")
	}

	return nil
}

// createCNSAttachmentButAlreadyExists tries to handle various conditions when a CnsNodeVmAttachment
// unexpected already exists. Usually the existing attachment is for a prior VC VM has been deleted
// from underneath us, and the replacement will have a different BiosUUID. The CnsNodeVmAttachment
// workflow was from CNS and has been very cumbersome to us to use in practice, with this controller
// being more complicated than it really should be for us to try to work around all the edge cases.
// Note that CNS doesn't reevaluate an attachment at all once Status.Attached=true. We could Update
// it instead - clearing Attached atomically since it doesn't have a Status subresource - but it
// should not be treating the Status as the SoT.
func (r *Reconciler) createCNSAttachmentButAlreadyExists(
	ctx *context.VolumeContextA2,
	attachmentName string) error {

	attachment := &cnsv1alpha1.CnsNodeVmAttachment{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: attachmentName, Namespace: ctx.VM.Namespace}, attachment)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Create failed but now the attachment does not exist. Most likely the attachment got GC'd.
			// Return an error to force another reconcile to create it.
			return errors.Errorf("stale client cache for CnsNodeVmAttachment %s that now does not exist. Force re-reconcile", attachmentName)
		}

		return err
	}

	if !metav1.IsControlledBy(attachment, ctx.VM) {
		// This attachment has our expected name but is not owned by us. Most likely, the owning VM
		// is in the process of being deleted, and the attachment will be GC after the owner is gone.
		// We just have to wait it out here: we cannot delete an attachment that isn't ours.
		return errors.Errorf("CnsNodeVmAttachment %s has a different controlling owner", attachmentName)
	}

	if attachment.Spec.NodeUUID != ctx.VM.Status.BiosUUID {
		// We are the owners of this attachment but the BiosUUIDs are different. What's most likely
		// happened is the VC VM was deleted, and then the VC VM is being recreated, generating a new
		// BiosUUID. Since this attachment is ours, delete it to let CNS remove the attachment from
		// the old VM - if it still exists - so that on a later reconcile we re-create the attachment
		// with our new BiosUUID.
		// We could check the attachment's DeletionTimestamp to avoid hitting Delete on it again but
		// this window should be short.
		if err := r.Client.Delete(ctx, attachment); err != nil {
			// The attachment may have been GC'd since the Get() above and that's fine.
			// Return an error to force another reconcile.
			return errors.Wrap(err, "Failed to delete existing CnsNodeVmAttachment with stale BiosUUID")
		}

		return errors.Errorf("deleted stale CnsNodeVmAttachment %s with old NodeUUID: %s", attachmentName, attachment.Spec.NodeUUID)
	}

	// The attachment is ours and has our BiosUUID. The client cache was stale so we didn't see it
	// in getAttachmentsForVM(). This should be transient. Return an error to force another reconcile.
	return errors.Errorf("stale client cache for expected CnsNodeVmAttachment %s. Force re-reconcile", attachmentName)
}

// This is a hack to preserve the prior behavior of including detach(ing) volumes that were
// removed from the Spec in the Status until they are actually deleted.
func (r *Reconciler) preserveOrphanedAttachmentStatus(
	ctx *context.VolumeContextA2,
	orphanedAttachments []cnsv1alpha1.CnsNodeVmAttachment) []vmopv1.VirtualMachineVolumeStatus {

	uuidAttachments := make(map[string]cnsv1alpha1.CnsNodeVmAttachment, len(orphanedAttachments))
	for _, attachment := range orphanedAttachments {
		if uuid := attachment.Status.AttachmentMetadata[AttributeFirstClassDiskUUID]; uuid != "" {
			uuidAttachments[uuid] = attachment
		}
	}

	// For the current Volumes in the Status, if there is an orphaned volume, add an Status entry
	// for this volume. This can lead to some odd results and we should rethink if we actually
	// want this behavior. It can be nice though to show detaching volumes. It would be nice if
	// the Volume status has a reference to the CnsNodeVmAttachment.
	var volumeStatus []vmopv1.VirtualMachineVolumeStatus
	for _, volume := range ctx.VM.Status.Volumes {
		if attachment, ok := uuidAttachments[volume.DiskUUID]; ok {
			volumeStatus = append(volumeStatus, attachmentToVolumeStatus(volume.Name, attachment))
		}
	}

	return volumeStatus
}

func (r *Reconciler) attachmentsToDelete(
	ctx *context.VolumeContextA2,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment) []cnsv1alpha1.CnsNodeVmAttachment {

	expectedAttachments := make(map[string]bool, len(ctx.VM.Spec.Volumes))
	for _, volume := range ctx.VM.Spec.Volumes {
		// Only process CNS volumes here.
		if volume.PersistentVolumeClaim != nil {
			attachmentName := CNSAttachmentNameForVolume(ctx.VM.Name, volume.Name)
			expectedAttachments[attachmentName] = true
		}
	}

	// From the existing attachment map, determine which ones shouldn't exist anymore from
	// the volumes Spec.
	var attachmentsToDelete []cnsv1alpha1.CnsNodeVmAttachment
	for _, attachment := range attachments {
		if exists := expectedAttachments[attachment.Name]; !exists {
			attachmentsToDelete = append(attachmentsToDelete, attachment)
		}
	}

	return attachmentsToDelete
}

func (r *Reconciler) deleteOrphanedAttachments(ctx *context.VolumeContextA2, attachments []cnsv1alpha1.CnsNodeVmAttachment) error {
	var errs []error

	for i := range attachments {
		attachment := attachments[i]

		if !attachment.DeletionTimestamp.IsZero() {
			continue
		}

		ctx.Logger.V(2).Info("Deleting orphaned CnsNodeVmAttachment", "attachment", attachment.Name)
		if err := r.Delete(ctx, &attachment); err != nil {
			if !apiErrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	return k8serrors.NewAggregate(errs)
}

// CNSAttachmentNameForVolume returns the name of the CnsNodeVmAttachment based on the VM and Volume name.
// This matches the naming used in previous code but there are situations where
// we may get a collision between VMs and Volume names. I'm not sure if there is
// an absolute way to avoid that: the same situation can happen with the claimName.
// Ideally, we would use GenerateName, but we lack the back-linkage to match
// Volumes and CnsNodeVmAttachment up.
// The VM webhook validate that this result will be a valid k8s name.
func CNSAttachmentNameForVolume(vmName string, volumeName string) string {
	return vmName + "-" + volumeName
}

// The CSI controller sometimes puts the serialized SOAP error into the CnsNodeVmAttachment
// error field, which contains things like OpIds and pointers that change on every failed
// reconcile attempt. Using this error as-is causes VM object churn, so try to avoid that
// here. The full error message is always available in the CnsNodeVmAttachment.
func sanitizeCNSErrorMessage(msg string) string {
	if strings.Contains(msg, "opId:") {
		idx := strings.Index(msg, ":")
		return msg[:idx]
	}

	return msg
}

func attachmentToVolumeStatus(
	volumeName string,
	attachment cnsv1alpha1.CnsNodeVmAttachment) vmopv1.VirtualMachineVolumeStatus {
	return vmopv1.VirtualMachineVolumeStatus{
		Name:     volumeName, // Name of the volume as in the Spec
		Attached: attachment.Status.Attached,
		DiskUUID: attachment.Status.AttachmentMetadata[AttributeFirstClassDiskUUID],
		Error:    sanitizeCNSErrorMessage(attachment.Status.Error),
	}
}
