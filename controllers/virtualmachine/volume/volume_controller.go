// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volume

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
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
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	AttributeFirstClassDiskUUID = "diskUUID"

	// CNSSelectedNodeIsZoneAnnotationKey is used to indicate to CNS that the selected-node annotation
	// is the name of the VM's Zone instead of a Node in the zone.
	CNSSelectedNodeIsZoneAnnotationKey = "cns.vmware.com/selected-node-is-zone"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerName      = "volume"
		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controllerName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

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
		ctrl.Log.WithName("controllers").WithName("volume"),
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

	// Watch for changes to VirtualMachine.
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&vmopv1.VirtualMachine{},
		&handler.TypedEnqueueRequestForObject[*vmopv1.VirtualMachine]{},
	)); err != nil {

		return err
	}

	// Watch for changes for CnsNodeVmAttachment, and enqueue VirtualMachine
	// which is the owner of CnsNodeVmAttachment.
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&cnsv1alpha1.CnsNodeVmAttachment{},
		handler.TypedEnqueueRequestForOwner[*cnsv1alpha1.CnsNodeVmAttachment](
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&vmopv1.VirtualMachine{},
			handler.OnlyControllerOwner(),
		),
	)); err != nil {

		return err
	}

	if pkgcfg.FromContext(ctx).Features.InstanceStorage {
		// Instance storage isn't enabled in all envs and is not that commonly used. Avoid the
		// memory and CPU cost of watching PVCs until we encounter a VM with instance storage.

		r.GetInstanceStoragePVCClient = func() (client.Reader, error) {
			r.isPVCWatchStartedLock.Lock()
			defer r.isPVCWatchStartedLock.Unlock()

			if r.isPVCWatchStarted {
				return r.isPVCCache, nil
			}

			if r.isPVCCache == nil {
				// PVC label we set in createInstanceStoragePVC().
				isPVCLabels := metav1.LabelSelector{
					MatchLabels: map[string]string{constants.InstanceStorageLabelKey: "true"},
				}
				labelSelector, err := metav1.LabelSelectorAsSelector(&isPVCLabels)
				if err != nil {
					return nil, err
				}

				// This cache will only contain instance storage PVCs because of the label selector.
				pvcCache, err := pkgmgr.NewLabelSelectorCacheForObject(
					mgr,
					&ctx.SyncPeriod,
					&corev1.PersistentVolumeClaim{},
					labelSelector)
				if err != nil {
					return nil, fmt.Errorf("failed to create PVC cache: %w", err)
				}

				r.isPVCCache = pvcCache
			}

			// Watch for changes for PersistentVolumeClaim, and enqueue
			// VirtualMachine which is the owner of PersistentVolumeClaim.
			if err := c.Watch(source.Kind(
				r.isPVCCache,
				&corev1.PersistentVolumeClaim{},
				handler.TypedEnqueueRequestForOwner[*corev1.PersistentVolumeClaim](
					mgr.GetScheme(),
					mgr.GetRESTMapper(),
					&vmopv1.VirtualMachine{},
					handler.OnlyControllerOwner(),
				),
			)); err != nil {

				return nil, fmt.Errorf("failed to start VirtualMachine watch: %w", err)
			}

			r.logger.Info("Started deferred PVC cache and watch for instance storage")
			r.isPVCWatchStarted = true
			return r.isPVCCache, nil
		}
	} else {
		r.GetInstanceStoragePVCClient = func() (client.Reader, error) {
			return nil, fmt.Errorf("method GetInstanceStoragePVCClient should only be called when the feature is enabled")
		}
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

	// The instance storage PVC cache and watch are deferred until actually required.
	GetInstanceStoragePVCClient func() (client.Reader, error)
	isPVCWatchStarted           bool
	isPVCWatchStartedLock       sync.Mutex
	isPVCCache                  ctrlcache.Cache
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
	// Do not requeue the reconcile here since removing the pause label will trigger a reconcile anyway.
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
	return r.reconcileResult(volCtx), nil
}

