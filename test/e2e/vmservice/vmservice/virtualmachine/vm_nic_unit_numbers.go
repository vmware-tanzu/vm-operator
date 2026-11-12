// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	// nicUnitNumberFirst and nicUnitNumberLast bound the PCI units that
	// Ethernet cards occupy on a virtual machine.
	nicUnitNumberFirst int32 = 7
	nicUnitNumberLast  int32 = 16

	// nicUnitNumberNudgeAnnotation is patched to trigger a reconcile.
	nicUnitNumberNudgeAnnotation = "e2e.vmoperator.vmware.com/nic-unit-numbers-nudge"
)

// VMNICUnitNumbersSpecInput is the input for the NIC unit number E2E spec.
type VMNICUnitNumbersSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// nicHardware is the identity of an Ethernet card as observed in vCenter.
type nicHardware struct {
	Key  int32
	MAC  string
	Unit int32
}

// nicUnitNumbersEnv bundles what the NIC unit number helpers need.
type nicUnitNumbersEnv struct {
	input         VMNICUnitNumbersSpecInput
	client        ctrlclient.Client
	propCollector *property.Collector
	vmiName       string
}

// createPoweredOffVM creates a powered off VM with numNICs interfaces, none of
// which carry a unit number, and returns its key and vSphere MoRef.
func (e *nicUnitNumbersEnv) createPoweredOffVM(
	ctx context.Context,
	prefix string,
	numNICs int) (types.NamespacedName, vimtypes.ManagedObjectReference) {

	GinkgoHelper()

	config := e.input.Config
	resources := config.InfraConfig.ManagementClusterConfig.Resources
	vmName := prefix + capiutil.RandomString(5)
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: e.input.WCPNamespaceName,
		},
		Spec: vmopv1.VirtualMachineSpec{
			ClassName:    resources.VMClassName,
			ImageName:    e.vmiName,
			StorageClass: resources.StorageClassName,
			PowerState:   vmopv1.VirtualMachinePowerStateOff,
			Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
				Disabled: true,
			},
			Network: &vmopv1.VirtualMachineNetworkSpec{},
		},
	}
	for i := range numNICs {
		// The network is left unset so that the default network is applied
		// to every interface at admission.
		vm.Spec.Network.Interfaces = append(
			vm.Spec.Network.Interfaces,
			vmopv1.VirtualMachineNetworkInterfaceSpec{
				Name: fmt.Sprintf("eth%d", i),
				Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
			})
	}
	vmoperator.CreateVirtualMachine(ctx, config, e.input.ClusterProxy, vm, e.input.SkipCleanup)

	// Create populates vm with the admitted object, so this checks admission
	// directly without racing a reconcile.
	By("Verifying admission does not assign unit numbers on create")
	Expect(vm.Spec.Network).ToNot(BeNil())
	Expect(vm.Spec.Network.Interfaces).To(HaveLen(numNICs))
	for _, iface := range vm.Spec.Network.Interfaces {
		Expect(iface.UnitNumber).To(BeNil(),
			"interface %s should not have a unit number before the VM is created", iface.Name)
	}

	vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, e.client, vm.Namespace, vm.Name)
	moID := vmoperator.WaitForVirtualMachineMOID(ctx, config, e.client, vm.Namespace, vm.Name)

	key := ctrlclient.ObjectKeyFromObject(vm)
	return key, vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: moID}
}

