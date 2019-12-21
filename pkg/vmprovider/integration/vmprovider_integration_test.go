// +build integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func getVMClassInstance(vmName, namespace string) *vmoperatorv1alpha1.VirtualMachineClass {
	return &vmoperatorv1alpha1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-class", vmName),
		},
		Spec: vmoperatorv1alpha1.VirtualMachineClassSpec{
			Hardware: vmoperatorv1alpha1.VirtualMachineClassHardware{
				Cpus:   4,
				Memory: resource.MustParse("1Mi"),
			},
			Policies: vmoperatorv1alpha1.VirtualMachineClassPolicies{
				Resources: vmoperatorv1alpha1.VirtualMachineClassResources{
					Requests: vmoperatorv1alpha1.VirtualMachineResourceSpec{
						Cpu:    resource.MustParse("1000Mi"),
						Memory: resource.MustParse("100Mi"),
					},
					Limits: vmoperatorv1alpha1.VirtualMachineResourceSpec{
						Cpu:    resource.MustParse("2000Mi"),
						Memory: resource.MustParse("200Mi"),
					},
				},
			},
		},
	}
}

func getVirtualMachineInstance(name, namespace, imageName, className string) *vmoperatorv1alpha1.VirtualMachine {
	return &vmoperatorv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSpec{
			ImageName:  imageName,
			ClassName:  className,
			PowerState: vmoperatorv1alpha1.VirtualMachinePoweredOn,
			Ports:      []vmoperatorv1alpha1.VirtualMachinePort{},
			VmMetadata: &vmoperatorv1alpha1.VirtualMachineMetadata{
				Transport: "ExtraConfig",
			},
		},
	}
}

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
		},
	}
}

