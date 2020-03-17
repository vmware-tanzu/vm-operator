// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func createResourcePool(rpName string) (*object.ResourcePool, error) {
	rpSpec := &vmoperatorv1alpha1.ResourcePoolSpec{
		Name: rpName,
	}
	_, err := session.CreateResourcePool(context.TODO(), rpSpec)
	Expect(err).NotTo(HaveOccurred())
	return session.GetResourcePoolByPath(context.Background(), session.ChildResourcePoolPath(rpSpec.Name))

}

func createFolder(folderName string) (*object.Folder, error) {
	folderSpec := &vmoperatorv1alpha1.FolderSpec{
		Name: folderName,
	}
	_, err := session.CreateFolder(context.TODO(), folderSpec)
	Expect(err).NotTo(HaveOccurred())
	return session.GetFolderByPath(context.Background(), session.ChildFolderPath(folderSpec.Name))
}

func getSimpleVirtualMachine(name string) *vmoperatorv1alpha1.VirtualMachine {
	return &vmoperatorv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func getVmConfigArgs(namespace, name string) vmprovider.VmConfigArgs {
	vmClass := getVMClassInstance(name, namespace)

	return vmprovider.VmConfigArgs{
		VmClass:          *vmClass,
		ResourcePolicy:   nil,
		VmMetadata:       map[string]string{},
		StorageProfileID: "foo",
	}
}

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
			ClusterModules: []vmoperatorv1alpha1.ClusterModuleSpec{
				{GroupName: "ControlPlane"},
				{GroupName: "NodeGroup1"},
			},
		},
	}
}

