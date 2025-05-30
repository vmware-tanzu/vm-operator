// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
)

type CloudInitMetadata struct {
	InstanceID    string          `json:"instance-id,omitempty"`
	LocalHostname string          `json:"local-hostname,omitempty"`
	Hostname      string          `json:"hostname,omitempty"`
	Network       netplan.Network `json:"network,omitempty"`
	PublicKeys    string          `json:"public-keys,omitempty"`
	WaitOnNetwork *WaitOnNetwork  `json:"wait-on-network,omitempty"`
}

type WaitOnNetwork struct {
	IPv4 bool `json:"ipv4,omitempty"`
	IPv6 bool `json:"ipv6,omitempty"`
}

// CloudInitUserDataSecretKeys are the Secret keys that in v1a1 we'd check for the userdata.
// Specifically, CAPBK uses "value" for its key, while "user-data" is the preferred key nowadays.
// The 'value' key lookup will eventually be deprecated.
var CloudInitUserDataSecretKeys = []string{"user-data", "value"}

func BootStrapCloudInitInstanceID(
	vmCtx pkgctx.VirtualMachineContext,
	cloudInitSpec *vmopv1.VirtualMachineBootstrapCloudInitSpec) string {

	// The Cloud-Init instance ID is from spec.bootstrap.cloudInit.instanceID.
	iid := cloudInitSpec.InstanceID

	if iid == "" {
		// For new VM resources that use the Cloud-Init bootstrap provider, if
		// the field spec.bootstrap.cloudInit.instanceID is empty when the VM
		// resource is created, the field defaults to the value of
		// spec.biosUUID.
		//
		// However, to maintain backwards compatibility with older VM resources
		// that existed prior to the introduction of the field
		// spec.bootstrap.cloudInit.instanceID, use the VM resource's object
		// UID as the Cloud-Init instance ID if the value of
		// spec.bootstrap.cloudInit.instanceID is empty.
		iid = string(vmCtx.VM.UID)
	}

	if v := vmCtx.VM.Annotations[vmopv1.InstanceIDAnnotation]; v != "" && v != iid {
		// Before the introduction of spec.bootstrap.cloudInit.instanceID,
		// the Cloud-Init instance ID was set to the value of the VM object's
		// metadata.uid field.
		//
		// The annotation vmopv1.InstanceIDAnnotation was introduced as part of
		// https://github.com/vmware-tanzu/vm-operator/pull/211 to contend with
		// VMs previously deployed with Cloud-Init but sans user data. This was
		// a bug that meant Cloud-Init was not actually used to bootstrap the
		// guest. If we fixed the bug and did nothing else, affected VMs that
		// were already deployed would suddenly be subject to Cloud-Init running
		// for what it thought was the very first time. This could lead to the
		// erasure of the VM's SSH host keys or other behavior not acceptable
		// post first-boot.
		//
		// To avoid this side-effect, during the upgrade process, if a VM
		// used Cloud-Init sans user data, the annotation
		// vmopv1.InstanceIDAnnotation was added to that VM with the value
		// "iid-datasource-none". This caused Cloud-Init to run, but to do
		// nothing, preventing any adverse impact to the deployed VM.
		//
		// If the instance ID from the annotation is non-empty *and* is
		// different than the current instance ID value (whether it is from
		// spec.bootstrap.cloudInit.instanceID or metadata.uid), go ahead and
		// use the value from the annotation.
		iid = v
		delete(vmCtx.VM.Annotations, vmopv1.InstanceIDAnnotation)
	}

	// If the value of the Cloud-Init instance ID is different than the
	// one from spec.bootstrap.cloudInit.instanceID, then go ahead and assign
	// the derived value to the field. This ensures the object's desired state
	// matches the intent, whether it was derived from the value of metadata.uid
	// or the value of the annotation vmopv1.InstanceIDAnnotation.
	cloudInitSpec.InstanceID = iid

	return iid
}

