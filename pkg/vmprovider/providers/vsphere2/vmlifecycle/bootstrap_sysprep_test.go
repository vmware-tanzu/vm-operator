// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vmlifecycle"
)

var _ = Describe("SysPrep Bootstrap", func() {
	const (
		macAddr = "43:AB:B4:1B:7E:87"
	)

	var (
		bsArgs     vmlifecycle.BootstrapArgs
		configInfo *types.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		configInfo = &types.VirtualMachineConfigInfo{}
		bsArgs.Data = map[string]string{}
	})

	AfterEach(func() {
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	Context("BootStrapSysPrep", func() {
		const unattendXML = "dummy-unattend-xml"

		var (
			configSpec *types.VirtualMachineConfigSpec
			custSpec   *types.CustomizationSpec
			err        error

			vmCtx          context.VirtualMachineContextA2
			vm             *vmopv1.VirtualMachine
			sysPrepSpec    *vmopv1.VirtualMachineBootstrapSysprepSpec
			vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec
		)

		BeforeEach(func() {
			sysPrepSpec = &vmopv1.VirtualMachineBootstrapSysprepSpec{}
			vAppConfigSpec = nil

			bsArgs.Data["unattend"] = unattendXML
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
					Name:      "sys-prep-bootstrap-test",
					Namespace: "test-ns",
				},
			}

			vmCtx = context.VirtualMachineContextA2{
				Context: goctx.Background(),
				Logger:  suite.GetLogger(),
				VM:      vm,
			}
		})

		JustBeforeEach(func() {
			configSpec, custSpec, err = vmlifecycle.BootstrapSysPrep(
				vmCtx,
				configInfo,
				sysPrepSpec,
				vAppConfigSpec,
				&bsArgs,
			)
		})

		Context("Inlined Sysprep", func() {
			BeforeEach(func() {
				sysPrepSpec.Sysprep = &sysprep.Sysprep{}
			})

			It("Returns TODO", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("TODO"))
			})
		})

		Context("RawSysPrep", func() {
			BeforeEach(func() {
				sysPrepSpec.RawSysprep = &corev1.SecretKeySelector{}
				sysPrepSpec.RawSysprep.Name = "sysprep-secret"
				sysPrepSpec.RawSysprep.Key = "unattend"
			})

			It("should return expected customization spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).To(BeNil())

				Expect(custSpec).ToNot(BeNil())
				Expect(custSpec.GlobalIPSettings.DnsServerList).To(Equal(bsArgs.DNSServers))
				Expect(custSpec.GlobalIPSettings.DnsSuffixList).To(Equal(bsArgs.SearchSuffixes))

				sysPrepText := custSpec.Identity.(*types.CustomizationSysprepText)
				Expect(sysPrepText.Value).To(Equal(unattendXML))

				Expect(custSpec.NicSettingMap).To(HaveLen(len(bsArgs.NetworkResults.Results)))
				Expect(custSpec.NicSettingMap[0].MacAddress).To(Equal(macAddr))
			})
		})

		Context("when has vAppConfig", func() {
			const key, value = "fooKey", "fooValue"

			BeforeEach(func() {
				configInfo.VAppConfig = &types.VmConfigInfo{
					Property: []types.VAppPropertyInfo{
						{
							Id:               key,
							Value:            "should-change",
							UserConfigurable: pointer.Bool(true),
						},
					},
				}

				vAppConfigSpec = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
					Properties: []common.KeyValueOrSecretKeySelectorPair{
						{
							Key:   key,
							Value: common.ValueOrSecretKeySelector{Value: pointer.String(value)},
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
