// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func BootStrapLinuxPrep(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	linuxPrepSpec *vmopv1.VirtualMachineBootstrapLinuxPrepSpec,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.V(4).Info("Reconciling LinuxPrep bootstrap state")

	if !vmCtx.IsOffToOn() {
		vmCtx.Logger.V(4).Info("Skipping LinuxPrep since VM is not powering on")
		return nil, nil, nil
	}

	nicSettingMap, err := network.GuestOSCustomization(bsArgs.NetworkResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GOSC NIC mappings: %w", err)
	}

	identity := &vimtypes.CustomizationLinuxPrep{
		HostName: &vimtypes.CustomizationFixedName{
			Name: bsArgs.HostName,
		},
		Domain:     bsArgs.DomainName,
		TimeZone:   linuxPrepSpec.TimeZone,
		HwClockUTC: linuxPrepSpec.HardwareClockIsUTC,
	}

	if pkgcfg.FromContext(vmCtx).Features.GuestCustomizationVCDParity {
		if linuxPrepSpec.ExpirePasswordAfterNextLogin {
			identity.ResetPassword = ptr.To(true)
		}

		if bsArgs.LinuxPrep != nil {
			if bsArgs.LinuxPrep.Password != "" {
				identity.Password = &vimtypes.CustomizationPassword{
					Value:     bsArgs.LinuxPrep.Password,
					PlainText: true,
				}
			}

			identity.ScriptText = bsArgs.LinuxPrep.ScriptText
		}

		if id := vmCtx.VM.Annotations[pkgconst.VCFAIDAnnotationKey]; id != "" {
			identity.ExtraConfig = pkgutil.OptionValues(identity.ExtraConfig).Merge(
				&vimtypes.OptionValue{
					Key:   GOSCVCFAHashID,
					Value: id,
				},
			)
		}
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
		configSpec.VAppConfig, err = GetOVFVAppConfigForConfigSpec(
			config,
			vAppConfigSpec,
			bsArgs.BootstrapData.VAppData,
			bsArgs.BootstrapData.VAppExData,
			bsArgs.TemplateRenderFn)
	}

	return configSpec, customSpec, err
}