// waitForBackfill waits until each of the numNICs interfaces has a unit number
// and returns the VM.
func (e *nicUnitNumbersEnv) waitForBackfill(
	ctx context.Context,
	key types.NamespacedName,
	numNICs int) *vmopv1.VirtualMachine {

	GinkgoHelper()

	By("Waiting for the schema upgrade to backfill the unit numbers")
	var vm *vmopv1.VirtualMachine
	Eventually(func(g Gomega) {
		var err error
		vm, err = utils.GetVirtualMachine(ctx, e.client, key.Namespace, key.Name)
		g.Expect(err).ToNot(HaveOccurred())
		// The webhooks gate on the upgrade annotation, so wait for it too.
		g.Expect(vm.Annotations).To(HaveKey(pkgconst.UpgradedToFeatureVersionAnnotationKey),
			"VM %s has not been upgraded yet", key)
		g.Expect(vm.Spec.Network).ToNot(BeNil())
		g.Expect(vm.Spec.Network.Interfaces).To(HaveLen(numNICs))
		for _, iface := range vm.Spec.Network.Interfaces {
			g.Expect(iface.UnitNumber).ToNot(BeNil(),
				"interface %s has no unit number yet", iface.Name)
		}
	}, e.input.Config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
		"Timed out waiting for unit numbers to be backfilled on VM %s", key)

	return vm
}

// hardwareNICs returns the Ethernet cards of the VM as seen by vCenter, sorted
// by unit number.
func (e *nicUnitNumbersEnv) hardwareNICs(
	ctx context.Context,
	g Gomega,
	moRef vimtypes.ManagedObjectReference) []nicHardware {

	var vmMO mo.VirtualMachine
	g.Expect(e.propCollector.RetrieveOne(ctx, moRef, []string{"config.hardware.device"}, &vmMO)).To(Succeed(),
		"failed to retrieve device config for VM %s", moRef.Value)
	g.Expect(vmMO.Config).ToNot(BeNil())

	var nics []nicHardware
	for _, dev := range object.VirtualDeviceList(vmMO.Config.Hardware.Device).
		SelectByType((*vimtypes.VirtualEthernetCard)(nil)) {

		vd := dev.GetVirtualDevice()
		g.Expect(vd.UnitNumber).ToNot(BeNil(), "Ethernet card %d has no unit number", vd.Key)

		nic := nicHardware{Key: vd.Key, Unit: *vd.UnitNumber}
		if card, ok := dev.(vimtypes.BaseVirtualEthernetCard); ok {
			nic.MAC = card.GetVirtualEthernetCard().MacAddress
		}
		nics = append(nics, nic)
	}
	slices.SortFunc(nics, func(a, b nicHardware) int { return int(a.Unit - b.Unit) })

	return nics
}

// expectStable asserts that the Ethernet cards still equal baseline and the
// spec unit numbers still equal units for the whole stability window.
//
// The MAC is part of the baseline because a NIC that is removed and re-added
// at the same unit number can get the same key back, but not the same
// generated MAC. That only holds when the MAC is generated, as with VDS
// networking. Providers that pin the MAC through the network interface CR
// (NSX-T, VPC) re-add the same MAC, so there the baseline cannot tell a
// replaced NIC from an untouched one and this check is a weak guard against
// churn.
func (e *nicUnitNumbersEnv) expectStable(
	ctx context.Context,
	key types.NamespacedName,
	moRef vimtypes.ManagedObjectReference,
	baseline []nicHardware,
	units []int32) {

	GinkgoHelper()

	// Touch a throwaway annotation so that the VM is reconciled at the start
	// of the window instead of sitting idle.
	Eventually(func() error {
		return vmoperator.PatchVirtualMachine(ctx, e.client, key.Namespace, key.Name,
			func(vm *vmopv1.VirtualMachine) error {
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations[nicUnitNumberNudgeAnnotation] = time.Now().Format(time.RFC3339Nano)
				return nil
			})
	}, e.input.Config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(),
		"failed to nudge reconcile of VM %s", key)

	Consistently(func(g Gomega) {
		g.Expect(e.hardwareNICs(ctx, g, moRef)).To(Equal(baseline),
			"Ethernet cards of VM %s changed identity", key)
		current, err := utils.GetVirtualMachine(ctx, e.client, key.Namespace, key.Name)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get VM %s", key)
		g.Expect(specUnitNumbers(g, current)).To(Equal(units),
			"spec unit numbers of VM %s changed", key)
	}, e.input.Config.GetIntervals("default", "consistent-vm-nic-unit-numbers")...).Should(Succeed())
}

