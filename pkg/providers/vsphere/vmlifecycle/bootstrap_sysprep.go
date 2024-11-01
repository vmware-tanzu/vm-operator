// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

func BootstrapSysPrep(
	_ context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	sysPrepSpec *vmopv1.VirtualMachineBootstrapSysprepSpec,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	var (
		data     string
		identity vimtypes.BaseCustomizationIdentitySettings
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

		identity = &vimtypes.CustomizationSysprepText{
			Value: data,
		}
	} else if sysPrep := sysPrepSpec.Sysprep; sysPrep != nil {
		identity = convertTo(sysPrep, bsArgs)
	} else {
		return nil, nil, fmt.Errorf("no Sysprep data")
	}

	nicSettingMap, err := network.GuestOSCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GSOC adapter mappings: %w", err)
	}

	customSpec := &vimtypes.CustomizationSpec{
		Identity: identity,
		GlobalIPSettings: vimtypes.CustomizationGlobalIPSettings{
			DnsSuffixList: bsArgs.SearchSuffixes,
			DnsServerList: bsArgs.DNSServers,
		},
		NicSettingMap: nicSettingMap,
	}

	var configSpec *vimtypes.VirtualMachineConfigSpec
	if vAppConfigSpec != nil {
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
		configSpec.VAppConfig = GetOVFVAppConfigForConfigSpec(
			config,
			vAppConfigSpec,
			bsArgs.BootstrapData.VAppData,
			bsArgs.BootstrapData.VAppExData,
			bsArgs.TemplateRenderFn)
	}

	return configSpec, customSpec, nil
}

func convertTo(from *vmopv1sysprep.Sysprep, bsArgs *BootstrapArgs) *vimtypes.CustomizationSysprep {
	bootstrapData := bsArgs.BootstrapData
	sysprepCustomization := &vimtypes.CustomizationSysprep{}

	sysprepCustomization.GuiUnattended = vimtypes.CustomizationGuiUnattended{}

	if from.GUIUnattended == nil {
		// If spec.bootstrap.sysprep.guiUnattended is not set, then default
		// the timezone to UTC, which is 85 per the Microsoft documentation at
		// https://learn.microsoft.com/en-us/previous-versions/windows/embedded/ms912391(v=winembedded.11).
		sysprepCustomization.GuiUnattended.TimeZone = 85
	} else {
		sysprepCustomization.GuiUnattended.TimeZone = from.GUIUnattended.TimeZone
		sysprepCustomization.GuiUnattended.AutoLogon = from.GUIUnattended.AutoLogon
		sysprepCustomization.GuiUnattended.AutoLogonCount = from.GUIUnattended.AutoLogonCount

		if bootstrapData.Sysprep != nil && bootstrapData.Sysprep.Password != "" {
			sysprepCustomization.GuiUnattended.Password = &vimtypes.CustomizationPassword{
				Value:     bootstrapData.Sysprep.Password,
				PlainText: true,
			}
		}
	}

	sysprepCustomization.UserData = vimtypes.CustomizationUserData{
		// This is a mandatory field
		ComputerName: &vimtypes.CustomizationFixedName{
			Name: bsArgs.HostName,
		},
	}
	sysprepCustomization.UserData.FullName = from.UserData.FullName
	sysprepCustomization.UserData.OrgName = from.UserData.OrgName
	// In the case of a VMI with volume license key, this might not be set.
	// Hence, add a check to see if the productID is set to empty.
	if bootstrapData.Sysprep != nil && bootstrapData.Sysprep.ProductID != "" {
		sysprepCustomization.UserData.ProductId = bootstrapData.Sysprep.ProductID
	}

	if from.GUIRunOnce != nil {
		sysprepCustomization.GuiRunOnce = &vimtypes.CustomizationGuiRunOnce{
			CommandList: from.GUIRunOnce.Commands,
		}
	}

	if from.Identification != nil {
		sysprepCustomization.Identification = vimtypes.CustomizationIdentification{
			JoinWorkgroup: from.Identification.JoinWorkgroup,
			JoinDomain:    bsArgs.DomainName,
			DomainAdmin:   from.Identification.DomainAdmin,
		}
		if bootstrapData.Sysprep != nil && bootstrapData.Sysprep.DomainPassword != "" {
			sysprepCustomization.Identification.DomainAdminPassword = &vimtypes.CustomizationPassword{
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

func convertLicenseFilePrintDataTo(from *vmopv1sysprep.LicenseFilePrintData) *vimtypes.CustomizationLicenseFilePrintData {
	custLicenseFilePrintData := &vimtypes.CustomizationLicenseFilePrintData{
		AutoMode: parseLicenseDataMode(from.AutoMode),
	}
	if from.AutoUsers != nil {
		custLicenseFilePrintData.AutoUsers = *from.AutoUsers
	}
	return custLicenseFilePrintData
}

func parseLicenseDataMode(mode vmopv1sysprep.CustomizationLicenseDataMode) vimtypes.CustomizationLicenseDataMode {
	switch mode {
	case vmopv1sysprep.CustomizationLicenseDataModePerServer:
		return vimtypes.CustomizationLicenseDataModePerServer
	case vmopv1sysprep.CustomizationLicenseDataModePerSeat:
		return vimtypes.CustomizationLicenseDataModePerSeat
	default:
		return ""
	}
}
