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

type Network struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}
type NetworkA2 struct {
	Interfaces []InterfaceSpec `json:"interfaces,omitempty"`
}
type InterfaceSpec struct {
	Name       string `json:"name,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}
type Bootstrap struct {
	CloudInit  *CloudInit  `json:"cloudInit,omitempty"`
	Sysprep    *Sysprep    `json:"sysprep,omitempty"`
	VAppConfig *VAppConfig `json:"vAppConfig,omitempty"`
	LinuxPrep  *LinuxPrep  `json:"linuxPrep,omitempty"`
}

type Cdrom struct {
	Name                string                        `json:"name,omitempty"`
	ImageName           string                        `json:"imageName,omitempty"`
	ImageKind           string                        `json:"imageKind,omitempty"`
	Connected           bool                          `json:"connected,omitempty"`
	AllowGuestControl   bool                          `json:"allowGuestControl,omitempty"`
	ControllerBusNumber *int32                        `json:"controllerBusNumber,omitempty"`
	ControllerType      *vmopv1.VirtualControllerType `json:"controllerType,omitempty"`
	UnitNumber          *int32                        `json:"unitNumber,omitempty"`
}

type CloudInit struct {
	RawCloudConfig *KeySelector `json:"rawCloudConfig,omitempty"`
	CloudConfig    *string      `json:"cloudConfig,omitempty"`
}

type Sysprep struct {
	RawSysprep *KeySelector `json:"rawSysprep,omitempty"`
	Sysprep    *string      `json:"sysprep,omitempty"`
}

type VAppConfig struct {
	RawProperties *string                            `json:"rawProperties,omitempty"`
	Properties    *[]KeyValueOrSecretKeySelectorPair `json:"properties,omitempty"`
}

type LinuxPrep struct {
	HardwareClockIsUTC     bool   `json:"hardwareClockIsUTC,omitempty"`
	TimeZone               string `json:"timeZone,omitempty"`
	CustomizeAtNextPowerOn *bool  `json:"customizeAtNextPowerOn,omitempty"`
}

type KeySelector struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

type KeyValueOrSecretKeySelectorPair struct {
	Key   string                   `json:"key"`
	Value ValueOrSecretKeySelector `json:"value,omitempty"`
}

// Only have Value for simplicity.
type ValueOrSecretKeySelector struct {
	Value string `json:"value,omitempty"`
}

type Crypto struct {
	EncryptionClassName   string `json:"encryptionClassName,omitempty"`
	UseDefaultKeyProvider bool   `json:"useDefaultKeyProvider,omitempty"`
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
	Namespace        string            `json:"namespace,omitempty"`
	Name             string            `json:"name,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	ImageName        string            `json:"image_name,omitempty"`
	VMClassName      string            `json:"vm_class_name,omitempty"`
	StorageClassName string            `json:"storage_class_name,omitempty"`
	ResourcePolicy   string            `json:"resource_policy,omitempty"`
	Network          Network           `json:"network,omitempty"`
	NetworkA2        NetworkA2         `json:"network_a2,omitempty"`
	Transport        string            `json:"transport,omitempty"`
	ConfigMapName    string            `json:"config_map_name,omitempty"`
	SecretName       string            `json:"secret_name,omitempty"`
	PowerState       string            `json:"power_state,omitempty"`
	PowerOffMode     string            `json:"power_off_mode,omitempty"`
	Bootstrap        Bootstrap         `json:"bootstrap,omitempty"`
	// Deprecated: For v1alpha5, use Hardware.Cdrom instead.
	Cdrom               []Cdrom                            `json:"cdrom,omitempty"`
	GuestID             string                             `json:"guest_id,omitempty"`
	PVCNames            []string                           `json:"pvc_names,omitempty"`
	Crypto              *Crypto                            `json:"crypto,omitempty"`
	GroupName           string                             `json:"groupName,omitempty"`
	CurrentSnapshotName string                             `json:"currentSnapshotName,omitempty"`
	PVCs                []PVC                              `json:"pvcs,omitempty"`
	Affinity            *vmopv1.AffinitySpec               `json:"affinity,omitempty"`
	Hardware            *vmopv1.VirtualMachineHardwareSpec `json:"hardware,omitempty"`
	Policies            []vmopv1.PolicySpec                `json:"policies,omitempty"`
}

// GetVirtualMachineYaml returns a v1alpha1 VirtualMachine YAML manifest.
func GetVirtualMachineYaml(vmYaml VirtualMachineYaml) []byte {
	return ToYAML(VirtualMachineA1(vmYaml))
}

// GetVirtualMachineYamlA2 returns a v1alpha2 VirtualMachine YAML manifest.
func GetVirtualMachineYamlA2(vmYaml VirtualMachineYaml) []byte {
	return ToYAML(must(VirtualMachineA2(vmYaml)))
}

// GetVirtualMachineWithMultiNetworkYamlA2 returns a v1alpha2 VirtualMachine
// YAML manifest with one network interface per vmYaml.NetworkA2.Interfaces
// entry.
func GetVirtualMachineWithMultiNetworkYamlA2(vmYaml VirtualMachineYaml) []byte {
	return GetVirtualMachineYamlA2(vmYaml)
}

// GetVirtualMachineYamlA3 returns a v1alpha3 VirtualMachine YAML manifest.
func GetVirtualMachineYamlA3(vmYaml VirtualMachineYaml) []byte {
	return ToYAML(VirtualMachineA3(vmYaml))
}

// GetVirtualMachineYamlA5 returns a multi-document manifest containing a
// v1alpha5 VirtualMachine followed by a PersistentVolumeClaim for each entry
// in vmYaml.PVCs.
func GetVirtualMachineYamlA5(vmYaml VirtualMachineYaml) []byte {
	return ToYAML(append(
		[]ctrlclient.Object{must(VirtualMachineA5(vmYaml))},
		must(persistentVolumeClaimObjects(vmYaml.PVCs))...)...)
}

// GetVirtualMachineYamlA6 returns a multi-document manifest containing a
// v1alpha6 VirtualMachine followed by a PersistentVolumeClaim for each entry
// in vmYaml.PVCs.
func GetVirtualMachineYamlA6(vmYaml VirtualMachineYaml) []byte {
	return ToYAML(append(
		[]ctrlclient.Object{must(VirtualMachineA6(vmYaml))},
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
		Name:        vmYaml.Name,
		Namespace:   vmYaml.Namespace,
		Labels:      vmYaml.Labels,
		Annotations: vmYaml.Annotations,
	}
}

// hasBootstrap returns true if any of the bootstrap providers are specified.
func (b Bootstrap) hasBootstrap() bool {
	return b.CloudInit != nil || b.Sysprep != nil || b.VAppConfig != nil || b.LinuxPrep != nil
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