func BootStrapCloudInit(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	cloudInitSpec *vmopv1.VirtualMachineBootstrapCloudInitSpec,
	bsArgs *BootstrapArgs) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	logger := logr.FromContextOrDiscard(vmCtx)
	logger.V(4).Info("Reconciling Cloud-Init bootstrap state")

	netPlan, err := network.NetPlanCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create NetPlan customization: %w", err)
	}

	sshPublicKeys := bsArgs.BootstrapData.Data["ssh-public-keys"]
	if len(cloudInitSpec.SSHAuthorizedKeys) > 0 {
		sshPublicKeys = strings.Join(cloudInitSpec.SSHAuthorizedKeys, "\n")
	}

	iid := BootStrapCloudInitInstanceID(vmCtx, cloudInitSpec)

	metadata, err := GetCloudInitMetadata(
		iid, bsArgs.HostName, bsArgs.DomainName, netPlan, sshPublicKeys,
		cloudInitSpec.WaitOnNetwork4, cloudInitSpec.WaitOnNetwork6)
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
		configSpec, customSpec, err = GetCloudInitPrepCustSpec(vmCtx, config, metadata, userdata)
	case constants.CloudInitTypeValueGuestInfo, "":
		fallthrough
	default:
		configSpec, err = GetCloudInitGuestInfoCustSpec(vmCtx, config, metadata, userdata)
	}

	if err != nil {
		return nil, nil, err
	}

	return configSpec, customSpec, nil
}

func GetCloudInitMetadata(
	instanceID, hostName, domainName string,
	netplan *netplan.Network,
	sshPublicKeys string,
	waitOnNetwork4, waitOnNetwork6 *bool) (string, error) {

	fqdn := hostName
	if domainName != "" {
		fqdn = hostName + "." + domainName
	}

	metadata := &CloudInitMetadata{
		InstanceID:    instanceID,
		LocalHostname: fqdn,
		Hostname:      fqdn,
		Network:       *netplan,
		PublicKeys:    sshPublicKeys,
	}

	// Only set WaitOnNetwork if at least one of the IPv4 or IPv6 values is set.
	if waitOnNetwork4 != nil || waitOnNetwork6 != nil {
		waitOnNetwork := WaitOnNetwork{}

		if waitOnNetwork4 != nil {
			waitOnNetwork.IPv4 = *waitOnNetwork4
		}

		if waitOnNetwork6 != nil {
			waitOnNetwork.IPv6 = *waitOnNetwork6
		}

		metadata.WaitOnNetwork = &waitOnNetwork
	}

	metadataBytes, err := yaml.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("yaml marshalling of cloud-init metadata failed: %w", err)
	}

	return string(metadataBytes), nil
}

func GetCloudInitPrepCustSpec(
	ctx context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	metadata, userdata string) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("Reconciling Cloud-Init Prep bootstrap state")

	if userdata != "" {
		// Ensure the data is normalized first to plain-text.
		plainText, err := pkgutil.TryToDecodeBase64Gzip([]byte(userdata))
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
	ctx context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	metadata, userdata string) (*vimtypes.VirtualMachineConfigSpec, error) {

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("Reconciling Cloud-Init GuestInfo bootstrap state")

	encodedMetadata, err := pkgutil.EncodeGzipBase64(metadata)
	if err != nil {
		return nil, fmt.Errorf("encoding cloud-init metadata failed: %w", err)
	}

	extraConfig := pkgutil.OptionValues{
		&vimtypes.OptionValue{
			Key:   constants.CloudInitGuestInfoMetadata,
			Value: encodedMetadata,
		},
		&vimtypes.OptionValue{
			Key:   constants.CloudInitGuestInfoMetadataEncoding,
			Value: "gzip+base64",
		},
	}

	if userdata != "" {
		// Ensure the data is normalized first to plain-text.
		plainText, err := pkgutil.TryToDecodeBase64Gzip([]byte(userdata))
		if err != nil {
			return nil, fmt.Errorf("decoding cloud-init userdata failed: %w", err)
		}

		encodedUserdata, err := pkgutil.EncodeGzipBase64(plainText)
		if err != nil {
			return nil, fmt.Errorf("encoding cloud-init userdata failed: %w", err)
		}

		extraConfig = append(
			extraConfig,
			&vimtypes.OptionValue{
				Key:   constants.CloudInitGuestInfoUserdata,
				Value: encodedUserdata,
			},
			&vimtypes.OptionValue{
				Key:   constants.CloudInitGuestInfoUserdataEncoding,
				Value: "gzip+base64",
			})
	}

	configSpec := &vimtypes.VirtualMachineConfigSpec{
		ExtraConfig: pkgutil.OptionValues(config.ExtraConfig).Diff(extraConfig...),
	}

	if config.VAppConfig != nil && config.VAppConfig.GetVmConfigInfo() != nil {
		// Remove the VAppConfig to ensure Cloud-Init inside the guest does not
		// activate and prefer the OVF datasource over the VMware datasource.
		// Only set this if needed so we don't do a needless Reconfigure.
		configSpec.VAppConfigRemoved = vimtypes.NewBool(true)
	}

	return configSpec, nil
}