var _ = Describe("VMProvider Tests", func() {

	Context("Creating a VM via vmprovider", func() {
		vmNamespace := integration.DefaultNamespace
		var vmProvider *vsphere.VSphereVmProvider
		var err error

		BeforeEach(func() {
			// Create a new VMProvder from the config provided by the test
			vmProvider, err = vsphere.NewVSphereVmProvider(clientSet, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and the IP is available on create", func() {
			It("should correctly update VirtualMachineStatus", func() {
				vmName := "test-vm-vmp"

				// Instruction to vcsim to give the VM an IP address, otherwise CreateVirtualMachine fails
				testIP := "10.0.0.1"
				vmMetadata := map[string]string{"SET.guest.ipAddress": testIP}
				imageName := "" // create, not clone
				vmClass := getVMClassInstance(vmName, vmNamespace)
				vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmClass.Name)
				Expect(vm.Status.BiosUuid).Should(BeEmpty())

				// Note that createVirtualMachine has the side effect of changing the vm input value
				err := vmProvider.CreateVirtualMachine(context.TODO(), vm, *vmClass, nil, vmMetadata, "testProfileID")
				Expect(err).NotTo(HaveOccurred())
				//Expect(vm.Status.VmIp).Should(Equal(testIP))
				//Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
				//Expect(vm.Status.BiosUuid).ShouldNot(BeEmpty())
			})
		})

		Context("and the IP is not available on create", func() {

			It("should correctly update VirtualMachineStatus", func() {
				vmName := "test-vm-vmp-noip"

				vmMetadata := map[string]string{}
				imageName := "" // create, not clone
				vmClass := getVMClassInstance(vmName, vmNamespace)
				vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmClass.Name)
				Expect(vm.Status.BiosUuid).Should(BeEmpty())

				// Note that createVirtualMachine has the side effect of changing the vm input value
				err = vmProvider.CreateVirtualMachine(context.TODO(), vm, *vmClass, nil, vmMetadata, "testProfileID")
				Expect(err).NotTo(HaveOccurred())
				//Expect(vm.Status.VmIp).Should(BeEmpty())
				//Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
				//Expect(vm.Status.BiosUuid).ShouldNot(BeEmpty())
			})
		})
	})

	Context("Creating and Updating a VM from Content Library", func() {
		var vmProvider *vsphere.VSphereVmProvider
		var err error
		vmNamespace := integration.DefaultNamespace
		vmName := "test-vm-vmp-deploy"

		It("reconfigure and power on without errors", func() {

			err = vsphere.InstallVSphereVmProviderConfig(clientSet, integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
				integration.SecretName)
			Expect(err).NotTo(HaveOccurred())

			vmProvider, err = vsphere.NewVSphereVmProvider(clientSet, nil)
			Expect(err).NotTo(HaveOccurred())

			// Instruction to vcsim to give the VM an IP address, otherwise CreateVirtualMachine fails
			testIP := "10.0.0.1"
			vmMetadata := map[string]string{"SET.guest.ipAddress": testIP}
			imageName := integration.IntegrationContentLibraryItemName
			vmClass := getVMClassInstance(vmName, vmNamespace)
			vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmClass.Name)
			Expect(vm.Status.BiosUuid).Should(BeEmpty())

			// CreateVirtualMachine from CL
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm, *vmClass, nil, vmMetadata, "testProfileID")
			Expect(err).NotTo(HaveOccurred())
			//Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOff))

			// Update Virtual Machine to Reconfigure with VM Class config
			err = vmProvider.UpdateVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata)
			Expect(vm.Status.VmIp).Should(Equal(testIP))
			Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
			Expect(vm.Status.BiosUuid).ShouldNot(BeEmpty())
		})
	})

	Context("ListVirtualMachineImages", func() {

		It("should list the virtualmachineimages available in CL", func() {

			vmProvider := vmprovider.GetVmProviderOrDie()
			var images []*vmoperatorv1alpha1.VirtualMachineImage
			var err error

			Eventually(func() int {
				images, err = vmProvider.ListVirtualMachineImages(context.TODO(), integration.DefaultNamespace)
				Expect(err).To(BeNil())
				return len(images)
			}, time.Second*15).Should(BeNumerically(">", 0))

			found := false
			for _, image := range images {
				if image.Name == integration.IntegrationContentLibraryItemName {
					found = true
				}
			}

			Expect(found).Should(BeTrue())
		})
	})

	Context("GetVirtualMachineImage", func() {

		It("should get the existing virtualmachineimage", func() {

			vmProvider := vmprovider.GetVmProviderOrDie()

			var image *vmoperatorv1alpha1.VirtualMachineImage

			Eventually(func() *vmoperatorv1alpha1.VirtualMachineImage {
				image, _ = vmProvider.GetVirtualMachineImage(context.TODO(), integration.DefaultNamespace, integration.IntegrationContentLibraryItemName)
				return image
			}, time.Second*15).ShouldNot(BeNil())
			Expect(image.Name).Should(BeEquivalentTo(integration.IntegrationContentLibraryItemName))
		})
	})

	Context("VirtualMachineSetResourcePolicy", func() {
		var (
			vmProvider          *vsphere.VSphereVmProvider
			resourcePolicy      *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
			testPolicyName      string
			testPolicyNamespace string
			err                 error
		)

		JustBeforeEach(func() {
			testPolicyName = "test-name"
			testPolicyNamespace = integration.DefaultNamespace

			vmProvider, err = vsphere.NewVSphereVmProvider(clientSet, nil)
			Expect(err).NotTo(HaveOccurred())

			resourcePolicy = getVirtualMachineSetResourcePolicy(testPolicyName, testPolicyNamespace)
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy)).To(Succeed())
		})

		JustAfterEach(func() {
			Expect(vmProvider.DeleteVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy)).To(Succeed())
		})

		Context("for an existing resource policy", func() {
			It("should update VirtualMachineSetResourcePolicy", func() {
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy)).To(Succeed())
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
	})

	Context("Compute CPU Min Frequency in the Cluster", func() {
		var vmProvider *vsphere.VSphereVmProvider
		var err error
		It("reconfigure and power on without errors", func() {
			vmProvider, err = vsphere.NewVSphereVmProvider(clientSet, nil)
			err = vmProvider.ComputeClusterCpuMinFrequency(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(session.GetCpuMinMHzInCluster()).Should(BeNumerically(">", 0))
		})
	})
})