func (r *Reconciler) reconcileResult(ctx *pkgctx.VolumeContext) ctrl.Result {
	if pkgcfg.FromContext(ctx).Features.InstanceStorage {
		// Requeue the request if all instance storage PVCs are not bound.
		_, pvcsBound := ctx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]
		if !pvcsBound && vmopv1util.IsInstanceStoragePresent(ctx.VM) {
			return ctrl.Result{RequeueAfter: wait.Jitter(
				pkgcfg.FromContext(ctx).InstanceStorage.SeedRequeueDuration,
				pkgcfg.FromContext(ctx).InstanceStorage.JitterMaxFactor,
			)}
		}
	}

	return ctrl.Result{}
}

func (r *Reconciler) ReconcileDelete(_ *pkgctx.VolumeContext) error {
	// Do nothing here since we depend on the Garbage Collector to do the deletion of the
	// dependent CNSNodeVMAttachment objects when their owning VM is deleted.
	// We require the Volume provider to handle the situation where the VM is deleted before
	// the volumes are detached & removed.
	return nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VolumeContext) error {
	ctx.Logger.Info("Reconciling VirtualMachine for processing volumes")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachine for processing volumes")
	}()

	if pkgcfg.FromContext(ctx).Features.InstanceStorage {
		ready, err := r.reconcileInstanceStoragePVCs(ctx)
		if err != nil || !ready {
			return err
		}
	}

	if ctx.VM.Status.BiosUUID == "" {
		// CSI requires the BiosUUID to match up the attachment request with the VM. Defer here
		// until it is set by the VirtualMachine controller.
		if len(ctx.VM.Spec.Volumes) != 0 {
			ctx.Logger.Info("VM Status does not yet have BiosUUID. Deferring volume attachment")
		}
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

	return apierrorsutil.NewAggregate([]error{deleteErr, processErr})
}

func (r *Reconciler) reconcileInstanceStoragePVCs(ctx *pkgctx.VolumeContext) (bool, error) {
	// NOTE: We could check for InstanceStoragePVCsBoundAnnotationKey here and short circuit
	// all of this. Might leave stale PVCs though. Need to think more: instance storage is
	// this odd quasi reconcilable thing.

	// If the VM Spec doesn't have any instance storage volumes, there is nothing for us to do.
	// We do not support removing - or changing really - this type of volume.
	isVolumes := vmopv1util.FilterInstanceStorageVolumes(ctx.VM)
	if len(isVolumes) == 0 {
		return true, nil
	}

	isPVCReader, err := r.GetInstanceStoragePVCClient()
	if err != nil {
		ctx.Logger.Error(err, "Failed to get deferred PVC client for instance storage")
		return false, err
	}

	pvcList, getErrs := r.getInstanceStoragePVCs(ctx, isPVCReader, isVolumes)
	if getErrs != nil {
		return false, apierrorsutil.NewAggregate(getErrs)
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

		if instanceStoragePVCFailed(ctx, &pvc) {
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
		ctx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = "true"
	}

	// There are some implicit relationship between these values. Like there should have been
	// nothing missing to be created if we were fully bound. There is a case where some or all
	// PVCs are successfully created but not found.
	// Returns
	//   1. (false, nil) if some or all PVCs not bound and all or some PVCs created.
	//   2. (false, err) if some or all PVCs not bound and error occurs while deleting or creating PVCs.
	//   3. (true, nil) if all PVCs are bound.
	return fullyBound, apierrorsutil.NewAggregate(append(deleteErrs, createErrs...))
}

func instanceStoragePVCFailed(ctx context.Context, pvc *corev1.PersistentVolumeClaim) bool {
	errAnn := pvc.Annotations[constants.InstanceStoragePVPlacementErrorAnnotationKey]
	if strings.HasPrefix(errAnn, constants.InstanceStoragePVPlacementErrorPrefix) &&
		time.Since(pvc.CreationTimestamp.Time) >= pkgcfg.FromContext(ctx).InstanceStorage.PVPlacementFailedTTL {
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
	ctx *pkgctx.VolumeContext,
	failedVolumesMap map[string]struct{}) error {

	// Tell the VM controller that it needs to compute placement again.
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeAnnotationKey)
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeMOIDAnnotationKey)

	objKeys := make([]client.ObjectKey, 0, len(failedVolumesMap))
	for volName := range failedVolumesMap {
		objKeys = append(objKeys, client.ObjectKey{Name: volName, Namespace: ctx.VM.Namespace})
	}
	deleteErrs := r.deleteInstanceStoragePVCs(ctx, objKeys)

	return apierrorsutil.NewAggregate(deleteErrs)
}

