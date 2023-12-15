// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
)

type CloudInitMetadata struct {
	InstanceID    string          `yaml:"instance-id,omitempty"`
	LocalHostname string          `yaml:"local-hostname,omitempty"`
	Hostname      string          `yaml:"hostname,omitempty"`
	Network       network.Netplan `yaml:"network,omitempty"`
	PublicKeys    string          `yaml:"public-keys,omitempty"`
}

func BootStrapCloudInit(
	vmCtx context.VirtualMachineContextA2,
	config *types.VirtualMachineConfigInfo,
	cloudInitSpec *vmopv1.VirtualMachineBootstrapCloudInitSpec,
	bsArgs *BootstrapArgs) (*types.VirtualMachineConfigSpec, *types.CustomizationSpec, error) {

	netPlan, err := network.NetPlanCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create NetPlan customization: %w", err)
	}

	sshPublicKeys := bsArgs.BootstrapData.Data["ssh-public-keys"]
	if len(cloudInitSpec.SSHAuthorizedKeys) > 0 {
		sshPublicKeys = strings.Join(cloudInitSpec.SSHAuthorizedKeys, "\n")
	}

	metadata, err := GetCloudInitMetadata(string(vmCtx.VM.UID), bsArgs.Hostname, netPlan, sshPublicKeys)
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
		// Check for the 'user-data' key as per official contract and API documentation.
		// Additionally, to support the cluster bootstrap data supplied by CAPBK's secret,
		// we check for a 'value' key when 'user-data' is not supplied.
		// The 'value' key lookup will eventually be deprecated.
		for _, key := range []string{raw.Key, "user-data", "value"} {
			if data := bsArgs.BootstrapData.Data[key]; data != "" {
				userdata = data
				break
			}
		}

		// NOTE: The old code didn't error out if userdata wasn't found, so keep going.
	}

	var configSpec *types.VirtualMachineConfigSpec
	var customSpec *types.CustomizationSpec

	switch vmCtx.VM.Annotations[constants.CloudInitTypeAnnotation] {
	case constants.CloudInitTypeValueCloudInitPrep:
		configSpec, customSpec, err = GetCloudInitPrepCustSpec(metadata, userdata)
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
	metadata, userdata string) (*types.VirtualMachineConfigSpec, *types.CustomizationSpec, error) {

	if userdata != "" {
		// Ensure the data is normalized first to plain-text.
		plainText, err := util.TryToDecodeBase64Gzip([]byte(userdata))
		if err != nil {
			return nil, nil, fmt.Errorf("decoding cloud-init prep userdata failed: %w", err)
		}

		userdata = plainText
	}

	// FIXME: This is the old behavior but isn't quite correct: we need current config, and only
	// do this if the transport isn't already set. Otherwise, this always results in a Reconfigure.
	configSpec := &types.VirtualMachineConfigSpec{
		VAppConfig: &types.VmConfigSpec{
			// Ensure the transport is guestInfo in case the VM does not have
			// a CD-ROM device required to use the ISO transport.
			OvfEnvironmentTransport: []string{OvfEnvironmentTransportGuestInfo},
		},
	}

	customSpec := &types.CustomizationSpec{
		Identity: &internal.CustomizationCloudinitPrep{
			Metadata: metadata,
			Userdata: userdata,
		},
	}

	return configSpec, customSpec, nil
}

func GetCloudInitGuestInfoCustSpec(
	config *types.VirtualMachineConfigInfo,
	metadata, userdata string) (*types.VirtualMachineConfigSpec, error) {

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

	// FIXME: This is the old behavior but isn't quite correct: we really need the current ExtraConfig,
	// and then only add/update it if new or updated (so we don't customize with stale data). Also,
	// always setting VAppConfigRemoved isn't correct: should only set it if the VM has vApp data.
	// As-is we will always Reconfigure the VM.
	configSpec := &types.VirtualMachineConfigSpec{}
	configSpec.ExtraConfig = util.AppendNewExtraConfigValues(config.ExtraConfig, extraConfig)
	// Remove the VAppConfig to ensure Cloud-Init inside the guest does not
	// activate and prefer the OVF datasource over the VMware datasource.
	configSpec.VAppConfigRemoved = types.NewBool(true) // FIXME

	return configSpec, nil
}