var _ = Describe("VMProvider Tests", func() {

	Context("Creating a VM via vmprovider", func() {
		vmNamespace := integration.DefaultNamespace
		var vmProvider vmprovider.VirtualMachineProviderInterface
		var err error

		BeforeEach(func() {
			vmProvider = vsphere.NewVSphereMachineProviderFromClients(clientSet, nil, nil)
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
				Expect(vm.Status.BiosUUID).Should(BeEmpty())

				// Note that createVirtualMachine has the side effect of changing the vm input value
				vmConfigArgs := vmprovider.VmConfigArgs{*vmClass, nil, vmMetadata, "foo"}
				err := vmProvider.CreateVirtualMachine(context.TODO(), vm, vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				//Expect(vm.Status.VmIp).Should(Equal(testIP))
				//Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
				//Expect(vm.Status.BiosUUID).ShouldNot(BeEmpty())
			})
		})

		Context("and the IP is not available on create", func() {

			It("should correctly update VirtualMachineStatus", func() {
				vmName := "test-vm-vmp-noip"
				imageName := "" // create, not clone
				vmConfigArgs := getVmConfigArgs(vmNamespace, vmName)

				vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmConfigArgs.VmClass.Name)
				Expect(vm.Status.BiosUUID).Should(BeEmpty())

				// Note that createVirtualMachine has the side effect of changing the vm input value
				err = vmProvider.CreateVirtualMachine(context.TODO(), vm, vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				//Expect(vm.Status.VmIp).Should(BeEmpty())
				//Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
				//Expect(vm.Status.BiosUUID).ShouldNot(BeEmpty())
			})
		})
	})

	Context("When using Content Library", func() {
		var vmProvider vmprovider.VirtualMachineProviderInterface
		var err error
		vmNamespace := integration.DefaultNamespace
		vmName := "test-vm-vmp-deploy"

		BeforeEach(func() {
			err = vsphere.InstallVSphereVmProviderConfig(clientSet, integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
				integration.SecretName)
			Expect(err).NotTo(HaveOccurred())

			vmProvider = vsphere.NewVSphereMachineProviderFromClients(clientSet, nil, nil)

			// Instruction to vcsim to give the VM an IP address, otherwise CreateVirtualMachine fails
			testIP := "10.0.0.1"
			vmMetadata := map[string]string{"SET.guest.ipAddress": testIP}
			imageName := integration.IntegrationContentLibraryItemName
			vmClass := getVMClassInstance(vmName, vmNamespace)
			vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmClass.Name)
			Expect(vm.Status.BiosUUID).Should(BeEmpty())

			// CreateVirtualMachine from CL
			vmConfigArgs := vmprovider.VmConfigArgs{*vmClass, nil, vmMetadata, "foo"}
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm, vmConfigArgs)
			Expect(err).NotTo(HaveOccurred())
			//Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOff))

			// Update Virtual Machine to Reconfigure with VM Class config
			err = vmProvider.UpdateVirtualMachine(context.TODO(), vm, vmConfigArgs)
			Expect(vm.Status.VmIp).Should(Equal(testIP))
			Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
			Expect(vm.Status.BiosUUID).ShouldNot(BeEmpty())
		})

		// DWB: Disabling this test until I work with Doug M. to determine why there is a FileAlreadyExists error being
		// emitted by Govmomi (I suspect from simulator/virtual_machine.go
		XIt("2 VMs with the same name should be created in different namespaces", func() {
			sameVmName := "same-vm"
			vmNamespace1 := vmNamespace + "-1"
			vmNamespace2 := vmNamespace + "-2"

			_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: vmNamespace1}})
			Expect(err).NotTo(HaveOccurred())

			_, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: vmNamespace2}})
			Expect(err).NotTo(HaveOccurred())

			folder1, err := createFolder(vmNamespace1)
			Expect(err).NotTo(HaveOccurred())

			rp1, err := createResourcePool(vmNamespace1)
			Expect(err).NotTo(HaveOccurred())

			folder2, err := createFolder(vmNamespace2)
			Expect(err).NotTo(HaveOccurred())

			rp2, err := createResourcePool(vmNamespace2)
			Expect(err).NotTo(HaveOccurred())

			resourcePolicy1 := &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmNamespace1,
					Name:      sameVmName,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSetResourcePolicySpec{
					ResourcePool: vmoperatorv1alpha1.ResourcePoolSpec{
						Name:         rp1.Name(),
						Reservations: vmoperatorv1alpha1.VirtualMachineResourceSpec{},
						Limits:       vmoperatorv1alpha1.VirtualMachineResourceSpec{},
					},
					Folder: vmoperatorv1alpha1.FolderSpec{
						Name: folder1.Name(),
					},
				},
			}
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy1)).To(Succeed())

			resourcePolicy2 := &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmNamespace2,
					Name:      sameVmName,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSetResourcePolicySpec{
					ResourcePool: vmoperatorv1alpha1.ResourcePoolSpec{
						Name:         rp2.Name(),
						Reservations: vmoperatorv1alpha1.VirtualMachineResourceSpec{},
						Limits:       vmoperatorv1alpha1.VirtualMachineResourceSpec{},
					},
					Folder: vmoperatorv1alpha1.FolderSpec{
						Name: folder2.Name(),
					},
				},
			}
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(context.TODO(), resourcePolicy2)).To(Succeed())

			vmConfigArgs1 := vmprovider.VmConfigArgs{
				VmClass:          *getVMClassInstance(sameVmName, vmNamespace1),
				ResourcePolicy:   resourcePolicy1,
				VmMetadata:       map[string]string{},
				StorageProfileID: "foo",
			}

			vmConfigArgs2 := vmprovider.VmConfigArgs{
				VmClass:          *getVMClassInstance(sameVmName, vmNamespace2),
				ResourcePolicy:   resourcePolicy2,
				VmMetadata:       map[string]string{},
				StorageProfileID: "foo",
			}

			imageName := integration.IntegrationContentLibraryItemName

			vm1 := &vmoperatorv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmNamespace1,
					Name:      sameVmName,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSpec{
					ImageName:  imageName,
					ClassName:  vmConfigArgs1.VmClass.Name,
					PowerState: vmoperatorv1alpha1.VirtualMachinePoweredOn,
					Ports:      []vmoperatorv1alpha1.VirtualMachinePort{},
					VmMetadata: &vmoperatorv1alpha1.VirtualMachineMetadata{
						Transport: "ExtraConfig",
					},
					ResourcePolicyName: resourcePolicy1.Name,
				},
			}

			// CreateVirtualMachine from CL
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm1, vmConfigArgs1)
			Expect(err).NotTo(HaveOccurred())

			vm2 := &vmoperatorv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmNamespace2,
					Name:      sameVmName,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSpec{
					ImageName:  imageName,
					ClassName:  vmConfigArgs2.VmClass.Name,
					PowerState: vmoperatorv1alpha1.VirtualMachinePoweredOn,
					Ports:      []vmoperatorv1alpha1.VirtualMachinePort{},
					VmMetadata: &vmoperatorv1alpha1.VirtualMachineMetadata{
						Transport: "ExtraConfig",
					},
					ResourcePolicyName: resourcePolicy2.Name,
				},
			}
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm2, vmConfigArgs2)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("ListVirtualMachineImages", func() {
		It("should list the virtualmachineimages available in CL", func() {
			var images []*vmoperatorv1alpha1.VirtualMachineImage

			//Configure to use Content Library
			vSphereConfig.ContentSource = integration.GetContentSourceID()
			err := session.ConfigureContent(context.TODO(), vSphereConfig.ContentSource)
			Expect(err).ShouldNot(HaveOccurred())

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
			var image *vmoperatorv1alpha1.VirtualMachineImage

			//Configure to use Content Library
			vSphereConfig.ContentSource = integration.GetContentSourceID()
			err := session.ConfigureContent(context.TODO(), vSphereConfig.ContentSource)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() *vmoperatorv1alpha1.VirtualMachineImage {
				image, err = vmProvider.GetVirtualMachineImage(context.TODO(), integration.DefaultNamespace, integration.IntegrationContentLibraryItemName)
				Expect(err).To(BeNil())
				return image
			}, time.Second*15).ShouldNot(BeNil())

			Expect(image.Name).Should(BeEquivalentTo(integration.IntegrationContentLibraryItemName))
		})
	})

	Context("VirtualMachineSetResourcePolicy", func() {
		var (
			vmProvider          vmprovider.VirtualMachineProviderInterface
			resourcePolicy      *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
			testPolicyName      string
			testPolicyNamespace string
		)

		JustBeforeEach(func() {
			testPolicyName = "test-name"
			testPolicyNamespace = integration.DefaultNamespace

			vmProvider = vsphere.NewVSphereMachineProviderFromClients(clientSet, nil, nil)

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
				resourcePolicy.Status.ClusterModules = append([]vmoperatorv1alpha1.ClusterModuleStatus{vmoperatorv1alpha1.ClusterModuleStatus{"invalid-group", "invalid-uuid"}}, resourcePolicy.Status.ClusterModules...)
			})
		})
	})

	Context("Compute CPU Min Frequency in the Cluster", func() {
		var vmProvider vmprovider.VirtualMachineProviderInterface
		var err error
		It("reconfigure and power on without errors", func() {
			vmProvider = vsphere.NewVSphereMachineProviderFromClients(clientSet, nil, nil)

			err = vmProvider.ComputeClusterCpuMinFrequency(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(session.GetCpuMinMHzInCluster()).Should(BeNumerically(">", 0))
		})
	})

	Context("Update PNID", func() {
		It("update pnid when the same pnid is supplied", func() {
			vmProvider := vsphere.NewVSphereVmProviderFromClients(clientSet, nil, nil)
			providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, "")
			Expect(err).NotTo(HaveOccurred())

			// Same PNID
			config := BuildNewWcpClusterConfigMap(providerConfig.VcPNID)
			err = vmProvider.UpdateVcPNID(context.TODO(), &config)
			Expect(err).NotTo(HaveOccurred())
		})
		It("update pnid when a different pnid is supplied", func() {
			vmProvider := vsphere.NewVSphereVmProviderFromClients(clientSet, nil, nil)
			providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, "")
			Expect(err).NotTo(HaveOccurred())

			// Different PNID
			pnid := providerConfig.VcPNID + "-01"
			config := BuildNewWcpClusterConfigMap(pnid)
			err = vmProvider.UpdateVcPNID(context.TODO(), &config)
			Expect(err).NotTo(HaveOccurred())
			providerConfig, _ = vsphere.GetProviderConfigFromConfigMap(clientSet, "")
			Expect(providerConfig.VcPNID).Should(Equal(pnid))
		})
	})
})

func BuildNewWcpClusterConfigMap(pnid string) v1.ConfigMap {
	wcpClusterConfig := &vsphere.WcpClusterConfig{
		VcPNID: pnid,
		VcPort: vsphere.DefaultVCPort,
	}

	configMap, err := vsphere.BuildNewWcpClusterConfigMap(wcpClusterConfig)
	Expect(err).NotTo(HaveOccurred())

	return configMap
}
