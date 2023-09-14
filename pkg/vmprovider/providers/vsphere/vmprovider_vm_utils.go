// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	clutils "github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
)

// SkipVMImageCLProviderCheck skips the checks that a VM Image has a provider and source,
// since a VM Image created for a VM template won't have either. This has been broken for
// a long time but was otherwise masked on how the tests used to be organized.
var SkipVMImageCLProviderCheck = false

func GetVirtualMachineClass(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client) (*vmopv1.VirtualMachineClass, error) {

	className := vmCtx.VM.Spec.ClassName
	key := ctrlclient.ObjectKey{Name: className}
	if lib.IsNamespacedVMClassFSSEnabled() {
		// namespace scoped VM classes
		key.Namespace = vmCtx.VM.Namespace
	}

	vmClass := &vmopv1.VirtualMachineClass{}
	if err := k8sClient.Get(vmCtx, key, vmClass); err != nil {
		msg := fmt.Sprintf("Failed to get VirtualMachineClass: %s", className)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.VirtualMachineClassNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, errors.Wrap(err, msg)
	}

	if lib.IsNamespacedVMClassFSSEnabled() {
		// After WCP_Namespaced_VM_Class FSS is enabled, VirtualMachineClass is migrated
		// from cluster scoped to namespace scoped. VirtualMachineClassBinding CRD will be removed
		// and doesn't make any sense anymore.
		// We can immediately return the VM class here and skip checking VM class bindings.
		return vmClass, nil
	}

	namespace := vmCtx.VM.Namespace

	classBindingList := &vmopv1.VirtualMachineClassBindingList{}
	if err := k8sClient.List(vmCtx, classBindingList, ctrlclient.InNamespace(namespace)); err != nil {
		msg := fmt.Sprintf("Failed to list VirtualMachineClassBindings in namespace: %s", namespace)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.VirtualMachineClassBindingNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, errors.Wrap(err, msg)
	}

	// Filter the bindings for the specified VM class.
	matchingClassBinding := false
	for _, classBinding := range classBindingList.Items {
		if classBinding.ClassRef.Kind == "VirtualMachineClass" && classBinding.ClassRef.Name == className {
			matchingClassBinding = true
			break
		}
	}

	if !matchingClassBinding {
		msg := fmt.Sprintf("Namespace %s does not have access to VirtualMachineClass %s", namespace, className)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.VirtualMachineClassBindingNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		err := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", className, vmCtx.VM.Namespace)
		return nil, err
	}

	return vmClass, nil
}

