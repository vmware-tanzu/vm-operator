// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// Bootstrap is the subset of the VM bootstrap configuration that the E2E
// suite sets through VirtualMachineYaml.
type Bootstrap struct {
	Sysprep *Sysprep `json:"sysprep,omitempty"`
}

// Sysprep holds an inline sysprep, which is decoded into its typed API
// representation.
type Sysprep struct {
	Sysprep *string `json:"sysprep,omitempty"`
}

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
	PowerOffMode     string                             `json:"power_off_mode,omitempty"`
	Bootstrap        Bootstrap                          `json:"bootstrap,omitempty"`
	GuestID          string                             `json:"guest_id,omitempty"`
	GroupName        string                             `json:"groupName,omitempty"`
	PVCs             []PVC                              `json:"pvcs,omitempty"`
	Hardware         *vmopv1.VirtualMachineHardwareSpec `json:"hardware,omitempty"`
}

// GetVirtualMachineYamlA5 returns a multi-document manifest containing a
// v1alpha5 VirtualMachine followed by a PersistentVolumeClaim for each entry
// in vmYaml.PVCs.
func GetVirtualMachineYamlA5(vmYaml VirtualMachineYaml) []byte {
	return ToYAML(append(
		[]ctrlclient.Object{must(VirtualMachineA5(vmYaml))},
		must(persistentVolumeClaimObjects(vmYaml.PVCs))...)...)
}

// GetPersistentVolumeClaimYaml returns a PersistentVolumeClaim YAML manifest
// from the same PVC fields used by GetVirtualMachineYamlA5.
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

// hasBootstrap returns true if any of the bootstrap providers are specified.
func (b Bootstrap) hasBootstrap() bool {
	return b.Sysprep != nil
}

// unmarshalInline decodes an inline YAML snippet, such as an inline cloud
// config or sysprep, into its typed API representation. A nil input returns
// nil.
func unmarshalInline[T any](s *string) (*T, error) {
	if s == nil {
		return nil, nil
	}

	var out T
	if err := yaml.UnmarshalStrict([]byte(*s), &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inline %T: %w", out, err)
	}

	return &out, nil
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
