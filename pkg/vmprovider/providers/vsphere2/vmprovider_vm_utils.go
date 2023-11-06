// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/instancestorage"
)

// TODO: This mostly just a placeholder until we spend time on something better. Individual types
// don't make much sense since we don't lump everything under a single prereq condition anymore.
func errToConditionReasonAndMessage(err error) (string, string) {
	switch {
	case apierrors.IsNotFound(err):
		return "NotFound", err.Error()
	case apierrors.IsForbidden(err):
		return "Forbidden", err.Error()
	case apierrors.IsInvalid(err):
		return "Invalid", err.Error()
	case apierrors.IsInternalError(err):
		return "InternalError", err.Error()
	default:
		return "GetError", err.Error()
	}
}

func GetVirtualMachineClass(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (*vmopv1.VirtualMachineClass, error) {

	key := ctrlclient.ObjectKey{Name: vmCtx.VM.Spec.ClassName, Namespace: vmCtx.VM.Namespace}
	vmClass := &vmopv1.VirtualMachineClass{}
	if err := k8sClient.Get(vmCtx, key, vmClass); err != nil {
		reason, msg := errToConditionReasonAndMessage(err)
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionClassReady, reason, msg)
		return nil, err
	}

	if !vmClass.Status.Ready {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionClassReady,
			"NotReady", "VirtualMachineClass is not marked as Ready")
		return nil, fmt.Errorf("VirtualMachineClass is not Ready")
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionClassReady)

	return vmClass, nil
}

func GetVirtualMachineImageSpecAndStatus(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (ctrlclient.Object, *vmopv1.VirtualMachineImageSpec, *vmopv1.VirtualMachineImageStatus, error) {

	var obj ctrlclient.Object
	var spec *vmopv1.VirtualMachineImageSpec
	var status *vmopv1.VirtualMachineImageStatus

	key := ctrlclient.ObjectKey{Name: vmCtx.VM.Spec.ImageName, Namespace: vmCtx.VM.Namespace}
	vmImage := &vmopv1.VirtualMachineImage{}
	if err := k8sClient.Get(vmCtx, key, vmImage); err != nil {
		clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}

		if apierrors.IsNotFound(err) {
			key.Namespace = ""
			err = k8sClient.Get(vmCtx, key, clusterVMImage)
		}

		if err != nil {
			// Don't use the k8s error as-is as we don't know to prefer the NS or cluster scoped error message.
			// This is the same error/message that the prior code used.
			reason, msg := "NotFound", fmt.Sprintf("Failed to get the VM's image: %s", key.Name)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, reason, msg)
			return nil, nil, nil, fmt.Errorf("%s: %w", msg, err)
		}

		obj, spec, status = clusterVMImage, &clusterVMImage.Spec, &clusterVMImage.Status
	} else {
		obj, spec, status = vmImage, &vmImage.Spec, &vmImage.Status
	}

	// TODO: Fix the image conditions so it just has a single Ready instead of bleeding the CL stuff.
	if !conditions.IsTrueFromConditions(status.Conditions, vmopv1.VirtualMachineImageSyncedCondition) {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady,
			"NotReady", "VirtualMachineImage is not ready")
		return nil, nil, nil, fmt.Errorf("VirtualMachineImage is not ready")
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady)

	return obj, spec, status, nil
}

func getSecretData(
	vmCtx context.VirtualMachineContextA2,
	name string,
	cmFallback bool,
	k8sClient ctrlclient.Client) (map[string]string, error) {

	var data map[string]string

	key := ctrlclient.ObjectKey{Name: name, Namespace: vmCtx.VM.Namespace}
	secret := &corev1.Secret{}
	if err := k8sClient.Get(vmCtx, key, secret); err != nil {
		configMap := &corev1.ConfigMap{}

		// For backwards compat if we cannot find the Secret, fallback to a ConfigMap. In v1a1, either a
		// Secret and ConfigMap was supported for metadata (bootstrap) as separate fields, but v1a2 only
		// supports Secrets.
		if cmFallback && apierrors.IsNotFound(err) {
			err = k8sClient.Get(vmCtx, key, configMap)
		}

		if err != nil {
			reason, msg := errToConditionReasonAndMessage(err)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
			return nil, err
		}

		data = configMap.Data
	} else {
		data = make(map[string]string, len(secret.Data))

		for k, v := range secret.Data {
			data[k] = string(v)
		}
	}

	return data, nil
}

