// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	goctx "context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/tags"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	vmopsession "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	testNamespace = "test-namespace"
	testVMName    = "test-vm"
)

func vmContext(ctx goctx.Context, vm *vmopv1alpha1.VirtualMachine) context.VirtualMachineContext {
	return context.VirtualMachineContext{
		Context: ctx,
		Logger:  integration.Log,
		VM:      vm,
	}
}

var _ = Describe("Sessions", func() {
	var (
		ctx     goctx.Context
		session *vmopsession.Session
	)

	BeforeEach(func() {
		ctx = goctx.Background()
		session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(session).ToNot(BeNil())
	})

	Describe("Clone VM", func() {

		Context("without specifying any networks in VM Spec", func() {

			It("should not override template networks", func() {
				imageName := "DC0_H0_VM0"
				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(testVMName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				// Cloning from inventory
				vmConfigArgs.ContentLibraryUUID = ""
				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM).ShouldNot(BeNil())

				// Existing NIF should not be changed.
				netDevices, err := clonedVM.GetNetworkDevices(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(netDevices).Should(HaveLen(1))

				dev := netDevices[0].GetVirtualDevice()
				// For the vcsim env the source VM is attached to a distributed port group. Hence, the cloned VM
				// should also be attached to the same network.
				_, ok := dev.Backing.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).Should(BeTrue())
			})
		})

		Context("by specifying networks in VM Spec", func() {

			BeforeEach(func() {
				err := config.InstallNetworkConfigMap(k8sClient, "8.8.8.8 8.8.4.4")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should override template networks", func() {
				imageName := "DC0_H0_VM0"
				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(testVMName+"change-net", testNamespace, imageName, vmConfigArgs.VMClass.Name)
				// Add two network interfaces to the VM and attach to different networks
				vm.Spec.NetworkInterfaces = []vmopv1alpha1.VirtualMachineNetworkInterface{
					{
						NetworkName: "VM Network",
					},
					{
						NetworkName:      "VM Network",
						EthernetCardType: "e1000",
					},
				}

				vmConfigArgs.ContentLibraryUUID = ""
				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())

				netDevices, err := clonedVM.GetNetworkDevices(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(netDevices).Should(HaveLen(2))

				// The interface type should be default vmxnet3
				dev1, ok := netDevices[0].(*vimTypes.VirtualVmxnet3)
				Expect(ok).Should(BeTrue())
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev1.Backing.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).Should(BeTrue())

				// The interface type should be e1000
				dev2, ok := netDevices[1].(*vimTypes.VirtualE1000)
				Expect(ok).To(BeTrue())
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev2.Backing.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).Should(BeTrue())
			})
		})

		Context("when a default network is specified", func() {

			BeforeEach(func() {
				// For the vcsim env the source VM is attached to a distributed port group. Hence, we are
				// using standard vswitch port group.
				// BMV: Is this used outside of the tests?!?
				vSphereConfig.Network = "VM Network"

				// Setup new session based on the default network
				session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should override network from the template", func() {
				imageName := "DC0_H0_VM0"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(testVMName+"with-default-net", testNamespace, imageName, vmConfigArgs.VMClass.Name)

				vmConfigArgs.ContentLibraryUUID = ""
				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM).ShouldNot(BeNil())

				// Existing NIF should not be changed.
				netDevices, err := clonedVM.GetNetworkDevices(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(netDevices).Should(HaveLen(1))

				dev := netDevices[0].GetVirtualDevice()
				// TODO: enhance the test to verify the moref of the network matches the default network.
				_, ok := dev.Backing.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).Should(BeTrue())
			})

			It("should not override networks specified in VM Spec ", func() {
				imageName := "DC0_H0_VM0"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vmConfigArgs.ContentLibraryUUID = ""

				vm := getVirtualMachineInstance(testVMName+"change-default-net", testNamespace, imageName, vmConfigArgs.VMClass.Name)
				// Add two network interfaces to the VM and attach to different networks
				vm.Spec.NetworkInterfaces = []vmopv1alpha1.VirtualMachineNetworkInterface{
					{
						NetworkName: "DC0_DVPG0",
					},
					{
						NetworkName:      "DC0_DVPG0",
						EthernetCardType: "e1000",
					},
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())

				netDevices, err := clonedVM.GetNetworkDevices(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(netDevices).Should(HaveLen(2))

				// The interface type should be default vmxnet3
				dev1, ok := netDevices[0].(*vimTypes.VirtualVmxnet3)
				Expect(ok).Should(BeTrue())

				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev1.Backing.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).Should(BeTrue())

				// The interface type should be e1000
				dev2, ok := netDevices[1].(*vimTypes.VirtualE1000)
				Expect(ok).To(BeTrue())
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev2.Backing.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).Should(BeTrue())
			})
		})

		Context("from Content Library", func() {

			var (
				clonedDeployedVMTX *resources.VirtualMachine
				clonedDeployedVM   *resources.VirtualMachine
			)

			It("should clone VM", func() {
				imageName := "test-item"
				vmName := "CL_DeployedVM"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
			})

			It("should delete cloned VM", func() {
				imageName := "test-item"
				vmName := "CL_DeleteMe_DeployedVM"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				err = session.DeleteVirtualMachine(vmContext(ctx, vm))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should delete cloned VM that is powered on", func() {
				imageName := "test-item"
				vmName := "CL_DeleteMe_PowerON_DeployedVM"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
				Expect(session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)).To(Succeed())

				err = session.DeleteVirtualMachine(vmContext(ctx, vm))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return guest heartbeat", func() {
				imageName := "test-item"
				vmName := "CL_Tools_PowerON_DeployedVM"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
				Expect(session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)).To(Succeed())

				// Just testing for property query: field not set in vcsim.
				heartbeat, err := session.GetVirtualMachineGuestHeartbeat(vmContext(ctx, vm))
				Expect(err).ToNot(HaveOccurred())
				Expect(heartbeat).To(BeEmpty())
			})

			It("should update VM status at all times", func() {
				vmName := "test-vm-status"
				imageName := "test-item"
				vmConfigArgs := getVmConfigArgs(testNamespace, vmName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)
				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn

				// Validate vm status update for the success case
				Expect(session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)).To(Succeed())
				Expect(vm.Status.PowerState).Should(Equal(vmopv1alpha1.VirtualMachinePoweredOn))
			})

			It("should clone VM with storage policy disk provisioning", func() {
				imageName := "test-item"
				vmName := "CL_DeployedVM-via-policy"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				// hardwired vcsim ID for "vSAN Default Storage Policy".
				// TODO: this test could lookup profile id by name or create a new profile
				vmConfigArgs.StorageProfileID = "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				// XXX clonedDeployedVM is used in another test
				var err error
				clonedDeployedVM, err = session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedDeployedVM.Name).Should(Equal(vmName))
			})

			It("should clone VM with VM volume disk thin provisioning option", func() {
				imageName := "test-item"
				vmName := "CL_DeployedVM-via-policy-thin-provisioned"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				// hardwired vcsim ID for "vSAN Default Storage Policy" - this is to show we explicitly use
				// the spec volume provisioning if the user specifies it
				vmConfigArgs.StorageProfileID = "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)
				thinProvisioned := true
				volOptions := &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
					ThinProvisioned: &thinProvisioned,
				}
				vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: volOptions,
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
				/* TODO verify volume properties */
			})

			It("should clone VM with VM volume disk thick provisioning option", func() {
				imageName := "test-item"
				vmName := "CL_DeployedVM-via-policy-thick-provisioned"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				// hardwired vcsim ID for "vSAN Default Storage Policy" - this is to show we explicitly use
				// the spec volume provisioning if the user specifies it
				vmConfigArgs.StorageProfileID = "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)
				thinProvisioned := false
				volOptions := &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
					ThinProvisioned: &thinProvisioned,
				}
				vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: volOptions,
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
				/* TODO verify volume properties */
			})

			It("should clone VM with VM volume disk eager zeroed option", func() {
				imageName := "test-item"
				vmName := "CL_DeployedVM-via-policy-eager-zeroed"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				// hardwired vcsim ID for "vSAN Default Storage Policy" - this is to show we explicitly use
				// the spec volume provisioning if the user specifies it
				vmConfigArgs.StorageProfileID = "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)
				eagerZeroed := true
				volOptions := &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
					EagerZeroed: &eagerZeroed,
				}
				vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: volOptions,
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
				/* TODO verify volume properties */
			})

			It("should clone VM with resized disk", func() {
				imageName := "test-item"
				vmName := "CL_DeployedVM-resized"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				virtualDisks, err := clonedVM.GetVirtualDisks(goctx.TODO())
				Expect(err).NotTo(HaveOccurred())
				Expect(virtualDisks).Should(HaveLen(1))

				// The device type should be virtual disk
				disk1, ok := virtualDisks[0].(*vimTypes.VirtualDisk)
				Expect(ok).Should(BeTrue())
				disk1Key := int(disk1.Key)
				desiredSize := resource.MustParse(fmt.Sprintf("%d", disk1.CapacityInBytes))
				// Make new size 1MB larger
				desiredSize.Add(resource.MustParse("1Mi"))

				vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
					{
						Name: "resized-root-disk",
						VsphereVolume: &vmopv1alpha1.VsphereVolumeSource{
							Capacity: corev1.ResourceList{
								corev1.ResourceEphemeralStorage: desiredSize,
							},
							DeviceKey: &disk1Key,
						},
					},
				}

				// UpdateVirtualMachine calls a Reconfigure on the cloned VM with a config spec
				// with the updated desired size of the VM disks
				err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())

				clonedVMDisks, err := clonedVM.GetVirtualDisks(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVMDisks).Should(HaveLen(1))

				// The device type should be virtual disk
				clonedVMDisk1, ok := clonedVMDisks[0].(*vimTypes.VirtualDisk)
				Expect(ok).Should(BeTrue())
				Expect(clonedVMDisk1.Key).Should(Equal(disk1.Key))
				// Check that cloned VM disk is the desired size
				resultingSize := resource.MustParse(fmt.Sprintf("%d", clonedVMDisk1.CapacityInBytes))
				Expect(resultingSize.Value()).Should(Equal(desiredSize.Value()))
			})

			It("should clone VMTX", func() {
				imageName := "test-item-vmtx"
				vmName := "CL_DeployedVMTX"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				// Expect this attempt to fail as we've not yet created the vm-template CL item
				_, err = session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("no library item named: %s", imageName)))

				// Create the vm-template CL item
				err := integration.CloneVirtualMachineToLibraryItem(ctx, vSphereConfig, session, "DC0_H0_VM0", imageName)
				Expect(err).NotTo(HaveOccurred())

				// Now expect clone to succeed
				clonedDeployedVMTX, err = session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedDeployedVMTX.Name).Should(Equal(vmName))
			})

			// This depends on the "should clone VMTX" test.
			It("should clone VMTX with eager zeroed", func() {
				imageName := "test-item-vmtx"
				vmName := "CL_DeployedVMTX-eager-zeroed"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				eagerZeroed := true
				volOptions := &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
					EagerZeroed: &eagerZeroed,
				}
				vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: volOptions,
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
				// The device type should be virtual disk
				clonedVMDisks, err := clonedVM.GetVirtualDisks(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVMDisks).Should(HaveLen(1))

				clonedVMDisk1, ok := clonedVMDisks[0].(*vimTypes.VirtualDisk)
				Expect(ok).Should(BeTrue())
				_, ok = clonedVMDisk1.Backing.(*vimTypes.VirtualDiskFlatVer2BackingInfo)
				Expect(ok).Should(BeTrue())

				/* - vcsim does not seem to support this (dramdass)
				// Check that cloned VM disk is eager zeroed
				Expect(diskBacking.EagerlyScrub).ShouldNot(BeNil())
				Expect(*diskBacking.EagerlyScrub).Should(BeTrue())
				*/
			})

			// This depends on the "should clone VMTX" test.
			It("should clone VMTX with thin provisioned", func() {
				imageName := "test-item-vmtx"
				vmName := "CL_DeployedVMTX-thin-provisioned"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				thinProvisioned := true
				volOptions := &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
					ThinProvisioned: &thinProvisioned,
				}
				vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: volOptions,
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				clonedVMDisks, err := clonedVM.GetVirtualDisks(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVMDisks).Should(HaveLen(1))

				// The device type should be virtual disk
				clonedVMDisk1, ok := clonedVMDisks[0].(*vimTypes.VirtualDisk)
				Expect(ok).Should(BeTrue())
				_, ok = clonedVMDisk1.Backing.(*vimTypes.VirtualDiskFlatVer2BackingInfo)
				Expect(ok).Should(BeTrue())

				/* - vcsim does not seem to support this (dramdass)
				// Check that cloned VM disk is thin provisioned
				Expect(diskBacking.ThinProvisioned).ShouldNot(BeNil())
				Expect(*diskBacking.ThinProvisioned).Should(BeTrue())
				*/
			})

			// This depends on the "should clone VMTX" test.
			It("should clone VMTX with resized disk", func() {
				imageName := "test-item-vmtx"
				vmName := "CL_DeployedVMTX-resized"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				virtualDisks, err := clonedDeployedVMTX.GetVirtualDisks(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(virtualDisks).Should(HaveLen(1))

				// The device type should be virtual disk
				disk1, ok := virtualDisks[0].(*vimTypes.VirtualDisk)
				Expect(ok).Should(BeTrue())
				disk1Key := int(disk1.Key)
				desiredSize := resource.MustParse(fmt.Sprintf("%d", disk1.CapacityInBytes))
				// Make new size 1MB larger
				desiredSize.Add(resource.MustParse("1Mi"))

				vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
					{
						Name: "resized-root-disk",
						VsphereVolume: &vmopv1alpha1.VsphereVolumeSource{
							Capacity: corev1.ResourceList{
								corev1.ResourceEphemeralStorage: desiredSize,
							},
							DeviceKey: &disk1Key,
						},
					},
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				clonedVMDisks, err := clonedVM.GetVirtualDisks(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVMDisks).Should(HaveLen(1))

				// The device type should be virtual disk
				clonedVMDisk1, ok := clonedVMDisks[0].(*vimTypes.VirtualDisk)
				Expect(ok).Should(BeTrue())
				Expect(clonedVMDisk1.Key).Should(Equal(disk1.Key))
				// Check that cloned VM disk is the desired size
				resultingSize := resource.MustParse(fmt.Sprintf("%d", clonedVMDisk1.CapacityInBytes))
				Expect(resultingSize.Value()).Should(Equal(desiredSize.Value()))
			})

			It("should power on vm with attached cns volume", func() {
				imageName := "test-item"
				vmName := "CL_PowerON_DeployedVM_Attached_PVC"
				cnsVolumeName := "cns-volume-1"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
					{
						Name: cnsVolumeName,
						PersistentVolumeClaim: &vmopv1alpha1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-volume-1",
							},
						},
					},
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
				vm.Status.Volumes = []vmopv1alpha1.VirtualMachineVolumeStatus{
					{
						Name:     cnsVolumeName,
						Attached: true,
					},
				}
				Expect(session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)).To(Succeed())
				Expect(vm.Status.PowerState).To(Equal(vmopv1alpha1.VirtualMachinePoweredOn))
			})

			It("should not power on vm when specified persistent volume is not attached", func() {
				imageName := "test-item"
				vmName := "CL_PowerON_DeployedVM_PVC_notAttached"
				cnsVolumeName := "cns-volume-1"
				dummyError := "dummy error"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
					{
						Name: cnsVolumeName,
						PersistentVolumeClaim: &vmopv1alpha1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-volume-1",
							},
						},
					},
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
				vm.Status.Volumes = []vmopv1alpha1.VirtualMachineVolumeStatus{
					{
						Name:     cnsVolumeName,
						Attached: false,
						Error:    dummyError,
					},
				}
				err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("persistent volume: %s not attached to VM", cnsVolumeName)))
				Expect(vm.Status.PowerState).ToNot(Equal(vmopv1alpha1.VirtualMachinePoweredOn))
			})

			It("should not power on vm when specified persistent volume's status update is pending", func() {
				imageName := "test-item"
				vmName := "CL_PowerON_DeployedVM_PVC_Update_Pending"
				cnsVolumeName := "cns-volume-1"

				vmConfigArgs := getVmConfigArgs(testNamespace, testVMName, imageName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmConfigArgs.VMClass.Name)

				vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
					{
						Name: cnsVolumeName,
						PersistentVolumeClaim: &vmopv1alpha1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-volume-1",
							},
						},
					},
				}

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
				err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status update pending for persistent volume: %s on VM", cnsVolumeName)))
				Expect(vm.Status.PowerState).ToNot(Equal(vmopv1alpha1.VirtualMachinePoweredOn))
			})
		})
	})

	Context("Session creation with invalid global extraConfig", func() {
		BeforeEach(func() {
			err = os.Setenv("JSON_EXTRA_CONFIG", "invalid-json")
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err = os.Setenv("JSON_EXTRA_CONFIG", "")
			Expect(err).NotTo(HaveOccurred())
		})
		It("Should fail", func() {
			session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
			Expect(err.Error()).To(MatchRegexp("Unable to parse value of 'JSON_EXTRA_CONFIG' environment variable"))
		})
	})

	Describe("Clone VM with global metadata", func() {
		JustBeforeEach(func() {
			session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
		})

		Context("with vm metadata and global extraConfig", func() {
			const (
				localKey  = "guestinfo.localK"
				localVal  = "localV"
				globalKey = "globalK"
				globalVal = "globalV"
			)

			BeforeEach(func() {
				err = os.Setenv("JSON_EXTRA_CONFIG", "{\""+globalKey+"\":\""+globalVal+"\"}")
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				err = os.Setenv("JSON_EXTRA_CONFIG", "")
				Expect(err).NotTo(HaveOccurred())
			})

			Context("with global extraConfig", func() {
				It("should copy the values into the VM", func() {
					imageName := "DC0_H0_VM0"
					vmClass := getVMClassInstance(testVMName, testNamespace)
					vm := getVirtualMachineInstance(testVMName+"-extraConfig", testNamespace, imageName, vmClass.Name)
					vmImage := builder.DummyVirtualMachineImage(imageName)
					vm.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
						Transport: vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
					}
					vmMetadata := vmprovider.VMMetadata{
						Data:      map[string]string{localKey: localVal},
						Transport: vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
					}
					vmConfigArgs := vmprovider.VMConfigArgs{
						VMClass:          *vmClass,
						VMImage:          vmImage,
						VMMetadata:       vmMetadata,
						StorageProfileID: "aa6d5a82-1c88-45da-85d3-3d74b91a5bad",
					}
					clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).NotTo(HaveOccurred())
					Expect(clonedVM).ShouldNot(BeNil())

					err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).NotTo(HaveOccurred())

					keysFound := map[string]bool{localKey: false, globalKey: false}
					// Add all the default keys
					for k := range vmopsession.DefaultExtraConfig {
						keysFound[k] = false
					}

					mo, err := clonedVM.GetProperties(ctx, nil)
					Expect(err).NotTo(HaveOccurred())
					for _, option := range mo.Config.ExtraConfig {
						key := option.GetOptionValue().Key
						keysFound[key] = true
						if key == localKey {
							Expect(option.GetOptionValue().Value).Should(Equal(localVal))
						} else if key == globalKey {
							Expect(option.GetOptionValue().Value).Should(Equal(globalVal))
						} else if defaultVal, ok := vmopsession.DefaultExtraConfig[key]; ok {
							Expect(option.GetOptionValue().Value).Should(Equal(defaultVal))
						}
					}
					for k, v := range keysFound {
						Expect(v).Should(BeTrue(), "Key %v not found in VM", k)
					}
				})
			})

			Context("without vm metadata or global extraConfig", func() {
				It("should copy the default values into the VM", func() {
					imageName := "DC0_H0_VM0"
					vmClass := getVMClassInstance(testVMName, testNamespace)
					vm := getVirtualMachineInstance(testVMName+"-default-extraConfig", testNamespace, imageName, vmClass.Name)
					vmImage := builder.DummyVirtualMachineImage(imageName)
					vmConfigArgs := vmprovider.VMConfigArgs{
						VMClass:          *vmClass,
						VMImage:          vmImage,
						StorageProfileID: "aa6d5a82-1c88-45da-85d3-3d74b91a5bad",
					}
					clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).NotTo(HaveOccurred())
					Expect(clonedVM).ShouldNot(BeNil())

					err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).NotTo(HaveOccurred())

					keysFound := map[string]bool{}
					// Add all the default keys
					for k := range vmopsession.DefaultExtraConfig {
						keysFound[k] = false
					}
					mo, err := clonedVM.GetProperties(ctx, nil)
					Expect(err).NotTo(HaveOccurred())
					for _, option := range mo.Config.ExtraConfig {
						key := option.GetOptionValue().Key
						keysFound[key] = true
						if defaultVal, ok := vmopsession.DefaultExtraConfig[key]; ok {
							Expect(option.GetOptionValue().Value).Should(Equal(defaultVal))
						}
					}
					for k, v := range keysFound {
						Expect(v).Should(BeTrue(), "Key %v not found in VM", k)
					}
				})
			})
		})

		Describe("Resource Pool", func() {
			var rpName string
			var rpSpec *vmopv1alpha1.ResourcePoolSpec

			BeforeEach(func() {
				rpName = "test-folder"
				rpSpec = &vmopv1alpha1.ResourcePoolSpec{
					Name: rpName,
				}
				rpMoId, err := session.CreateResourcePool(ctx, rpSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(rpMoId).ToNot(BeEmpty())
			})

			AfterEach(func() {
				// RP would already be deleted after the deletion test. But DeleteResourcePool handles delete of an RP if it's already deleted.
				Expect(session.DeleteResourcePool(ctx, rpSpec.Name)).To(Succeed())
			})

			Context("Create a ResourcePool, verify it exists and delete it", func() {
				It("Verifies if a ResourcePool exists", func() {
					exists, err := session.DoesResourcePoolExist(ctx, rpSpec.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue())
				})
			})

			Context("Create a ResourcePool, rename the VC cluster, verify that resource pool is still found", func() {
				var defaultVCSimCluster string

				BeforeEach(func() {
					defaultVCSimCluster = simulator.Map.Any("ClusterComputeResource").Entity().Name
					newClusterName := "newCluster"
					Expect(session.RenameSessionCluster(ctx, newClusterName)).To(Succeed())
				})

				AfterEach(func() {
					Expect(session.RenameSessionCluster(ctx, defaultVCSimCluster)).To(Succeed())
				})

				It("Verify that the resource pool exists", func() {
					exists, err := session.DoesResourcePoolExist(ctx, rpSpec.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue())
				})
			})

			Context("Create two resource pools with the duplicate names", func() {
				It("second resource pool should fail to create", func() {
					// Try to create another ResourcePool with the same spec.
					rpMoId, err := session.CreateResourcePool(ctx, rpSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("ServerFaultCode: DuplicateName"))
					Expect(rpMoId).To(BeEmpty())
				})
			})

			Context("ChildResourcePool", func() {
				It("returns NotFoundError for a resource pool that doesn't exist", func() {
					_, err := session.ChildResourcePool(ctx, "nonExistentResourcePool")
					Expect(err).To(HaveOccurred())
					_, ok := err.(*find.NotFoundError)
					Expect(ok).Should(BeTrue())
				})
			})

			Context("Delete a Resource Pool that doesn't exist", func() {
				It("should succeed", func() {
					Expect(session.DeleteResourcePool(ctx, "nonexistent-resourcepool")).To(Succeed())
				})
			})

			Context("Resource Pool as moID", func() {
				It("returns Resource Pool object without error", func() {
					pools, err := session.Finder.ResourcePoolList(ctx, "*")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(pools).ToNot(BeEmpty())

					existingPool := pools[0]
					pool, err := session.GetResourcePoolByMoID(ctx, existingPool.Reference().Value)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(pool.InventoryPath).To(Equal(existingPool.InventoryPath))
					Expect(pool.Reference().Value).To(Equal(existingPool.Reference().Value))
				})
			})
		})

		Describe("Clone VM gracefully fails", func() {
			Context("Should fail gracefully", func() {
				var savedDatastoreAttribute string
				var err error
				imageName := "test-item"
				vmImage := builder.DummyVirtualMachineImage(imageName)

				vm := &vmopv1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "TestVM",
					},
				}

				BeforeEach(func() {
					savedDatastoreAttribute = vSphereConfig.Datastore
				})

				AfterEach(func() {
					vSphereConfig.Datastore = savedDatastoreAttribute
					vSphereConfig.StorageClassRequired = false
				})

				It("with existing content source, empty datastore and empty profile id", func() {
					vSphereConfig.Datastore = ""

					session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
					Expect(err).NotTo(HaveOccurred())

					vmConfigArgs := vmprovider.VMConfigArgs{
						VMImage:            vmImage,
						ContentLibraryUUID: integration.GetContentSourceID(),
					}
					clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("cannot clone VM when neither storage class or datastore is specified"))
					Expect(clonedVM).Should(BeNil())
				})

				It("with existing content source but mandatory profile id is not set", func() {
					vSphereConfig.StorageClassRequired = true
					session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
					Expect(err).NotTo(HaveOccurred())

					vmConfigArgs := vmprovider.VMConfigArgs{
						VMImage:            vmImage,
						ContentLibraryUUID: integration.GetContentSourceID(),
					}
					clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("storage class is required but not specified"))
					Expect(clonedVM).Should(BeNil())
				})

				It("without content source and missing mandatory profile ID", func() {
					vSphereConfig.StorageClassRequired = true
					session, err = vmopsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
					Expect(err).NotTo(HaveOccurred())

					vmConfigArgs := vmprovider.VMConfigArgs{
						VMImage:            vmImage,
						ContentLibraryUUID: integration.GetContentSourceID(),
					}
					clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("storage class is required but not specified"))
					Expect(clonedVM).Should(BeNil())
				})
			})
		})
	})

	Describe("vSphere Tags", func() {
		var resVM *resources.VirtualMachine
		tagCatName := "tag-category-name"
		tagName := "tag-name"

		BeforeEach(func() {
			resVM, err = session.GetVirtualMachine(vmContext(ctx, getSimpleVirtualMachine("DC0_H0_VM0")))
			Expect(err).NotTo(HaveOccurred())
			Expect(resVM).NotTo(BeNil())

			manager := tags.NewManager(session.Client.RestClient())

			// Create a tag category and a tag
			cat := tags.Category{
				Name:            tagCatName,
				Description:     "test-description",
				Cardinality:     "SINGLE",
				AssociableTypes: []string{"VirtualMachine"},
			}
			catId, err := manager.CreateCategory(ctx, &cat)
			Expect(err).NotTo(HaveOccurred())
			Expect(catId).NotTo(BeEmpty())

			tag := tags.Tag{
				Name:        tagName,
				Description: "test-description",
				CategoryID:  catId,
			}
			tagId, err := manager.CreateTag(ctx, &tag)
			Expect(err).NotTo(HaveOccurred())
			Expect(tagId).NotTo(BeEmpty())
		})

		Context("Attach a tag to a VM", func() {
			It("Attach/Detach", func() {
				err = session.AttachTagToVM(ctx, tagName, tagCatName, resVM.MoRef())
				Expect(err).NotTo(HaveOccurred())

				err = session.DetachTagFromVM(ctx, tagName, tagCatName, resVM.MoRef())
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Attach tags and modules to a VM", func() {
		namespace := integration.DefaultNamespace
		vmName := "getvm-with-rp-and-without-moid"
		imageName := "test-item"
		var vm *vmopv1alpha1.VirtualMachine
		var vmConfigArgs vmprovider.VMConfigArgs
		badClusterModuleName := "badClusterModuleName"
		badProviderTagsName := "badProviderTagsName"
		var resourcePolicy *vmopv1alpha1.VirtualMachineSetResourcePolicy

		BeforeEach(func() {
			resourcePolicy = getVirtualMachineSetResourcePolicy(vmName, namespace)
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
			vmConfigArgs = getVmConfigArgs(namespace, vmName, imageName)
			vmConfigArgs.ResourcePolicy = resourcePolicy
			vm = getVirtualMachineInstance(vmName, namespace, imageName, vmConfigArgs.VMClass.Name)
			vm.Spec.ResourcePolicyName = resourcePolicy.Name

			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}
		})

		Context("With non-exist clusterModule", func() {
			It("Should fail", func() {
				Expect(k8sClient.Create(ctx, resourcePolicy)).To(Succeed())

				clonedVM, err := session.CloneVirtualMachine(vmContext(ctx, vm), vmConfigArgs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))

				vm.Annotations[pkg.ClusterModuleNameKey] = badClusterModuleName
				vm.Annotations[pkg.ProviderTagsAnnotationKey] = badProviderTagsName
				err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("ClusterModule %s not found", badClusterModuleName)))
			})
		})

		// This depends on the prior test.
		Context("With empty tagName", func() {
			It("Should fail", func() {
				vm.Annotations[pkg.ClusterModuleNameKey] = "ControlPlane"
				vm.Annotations[pkg.ProviderTagsAnnotationKey] = badProviderTagsName
				err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("empty tagName, TagInfo %s not found", badProviderTagsName)))
			})
		})
	})
})
