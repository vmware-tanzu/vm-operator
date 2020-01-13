// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
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
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"
)

const networkId = "dummy-opnetwork-id"

var _ = Describe("NetworkProvider", func() {
	var (
		name        string
		namespace   string
		networkName string

		vif *v1alpha1.VirtualMachineNetworkInterface
		vm  *v1alpha1.VirtualMachine
	)

	BeforeEach(func() {
		name, namespace = "dummy", "dummyNs"
		networkName = "gc-dummy-network"

		vif = &v1alpha1.VirtualMachineNetworkInterface{
			NetworkName: networkName,
		}

		vm = &v1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha1.VirtualMachineSpec{
				NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
					*vif,
				},
			},
		}
	})

	Context("default network provider", func() {
		Specify("create vnic", func() {
			res := simulator.ESX().Run(func(ctx context.Context, c *vim25.Client) error {
				np, err := vsphere.NetworkProviderByType("", find.NewFinder(c), nil)
				Expect(err).To(BeNil())

				host := object.NewHostSystem(c, esx.HostSystem.Reference())
				ns, err := host.ConfigManager().NetworkSystem(ctx)
				Expect(err).To(BeNil())

				spec := types.HostPortGroupSpec{
					Name:        networkName,
					VswitchName: "vSwitch0",
				}
				Expect(ns.AddPortGroup(ctx, spec)).To(BeNil())

				dev, err := np.CreateVnic(ctx, vm, vif)
				Expect(err).To(BeNil())
				Expect(dev).NotTo(BeNil())

				return nil
			})
			Expect(res).To(BeNil())
		})
	})

	Context("nsx-t network provider", func() {
		Specify("create vnic", func() {
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				// config.logicalSwitchUuid not yet supported in govcsim.
				pc := new(propertyCollectorRetrievePropertiesOverride)
				pc.Self = c.ServiceContent.PropertyCollector
				simulator.Map.Put(pc)

				finder := find.NewFinder(c)
				ncpClient := ncpfake.NewSimpleClientset()
				np, err := vsphere.NetworkProviderByType("nsx-t", finder, ncpClient)
				Expect(err).To(BeNil())

				// Must create the interface now with Status because Provider polls for it to be populated.
				vnetif := &ncpv1alpha1.VirtualNetworkInterface{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-%s-lsp", vif.NetworkName, vm.Name),
						Namespace: namespace,
					},
					Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
						VirtualNetwork: vif.NetworkName,
					},
					Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
						MacAddress: "01-23-45-67-89-AB-CD-EF",
						Conditions: []ncpv1alpha1.VirtualNetworkCondition{{Type: "Ready", Status: "True"}},
						ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
							NsxLogicalSwitchID: networkId,
						},
					},
				}
				_, err = ncpClient.VmwareV1alpha1().VirtualNetworkInterfaces(namespace).Create(vnetif)
				Expect(err).To(BeNil())

				dev, err := np.CreateVnic(ctx, vm, vif)
				Expect(err).To(BeNil())
				Expect(dev).NotTo(BeNil())

				return nil
			})
			Expect(res).To(BeNil())
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
							Val:  networkId,
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