func GetVirtualMachineBootstrap(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (map[string]string, map[string]string, map[string]map[string]string, error) {

	bootstrapSpec := &vmCtx.VM.Spec.Bootstrap
	var secretName string
	var data, vAppData map[string]string
	var vAppExData map[string]map[string]string

	if cloudInit := bootstrapSpec.CloudInit; cloudInit != nil {
		secretName = cloudInit.RawCloudConfig.Name
	} else if sysprep := bootstrapSpec.Sysprep; sysprep != nil {
		secretName = sysprep.RawSysprep.Name
	}

	if secretName != "" {
		var err error

		data, err = getSecretData(vmCtx, secretName, true, k8sClient)
		if err != nil {
			reason, msg := errToConditionReasonAndMessage(err)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
			return nil, nil, nil, err
		}
	}

	// vApp bootstrap can be used alongside LinuxPrep/Sysprep.
	if vApp := bootstrapSpec.VAppConfig; vApp != nil {

		if vApp.RawProperties != "" {
			var err error

			vAppData, err = getSecretData(vmCtx, vApp.RawProperties, true, k8sClient)
			if err != nil {
				reason, msg := errToConditionReasonAndMessage(err)
				conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
				return nil, nil, nil, err
			}

		} else {
			for _, p := range vApp.Properties {
				from := p.Value.From
				if from == nil {
					continue
				}

				if _, ok := vAppExData[from.Name]; !ok {
					// Do the easy thing here and carry along each Secret's entire data. We could instead
					// shoehorn this in the vAppData with a concat key using an invalid k8s name delimiter.
					// TODO: Check that key exists, and/or deal with from.Optional. Too many options.
					fromData, err := getSecretData(vmCtx, from.Name, false, k8sClient)
					if err != nil {
						reason, msg := errToConditionReasonAndMessage(err)
						conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
						return nil, nil, nil, err
					}

					if vAppExData == nil {
						vAppExData = make(map[string]map[string]string)
					}
					vAppExData[from.Name] = fromData
				}
			}
		}
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)

	return data, vAppData, vAppExData, nil
}

func GetVMSetResourcePolicy(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (*vmopv1.VirtualMachineSetResourcePolicy, error) {

	rpName := vmCtx.VM.Spec.Reserved.ResourcePolicyName
	if rpName == "" {
		conditions.Delete(vmCtx.VM, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)
		return nil, nil
	}

	key := ctrlclient.ObjectKey{Name: rpName, Namespace: vmCtx.VM.Namespace}
	resourcePolicy := &vmopv1.VirtualMachineSetResourcePolicy{}
	if err := k8sClient.Get(vmCtx, key, resourcePolicy); err != nil {
		reason, msg := errToConditionReasonAndMessage(err)
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady, reason, msg)
		return nil, err
	}

	// The VirtualMachineSetResourcePolicy doesn't have a Ready condition or field but don't
	// allow a VM to use a policy that's being deleted.
	if !resourcePolicy.DeletionTimestamp.IsZero() {
		err := fmt.Errorf("VirtualMachineSetResourcePolicy is being deleted")
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady,
			"NotReady", err.Error())
		return nil, err
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)

	return resourcePolicy, nil
}

// AddInstanceStorageVolumes checks if VM class is configured with instance storage volumes and appends the
// volumes to the VM's Spec if not already done. Return true if the VM had or now has instance storage volumes.
func AddInstanceStorageVolumes(
	vmCtx context.VirtualMachineContextA2,
	vmClass *vmopv1.VirtualMachineClass) bool {

	if instancestorage.IsPresent(vmCtx.VM) {
		// Instance storage disks are copied from the class to the VM only once, regardless
		// if the class changes.
		return true
	}

	is := vmClass.Spec.Hardware.InstanceStorage
	if len(is.Volumes) == 0 {
		return false
	}

	volumes := make([]vmopv1.VirtualMachineVolume, 0, len(is.Volumes))

	for _, isv := range is.Volumes {
		name := constants.InstanceStoragePVCNamePrefix + uuid.NewString()

		vmv := vmopv1.VirtualMachineVolume{
			Name: name,
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name,
						ReadOnly:  false,
					},
					InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
						StorageClass: is.StorageClass,
						Size:         isv.Size,
					},
				},
			},
		}
		volumes = append(volumes, vmv)
	}

	vmCtx.VM.Spec.Volumes = append(vmCtx.VM.Spec.Volumes, volumes...)
	return true
}

func GetVMClassConfigSpec(raw json.RawMessage) (*types.VirtualMachineConfigSpec, error) {
	classConfigSpec, err := util.UnmarshalConfigSpecFromJSON(raw)
	if err != nil {
		return nil, err
	}
	util.SanitizeVMClassConfigSpec(classConfigSpec)

	return classConfigSpec, nil
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

// GetAttachedDiskUUIDToPVC returns a map of disk UUID to PVC object for all
// attached disks by checking the VM's spec and status of volumes.
func GetAttachedDiskUUIDToPVC(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (map[string]corev1.PersistentVolumeClaim, error) {
	if !HasPVC(vmCtx.VM.Spec) {
		return nil, nil
	}

	vmVolNameToPVCName := map[string]string{}
	for _, vol := range vmCtx.VM.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			vmVolNameToPVCName[vol.Name] = pvc.ClaimName
		}
	}

	diskUUIDToPVC := map[string]corev1.PersistentVolumeClaim{}
	for _, vol := range vmCtx.VM.Status.Volumes {
		if !vol.Attached || vol.DiskUUID == "" {
			continue
		}

		pvcName := vmVolNameToPVCName[vol.Name]
		// This could happen if the volume was just removed from VM spec but not reconciled yet.
		if pvcName == "" {
			continue
		}

		pvcObj := corev1.PersistentVolumeClaim{}
		objKey := ctrlclient.ObjectKey{Name: pvcName, Namespace: vmCtx.VM.Namespace}
		if err := k8sClient.Get(vmCtx, objKey, &pvcObj); err != nil {
			return nil, err
		}

		diskUUIDToPVC[vol.DiskUUID] = pvcObj
	}

	return diskUUIDToPVC, nil
}
