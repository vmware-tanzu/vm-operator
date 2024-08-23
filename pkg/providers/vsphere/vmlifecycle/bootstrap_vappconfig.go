// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func BootstrapVAppConfig(
	_ context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	configSpec := &vimtypes.VirtualMachineConfigSpec{}
	configSpec.VAppConfig = GetOVFVAppConfigForConfigSpec(
		config,
		vAppConfigSpec,
		bsArgs.BootstrapData.VAppData,
		bsArgs.BootstrapData.VAppExData,
		bsArgs.TemplateRenderFn)

	return configSpec, nil, nil
}

func GetOVFVAppConfigForConfigSpec(
	config *vimtypes.VirtualMachineConfigInfo,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	vAppData map[string]string,
	vAppExData map[string]map[string]string,
	templateRenderFn TemplateRenderFunc) vimtypes.BaseVmConfigSpec {

	if config.VAppConfig == nil {
		// BMV: Should we really silently return here and below?
		return nil
	}

	vAppConfigInfo := config.VAppConfig.GetVmConfigInfo()
	if vAppConfigInfo == nil {
		return nil
	}

	if len(vAppConfigSpec.Properties) > 0 {
		vAppData = map[string]string{}

		for _, p := range vAppConfigSpec.Properties {
			if p.Value.Value != nil {
				vAppData[p.Key] = *p.Value.Value
			} else if p.Value.From != nil {
				from := p.Value.From
				vAppData[p.Key] = vAppExData[from.Name][from.Key]
			}
		}
	}

	if templateRenderFn != nil {
		// If we have a templating func, apply it to whatever data we have, regardless of the source.
		for k, v := range vAppData {
			vAppData[k] = templateRenderFn(k, v)
		}
	}

	return GetMergedvAppConfigSpec(vAppData, vAppConfigInfo.Property)
}

// GetMergedvAppConfigSpec prepares a vApp VmConfigSpec which will set the provided key/value fields.
// Only fields marked userConfigurable and pre-existing on the VM (ie. originated from the OVF Image)
// will be set, and all others will be ignored.
func GetMergedvAppConfigSpec(inProps map[string]string, vmProps []vimtypes.VAppPropertyInfo) vimtypes.BaseVmConfigSpec {
	outProps := make([]vimtypes.VAppPropertySpec, 0)

	for _, vmProp := range vmProps {
		if vmProp.UserConfigurable == nil || !*vmProp.UserConfigurable {
			continue
		}

		inPropValue, found := inProps[vmProp.Id]
		if !found || vmProp.Value == inPropValue {
			continue
		}

		vmPropCopy := vmProp
		vmPropCopy.Value = inPropValue
		outProp := vimtypes.VAppPropertySpec{
			ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{
				Operation: vimtypes.ArrayUpdateOperationEdit,
			},
			Info: &vmPropCopy,
		}
		outProps = append(outProps, outProp)
	}

	if len(outProps) == 0 {
		return nil
	}

	return &vimtypes.VmConfigSpec{
		Property: outProps,
		// Ensure the transport is guestInfo in case the VM does not have
		// a CD-ROM device required to use the ISO transport.
		OvfEnvironmentTransport: []string{OvfEnvironmentTransportGuestInfo},
	}
}