func (r *Reconciler) createMissingInstanceStoragePVCs(
	ctx *pkgctx.VolumeContext,
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
	ctx *pkgctx.VolumeContext,
	volume vmopv1.VirtualMachineVolume,
	selectedNode string) error {

	claim := volume.PersistentVolumeClaim.InstanceVolumeClaim

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volume.PersistentVolumeClaim.ClaimName,
			Namespace: ctx.VM.Namespace,
			Labels:    map[string]string{constants.InstanceStorageLabelKey: "true"},
			Annotations: map[string]string{
				constants.KubernetesSelectedNodeAnnotationKey: selectedNode,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &claim.StorageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: claim.Size,
				},
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}

	if c := ctx.VM.Spec.Crypto; c != nil && c.EncryptionClassName != "" {
		// Assign the InstanceStorage PVC the same EncryptionClass as the VM.
		pvc.Annotations[constants.EncryptionClassNameAnnotation] = c.EncryptionClassName
	}

	if err := controllerutil.SetControllerReference(ctx.VM, pvc, r.Client.Scheme()); err != nil {
		// This is an unexpected error.
		return fmt.Errorf("cannot set controller reference on PersistentVolumeClaim: %w", err)
	}

	// We merely consider creating non-existing PVCs in reconcileInstanceStoragePVCs flow.
	// We specifically don't need of CreateOrUpdate / CreateOrPatch.
	if err := r.Create(ctx, pvc); err != nil {
		if vmopv1util.IsInsufficientQuota(err) {
			r.recorder.EmitEvent(ctx.VM, "Create", err, true)
		}
		return err
	}

	return nil
}

