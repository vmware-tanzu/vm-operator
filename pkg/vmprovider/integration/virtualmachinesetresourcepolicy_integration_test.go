// +build integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func getVirtualMachineSetResourcePolicy(name, namespace string) *vmopv1alpha1.VirtualMachineSetResourcePolicy {
	return &vmopv1alpha1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-resourcepolicy", name),
		},
		Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmopv1alpha1.ResourcePoolSpec{
				Name:         fmt.Sprintf("%s-resourcepool", name),
				Reservations: vmopv1alpha1.VirtualMachineResourceSpec{},
				Limits:       vmopv1alpha1.VirtualMachineResourceSpec{},
			},
			Folder: vmopv1alpha1.FolderSpec{
				Name: fmt.Sprintf("%s-folder", name),
			},
			ClusterModules: []vmopv1alpha1.ClusterModuleSpec{
				{GroupName: "ControlPlane"},
				{GroupName: "NodeGroup1"},
			},
		},
	}
}

var _ = Describe("vSphere VM resource policy tests", func() {

	Context("Cluster Modules", func() {
		var (
			moduleGroup  string
			moduleSpec   *vmopv1alpha1.ClusterModuleSpec
			moduleStatus *vmopv1alpha1.ClusterModuleStatus
			clusterRef   types.ManagedObjectReference
			resVM        *resources.VirtualMachine
		)

		BeforeEach(func() {
			clusterRef = session.Cluster().Reference()

			moduleGroup = "controller-group"
			moduleSpec = &vmopv1alpha1.ClusterModuleSpec{
				GroupName: moduleGroup,
			}

			moduleId, err := vcClient.ClusterModuleClient().CreateModule(ctx, clusterRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(moduleId).ToNot(BeEmpty())

			moduleStatus = &vmopv1alpha1.ClusterModuleStatus{
				GroupName:  moduleSpec.GroupName,
				ModuleUuid: moduleId,
			}

			resVM, err = session.GetVirtualMachine(vmContext(ctx, getSimpleVirtualMachine("DC0_C0_RP0_VM0")))
			Expect(err).NotTo(HaveOccurred())
			Expect(resVM).NotTo(BeNil())
		})

		AfterEach(func() {
			Expect(vcClient.ClusterModuleClient().DeleteModule(ctx, moduleStatus.ModuleUuid)).To(Succeed())
		})

		Context("Create a ClusterModule, verify it exists and delete it", func() {
			It("Verifies if a ClusterModule exists", func() {
				exists, err := vcClient.ClusterModuleClient().DoesModuleExist(ctx, moduleStatus.ModuleUuid, clusterRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})

		Context("ClusterModule-VM association", func() {
			It("check membership doesn't exist", func() {
				vmCtx := vmContext(ctx, &vmopv1alpha1.VirtualMachine{})
				isMember, err := vcClient.ClusterModuleClient().IsMoRefModuleMember(vmCtx, moduleStatus.ModuleUuid, resVM.MoRef())
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeFalse())
			})

			It("Associate a VM with a clusterModule, check the membership and remove it", func() {
				vmCtx := vmContext(ctx, &vmopv1alpha1.VirtualMachine{})

				By("Associate VM")
				err = vcClient.ClusterModuleClient().AddMoRefToModule(vmCtx, moduleStatus.ModuleUuid, resVM.MoRef())
				Expect(err).NotTo(HaveOccurred())

				By("Verify membership")
				isMember, err := vcClient.ClusterModuleClient().IsMoRefModuleMember(vmCtx, moduleStatus.ModuleUuid, resVM.MoRef())
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeTrue())

				By("Remove the association")
				err = vcClient.ClusterModuleClient().RemoveMoRefFromModule(vmCtx, moduleStatus.ModuleUuid, resVM.MoRef())
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
