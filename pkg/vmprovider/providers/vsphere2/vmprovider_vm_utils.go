// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
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
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vmlifecycle"
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

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionClassReady)

	return vmClass, nil
}

func GetVirtualMachineImageSpecAndStatus(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (ctrlclient.Object, *vmopv1.VirtualMachineImageSpec, *vmopv1.VirtualMachineImageStatus, error) {

	var obj conditions.Getter
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

	vmiNotReadyMessage := "VirtualMachineImage is not ready"

	conditions.SetMirror(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, obj,
		conditions.WithFallbackValue(false,
			"NotReady",
			vmiNotReadyMessage),
	)
	if conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady) {
		return nil, nil, nil, fmt.Errorf(vmiNotReadyMessage)
	}

	return obj, spec, status, nil
}

func getSecretData(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client,
	secretName, secretKey string,
	configMapFallback, isCloudInitSecret bool) (map[string]string, error) {

	var data map[string]string

	key := ctrlclient.ObjectKey{Name: secretName, Namespace: vmCtx.VM.Namespace}
	secret := &corev1.Secret{}
	if err := k8sClient.Get(vmCtx, key, secret); err != nil {
		configMap := &corev1.ConfigMap{}

		// For backwards compat if we cannot find the Secret, fallback to a ConfigMap. In v1a1, either a
		// Secret and ConfigMap was supported for metadata (bootstrap) as separate fields, but v1a2 only
		// supports Secrets.
		if configMapFallback && apierrors.IsNotFound(err) {
			// Use the Secret error since the error message isn't misleading.
			if k8sClient.Get(vmCtx, key, configMap) == nil {
				err = nil
			}
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

	if secretKey != "" {
		secretKeys := []string{secretKey}
		if isCloudInitSecret {
			// Hack: the v1a1 bootstrap did not have a Key field so for CloudInit we'd check
			// for a few well-known keys. Check for the existence of those other keys when
			// dealing with a CloudInit Secret so existing v1a1 users continue to work.
			secretKeys = append(secretKeys, vmlifecycle.CloudInitUserDataSecretKeys...)
		}

		found := false
		for _, k := range secretKeys {
			if _, ok := data[k]; ok {
				found = true
				break
			}
		}

		if !found {
			err := fmt.Errorf("required key %q not found in Secret %s", secretKey, secretName)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, "RequiredKeyNotFound", err.Error())
			return nil, err
		}
	}

	return data, nil
}

func GetVirtualMachineBootstrap(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (vmlifecycle.BootstrapData, error) {

	bootstrapSpec := vmCtx.VM.Spec.Bootstrap
	if bootstrapSpec == nil {
		conditions.Delete(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)
		return vmlifecycle.BootstrapData{}, nil
	}

	var data, vAppData map[string]string
	var vAppExData map[string]map[string]string
	var cloudConfigSecretData *cloudinit.CloudConfigSecretData
	var sysprepSecretData *sysprep.SecretData

	if v := bootstrapSpec.CloudInit; v != nil {
		if cooked := v.CloudConfig; cooked != nil {
			out, err := cloudinit.GetCloudConfigSecretData(
				vmCtx,
				k8sClient,
				vmCtx.VM.Namespace,
				*cooked)
			if err != nil {
				reason, msg := errToConditionReasonAndMessage(err)
				conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
				return vmlifecycle.BootstrapData{}, err
			}
			cloudConfigSecretData = &out
		} else if raw := v.RawCloudConfig; raw != nil {
			var err error
			data, err = getSecretData(vmCtx, k8sClient, raw.Name, raw.Key, true, true)
			if err != nil {
				reason, msg := errToConditionReasonAndMessage(err)
				conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
				return vmlifecycle.BootstrapData{}, err
			}
		}
	} else if v := bootstrapSpec.Sysprep; v != nil {
		if cooked := v.Sysprep; cooked != nil {
			out, err := sysprep.GetSysprepSecretData(
				vmCtx,
				k8sClient,
				vmCtx.VM.Namespace,
				cooked)
			if err != nil {
				reason, msg := errToConditionReasonAndMessage(err)
				conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
				return vmlifecycle.BootstrapData{}, err
			}
			sysprepSecretData = &out
		} else if raw := v.RawSysprep; raw != nil {
			var err error
			data, err = getSecretData(vmCtx, k8sClient, raw.Name, raw.Key, true, false)
			if err != nil {
				reason, msg := errToConditionReasonAndMessage(err)
				conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
				return vmlifecycle.BootstrapData{}, err
			}
		}
	}

	// vApp bootstrap can be used alongside LinuxPrep/Sysprep.
	if vApp := bootstrapSpec.VAppConfig; vApp != nil {

		if vApp.RawProperties != "" {
			var err error
			vAppData, err = getSecretData(vmCtx, k8sClient, vApp.RawProperties, "", true, false)
			if err != nil {
				reason, msg := errToConditionReasonAndMessage(err)
				conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
				return vmlifecycle.BootstrapData{}, err
			}

		} else {
			for _, p := range vApp.Properties {
				from := p.Value.From
				if from == nil {
					continue
				}

				if data, ok := vAppExData[from.Name]; !ok {
					// Do the easy thing here and carry along each Secret's entire data. We could instead
					// shoehorn this in the vAppData with a concat key using an invalid k8s name delimiter.
					fromData, err := getSecretData(vmCtx, k8sClient, from.Name, from.Key, false, false)
					if err != nil {
						reason, msg := errToConditionReasonAndMessage(err)
						conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
						return vmlifecycle.BootstrapData{}, err
					}

					if vAppExData == nil {
						vAppExData = make(map[string]map[string]string)
					}
					vAppExData[from.Name] = fromData
				} else if from.Key != "" {
					if _, ok := data[from.Key]; !ok {
						err := fmt.Errorf("required key %q not found in vApp Properties Secret %s", from.Key, from.Name)
						conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, "RequiredKeyNotFound", err.Error())
						return vmlifecycle.BootstrapData{}, err
					}
				}
			}
		}
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)

	return vmlifecycle.BootstrapData{
		Data:        data,
		VAppData:    vAppData,
		VAppExData:  vAppExData,
		CloudConfig: cloudConfigSecretData,
		Sysprep:     sysprepSecretData,
	}, nil
}

func GetVMSetResourcePolicy(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (*vmopv1.VirtualMachineSetResourcePolicy, error) {

	var rpName string
	if reserved := vmCtx.VM.Spec.Reserved; reserved != nil {
		rpName = reserved.ResourcePolicyName
	}
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

func GetVMClassConfigSpec(ctx goctx.Context, raw json.RawMessage) (*types.VirtualMachineConfigSpec, error) {
	classConfigSpec, err := util.UnmarshalConfigSpecFromJSON(raw)
	if err != nil {
		return nil, err
	}
	util.SanitizeVMClassConfigSpec(ctx, classConfigSpec)

	return classConfigSpec, nil
}

// GetAttachedDiskUUIDToPVC returns a map of disk UUID to PVC object for all
// attached disks by checking the VM's spec and status of volumes.
func GetAttachedDiskUUIDToPVC(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) (map[string]corev1.PersistentVolumeClaim, error) {

	if len(vmCtx.VM.Spec.Volumes) == 0 {
		return nil, nil
	}

	vmVolNameToPVCName := map[string]string{}
	for _, vol := range vmCtx.VM.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			vmVolNameToPVCName[vol.Name] = pvc.ClaimName
		}
	}

	if len(vmVolNameToPVCName) == 0 {
		return nil, nil
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

// GetAdditionalResourcesForBackup returns a list of Kubernetes client objects
// that are relevant for VM backup (e.g. bootstrap referenced resources).
func GetAdditionalResourcesForBackup(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client) ([]ctrlclient.Object, error) {
	var objects []ctrlclient.Object
	// Get bootstrap related objects from CloudInit or Sysprep (mutually exclusive).
	if bootstrapSpec := vmCtx.VM.Spec.Bootstrap; bootstrapSpec != nil {
		if v := bootstrapSpec.CloudInit; v != nil {
			if cooked := v.CloudConfig; cooked != nil {
				out, err := cloudinit.GetSecretResources(vmCtx, k8sClient, vmCtx.VM.Namespace, *cooked)
				if err != nil {
					return nil, err
				}
				// GVK is dropped when getting a core K8s resource from client.
				// Add it in backup so that the resource can be applied successfully during restore.
				for i := range out {
					out[i].GetObjectKind().SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
				}
				objects = append(objects, out...)
			} else if raw := v.RawCloudConfig; raw != nil {
				obj, err := getSecretOrConfigMapObject(vmCtx, k8sClient, raw.Name, true)
				if err != nil {
					return nil, err
				}
				objects = append(objects, obj)
			}
		} else if v := bootstrapSpec.Sysprep; v != nil {
			if cooked := v.Sysprep; cooked != nil {
				out, err := sysprep.GetSecretResources(vmCtx, k8sClient, vmCtx.VM.Namespace, cooked)
				if err != nil {
					return nil, err
				}
				// GVK is dropped when getting a K8s core resource from client.
				// Add it in backup so that the resource can be applied successfully during restore.
				for i := range out {
					out[i].GetObjectKind().SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
				}
				objects = append(objects, out...)
			} else if raw := v.RawSysprep; raw != nil {
				obj, err := getSecretOrConfigMapObject(vmCtx, k8sClient, raw.Name, true)
				if err != nil {
					return nil, err
				}
				objects = append(objects, obj)
			}
		}

		// Get bootstrap related objects from vAppConfig (can be used alongside LinuxPrep/Sysprep).
		if vApp := bootstrapSpec.VAppConfig; vApp != nil {
			if cooked := vApp.Properties; cooked != nil {
				uniqueSecrets := map[string]struct{}{}
				for _, p := range vApp.Properties {
					if from := p.Value.From; from != nil {
						// vAppConfig Properties are backed by Secret resources only.
						// Only return the secret if it has not already been captured.
						if _, captured := uniqueSecrets[from.Name]; captured {
							continue
						}
						obj, err := getSecretOrConfigMapObject(vmCtx, k8sClient, from.Name, false)
						if err != nil {
							return nil, err
						}
						objects = append(objects, obj)
						uniqueSecrets[from.Name] = struct{}{}
					}
				}
			} else if raw := vApp.RawProperties; raw != "" {
				obj, err := getSecretOrConfigMapObject(vmCtx, k8sClient, raw, true)
				if err != nil {
					return nil, err
				}
				objects = append(objects, obj)
			}
		}
	}

	return objects, nil
}

func getSecretOrConfigMapObject(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client,
	resourceName string,
	configMapFallback bool) (ctrlclient.Object, error) {
	key := ctrlclient.ObjectKey{Name: resourceName, Namespace: vmCtx.VM.Namespace}
	secret := &corev1.Secret{}
	err := k8sClient.Get(vmCtx, key, secret)
	if err != nil {
		configMap := &corev1.ConfigMap{}
		// For backwards compat if we cannot find the Secret, fallback to a ConfigMap. In v1a1, either a
		// Secret and ConfigMap was supported for metadata (bootstrap) as separate fields, but v1a2 only
		// supports Secrets.
		if configMapFallback && apierrors.IsNotFound(err) {
			if k8sClient.Get(vmCtx, key, configMap) == nil {
				// GVK is dropped when getting a core K8s resource from client.
				// Add it in backup so that the resource can be applied successfully during restore.
				configMap.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				err = nil
			}
		}

		return configMap, err
	}

	// GVK is dropped when getting a core K8s resource from client.
	// Add it in backup so that the resource can be applied successfully during restore.
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

	return secret, err
}
