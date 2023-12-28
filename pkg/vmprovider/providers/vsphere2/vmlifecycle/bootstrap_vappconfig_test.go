// Copyright (c) 2021-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/pointer"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vmlifecycle"
)

var _ = Describe("VAppConfig Bootstrap", func() {
	const key, value = "fooKey", "fooValue"

	var (
		configInfo       *types.VirtualMachineConfigInfo
		vAppConfigSpec   *vmopv1.VirtualMachineBootstrapVAppConfigSpec
		bsArgs           vmlifecycle.BootstrapArgs
		baseVMConfigSpec types.BaseVmConfigSpec
	)

	BeforeEach(func() {
		configInfo = &types.VirtualMachineConfigInfo{}
		configInfo.VAppConfig = &types.VmConfigInfo{
			Property: []types.VAppPropertyInfo{
				{
					Id:               key,
					Value:            "should-change",
					UserConfigurable: pointer.Bool(true),
				},
			},
		}

		vAppConfigSpec = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{}
		bsArgs.VAppData = make(map[string]string)
		bsArgs.VAppExData = make(map[string]map[string]string)
	})

	AfterEach(func() {
		vAppConfigSpec = nil
		baseVMConfigSpec = nil
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	Context("GetOVFVAppConfigForConfigSpec", func() {

		JustBeforeEach(func() {
			baseVMConfigSpec = vmlifecycle.GetOVFVAppConfigForConfigSpec(
				configInfo,
				vAppConfigSpec,
				bsArgs.VAppData,
				bsArgs.VAppExData,
				bsArgs.TemplateRenderFn)
		})

		Context("Empty input", func() {
			It("No changes", func() {
				Expect(baseVMConfigSpec).To(BeNil())
			})
		})

		Context("vAppData Map", func() {
			BeforeEach(func() {
				bsArgs.VAppData[key] = value
			})

			It("Expected VAppConfig", func() {
				Expect(baseVMConfigSpec).ToNot(BeNil())

				vmCs := baseVMConfigSpec.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Id).To(Equal(key))
				Expect(vmCs.Property[0].Info.Value).To(Equal(value))
			})

			Context("Applies TemplateRenderFn when specified", func() {
				BeforeEach(func() {
					bsArgs.TemplateRenderFn = func(_, v string) string {
						return strings.ToUpper(v)
					}
				})

				It("Expected VAppConfig", func() {
					Expect(baseVMConfigSpec).ToNot(BeNil())

					vmCs := baseVMConfigSpec.GetVmConfigSpec()
					Expect(vmCs).ToNot(BeNil())
					Expect(vmCs.Property).To(HaveLen(1))
					Expect(vmCs.Property[0].Info).ToNot(BeNil())
					Expect(vmCs.Property[0].Info.Id).To(Equal(key))
					Expect(vmCs.Property[0].Info.Value).To(Equal(strings.ToUpper(value)))
				})
			})
		})

		Context("vAppDataConfig Inlined Properties", func() {
			BeforeEach(func() {
				vAppConfigSpec = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
					Properties: []common.KeyValueOrSecretKeySelectorPair{
						{
							Key:   key,
							Value: common.ValueOrSecretKeySelector{Value: pointer.String(value)},
						},
					},
				}
			})

			It("Expected VAppConfig", func() {
				Expect(baseVMConfigSpec).ToNot(BeNil())

				vmCs := baseVMConfigSpec.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Id).To(Equal(key))
				Expect(vmCs.Property[0].Info.Value).To(Equal(value))
			})

			Context("Applies TemplateRenderFn when specified", func() {
				BeforeEach(func() {
					bsArgs.TemplateRenderFn = func(_, v string) string {
						return strings.ToUpper(v)
					}
				})

				It("Expected VAppConfig", func() {
					Expect(baseVMConfigSpec).ToNot(BeNil())

					vmCs := baseVMConfigSpec.GetVmConfigSpec()
					Expect(vmCs).ToNot(BeNil())
					Expect(vmCs.Property).To(HaveLen(1))
					Expect(vmCs.Property[0].Info).ToNot(BeNil())
					Expect(vmCs.Property[0].Info.Id).To(Equal(key))
					Expect(vmCs.Property[0].Info.Value).To(Equal(strings.ToUpper(value)))
				})
			})
		})

		Context("vAppDataConfig From Properties", func() {
			const secretName = "my-other-secret"

			BeforeEach(func() {
				vAppConfigSpec = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
					Properties: []common.KeyValueOrSecretKeySelectorPair{
						{
							Key: key,
							Value: common.ValueOrSecretKeySelector{
								From: &common.SecretKeySelector{
									Name: secretName,
									Key:  key,
								},
							},
						},
					},
				}

				bsArgs.VAppExData[secretName] = map[string]string{key: value}
			})

			It("Expected VAppConfig", func() {
				Expect(baseVMConfigSpec).ToNot(BeNil())

				vmCs := baseVMConfigSpec.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Id).To(Equal(key))
				Expect(vmCs.Property[0].Info.Value).To(Equal(value))
			})

			Context("Applies TemplateRenderFn when specified", func() {
				BeforeEach(func() {
					bsArgs.TemplateRenderFn = func(_, v string) string {
						return strings.ToUpper(v)
					}
				})

				It("Expected VAppConfig", func() {
					Expect(baseVMConfigSpec).ToNot(BeNil())

					vmCs := baseVMConfigSpec.GetVmConfigSpec()
					Expect(vmCs).ToNot(BeNil())
					Expect(vmCs.Property).To(HaveLen(1))
					Expect(vmCs.Property[0].Info).ToNot(BeNil())
					Expect(vmCs.Property[0].Info.Id).To(Equal(key))
					Expect(vmCs.Property[0].Info.Value).To(Equal(strings.ToUpper(value)))
				})
			})
		})
	})
})

