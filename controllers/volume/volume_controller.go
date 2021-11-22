// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package volume

import (
	goctx "context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
		mgr.GetScheme(),
	)

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachine.
	err = c.Watch(&source.Kind{Type: &vmopv1alpha1.VirtualMachine{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes for CnsNodeVmAttachment, and enqueue VirtualMachine which is the owner of CnsNodeVmAttachment.
	err = c.Watch(&source.Kind{Type: &cnsv1alpha1.CnsNodeVmAttachment{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &vmopv1alpha1.VirtualMachine{},
		IsController: true,
	})
	if err != nil {
		return err
	}

	// Watch for changes for PersistentVolumeClaim, and enqueue VirtualMachine which is the owner of PersistentVolumeClaim.
	err = c.Watch(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &vmopv1alpha1.VirtualMachine{},
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
	scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		Client:   client,
		logger:   logger,
		recorder: recorder,
		scheme:   scheme,
	}
}

var _ reconcile.Reconciler = &Reconciler{}

type Reconciler struct {
	client.Client
	logger   logr.Logger
	recorder record.Recorder
	scheme   *runtime.Scheme
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
	vm := &vmopv1alpha1.VirtualMachine{}
	if err := r.Get(ctx, request.NamespacedName, vm); err != nil {
		if apiErrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	volCtx := &context.VolumeContext{
		Context:                   ctx,
		Logger:                    ctrl.Log.WithName("Volumes").WithValues("name", vm.NamespacedName()),
		VM:                        vm,
		InstanceStorageFSSEnabled: lib.IsInstanceStorageFSSEnabled(),
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

func (r *Reconciler) reconcileResult(ctx *context.VolumeContext) ctrl.Result {
	if ctx.InstanceStorageFSSEnabled {
		// Requeue the request if all instance storage PVCs are not bound.
		_, pvcsBound := ctx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]
		if instancestorage.IsConfigured(ctx.VM) && !pvcsBound {
			return ctrl.Result{RequeueAfter: lib.GetInstanceStorageRequeueDelay()}
		}
	}

	return ctrl.Result{}
}

func (r *Reconciler) ReconcileDelete(_ *context.VolumeContext) error {
	// Do nothing here since we depend on the Garbage Collector to do the deletion of the
	// dependent CNSNodeVMAttachment objects when their owning VM is deleted.
	// We require the Volume provider to handle the situation where the VM is deleted before
	// the volumes are detached & removed.
	return nil
}

func (r *Reconciler) ReconcileNormal(ctx *context.VolumeContext) error {
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

func (r *Reconciler) reconcileInstanceStoragePVCs(ctx *context.VolumeContext) (bool, error) {
	// NOTE: We could check for InstanceStoragePVCsBoundAnnotationKey here and short circuit
	// all of this. Might leave stale PVCs though. Need to think more: instance storage is
	// this odd quasi reconcilable thing.

	// If the VM Spec doesn't have any instance storage volumes, there is nothing for us to do.
	// We do not support removing - or changing really - this type of volume.
	isVolumes := instancestorage.FilterVolumes(ctx.VM)
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
	ctx *context.VolumeContext,
	failedVolumesMap map[string]struct{}) error {

	// Tell the VM controller that it needs to compute placement again.
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeAnnotationKey)

	objKeys := make([]client.ObjectKey, 0, len(failedVolumesMap))
	for volName := range failedVolumesMap {
		objKeys = append(objKeys, client.ObjectKey{Name: volName, Namespace: ctx.VM.Namespace})
	}
	deleteErrs := r.deleteInstanceStoragePVCs(ctx, objKeys)

	return k8serrors.NewAggregate(deleteErrs)
}

func (r *Reconciler) createMissingInstanceStoragePVCs(
	ctx *context.VolumeContext,
	isVolumes []vmopv1alpha1.VirtualMachineVolume,
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
	ctx *context.VolumeContext,
	volume vmopv1alpha1.VirtualMachineVolume,
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

	if err := controllerutil.SetControllerReference(ctx.VM, pvc, r.scheme); err != nil {
		// This is an unexpected error.
		return errors.Wrap(err, "Cannot set controller reference on PersistentVolumeClaim")
	}

	// We merely consider creating non-existing PVCs in reconcileInstanceStoragePVCs flow.
	// We specifically don't need of CreateOrUpdate / CreateOrPatch.
	return r.Create(ctx, pvc)
}

func (r *Reconciler) getInstanceStoragePVCs(
	ctx *context.VolumeContext,
	volumes []vmopv1alpha1.VirtualMachineVolume) ([]corev1.PersistentVolumeClaim, []error) {

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
	ctx *context.VolumeContext,
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
func (r *Reconciler) getAttachmentsForVM(ctx *context.VolumeContext) (map[string]cnsv1alpha1.CnsNodeVmAttachment, error) {
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
	ctx *context.VolumeContext,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment,
	orphanedAttachments []cnsv1alpha1.CnsNodeVmAttachment) error {
	var volumeStatus []vmopv1alpha1.VirtualMachineVolumeStatus
	var createErrs []error

	// Use Spec.Volumes order when attaching as a best effort to preserve spec order. There
	// is no guarantee order will be preserved however, as the CNS attachment controller may
	// not receive/process the requests in order. The nuclear option would be for us to only
	// have one pending volume attachment outstanding per VM at a time.
	// Create() errors below may also result in attachments being out of the original spec
	// order.
	for _, volume := range ctx.VM.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// Don't process VsphereVolumes here. Note that we don't have Volume status
			// for Vsphere volumes, so there is nothing to preserve here.
			continue
		}

		attachmentName := CNSAttachmentNameForVolume(ctx.VM, volume.Name)
		if attachment, ok := attachments[attachmentName]; ok {
			// The attachment for this volume already exists. Note it might make sense to ensure
			// that the existing attachment matches what we expect (so like do an Update if needed)
			// but the old code didn't and let's match that behavior until we need to do otherwise.
			// Also, the CNS attachment controller doesn't reconcile Spec changes once the volume
			// is attached.
			volumeStatus = append(volumeStatus, attachmentToVolumeStatus(volume.Name, attachment))
			continue
		}

		if err := r.createCNSAttachment(ctx, attachmentName, volume); err != nil {
			createErrs = append(createErrs, errors.Wrap(err, "Cannot create CnsNodeVmAttachment"))
		} else {
			// Add a placeholder Status entry for this volume. We'll populate it fully on a later
			// reconcile after the CNS attachment controller updates it.
			volumeStatus = append(volumeStatus, vmopv1alpha1.VirtualMachineVolumeStatus{Name: volume.Name})
		}
	}

	// Fix up the Volume Status so that attachments that are no longer referenced in the Spec but
	// still exist are included in the Status. This is more than a little odd.
	volumeStatus = append(volumeStatus, r.preserveOrphanedAttachmentStatus(ctx, orphanedAttachments)...)

	// This is how the previous code sorted, but IMO keeping in Spec order makes more sense.
	sort.Slice(volumeStatus, func(i, j int) bool {
		return volumeStatus[i].DiskUuid < volumeStatus[j].DiskUuid
	})
	ctx.VM.Status.Volumes = volumeStatus

	return k8serrors.NewAggregate(createErrs)
}

func (r *Reconciler) createCNSAttachment(
	ctx *context.VolumeContext,
	attachmentName string,
	volume vmopv1alpha1.VirtualMachineVolume) error {
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

	if err := controllerutil.SetControllerReference(ctx.VM, attachment, r.scheme); err != nil {
		// This is an unexpected error.
		return errors.Wrap(err, "Cannot set controller reference on CnsNodeVmAttachment")
	}

	if err := r.Create(ctx, attachment); err != nil {
		// An IsAlreadyExists error is not really expected because the only way we'll do a Create()
		// is if the attachment wasn't returned in the List() done at the start of the reconcile.
		// The Client cache might have been stale, so ignore the error, but like the comment in the
		// caller, we might want to perform an Update() in this case too.
		if !apiErrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "Error creating CnsNodeVmAttachment")
		}
	}

	return nil
}

// This is a hack to preserve the prior behavior of including detach(ing) volumes that were
// removed from the Spec in the Status until they are actually deleted.
func (r *Reconciler) preserveOrphanedAttachmentStatus(
	ctx *context.VolumeContext,
	orphanedAttachments []cnsv1alpha1.CnsNodeVmAttachment) []vmopv1alpha1.VirtualMachineVolumeStatus {
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
	var volumeStatus []vmopv1alpha1.VirtualMachineVolumeStatus
	for _, volume := range ctx.VM.Status.Volumes {
		if attachment, ok := uuidAttachments[volume.DiskUuid]; ok {
			volumeStatus = append(volumeStatus, attachmentToVolumeStatus(volume.Name, attachment))
		}
	}

	return volumeStatus
}

func (r *Reconciler) attachmentsToDelete(
	ctx *context.VolumeContext,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment) []cnsv1alpha1.CnsNodeVmAttachment {
	expectedAttachments := make(map[string]bool, len(ctx.VM.Spec.Volumes))
	for _, volume := range ctx.VM.Spec.Volumes {
		// Only process CNS volumes here.
		if volume.PersistentVolumeClaim != nil {
			attachmentName := CNSAttachmentNameForVolume(ctx.VM, volume.Name)
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

func (r *Reconciler) deleteOrphanedAttachments(ctx *context.VolumeContext, attachments []cnsv1alpha1.CnsNodeVmAttachment) error {
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
func CNSAttachmentNameForVolume(vm *vmopv1alpha1.VirtualMachine, volumeName string) string {
	return vm.Name + "-" + volumeName
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
	attachment cnsv1alpha1.CnsNodeVmAttachment) vmopv1alpha1.VirtualMachineVolumeStatus {
	return vmopv1alpha1.VirtualMachineVolumeStatus{
		Name:     volumeName, // Name of the volume as in the Spec
		Attached: attachment.Status.Attached,
		DiskUuid: attachment.Status.AttachmentMetadata[AttributeFirstClassDiskUUID],
		Error:    sanitizeCNSErrorMessage(attachment.Status.Error),
	}
}
