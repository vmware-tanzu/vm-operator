// +build !integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package vsphere_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	dummyObjectName  = "dummy-object"
	dummyNamespace   = "dummy-ns"
	dummyNsxSwitchId = "dummy-opaque-network-id"
	macAddress       = "01-23-45-67-89-AB-CD-EF"
	interfaceId      = "interface-id"

	vcsimNetworkName    = "DC0_DVPG0"
	dummyVirtualNetwork = "dummy-virtual-net"
)

var _ = Describe("NetworkProvider", func() {
	var (
		c      *govmomi.Client
		finder *find.Finder

		cluster *object.ClusterComputeResource
		network object.NetworkReference

		ctx   context.Context
		vmNif *v1alpha1.VirtualMachineNetworkInterface
		vm    *v1alpha1.VirtualMachine

		np vsphere.NetworkProvider

		name      string
		namespace string
	)

	BeforeEach(func() {
		ctx = context.TODO()
		c, _ = govmomi.NewClient(ctx, server.URL, true)
		finder = find.NewFinder(c.Client)

		dc, err := finder.DefaultDatacenter(ctx)
		Expect(err).To(BeNil())
		finder.SetDatacenter(dc)

		cluster, err = finder.DefaultClusterComputeResource(ctx)
		Expect(err).To(BeNil())

		network, err = finder.Network(ctx, vcsimNetworkName)
		Expect(err).To(BeNil())

		name = dummyObjectName
		namespace = dummyNamespace

		vmNif = &v1alpha1.VirtualMachineNetworkInterface{
			NetworkName: vcsimNetworkName,
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

	Context("when getting the network by type", func() {
		var (
			ncpClient clientset.Interface
		)
		BeforeEach(func() {
			ncpClient = ncpfake.NewSimpleClientset()
		})

		It("should find NSX-T", func() {
			expectedProvider := vsphere.NsxtNetworkProvider(finder, ncpClient, nil)

			np, err := vsphere.NetworkProviderByType("nsx-t", finder, ncpClient, nil)
			Expect(err).To(BeNil())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})

		It("should find the default", func() {
			expectedProvider := vsphere.DefaultNetworkProvider(finder)

			np, err := vsphere.NetworkProviderByType("", finder, ncpClient, nil)
			Expect(err).To(BeNil())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})
	})

	Context("when using default network provider", func() {
		BeforeEach(func() {
			np = vsphere.DefaultNetworkProvider(finder)
		})

		Context("when creating vnic", func() {
			It("create vnic should succeed", func() {
				dev, err := np.CreateVnic(ctx, vm, vmNif)
				Expect(err).To(BeNil())
				Expect(dev).NotTo(BeNil())
				Expect(dev.GetVirtualDevice()).NotTo(BeNil())
				info := dev.GetVirtualDevice().Backing
				Expect(info).NotTo(BeNil())

				deviceInfo, ok := info.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(deviceInfo.Port.PortgroupKey).To(Equal(network.Reference().Value))
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
			np = vsphere.NsxtNetworkProvider(finder, ncpClient, cluster)
			ncpVif = &ncpv1alpha1.VirtualNetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s-lsp", vmNif.NetworkName, vm.Name),
					Namespace: dummyNamespace,
				},
				Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
					VirtualNetwork: dummyVirtualNetwork,
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
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetif '"))
				})
			})

			Context("when interface has no opaque network id defined in the provider status", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = nil
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetif '"))
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
					cluster, err := finder.DefaultClusterComputeResource(ctx)
					Expect(err).To(BeNil())
					np, err := vsphere.NetworkProviderByType("nsx-t", finder, ncpClient, cluster)
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
				Expect(err).To(BeNil())
			})
		})
	})
})

// See https://github.com/dougm/govmomi/commit/ef3672a24c5dcfcc4637c771a7f96fff4ff23938
type propertyCollectorRetrievePropertiesOverride struct {
	simulator.PropertyCollector
}

// RetrieveProperties overrides simulator.PropertyCollector.RetrieveProperties, returning a custom value for the "config.logicalSwitchUuid" field
// The property `config.logicalSwitchUuid` is not available on a dvpg object model in govmomi (@v0.22.2 at the time of writing this comment).
// Hence, as a workaround we replace the entire property collector in the simulator with our version of property collector that returns a fake value for that property.
// Typically, a property collector returns the queried properties & their values in a propSet object. If the specified properties are missing from the vim object,
// it additionally populates a missingSet object with the missing properties. Here, we look for `config.logicalSwitchUuid` in the missingSet and if found, include a
// fake value for that property in the propSet object. This ensures we are not overwriting any other properties queried by the caller.
func (pc *propertyCollectorRetrievePropertiesOverride) RetrieveProperties(ctx *simulator.Context, req *types.RetrieveProperties) soap.HasFault {

	fault := pc.PropertyCollector.RetrieveProperties(ctx, req)
	body := fault.(*methods.RetrievePropertiesBody)

	var newObjectContent []types.ObjectContent
	//TODO: Remove this when we move to the latest govmomi that supports `config.logicalSwitchUuid` on a DVPG.
	for _, objContent := range body.Res.Returnval {
		missingProp := false
		var newMissingSet []types.MissingProperty
		for _, prop := range objContent.MissingSet {
			if prop.Path == "config.logicalSwitchUuid" {
				missingProp = true
			} else {
				newMissingSet = append(newMissingSet, prop)
			}
		}
		if missingProp {
			logicalSwitchUuidProp := types.DynamicProperty{
				Name: "config.logicalSwitchUuid",
				Val:  dummyNsxSwitchId,
			}
			objContent.PropSet = append(objContent.PropSet, logicalSwitchUuidProp)
		}
		// Remove the property from the MissingSet since we now have replaced its value with a fake.
		objContent.MissingSet = newMissingSet
		newObjectContent = append(newObjectContent, objContent)
	}
	body.Res.Returnval = newObjectContent
	return body
}
