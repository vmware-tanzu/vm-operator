// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configpolicysync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/configpolicysync"
)

var _ = Describe("Merge", Label(testlabels.API), func() {
	When("no targets are supplied", func() {
		It("returns spec unchanged", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{
				Zone:       "zone-1",
				CreateMode: vimv1.VirtualMachineConfigPolicyModeDeny,
			}

			Expect(configpolicysync.Merge(spec)).To(Equal(spec))
		})
	})

	When("a single target is supplied", func() {
		It("copies its capacity limits and security flags into spec, preserving non-derived fields", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{
				Zone:       "zone-1",
				CreateMode: vimv1.VirtualMachineConfigPolicyModeDeny,
				ExtraConfig: &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
					Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
						{Type: vimv1.MatchTypeFixed, Key: "some.key"},
					},
				},
			}

			target := vimv1.ConfigTargetStatus{
				NumCPUCores:            8,
				NumNumaNodes:           2,
				MaxSimultaneousThreads: 16,
				SupportedMaxMem:        quantityPtr("64Gi"),
				SMCPresent:             true,
				SEVSupported:           true,
			}

			merged := configpolicysync.Merge(spec, target)

			Expect(merged.Zone).To(Equal("zone-1"))
			Expect(merged.CreateMode).To(Equal(vimv1.VirtualMachineConfigPolicyModeDeny))
			Expect(merged.ExtraConfig).To(Equal(spec.ExtraConfig))

			Expect(merged.NumCPUCores).ToNot(BeNil())
			Expect(merged.NumCPUCores.Max).To(Equal(int32(8)))
			Expect(merged.NumNUMANodes.Max).To(Equal(int32(2)))
			Expect(merged.NumSimultaneousThreads.Max).To(Equal(int32(16)))
			Expect(merged.Memory.Max.Equal(resource.MustParse("64Gi"))).To(BeTrue())
			Expect(merged.SMCPresent).To(BeTrue())
			Expect(merged.SEVSupported).To(BeTrue())
			Expect(merged.SEVSNPSupported).To(BeFalse())
			Expect(merged.TDXSupported).To(BeFalse())
		})

		It("preserves an existing Min on a range field", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{
				NumCPUCores: &vimv1.IntRange{Min: 2, Max: 4},
			}

			merged := configpolicysync.Merge(spec, vimv1.ConfigTargetStatus{NumCPUCores: 8})

			Expect(merged.NumCPUCores.Min).To(Equal(int32(2)))
			Expect(merged.NumCPUCores.Max).To(Equal(int32(8)))
		})
	})

	When("multiple targets are supplied (multi-cluster zone)", func() {
		It("intersects numeric ranges to the minimum of the per-target maxima", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{}

			merged := configpolicysync.Merge(spec,
				vimv1.ConfigTargetStatus{NumCPUCores: 8, NumNumaNodes: 2, MaxSimultaneousThreads: 16},
				vimv1.ConfigTargetStatus{NumCPUCores: 4, NumNumaNodes: 4, MaxSimultaneousThreads: 32},
			)

			Expect(merged.NumCPUCores.Max).To(Equal(int32(4)))
			Expect(merged.NumNUMANodes.Max).To(Equal(int32(2)))
			Expect(merged.NumSimultaneousThreads.Max).To(Equal(int32(16)))
		})

		It("intersects memory to the minimum of the per-target maxima", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{}

			merged := configpolicysync.Merge(spec,
				vimv1.ConfigTargetStatus{SupportedMaxMem: quantityPtr("128Gi")},
				vimv1.ConfigTargetStatus{SupportedMaxMem: quantityPtr("64Gi")},
			)

			Expect(merged.Memory.Max.Equal(resource.MustParse("64Gi"))).To(BeTrue())
		})

		It("ANDs boolean support flags", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{}

			merged := configpolicysync.Merge(spec,
				vimv1.ConfigTargetStatus{SEVSupported: true, TDXSupported: true},
				vimv1.ConfigTargetStatus{SEVSupported: true, TDXSupported: false},
			)

			Expect(merged.SEVSupported).To(BeTrue())
			Expect(merged.TDXSupported).To(BeFalse())
		})

		It("intersects ConfigTargetDevices to entries common to every target", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{}

			shared := vimv1.VirtualMachineCdromInfo{
				VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: "cdrom-shared"},
			}
			onlyFirst := vimv1.VirtualMachineCdromInfo{
				VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: "cdrom-only-first"},
			}

			merged := configpolicysync.Merge(spec,
				vimv1.ConfigTargetStatus{
					ConfigTargetDevices: vimv1.ConfigTargetDevices{
						CDROM: []vimv1.VirtualMachineCdromInfo{shared, onlyFirst},
					},
				},
				vimv1.ConfigTargetStatus{
					ConfigTargetDevices: vimv1.ConfigTargetDevices{
						CDROM: []vimv1.VirtualMachineCdromInfo{shared},
					},
				},
			)

			Expect(merged.ConfigTargetDevices.CDROM).To(ConsistOf(shared))
		})

		It("intersects a zero value on one target literally, as real reported data", func() {
			// Merge's contract is that every target passed in has already
			// been established as Ready by the caller (see the
			// Reconciler's getConfigTargets) -- so a zero here is a real
			// "this cluster does not support it," not "not populated yet,"
			// and must narrow the intersection like any other value.
			spec := vimv1.VirtualMachineConfigPolicySpec{}

			merged := configpolicysync.Merge(spec,
				vimv1.ConfigTargetStatus{NumCPUCores: 8},
				vimv1.ConfigTargetStatus{NumCPUCores: 0},
			)

			Expect(merged.NumCPUCores.Max).To(Equal(int32(0)))
		})
	})

	When("syncMode=Disabled fields are untouched", func() {
		It("never sets fields that have no ConfigTarget source", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{
				LatencySensitivityLevels: []vimv1.LatencySensitivityLevel{vimv1.LatencySensitivityLevelHigh},
				IOMMUSupported:           true,
			}

			merged := configpolicysync.Merge(spec, vimv1.ConfigTargetStatus{})

			Expect(merged.LatencySensitivityLevels).To(Equal(spec.LatencySensitivityLevels))
			Expect(merged.IOMMUSupported).To(BeTrue())
		})
	})
})

func quantityPtr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}
