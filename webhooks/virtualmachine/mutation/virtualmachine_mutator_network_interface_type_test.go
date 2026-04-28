// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

var _ = Describe(
	"SetDefaultNetworkInterfaceTypes",
	Label(
		testlabels.API,
		testlabels.Mutation,
		testlabels.Webhook,
	),
	func() {
		var (
			ctx *pkgctx.WebhookRequestContext
		)

		BeforeEach(func() {
			ctx = &pkgctx.WebhookRequestContext{WebhookContext: &pkgctx.WebhookContext{}}
		})

		Context("OnCreate", func() {
			It("should default to VMXNet3 when interface type is empty", func() {
				vm := &vmopv1.VirtualMachine{}
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
				}

				ok, err := mutation.SetDefaultNetworkInterfaceTypesOnCreate(ctx, nil, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue(), "expected mutation to occur")
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})

		Context("OnUpdate", func() {
			It("should preserve existing interface type from old VM", func() {
				newVM := &vmopv1.VirtualMachine{}
				newVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
				}

				oldVM := &vmopv1.VirtualMachine{}
				oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV},
					},
				}

				ok, err := mutation.SetDefaultNetworkInterfaceTypesOnUpdate(ctx, nil, newVM, oldVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue(), "expected mutation to occur")
				Expect(newVM.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			})
		})
	},
)
