// Copyright (c) 2021-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"text/template"

	vimTypes "github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/vmware/govmomi/task"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
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
	PublicKeys    string          `yaml:"public-keys,omitempty"`
}

func GetCloudInitMetadata(vm *v1alpha1.VirtualMachine,
	netplan network.Netplan,
	data map[string]string) (string, error) {

	metadataObj := &CloudInitMetadata{
		InstanceID:    string(vm.UID),
		LocalHostname: vm.Name,
		Hostname:      vm.Name,
		Network:       netplan,
		PublicKeys:    data["ssh-public-keys"],
	}

	metadataBytes, err := yaml.Marshal(metadataObj)
	if err != nil {
		return "", fmt.Errorf("yaml marshalling of cloud-init metadata failed %v", err)
	}

	return string(metadataBytes), nil
}

func GetCloudInitPrepCustSpec(
	cloudInitMetadata string,
	updateArgs VMUpdateArgs) (*vimTypes.CustomizationSpec, error) {

	userdata := updateArgs.VMMetadata.Data["user-data"]

	if userdata != "" {
		// Ensure the data is normalized first to plain-text.
		plainText, err := util.TryToDecodeBase64Gzip([]byte(userdata))
		if err != nil {
			return nil, fmt.Errorf("decoding cloud-init prep userdata failed %v", err)
		}
		userdata = plainText
	}

	return &vimTypes.CustomizationSpec{
		Identity: &internal.CustomizationCloudinitPrep{
			Metadata: cloudInitMetadata,
			Userdata: userdata,
		},
	}, nil
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
		// Ensure the data is normalized first to plain-text.
		plainText, err := util.TryToDecodeBase64Gzip([]byte(data))
		if err != nil {
			return nil, fmt.Errorf("decoding cloud-init userdata failed %v", err)
		}

		encodedUserdata, err := EncodeGzipBase64(plainText)
		if err != nil {
			return nil, fmt.Errorf("encoding cloud-init userdata failed %v", err)
		}

		extraConfig[constants.CloudInitGuestInfoUserdata] = encodedUserdata
		extraConfig[constants.CloudInitGuestInfoUserdataEncoding] = "gzip+base64"
	}

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	configSpec.ExtraConfig = MergeExtraConfig(config.ExtraConfig, extraConfig)

	// Remove the VAppConfig to ensure Cloud-Init inside of the guest does not
	// activate and prefer the OVF datasource over the VMware datasource.
	vappConfigRemoved := true
	configSpec.VAppConfigRemoved = &vappConfigRemoved

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

	cloudInitMetadata, err := GetCloudInitMetadata(vmCtx.VM, netplan, updateArgs.VMMetadata.Data)
	if err != nil {
		return nil, nil, err
	}

	var configSpec *vimTypes.VirtualMachineConfigSpec
	var custSpec *vimTypes.CustomizationSpec

	switch vmCtx.VM.Annotations[constants.CloudInitTypeAnnotation] {
	case constants.CloudInitTypeValueCloudInitPrep:
		custSpec, err = GetCloudInitPrepCustSpec(cloudInitMetadata, updateArgs)
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

	if lib.IsVMServicePublicCloudBYOIFSSEnabled() {
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
	case v1alpha1.VirtualMachineMetadataVAppConfigTransport:
		configSpec = GetOvfEnvCustSpec(config, updateArgs)
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

func NicInfoToDevicesStatus(updateArgs VMUpdateArgs) []v1alpha1.NetworkDeviceStatus {
	networkDevicesStatus := make([]v1alpha1.NetworkDeviceStatus, 0, len(updateArgs.NetIfList))

	// TODO: Add MacAddress field when the it's present in updateArgs.NetIfList
	for _, info := range updateArgs.NetIfList {
		ipConfig := info.IPConfiguration
		networkDevice := v1alpha1.NetworkDeviceStatus{
			Gateway4:    ipConfig.Gateway,
			IPAddresses: []string{network.ToCidrNotation(ipConfig.IP, ipConfig.SubnetMask)},
		}
		networkDevicesStatus = append(networkDevicesStatus, networkDevice)
	}
	return networkDevicesStatus
}

// TemplateVMMetadata can convert templated expressions to dynamic configuration data.
func TemplateVMMetadata(vmCtx context.VirtualMachineContext, updateArgs VMUpdateArgs) {
	networkDevicesStatus := NicInfoToDevicesStatus(updateArgs)

	networkStatus := v1alpha1.NetworkStatus{
		Devices:     networkDevicesStatus,
		Nameservers: updateArgs.DNSServers,
	}

	templateData := struct {
		V1alpha1 v1alpha1.VirtualMachineTemplate
	}{
		V1alpha1: v1alpha1.VirtualMachineTemplate{
			Net: networkStatus,
			VM:  vmCtx.VM,
		},
	}

	// TODO: Differentiate IP4 and IP6 when IP6 is supported
	// eg. adding V1alpha1_FirstIPv4FromNIC

	// Get the first IP address from the first NIC.
	v1alpha1FirstIP := func() (string, error) {
		if len(networkDevicesStatus) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatus[0].IPAddresses[0], nil
	}

	// Get the first IP address from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha1FirstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatus) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatus) {
			return "", errors.New("index out of bound")
		}
		return networkDevicesStatus[index].IPAddresses[0], nil
	}

	// Get all IP addresses from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha1IPsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatus) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatus) {
			return []string{""}, errors.New("index out of bound")
		}
		return networkDevicesStatus[index].IPAddresses, nil
	}

	// Get subnet mask from a CIDR notation IP address and prefix length
	// if IP address and prefix length not valid, throw an error and template string won't be parsed
	v1alpha1SubnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	// Format an IP address with default netmask CIDR
	// if IP not valid, throw an error and template string won't be parsed
	v1alpha1IP := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + fmt.Sprintf("%d", int32(ones))
		return expectedCidrNotation, nil
	}

	// Format an IP address with network length(eg. /24) or decimal notation(eg. 255.255.255.0)
	// if input netmask not valid, throw an error and template string won't be parsed
	v1alpha1FormatIP := func(IP string, netmask string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		var userIP string
		if strings.HasPrefix(netmask, "/") {
			userIP = IP + netmask
		} else {
			userIP = network.ToCidrNotation(IP, netmask)
		}
		// validate IP and netmask's combination
		_, _, err := net.ParseCIDR(userIP)
		if err != nil {
			return "", err
		}
		return userIP, nil
	}

	// Format the first occurred count of nameservers with specific delimiter
	// A negative count number would mean format all nameservers
	v1alpha1FormatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		if len(networkStatus.Nameservers) == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		if count < 0 || count >= len(networkStatus.Nameservers) {
			nameservers = networkStatus.Nameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = networkStatus.Nameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	funcMap := template.FuncMap{
		constants.V1alpha1FirstIPFromNIC:    v1alpha1FirstIPFromNIC,
		constants.V1alpha1FirstIP:           v1alpha1FirstIP,
		constants.V1alpha1IPsFromNIC:        v1alpha1IPsFromNIC,
		constants.V1alpha1FormatIP:          v1alpha1FormatIP,
		constants.V1alpha1IP:                v1alpha1IP,
		constants.V1alpha1SubnetMask:        v1alpha1SubnetMask,
		constants.V1alpha1FormatNameservers: v1alpha1FormatNameservers,
	}

	// skip parsing when encountering escape character('\{',"\}")
	normalizeStr := func(str string) string {
		if strings.Contains(str, "\\{") || strings.Contains(str, "\\}") {
			str = strings.ReplaceAll(str, "\\{", "{")
			str = strings.ReplaceAll(str, "\\}", "}")
		}
		return str
	}

	renderTemplate := func(name, templateStr string) string {
		templ, err := template.New(name).Funcs(funcMap).Parse(templateStr)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to parse template", "templateStr", templateStr)
			// TODO: emit related events
			return normalizeStr(templateStr)
		}
		var doc bytes.Buffer
		err = templ.Execute(&doc, &templateData)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to execute template", "templateStr", templateStr)
			// TODO: emit related events
			return normalizeStr(templateStr)
		}
		return normalizeStr(doc.String())
	}

	data := updateArgs.VMMetadata.Data
	for key, val := range data {
		data[key] = renderTemplate(key, val)
	}
}
