// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func resourcePoolTests() {
	Describe("GetResourcePool", getResourcePoolTests)
	Describe("CreateDeleteExistResourcePoolChild", createDeleteExistResourcePoolChild)
}

func getResourcePoolTests() {
	var (
		ctx        *builder.TestContextForVCSim
		nsInfo     builder.WorkloadNamespaceInfo
		nsRP       *object.ResourcePool
		testConfig builder.VCSimTestConfig
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		ctx = suite.NewTestContextForVCSim(testConfig)
		nsInfo = ctx.CreateWorkloadNamespace()
		nsRP = ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		nsRP = nil
	})

	Context("GetResourcePoolByMoID", func() {
		It("returns success", func() {
			rp, err := vcenter.GetResourcePoolByMoID(ctx, ctx.Finder, nsRP.Reference().Value)
			Expect(err).ToNot(HaveOccurred())
			Expect(rp).ToNot(BeNil())
			Expect(rp.Reference()).To(Equal(nsRP.Reference()))
		})

		It("returns error when MoID does not exist", func() {
			rp, err := vcenter.GetResourcePoolByMoID(ctx, ctx.Finder, "bogus")
			Expect(err).To(HaveOccurred())
			Expect(rp).To(BeNil())
		})
	})

	Context("GetResourcePoolOwnerMoRef", func() {
		It("returns success", func() {
			ccr, err := vcenter.GetResourcePoolOwnerMoRef(ctx, ctx.VCClient.Client, nsRP.Reference().Value)
			Expect(err).ToNot(HaveOccurred())
			Expect(ccr).To(Equal(ctx.GetFirstClusterFromFirstZone().Reference()))
		})

		It("returns error when MoID does not exist", func() {
			_, err := vcenter.GetResourcePoolOwnerMoRef(ctx, ctx.VCClient.Client, "bogus")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetChildResourcePool", func() {
		It("returns success", func() {
			// Quick way for a child RP is to create a VMSetResourcePolicy.
			resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
			Expect(resourcePolicy).ToNot(BeNil())
			childRPName := resourcePolicy.Spec.ResourcePool.Name
			Expect(childRPName).ToNot(BeEmpty())

			childRP, err := vcenter.GetChildResourcePool(ctx, nsRP, childRPName)
			Expect(err).ToNot(HaveOccurred())
			Expect(childRP).ToNot(BeNil())

			objRef, err := ctx.Finder.ObjectReference(ctx, childRP.Reference())
			Expect(err).ToNot(HaveOccurred())
			childRP, ok := objRef.(*object.ResourcePool)
			Expect(ok).To(BeTrue())
			Expect(childRP.Name()).To(Equal(resourcePolicy.Name))
		})

		It("returns error when child RP does not exist", func() {
			childRP, err := vcenter.GetChildResourcePool(ctx, nsRP, "bogus")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found under parent ResourcePool"))
			Expect(childRP).To(BeNil())
		})
	})
}

func createDeleteExistResourcePoolChild() {

	var (
		ctx        *builder.TestContextForVCSim
		nsInfo     builder.WorkloadNamespaceInfo
		nsRP       *object.ResourcePool
		testConfig builder.VCSimTestConfig

		parentRPMoID   string
		resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		ctx = suite.NewTestContextForVCSim(testConfig)
		nsInfo = ctx.CreateWorkloadNamespace()
		nsRP = ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")

		parentRPMoID = nsRP.Reference().Value

		resourcePolicy, _ = ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
		Expect(resourcePolicy).ToNot(BeNil())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		nsRP = nil
		parentRPMoID = ""
		resourcePolicy = nil
	})

	Context("CreateOrUpdateChildResourcePool", func() {
		It("creates child ResourcePool", func() {
			childMoID, err := vcenter.CreateOrUpdateChildResourcePool(ctx, ctx.VCClient.Client, parentRPMoID, &resourcePolicy.Spec.ResourcePool)
			Expect(err).ToNot(HaveOccurred())
			Expect(childMoID).ToNot(BeEmpty())

			By("returns success when child ResourcePool already exists", func() {
				moID, err := vcenter.CreateOrUpdateChildResourcePool(ctx, ctx.VCClient.Client, parentRPMoID, &resourcePolicy.Spec.ResourcePool)
				Expect(err).ToNot(HaveOccurred())
				Expect(moID).To(Equal(childMoID))
			})

			By("child ResourcePool is found by MoID", func() {
				rp, err := vcenter.GetResourcePoolByMoID(ctx, ctx.Finder, childMoID)
				Expect(err).ToNot(HaveOccurred())
				Expect(rp.Reference().Value).To(Equal(childMoID))
			})
		})

		It("returns error when when parent ResourcePool MoID does not exist", func() {
			childMoID, err := vcenter.CreateOrUpdateChildResourcePool(ctx, ctx.VCClient.Client, "bogus", &resourcePolicy.Spec.ResourcePool)
			Expect(err).To(HaveOccurred())
			Expect(childMoID).To(BeEmpty())
		})
	})

	Context("DoesChildResourcePoolExist", func() {
		It("returns true when child ResourcePool exists", func() {
			childName := resourcePolicy.Spec.ResourcePool.Name

			_, err := vcenter.CreateOrUpdateChildResourcePool(ctx, ctx.VCClient.Client, parentRPMoID, &resourcePolicy.Spec.ResourcePool)
			Expect(err).ToNot(HaveOccurred())

			exists, err := vcenter.DoesChildResourcePoolExist(ctx, ctx.VCClient.Client, parentRPMoID, childName)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("returns false when child ResourcePool does not exist", func() {
			exists, err := vcenter.DoesChildResourcePoolExist(ctx, ctx.VCClient.Client, parentRPMoID, "bogus")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("returns error when parent ResourcePool MoID does not exist", func() {
			exists, err := vcenter.DoesChildResourcePoolExist(ctx, ctx.VCClient.Client, "bogus", "bogus")
			Expect(err).To(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Context("DeleteChildResourcePool", func() {
		It("deletes child ResourcePool", func() {
			childName := resourcePolicy.Spec.ResourcePool.Name

			childMoID, err := vcenter.CreateOrUpdateChildResourcePool(ctx, ctx.VCClient.Client, parentRPMoID, &resourcePolicy.Spec.ResourcePool)
			Expect(err).ToNot(HaveOccurred())

			err = vcenter.DeleteChildResourcePool(ctx, ctx.VCClient.Client, parentRPMoID, childName)
			Expect(err).ToNot(HaveOccurred())

			By("child ResourcePool is not found by MoID", func() {
				_, err := vcenter.GetResourcePoolByMoID(ctx, ctx.Finder, childMoID)
				Expect(err).To(HaveOccurred())
			})

			By("NoOp when child does not exist", func() {
				err := vcenter.DeleteChildResourcePool(ctx, ctx.VCClient.Client, parentRPMoID, childName)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
