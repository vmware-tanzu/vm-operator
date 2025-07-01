// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package paused_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/paused"
)

var _ = Describe("ByAdmin", func() {
	var mgdObj mo.VirtualMachine

	BeforeEach(func() {
		mgdObj = mo.VirtualMachine{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: vimtypes.ManagedObjectReference{
						Type:  "VirtualMachine",
						Value: "vm-44",
					},
				},
			},
			Config: &vimtypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimtypes.BaseOptionValue{},
			},
		}
	})

	It("should return false when moVM.Config is nil", func() {
		mgdObj.Config = nil
		Expect(paused.ByAdmin(mgdObj)).To(BeFalse())
	})

	It("should return false when PauseVMExtraConfigKey is not set", func() {
		Expect(paused.ByAdmin(mgdObj)).To(BeFalse())
	})

	It("should return false when PauseVMExtraConfigKey is set to False", func() {
		mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
			Key:   vmopv1.PauseVMExtraConfigKey,
			Value: constants.ExtraConfigFalse,
		})

		Expect(paused.ByAdmin(mgdObj)).To(BeFalse())
	})

	It("should return true when PauseVMExtraConfigKey is set to 1", func() {
		mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
			Key:   vmopv1.PauseVMExtraConfigKey,
			Value: 1,
		})

		Expect(paused.ByAdmin(mgdObj)).To(BeTrue())
	})

	It("should return true when PauseVMExtraConfigKey is set to 'true'", func() {
		mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
			Key:   vmopv1.PauseVMExtraConfigKey,
			Value: "true",
		})

		Expect(paused.ByAdmin(mgdObj)).To(BeTrue())
	})

	It("should return true when PauseVMExtraConfigKey is set to 'True'", func() {
		mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
			Key:   vmopv1.PauseVMExtraConfigKey,
			Value: "True",
		})

		Expect(paused.ByAdmin(mgdObj)).To(BeTrue())
	})

	It("should return true when PauseVMExtraConfigKey is set to 'TRUE'", func() {
		mgdObj.Config.ExtraConfig = append(mgdObj.Config.ExtraConfig, &vimtypes.OptionValue{
			Key:   vmopv1.PauseVMExtraConfigKey,
			Value: constants.ExtraConfigTrue,
		})

		Expect(paused.ByAdmin(mgdObj)).To(BeTrue())
	})
})

var _ = Describe("ByDevOps", func() {
	When("paused", func() {
		It("should return true", func() {
			Expect(paused.ByDevOps(&vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						vmopv1.PauseAnnotation: "true",
					},
				},
			})).To(BeTrue())
		})
	})
	When("not paused", func() {
		It("should return false", func() {
			Expect(paused.ByDevOps(&vmopv1.VirtualMachine{})).To(BeFalse())
		})
	})
})
