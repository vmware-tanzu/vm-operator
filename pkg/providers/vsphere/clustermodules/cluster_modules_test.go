// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func cmTests() {
	Describe("Cluster Modules", func() {

		var (
			ctx        *builder.TestContextForVCSim
			cmProvider clustermodules.Provider

			moduleGroup  string
			moduleSpec   *vmopv1a1.ClusterModuleSpec
			moduleStatus *vmopv1a1.ClusterModuleStatus
			clusterRef   types.ManagedObjectReference
			vmRef        types.ManagedObjectReference
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			cmProvider = clustermodules.NewProvider(ctx.RestClient)

			clusterRef = ctx.GetFirstClusterFromFirstZone().Reference()

			moduleGroup = "controller-group"
			moduleSpec = &vmopv1a1.ClusterModuleSpec{
				GroupName: moduleGroup,
			}

			moduleID, err := cmProvider.CreateModule(ctx, clusterRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(moduleID).ToNot(BeEmpty())

			moduleStatus = &vmopv1a1.ClusterModuleStatus{
				GroupName:  moduleSpec.GroupName,
				ModuleUuid: moduleID,
			}

			// TODO: Create VM instead of using one that vcsim creates for free.
			vm, err := ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
			Expect(err).ToNot(HaveOccurred())
			vmRef = vm.Reference()
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		It("Create a ClusterModule, verify it exists and delete it", func() {
			exists, err := cmProvider.DoesModuleExist(ctx, moduleStatus.ModuleUuid, clusterRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())

			Expect(cmProvider.DeleteModule(ctx, moduleStatus.ModuleUuid)).To(Succeed())

			exists, err = cmProvider.DoesModuleExist(ctx, moduleStatus.ModuleUuid, clusterRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		Context("ClusterModule-VM association", func() {
			It("check membership doesn't exist", func() {
				isMember, err := cmProvider.IsMoRefModuleMember(ctx, moduleStatus.ModuleUuid, vmRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeFalse())
			})

			It("Associate a VM with a clusterModule, check the membership and remove it", func() {
				By("Associate VM")
				err := cmProvider.AddMoRefToModule(ctx, moduleStatus.ModuleUuid, vmRef)
				Expect(err).NotTo(HaveOccurred())

				By("Verify membership")
				isMember, err := cmProvider.IsMoRefModuleMember(ctx, moduleStatus.ModuleUuid, vmRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeTrue())

				By("Remove the association")
				err = cmProvider.RemoveMoRefFromModule(ctx, moduleStatus.ModuleUuid, vmRef)
				Expect(err).NotTo(HaveOccurred())

				By("Verify no longer a member")
				isMember, err = cmProvider.IsMoRefModuleMember(ctx, moduleStatus.ModuleUuid, vmRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeFalse())
			})
		})
	})
}
