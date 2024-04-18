// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package concepts

import (
	"bytes"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/yaml"
)

var _ = Describe("VirtualMachineClass", func() {
	Context("ConfigSpec", func() {
		var (
			obj vimtypes.VirtualMachineConfigSpec
		)
		Context("JSON", func() {
			var (
				act string
			)
			JustBeforeEach(func() {
				var w bytes.Buffer
				enc := vimtypes.NewJSONEncoder(&w)
				ExpectWithOffset(1, enc.Encode(obj)).To(Succeed())
				act = w.String()
			})
			AfterEach(func() {
				if !CurrentSpecReport().Failed() {
					rawj := json.RawMessage(act)
					data, err := json.MarshalIndent(rawj, "", "  ")
					ExpectWithOffset(1, err).ToNot(HaveOccurred())
					fmt.Fprintf(GinkgoWriter, "JSON\n----\n%s\n\n", string(data))

					data, err = yaml.Marshal(rawj)
					ExpectWithOffset(1, err).ToNot(HaveOccurred())
					fmt.Fprintf(GinkgoWriter, "YAML\n----\n%s\n\n", string(data))
				}
			})

			Context("Basic", func() {
				BeforeEach(func() {
					obj = vimtypes.VirtualMachineConfigSpec{
						NumCPUs:  2,
						MemoryMB: 2048,
					}
				})
				It("should match the expected JSON", func() {
					Expect(act).To(MatchJSON(
						`{
							"_typeName": "VirtualMachineConfigSpec",
							"numCPUs": 2,
							"memoryMB": 2048
						}`,
					))
				})
			})

			Context("ExtraConfig", func() {
				BeforeEach(func() {
					obj = vimtypes.VirtualMachineConfigSpec{
						NumCPUs:  2,
						MemoryMB: 2048,
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "my-key-1",
								Value: "my-value-1",
							},
							&vimtypes.OptionValue{
								Key:   "my-key-2",
								Value: "my-value-2",
							},
						},
					}
				})
				It("should match the expected JSON", func() {
					Expect(act).To(MatchJSON(
						`{
							"_typeName": "VirtualMachineConfigSpec",
							"numCPUs": 2,
							"memoryMB": 2048,
							"extraConfig": [
								{
									"_typeName": "OptionValue",
									"key": "my-key-1",
									"value": {
										"_typeName": "string",
										"_value": "my-value-1"
									}
								},
								{
									"_typeName": "OptionValue",
									"key": "my-key-2",
									"value": {
										"_typeName": "string",
										"_value": "my-value-2"
									}
								}
							]
						}`,
					))
				})
			})

			Context("vGPU", func() {
				BeforeEach(func() {
					obj = vimtypes.VirtualMachineConfigSpec{
						DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
							&vimtypes.VirtualDeviceConfigSpec{
								Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
								Device: &vimtypes.VirtualPCIPassthrough{
									VirtualDevice: vimtypes.VirtualDevice{
										Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
											Vgpu: "my-vgpu-profile",
										},
									},
								},
							},
						},
					}
				})
				It("should match the expected JSON", func() {
					Expect(act).To(MatchJSON(
						`{
							"_typeName": "VirtualMachineConfigSpec",
							"deviceChange": [
								{
									"_typeName": "VirtualDeviceConfigSpec",
									"operation": "add",
									"device": {
										"_typeName": "VirtualPCIPassthrough",
										"key": 0,
										"backing": {
											"_typeName": "VirtualPCIPassthroughVmiopBackingInfo",
											"vgpu": "my-vgpu-profile"
										}
									}
								}
							]
						}`,
					))
				})
			})

			Context("Dynamic Direct Path I/O", func() {
				BeforeEach(func() {
					obj = vimtypes.VirtualMachineConfigSpec{
						DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
							&vimtypes.VirtualDeviceConfigSpec{
								Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
								Device: &vimtypes.VirtualPCIPassthrough{
									VirtualDevice: vimtypes.VirtualDevice{
										Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
											AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
												{
													DeviceId: -999,
													VendorId: -999,
												},
											},
										},
									},
								},
							},
						},
					}
				})
				It("should match the expected JSON", func() {
					Expect(act).To(MatchJSON(
						`{
							"_typeName": "VirtualMachineConfigSpec",
							"deviceChange": [
								{
									"_typeName": "VirtualDeviceConfigSpec",
									"operation": "add",
									"device": {
										"_typeName": "VirtualPCIPassthrough",
										"key": 0,
										"backing": {
											"_typeName": "VirtualPCIPassthroughDynamicBackingInfo",
											"deviceName": "",
											"allowedDevice": [
												{
													"_typeName": "VirtualPCIPassthroughAllowedDevice",
													"vendorId": -999,
													"deviceId": -999
												}
											]
										}
									}
								}
							]
						}`,
					))
				})
			})
		})
	})
})
