// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	vimTypes "github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/task"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func IsCustomizationPendingExtraConfig(extraConfig []vimTypes.BaseOptionValue) bool {
	for _, opt := range extraConfig {
		if optValue := opt.GetOptionValue(); optValue != nil {
			if optValue.Key == constants.GOSCPendingExtraConfigKey {
				return optValue.Value.(string) != ""
			}
		}
	}
	return false
}

func isCustomizationPendingError(err error) bool {
	if te, ok := err.(task.Error); ok {
		if _, ok := te.Fault().(*vimTypes.CustomizationPending); ok {
			return true
		}
	}
	return false
}

func GetLinuxPrepCustSpec(vmName string, updateArgs VMUpdateArgs) *vimTypes.CustomizationSpec {
	return &vimTypes.CustomizationSpec{
		Identity: &vimTypes.CustomizationLinuxPrep{
			HostName: &vimTypes.CustomizationFixedName{
				Name: vmName,
			},
			HwClockUTC: vimTypes.NewBool(true),
		},
		GlobalIPSettings: vimTypes.CustomizationGlobalIPSettings{
			DnsServerList: updateArgs.DNSServers,
		},
		NicSettingMap: updateArgs.NetIfList.GetInterfaceCustomizations(),
	}
}

type CloudInitMetadata struct {
	InstanceID    string          `yaml:"instance-id,omitempty"`
	LocalHostname string          `yaml:"local-hostname,omitempty"`
	Hostname      string          `yaml:"hostname,omitempty"`
	Network       network.Netplan `yaml:"network,omitempty"`
}

func GetCloudInitMetadata(
	vmName string,
	netplan network.Netplan) (string, error) {

	metadataObj := &CloudInitMetadata{
		InstanceID:    vmName,
		LocalHostname: vmName,
		Hostname:      vmName,
		Network:       netplan,
	}

	metadataBytes, err := yaml.Marshal(metadataObj)
	if err != nil {
		return "", fmt.Errorf("yaml marshalling of cloud-init metadata failed %v", err)
	}

	return string(metadataBytes), nil
}

func GetCloudInitPrepCustSpec(
	cloudInitMetadata string,
	updateArgs VMUpdateArgs) *vimTypes.CustomizationSpec {

	return &vimTypes.CustomizationSpec{
		Identity: &internal.CustomizationCloudinitPrep{
			Metadata: cloudInitMetadata,
			Userdata: updateArgs.VMMetadata.Data["user-data"],
		},
	}
}

func GetCloudInitGuestInfoCustSpec(
	cloudInitMetadata string,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs) (*vimTypes.VirtualMachineConfigSpec, error) {

	extraConfig := map[string]string{}

	encodedMetadata, err := EncodeGzipBase64(cloudInitMetadata)
	if err != nil {
		return nil, fmt.Errorf("encoding cloud-init metadata failed %v", err)
	}
	extraConfig[constants.CloudInitGuestInfoMetadata] = encodedMetadata
	extraConfig[constants.CloudInitGuestInfoMetadataEncoding] = "gzip+base64"

	var data string
	// Check for the 'user-data' key as per official contract and API documentation.
	// Additionally, To support the cluster bootstrap data supplied by CAPBK's secret,
	// we check for a 'value' key when 'user-data' is not supplied. The 'value' key
	// lookup will eventually be deprecated.
	if userdata := updateArgs.VMMetadata.Data["user-data"]; userdata != "" {
		data = userdata
	} else if value := updateArgs.VMMetadata.Data["value"]; value != "" {
		data = value
	}

	if data != "" {
		encodedUserdata, err := EncodeGzipBase64(data)
		if err != nil {
			return nil, fmt.Errorf("encoding cloud-init userdata failed %v", err)
		}

		extraConfig[constants.CloudInitGuestInfoUserdata] = encodedUserdata
		extraConfig[constants.CloudInitGuestInfoUserdataEncoding] = "gzip+base64"
	}

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	configSpec.ExtraConfig = MergeExtraConfig(config.ExtraConfig, extraConfig)
	return configSpec, nil
}

func GetExtraConfigCustSpec(
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs) *vimTypes.VirtualMachineConfigSpec {

	extraConfig := make(map[string]string)
	for k, v := range updateArgs.VMMetadata.Data {
		if strings.HasPrefix(k, constants.ExtraConfigGuestInfoPrefix) {
			extraConfig[k] = v
		}
	}
	if len(extraConfig) == 0 {
		return nil
	}

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	configSpec.ExtraConfig = MergeExtraConfig(config.ExtraConfig, extraConfig)
	return configSpec
}

func GetOvfEnvCustSpec(
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs) *vimTypes.VirtualMachineConfigSpec {

	if config.VAppConfig == nil {
		return nil
	}

	vAppConfigInfo := config.VAppConfig.GetVmConfigInfo()
	if vAppConfigInfo == nil {
		return nil
	}

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	configSpec.VAppConfig = GetMergedvAppConfigSpec(updateArgs.VMMetadata.Data, vAppConfigInfo.Property)
	return configSpec
}

