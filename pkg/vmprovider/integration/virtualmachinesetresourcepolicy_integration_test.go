// +build integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func getVirtualMachineSetResourcePolicy(name, namespace string) *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy {
	return &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-resourcepolicy", name),
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmoperatorv1alpha1.ResourcePoolSpec{
				Name:         fmt.Sprintf("%s-resourcepool", name),
				Reservations: vmoperatorv1alpha1.VirtualMachineResourceSpec{},
				Limits:       vmoperatorv1alpha1.VirtualMachineResourceSpec{},
			},
			Folder: vmoperatorv1alpha1.FolderSpec{
				Name: fmt.Sprintf("%s-folder", name),
			},
			ClusterModules: []vmoperatorv1alpha1.ClusterModuleSpec{
				{GroupName: "ControlPlane"},
				{GroupName: "NodeGroup1"},
			},
		},
	}
}

var _ = Describe("vSphere VM provider tests", func() {

	Context("VirtualMachineSetResourcePolicy", func() {
		var (
			resourcePolicy      *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
			testPolicyName      string
			testPolicyNamespace string
		)

		JustBeforeEach(func() {
			testPolicyName = "test-name"
			testPolicyNamespace = integration.DefaultNamespace

			resourcePolicy = getVirtualMachineSetResourcePolicy(testPolicyName, testPolicyNamespace)
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy)).To(Succeed())
			Expect(len(resourcePolicy.Status.ClusterModules)).Should(BeNumerically("==", 2))
		})

		JustAfterEach(func() {
			Expect(vmProvider.DeleteVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy)).To(Succeed())
			Expect(len(resourcePolicy.Status.ClusterModules)).Should(BeNumerically("==", 0))
		})

		Context("for an existing resource policy", func() {
			It("should update VirtualMachineSetResourcePolicy", func() {
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy)).To(Succeed())
				Expect(len(resourcePolicy.Status.ClusterModules)).Should(BeNumerically("==", 2))
			})

			It("successfully able to find the resourcepolicy", func() {
				exists, err := vmProvider.DoesVirtualMachineSetResourcePolicyExist(context.TODO(), resourcePolicy)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})

		Context("for an absent resource policy", func() {
			It("should fail to find the resource policy without any errors", func() {
				failResPolicy := getVirtualMachineSetResourcePolicy("test-policy", testPolicyNamespace)
				exists, err := vmProvider.DoesVirtualMachineSetResourcePolicyExist(context.TODO(), failResPolicy)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).NotTo(BeTrue())
			})
		})

		Context("for a resource policy with invalid cluster module", func() {
			It("successfully able to delete the resourcepolicy", func() {
				resourcePolicy.Status.ClusterModules = append([]vmoperatorv1alpha1.ClusterModuleStatus{
					{
						GroupName:  "invalid-group",
						ModuleUuid: "invalid-uuid",
					},
				}, resourcePolicy.Status.ClusterModules...)
			})
		})
	})
})
