// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("VAppConfig Bootstrap", func() {
	const key, value = "fooKey", "fooValue"

	var (
		err              error
		configInfo       *vimtypes.VirtualMachineConfigInfo
		vAppConfigSpec   *vmopv1.VirtualMachineBootstrapVAppConfigSpec
		bsArgs           vmlifecycle.BootstrapArgs
		baseVMConfigSpec vimtypes.BaseVmConfigSpec
	)

	BeforeEach(func() {
		configInfo = &vimtypes.VirtualMachineConfigInfo{}
		configInfo.VAppConfig = &vimtypes.VmConfigInfo{
			Property: []vimtypes.VAppPropertyInfo{
				{
					Id:               key,
					Value:            "should-change",
					UserConfigurable: ptr.To(true),
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
			baseVMConfigSpec, err = vmlifecycle.GetOVFVAppConfigForConfigSpec(
				configInfo,
				vAppConfigSpec,
				bsArgs.VAppData,
				bsArgs.VAppExData,
				bsArgs.TemplateRenderFn)
		})

		When("config.vAppConfig is nil", func() {
			BeforeEach(func() {
				configInfo.VAppConfig = nil
			})
			It("Should return an error", func() {
				Expect(err).To(MatchError("vAppConfig is not yet available"))
				Expect(baseVMConfigSpec).To(BeNil())
			})
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
							Value: common.ValueOrSecretKeySelector{Value: ptr.To(value)},
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
		func(inProps map[string]string, vmProps []vimtypes.VAppPropertyInfo, expected *vimtypes.VmConfigSpec, expectedErr error) {
			baseVAppConfigSpec, err := vmlifecycle.GetMergedvAppConfigSpec(inProps, vmProps)
			if expectedErr != nil {
				Expect(err).To(MatchError(expectedErr))
				Expect(baseVAppConfigSpec).To(BeNil())
			} else if expected == nil {
				Expect(err).To(BeNil())
				Expect(baseVAppConfigSpec).To(BeNil())
			} else {
				Expect(err).To(BeNil())
				Expect(baseVAppConfigSpec).ToNot(BeNil())
				vAppConfigSpec := baseVAppConfigSpec.GetVmConfigSpec()
				Expect(vAppConfigSpec.Property).To(HaveLen(len(expected.Property)))
				for i := range vAppConfigSpec.Property {
					Expect(vAppConfigSpec.Property[i].Info.Key).To(Equal(expected.Property[i].Info.Key))
					Expect(vAppConfigSpec.Property[i].Info.Id).To(Equal(expected.Property[i].Info.Id))
					Expect(vAppConfigSpec.Property[i].Info.Value).To(Equal(expected.Property[i].Info.Value))
					Expect(vAppConfigSpec.Property[i].ArrayUpdateSpec.Operation).To(Equal(vimtypes.ArrayUpdateOperationEdit))
				}
				Expect(vAppConfigSpec.OvfEnvironmentTransport).To(HaveLen(1))
				Expect(vAppConfigSpec.OvfEnvironmentTransport[0]).To(Equal(vmlifecycle.OvfEnvironmentTransportGuestInfo))
			}
		},
		Entry("return nil for absent vm and input props",
			map[string]string{},
			[]vimtypes.VAppPropertyInfo{},
			nil,
			nil,
		),
		Entry("return nil for non UserConfigurable vm props",
			map[string]string{
				"one-id": "one-override-value",
				"two-id": "two-override-value",
			},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "one-id", Value: "one-value"},
				{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: ptr.To(false)},
			},
			nil,
			nil,
		),
		Entry("return nil for UserConfigurable vm props but no input props",
			map[string]string{},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "one-id", Value: "one-value"},
				{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: ptr.To(true)},
			},
			nil,
			nil,
		),
		Entry("return valid vAppConfigSpec for setting mixed UserConfigurable props",
			map[string]string{
				"one-id":   "one-override-value",
				"two-id":   "two-override-value",
				"three-id": "three-override-value",
			},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "one-id", Value: "one-value", UserConfigurable: nil},
				{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: ptr.To(true)},
				{Key: 3, Id: "three-id", Value: "three-value", UserConfigurable: ptr.To(false)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 2, Id: "two-id", Value: "two-override-value"}},
				},
			},
			nil,
		),
		// Basic type tests
		Entry("string type with valid value",
			map[string]string{"string-prop": "test-value"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "string-prop", Value: "old-value", Type: "string", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "string-prop", Value: "test-value", Type: "string"}},
				},
			},
			nil,
		),
		Entry("password type with valid value",
			map[string]string{"password-prop": "secret-password"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "password-prop", Value: "old-password", Type: "password", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "password-prop", Value: "secret-password", Type: "password"}},
				},
			},
			nil,
		),
		Entry("boolean type with true value",
			map[string]string{"bool-prop": "true"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "bool-prop", Value: "False", Type: "boolean", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "bool-prop", Value: "True", Type: "boolean"}},
				},
			},
			nil,
		),
		Entry("boolean type with false value",
			map[string]string{"bool-prop": "false"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "bool-prop", Value: "True", Type: "boolean", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "bool-prop", Value: "False", Type: "boolean"}},
				},
			},
			nil,
		),
		Entry("int type with valid value",
			map[string]string{"int-prop": "42"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-prop", Value: "0", Type: "int", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-prop", Value: "42", Type: "int"}},
				},
			},
			nil,
		),
		Entry("int type with negative value",
			map[string]string{"int-prop": "-42"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-prop", Value: "0", Type: "int", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-prop", Value: "-42", Type: "int"}},
				},
			},
			nil,
		),
		Entry("real type with valid value",
			map[string]string{"real-prop": "3.14159"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-prop", Value: "0.0", Type: "real", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "real-prop", Value: "3.14159", Type: "real"}},
				},
			},
			nil,
		),
		Entry("ip type with valid IPv4",
			map[string]string{"ip-prop": "192.168.1.1"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "ip-prop", Value: "0.0.0.0", Type: "ip", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "ip-prop", Value: "192.168.1.1", Type: "ip"}},
				},
			},
			nil,
		),
		Entry("ip type with invalid IPv4",
			map[string]string{"ip-prop": "192.168.1.1.1"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "ip-prop", Value: "0.0.0.0", Type: "ip", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "ip-prop", "ip", "192.168.1.1.1"),
		),
		Entry("ip type with valid IPv6",
			map[string]string{"ip-prop": "2001:db8::1"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "ip-prop", Value: "::1", Type: "ip", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "ip-prop", Value: "2001:db8::1", Type: "ip"}},
				},
			},
			nil,
		),
		Entry("ip type with invalid IPv6",
			map[string]string{"ip-prop": "2001:db8::1:::::::::"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "ip-prop", Value: "::1", Type: "ip", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "ip-prop", "ip", "2001:db8::1:::::::::"),
		),
		Entry("ip:network type with valid value",
			map[string]string{"ip-network-prop": "192.168.1.0/24"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "ip-network-prop", Value: "0.0.0.0/0", Type: "ip:network", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "ip-network-prop", Value: "192.168.1.0/24", Type: "ip:network"}},
				},
			},
			nil,
		),
		Entry("expression type with valid value",
			map[string]string{"expr-prop": "some-expression"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "expr-prop", Value: "old-expr", Type: "expression", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "expr-prop", Value: "some-expression", Type: "expression"}},
				},
			},
			nil,
		),
		// Error tests for basic types
		Entry("string type with value too long",
			map[string]string{"string-prop": string(make([]byte, 65536))}, // 65536 bytes, exceeds maxVAppPropStringLen
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "string-prop", Value: "old-value", Type: "string", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to length: len=%d, min=%d, max=%d", "string-prop", "string", 65536, 0, 65535),
		),
		Entry("int type with invalid value",
			map[string]string{"int-prop": "not-a-number"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-prop", Value: "0", Type: "int", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "int-prop", "int", "not-a-number"),
		),
		Entry("int type with value too large for int32",
			map[string]string{"int-prop": "2147483648"}, // Max int32 + 1
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-prop", Value: "0", Type: "int", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "int-prop", "int", "2147483648"),
		),
		Entry("int type with value too small for int32",
			map[string]string{"int-prop": "-2147483649"}, // Min int32 - 1
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-prop", Value: "0", Type: "int", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "int-prop", "int", "-2147483649"),
		),
		Entry("real type with invalid value",
			map[string]string{"real-prop": "not-a-float"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-prop", Value: "0.0", Type: "real", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "real-prop", "real", "not-a-float"),
		),
		Entry("ip type with invalid value",
			map[string]string{"ip-prop": "192.168.1.1/33"}, // Invalid CIDR that causes ParseIP to return error
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "ip-prop", Value: "0.0.0.0", Type: "ip", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "ip-prop", "ip", "192.168.1.1/33"),
		),
		// Regex-based type tests
		Entry("string with minimum length - valid",
			map[string]string{"str-min-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-min-prop", Value: "a", Type: "string(..3)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "str-min-prop", Value: "abc", Type: "string(..3)"}},
				},
			},
			nil,
		),
		Entry("string with minimum length - invalid",
			map[string]string{"str-min-prop": "ab"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-min-prop", Value: "a", Type: "string(..3)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to length: len=%d, min=%d, max=%d", "str-min-prop", "string(..3)", 2, 3, 65535),
		),
		Entry("string with maximum length - valid",
			map[string]string{"str-max-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-max-prop", Value: "a", Type: "string(3..)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "str-max-prop", Value: "abc", Type: "string(3..)"}},
				},
			},
			nil,
		),
		Entry("string with maximum length - invalid",
			map[string]string{"str-max-prop": "abcd"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-max-prop", Value: "a", Type: "string(3..)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to length: len=%d, min=%d, max=%d", "str-max-prop", "string(3..)", 4, 0, 3),
		),
		Entry("string with min/max length - valid",
			map[string]string{"str-range-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-range-prop", Value: "a", Type: "string(2..4)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "str-range-prop", Value: "abc", Type: "string(2..4)"}},
				},
			},
			nil,
		),
		Entry("string with min/max length - too short",
			map[string]string{"str-range-prop": "a"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-range-prop", Value: "ab", Type: "string(2..4)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to length: len=%d, min=%d, max=%d", "str-range-prop", "string(2..4)", 1, 2, 4),
		),
		Entry("string with min/max length - too long",
			map[string]string{"str-range-prop": "abcde"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-range-prop", Value: "ab", Type: "string(2..4)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to length: len=%d, min=%d, max=%d", "str-range-prop", "string(2..4)", 5, 2, 4),
		),
		Entry("string with invalid min/max range",
			map[string]string{"str-range-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-range-prop", Value: "ab", Type: "string(4..2)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to min=%d > max=%d", "str-range-prop", "string(4..2)", 4, 2),
		),
		Entry("int with range - valid positive",
			map[string]string{"int-range-prop": "100"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(0..255)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "100", Type: "int(0..255)"}},
				},
			},
			nil,
		),
		Entry("int with range - valid negative",
			map[string]string{"int-range-prop": "-50"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(-100..100)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "-50", Type: "int(-100..100)"}},
				},
			},
			nil,
		),
		Entry("int with range - out of range (too high)",
			map[string]string{"int-range-prop": "300"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(0..255)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to size: val=%d, min=%d, max=%d", "int-range-prop", "int(0..255)", 300, 0, 255),
		),
		Entry("int with range - out of range (too low)",
			map[string]string{"int-range-prop": "-10"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(0..255)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "int-range-prop", "int(0..255)", "-10"),
		),
		Entry("int with range - invalid range (min > max)",
			map[string]string{"int-range-prop": "100"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(255..0)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to min=%d > max=%d", "int-range-prop", "int(255..0)", 255, 0),
		),
		Entry("int with range - invalid value",
			map[string]string{"int-range-prop": "not-a-number"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(0..255)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "int-range-prop", "int(0..255)", "not-a-number"),
		),
		Entry("real with range - valid",
			map[string]string{"real-range-prop": "1.5"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(-2.0..2.0)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "real-range-prop", Value: "1.5", Type: "real(-2.0..2.0)"}},
				},
			},
			nil,
		),
		Entry("real with range - out of range (too high)",
			map[string]string{"real-range-prop": "3.0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(-2.0..2.0)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to size: val=%f, min=%f, max=%f", "real-range-prop", "real(-2.0..2.0)", 3.0, -2.0, 2.0),
		),
		Entry("real with range - out of range (too low)",
			map[string]string{"real-range-prop": "-3.0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(-2.0..2.0)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to size: val=%f, min=%f, max=%f", "real-range-prop", "real(-2.0..2.0)", -3.0, -2.0, 2.0),
		),
		Entry("real with range - invalid range (min > max)",
			map[string]string{"real-range-prop": "1.0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(2.0..-2.0)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to min=%f > max=%f", "real-range-prop", "real(2.0..-2.0)", 2.0, -2.0),
		),
		Entry("real with range - invalid value",
			map[string]string{"real-range-prop": "not-a-float"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(-2.0..2.0)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "real-range-prop", "real(-2.0..2.0)", "not-a-float"),
		),
		Entry("unknown type - should pass through",
			map[string]string{"unknown-prop": "some-value"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "unknown-prop", Value: "old-value", Type: "unknown-type", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "unknown-prop", Value: "some-value", Type: "unknown-type"}},
				},
			},
			nil,
		),
		// Edge cases
		Entry("nil UserConfigurable - should skip",
			map[string]string{"nil-prop": "some-value"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "nil-prop", Value: "old-value", Type: "string", UserConfigurable: nil},
			},
			nil,
			nil,
		),
		Entry("false UserConfigurable - should skip",
			map[string]string{"false-prop": "some-value"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "false-prop", Value: "old-value", Type: "string", UserConfigurable: ptr.To(false)},
			},
			nil,
			nil,
		),
		Entry("value not found in keyVals - should skip",
			map[string]string{},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "missing-prop", Value: "old-value", Type: "string", UserConfigurable: ptr.To(true)},
			},
			nil,
			nil,
		),
		Entry("value same as existing - should skip",
			map[string]string{"same-prop": "same-value"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "same-prop", Value: "same-value", Type: "string", UserConfigurable: ptr.To(true)},
			},
			nil,
			nil,
		),
		Entry("password with minimum length - valid",
			map[string]string{"pass-min-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "pass-min-prop", Value: "a", Type: "password(..3)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "pass-min-prop", Value: "abc", Type: "password(..3)"}},
				},
			},
			nil,
		),
		Entry("password with maximum length - valid",
			map[string]string{"pass-max-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "pass-max-prop", Value: "a", Type: "password(3..)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "pass-max-prop", Value: "abc", Type: "password(3..)"}},
				},
			},
			nil,
		),
		Entry("password with min/max length - valid",
			map[string]string{"pass-range-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "pass-range-prop", Value: "a", Type: "password(2..4)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "pass-range-prop", Value: "abc", Type: "password(2..4)"}},
				},
			},
			nil,
		),
		// Additional edge cases to reach 100% coverage
		Entry("int with range - zero value",
			map[string]string{"int-range-prop": "0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "1", Type: "int(0..10)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(0..10)"}},
				},
			},
			nil,
		),
		Entry("int with range - boundary value (min)",
			map[string]string{"int-range-prop": "0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "1", Type: "int(0..10)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(0..10)"}},
				},
			},
			nil,
		),
		Entry("int with range - boundary value (max)",
			map[string]string{"int-range-prop": "10"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "1", Type: "int(0..10)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "10", Type: "int(0..10)"}},
				},
			},
			nil,
		),
		Entry("real with range - boundary value (min)",
			map[string]string{"real-range-prop": "-2.0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(-2.0..2.0)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "real-range-prop", Value: "-2.0", Type: "real(-2.0..2.0)"}},
				},
			},
			nil,
		),
		Entry("real with range - boundary value (max)",
			map[string]string{"real-range-prop": "2.0"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "real-range-prop", Value: "0.0", Type: "real(-2.0..2.0)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "real-range-prop", Value: "2.0", Type: "real(-2.0..2.0)"}},
				},
			},
			nil,
		),
		Entry("string with exact max length",
			map[string]string{"str-max-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-max-prop", Value: "a", Type: "string(3..)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "str-max-prop", Value: "abc", Type: "string(3..)"}},
				},
			},
			nil,
		),
		Entry("string with exact min length",
			map[string]string{"str-min-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-min-prop", Value: "a", Type: "string(..3)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "str-min-prop", Value: "abc", Type: "string(..3)"}},
				},
			},
			nil,
		),
		Entry("string with exact min/max length",
			map[string]string{"str-exact-prop": "abc"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "str-exact-prop", Value: "a", Type: "string(3..3)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "str-exact-prop", Value: "abc", Type: "string(3..3)"}},
				},
			},
			nil,
		),
		// Additional edge cases for complete coverage
		Entry("int with range - negative boundary (min)",
			map[string]string{"int-range-prop": "-10"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(-10..10)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "-10", Type: "int(-10..10)"}},
				},
			},
			nil,
		),
		Entry("int with range - negative boundary (max)",
			map[string]string{"int-range-prop": "10"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(-10..10)", UserConfigurable: ptr.To(true)},
			},
			&vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{Info: &vimtypes.VAppPropertyInfo{Key: 1, Id: "int-range-prop", Value: "10", Type: "int(-10..10)"}},
				},
			},
			nil,
		),
		Entry("int with range - out of range (negative too low)",
			map[string]string{"int-range-prop": "-11"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(-10..10)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to size: val=%d, min=%d, max=%d", "int-range-prop", "int(-10..10)", -11, -10, 10),
		),
		Entry("int with range - out of range (negative too high)",
			map[string]string{"int-range-prop": "11"},
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(-10..10)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s due to size: val=%d, min=%d, max=%d", "int-range-prop", "int(-10..10)", 11, -10, 10),
		),
		Entry("int with range - negative range with signed int64 overflow",
			map[string]string{"int-range-prop": "9223372036854775808"}, // Max int64 + 1
			[]vimtypes.VAppPropertyInfo{
				{Key: 1, Id: "int-range-prop", Value: "0", Type: "int(-10..10)", UserConfigurable: ptr.To(true)},
			},
			nil,
			fmt.Errorf("failed to parse prop=%q, type=%s, value=%v", "int-range-prop", "int(-10..10)", "9223372036854775808"),
		),
	)
})
