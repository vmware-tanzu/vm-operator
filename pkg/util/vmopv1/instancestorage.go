// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// IsInstanceStoragePresent checks if VM Spec has instance volumes added to its Volumes list.
func IsInstanceStoragePresent(vm *vmopv1.VirtualMachine) bool {
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			return true
		}
	}
	return false
}

// FilterInstanceStorageVolumes returns instance storage volumes present in VM spec.
func FilterInstanceStorageVolumes(vm *vmopv1.VirtualMachine) []vmopv1.VirtualMachineVolume {
	var volumes []vmopv1.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			volumes = append(volumes, vol)
		}
	}

	return volumes
}

func IsInsufficientQuota(err error) bool {
	if apierrors.IsForbidden(err) {
		e := err.Error()
		return strings.Contains(e, "insufficient quota") || strings.Contains(e, "exceeded quota")
	}

	return false
}

// ShouldRequeueForInstanceStoragePVCs checks if a reconciliation should be requeued
// while waiting for instance storage PVCs to be bound. Returns a ctrl.Result with
// RequeueAfter set if requeue is needed, or an empty ctrl.Result otherwise.
func ShouldRequeueForInstanceStoragePVCs(
	ctx context.Context, vm *vmopv1.VirtualMachine) ctrl.Result {

	if !pkgcfg.FromContext(ctx).Features.InstanceStorage {
		return ctrl.Result{}
	}

	// Requeue the request if all instance storage PVCs are not bound.
	_, pvcsBound := vm.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]
	if !pvcsBound && IsInstanceStoragePresent(vm) {
		return ctrl.Result{RequeueAfter: wait.Jitter(
			pkgcfg.FromContext(ctx).InstanceStorage.SeedRequeueDuration,
			pkgcfg.FromContext(ctx).InstanceStorage.JitterMaxFactor,
		)}
	}

	return ctrl.Result{}
}

// ReconcileInstanceStoragePVCs reconciles instance storage PVCs for a VM.
// This is the core reconciliation logic shared between volume controllers.
// Returns (ready bool, error):
//   - (true, nil): All instance storage PVCs are bound
//   - (false, nil): PVCs exist but not yet bound (caller should requeue)
//   - (false, err): Error occurred
func ReconcileInstanceStoragePVCs(
	ctx *pkgctx.VolumeContext,
	k8sClient client.Client,
	reader client.Reader,
	recorder record.Recorder,
) (bool, error) {
	// NOTE: We could check for InstanceStoragePVCsBoundAnnotationKey here and short circuit
	// all of this. Might leave stale PVCs though. Need to think more: instance storage is
	// this odd quasi reconcilable thing.

	// If the VM Spec doesn't have any instance storage volumes, there is nothing for us to do.
	// We do not support removing - or changing really - this type of volume.
	isVolumes := FilterInstanceStorageVolumes(ctx.VM)
	if len(isVolumes) == 0 {
		return true, nil
	}

	pvcList, getErrs := getInstanceStoragePVCs(ctx, reader, ctx.VM.Namespace, isVolumes)
	if getErrs != nil {
		return false, apierrorsutil.NewAggregate(getErrs)
	}

	var (
		stalePVCs       []client.ObjectKey
		createErrs      []error
		existingVolumes = make(map[string]struct{})
		failedVolumes   = make(map[string]struct{})
		boundCount      = 0
		selectedNode    = ctx.VM.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey]
		createPVCs      = len(selectedNode) > 0
	)

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

		existingVolumes[pvc.Name] = struct{}{}

		pvcNode, exists := pvc.Annotations[storagehelpers.AnnSelectedNode]
		if !exists || pvcNode != selectedNode {
			// This PVC is ours but NOT on our selected node. Likely, placement previously failed
			// and we're trying again on a different node.
			// NOTE: This includes even when selectedNode is "". Bias for full cleanup.
			stalePVCs = append(stalePVCs, client.ObjectKeyFromObject(&pvc))
			continue
		}

		if instanceStoragePVCFailed(ctx, &pvc) {
			// This PVC is ours but has failed. This instance storage placement is doomed.
			failedVolumes[pvc.Name] = struct{}{}
			continue
		}

		if pvc.Status.Phase != corev1.ClaimBound {
			// CSI is still processing this PVC.
			continue
		}

		// This PVC is successfully bound to our selected host, and is ready for attachment.
		boundCount++
	}

	if len(failedVolumes) > 0 {
		// Need to start placement over. PVCs successfully realized are recreated or
		// retained depending on the next host selection.
		return false, instanceStoragePlacementFailed(ctx, k8sClient, failedVolumes)
	}

	deleteErrs := deleteInstanceStoragePVCs(ctx, k8sClient, stalePVCs)
	if createPVCs {
		createErrs = createMissingInstanceStoragePVCs(ctx, k8sClient, recorder, isVolumes, existingVolumes, selectedNode)
	}

	fullyBound := boundCount == len(isVolumes)
	if fullyBound {
		// All of our instance storage volumes are bound. This is our final state.
		if ctx.VM.Annotations == nil {
			ctx.VM.Annotations = make(map[string]string)
		}
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

// instanceStoragePVCFailed checks if a PVC has failed placement.
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

// instanceStoragePlacementFailed handles failed instance storage placement.
func instanceStoragePlacementFailed(
	ctx *pkgctx.VolumeContext,
	k8sClient client.Client,
	failedVolumes map[string]struct{},
) error {

	// Tell the VM controller that it needs to compute placement again.
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeAnnotationKey)
	delete(ctx.VM.Annotations, constants.InstanceStorageSelectedNodeMOIDAnnotationKey)

	objKeys := make([]client.ObjectKey, 0, len(failedVolumes))
	for volName := range failedVolumes {
		objKeys = append(objKeys, client.ObjectKey{Name: volName, Namespace: ctx.VM.Namespace})
	}
	deleteErrs := deleteInstanceStoragePVCs(ctx, k8sClient, objKeys)

	return apierrorsutil.NewAggregate(deleteErrs)
}

