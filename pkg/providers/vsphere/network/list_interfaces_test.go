// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"fmt"

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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	netsetutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/networksettings"
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

	DescribeTableSubtree("Network Provider Type",
		func(providerType pkgcfg.NetworkProviderType) {
			var obj ctrlclient.Object

			createInterfaceCr := func(name string, vm *vmopv1.VirtualMachine, makeOwner bool) ctrlclient.Object {
				switch providerType {
				case pkgcfg.NetworkProviderTypeVDS:
					obj = &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: vm.Namespace,
						},
					}
				case pkgcfg.NetworkProviderTypeNSXT:
					obj = &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: vm.Namespace,
						},
					}
				case pkgcfg.NetworkProviderTypeVPC:
					obj = &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: vm.Namespace,
						},
					}
				default:
					Fail(fmt.Sprintf("Unknown network provider type: %v", providerType))
				}

				obj.SetLabels(map[string]string{network.VMNameLabel: vm.Name})
				if makeOwner {
					Expect(ctrlutil.SetOwnerReference(vm, obj, ctx.Client.Scheme())).To(Succeed())
				}

				return obj
			}

			BeforeEach(func() {
				testConfig.WithNetworkConfig = []builder.VCSimNetworkConfig{
					{
						Provider: providerType,
					},
				}
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
							ObjectProviderType: providerType,
							ObjectName:         ownedNetIf2.GetName(),
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(HaveLen(1))
					Expect(results.OrphanedNetworkInterfaces[0].GetName()).To(Equal(ownedNetIf1.GetName()))
				})
			})
		},
		Entry("VDS", pkgcfg.NetworkProviderTypeVDS),
		Entry("NSX-T", pkgcfg.NetworkProviderTypeNSXT),
		Entry("VPC", pkgcfg.NetworkProviderTypeVPC),
	)
})