// specUnitNumbers returns the sorted unit numbers of the spec interfaces.
func specUnitNumbers(g Gomega, vm *vmopv1.VirtualMachine) []int32 {
	GinkgoHelper()

	var units []int32
	for _, iface := range vm.Spec.Network.Interfaces {
		g.Expect(iface.UnitNumber).ToNot(BeNil(), "interface %s has no unit number", iface.Name)
		units = append(units, *iface.UnitNumber)
	}
	slices.Sort(units)

	return units
}

// expectValidUnitNumbers asserts the sorted unit numbers are in the Ethernet
// card band and unique.
func expectValidUnitNumbers(units []int32) {
	GinkgoHelper()

	for _, u := range units {
		Expect(u).To(BeNumerically(">=", nicUnitNumberFirst))
		Expect(u).To(BeNumerically("<=", nicUnitNumberLast))
	}
	Expect(slices.Compact(slices.Clone(units))).To(Equal(units),
		"unit numbers must be unique: %v", units)
}

// hardwareUnitNumbers returns the sorted unit numbers of the cards.
func hardwareUnitNumbers(nics []nicHardware) []int32 {
	units := make([]int32, 0, len(nics))
	for _, n := range nics {
		units = append(units, n.Unit)
	}
	slices.Sort(units)

	return units
}

// lowestFreeNICUnitNumber returns the lowest unit number in the Ethernet card
// band that is not in use.
func lowestFreeNICUnitNumber(used []int32) int32 {
	u := nicUnitNumberFirst
	for slices.Contains(used, u) {
		u++
	}
	return u
}

