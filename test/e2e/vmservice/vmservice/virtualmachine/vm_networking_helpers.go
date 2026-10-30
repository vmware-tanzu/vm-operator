// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

const (
	// vmNameLabel is the label VM Operator puts on a network interface CR to
	// identify its VM by name.
	vmNameLabel = "vmoperator.vmware.com/vm-name"

	// externalNetworkInterfaceAnnotation is the annotation VM Operator puts on
	// an external network interface CR that a VM is using.
	externalNetworkInterfaceAnnotation = "vmoperator.vmware.com/external-network-interface"

	// vpcAttachmentRefAnnotation is the annotation that associates a VPC
	// SubnetPort with the VM interface using it.
	vpcAttachmentRefAnnotation = "nsx.vmware.com/attachment_ref"
)

// newExternalNetworkInterface returns a network interface CR of the
// environment's network provider type, not owned by any VM, and the reference
// to use in a VM interface's ExternalRef. The CR does not name a network, so it
// uses the namespace's default network, just like the VM's default interface.
func newExternalNetworkInterface(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, name string) (ctrlclient.Object, *vmopv1common.LocalObjectRef) {

	GinkgoHelper()

	objectMeta := metav1.ObjectMeta{
		Namespace: namespace,
		Name:      name,
	}

	switch {
	case vmoperator.IsNetworkNsxtVPC(ctx, client, config):
		return &vpcv1alpha1.SubnetPort{ObjectMeta: objectMeta},
			&vmopv1common.LocalObjectRef{
				APIVersion: vpcv1alpha1.SchemeGroupVersion.String(),
				Kind:       "SubnetPort",
				Name:       name,
			}
	case framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.NSX):
		return &ncpv1alpha1.VirtualNetworkInterface{ObjectMeta: objectMeta},
			&vmopv1common.LocalObjectRef{
				APIVersion: ncpv1alpha1.SchemeGroupVersion.String(),
				Kind:       "VirtualNetworkInterface",
				Name:       name,
			}
	default:
		return &netopv1alpha1.NetworkInterface{ObjectMeta: objectMeta},
			&vmopv1common.LocalObjectRef{
				APIVersion: netopv1alpha1.SchemeGroupVersion.String(),
				Kind:       "NetworkInterface",
				Name:       name,
			}
	}
}

// getNetworkInterface returns the current state of the network interface CR in
// a new object, leaving netIf untouched.
func getNetworkInterface(
	ctx context.Context,
	client ctrlclient.Client,
	netIf ctrlclient.Object) (ctrlclient.Object, error) {

	current, ok := netIf.DeepCopyObject().(ctrlclient.Object)
	if !ok {
		return nil, fmt.Errorf("%T is not a client object", netIf)
	}
	if err := client.Get(ctx, ctrlclient.ObjectKeyFromObject(netIf), current); err != nil {
		return nil, fmt.Errorf("failed to get network interface %s: %w", netIf.GetName(), err)
	}

	return current, nil
}

// networkInterfaceMAC returns the MAC address the provider reports on an
// external network interface CR, or empty if the provider does not report one
// (VDS) or has not yet assigned one.
func networkInterfaceMAC(netIf ctrlclient.Object) string {
	var mac string

	switch o := netIf.(type) {
	case *vpcv1alpha1.SubnetPort:
		mac = o.Status.NetworkInterfaceConfig.MACAddress
	case *ncpv1alpha1.VirtualNetworkInterface:
		mac = o.Status.MacAddress
	}

	mac = strings.ToLower(mac)
	if mac == "00:00:00:00:00:00" {
		return ""
	}

	return mac
}

// countOwnedNetworkInterfaces returns how many network interface CRs, of the
// same type as sample, in the namespace are owned by the owner UID.
func countOwnedNetworkInterfaces(
	ctx context.Context,
	client ctrlclient.Client,
	sample ctrlclient.Object,
	namespace string,
	ownerUID string) (int, error) {

	var list ctrlclient.ObjectList

	switch sample.(type) {
	case *vpcv1alpha1.SubnetPort:
		list = &vpcv1alpha1.SubnetPortList{}
	case *ncpv1alpha1.VirtualNetworkInterface:
		list = &ncpv1alpha1.VirtualNetworkInterfaceList{}
	default:
		list = &netopv1alpha1.NetworkInterfaceList{}
	}

	if err := client.List(ctx, list, ctrlclient.InNamespace(namespace)); err != nil {
		return 0, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	items, err := apimeta.ExtractList(list)
	if err != nil {
		return 0, fmt.Errorf("failed to extract network interfaces: %w", err)
	}

	var owned int
	for _, item := range items {
		obj, ok := item.(ctrlclient.Object)
		if !ok {
			continue
		}
		for _, ref := range obj.GetOwnerReferences() {
			if string(ref.UID) == ownerUID {
				owned++
			}
		}
	}

	return owned, nil
}

// getEthCards returns the EthernetCards of the VM in vCenter.
func getEthCards(
	ctx context.Context,
	propCollector *property.Collector,
	vmMoRef types.ManagedObjectReference) ([]types.BaseVirtualEthernetCard, error) {

	var vmMO mo.VirtualMachine
	if err := propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO); err != nil {
		return nil, fmt.Errorf("failed to retrieve config of VM %s: %w", vmMoRef.Value, err)
	}
	if vmMO.Config == nil {
		return nil, fmt.Errorf("VM %s has no config", vmMoRef.Value)
	}

	var cards []types.BaseVirtualEthernetCard
	devices := object.VirtualDeviceList(vmMO.Config.Hardware.Device)
	for _, d := range devices.SelectByType((*types.VirtualEthernetCard)(nil)) {
		if card, ok := d.(types.BaseVirtualEthernetCard); ok {
			cards = append(cards, card)
		}
	}

	return cards, nil
}

