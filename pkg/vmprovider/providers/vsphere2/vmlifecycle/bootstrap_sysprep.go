// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	goctx "context"
	"fmt"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
)

func BootstrapSysPrep(
	ctx goctx.Context,
	config *vimTypes.VirtualMachineConfigInfo,
	sysPrepSpec *vmopv1.VirtualMachineBootstrapSysprepSpec,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimTypes.VirtualMachineConfigSpec, *vimTypes.CustomizationSpec, error) {

	var data string
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

	} else if sysPrepSpec.Sysprep != nil {
		return nil, nil, fmt.Errorf("TODO: inlined Sysprep")
	}

	if bsArgs.TemplateRenderFn != nil {
		data = bsArgs.TemplateRenderFn(key, data)
	}

	nicSettingMap, err := network.GuestOSCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GSOC adapter mappings: %w", err)
	}

	customSpec := &vimTypes.CustomizationSpec{
		Identity: &vimTypes.CustomizationSysprepText{
			Value: data,
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
