// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	goctx "context"
	"fmt"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
)

func BootStrapLinuxPrep(
	ctx goctx.Context,
	config *vimTypes.VirtualMachineConfigInfo,
	linuxPrepSpec *vmopv1.VirtualMachineBootstrapLinuxPrepSpec,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimTypes.VirtualMachineConfigSpec, *vimTypes.CustomizationSpec, error) {

	nicSettingMap, err := network.GuestOSCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GOSC NIC mappings: %w", err)
	}

	customSpec := &vimTypes.CustomizationSpec{
		Identity: &vimTypes.CustomizationLinuxPrep{
			HostName: &vimTypes.CustomizationFixedName{
				Name: bsArgs.Hostname,
			},
			TimeZone:   linuxPrepSpec.TimeZone,
			HwClockUTC: vimTypes.NewBool(linuxPrepSpec.HardwareClockIsUTC),
		},
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
