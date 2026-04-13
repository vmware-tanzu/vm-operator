// Copyright (c) 2020-2024 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"bytes"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

const (
	vmYamlDir = "test/e2e/fixtures/yaml/vmoperator/virtualmachines"
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
	Name                string                          `json:"name,omitempty"`
	ImageName           string                          `json:"imageName,omitempty"`
	ImageKind           string                          `json:"imageKind,omitempty"`
	Connected           bool                            `json:"connected,omitempty"`
	AllowGuestControl   bool                            `json:"allowGuestControl,omitempty"`
	ControllerBusNumber *int32                          `json:"controllerBusNumber,omitempty"`
	ControllerType      *vmopv1a5.VirtualControllerType `json:"controllerType,omitempty"`
	UnitNumber          *int32                          `json:"unitNumber,omitempty"`
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
	ControllerType  *vmopv1a5.VirtualControllerType     `json:"controller_type,omitempty"`
	ApplicationType vmopv1a5.VolumeApplicationType      `json:"application_type,omitempty"`
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
	Bootstrap        Bootstrap         `json:"bootstrap,omitempty"`
	// Deprecated: For v1alpha5, use Hardware.Cdrom instead.
	Cdrom               []Cdrom                              `json:"cdrom,omitempty"`
	GuestID             string                               `json:"guest_id,omitempty"`
	PVCNames            []string                             `json:"pvc_names,omitempty"`
	Crypto              *Crypto                              `json:"crypto,omitempty"`
	GroupName           string                               `json:"groupName,omitempty"`
	CurrentSnapshotName string                               `json:"currentSnapshotName,omitempty"`
	PVCs                []PVC                                `json:"pvcs,omitempty"`
	Affinity            *vmopv1a5.AffinitySpec               `json:"affinity,omitempty"`
	Hardware            *vmopv1a5.VirtualMachineHardwareSpec `json:"hardware,omitempty"`
	Policies            []vmopv1a5.PolicySpec                `json:"policies,omitempty"`
}

// GetVirtualMachineYaml returns a v1alpha1 VirtualMachine yaml from a templatized fixture.
func GetVirtualMachineYaml(vmYaml VirtualMachineYaml) []byte {
	vmYamlIn := fixtures.ReadFile(vmYamlDir, "singlevm.yaml.in")
	vmYamlBytes, _ := ReadVirtualMachineTemplate(vmYaml, vmYamlIn)

	return vmYamlBytes
}

// GetVirtualMachineYamlA2 returns a v1alpha2 VirtualMachine yaml from a templatized fixture.
func GetVirtualMachineYamlA2(vmYaml VirtualMachineYaml) []byte {
	vmYamlIn := fixtures.ReadFile(vmYamlDir, "v1a2singlevm.yaml.in")
	vmYamlBytes, _ := ReadVirtualMachineTemplate(vmYaml, vmYamlIn)

	return vmYamlBytes
}

// GetVirtualMachineWithMultiNetworkYamlA2 returns a v1alpha2 VirtualMachine with multiple network yaml
// from a templatized fixture.
func GetVirtualMachineWithMultiNetworkYamlA2(vmYaml VirtualMachineYaml) []byte {
	vmYamlIn := fixtures.ReadFile(vmYamlDir, "v1a2vm-multi-network.yaml.in")
	vmYamlBytes, _ := ReadVirtualMachineTemplate(vmYaml, vmYamlIn)

	return vmYamlBytes
}

// GetVirtualMachineYamlA3 returns a v1alpha3 VirtualMachine YAML from a templated fixture.
func GetVirtualMachineYamlA3(vmYaml VirtualMachineYaml) []byte {
	vmYamlIn := fixtures.ReadFile(vmYamlDir, "v1a3singlevm.yaml.in")
	vmYamlBytes, _ := ReadVirtualMachineTemplate(vmYaml, vmYamlIn)

	return vmYamlBytes
}

// GetVirtualMachineYamlA5 returns a v1alpha5 VirtualMachine YAML from a templated fixture.
func GetVirtualMachineYamlA5(vmYaml VirtualMachineYaml) []byte {
	vmYamlIn := fixtures.ReadFile(vmYamlDir, "v1a5singlevm.yaml.in")
	vmYamlBytes, _ := ReadVirtualMachineTemplate(vmYaml, vmYamlIn)

	return vmYamlBytes
}

func ReadVirtualMachineTemplate(vmYaml VirtualMachineYaml, input string) ([]byte, error) {
	tmpl := template.Must(template.New("vm").Parse(input))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, vmYaml)
	if err != nil {
		e2eframework.Failf("Failed executing vm template: %v", err)
	}

	return parsed.Bytes(), nil
}
