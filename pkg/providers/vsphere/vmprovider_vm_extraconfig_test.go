// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	vmconfextraconfig "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/extraconfig"
	vmconfnetworkextraconfig "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/networkextraconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmExtraConfigTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		// Set up vmconfig context so OnResult can be dispatched via the
		// vmconfig framework when TelcoVMServiceAPI is enabled.
		parentCtx = vmconfig.WithContext(parentCtx)
		parentCtx = vmconfig.Register(parentCtx, vmconfextraconfig.New())
		parentCtx = vmconfig.Register(parentCtx, vmconfnetworkextraconfig.New())

		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
			config.Features.TelcoVMServiceAPI = true
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm", "")

		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}
		Expect(ctx.Client.Get(
			ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
			clusterVMI1)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		zoneName := ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false

		vmClass = nil
		vm = nil
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	// getECVal returns the string value of a vSphere ExtraConfig key, or "" if
	// not present.
	getECVal := func(ec []vimtypes.BaseOptionValue, key string) string {
		for _, bov := range ec {
			if kv := bov.GetOptionValue(); kv != nil && kv.Key == key {
				v, _ := kv.Value.(string)
				return v
			}
		}
		return ""
	}

	// getEthernetKeyPrefix returns the ExtraConfig key prefix (e.g. "ethernet-3797.")
	// and the NIC index for the first NIC in hw, derived from its actual device key.
	// vcsim OVF templates assign keys outside the 4000+ range used by real vSphere, so
	// assertions must use the live key rather than assuming "ethernet0.".
	getEthernetKeyPrefix := func(hw []vimtypes.BaseVirtualDevice) (string, int32) {
		GinkgoHelper()
		nics := object.VirtualDeviceList(hw).SelectByType(&vimtypes.VirtualEthernetCard{})
		ExpectWithOffset(1, nics).ToNot(BeEmpty())
		key := nics[0].GetVirtualDevice().Key
		return vmopv1util.EthernetExtraConfigPrefix(key), key - vmopv1util.EthernetDeviceKeyBase
	}

	// createAndUpdate creates the VM then updates it so that the ExtraConfig
	// reconciler has a live moVM.Config to work with on the update pass.
	createAndUpdate := func() *mo.VirtualMachine {
		GinkgoHelper()
		// First pass: creates the VM in vSphere.
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		// Second pass: updates the VM; ExtraConfig reconciler now has a live
		// moVM.Config and can apply spec.advanced fields.
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
		return &o
	}

	It("applies a first-class spec.advanced field to vSphere ExtraConfig", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled: ptr.To(true),
		}
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		o := createAndUpdate()
		Expect(getECVal(o.Config.ExtraConfig, "numa.vcpu.preferHT")).To(Equal("TRUE"))
	})

	It("applies a bag key and writes the managed-keys sentinel to vSphere ExtraConfig", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{
				{Key: "telco.test.key", Value: "hello"},
			},
		}
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		o := createAndUpdate()
		Expect(getECVal(o.Config.ExtraConfig, "telco.test.key")).To(Equal("hello"))
		Expect(getECVal(o.Config.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).
			To(Equal("telco.test.key"))
	})

	It("clears a bag key removed from spec on the subsequent update", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{
				{Key: "telco.test.key", Value: "hello"},
			},
		}
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		// Apply the key first.
		createAndUpdate()

		// Remove the key from spec and trigger another update.
		vm.Spec.Advanced.ExtraConfig = nil
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
		// vSphere removes a key when its value is set to "".
		Expect(getECVal(o.Config.ExtraConfig, "telco.test.key")).To(BeEmpty())
	})

	It("sets VirtualMachineNetworkConfigSynced condition via networkextraconfig OnResult", func() {
		// The network is disabled so no NICs are provisioned. The
		// networkextraconfig reconciler still runs OnResult and marks the
		// condition True when there is nothing blocked.
		createAndUpdate()

		Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineNetworkConfigSynced)).To(BeTrue())
	})

	// nicBeforeEach configures the VM spec for a single eth0 NIC using the
	// named network environment. Must be called from an inner BeforeEach.
	nicBeforeEach := func(iface vmopv1.VirtualMachineNetworkInterfaceSpec) {
		testConfig.WithNetworkEnv = builder.NetworkEnvNamed
		vm.Spec.Network.Disabled = false
		vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{iface}
	}

	When("all NIC properties are set at once", func() {
		BeforeEach(func() {
			nicBeforeEach(vmopv1.VirtualMachineNetworkInterfaceSpec{
				Name:        "eth0",
				Network:     &vmopv1common.PartialObjectRef{Name: "VM Network"},
				VNUMANodeID: ptr.To(int32(2)),
				VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
					UPTv2Enabled:      ptr.To(true),
					CtxPerDev:         ptr.To(vmopv1.TxContextThreadingModePerDevice),
					RSSOffloadEnabled: ptr.To(true),
					UDPRSSEnabled:     ptr.To(vmopv1.UDPRSSModeEnabled),
					PNICFeatures:      []vmopv1.PNICQueueFeature{vmopv1.PNICQueueFeatureReceiveSideScaling},
					CoalescingScheme:  ptr.To(vmopv1.CoalescingSchemeDisabled),
					CoalescingParams:  ptr.To("4000"),
				},
				AdvancedProperties: []vmopv1common.KeyValuePair{
					{Key: "nic.bag.key", Value: "nic-bag-val"},
				},
			})
			// VNUMANodeID requires VMX20 hardware, EFI firmware, and vNUMA topology.
			// UPTv2Enabled additionally requires full memory reservation.
			vm.Spec.MinHardwareVersion = 20
			vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
				Firmware: vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
			}
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: ptr.To(int32(2)),
				},
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
		})

		It("applies all properties to vSphere and reflects them in status", func() {
			// Pass 1+2: provision NIC and apply all properties.
			o := createAndUpdate()

			// DeviceChange: NumaNode and Uptv2Enabled on the hardware device.
			devList := object.VirtualDeviceList(o.Config.Hardware.Device)
			nics := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
			Expect(nics).ToNot(BeEmpty())
			Expect(nics[0].GetVirtualDevice().NumaNode).To(Equal(ptr.To(int32(2))))
			vmxnet3, ok := nics[0].(*vimtypes.VirtualVmxnet3)
			Expect(ok).To(BeTrue(), "NIC should be VirtualVmxnet3")
			Expect(vmxnet3.Uptv2Enabled).ToNot(BeNil())
			Expect(*vmxnet3.Uptv2Enabled).To(BeTrue())

			// ExtraConfig first-class NIC keys (wire formats from vSphere encoders).
			// Use the dynamic prefix because vcsim OVF templates assign device keys
			// outside the 4000+ range used by real vSphere.
			p, nicIdx := getEthernetKeyPrefix(o.Config.Hardware.Device)
			Expect(getECVal(o.Config.ExtraConfig, p+"ctxPerDev")).To(Equal("1"))
			Expect(getECVal(o.Config.ExtraConfig, p+"rssoffload")).To(Equal("TRUE"))
			Expect(getECVal(o.Config.ExtraConfig, p+"udpRSS")).To(Equal("1"))
			Expect(getECVal(o.Config.ExtraConfig, p+"pnicFeatures")).To(Equal("4"))
			Expect(getECVal(o.Config.ExtraConfig, p+"coalescingScheme")).To(Equal("disabled"))
			Expect(getECVal(o.Config.ExtraConfig, p+"coalescingParams")).To(Equal("4000"))

			// AdvancedProperties bag key and managed-keys sentinel.
			Expect(getECVal(o.Config.ExtraConfig,
				fmt.Sprintf(vsphereconst.NICExtraConfigManagedKeysKeyFmt, nicIdx))).
				To(Equal("nic.bag.key"))
			Expect(getECVal(o.Config.ExtraConfig, p+"nic.bag.key")).To(Equal("nic-bag-val"))

			// Pass 3: re-run so OnResult observes the updated moVM.
			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			Expect(vm.Status.Network.Interfaces).ToNot(BeEmpty())
			ifaceStatus := vm.Status.Network.Interfaces[0]
			Expect(ifaceStatus.VNUMANodeID).To(Equal(ptr.To(int32(2))))
			Expect(ifaceStatus.VMXNet3).ToNot(BeNil())
			Expect(ifaceStatus.VMXNet3.UPTv2Enabled).ToNot(BeNil())
			Expect(*ifaceStatus.VMXNet3.UPTv2Enabled).To(BeTrue())
		})
	})

	When("NIC properties are modified and cleared on a subsequent update", func() {
		BeforeEach(func() {
			nicBeforeEach(vmopv1.VirtualMachineNetworkInterfaceSpec{
				Name:    "eth0",
				Network: &vmopv1common.PartialObjectRef{Name: "VM Network"},
				VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
					CtxPerDev:        ptr.To(vmopv1.TxContextThreadingModePerDevice),
					CoalescingScheme: ptr.To(vmopv1.CoalescingSchemeDisabled),
				},
				AdvancedProperties: []vmopv1common.KeyValuePair{
					{Key: "nic.bag.key", Value: "nic-bag-val"},
				},
			})
		})

		It("applies updated values and clears removed properties in vSphere ExtraConfig", func() {
			// Pass 1+2: provision NIC and apply initial properties.
			createAndUpdate()

			// Modify CtxPerDev, clear CoalescingScheme, remove the bag key.
			vm.Spec.Network.Interfaces[0].VMXNet3.CtxPerDev =
				ptr.To(vmopv1.TxContextThreadingModePerQueue)
			vm.Spec.Network.Interfaces[0].VMXNet3.CoalescingScheme = nil
			vm.Spec.Network.Interfaces[0].AdvancedProperties = nil
			Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

			// Pass 3: apply the changed spec.
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			p, nicIdx := getEthernetKeyPrefix(o.Config.Hardware.Device)
			// CtxPerDev updated: PerDevice ("1") → PerQueue ("3").
			Expect(getECVal(o.Config.ExtraConfig, p+"ctxPerDev")).To(Equal("3"))
			// CoalescingScheme cleared; vSphere removes a key when its value is "".
			Expect(getECVal(o.Config.ExtraConfig, p+"coalescingScheme")).To(BeEmpty())
			// Bag key and its managed-keys sentinel cleared.
			Expect(getECVal(o.Config.ExtraConfig, p+"nic.bag.key")).To(BeEmpty())
			Expect(getECVal(o.Config.ExtraConfig,
				fmt.Sprintf(vsphereconst.NICExtraConfigManagedKeysKeyFmt, nicIdx))).To(BeEmpty())
		})
	})
}