func (r *Reconciler) getInstanceStoragePVCs(
	ctx *pkgctx.VolumeContext,
	pvcReader client.Reader,
	volumes []vmopv1.VirtualMachineVolume) ([]corev1.PersistentVolumeClaim, []error) {

	var errs []error
	pvcList := make([]corev1.PersistentVolumeClaim, 0)

	for _, vol := range volumes {
		objKey := client.ObjectKey{
			Namespace: ctx.VM.Namespace,
			Name:      vol.PersistentVolumeClaim.ClaimName,
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := pvcReader.Get(ctx, objKey, pvc); err != nil {
			if client.IgnoreNotFound(err) != nil {
				errs = append(errs, err)
			}
			continue
		}

		pvcList = append(pvcList, *pvc)
	}

	return pvcList, errs
}

func (r *Reconciler) deleteInstanceStoragePVCs(
	ctx *pkgctx.VolumeContext,
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
func (r *Reconciler) getAttachmentsForVM(ctx *pkgctx.VolumeContext) (map[string]cnsv1alpha1.CnsNodeVmAttachment, error) {
	// We need to filter the attachments for the ones for this VM. There are a few ways we can do this:
	//  - Look at the OwnerRefs for this VM. Note that we'd need to compare by the UUID, not the name,
	//    to handle the situation we the VM is deleted and recreated before the GC deletes any prior
	//    attachments.
	//  - Match the attachment NodeUUID to the VM BiosUUID.
	//
	// We use the NodeUUID option here. We do a List() here so we discover all attachments including
	// orphaned ones for this VM (previous code used the VM Status.Volumes as the source of truth).

	list := &cnsv1alpha1.CnsNodeVmAttachmentList{}
	err := r.Client.List(ctx, list,
		client.InNamespace(ctx.VM.Namespace),
		client.MatchingFields{"spec.nodeuuid": ctx.VM.Status.BiosUUID})
	if err != nil {
		return nil, fmt.Errorf("failed to list CnsNodeVmAttachments: %w", err)
	}

	attachments := make(map[string]cnsv1alpha1.CnsNodeVmAttachment, len(list.Items))
	for _, attachment := range list.Items {
		attachments[attachment.Name] = attachment
	}

	return attachments, nil
}

func (r *Reconciler) processAttachments(
	ctx *pkgctx.VolumeContext,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment,
	orphanedAttachments []cnsv1alpha1.CnsNodeVmAttachment) error {

	// Record the existing status information for managed volumes.
	existingManagedVols := map[string]vmopv1.VirtualMachineVolumeStatus{}
	for i := range ctx.VM.Status.Volumes {
		vol := ctx.VM.Status.Volumes[i]
		if vol.Type != vmopv1.VolumeTypeClassic {
			existingManagedVols[vol.Name] = vol
		}
	}

	var volumeStatuses []vmopv1.VirtualMachineVolumeStatus
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

		attachmentName := pkgutil.CNSAttachmentNameForVolume(ctx.VM.Name, volume.Name)
		if attachment, ok := attachments[attachmentName]; ok {
			// The attachment for this volume already existed when we listed the attachments for this VM.
			// If this attachment ClaimName refers to the same PVC, then that is the current attachment so
			// update our Status with it. Otherwise, the ClaimName has been changed so we previously deleted
			// the existing attachment and need to create a new one below.
			// CNS noop reconciles of an attachment that is already attached so we must delete the existing
			// attachment and create a new one.
			if volume.PersistentVolumeClaim.ClaimName == attachment.Spec.VolumeName {
				volumeStatus := attachmentToVolumeStatus(volume.Name, attachment)
				volumeStatus.Used = existingManagedVols[volume.Name].Used
				volumeStatus.Crypto = existingManagedVols[volume.Name].Crypto
				if err := updateVolumeStatusWithLimitAndRequest(ctx, r.Client, *volume.PersistentVolumeClaim, &volumeStatus); err != nil {
					ctx.Logger.Error(err, "failed to get volume status limit")
				}
				volumeStatuses = append(volumeStatuses, volumeStatus)
				hasPendingAttachment = hasPendingAttachment || !attachment.Status.Attached
				continue
			}

			// We don't want the existing attachment status to be passed into preserveOrphanedAttachmentStatus()
			// because that would be misleading because that attachment could be marked as attached, but that is
			// for the prior PVC. Of course there is a window where after the ClaimName is changed but before
			// Volume reconciliation that a user could get stale data.
			// Changing the ClaimName should be a rare condition.
			for i := range orphanedAttachments {
				if orphanedAttachments[i].Name == attachment.Name {
					orphanedAttachments = slices.Delete(orphanedAttachments, i, i+1)
					break
				}
			}
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
		if len(volumeStatuses) == 0 && len(attachments) == 0 {
			hardwareVersion, err := r.VMProvider.GetVirtualMachineHardwareVersion(ctx, ctx.VM)
			if err != nil {
				return fmt.Errorf("failed to get VM hardware version: %w", err)
			}

			// If hardware version is 0, which means we failed to parse the version from VM, then just assume that it
			// is above minimal requirement.
			if hardwareVersion.IsValid() && hardwareVersion < pkgconst.MinSupportedHWVersionForPVC {
				retErr := fmt.Errorf("vm has an unsupported "+
					"hardware version %d for PersistentVolumes. Minimum supported hardware version %d",
					hardwareVersion, pkgconst.MinSupportedHWVersionForPVC)
				r.recorder.EmitEvent(ctx.VM, "VolumeAttachment", retErr, true)
				return retErr
			}
		}

		// Must mark this as true now even if something below fails in order to try
		// to keep the volumes attached in order.
		hasPendingAttachment = true

		if err := r.handlePVCWithWFFC(ctx, volume); err != nil {
			createErrs = append(createErrs, err)
			continue
		}

		if err := r.createCNSAttachment(ctx, attachmentName, volume); err != nil {
			createErrs = append(createErrs, fmt.Errorf("cannot create CnsNodeVmAttachment: %w", err))
		} else {
			// Add a placeholder Status entry for this volume. We'll populate it fully on a later
			// reconcile after the CNS attachment controller updates it.
			volumeStatuses = append(
				volumeStatuses,
				vmopv1.VirtualMachineVolumeStatus{
					Name: volume.Name,
					Type: vmopv1.VolumeTypeManaged,
				})
		}
	}

	// Fix up the Volume Status so that attachments that are no longer referenced in the Spec but
	// still exist are included in the Status. This is more than a little odd.
	volumeStatuses = append(volumeStatuses, r.preserveOrphanedAttachmentStatus(ctx, orphanedAttachments)...)

	// Remove any managed volumes from the existing status.
	ctx.VM.Status.Volumes = slices.DeleteFunc(ctx.VM.Status.Volumes,
		func(e vmopv1.VirtualMachineVolumeStatus) bool {
			return e.Type != vmopv1.VolumeTypeClassic
		})

	// Update the existing status with the new list of managed volumes.
	ctx.VM.Status.Volumes = append(ctx.VM.Status.Volumes, volumeStatuses...)

	// This is how the previous code sorted, but IMO keeping in Spec order makes
	// more sense.
	vmopv1.SortVirtualMachineVolumeStatuses(ctx.VM.Status.Volumes)

	return apierrorsutil.NewAggregate(createErrs)
}

func (r *Reconciler) createCNSAttachment(
	ctx *pkgctx.VolumeContext,
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
		return fmt.Errorf("cannot set controller reference on CnsNodeVmAttachment: %w", err)
	}

	if err := r.Create(ctx, attachment); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return r.createCNSAttachmentButAlreadyExists(ctx, attachmentName)
		}
		return fmt.Errorf("cannot create CnsNodeVmAttachment: %w", err)
	}

	return nil
}

