// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	goctx "context"
	"fmt"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
)

func BootstrapSysPrep(
	_ goctx.Context,
	config *vimTypes.VirtualMachineConfigInfo,
	sysPrepSpec *vmopv1.VirtualMachineBootstrapSysprepSpec,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimTypes.VirtualMachineConfigSpec, *vimTypes.CustomizationSpec, error) {

	var (
		data     string
		identity vimTypes.BaseCustomizationIdentitySettings
	)
	key := "unattend"

	if sysPrepSpec.RawSysprep != nil {
		var err error

		if sysPrepSpec.RawSysprep.Key != "" {
			key = sysPrepSpec.RawSysprep.Key
		}

		data = bsArgs.BootstrapData.Data[key]
		if data == "" {
			return nil, nil, fmt.Errorf("no Sysprep XML data with key %q", key)
		}

		// Ensure the data is normalized first to plain-text.
		data, err = util.TryToDecodeBase64Gzip([]byte(data))
		if err != nil {
			return nil, nil, fmt.Errorf("decoding Sysprep unattend XML failed: %w", err)
		}

		if bsArgs.TemplateRenderFn != nil {
			data = bsArgs.TemplateRenderFn(key, data)
		}

		identity = &vimTypes.CustomizationSysprepText{
			Value: data,
		}
	} else if sysPrep := sysPrepSpec.Sysprep; sysPrep != nil {
		identity = convertTo(sysPrep, bsArgs)
	}

	nicSettingMap, err := network.GuestOSCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GSOC adapter mappings: %w", err)
	}

	customSpec := &vimTypes.CustomizationSpec{
		Identity: identity,
		GlobalIPSettings: vimTypes.CustomizationGlobalIPSettings{
			DnsSuffixList: bsArgs.SearchSuffixes,
			DnsServerList: bsArgs.DNSServers,
		},
		NicSettingMap: nicSettingMap,
	}

	var configSpec *vimTypes.VirtualMachineConfigSpec
	if vAppConfigSpec != nil {
		configSpec = &vimTypes.VirtualMachineConfigSpec{}
		configSpec.VAppConfig = GetOVFVAppConfigForConfigSpec(
			config,
			vAppConfigSpec,
			bsArgs.BootstrapData.VAppData,
			bsArgs.BootstrapData.VAppExData,
			bsArgs.TemplateRenderFn)
	}

	return configSpec, customSpec, nil
}

func convertTo(from *vmopv1sysprep.Sysprep, bsArgs *BootstrapArgs) *vimTypes.CustomizationSysprep {
	bootstrapData := bsArgs.BootstrapData
	sysprepCustomization := &vimTypes.CustomizationSysprep{}

	if from.GUIUnattended != nil {
		sysprepCustomization.GuiUnattended = vimTypes.CustomizationGuiUnattended{
			TimeZone:       from.GUIUnattended.TimeZone,
			AutoLogon:      from.GUIUnattended.AutoLogon,
			AutoLogonCount: from.GUIUnattended.AutoLogonCount,
		}
		if bootstrapData.Sysprep != nil && bootstrapData.Sysprep.Password != "" {
			sysprepCustomization.GuiUnattended.Password = &vimTypes.CustomizationPassword{
				Value:     bootstrapData.Sysprep.Password,
				PlainText: true,
			}
		}
	}

	sysprepCustomization.UserData = vimTypes.CustomizationUserData{
		// This is a mandatory field
		ComputerName: &vimTypes.CustomizationFixedName{
			Name: bsArgs.ComputerName,
		},
	}
	if from.UserData != nil {
		sysprepCustomization.UserData.FullName = from.UserData.FullName
		sysprepCustomization.UserData.OrgName = from.UserData.OrgName
		// In the case of a VMI with volume license key, this might not be set.
		// Hence, add a check to see if the productID is set to empty.
		if bootstrapData.Sysprep != nil && bootstrapData.Sysprep.ProductID != "" {
			sysprepCustomization.UserData.ProductId = bootstrapData.Sysprep.ProductID
		}
	}

	sysprepCustomization.GuiRunOnce = &vimTypes.CustomizationGuiRunOnce{
		CommandList: from.GUIRunOnce.Commands,
	}

	if from.Identification != nil {
		sysprepCustomization.Identification = vimTypes.CustomizationIdentification{
			JoinWorkgroup: from.Identification.JoinWorkgroup,
			JoinDomain:    from.Identification.JoinDomain,
			DomainAdmin:   from.Identification.DomainAdmin,
		}
		if bootstrapData.Sysprep != nil && bootstrapData.Sysprep.DomainPassword != "" {
			sysprepCustomization.Identification.DomainAdminPassword = &vimTypes.CustomizationPassword{
				Value:     bootstrapData.Sysprep.DomainPassword,
				PlainText: true,
			}
		}
	}

	if from.LicenseFilePrintData != nil {
		sysprepCustomization.LicenseFilePrintData = convertLicenseFilePrintDataTo(from.LicenseFilePrintData)
	}

	return sysprepCustomization
}

func convertLicenseFilePrintDataTo(from *vmopv1sysprep.LicenseFilePrintData) *vimTypes.CustomizationLicenseFilePrintData {
	custLicenseFilePrintData := &vimTypes.CustomizationLicenseFilePrintData{
		AutoMode: parseLicenseDataMode(from.AutoMode),
	}
	if from.AutoUsers != nil {
		custLicenseFilePrintData.AutoUsers = *from.AutoUsers
	}
	return custLicenseFilePrintData
}

func parseLicenseDataMode(mode vmopv1sysprep.CustomizationLicenseDataMode) vimTypes.CustomizationLicenseDataMode {
	switch mode {
	case vmopv1sysprep.CustomizationLicenseDataModePerServer:
		return vimTypes.CustomizationLicenseDataModePerServer
	case vmopv1sysprep.CustomizationLicenseDataModePerSeat:
		return vimTypes.CustomizationLicenseDataModePerSeat
	default:
		return ""
	}
}
