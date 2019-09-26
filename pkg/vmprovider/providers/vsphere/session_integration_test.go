// +build integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	testNamespace = "test-namespace"
	testVMName    = "test-vm"
)

var _ = Describe("Sessions", func() {
	var (
		session *vsphere.Session
	)
	BeforeEach(func() {
		var err error
		//Setup session
		session, err = vsphere.NewSessionAndConfigure(context.TODO(), config, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Query VM images", func() {
		Context("From Inventory - VMs", func() {
			BeforeEach(func() {
				//set source to use VM inventory
				config.ContentSource = ""
				err = session.ConfigureContent(context.TODO(), config.ContentSource)
				Expect(err).NotTo(HaveOccurred())
			})
			// TODO: The default govcsim setups 2 VM's per resource pool however we should create our own fixture for better
			// consistency and avoid failures when govcsim is updated.
			It("should list virtualmachines", func() {
				vms, err := session.ListVirtualMachines(context.TODO(), "*")
				Expect(err).NotTo(HaveOccurred())
				Expect(vms).ShouldNot(BeEmpty())
			})

			It("should get virtualmachine", func() {
				vm, err := session.GetVirtualMachine(context.TODO(), "DC0_H0_VM0")
				Expect(err).NotTo(HaveOccurred())
				Expect(vm.Name).Should(Equal("DC0_H0_VM0"))
			})
		})
		Context("From Content Library", func() {
			BeforeEach(func() {
				//set source to use CL
				config.ContentSource = integration.ContentSourceName
				err = session.ConfigureContent(context.TODO(), config.ContentSource)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should list virtualmachineimages from CL", func() {
				images, err := session.ListVirtualMachineImagesFromCL(context.TODO(), testNamespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(images).ShouldNot(BeEmpty())
				Expect(images[0].ObjectMeta.Name).Should(Equal("test-item"))
				Expect(images[0].Spec.Type).Should(Equal("ovf"))
			})

			It("should get virtualmachineimage from CL", func() {
				image, err := session.GetVirtualMachineImageFromCL(context.TODO(), "test-item", testNamespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(image.ObjectMeta.Name).Should(Equal("test-item"))
				Expect(image.Spec.Type).Should(Equal("ovf"))
			})

			It("should not get virtualmachineimage from CL", func() {
				image, err := session.GetVirtualMachineImageFromCL(context.TODO(), "invalid", testNamespace)
				Expect(err).Should(MatchError("item: invalid is not found in CL"))
				Expect(image).Should(BeNil())
			})
		})
	})

	Describe("Clone VM", func() {
		BeforeEach(func() {
			//set source to use VM inventory
			config.ContentSource = ""
			err = session.ConfigureContent(context.TODO(), config.ContentSource)
			Expect(err).NotTo(HaveOccurred())
		})
		Context("without specifying any networks in VM Spec", func() {
			It("should not override template networks", func() {
				imageName := "DC0_H0_VM0"
				vmClass := getVMClassInstance(testVMName, testNamespace)
				vm := getVirtualMachineInstance(testVMName, testNamespace, imageName, vmClass.Name)
				vmMetadata := map[string]string{}
				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM).ShouldNot(BeNil())
				// Existing NIF should not be changed.
				netDevices, err := clonedVM.GetNetworkDevices(context.TODO())
				Expect(err).NotTo(HaveOccurred())
				Expect(len(netDevices)).Should(Equal(1))
				dev := netDevices[0].GetVirtualDevice()
				// For the vcsim env the source VM is attached to a distributed port group. Hence, the cloned VM
				// should also be attached to the same network.
				_, ok := dev.Backing.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).Should(BeTrue())

			})
		})
		Context("by speciying networks in VM Spec", func() {
			It("should override template networks", func() {
				imageName := "DC0_H0_VM0"
				vmClass := getVMClassInstance(testVMName, testNamespace)
				vm := getVirtualMachineInstance(testVMName+"change-net", testNamespace, imageName, vmClass.Name)
				// Add two network interfaces to the VM and attach to different networks
				vm.Spec.NetworkInterfaces = []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
					{
						NetworkName: "VM Network",
					},
					{
						NetworkName:      "VM Network",
						EthernetCardType: "e1000",
					},
				}
				vmMetadata := map[string]string{}
				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "foo")
				Expect(err).NotTo(HaveOccurred())
				netDevices, err := clonedVM.GetNetworkDevices(context.TODO())
				Expect(err).NotTo(HaveOccurred())
				Expect(len(netDevices)).Should(Equal(2))
				// The interface type should be default vmxnet3
				dev1, ok := netDevices[0].(*vimTypes.VirtualVmxnet3)
				Expect(ok).Should(BeTrue())
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev1.Backing.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).Should(BeTrue())
				// The interface type should be e1000
				dev2, ok := netDevices[1].(*vimTypes.VirtualE1000)
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev2.Backing.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).Should(BeTrue())
			})
		})
		Context("when a default network is specified", func() {
			BeforeEach(func() {
				var err error
				// For the vcsim env the source VM is attached to a distributed port group. Hence, we are using standard
				// vswitch port group.
				config.Network = "VM Network"
				//Setup new session based on the default network
				session, err = vsphere.NewSessionAndConfigure(context.TODO(), config, nil)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should override network from the template", func() {
				imageName := "DC0_H0_VM0"
				vmClass := getVMClassInstance(testVMName, testNamespace)
				vm := getVirtualMachineInstance(testVMName+"with-default-net", testNamespace, imageName, vmClass.Name)
				vmMetadata := map[string]string{}
				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM).ShouldNot(BeNil())
				// Existing NIF should not be changed.
				netDevices, err := clonedVM.GetNetworkDevices(context.TODO())
				Expect(err).NotTo(HaveOccurred())
				Expect(len(netDevices)).Should(Equal(1))
				dev := netDevices[0].GetVirtualDevice()
				// TODO: enhance the test to verify the moref of the network matches the default network.
				_, ok := dev.Backing.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).Should(BeTrue())

			})
			It("should not override networks specified in VM Spec ", func() {
				imageName := "DC0_H0_VM0"
				vmClass := getVMClassInstance(testVMName, testNamespace)
				vm := getVirtualMachineInstance(testVMName+"change-default-net", testNamespace, imageName, vmClass.Name)
				// Add two network interfaces to the VM and attach to different networks
				vm.Spec.NetworkInterfaces = []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
					{
						NetworkName: "DC0_DVPG0",
					},
					{
						NetworkName:      "DC0_DVPG0",
						EthernetCardType: "e1000",
					},
				}
				vmMetadata := map[string]string{}
				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "foo")
				Expect(err).NotTo(HaveOccurred())
				netDevices, err := clonedVM.GetNetworkDevices(context.TODO())
				Expect(err).NotTo(HaveOccurred())
				Expect(len(netDevices)).Should(Equal(2))
				// The interface type should be default vmxnet3
				dev1, ok := netDevices[0].(*vimTypes.VirtualVmxnet3)
				Expect(ok).Should(BeTrue())
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev1.Backing.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).Should(BeTrue())
				// The interface type should be e1000
				dev2, ok := netDevices[1].(*vimTypes.VirtualE1000)
				// TODO: enhance the test to verify the moref of the network matches the name of the network in spec.
				_, ok = dev2.Backing.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).Should(BeTrue())
			})
		})
		Context("from Content-library", func() {
			BeforeEach(func() {
				//set source to use CL
				config.ContentSource = integration.ContentSourceName
				err = session.ConfigureContent(context.TODO(), config.ContentSource)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should clone VM", func() {
				imageName := "test-item"

				vmClass := getVMClassInstance(testVMName, testNamespace)
				vmName := "CL_DeployedVM"
				vm := getVirtualMachineInstance(vmName, testNamespace, imageName, vmClass.Name)

				vmMetadata := map[string]string{}
				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
			})
		})
	})
	Describe("Clone VM with global metadata", func() {
		const (
			localKey  = "localK"
			localVal  = "localV"
			globalKey = "globalK"
			globalVal = "globalV"
		)

		BeforeEach(func() {
			//set source to use VM inventory

			os.Setenv("JSON_EXTRA_CONFIG", "{\""+globalKey+"\":\""+globalVal+"\"}")
			// Create a new session which should pick up the config
			session, err = vsphere.NewSessionAndConfigure(context.TODO(), config, nil)
			config.ContentSource = ""
			err = session.ConfigureContent(context.TODO(), config.ContentSource)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			os.Setenv("JSON_EXTRA_CONFIG", "")
		})
		Context("with global extraConfig", func() {
			It("should copy the values into the VM", func() {
				imageName := "DC0_H0_VM0"
				vmClass := getVMClassInstance(testVMName, testNamespace)
				vm := getVirtualMachineInstance(testVMName+"-extraConfig", testNamespace, imageName, vmClass.Name)
				vm.Spec.VmMetadata.Transport = "ExtraConfig"
				vmMetadata := map[string]string{localKey: localVal}

				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM).ShouldNot(BeNil())

				keysFound := map[string]bool{localKey: false, globalKey: false}
				mo, err := clonedVM.ManagedObject(context.TODO())
				for _, option := range mo.Config.ExtraConfig {
					key := option.GetOptionValue().Key
					keysFound[key] = true
					if key == localKey {
						Expect(option.GetOptionValue().Value).Should(Equal(localVal))
					} else if key == globalKey {
						Expect(option.GetOptionValue().Value).Should(Equal(globalVal))
					}
				}
				for k, v := range keysFound {
					Expect(v).Should(BeTrue(), "Key %v not found in VM", k)
				}
			})
		})
	})
})
