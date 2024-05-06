// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
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
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) (ctrlclient.Object, *vmopv1.VirtualMachineImageSpec, *vmopv1.VirtualMachineImageStatus, error) {

	if vmCtx.VM.Spec.Image == nil {
		// This should never be possible in production as this function is
		// only called in the create workflow, where spec.image is never nil.
		// Still, it's better to test for this and set a condition in case the
		// call stack for this function changes.
		reason := "NotSet"
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, reason, "")
		return nil, nil, nil, fmt.Errorf(reason)
	}

	var (
		obj    conditions.Getter
		objErr error
		spec   *vmopv1.VirtualMachineImageSpec
		status *vmopv1.VirtualMachineImageStatus
		key    = ctrlclient.ObjectKey{
			Name:      vmCtx.VM.Spec.Image.Name,
			Namespace: vmCtx.VM.Namespace,
		}
	)

	switch vmCtx.VM.Spec.Image.Kind {
	case "VirtualMachineImage":
		var img vmopv1.VirtualMachineImage
		if objErr = k8sClient.Get(vmCtx, key, &img); objErr == nil {
			obj, spec, status = &img, &img.Spec, &img.Status
		}

	case "ClusterVirtualMachineImage":
		key.Namespace = ""
		var img vmopv1.ClusterVirtualMachineImage
		if objErr = k8sClient.Get(vmCtx, key, &img); objErr == nil {
			obj, spec, status = &img, &img.Spec, &img.Status
		}
	case "":
		// This is only possible IFF VirtualMachine API resources created at a
		// schema version prior to spec.image were not yet deployed when VM Op
		// as upgraded to the schema version with spec.image. This means the
		// create webhook did not run for those resources, populating their
		// spec.image from spec.imageName. Therefore, to be safe, this case
		// does what the webhook does -- looks up the image by both vmi and
		// friendly name.
		if img, err := vmopv1util.ResolveImageName(
			vmCtx,
			k8sClient,
			vmCtx.VM.Namespace,
			vmCtx.VM.Spec.Image.Name); err != nil {

			objErr = err
		} else {
			switch timg := img.(type) {
			case *vmopv1.VirtualMachineImage:
				vmCtx.VM.Spec.Image = &vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: timg.Name,
				}
				obj, spec, status = timg, &timg.Spec, &timg.Status
			case *vmopv1.ClusterVirtualMachineImage:
				vmCtx.VM.Spec.Image = &vmopv1.VirtualMachineImageRef{
					Kind: "ClusterVirtualMachineImage",
					Name: timg.Name,
				}
				obj, spec, status = timg, &timg.Spec, &timg.Status
			}
		}
	default:
		// This should never be possible in production as this function is
		// only called in the create workflow, where spec.image is never nil.
		// Still, it's better to test for this and set a condition in case the
		// call stack for this function changes.
		reason := "UnknownKind"
		msg := vmCtx.VM.Spec.Image.Kind
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, reason, msg)
		return nil, nil, nil, fmt.Errorf("%s: %s", reason, msg)
	}

	if objErr != nil {
		reason := "Unknown"
		if apierrors.IsNotFound(objErr) {
			reason = "NotFound"
		}
		msg := objErr.Error()
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, reason, msg)
		return nil, nil, nil, fmt.Errorf("%s: %w", msg, objErr)
	}

	// Ensure the GVK for the object is synced back into the object since
	// the object's APIVersion and Kind fields may be used later.
	if err := kube.SyncGVKToObject(obj, k8sClient.Scheme()); err != nil {
		return nil, nil, nil, err
	}

	vmiNotReadyMessage := "VirtualMachineImage is not ready"
	conditions.SetMirror(
		vmCtx.VM,
		vmopv1.VirtualMachineConditionImageReady,
		obj,
		conditions.WithFallbackValue(false, "NotReady", vmiNotReadyMessage))
	if conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady) {
		return nil, nil, nil, fmt.Errorf(vmiNotReadyMessage)
	}

	return obj, spec, status, nil
}

func getSecretData(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	resName, resKey string,
	isCloudInitSecret bool) (map[string]string, error) {

	var data map[string]string

	// For backwards compat use a ConfigMap when V1alpha1ConfigMapTransportAnnotation is set. In v1a1, either a
	// Secret and ConfigMap was supported for metadata (bootstrap) as separate fields, but later versions only
	// supports Secrets for bootstrap data.
	useConfigMap := false
	if _, ok := vmCtx.VM.Annotations[vmopv1.V1alpha1ConfigMapTransportAnnotation]; ok {
		useConfigMap = true
	}

	key := ctrlclient.ObjectKey{Name: resName, Namespace: vmCtx.VM.Namespace}
	if useConfigMap {
		configMap := &corev1.ConfigMap{}
		if err := k8sClient.Get(vmCtx, key, configMap); err != nil {
			reason, msg := errToConditionReasonAndMessage(err)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
			return nil, err
		}

		data = configMap.Data
	} else {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(vmCtx, key, secret); err != nil {
			reason, msg := errToConditionReasonAndMessage(err)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, reason, msg)
			return nil, err
		}

		data = make(map[string]string, len(secret.Data))

		for k, v := range secret.Data {
			data[k] = string(v)
		}
	}

	if resKey != "" {
		secretKeys := []string{resKey}
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
			err := fmt.Errorf("required key %q not found in Secret %s", resKey, resName)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady, "RequiredKeyNotFound", err.Error())
			return nil, err
		}
	}

	return data, nil
}

func GetVirtualMachineBootstrap(
	vmCtx pkgctx.VirtualMachineContext,
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
			data, err = getSecretData(vmCtx, k8sClient, raw.Name, raw.Key, true)
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
			data, err = getSecretData(vmCtx, k8sClient, raw.Name, raw.Key, false)
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
			vAppData, err = getSecretData(vmCtx, k8sClient, vApp.RawProperties, "", false)
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
					fromData, err := getSecretData(vmCtx, k8sClient, from.Name, from.Key, false)
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
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
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

func GetVMClassConfigSpec(
	ctx context.Context,
	raw json.RawMessage) (vimtypes.VirtualMachineConfigSpec, error) {

	configSpec, err := util.UnmarshalConfigSpecFromJSON(raw)
	if err != nil {
		return vimtypes.VirtualMachineConfigSpec{}, err
	}
	util.SanitizeVMClassConfigSpec(ctx, &configSpec)

	return configSpec, nil
}

// GetAttachedDiskUUIDToPVC returns a map of disk UUID to PVC object for all
// attached disks by checking the VM's spec and status of volumes.
func GetAttachedDiskUUIDToPVC(
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
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
