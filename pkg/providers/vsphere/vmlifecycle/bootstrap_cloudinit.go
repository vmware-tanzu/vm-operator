// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"fmt"
	"slices"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
)

type CloudInitMetadata struct {
	InstanceID    string          `yaml:"instance-id,omitempty"`
	LocalHostname string          `yaml:"local-hostname,omitempty"`
	Hostname      string          `yaml:"hostname,omitempty"`
	Network       network.Netplan `yaml:"network,omitempty"`
	PublicKeys    string          `yaml:"public-keys,omitempty"`
}

// CloudInitUserDataSecretKeys are the Secret keys that in v1a1 we'd check for the userdata.
// Specifically, CAPBK uses "value" for its key, while "user-data" is the preferred key nowadays.
// The 'value' key lookup will eventually be deprecated.
var CloudInitUserDataSecretKeys = []string{"user-data", "value"}

func BootStrapCloudInit(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	cloudInitSpec *vmopv1.VirtualMachineBootstrapCloudInitSpec,
	bsArgs *BootstrapArgs) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	netPlan, err := network.NetPlanCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create NetPlan customization: %w", err)
	}

	sshPublicKeys := bsArgs.BootstrapData.Data["ssh-public-keys"]
	if len(cloudInitSpec.SSHAuthorizedKeys) > 0 {
		sshPublicKeys = strings.Join(cloudInitSpec.SSHAuthorizedKeys, "\n")
	}

	uid := cloudInitSpec.InstanceID
	if uid == "" {
		// InstanceID will be set to the VM bios uuid when creating new VMs.
		// Use VM.UID for backward compatibility when InstanceID is not set.
		uid = string(vmCtx.VM.UID)
	}
	if value, ok := vmCtx.VM.Annotations[vmopv1.InstanceIDAnnotation]; ok {
		uid = value
	}
	metadata, err := GetCloudInitMetadata(uid, bsArgs.Hostname, netPlan, sshPublicKeys)
	if err != nil {
		return nil, nil, err
	}

	var userdata string
	if cooked := cloudInitSpec.CloudConfig; cooked != nil {
		if bsArgs.CloudConfig == nil {
			return nil, nil, fmt.Errorf("cloudConfigSecretData is nil")
		}
		data, err := cloudinit.MarshalYAML(*cooked, *bsArgs.CloudConfig)
		if err != nil {
			return nil, nil, err
		}
		userdata = data
	} else if raw := cloudInitSpec.RawCloudConfig; raw != nil {
		keys := []string{raw.Key}
		for _, key := range append(keys, CloudInitUserDataSecretKeys...) {
			if data := bsArgs.BootstrapData.Data[key]; data != "" {
				userdata = data
				break
			}
		}

		// NOTE: The old code didn't error out if userdata wasn't found, so keep going.
	}

	var configSpec *vimtypes.VirtualMachineConfigSpec
	var customSpec *vimtypes.CustomizationSpec

	switch vmCtx.VM.Annotations[constants.CloudInitTypeAnnotation] {
	case constants.CloudInitTypeValueCloudInitPrep:
		configSpec, customSpec, err = GetCloudInitPrepCustSpec(config, metadata, userdata)
	case constants.CloudInitTypeValueGuestInfo, "":
		fallthrough
	default:
		configSpec, err = GetCloudInitGuestInfoCustSpec(config, metadata, userdata)
	}

	if err != nil {
		return nil, nil, err
	}

	return configSpec, customSpec, nil
}

func GetCloudInitMetadata(
	uid string,
	hostname string,
	netplan *network.Netplan,
	sshPublicKeys string) (string, error) {

	metadata := &CloudInitMetadata{
		InstanceID:    uid,
		LocalHostname: hostname,
		Hostname:      hostname,
		Network:       *netplan,
		PublicKeys:    sshPublicKeys,
	}

	metadataBytes, err := yaml.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("yaml marshalling of cloud-init metadata failed: %w", err)
	}

	return string(metadataBytes), nil
}

func GetCloudInitPrepCustSpec(
	config *vimtypes.VirtualMachineConfigInfo,
	metadata, userdata string) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	if userdata != "" {
		// Ensure the data is normalized first to plain-text.
		plainText, err := util.TryToDecodeBase64Gzip([]byte(userdata))
		if err != nil {
			return nil, nil, fmt.Errorf("decoding cloud-init prep userdata failed: %w", err)
		}

		userdata = plainText
	}

	var configSpec *vimtypes.VirtualMachineConfigSpec

	// Set the ConfigSpec if the vAppConfig needs updating so a Reconfigure is only done if needed.
	if vAppConfig := config.VAppConfig; vAppConfig == nil || vAppConfig.GetVmConfigInfo() == nil ||
		!slices.Contains(vAppConfig.GetVmConfigInfo().OvfEnvironmentTransport, OvfEnvironmentTransportGuestInfo) {

		configSpec = &vimtypes.VirtualMachineConfigSpec{
			VAppConfig: &vimtypes.VmConfigSpec{
				// Ensure the transport is guestInfo in case the VM does not have
				// a CD-ROM device required to use the ISO transport.
				OvfEnvironmentTransport: []string{OvfEnvironmentTransportGuestInfo},
			},
		}
	}

	customSpec := &vimtypes.CustomizationSpec{
		Identity: &internal.CustomizationCloudinitPrep{
			Metadata: metadata,
			Userdata: userdata,
		},
	}

	return configSpec, customSpec, nil
}

func GetCloudInitGuestInfoCustSpec(
	config *vimtypes.VirtualMachineConfigInfo,
	metadata, userdata string) (*vimtypes.VirtualMachineConfigSpec, error) {

	encodedMetadata, err := util.EncodeGzipBase64(metadata)
	if err != nil {
		return nil, fmt.Errorf("encoding cloud-init metadata failed: %w", err)
	}

	extraConfig := map[string]string{
		constants.CloudInitGuestInfoMetadata:         encodedMetadata,
		constants.CloudInitGuestInfoMetadataEncoding: "gzip+base64",
	}

	if userdata != "" {
		// Ensure the data is normalized first to plain-text.
		plainText, err := util.TryToDecodeBase64Gzip([]byte(userdata))
		if err != nil {
			return nil, fmt.Errorf("decoding cloud-init userdata failed: %w", err)
		}

		encodedUserdata, err := util.EncodeGzipBase64(plainText)
		if err != nil {
			return nil, fmt.Errorf("encoding cloud-init userdata failed: %w", err)
		}

		extraConfig[constants.CloudInitGuestInfoUserdata] = encodedUserdata
		extraConfig[constants.CloudInitGuestInfoUserdataEncoding] = "gzip+base64"
	}

	configSpec := &vimtypes.VirtualMachineConfigSpec{}
	configSpec.ExtraConfig = util.MergeExtraConfig(config.ExtraConfig, extraConfig)
	if config.VAppConfig != nil && config.VAppConfig.GetVmConfigInfo() != nil {
		// Remove the VAppConfig to ensure Cloud-Init inside the guest does not
		// activate and prefer the OVF datasource over the VMware datasource.
		// Only set this if needed so we don't do a needless Reconfigure.
		configSpec.VAppConfigRemoved = vimtypes.NewBool(true)
	}

	return configSpec, nil
}
