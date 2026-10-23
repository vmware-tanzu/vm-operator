// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

type PVC struct {
	VolumeName          string  `json:"volume_name,omitempty"`
	ClaimName           string  `json:"claim_name,omitempty"`
	StorageClassName    string  `json:"storage_class_name,omitempty"`
	RequestSize         string  `json:"request_size,omitempty"`
	Namespace           string  `json:"namespace,omitempty"`
	ControllerBusNumber *int32  `json:"controller_bus_number,omitempty"`
	UnitNumber          *int32  `json:"unit_number,omitempty"`
	SharingMode         *string `json:"sharing_mode,omitempty"`
	DiskMode            *string `json:"disk_mode,omitempty"`

	VolumeMode      *corev1.PersistentVolumeMode        `json:"volume_mode,omitempty"`
	AccessModes     []corev1.PersistentVolumeAccessMode `json:"access_modes,omitempty"`
	ControllerType  *vmopv1.VirtualControllerType       `json:"controller_type,omitempty"`
	ApplicationType vmopv1.VolumeApplicationType        `json:"application_type,omitempty"`
}

type VirtualMachineYaml struct {
	Namespace        string                             `json:"namespace,omitempty"`
	Name             string                             `json:"name,omitempty"`
	ImageName        string                             `json:"image_name,omitempty"`
	VMClassName      string                             `json:"vm_class_name,omitempty"`
	StorageClassName string                             `json:"storage_class_name,omitempty"`
	PowerState       string                             `json:"power_state,omitempty"`
	GuestID          string                             `json:"guest_id,omitempty"`
	GroupName        string                             `json:"groupName,omitempty"`
	PVCs             []PVC                              `json:"pvcs,omitempty"`
	Hardware         *vmopv1.VirtualMachineHardwareSpec `json:"hardware,omitempty"`
}

// GetPersistentVolumeClaimYaml returns a PersistentVolumeClaim YAML manifest
// from the same PVC fields used by VirtualMachineA5.
func GetPersistentVolumeClaimYaml(pvc PVC) []byte {
	return ToYAML(must(PersistentVolumeClaim(pvc)))
}

// objectMeta returns the ObjectMeta shared by all the VirtualMachine builders.
func objectMeta(vmYaml VirtualMachineYaml) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      vmYaml.Name,
		Namespace: vmYaml.Namespace,
	}
}

// must returns obj, failing the current test if err is not nil.
func must[T any](obj T, err error) T {
	if err != nil {
		e2eframework.Failf("Failed to build manifest: %v", err)
	}
	return obj
}

// typeMeta returns the TypeMeta for the given kind in the given API version.
func typeMeta(apiVersion, kind string) metav1.TypeMeta {
	return metav1.TypeMeta{APIVersion: apiVersion, Kind: kind}
}
