// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	vmopsession "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/test/integration"
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

	Describe("Attach cluster modules to a VM", func() {
		namespace := integration.DefaultNamespace
		vmName := "getvm-with-rp-and-without-moid"
		imageName := "test-item"
		var vm *vmopv1alpha1.VirtualMachine
		var vmConfigArgs vmprovider.VMConfigArgs
		badClusterModuleName := "badClusterModuleName"
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
				err = session.UpdateVirtualMachine(vmContext(ctx, vm), vmConfigArgs)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("ClusterModule %s not found", badClusterModuleName)))
			})
		})
	})
})