func GetVMImageAndContentLibraryUUID(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client) (*vmopv1.VirtualMachineImageSpec, *vmopv1.VirtualMachineImageStatus, string, error) {

	imageName := vmCtx.VM.Spec.ImageName
	if lib.IsWCPVMImageRegistryEnabled() {
		vmImageSpec, vmImageStatus, err := resolveVMImage(vmCtx, k8sClient, imageName)
		if err != nil {
			return nil, nil, "", err
		}
		uuid, err := resolveContentLibraryUUID(vmCtx, k8sClient, vmImageStatus)
		if err != nil {
			return nil, nil, "", err
		}
		return vmImageSpec, vmImageStatus, uuid, nil
	}

	// The following code path is reachable only when the WCP-VM-Image-Registry FSS is disabled.
	// In which case the vmopv1.VirtualMachineImage resource remains in cluster scope.
	vmImage := &vmopv1.VirtualMachineImage{}
	if err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: imageName}, vmImage); err != nil {
		msg := fmt.Sprintf("Failed to get VirtualMachineImage: %s", imageName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.VirtualMachineImageNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, nil, "", errors.Wrap(err, msg)
	}

	var clProviderName string
	for _, ownerRef := range vmImage.OwnerReferences {
		if ownerRef.Kind == "ContentLibraryProvider" {
			clProviderName = ownerRef.Name
			break
		}
	}
	if clProviderName == "" {
		if SkipVMImageCLProviderCheck {
			return &vmImage.Spec, &vmImage.Status, "", nil
		}

		msg := fmt.Sprintf("VirtualMachineImage %s does not have a ContentLibraryProvider OwnerReference", imageName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentLibraryProviderNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, nil, "", errors.New(msg)
	}

	clProvider := &vmopv1.ContentLibraryProvider{}
	if err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: clProviderName}, clProvider); err != nil {
		msg := fmt.Sprintf("Failed to get ContentLibraryProvider: %s", clProviderName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentLibraryProviderNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, nil, "", errors.Wrap(err, msg)
	}

	clUUID := clProvider.Spec.UUID

	// With VM Service, we only allow deploying a VM from an image that a developer's namespace has access to.
	var contentSourceName string
	for _, ownerRef := range clProvider.OwnerReferences {
		if ownerRef.Kind == "ContentSource" {
			contentSourceName = ownerRef.Name
			break
		}
	}
	if contentSourceName == "" {
		msg := fmt.Sprintf("ContentLibraryProvider %s does not have a ContentSource OwnerReference", clProvider.Name)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentSourceBindingNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, nil, "", errors.New(msg)
	}

	csBindingList := &vmopv1.ContentSourceBindingList{}
	if err := k8sClient.List(vmCtx, csBindingList, ctrlclient.InNamespace(vmCtx.VM.Namespace)); err != nil {
		msg := fmt.Sprintf("Failed to list ContentSourceBindings in namespace: %s", vmCtx.VM.Namespace)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentSourceBindingNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		vmCtx.Logger.Error(err, msg)
		return nil, nil, "", errors.Wrap(err, msg)
	}

	// Filter the bindings for the specified ContentSource.
	matchingContentSourceBinding := false
	for _, csBinding := range csBindingList.Items {
		if csBinding.ContentSourceRef.Kind == "ContentSource" && csBinding.ContentSourceRef.Name == contentSourceName {
			matchingContentSourceBinding = true
			break
		}
	}

	if !matchingContentSourceBinding {
		msg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
			vmCtx.VM.Namespace, clUUID, vmImage.Name)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentSourceBindingNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg)
		return nil, nil, "", errors.New(msg)
	}

	return &vmImage.Spec, &vmImage.Status, clUUID, nil
}

func GetVMMetadata(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client) (vmMetadata, error) {

	metadata := vmCtx.VM.Spec.VmMetadata
	vmMD := vmMetadata{}

	if metadata == nil {
		return vmMD, nil
	}

	// The VM's MetaData ConfigMapName and SecretName are mutually exclusive. Validation webhook checks
	// this during create/update but double check it here.
	if metadata.ConfigMapName != "" && metadata.SecretName != "" {
		return vmMD, errors.New("invalid VM Metadata: both ConfigMapName and SecretName are specified")
	}

	// ConfigMap and Secret can be both empty.
	vmMD.Transport = metadata.Transport

	if metadata.ConfigMapName != "" {
		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: metadata.ConfigMapName, Namespace: vmCtx.VM.Namespace}, cm)
		if err != nil {
			// TODO: Condition
			return vmMD, errors.Wrap(err, "Failed to get VM Metadata ConfigMap")
		}
		vmMD.Data = cm.Data
	} else if metadata.SecretName != "" {
		secret := &corev1.Secret{}
		err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: metadata.SecretName, Namespace: vmCtx.VM.Namespace}, secret)
		if err != nil {
			// TODO: Condition
			return vmMD, errors.Wrap(err, "Failed to get VM Metadata Secret")
		}
		vmMD.Data = make(map[string]string)
		for k, v := range secret.Data {
			vmMD.Data[k] = string(v)
		}
	}

	return vmMD, nil
}