func customizeCloudInit(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs) (*vimTypes.VirtualMachineConfigSpec, *vimTypes.CustomizationSpec, error) {

	ethCards, err := resVM.GetNetworkDevices(vmCtx)
	if err != nil {
		return nil, nil, err
	}

	netplan := updateArgs.NetIfList.GetNetplan(ethCards, updateArgs.DNSServers)

	cloudInitMetadata, err := GetCloudInitMetadata(vmCtx.VM.Name, netplan)
	if err != nil {
		return nil, nil, err
	}

	var configSpec *vimTypes.VirtualMachineConfigSpec
	var custSpec *vimTypes.CustomizationSpec

	switch vmCtx.VM.Annotations[constants.CloudInitTypeAnnotation] {
	case constants.CloudInitTypeValueCloudInitPrep:
		custSpec = GetCloudInitPrepCustSpec(cloudInitMetadata, updateArgs)
	case constants.CloudInitTypeValueGuestInfo, "":
		fallthrough
	default:
		configSpec, err = GetCloudInitGuestInfoCustSpec(cloudInitMetadata, config, updateArgs)
	}

	if err != nil {
		return nil, nil, err
	}

	return configSpec, custSpec, nil
}

func (s *Session) customize(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs) error {

	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		TemplateVMMetadata(vmCtx, updateArgs)
	}

	transport := updateArgs.VMMetadata.Transport

	var configSpec *vimTypes.VirtualMachineConfigSpec
	var custSpec *vimTypes.CustomizationSpec
	var err error

	switch transport {
	case v1alpha1.VirtualMachineMetadataCloudInitTransport:
		configSpec, custSpec, err = customizeCloudInit(vmCtx, resVM, config, updateArgs)
	case v1alpha1.VirtualMachineMetadataOvfEnvTransport:
		configSpec = GetOvfEnvCustSpec(config, updateArgs)
		custSpec = GetLinuxPrepCustSpec(vmCtx.VM.Name, updateArgs)
	case v1alpha1.VirtualMachineMetadataExtraConfigTransport:
		configSpec = GetExtraConfigCustSpec(config, updateArgs)
		custSpec = GetLinuxPrepCustSpec(vmCtx.VM.Name, updateArgs)
	default:
		custSpec = GetLinuxPrepCustSpec(vmCtx.VM.Name, updateArgs)
	}

	if err != nil {
		return err
	}

	if configSpec != nil {
		defaultConfigSpec := &vimTypes.VirtualMachineConfigSpec{}
		if !apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
			vmCtx.Logger.Info("Customization Reconfigure", "configSpec", configSpec)
			if err := resVM.Reconfigure(vmCtx, configSpec); err != nil {
				vmCtx.Logger.Error(err, "customization reconfigure failed")
				return err
			}
		}
	}

	if custSpec != nil {
		if vmCtx.VM.Annotations[constants.VSphereCustomizationBypassKey] == constants.VSphereCustomizationBypassDisable {
			vmCtx.Logger.Info("Skipping vsphere customization because of vsphere-customization bypass annotation")
			return nil
		}
		if IsCustomizationPendingExtraConfig(config.ExtraConfig) {
			vmCtx.Logger.Info("Skipping customization because it is already pending")
			// TODO: We should really determine if the pending customization is stale, clear it
			// if so, and then re-customize. Otherwise, the Customize call could perpetually fail
			// preventing power on.
			return nil
		}
		vmCtx.Logger.Info("Customizing VM", "customizationSpec", *custSpec)
		if err := resVM.Customize(vmCtx, *custSpec); err != nil {
			// isCustomizationPendingExtraConfig() above is suppose to prevent this error, but
			// handle it explicitly here just in case so VM reconciliation can proceed.
			if !isCustomizationPendingError(err) {
				return err
			}
		}
	}

	return nil
}

// TemplateData is used to specify templating values
// for guest customization data. Users will be able
// to specify fields from this struct as values
// for customization. E.g.: {{ (index .NetworkInterfaces 0).Gateway }}.
type TemplateData struct {
	NetworkInterfaces []network.IPConfig
	NameServers       []string
}

func TemplateVMMetadata(vmCtx context.VirtualMachineContext, updateArgs VMUpdateArgs) {
	templateData := TemplateData{
		NetworkInterfaces: updateArgs.NetIfList.GetIPConfigs(),
		NameServers:       updateArgs.DNSServers,
	}

	renderTemplate := func(name, templateStr string) string {
		templ, err := template.New(name).Parse(templateStr)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to parse template", "templateStr", templateStr)
			// TODO: emit related events
			return templateStr
		}
		var doc bytes.Buffer
		err = templ.Execute(&doc, &templateData)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to execute template", "templateStr", templateStr)
			// TODO: emit related events
			return templateStr
		}
		return doc.String()
	}

	data := updateArgs.VMMetadata.Data
	for key, val := range data {
		data[key] = renderTemplate(key, val)
	}
}
