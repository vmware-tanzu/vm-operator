// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"context"

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
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"
)

const (
	dummyObjectName = "dummy"
	dummyNamespace  = "dummy"
)

var _ = Describe("NetworkProvider", func() {
	var (
		err    error
		finder *find.Finder
		ns     *object.HostNetworkSystem

		ctx context.Context
		vif *v1alpha1.VirtualMachineNetworkInterface

		np  vsphere.NetworkProvider
		dev types.BaseVirtualDevice

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
	})
	JustBeforeEach(func() {
		vm := &v1alpha1.VirtualMachine{
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
		dev, err = np.CreateVnic(ctx, vm, vif)
	})

	Context("default network provider", func() {
		BeforeEach(func() {
			name = dummyObjectName
			namespace = dummyNamespace
			vif = &v1alpha1.VirtualMachineNetworkInterface{
				NetworkName: "dummy-network",
			}
			np = vsphere.DefaultNetworkProvider(finder)
			spec := types.HostPortGroupSpec{
				Name:        "dummy-network",
				VswitchName: "vSwitch0",
			}
			Expect(ns.AddPortGroup(ctx, spec)).To(BeNil())
		})

		It("should succeed", func() {
			Expect(err).To(BeNil())
			Expect(dev).NotTo(BeNil())
		})
	})

	Context("nsx-t network provider", func() {
		var (
			ncpClient clientset.Interface
		)
		BeforeEach(func() {
			Skip("Skipping since HostNetworkSystem:hostnetworksystem-2 does not implement: AddVirtualNic")
			ncpClient = ncpfake.NewSimpleClientset()
			name = dummyObjectName
			namespace = dummyNamespace
			vif = &v1alpha1.VirtualMachineNetworkInterface{
				NetworkName: "gc-network",
			}
			nsxtNp := vsphere.NsxtNetworkProvider(finder, ncpClient)
			np = nsxtNp
			// mock vnetif object
			mockVnetif := &ncpv1alpha1.VirtualNetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nsxtNp.GenerateNsxVnetifName("gc-network", dummyObjectName),
					Namespace: dummyNamespace,
				},
				Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
					VirtualNetwork: "gc-network",
				},
				Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
					MacAddress: "01-23-45-67-89-AB-CD-EF",
					Conditions: []ncpv1alpha1.VirtualNetworkCondition{{Type: "Ready", Status: "True"}},
					ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: "dummy-opnetwork-id",
					},
				},
			}
			_, err = ncpClient.VmwareV1alpha1().VirtualNetworkInterfaces(dummyNamespace).Create(mockVnetif)
			Expect(err).To(BeNil())
			portGroup := types.HostPortGroupSpec{
				Name:        nsxtNp.GenerateNsxVnetifName("gc-network", dummyObjectName),
				VswitchName: "vSwitch0",
			}
			Expect(ns.AddPortGroup(ctx, portGroup)).To(BeNil())
			vnic := types.HostVirtualNicSpec{
				OpaqueNetwork: &types.HostVirtualNicOpaqueNetworkSpec{
					OpaqueNetworkId:   "dummy-opnetwork-id",
					OpaqueNetworkType: "nsx-t",
				},
			}
			_, err = ns.AddVirtualNic(ctx, "", vnic)
			Expect(err).To(BeNil())
		})

		It("should succeed", func() {
			Expect(err).To(BeNil())
			Expect(dev).NotTo(BeNil())
		})
	})
})