var _ = Describe("GetMergedvAppConfigSpec", func() {

	DescribeTable("returns expected props",
		func(inProps map[string]string, vmProps []types.VAppPropertyInfo, expected *types.VmConfigSpec) {
			vAppConfigSpec := vmlifecycle.GetMergedvAppConfigSpec(inProps, vmProps)
			if expected == nil {
				Expect(vAppConfigSpec).To(BeNil())
			} else {
				Expect(vAppConfigSpec.Property).To(HaveLen(len(expected.Property)))
				for i := range vAppConfigSpec.Property {
					Expect(vAppConfigSpec.Property[i].Info.Key).To(Equal(expected.Property[i].Info.Key))
					Expect(vAppConfigSpec.Property[i].Info.Id).To(Equal(expected.Property[i].Info.Id))
					Expect(vAppConfigSpec.Property[i].Info.Value).To(Equal(expected.Property[i].Info.Value))
					Expect(vAppConfigSpec.Property[i].ArrayUpdateSpec.Operation).To(Equal(types.ArrayUpdateOperationEdit))
				}
				Expect(vAppConfigSpec.OvfEnvironmentTransport).To(HaveLen(1))
				Expect(vAppConfigSpec.OvfEnvironmentTransport[0]).To(Equal(vmlifecycle.OvfEnvironmentTransportGuestInfo))
			}
		},
		Entry("return nil for absent vm and input props",
			map[string]string{},
			[]types.VAppPropertyInfo{},
			nil,
		),
		Entry("return nil for non UserConfigurable vm props",
			map[string]string{
				"one-id": "one-override-value",
				"two-id": "two-override-value",
			},
			[]types.VAppPropertyInfo{
				{Key: 1, Id: "one-id", Value: "one-value"},
				{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: pointer.Bool(false)},
			},
			nil,
		),
		Entry("return nil for UserConfigurable vm props but no input props",
			map[string]string{},
			[]types.VAppPropertyInfo{
				{Key: 1, Id: "one-id", Value: "one-value"},
				{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: pointer.Bool(true)},
			},
			nil,
		),
		Entry("return valid vAppConfigSpec for setting mixed UserConfigurable props",
			map[string]string{
				"one-id":   "one-override-value",
				"two-id":   "two-override-value",
				"three-id": "three-override-value",
			},
			[]types.VAppPropertyInfo{
				{Key: 1, Id: "one-id", Value: "one-value", UserConfigurable: nil},
				{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: pointer.Bool(true)},
				{Key: 3, Id: "three-id", Value: "three-value", UserConfigurable: pointer.Bool(false)},
			},
			&types.VmConfigSpec{
				Property: []types.VAppPropertySpec{
					{Info: &types.VAppPropertyInfo{Key: 2, Id: "two-id", Value: "two-override-value"}},
				},
			},
		),
	)
})
