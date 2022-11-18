// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
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
	k8sClient ctrlclient.Client) (*vmopv1alpha1.VirtualMachineClass, error) {

	className := vmCtx.VM.Spec.ClassName

	vmClass := &vmopv1alpha1.VirtualMachineClass{}
	if err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: className}, vmClass); err != nil {
		msg := fmt.Sprintf("Failed to get VirtualMachineClass: %s", className)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.VirtualMachineClassNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		return nil, errors.Wrap(err, msg)
	}

	namespace := vmCtx.VM.Namespace

	classBindingList := &vmopv1alpha1.VirtualMachineClassBindingList{}
	if err := k8sClient.List(vmCtx, classBindingList, ctrlclient.InNamespace(namespace)); err != nil {
		msg := fmt.Sprintf("Failed to list VirtualMachineClassBindings in namespace: %s", namespace)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
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
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		err := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", className, vmCtx.VM.Namespace)
		return nil, err
	}

	return vmClass, nil
}

func GetVMImageAndContentLibraryUUID(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client) (*vmopv1alpha1.VirtualMachineImage, string, error) {

	imageName := vmCtx.VM.Spec.ImageName

	vmImage := &vmopv1alpha1.VirtualMachineImage{}
	if err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: imageName}, vmImage); err != nil {
		msg := fmt.Sprintf("Failed to get VirtualMachineImage: %s", imageName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.VirtualMachineImageNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		return nil, "", errors.Wrap(err, msg)
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
			return vmImage, "", nil
		}

		msg := fmt.Sprintf("VirtualMachineImage %s does not have a ContentLibraryProvider OwnerReference", imageName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.ContentLibraryProviderNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		return nil, "", errors.New(msg)
	}

	clProvider := &vmopv1alpha1.ContentLibraryProvider{}
	if err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: clProviderName}, clProvider); err != nil {
		msg := fmt.Sprintf("Failed to get ContentLibraryProvider: %s", clProviderName)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.ContentLibraryProviderNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		return nil, "", errors.Wrap(err, msg)
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
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.ContentSourceBindingNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		return nil, "", errors.New(msg)
	}

	csBindingList := &vmopv1alpha1.ContentSourceBindingList{}
	if err := k8sClient.List(vmCtx, csBindingList, ctrlclient.InNamespace(vmCtx.VM.Namespace)); err != nil {
		msg := fmt.Sprintf("Failed to list ContentSourceBindings in namespace: %s", vmCtx.VM.Namespace)
		conditions.MarkFalse(vmCtx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.ContentSourceBindingNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		vmCtx.Logger.Error(err, msg)
		return nil, "", errors.Wrap(err, msg)
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
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.ContentSourceBindingNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		return nil, "", errors.New(msg)
	}

	return vmImage, clUUID, nil
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

	if metadata.ConfigMapName != "" {
		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: metadata.ConfigMapName, Namespace: vmCtx.VM.Namespace}, cm)
		if err != nil {
			// TODO: Condition
			return vmMD, errors.Wrap(err, "Failed to get VM Metadata ConfigMap")
		}

		vmMD.Transport = metadata.Transport
		vmMD.Data = cm.Data
	} else if metadata.SecretName != "" {
		secret := &corev1.Secret{}
		err := k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: metadata.SecretName, Namespace: vmCtx.VM.Namespace}, secret)
		if err != nil {
			// TODO: Condition
			return vmMD, errors.Wrap(err, "Failed to get VM Metadata Secret")
		}

		vmMD.Transport = metadata.Transport
		vmMD.Data = make(map[string]string)
		for k, v := range secret.Data {
			vmMD.Data[k] = string(v)
		}
	}

	return vmMD, nil
}

func GetVMSetResourcePolicy(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client) (*vmopv1alpha1.VirtualMachineSetResourcePolicy, error) {

	rpName := vmCtx.VM.Spec.ResourcePolicyName
	if rpName == "" {
		return nil, nil
	}

	resourcePolicy := &vmopv1alpha1.VirtualMachineSetResourcePolicy{}
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
	vmClass *vmopv1alpha1.VirtualMachineClass) error {

	if instancestorage.IsConfigured(vmCtx.VM) {
		return nil
	}

	instanceStorage := vmClass.Spec.Hardware.InstanceStorage

	if len(instanceStorage.Volumes) == 0 {
		vmCtx.Logger.V(5).Info("VMClass does not have instance storage volumes",
			"VMClass", vmClass.Name)
		return nil
	}

	volumes := make([]vmopv1alpha1.VirtualMachineVolume, 0, len(instanceStorage.Volumes))

	for _, isv := range instanceStorage.Volumes {
		id, err := uuid.NewUUID()
		if err != nil {
			return err
		}

		name := constants.InstanceStoragePVCNamePrefix + id.String()

		vmv := vmopv1alpha1.VirtualMachineVolume{
			Name: name,
			PersistentVolumeClaim: &vmopv1alpha1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: name,
					ReadOnly:  false,
				},
				InstanceVolumeClaim: &vmopv1alpha1.InstanceVolumeClaimVolumeSource{
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

func GetVMClassConfigSpecFromXML(configSpecXML string) (*types.VirtualMachineConfigSpec, error) {
	if configSpecXML == "" {
		return nil, nil
	}

	classConfigSpec, err := util.UnmarshalConfigSpecFromBase64XML([]byte(configSpecXML))
	if err != nil {
		return nil, err
	}

	util.SanitizeVMClassConfigSpec(classConfigSpec)

	return classConfigSpec, nil
}
