// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
)

var _ = Describe("UpdateConfigSpecExtensionCompatibilityConstraint", func() {

	newConstraint := func(
		constraintType vimtypes.VirtualMachineExtensionCompatibilityConstraintType,
		name string,
	) vimtypes.VirtualMachineExtensionCompatibilityConstraint {
		return vimtypes.VirtualMachineExtensionCompatibilityConstraint{
			ConstraintName: name,
			ConstraintType: string(constraintType),
			ConstraintKind: string(vimtypes.VirtualMachineExtensionCompatibilityConstraintKindINVARIANT),
		}
	}

	desiredConstraints := func(names ...string) []vimtypes.VirtualMachineExtensionCompatibilityConstraint {
		types := []vimtypes.VirtualMachineExtensionCompatibilityConstraintType{
			vimtypes.VirtualMachineExtensionCompatibilityConstraintTypeSERVICE,
			vimtypes.VirtualMachineExtensionCompatibilityConstraintTypeFOLDER,
			vimtypes.VirtualMachineExtensionCompatibilityConstraintTypePOOL,
			vimtypes.VirtualMachineExtensionCompatibilityConstraintTypeVM_STORAGE_POLICY,
			vimtypes.VirtualMachineExtensionCompatibilityConstraintTypeDISK_STORAGE_POLICY,
			vimtypes.VirtualMachineExtensionCompatibilityConstraintTypeDEVICE,
		}

		out := make([]vimtypes.VirtualMachineExtensionCompatibilityConstraint, 0, len(types))
		for i, t := range types {
			name := ""
			if i < len(names) {
				name = names[i]
			}
			out = append(out, newConstraint(t, name))
		}
		return out
	}

	var (
		config     *vimtypes.VirtualMachineConfigInfo
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		config = &vimtypes.VirtualMachineConfigInfo{}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		virtualmachine.UpdateConfigSpecExtensionCompatibilityConstraint(config, configSpec)
	})

	When("config has no extension compatibility constraint set", func() {
		BeforeEach(func() {
			config.ExtensionCompatibilityConstraint = nil
		})

		It("sets the full desired set on the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).ToNot(BeNil())
			Expect(configSpec.ExtensionCompatibilityConstraint.Constraint).To(HaveLen(6))
		})
	})

	When("config has an empty constraint set", func() {
		BeforeEach(func() {
			config.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{}
		})

		It("sets the full desired set on the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).ToNot(BeNil())
			Expect(configSpec.ExtensionCompatibilityConstraint.Constraint).To(HaveLen(6))
		})
	})

	When("config is missing one of the desired constraints", func() {
		BeforeEach(func() {
			all := desiredConstraints()
			config.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{
				Constraint: all[:5], // drop the last one (DEVICE)
			}
		})

		It("re-sends the full desired set", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).ToNot(BeNil())
			Expect(configSpec.ExtensionCompatibilityConstraint.Constraint).To(HaveLen(6))
		})
	})

	When("config has an extra, unexpected constraint", func() {
		BeforeEach(func() {
			all := desiredConstraints()
			all = append(all, newConstraint("SOME_STALE_TYPE", "stale"))
			config.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{
				Constraint: all,
			}
		})

		It("re-sends the full desired set, dropping the stale entry", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).ToNot(BeNil())
			Expect(configSpec.ExtensionCompatibilityConstraint.Constraint).To(HaveLen(6))
		})
	})

	When("config's constraints are in a different order than desired", func() {
		BeforeEach(func() {
			all := desiredConstraints()
			reordered := make([]vimtypes.VirtualMachineExtensionCompatibilityConstraint, len(all))
			copy(reordered, all)
			reordered[0], reordered[len(reordered)-1] = reordered[len(reordered)-1], reordered[0]

			config.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{
				Constraint: reordered,
			}
		})

		It("does not modify the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("config's constraints match but with different ConstraintName values", func() {
		BeforeEach(func() {
			config.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{
				Constraint: desiredConstraints("some", "other", "names", "than", "vm operator", "would use"),
			}
		})

		It("does not modify the ConfigSpec, since ConstraintName is not part of identity", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("config already has exactly the desired set", func() {
		BeforeEach(func() {
			config.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{
				Constraint: desiredConstraints(),
			}
		})

		It("does not modify the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})
})

var _ = Describe("ClearConfigSpecExtensionCompatibilityConstraint", func() {

	var (
		config     *vimtypes.VirtualMachineConfigInfo
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		config = &vimtypes.VirtualMachineConfigInfo{
			ManagedBy: &vimtypes.ManagedByInfo{
				ExtensionKey: vmopv1.ManagedByExtensionKey,
				Type:         vmopv1.ManagedByExtensionType,
			},
			ExtensionCompatibilityConstraint: &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{
				Constraint: []vimtypes.VirtualMachineExtensionCompatibilityConstraint{
					{ConstraintType: string(vimtypes.VirtualMachineExtensionCompatibilityConstraintTypeDEVICE)},
				},
			},
		}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		virtualmachine.ClearConfigSpecExtensionCompatibilityConstraint(config, configSpec)
	})

	When("config is nil", func() {
		BeforeEach(func() {
			config = nil
		})

		It("does not modify the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("config has no constraint set", func() {
		BeforeEach(func() {
			config.ExtensionCompatibilityConstraint = nil
		})

		It("does not modify the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("config has a constraint set but is not managed by VM Operator", func() {
		BeforeEach(func() {
			config.ManagedBy = nil
		})

		It("does not modify the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("config has a constraint set but is managed by a different extension", func() {
		BeforeEach(func() {
			config.ManagedBy = &vimtypes.ManagedByInfo{
				ExtensionKey: "some.other.extension",
				Type:         "someType",
			}
		})

		It("does not modify the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("config has a constraint set and is managed by VM Operator", func() {
		It("sets a non-nil, empty constraint set on the ConfigSpec", func() {
			Expect(configSpec.ExtensionCompatibilityConstraint).ToNot(BeNil())
			Expect(configSpec.ExtensionCompatibilityConstraint.Constraint).To(BeEmpty())
		})
	})
})
