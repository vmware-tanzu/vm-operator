// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

var _ = Describe("Delta ConfigSpec", func() {

	var config *vimTypes.VirtualMachineConfigInfo
	var configSpec *vimTypes.VirtualMachineConfigSpec

	trueVar := true
	falseVar := false

	BeforeEach(func() {
		config = &vimTypes.VirtualMachineConfigInfo{}
		configSpec = &vimTypes.VirtualMachineConfigSpec{}
	})

	// Just a few examples for testing these things here. Need to think more about whether this
	// is a good way or not. Probably better to do this via UpdateVirtualMachine when we have
	// better integration tests.

	Context("ExtraConfig", func() {

		var vmImage *v1alpha1.VirtualMachineImage
		var vmSpec v1alpha1.VirtualMachineSpec
		var vmMetadata *vmprovider.VmMetadata
		var globalExtraConfig map[string]string
		var ecMap map[string]string

		BeforeEach(func() {
			vmImage = &v1alpha1.VirtualMachineImage{}
			vmSpec = v1alpha1.VirtualMachineSpec{}
			vmMetadata = &vmprovider.VmMetadata{
				Data:      make(map[string]string),
				Transport: v1alpha1.VirtualMachineMetadataExtraConfigTransport,
			}
			globalExtraConfig = make(map[string]string)
		})

		JustBeforeEach(func() {
			deltaConfigSpecExtraConfig(
				config,
				configSpec,
				vmImage,
				vmSpec,
				vmMetadata,
				globalExtraConfig)

			ecMap = make(map[string]string)
			for _, ec := range configSpec.ExtraConfig {
				if optionValue := ec.GetOptionValue(); optionValue != nil {
					ecMap[optionValue.Key] = optionValue.Value.(string)
				}
			}
		})

		Context("Empty input", func() {
			It("No changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		Context("Updates configSpec.ExtraConfig", func() {
			BeforeEach(func() {
				conditions.MarkTrue(vmImage, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
				config.ExtraConfig = append(config.ExtraConfig, &vimTypes.OptionValue{
					Key: VMOperatorV1Alpha1ExtraConfigKey, Value: VMOperatorV1Alpha1ConfigReady})
				vmMetadata.Data["guestinfo.test"] = "test"
				vmMetadata.Data["nvram"] = "this should ignored"
				globalExtraConfig["global"] = "test"
			})

			It("Expected configSpec.ExtraConfig", func() {
				By("VM Metadata", func() {
					Expect(ecMap).To(HaveKeyWithValue("guestinfo.test", "test"))
					Expect(ecMap).ToNot(HaveKey("nvram"))
				})

				By("VM Image compatible", func() {
					Expect(ecMap).To(HaveKeyWithValue("guestinfo.vmservice.defer-cloud-init", "enabled"))
				})

				By("Global map", func() {
					Expect(ecMap).To(HaveKeyWithValue("global", "test"))
				})
			})
		})

		Context("ExtraConfig value already exists", func() {
			BeforeEach(func() {
				config.ExtraConfig = append(config.ExtraConfig, &vimTypes.OptionValue{Key: "foo", Value: "bar"})
				vmMetadata.Data["foo"] = "bar"
			})

			It("No changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})
	})

	Context("ChangeBlockTracking", func() {
		var vmSpec v1alpha1.VirtualMachineSpec

		BeforeEach(func() {
			vmSpec = v1alpha1.VirtualMachineSpec{
				AdvancedOptions: &v1alpha1.VirtualMachineAdvancedOptions{},
			}
			config.ChangeTrackingEnabled = nil
		})

		It("cbt and status cbt unset", func() {
			deltaConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to true", func() {
			config.ChangeTrackingEnabled = &trueVar
			vmSpec.AdvancedOptions.ChangeBlockTracking = &falseVar

			deltaConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to false", func() {
			config.ChangeTrackingEnabled = &falseVar
			vmSpec.AdvancedOptions.ChangeBlockTracking = &trueVar

			deltaConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
		})

		It("configSpec cbt matches", func() {
			config.ChangeTrackingEnabled = &trueVar
			vmSpec.AdvancedOptions.ChangeBlockTracking = &trueVar

			deltaConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})
	})
})