func GetVMSetResourcePolicy(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client) (*vmopv1.VirtualMachineSetResourcePolicy, error) {

	rpName := vmCtx.VM.Spec.ResourcePolicyName
	if rpName == "" {
		return nil, nil
	}

	resourcePolicy := &vmopv1.VirtualMachineSetResourcePolicy{}
	if err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: rpName, Namespace: vmCtx.VM.Namespace}, resourcePolicy); err != nil {
		vmCtx.Logger.Error(err, "Failed to get VirtualMachineSetResourcePolicy", "resourcePolicyName", rpName)
		// TODO: Condition
		return nil, err
	}

	return resourcePolicy, nil
}

// AddInstanceStorageVolumes checks if VM class is configured with instance volumes and appends the
// volumes to the VM's Spec if it was not already done.
func AddInstanceStorageVolumes(
	vmCtx context.VirtualMachineContext,
	vmClass *vmopv1.VirtualMachineClass) error {

	if instancestorage.IsConfigured(vmCtx.VM) {
		return nil
	}

	instanceStorage := vmClass.Spec.Hardware.InstanceStorage

	if len(instanceStorage.Volumes) == 0 {
		vmCtx.Logger.V(5).Info("VMClass does not have instance storage volumes",
			"VMClass", vmClass.Name)
		return nil
	}

	volumes := make([]vmopv1.VirtualMachineVolume, 0, len(instanceStorage.Volumes))

	for _, isv := range instanceStorage.Volumes {
		id, err := uuid.NewUUID()
		if err != nil {
			return err
		}

		name := constants.InstanceStoragePVCNamePrefix + id.String()

		vmv := vmopv1.VirtualMachineVolume{
			Name: name,
			PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: name,
					ReadOnly:  false,
				},
				InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
					StorageClass: instanceStorage.StorageClass,
					Size:         isv.Size,
				},
			},
		}
		volumes = append(volumes, vmv)
	}

	// Append all together here in case the loop above errors out.
	vmCtx.VM.Spec.Volumes = append(vmCtx.VM.Spec.Volumes, volumes...)

	return nil
}

func GetVMClassConfigSpec(raw json.RawMessage) (*types.VirtualMachineConfigSpec, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	classConfigSpec, err := util.UnmarshalConfigSpecFromJSON(raw)
	if err != nil {
		return nil, err
	}
	util.SanitizeVMClassConfigSpec(classConfigSpec)
	return classConfigSpec, nil
}

// resolveVMImage returns a VirtualMachineImageSpec and VirtualMachineImageStatus from the given image name.
// It updates the VM condition if the image is not found or not in expected condition.
func resolveVMImage(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	imageName string) (*vmopv1.VirtualMachineImageSpec, *vmopv1.VirtualMachineImageStatus, error) {

	imageSpec, imageStatus, err := clutils.GetVMImageSpecStatus(vmCtx, k8sClient, imageName, vmCtx.VM.Namespace)
	if err != nil {
		imageNotFoundMsg := fmt.Sprintf("Failed to get the VM's image: %s", imageName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.VirtualMachineImageNotFoundReason,
			vmopv1.ConditionSeverityError,
			imageNotFoundMsg,
		)
		return nil, nil, errors.Wrap(err, imageNotFoundMsg)
	}

	// Do not return the VM image status if the current image condition is not satisfied.
	imageNotReadyMsgs := []string{}
	if !conditions.IsTrueFromConditions(imageStatus.Conditions, vmopv1.VirtualMachineImageProviderReadyCondition) {
		imageNotReadyMsgs = append(imageNotReadyMsgs, fmt.Sprintf("VM's image provider is not ready: %s", imageName))
	}
	if !conditions.IsTrueFromConditions(imageStatus.Conditions, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition) {
		imageNotReadyMsgs = append(imageNotReadyMsgs, fmt.Sprintf("VM's image provider is not security compliant: %s", imageName))
	}
	if !conditions.IsTrueFromConditions(imageStatus.Conditions, vmopv1.VirtualMachineImageSyncedCondition) {
		imageNotReadyMsgs = append(imageNotReadyMsgs, fmt.Sprintf("VM's image content version is not synced: %s", imageName))
	}

	if len(imageNotReadyMsgs) > 0 {
		imageNotReadyMsg := strings.Join(imageNotReadyMsgs, "; ")
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.VirtualMachineImageNotReadyReason,
			vmopv1.ConditionSeverityError,
			imageNotReadyMsg,
		)
		return nil, nil, errors.New(imageNotReadyMsg)
	}

	return imageSpec, imageStatus, nil
}