// createMissingInstanceStoragePVCs creates PVCs that don't exist yet.
func createMissingInstanceStoragePVCs(
	ctx *pkgctx.VolumeContext,
	k8sClient client.Client,
	recorder record.Recorder,
	isVolumes []vmopv1.VirtualMachineVolume,
	existingVolumes map[string]struct{},
	selectedNode string,
) []error {

	var createErrs []error

	for _, vol := range isVolumes {
		if _, exists := existingVolumes[vol.Name]; !exists {
			err := createInstanceStoragePVC(ctx, k8sClient, recorder, vol, selectedNode)
			if err != nil {
				createErrs = append(createErrs, err)
			}
		}
	}

	return createErrs
}

// createInstanceStoragePVC creates a single instance storage PVC.
func createInstanceStoragePVC(
	ctx *pkgctx.VolumeContext,
	k8sClient client.Client,
	recorder record.Recorder,
	volume vmopv1.VirtualMachineVolume,
	selectedNode string,
) error {

	claim := volume.PersistentVolumeClaim.InstanceVolumeClaim

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volume.PersistentVolumeClaim.ClaimName,
			Namespace: ctx.VM.Namespace,
			Labels:    map[string]string{constants.InstanceStorageLabelKey: "true"},
			Annotations: map[string]string{
				storagehelpers.AnnSelectedNode: selectedNode,
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

	if err := controllerutil.SetControllerReference(ctx.VM, pvc, k8sClient.Scheme()); err != nil {
		// This is an unexpected error.
		return fmt.Errorf("cannot set controller reference on PersistentVolumeClaim: %w", err)
	}

	// We merely consider creating non-existing PVCs in reconcileInstanceStoragePVCs flow.
	// We specifically don't need of CreateOrUpdate / CreateOrPatch.
	if err := k8sClient.Create(ctx, pvc); err != nil {
		if IsInsufficientQuota(err) {
			recorder.EmitEvent(ctx.VM, "Create", err, true)
		}
		return err
	}

	return nil
}

// getInstanceStoragePVCs retrieves all instance storage PVCs for the given volumes.
func getInstanceStoragePVCs(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	volumes []vmopv1.VirtualMachineVolume,
) ([]corev1.PersistentVolumeClaim, []error) {

	var errs []error
	pvcList := make([]corev1.PersistentVolumeClaim, 0)

	for _, vol := range volumes {
		objKey := client.ObjectKey{
			Namespace: namespace,
			Name:      vol.PersistentVolumeClaim.ClaimName,
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := reader.Get(ctx, objKey, pvc); err != nil {
			if client.IgnoreNotFound(err) != nil {
				errs = append(errs, err)
			}
			continue
		}

		pvcList = append(pvcList, *pvc)
	}

	return pvcList, errs
}

// deleteInstanceStoragePVCs deletes the specified PVCs.
func deleteInstanceStoragePVCs(
	ctx context.Context,
	k8sClient client.Client,
	objKeys []client.ObjectKey,
) []error {

	var errs []error

	for _, objKey := range objKeys {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      objKey.Name,
				Namespace: objKey.Namespace,
			},
		}

		if err := k8sClient.Delete(ctx, pvc); client.IgnoreNotFound(err) != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
