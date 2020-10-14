// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	stdlog "log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

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

	return session.ChildResourcePool(context.TODO(), rpSpec.Name)
}

func createFolder(folderName string) (*object.Folder, error) {
	folderSpec := &vmoperatorv1alpha1.FolderSpec{
		Name: folderName,
	}
	_, err := session.CreateFolder(context.TODO(), folderSpec)
	Expect(err).NotTo(HaveOccurred())

	return session.ChildFolder(context.TODO(), folderSpec.Name)
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
		VmMetadata:       &vmprovider.VmMetadata{},
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
		},
	}
}

var _ = Describe("VMProvider Inventory Tests", func() {

	BeforeEach(func() {
		Expect(vmProvider.(vsphere.VSphereVmProviderGetSessionHack).SetContentLibrary(ctx, "")).To(Succeed())
	})

	Context("When using inventory", func() {
		It("should support controller like workflow", func() {
			vmNamespace := integration.DefaultNamespace
			vmName := "test-vm-vmp-invt-deploy"

			images, err := vmProvider.ListVirtualMachineImages(context.TODO(), integration.DefaultNamespace)
			Expect(err).NotTo(HaveOccurred())
			for _, image := range images {
				stdlog.Printf("image %s\n", image.Name)
			}

			vmMetadata := &vmprovider.VmMetadata{
				Transport: vmoperatorv1alpha1.VirtualMachineMetadataOvfEnvTransport,
			}
			imageName := "DC0_H0_VM0" // Default govcsim image name
			vmClass := getVMClassInstance(vmName, vmNamespace)
			vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmClass.Name)
			Expect(vm.Status.BiosUUID).Should(BeEmpty())

			exists, err := vmProvider.DoesVirtualMachineExist(ctx, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())

			vmConfigArgs := vmprovider.VmConfigArgs{*vmClass, nil, vmMetadata, "foo"}
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm, vmConfigArgs)
			Expect(err).NotTo(HaveOccurred())

			// Update Virtual Machine to Reconfigure with VM Class config
			err = vmProvider.UpdateVirtualMachine(context.TODO(), vm, vmConfigArgs)
			Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
			Expect(vm.Status.BiosUUID).ShouldNot(BeEmpty())

			exists, err = vmProvider.DoesVirtualMachineExist(ctx, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())

			err = vmProvider.DeleteVirtualMachine(ctx, vm)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("VMProvider Tests", func() {
	BeforeEach(func() {
		Expect(vmProvider.(vsphere.VSphereVmProviderGetSessionHack).SetContentLibrary(ctx, integration.GetContentSourceID())).To(Succeed())
	})

	Context("When using Content Library", func() {
		var vmProvider vmprovider.VirtualMachineProviderInterface
		var err error
		vmNamespace := integration.DefaultNamespace
		vmName := "test-vm-vmp-deploy"

		BeforeEach(func() {
			err = vsphere.InstallVSphereVmProviderConfig(k8sClient, integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
				integration.SecretName)
			Expect(err).NotTo(HaveOccurred())

			vmProvider = vsphere.NewVSphereVmProviderFromClients(nil, k8sClient, nil)

			// Instruction to vcsim to give the VM an IP address, otherwise CreateVirtualMachine fails
			testIP := "10.0.0.1"
			vmMetadata := &vmprovider.VmMetadata{
				Data:      map[string]string{"SET.guest.ipAddress": testIP},
				Transport: vmoperatorv1alpha1.VirtualMachineMetadataExtraConfigTransport,
			}
			imageName := integration.IntegrationContentLibraryItemName
			vmClass := getVMClassInstance(vmName, vmNamespace)
			vm := getVirtualMachineInstance(vmName, vmNamespace, imageName, vmClass.Name)
			Expect(vm.Status.BiosUUID).Should(BeEmpty())

			exists, err := vmProvider.DoesVirtualMachineExist(ctx, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())

			// CreateVirtualMachine from CL
			vmConfigArgs := vmprovider.VmConfigArgs{*vmClass, nil, vmMetadata, "foo"}
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm, vmConfigArgs)
			Expect(err).NotTo(HaveOccurred())

			// Update Virtual Machine to Reconfigure with VM Class config
			err = vmProvider.UpdateVirtualMachine(context.TODO(), vm, vmConfigArgs)
			Expect(vm.Status.VmIp).Should(Equal(testIP))
			Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
			Expect(vm.Status.BiosUUID).ShouldNot(BeEmpty())
		})

		It("should work", func() {
			// Everything done in the BeforeEach().
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
				VmMetadata:       &vmprovider.VmMetadata{},
				StorageProfileID: "foo",
			}

			vmConfigArgs2 := vmprovider.VmConfigArgs{
				VmClass:          *getVMClassInstance(sameVmName, vmNamespace2),
				ResourcePolicy:   resourcePolicy2,
				VmMetadata:       &vmprovider.VmMetadata{},
				StorageProfileID: "foo",
			}

			imageName := integration.IntegrationContentLibraryItemName

			vm1 := &vmoperatorv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmNamespace1,
					Name:      sameVmName,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSpec{
					ImageName:          imageName,
					ClassName:          vmConfigArgs1.VmClass.Name,
					PowerState:         vmoperatorv1alpha1.VirtualMachinePoweredOn,
					Ports:              []vmoperatorv1alpha1.VirtualMachinePort{},
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
					ImageName:          imageName,
					ClassName:          vmConfigArgs2.VmClass.Name,
					PowerState:         vmoperatorv1alpha1.VirtualMachinePoweredOn,
					Ports:              []vmoperatorv1alpha1.VirtualMachinePort{},
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

			Eventually(func() int {
				images, err = vmProvider.ListVirtualMachineImages(context.TODO(), integration.DefaultNamespace)
				Expect(err).ToNot(HaveOccurred())
				return len(images)
			}, time.Second*15).Should(BeNumerically(">", 0))

			found := false
			for _, image := range images {
				if image.Name == integration.IntegrationContentLibraryItemName {
					found = true
					break
				}
			}
			Expect(found).Should(BeTrue())
		})
	})

	Context("GetVirtualMachineImage", func() {

		It("should get the existing virtualmachineimage", func() {
			var image *vmoperatorv1alpha1.VirtualMachineImage

			Eventually(func() *vmoperatorv1alpha1.VirtualMachineImage {
				image, err = vmProvider.GetVirtualMachineImage(context.TODO(), integration.DefaultNamespace, integration.IntegrationContentLibraryItemName)
				Expect(err).ToNot(HaveOccurred())
				return image
			}, time.Second*15).ShouldNot(BeNil())

			Expect(image.Name).Should(BeEquivalentTo(integration.IntegrationContentLibraryItemName))
		})
	})

	Context("Compute CPU Min Frequency in the Cluster", func() {
		It("reconfigure and power on without errors", func() {
			vmProvider := vsphere.NewVSphereVmProviderFromClients(nil, k8sClient, nil)
			err := vmProvider.ComputeClusterCpuMinFrequency(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(session.GetCpuMinMHzInCluster()).Should(BeNumerically(">", 0))
		})
	})

	Context("Update PNID", func() {
		It("update pnid when the same pnid is supplied", func() {
			vmProvider := vsphere.NewVSphereVmProviderFromClients(nil, k8sClient, nil)
			providerConfig, err := vsphere.GetProviderConfigFromConfigMap(k8sClient, "")
			Expect(err).NotTo(HaveOccurred())

			// Same PNID
			config := BuildNewWcpClusterConfigMap(providerConfig.VcPNID)
			err = vmProvider.UpdateVcPNID(context.TODO(), &config)
			Expect(err).NotTo(HaveOccurred())
		})

		// This test ends up causes races with other tests in this suite because it changes the PNID
		// but all the tests in this directory run in the same suite with the same testenv so the
		// same apiserver. We could restore the valid PNID after but we should really fix the tests
		// to use a unique vcsim env per Describe() context. This VM Provider code is also executed
		// and tested in the infra controller test so disable it here.
		XIt("update pnid when a different pnid is supplied", func() {
			vmProvider := vsphere.NewVSphereVmProviderFromClients(nil, k8sClient, nil)
			providerConfig, err := vsphere.GetProviderConfigFromConfigMap(k8sClient, "")
			Expect(err).NotTo(HaveOccurred())

			// Different PNID
			pnid := providerConfig.VcPNID + "-01"
			config := BuildNewWcpClusterConfigMap(pnid)
			err = vmProvider.UpdateVcPNID(context.TODO(), &config)
			Expect(err).NotTo(HaveOccurred())
			providerConfig, _ = vsphere.GetProviderConfigFromConfigMap(k8sClient, "")
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