// resolveContentLibraryUUID returns the content library UUID from the given image status reference.
// It updated the VM condition if the referred content library object is not found.
func resolveContentLibraryUUID(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	imageStatus *vmopv1.VirtualMachineImageStatus) (string, error) {

	if imageStatus.ContentLibraryRef == nil {
		msg := "VM's image status doesn't have a ContentLibraryRef"
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentLibraryProviderNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg,
		)
		return "", errors.New(msg)
	}

	libraryName := imageStatus.ContentLibraryRef.Name
	libraryUUID := ""
	var err error
	switch imageStatus.ContentLibraryRef.Kind {
	case "ContentLibrary":
		// Get the UUID from content library under VM's namespace.
		cl := &imgregv1a1.ContentLibrary{}
		if err = k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Namespace: vmCtx.VM.Namespace, Name: libraryName}, cl); err == nil {
			libraryUUID = string(cl.Spec.UUID)
		}

	case "ClusterContentLibrary":
		// Get the UUID from cluster content library.
		ccl := &imgregv1a1.ClusterContentLibrary{}
		if err = k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: libraryName}, ccl); err == nil {
			libraryUUID = string(ccl.Spec.UUID)
		}

	default:
		err = errors.Errorf("VM's image status has an invalid ContentLibraryRef kind: %s", imageStatus.ContentLibraryRef.Kind)
	}

	if err != nil {
		msg := fmt.Sprintf("Failed to get content library reference from VM image: %v", err)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachinePrereqReadyCondition,
			vmopv1.ContentLibraryProviderNotFoundReason,
			vmopv1.ConditionSeverityError,
			msg,
		)
		return "", errors.Wrap(err, msg)
	}

	return libraryUUID, nil
}

// HasPVC returns true if the VirtualMachine spec has a Persistent Volume claim.
func HasPVC(vmSpec vmopv1.VirtualMachineSpec) bool {
	for _, vol := range vmSpec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

// HardwareVersionForPVCandPCIDevices returns a hardware version for VMs with PVCs and PCI devices(vGPUs/DDPIO devices)
// The hardware version is determined based on the below criteria: VMs with
// - Persistent Volume Claim (PVC) get the max(the image hardware version, minimum supported virtual hardware version for persistent volumes)
// - vGPUs/DDPIO devices get the max(the image hardware version, minimum supported virtual hardware version for PCI devices)
// - Both vGPU/DDPIO devices and PVCs get the max(the image hardware version, minimum supported virtual hardware version for PCI devices)
// - none of the above returns 0.
func HardwareVersionForPVCandPCIDevices(imageHWVersion int32, configSpec *types.VirtualMachineConfigSpec, hasPVC bool) int32 {
	var configSpecHWVersion int32
	configSpecDevs := util.DevicesFromConfigSpec(configSpec)

	if len(util.SelectNvidiaVgpu(configSpecDevs)) > 0 || len(util.SelectDynamicDirectPathIO(configSpecDevs)) > 0 {
		configSpecHWVersion = constants.MinSupportedHWVersionForPCIPassthruDevices
		if imageHWVersion != 0 && imageHWVersion > constants.MinSupportedHWVersionForPCIPassthruDevices {
			configSpecHWVersion = imageHWVersion
		}
	} else if hasPVC {
		configSpecHWVersion = constants.MinSupportedHWVersionForPVC
		if imageHWVersion != 0 && imageHWVersion > constants.MinSupportedHWVersionForPVC {
			configSpecHWVersion = imageHWVersion
		}
	}

	return configSpecHWVersion
}