var _ = Describe("ListNetworkInterfaces", func() {

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim

		vmCtx pkgctx.VirtualMachineContext
		vm    *vmopv1.VirtualMachine

		initObjects []ctrlclient.Object
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "list-net-if-test-vm",
				Namespace: "list-net-if-test-ns",
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

	newNetworkSettings := func(provider, legacyProvider netopv1alpha1.NetworkProvider) *netopv1alpha1.NetworkSettings {
		return &netopv1alpha1.NetworkSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: vm.Namespace,
			},
			Provider:       provider,
			LegacyProvider: legacyProvider,
		}
	}

	makeNetOPIf := func(name string, makeOwner bool) *netopv1alpha1.NetworkInterface {
		obj := &netopv1alpha1.NetworkInterface{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: vm.Namespace,
				Labels:    map[string]string{network.VMNameLabel: vm.Name},
			},
		}
		if makeOwner {
			Expect(ctrlutil.SetOwnerReference(vm, obj, ctx.Client.Scheme())).To(Succeed())
		}
		return obj
	}

	makeNCPIf := func(name string, makeOwner bool) *ncpv1alpha1.VirtualNetworkInterface {
		obj := &ncpv1alpha1.VirtualNetworkInterface{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: vm.Namespace,
				Labels:    map[string]string{network.VMNameLabel: vm.Name},
			},
		}
		if makeOwner {
			Expect(ctrlutil.SetOwnerReference(vm, obj, ctx.Client.Scheme())).To(Succeed())
		}
		return obj
	}

	makeVPCIf := func(name string, makeOwner bool) *vpcv1alpha1.SubnetPort {
		obj := &vpcv1alpha1.SubnetPort{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: vm.Namespace,
				Labels:    map[string]string{network.VMNameLabel: vm.Name},
			},
		}
		if makeOwner {
			Expect(ctrlutil.SetOwnerReference(vm, obj, ctx.Client.Scheme())).To(Succeed())
		}
		return obj
	}

	Context("PerNamespaceNetworkProvider", func() {

		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(cfg *pkgcfg.Config) {
				cfg.Features.PerNamespaceNetworkProvider = true
			})
			// Re-wrap so vmCtx picks up the updated feature flag.
			vmCtx = pkgctx.VirtualMachineContext{
				Context: ctx,
				Logger:  suite.GetLogger().WithName("list_interfaces_test"),
				VM:      vm,
			}
		})

		It("returns an error when no NetworkSettings/default exists", func() {
			_, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(netsetutil.ErrNetworkSettingsNotFound))
		})

		Context("with VDS NetworkSettings", func() {

			JustBeforeEach(func() {
				ns := newNetworkSettings(netopv1alpha1.NetworkProviderVSphereDistributed, "")
				Expect(ctx.Client.Create(ctx, ns)).To(Succeed())
			})

			It("returns empty list when no interface CRs exist", func() {
				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(BeEmpty())
			})

			It("lists owned NetOP CRs", func() {
				owned := makeNetOPIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, owned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(owned.GetName()))
			})

			It("excludes unowned NetOP CRs", func() {
				owned := makeNetOPIf(vm.Name+"-owned", true)
				Expect(ctx.Client.Create(ctx, owned)).To(Succeed())
				unowned := makeNetOPIf(vm.Name+"-unowned", false)
				Expect(ctx.Client.Create(ctx, unowned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(owned.GetName()))
			})

			Context("ListOrphanedNetworkInterfaces", func() {

				It("does not confuse an active result from a different provider with a same-named VDS CR", func() {
					// VPC CR "foo" exists in the cluster (leftover from a migration).
					vpcIf := makeVPCIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, vpcIf)).To(Succeed())

					// NetOP CR "foo" is the currently active one.
					netopIf := makeNetOPIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, netopIf)).To(Succeed())

					// Only the NetOP CR is in the active results.  The ignore key is
					// "VSPHERE_NETWORK/foo", so "NSXT_VPC/foo" is still orphaned.
					results := network.NetworkInterfaceResults{
						Results: []network.NetworkInterfaceResult{
							{
								ObjectName:         netopIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeVDS,
							},
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(BeEmpty())
				})
			})
		})

		Context("with NSXT NetworkSettings", func() {

			JustBeforeEach(func() {
				ns := newNetworkSettings(netopv1alpha1.NetworkProviderNSXTier1, "")
				Expect(ctx.Client.Create(ctx, ns)).To(Succeed())
			})

			It("returns empty list when no interface CRs exist", func() {
				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(BeEmpty())
			})

			It("lists owned NCP VirtualNetworkInterface CRs", func() {
				owned := makeNCPIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, owned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(owned.GetName()))
			})

			It("excludes unowned NCP VirtualNetworkInterface CRs", func() {
				owned := makeNCPIf(vm.Name+"-owned", true)
				Expect(ctx.Client.Create(ctx, owned)).To(Succeed())
				unowned := makeNCPIf(vm.Name+"-unowned", false)
				Expect(ctx.Client.Create(ctx, unowned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(owned.GetName()))
			})

			It("does not list NetOP or VPC CRs even if they exist", func() {
				ncpOwned := makeNCPIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, ncpOwned)).To(Succeed())
				// A NetOP CR with the same VM-name label should be invisible.
				netopIf := makeNetOPIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, netopIf)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(ncpOwned.GetName()))
			})

			Context("ListOrphanedNetworkInterfaces", func() {

				It("same-named NetOP CR from previous provider is invisible when not tracked as a legacy provider", func() {
					// A leftover NetOP CR with the same name as the active NCP CR.
					netopIf := makeNetOPIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, netopIf)).To(Succeed())

					// Active NCP CR.
					ncpIf := makeNCPIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, ncpIf)).To(Succeed())

					results := network.NetworkInterfaceResults{
						Results: []network.NetworkInterfaceResult{
							{
								ObjectName:         ncpIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeNSXT,
							},
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(BeEmpty())
				})
			})
		})

		Context("with VPC NetworkSettings", func() {

			JustBeforeEach(func() {
				ns := newNetworkSettings(netopv1alpha1.NetworkProviderVPC, "")
				Expect(ctx.Client.Create(ctx, ns)).To(Succeed())
			})

			It("returns empty list when no interface CRs exist", func() {
				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(BeEmpty())
			})

			It("lists owned SubnetPort CRs", func() {
				owned := makeVPCIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, owned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(owned.GetName()))
			})

			It("excludes unowned SubnetPort CRs", func() {
				owned := makeVPCIf(vm.Name+"-owned", true)
				Expect(ctx.Client.Create(ctx, owned)).To(Succeed())
				unowned := makeVPCIf(vm.Name+"-unowned", false)
				Expect(ctx.Client.Create(ctx, unowned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(owned.GetName()))
			})

			It("does not list NetOP or NCP CRs even if they exist", func() {
				vpcOwned := makeVPCIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, vpcOwned)).To(Succeed())
				// A NetOP CR with the same VM-name label should be invisible.
				netopIf := makeNetOPIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, netopIf)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(vpcOwned.GetName()))
			})

			Context("ListOrphanedNetworkInterfaces", func() {

				It("same-named NCP CR from previous provider is invisible when not tracked as a legacy provider", func() {
					// A leftover NCP CR with the same name as the active VPC CR.
					ncpIf := makeNCPIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, ncpIf)).To(Succeed())

					// Active VPC CR.
					vpcIf := makeVPCIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, vpcIf)).To(Succeed())

					results := network.NetworkInterfaceResults{
						Results: []network.NetworkInterfaceResult{
							{
								ObjectName:         vpcIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeVPC,
							},
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(BeEmpty())
				})
			})
		})

		Context("with VPC NetworkSettings and a legacy VDS provider", func() {

			JustBeforeEach(func() {
				ns := newNetworkSettings(netopv1alpha1.NetworkProviderVPC, netopv1alpha1.NetworkProviderVSphereDistributed)
				Expect(ctx.Client.Create(ctx, ns)).To(Succeed())
			})

			It("lists owned CRs from both the current and the legacy provider", func() {
				vpcOwned := makeVPCIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, vpcOwned)).To(Succeed())
				netopOwned := makeNetOPIf(vm.Name+"-eth1", true)
				Expect(ctx.Client.Create(ctx, netopOwned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				names := make([]string, 0, len(ifaces))
				for _, obj := range ifaces {
					names = append(names, obj.GetName())
				}
				Expect(names).To(ConsistOf(vpcOwned.GetName(), netopOwned.GetName()))
			})

			It("excludes unowned CRs from either the current or the legacy provider", func() {
				vpcOwned := makeVPCIf(vm.Name+"-owned", true)
				Expect(ctx.Client.Create(ctx, vpcOwned)).To(Succeed())
				vpcUnowned := makeVPCIf(vm.Name+"-vpc-unowned", false)
				Expect(ctx.Client.Create(ctx, vpcUnowned)).To(Succeed())
				netopOwned := makeNetOPIf(vm.Name+"-owned-legacy", true)
				Expect(ctx.Client.Create(ctx, netopOwned)).To(Succeed())
				netopUnowned := makeNetOPIf(vm.Name+"-netop-unowned", false)
				Expect(ctx.Client.Create(ctx, netopUnowned)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				names := make([]string, 0, len(ifaces))
				for _, obj := range ifaces {
					names = append(names, obj.GetName())
				}
				Expect(names).To(ConsistOf(vpcOwned.GetName(), netopOwned.GetName()))
			})

			It("does not list NCP CRs, which are neither the current nor the legacy provider", func() {
				vpcOwned := makeVPCIf(vm.Name+"-eth0", true)
				Expect(ctx.Client.Create(ctx, vpcOwned)).To(Succeed())
				ncpIf := makeNCPIf(vm.Name+"-eth1", true)
				Expect(ctx.Client.Create(ctx, ncpIf)).To(Succeed())

				ifaces, err := network.ListNetworkInterfaces(vmCtx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(ifaces).To(HaveLen(1))
				Expect(ifaces[0].GetName()).To(Equal(vpcOwned.GetName()))
			})

			Context("ListOrphanedNetworkInterfaces", func() {

				It("treats an unreferenced legacy-provider CR as orphaned, now that it is visible", func() {
					// Active VPC CR, referenced in the current results.
					vpcIf := makeVPCIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, vpcIf)).To(Succeed())
					// Leftover NetOP CR from before the migration to VPC; it is
					// no longer referenced by any result.
					netopIf := makeNetOPIf(vm.Name+"-eth0-legacy", true)
					Expect(ctx.Client.Create(ctx, netopIf)).To(Succeed())

					results := network.NetworkInterfaceResults{
						Results: []network.NetworkInterfaceResult{
							{
								ObjectName:         vpcIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeVPC,
							},
						},
					}

					// Both the current (VPC) and legacy (VDS) providers are
					// now listed, so the leftover NetOP CR is visible and
					// correctly reported as orphaned.
					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(HaveLen(1))
					Expect(results.OrphanedNetworkInterfaces[0].GetName()).To(Equal(netopIf.GetName()))
				})

				It("does not orphan a legacy-provider CR that is still referenced in the results", func() {
					vpcIf := makeVPCIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, vpcIf)).To(Succeed())
					netopIf := makeNetOPIf(vm.Name+"-eth1", true)
					Expect(ctx.Client.Create(ctx, netopIf)).To(Succeed())

					results := network.NetworkInterfaceResults{
						Results: []network.NetworkInterfaceResult{
							{
								ObjectName:         vpcIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeVPC,
							},
							{
								ObjectName:         netopIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeVDS,
							},
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(BeEmpty())
				})

				It("does not list NCP CRs, which are neither the current nor the legacy provider", func() {
					vpcIf := makeVPCIf(vm.Name+"-eth0", true)
					Expect(ctx.Client.Create(ctx, vpcIf)).To(Succeed())
					ncpIf := makeNCPIf(vm.Name+"-eth1", true)
					Expect(ctx.Client.Create(ctx, ncpIf)).To(Succeed())

					results := network.NetworkInterfaceResults{
						Results: []network.NetworkInterfaceResult{
							{
								ObjectName:         vpcIf.GetName(),
								ObjectProviderType: pkgcfg.NetworkProviderTypeVPC,
							},
						},
					}

					err := network.ListOrphanedNetworkInterfaces(vmCtx, ctx.Client, &results)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.OrphanedNetworkInterfaces).To(BeEmpty())
				})
			})
		})
	})
})