// VMNICUnitNumbersSpec exercises spec.network.interfaces[].unitNumber across
// VM creation, the schema-upgrade backfill that runs after the VM is created,
// and the admission-time assignment for interfaces added afterwards.
//
// Out of scope until the reconcile and status changes land: explicit values on
// create, unit-number matching and NIC replacement, and
// status.network.interfaces[].unitNumber.
func VMNICUnitNumbersSpec(ctx context.Context, inputGetter func() VMNICUnitNumbersSpecInput) {
	const specName = "vm-nic-unit-numbers"

	var (
		input VMNICUnitNumbersSpecInput
		env   *nicUnitNumbersEnv
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(),
			"Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		skipper.SkipUnlessSupervisorCapabilityEnabled(
			input.Config.SupervisorCapabilities,
			consts.VMNetworkInterfaceUnitNumbersCapabilityName)

		config := input.Config
		svClusterClient := input.ClusterProxy.GetClient()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			input.ClusterProxy.GetRESTConfig(),
			filepath.Join(input.ArtifactFolder, specName),
		)
		DeferCleanup(cancelPodWatches)

		vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, input.ClusterProxy.GetKubeconfigPath())
		DeferCleanup(vcenter.LogoutVimClient, vCenterClient)

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(
			config.InfraConfig.ManagementClusterConfig.Resources)

		env = &nicUnitNumbersEnv{
			input:         input,
			client:        svClusterClient,
			propCollector: property.DefaultCollector(vCenterClient),
			vmiName: vmoperator.WaitForVirtualMachineImageName(
				ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName),
		}
	})

	It("backfills the unit number of the only interface from the device after the VM is created",
		Label("core-functional", "experimental"), func() {
			key, moRef := env.createPoweredOffVM(ctx, "nic-un-one-", 1)
			units := specUnitNumbers(Default, env.waitForBackfill(ctx, key, 1))
			expectValidUnitNumbers(units)

			By("Verifying the backfilled value is the unit number of the Ethernet card")
			Eventually(func(g Gomega) {
				nics := env.hardwareNICs(ctx, g, moRef)
				g.Expect(nics).To(HaveLen(1))
				g.Expect(hardwareUnitNumbers(nics)).To(Equal(units))
			}, input.Config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed())
		})

	It("backfills unique unit numbers that match the devices for a VM with multiple interfaces",
		Label("core-functional", "experimental"), func() {
			key, moRef := env.createPoweredOffVM(ctx, "nic-un-multi-", 3)
			units := specUnitNumbers(Default, env.waitForBackfill(ctx, key, 3))
			expectValidUnitNumbers(units)

			By("Verifying the set of backfilled values matches the Ethernet cards")
			Eventually(func(g Gomega) {
				nics := env.hardwareNICs(ctx, g, moRef)
				g.Expect(nics).To(HaveLen(3))
				g.Expect(hardwareUnitNumbers(nics)).To(Equal(units))
			}, input.Config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed())
		})

	It("assigns the next free unit number to an interface added after the backfill",
		Label("core-functional", "experimental"), func() {
			skipper.SkipUnlessSupervisorCapabilityEnabled(
				input.Config.SupervisorCapabilities, consts.MutableNetworksCapabilityName)

			key, moRef := env.createPoweredOffVM(ctx, "nic-un-add-", 2)

			// The backfill must have completed first: until then, changes to
			// spec.network.interfaces are rejected.
			vm := env.waitForBackfill(ctx, key, 2)
			before := specUnitNumbers(Default, vm)
			expectValidUnitNumbers(before)

			By("Appending an interface without a unit number")
			// A raw patch is used so that vm is populated with the admitted
			// object, which carries the mutation under test.
			patch := ctrlclient.MergeFrom(vm.DeepCopy())
			added := vm.Spec.Network.Interfaces[0].DeepCopy()
			added.Name = "ethadded"
			added.UnitNumber = nil
			added.MACAddr = ""
			vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, *added)
			Expect(env.client.Patch(ctx, vm, patch)).To(Succeed(),
				"failed to append an interface to VM %s", key)

			By("Verifying admission assigned the lowest free unit number")
			wantNext := lowestFreeNICUnitNumber(before)
			Expect(vm.Spec.Network.Interfaces).To(HaveLen(3))
			Expect(vm.Spec.Network.Interfaces[2].UnitNumber).ToNot(BeNil())
			Expect(*vm.Spec.Network.Interfaces[2].UnitNumber).To(Equal(wantNext))

			By("Verifying the existing interfaces kept their unit numbers")
			current, err := utils.GetVirtualMachine(ctx, env.client, key.Namespace, key.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(specUnitNumbers(Default, current)).To(ConsistOf(append(slices.Clone(before), wantNext)))
			for i := range 2 {
				Expect(current.Spec.Network.Interfaces[i].UnitNumber).To(Equal(
					vm.Spec.Network.Interfaces[i].UnitNumber),
					"interface %d should keep its unit number", i)
			}

			By("Waiting for the new Ethernet card to be added")
			Eventually(func(g Gomega) {
				g.Expect(env.hardwareNICs(ctx, g, moRef)).To(HaveLen(3))
			}, input.Config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed())
		})

	It("keeps unit numbers and devices stable across reconciles and a power cycle",
		Label("extended-functional", "experimental"), func() {
			key, moRef := env.createPoweredOffVM(ctx, "nic-un-stable-", 2)
			units := specUnitNumbers(Default, env.waitForBackfill(ctx, key, 2))
			expectValidUnitNumbers(units)

			var baseline []nicHardware
			Eventually(func(g Gomega) {
				baseline = env.hardwareNICs(ctx, g, moRef)
				g.Expect(baseline).To(HaveLen(2))
				g.Expect(hardwareUnitNumbers(baseline)).To(Equal(units))
			}, input.Config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed())

			By("Verifying nothing changes across reconciles of the powered off VM")
			env.expectStable(ctx, key, moRef, baseline, units)

			By("Powering on the VM")
			vmoperator.PowerOnVirtualMachineAndWait(ctx, input.Config, env.client, key.Namespace, key.Name)
			env.expectStable(ctx, key, moRef, baseline, units)

			By("Power cycling the VM")
			powerCycleVM(ctx, env.client, input.Config, key)

			By("Verifying nothing changes after the power cycle")
			env.expectStable(ctx, key, moRef, baseline, units)
		})
}
