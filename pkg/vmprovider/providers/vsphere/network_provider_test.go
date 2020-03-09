// +build !integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package vsphere_test

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/esx"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	dummyObjectName  = "dummy-object"
	dummyNamespace   = "dummy-ns"
	dummyNetworkName = "dummy-network"
	dummyNsxSwitchId = "dummy-opaque-network-id"
	macAddress       = "01-23-45-67-89-AB-CD-EF"
	interfaceId      = "interface-id"
)

var _ = Describe("NetworkProvider", func() {
	var (
		finder *find.Finder
		ns     *object.HostNetworkSystem

		ctx   context.Context
		vmNif *v1alpha1.VirtualMachineNetworkInterface
		vm    *v1alpha1.VirtualMachine

		np vsphere.NetworkProvider

		name      string
		namespace string
	)

	BeforeEach(func() {
		ctx = context.TODO()
		s := simulator.New(simulator.NewServiceInstance(esx.ServiceContent, esx.RootFolder))
		client, err := vim25.NewClient(ctx, s)
		Expect(err).To(BeNil())
		finder = find.NewFinder(client)
		host := object.NewHostSystem(client, esx.HostSystem.Reference())
		ns, err = host.ConfigManager().NetworkSystem(ctx)
		Expect(err).To(BeNil())
		finder.SetDatacenter(object.NewDatacenter(client, esx.Datacenter.Reference()))

		name = dummyObjectName
		namespace = dummyNamespace

		vmNif = &v1alpha1.VirtualMachineNetworkInterface{
			NetworkName: dummyNetworkName,
		}

		vm = &v1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha1.VirtualMachineSpec{
				NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
					*vmNif,
				},
			},
		}
	})

	Context("when using default network provider", func() {
		BeforeEach(func() {
			np = vsphere.DefaultNetworkProvider(finder)

			spec := types.HostPortGroupSpec{
				Name:        dummyNetworkName,
				VswitchName: "vSwitch0",
			}
			Expect(ns.AddPortGroup(ctx, spec)).To(BeNil())
		})

		Context("when creating vnic", func() {
			It("create vnic should succeed", func() {
				dev, err := np.CreateVnic(ctx, vm, vmNif)
				Expect(err).To(BeNil())
				Expect(dev).NotTo(BeNil())
				Expect(dev.GetVirtualDevice()).NotTo(BeNil())
				info := dev.GetVirtualDevice().Backing
				Expect(info).NotTo(BeNil())

				deviceInfo, ok := info.(*types.VirtualEthernetCardNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(deviceInfo.DeviceName).To(Equal(dummyNetworkName))
			})

			It("should return an error if network does not exist", func() {
				_, err := np.CreateVnic(ctx, vm, &v1alpha1.VirtualMachineNetworkInterface{
					NetworkName: "does-not-exist",
				})
				Expect(err).To(MatchError("unable to find network \"does-not-exist\": network 'does-not-exist' not found"))
			})

			It("should ignore if vm is nil", func() {
				_, err := np.CreateVnic(ctx, nil, vmNif)
				Expect(err).To(BeNil())
			})

		})
	})

	Context("when using NSX-T network provider", func() {
		var (
			ncpClient clientset.Interface
			ncpVif    *ncpv1alpha1.VirtualNetworkInterface
		)
		BeforeEach(func() {
			ncpClient = ncpfake.NewSimpleClientset()
			np = vsphere.NsxtNetworkProvider(finder, ncpClient)

			ncpVif = &ncpv1alpha1.VirtualNetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s-lsp", vmNif.NetworkName, vm.Name),
					Namespace: dummyNamespace,
				},
				Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
					VirtualNetwork: dummyNetworkName,
				},
				Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
					MacAddress:  macAddress,
					InterfaceID: interfaceId,
					Conditions:  []ncpv1alpha1.VirtualNetworkCondition{{Type: "Ready", Status: "True"}},
					ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: dummyNsxSwitchId,
					},
				},
			}
		})

		// Creates the vnetif and other objects in the system the way we want to test them
		// This runs after all BeforeEach()
		JustBeforeEach(func() {
			_, err := ncpClient.VmwareV1alpha1().VirtualNetworkInterfaces(dummyNamespace).Create(ncpVif)
			Expect(err).To(BeNil())
		})

		Context("when creating vnic", func() {
			It("should return an error if vm is nil", func() {
				_, err := np.CreateVnic(ctx, nil, vmNif)
				Expect(err).To(MatchError("Virtual machine can not be nil when creating vnetif"))
			})

			It("should return an error if vif is nil", func() {
				_, err := np.CreateVnic(ctx, vm, nil)
				Expect(err).To(MatchError("Virtual machine network interface can not be nil when creating vnetif"))
			})

			Context("when interface has no provider status defined", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: "",
					}
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("Failed to get for nsx-t opaque network ID for vnetif '"))
				})
			})

			Context("when interface has no opaque network id defined in the provider status", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = nil
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("Failed to get for nsx-t opaque network ID for vnetif '"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					ncpVif.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(MatchError("timed out waiting for the condition"))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus.NsxLogicalSwitchID = "does-not-exist"
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(MatchError("opaque network with ID 'does-not-exist' not found"))
				})
			})

			It("should succeed", func() {
				res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
					// config.logicalSwitchUuid not yet supported in govcsim.
					pc := new(propertyCollectorRetrievePropertiesOverride)
					pc.Self = c.ServiceContent.PropertyCollector
					simulator.Map.Put(pc)

					finder := find.NewFinder(c)
					np, err := vsphere.NetworkProviderByType("nsx-t", finder, ncpClient)
					Expect(err).To(BeNil())

					dev, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(BeNil())
					Expect(dev).NotTo(BeNil())

					nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())

					Expect(nic.ExternalId).To(Equal(interfaceId))
					Expect(nic.MacAddress).To(Equal(macAddress))
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))

					return nil
				})
				Expect(res).To(BeNil())
			})

			It("should update owner reference if already exist", func() {
				otherVmWithDifferentUid := &v1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						UID:       "another-uid",
					},
					Spec: v1alpha1.VirtualMachineSpec{
						NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
							*vmNif,
						},
					},
				}
				// TODO: we can't test the owner reference is correct
				// But this test exercises the path that there is an existing network interface owned by a different VM
				// and this interface should be updated with the new VM.
				_, err := np.CreateVnic(ctx, otherVmWithDifferentUid, vmNif)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Context("when getting the network by type", func() {
		var (
			ncpClient clientset.Interface
		)
		BeforeEach(func() {
			ncpClient = ncpfake.NewSimpleClientset()
		})

		It("should find NSX-T", func() {
			expectedProvider := vsphere.NsxtNetworkProvider(finder, ncpClient)

			np, err := vsphere.NetworkProviderByType("nsx-t", finder, ncpClient)
			Expect(err).To(BeNil())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})

		It("should find the default", func() {
			expectedProvider := vsphere.DefaultNetworkProvider(finder)

			np, err := vsphere.NetworkProviderByType("", finder, ncpClient)
			Expect(err).To(BeNil())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})
	})
})

// See https://github.com/dougm/govmomi/commit/ef3672a24c5dcfcc4637c771a7f96fff4ff23938
type propertyCollectorRetrievePropertiesOverride struct {
	simulator.PropertyCollector
}

// RetrieveProperties overrides simulator.PropertyCollector.RetrieveProperties, returning a custom value for the "config.description" field
func (pc *propertyCollectorRetrievePropertiesOverride) RetrieveProperties(ctx *simulator.Context, req *types.RetrieveProperties) soap.HasFault {
	body := &methods.RetrievePropertiesBody{
		Res: &types.RetrievePropertiesResponse{
			Returnval: []types.ObjectContent{
				{
					Obj: req.SpecSet[0].ObjectSet[0].Obj,
					PropSet: []types.DynamicProperty{
						{
							Name: "config.logicalSwitchUuid",
							Val:  dummyNsxSwitchId,
						},
					},
				},
			},
		},
	}

	for _, spec := range req.SpecSet {
		for _, prop := range spec.PropSet {
			for _, path := range prop.PathSet {
				if path == "config.logicalSwitchUuid" {
					return body
				}
			}
		}
	}

	return pc.PropertyCollector.RetrieveProperties(ctx, req)
}
