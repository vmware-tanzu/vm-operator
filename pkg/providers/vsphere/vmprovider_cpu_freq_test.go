// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CPU Frequency", func() {
	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vmProvider = nil
	})

	Context("ComputeCPUMinFrequency", func() {
		It("returns success", func() {
			Expect(vmProvider.ComputeCPUMinFrequency(ctx)).To(Succeed())
		})
	})

	Context("GetVirtualMachineClassResources", func() {
		It("converts CPU defaults using the current Supervisor minimum frequency", func() {
			cpu := resource.MustParse("2")
			spec := vmopv1.VirtualMachineClassSpec{
				Resources: vmopv1.VirtualMachineClassResources{
					Requests: vmopv1.VirtualMachineClassResourceSpec{
						CPU: &vmopv1.ResourceQuantityRange{Default: &cpu},
					},
				},
			}

			resources, err := vmProvider.GetVirtualMachineClassResources(ctx, spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources.Requests.CPU.Value()).To(Equal(virtualmachine.CPUQuantityToMhz(cpu, vcsimCPUFreq)))
		})

		It("uses raw ConfigSpec allocation values and needs no CPU frequency without policy defaults", func() {
			configSpec := vimtypes.VirtualMachineConfigSpec{
				NumCPUs:  4,
				MemoryMB: 4096,
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](1234), Limit: ptr.To[int64](2345),
				},
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](512), Limit: ptr.To[int64](1024),
				},
			}
			raw, err := util.MarshalConfigSpecToJSON(configSpec)
			Expect(err).ToNot(HaveOccurred())
			spec := vmopv1.VirtualMachineClassSpec{ConfigSpec: raw}

			resources, err := vmProvider.GetVirtualMachineClassResources(pkgcfg.NewContextWithDefaultConfig(), spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources.Size.CPU.Value()).To(Equal(int64(4)))
			Expect(resources.Size.Memory.String()).To(Equal("4Gi"))
			Expect(resources.Requests.CPU.Value()).To(Equal(int64(1234)))
			Expect(resources.Limits.CPU.Value()).To(Equal(int64(2345)))
			Expect(resources.Requests.Memory.Value()).To(Equal(int64(512 * 1024 * 1024)))
			Expect(resources.Limits.Memory.Value()).To(Equal(int64(1024 * 1024 * 1024)))
		})

		It("returns an error when a CPU policy has no current Supervisor frequency", func() {
			var zones topologyv1.AvailabilityZoneList
			Expect(ctx.Client.List(ctx, &zones)).To(Succeed())
			for i := range zones.Items {
				Expect(ctx.Client.Delete(ctx, &zones.Items[i])).To(Succeed())
			}

			cpu := resource.MustParse("1")
			spec := vmopv1.VirtualMachineClassSpec{
				Resources: vmopv1.VirtualMachineClassResources{
					Requests: vmopv1.VirtualMachineClassResourceSpec{
						CPU: &vmopv1.ResourceQuantityRange{Default: &cpu},
					},
				},
			}
			_, err := vmProvider.GetVirtualMachineClassResources(ctx, spec)
			Expect(err).To(HaveOccurred())
		})
	})
})
