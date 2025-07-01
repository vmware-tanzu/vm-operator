// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("ListOrphanedNetworkInterfaces", func() {

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim

		vmCtx pkgctx.VirtualMachineContext
		vm    *vmopv1.VirtualMachine

		results     network.NetworkInterfaceResults
		initObjects []ctrlclient.Object
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		results = network.NetworkInterfaceResults{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "list-interfaces-test-vm",
				Namespace: "list-interfaces-test-ns",
				UID:       types.UID(uuid.NewString()),
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithName("list_interfaces_test"),
			VM:      vm,
		}
	})

	AfterEach(func() {
		initObjects = nil
	})

	DescribeTableSubtree("Network Env",
		func(networkEnv builder.NetworkEnv) {
			var obj ctrlclient.Object

			createInterfaceCr := func(name string, vm *vmopv1.VirtualMachine, makeOwner bool) ctrlclient.Object {
				switch networkEnv {
				case builder.NetworkEnvVDS:
					obj = &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: vm.Namespace,
						},
					}
				case builder.NetworkEnvNSXT:
					obj = &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: vm.Namespace,
						},
					}
				case builder.NetworkEnvVPC:
					obj = &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: vm.Namespace,
						},
					}
				default:
					Expect(false).To(BeTrue())
				}

				obj.SetLabels(map[string]string{network.VMNameLabel: vm.Name})
				if makeOwner {
					Expect(ctrlutil.SetOwnerReference(vm, obj, ctx.Client.Scheme())).To(Succeed())
				}

				return obj
			}

			BeforeEach(func() {
				testConfig.WithNetworkEnv = networkEnv
			})

			It("Returns orphaned owned interface CR", func() {
				ownedNetIf := createInterfaceCr(vm.Name+"-owned", vm, true)
				Expect(ctx.Client.Create(ctx, ownedNetIf)).To(Succeed())
				unownedNetIf := createInterfaceCr(vm.Name+"-unowned", vm, false)
				Expect(ctx.Client.Create(ctx, unownedNetIf)).To(Succeed())

				err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
				Expect(err).ToNot(HaveOccurred())
				Expect(results.OrphanedNetworkInterfaces).To(HaveLen(1))
				Expect(results.OrphanedNetworkInterfaces[0].GetName()).To(Equal(ownedNetIf.GetName()))
			})

			Context("Multiple owned interfaces", func() {

				It("Returns just orphaned interface", func() {
					ownedNetIf1 := createInterfaceCr(vm.Name+"-owned1", vm, true)
					Expect(ctx.Client.Create(ctx, ownedNetIf1)).To(Succeed())
					ownedNetIf2 := createInterfaceCr(vm.Name+"-owned2", vm, true)
					Expect(ctx.Client.Create(ctx, ownedNetIf2)).To(Succeed())
					unownedNetIf := createInterfaceCr(vm.Name+"-unowned", vm, false)
					Expect(ctx.Client.Create(ctx, unownedNetIf)).To(Succeed())

					results.Results = []network.NetworkInterfaceResult{
						{
							ObjectName: ownedNetIf2.GetName(),
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(HaveLen(1))
					Expect(results.OrphanedNetworkInterfaces[0].GetName()).To(Equal(ownedNetIf1.GetName()))
				})
			})
		},
		Entry("VDS", builder.NetworkEnvVDS),
		Entry("NSX-T", builder.NetworkEnvNSXT),
		Entry("VPC", builder.NetworkEnvVPC),
	)
})
