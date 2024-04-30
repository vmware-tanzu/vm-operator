// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

const doesNotExist = "does-not-exist"

func managedObjectTests() {
	getVMMoRef := func() vimtypes.ManagedObjectReference {
		return vimtypes.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: "vm-44",
		}
	}

	Context("ManagedObjectFromMoRef", func() {
		It("should return a mo.VirtualMachine from the provided ManagedObjectReference", func() {
			Expect(vmutil.ManagedObjectFromMoRef(getVMMoRef()).Self).To(Equal(getVMMoRef()))
		})
	})
	Context("ManagedObjectFromObject", func() {
		It("should return a mo.VirtualMachine from the provided object.VirtualMachine", func() {
			Expect(vmutil.ManagedObjectFromObject(object.NewVirtualMachine(nil, getVMMoRef())).Reference()).To(Equal(getVMMoRef()))
		})
	})
	Context("IsPausedByAdmin", func() {
		var (
			mgdObj mo.VirtualMachine
		)

		BeforeEach(func() {
			moRef := getVMMoRef()
			mgdObj = vmutil.ManagedObjectFromMoRef(moRef)
			mgdObj.Config = &vimtypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimtypes.BaseOptionValue{},
			}
		})

		It("should return false when PauseVMExtraConfigKey is not set", func() {
			Expect(vmutil.IsPausedByAdmin(&mgdObj)).To(BeFalse())
		})

		It("should return false when PauseVMExtraConfigKey is set to False", func() {
			mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
				Key:   vmopv1.PauseVMExtraConfigKey,
				Value: constants.ExtraConfigFalse,
			})

			Expect(vmutil.IsPausedByAdmin(&mgdObj)).To(BeFalse())
		})

		It("should return false when PauseVMExtraConfigKey is set to 1", func() {
			mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
				Key:   vmopv1.PauseVMExtraConfigKey,
				Value: 1,
			})

			Expect(vmutil.IsPausedByAdmin(&mgdObj)).To(BeFalse())
		})

		It("should return true when PauseVMExtraConfigKey is set to 'true'", func() {
			mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
				Key:   vmopv1.PauseVMExtraConfigKey,
				Value: "true",
			})

			Expect(vmutil.IsPausedByAdmin(&mgdObj)).To(BeTrue())
		})

		It("should return true when PauseVMExtraConfigKey is set to 'True'", func() {
			mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
				Key:   vmopv1.PauseVMExtraConfigKey,
				Value: "True",
			})

			Expect(vmutil.IsPausedByAdmin(&mgdObj)).To(BeTrue())
		})

		It("should return true when PauseVMExtraConfigKey is set to 'TRUE'", func() {
			mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
				Key:   vmopv1.PauseVMExtraConfigKey,
				Value: constants.ExtraConfigTrue,
			})

			Expect(vmutil.IsPausedByAdmin(&mgdObj)).To(BeTrue())
		})
	})
}