// handlePVCWithWFFC sets the selected-node annotation on an unbound PVC if it has
// a WaitForFirstConsumer StorageClass.
func (r *Reconciler) handlePVCWithWFFC(
	ctx *pkgctx.VolumeContext,
	volume vmopv1.VirtualMachineVolume,
) error {

	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.InstanceVolumeClaim != nil {
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

	if pvc.Annotations[constants.KubernetesSelectedNodeAnnotationKey] != "" {
		// Once set, this annotation cannot really be changed so just keep going.
		// TBD: Make check if the Node still exists, and is in the VM's Zone, but
		// CSI doesn't support changing this yet.
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

	if mode := sc.VolumeBindingMode; mode == nil || *mode != storagev1.VolumeBindingWaitForFirstConsumer {
		return nil
	}

	zoneName := ctx.VM.Status.Zone
	if zoneName == "" {
		// Fallback to the label value if Status hasn't been updated yet.
		zoneName = ctx.VM.Labels[topology.KubernetesTopologyZoneLabelKey]
		if zoneName == "" {
			return fmt.Errorf("VM does not have Zone set")
		}
	}

	if pvc.Annotations == nil {
		pvc.Annotations = map[string]string{}
	}
	pvc.Annotations[CNSSelectedNodeIsZoneAnnotationKey] = "true"
	pvc.Annotations[constants.KubernetesSelectedNodeAnnotationKey] = zoneName

	if err := r.Client.Update(ctx, &pvc); err != nil {
		return fmt.Errorf("cannot update PVC to add selected-node annotation: %w", err)
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
	ctx *pkgctx.VolumeContext,
	attachmentName string) error {

	attachment := &cnsv1alpha1.CnsNodeVmAttachment{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: attachmentName, Namespace: ctx.VM.Namespace}, attachment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create failed but now the attachment does not exist. Most likely the attachment got GC'd.
			// Return an error to force another reconcile to create it.
			return fmt.Errorf("stale client cache for CnsNodeVmAttachment %s that now does not exist. Force re-reconcile", attachmentName)
		}

		return err
	}

	if !metav1.IsControlledBy(attachment, ctx.VM) {
		// This attachment has our expected name but is not owned by us. Most likely, the owning VM
		// is in the process of being deleted, and the attachment will be GC after the owner is gone.
		// We just have to wait it out here: we cannot delete an attachment that isn't ours.
		return fmt.Errorf("the CnsNodeVmAttachment %s has a different controlling owner", attachmentName)
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
			return fmt.Errorf("failed to delete existing CnsNodeVmAttachment with stale BiosUUID: %w", err)
		}

		return fmt.Errorf("deleted stale CnsNodeVmAttachment %s with old NodeUUID: %s", attachmentName, attachment.Spec.NodeUUID)
	}

	// The attachment is ours and has our BiosUUID. The client cache was stale so we didn't see it
	// in getAttachmentsForVM(). This should be transient. Return an error to force another reconcile.
	return fmt.Errorf("stale client cache for expected CnsNodeVmAttachment %s. Force re-reconcile", attachmentName)
}

