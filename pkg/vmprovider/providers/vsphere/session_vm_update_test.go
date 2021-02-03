// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimTypes "github.com/vmware/govmomi/vim25/types"
)

var _ = Describe("Delta ConfigSpec", func() {

	trueVar := true
	falseVar := false

	// Just a sample for testing these things. Need to think more about whether this
	// is a good way or not.
	Context("ChangeBlockTracking", func() {
		var vm v1alpha1.VirtualMachine
		var config *vimTypes.VirtualMachineConfigInfo
		var configSpec *vimTypes.VirtualMachineConfigSpec

		BeforeEach(func() {
			vm = v1alpha1.VirtualMachine{
				Spec: v1alpha1.VirtualMachineSpec{
					AdvancedOptions: &v1alpha1.VirtualMachineAdvancedOptions{},
				},
			}
			config = &vimTypes.VirtualMachineConfigInfo{}
			configSpec = &vimTypes.VirtualMachineConfigSpec{}
		})

		BeforeEach(func() {
			vm.Spec.AdvancedOptions.ChangeBlockTracking = nil
			config.ChangeTrackingEnabled = nil
		})

		It("cbt and status cbt unset", func() {
			deltaConfigSpecChangeBlockTracking(config, configSpec, vm.Spec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to true", func() {
			config.ChangeTrackingEnabled = &trueVar
			vm.Spec.AdvancedOptions.ChangeBlockTracking = &falseVar

			deltaConfigSpecChangeBlockTracking(config, configSpec, vm.Spec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to false", func() {
			config.ChangeTrackingEnabled = &falseVar
			vm.Spec.AdvancedOptions.ChangeBlockTracking = &trueVar

			deltaConfigSpecChangeBlockTracking(config, configSpec, vm.Spec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
		})

		It("configSpec cbt matches", func() {
			config.ChangeTrackingEnabled = &trueVar
			vm.Spec.AdvancedOptions.ChangeBlockTracking = &trueVar

			deltaConfigSpecChangeBlockTracking(config, configSpec, vm.Spec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})
	})
})
