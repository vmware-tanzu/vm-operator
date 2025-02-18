// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("LinuxPrep Bootstrap", func() {
	const (
		macAddr = "43:AB:B4:1B:7E:87"
	)

	var (
		bsArgs     vmlifecycle.BootstrapArgs
		configInfo *vimtypes.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		configInfo = &vimtypes.VirtualMachineConfigInfo{}
		bsArgs.Data = map[string]string{}
	})

	AfterEach(func() {
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	Context("BootStrapLinuxPrep", func() {

		var (
			configSpec *vimtypes.VirtualMachineConfigSpec
			custSpec   *vimtypes.CustomizationSpec
			err        error

			vmCtx          pkgctx.VirtualMachineContext
			vm             *vmopv1.VirtualMachine
			linuxPrepSpec  *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
			vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec
		)

		BeforeEach(func() {
			linuxPrepSpec = &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{}
			vAppConfigSpec = nil

			bsArgs.HostName = "my-hostname"
			bsArgs.SearchSuffixes = []string{"suffix1", "suffix2"}
			bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
				{
					MacAddress: macAddr,
					IPConfigs: []network.NetworkInterfaceIPConfig{
						{
							Gateway: "192.168.1.1",
							IPCIDR:  "192.168.1.10/24",
							IsIPv4:  true,
						},
					},
				},
			}

			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "linux-prep-bootstrap-test",
					Namespace: "test-ns",
				},
			}

			vmCtx = pkgctx.VirtualMachineContext{
				Context: context.Background(),
				Logger:  suite.GetLogger(),
				VM:      vm,
			}
		})

		JustBeforeEach(func() {
			configSpec, custSpec, err = vmlifecycle.BootStrapLinuxPrep(
				vmCtx,
				configInfo,
				linuxPrepSpec,
				vAppConfigSpec,
				&bsArgs,
			)
		})

		It("should return expected customization spec", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(configSpec).To(BeNil())

			Expect(custSpec).ToNot(BeNil())
			Expect(custSpec.GlobalIPSettings.DnsServerList).To(Equal(bsArgs.DNSServers))
			Expect(custSpec.GlobalIPSettings.DnsSuffixList).To(Equal(bsArgs.SearchSuffixes))

			linuxSpec := custSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
			hostName := linuxSpec.HostName.(*vimtypes.CustomizationFixedName).Name
			Expect(hostName).To(Equal(bsArgs.HostName))
			Expect(linuxSpec.TimeZone).To(Equal(linuxPrepSpec.TimeZone))
			Expect(linuxSpec.HwClockUTC).To(Equal(linuxPrepSpec.HardwareClockIsUTC))

			Expect(custSpec.NicSettingMap).To(HaveLen(len(bsArgs.NetworkResults.Results)))
			Expect(custSpec.NicSettingMap[0].MacAddress).To(Equal(macAddr))
		})

		Context("when has vAppConfig", func() {
			const key, value = "fooKey", "fooValue"

			BeforeEach(func() {
				configInfo.VAppConfig = &vimtypes.VmConfigInfo{
					Property: []vimtypes.VAppPropertyInfo{
						{
							Id:               key,
							Value:            "should-change",
							UserConfigurable: ptr.To(true),
						},
					},
				}

				vAppConfigSpec = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
					Properties: []common.KeyValueOrSecretKeySelectorPair{
						{
							Key:   key,
							Value: common.ValueOrSecretKeySelector{Value: ptr.To(value)},
						},
					},
				}
			})

			It("should return expected customization spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(custSpec).ToNot(BeNil())

				Expect(configSpec).ToNot(BeNil())
				Expect(configSpec.VAppConfig).ToNot(BeNil())
				vmCs := configSpec.VAppConfig.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Id).To(Equal(key))
				Expect(vmCs.Property[0].Info.Value).To(Equal(value))
			})
		})
	})
})