// This is a hack to preserve the prior behavior of including detach(ing) volumes that were
// removed from the Spec in the Status until they are actually deleted.
func (r *Reconciler) preserveOrphanedAttachmentStatus(
	ctx *pkgctx.VolumeContext,
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
			volName := volume.Name
			if !strings.HasSuffix(volName, ":detaching") {
				volName += ":detaching"
			}
			volumeStatus = append(volumeStatus, attachmentToVolumeStatus(volName, attachment))
		}
	}

	return volumeStatus
}

func (r *Reconciler) attachmentsToDelete(
	ctx *pkgctx.VolumeContext,
	attachments map[string]cnsv1alpha1.CnsNodeVmAttachment) []cnsv1alpha1.CnsNodeVmAttachment {

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

func (r *Reconciler) deleteOrphanedAttachments(ctx *pkgctx.VolumeContext, attachments []cnsv1alpha1.CnsNodeVmAttachment) error {
	var errs []error

	for i := range attachments {
		attachment := attachments[i]

		if !attachment.DeletionTimestamp.IsZero() {
			continue
		}

		ctx.Logger.V(2).Info("Deleting orphaned CnsNodeVmAttachment", "attachment", attachment.Name)
		if err := r.Delete(ctx, &attachment); err != nil {
			if !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	return apierrorsutil.NewAggregate(errs)
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
		Type:     vmopv1.VolumeTypeManaged,
	}
}

func updateVolumeStatusWithLimitAndRequest(
	ctx *pkgctx.VolumeContext,
	c client.Reader,
	pvcSpec vmopv1.PersistentVolumeClaimVolumeSource,
	status *vmopv1.VirtualMachineVolumeStatus) error {

	// See if the volume is an instance storage volume.
	if pvcSpec.InstanceVolumeClaim != nil {
		// Short-cut the rest of the function since instance storage
		// volumes already have the requested size and the PVC does
		// not need to be fetched.
		status.Limit = &pvcSpec.InstanceVolumeClaim.Size
		status.Requested = &pvcSpec.InstanceVolumeClaim.Size
		return nil
	}

	var (
		pvc    corev1.PersistentVolumeClaim
		pvcKey = client.ObjectKey{
			Namespace: ctx.VM.Namespace,
			Name:      pvcSpec.ClaimName,
		}
	)

	if err := c.Get(ctx, pvcKey, &pvc); err != nil {
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
