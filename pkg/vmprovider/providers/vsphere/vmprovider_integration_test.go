// +build integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

var _ = Describe("VMProvider Tests", func() {
	Context("Creating a VM via vmprovier", func() {
		It("should correctly update VirtualMachineStatus", func() {
			testNamespace := "test-namespace-vmp"
			testVMName := "test-vm-vmp"

			// Create a new VMProvder from the config provided by the test
			vmProvider, err := vsphere.NewVSphereVmProviderFromConfig(testNamespace, config)
			Expect(err).NotTo(HaveOccurred())

			// Instruction to vcsim to give the VM an IP address, otherwise CreateVirtualMachine fails
			testIP := "10.0.0.1"
			vmMetadata := map[string]string{"SET.guest.ipAddress": testIP}
			imageName := "" // create, not clone
			vmClass := getVMClassInstance(testVMName, testNamespace)
			vm := getVirtualMachineInstance(testVMName, testNamespace, imageName, vmClass.Name)
			Expect(vm.Status.BiosUuid).Should(BeEmpty())

			// Note that createVirtualMachine has the side effect of changing the vm input value
			err = vmProvider.CreateVirtualMachine(context.TODO(), vm, *vmClass, vmMetadata, "testProfileID")
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Status.VmIp).Should(Equal(testIP))
			Expect(vm.Status.PowerState).Should(Equal(vmoperatorv1alpha1.VirtualMachinePoweredOn))
			Expect(vm.Status.BiosUuid).ShouldNot(BeEmpty())
		})
	})
})