// expectEthCards waits for the VM to have exactly one EthernetCard in
// vCenter and, if connected is true, for that card to be connected.
func expectEthCards(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	propCollector *property.Collector,
	vmMoRef types.ManagedObjectReference,
	connected bool) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		cards, err := getEthCards(ctx, propCollector, vmMoRef)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get EthernetCards")
		g.Expect(cards).To(HaveLen(1), "unexpected number of EthernetCards")

		if connected {
			for _, c := range cards {
				nic := c.GetVirtualEthernetCard()
				g.Expect(nic.Connectable).ToNot(BeNil(), "EthernetCard has no connectable info")
				g.Expect(nic.Connectable.Connected).To(BeTrue(), "EthernetCard should be connected")
			}
		}
	}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(),
		"VM should have 1 EthernetCard (connected required: %t)", connected)
}

// expectEthCardsBackedByExternal asserts that the VM has numCards
// EthernetCards, that the external network interfaces are the ones in use,
// and that VM Operator did not create a managed interface to stand in for an
// external one: the VM should own exactly numCards-len(externalNetIfs) network
// interface CRs.
//
// The MAC address assigned by the provider to an external interface must be on
// one of the cards. VDS does not report a MAC address, so for it the check
// rests on the owned-CR count.
func expectEthCardsBackedByExternal(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	propCollector *property.Collector,
	vmMoRef types.ManagedObjectReference,
	namespace, vmName string,
	numCards int,
	externalNetIfs ...ctrlclient.Object) {

	GinkgoHelper()

	Expect(externalNetIfs).ToNot(BeEmpty(), "at least one external network interface is required")

	vm, err := utils.GetVirtualMachine(ctx, client, namespace, vmName)
	Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine %s", vmName)

	Eventually(func(g Gomega) {
		cards, err := getEthCards(ctx, propCollector, vmMoRef)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get EthernetCards")
		g.Expect(cards).To(HaveLen(numCards), "unexpected number of EthernetCards")

		var cardMACs []string
		for _, c := range cards {
			cardMACs = append(cardMACs, strings.ToLower(c.GetVirtualEthernetCard().MacAddress))
		}

		for _, netIf := range externalNetIfs {
			current, err := getNetworkInterface(ctx, client, netIf)
			g.Expect(err).ToNot(HaveOccurred(), "failed to get external network interface")
			if mac := networkInterfaceMAC(current); mac != "" {
				g.Expect(cardMACs).To(ContainElement(mac), "no EthernetCard has the MAC of %s", netIf.GetName())
			}
		}

		owned, err := countOwnedNetworkInterfaces(ctx, client, externalNetIfs[0], namespace, string(vm.UID))
		g.Expect(err).ToNot(HaveOccurred(), "failed to count owned network interfaces")
		g.Expect(owned).To(Equal(numCards-len(externalNetIfs)),
			"unexpected number of managed network interfaces owned by the VM")
	}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(),
		"EthernetCards should be backed by the external network interfaces")
}

// expectExternalInterfaceAssociated waits for the external network interface
// CR to be associated with the VM's interface and not owned by the VM.
func expectExternalInterfaceAssociated(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	netIf ctrlclient.Object,
	vmName, interfaceName string) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		current, err := getNetworkInterface(ctx, client, netIf)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get external network interface")
		g.Expect(current.GetOwnerReferences()).To(BeEmpty(), "external network interface should not be owned")
		g.Expect(current.GetLabels()).To(HaveKeyWithValue(vmNameLabel, vmName), "external network interface should be labeled with the VM name")
		g.Expect(current.GetAnnotations()).To(HaveKey(externalNetworkInterfaceAnnotation), "external network interface should be annotated as external")
		if _, ok := current.(*vpcv1alpha1.SubnetPort); ok {
			g.Expect(current.GetAnnotations()).To(HaveKeyWithValue(
				vpcAttachmentRefAnnotation, "virtualmachine/"+vmName+"/"+interfaceName),
				"SubnetPort should reference the VM interface")
		}
	}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(),
		"external network interface %s should be associated with %s/%s", netIf.GetName(), vmName, interfaceName)
}

// expectExternalInterfaceReleased waits for the external network interface CR
// to no longer be associated with a VM, then verifies that it is the same CR
// that was created, still exists, and is not owned by anything.
func expectExternalInterfaceReleased(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	netIf ctrlclient.Object) {

	GinkgoHelper()

	expectReleased := func(g Gomega) {
		current, err := getNetworkInterface(ctx, client, netIf)
		g.Expect(err).ToNot(HaveOccurred(), "external network interface should still exist")
		g.Expect(current.GetUID()).To(Equal(netIf.GetUID()), "external network interface was recreated")
		g.Expect(current.GetOwnerReferences()).To(BeEmpty(), "external network interface should not be owned")
		g.Expect(current.GetLabels()).ToNot(HaveKey(vmNameLabel), "VM name label should be removed")
		g.Expect(current.GetAnnotations()).ToNot(HaveKey(vpcAttachmentRefAnnotation), "attachment ref should be removed")
		g.Expect(current.GetAnnotations()).ToNot(HaveKey(externalNetworkInterfaceAnnotation), "external annotation should be removed")
	}

	Eventually(expectReleased,
		config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(),
		"external network interface %s should be released", netIf.GetName())
	Consistently(expectReleased,
		config.GetIntervals("default", "consistent-virtual-machine-network-interface")...).Should(Succeed(),
		"external network interface %s should stay released", netIf.GetName())
}
