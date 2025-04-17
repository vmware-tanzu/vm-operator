// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

type fakeNetworkProvider interface {
	simulateInterfaceReconcile(ctx *builder.TestContextForVCSim, vm *vmopv1.VirtualMachine, networkName, interfaceName string, idx int)
	assertEthCardBacking(ctx *builder.TestContextForVCSim, dev vimtypes.BaseVirtualDevice, idx int)
	assertNetworkInterfacesDNE(ctx *builder.TestContextForVCSim, vm *vmopv1.VirtualMachine, networkName, interfaceName string)
}

type vdsNetworkProvider struct{}

func (v vdsNetworkProvider) simulateInterfaceReconcile(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine,
	networkName, interfaceName string,
	idx int) {

	netInterface := &netopv1alpha1.NetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
			Namespace: vm.Namespace,
		},
	}
	Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
	Expect(netInterface.Spec.NetworkName).To(Equal(networkName))

	netInterface.Status.NetworkID = ctx.NetworkRefs[idx].Reference().Value
	netInterface.Status.MacAddress = "" // NetOP doesn't set this.
	netInterface.Status.IPConfigs = []netopv1alpha1.IPConfig{
		{
			IP:         fmt.Sprintf("192.168.1.11%d", idx),
			IPFamily:   corev1.IPv4Protocol,
			Gateway:    "192.168.1.1",
			SubnetMask: "255.255.255.0",
		},
	}
	netInterface.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
		{
			Type:   netopv1alpha1.NetworkInterfaceReady,
			Status: corev1.ConditionTrue,
		},
	}
	Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
}

func (v vdsNetworkProvider) assertEthCardBacking(
	ctx *builder.TestContextForVCSim,
	dev vimtypes.BaseVirtualDevice,
	idx int) {

	backing := dev.GetVirtualDevice().Backing
	backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
	Expect(ok).Should(BeTrue())
	Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRefs[idx].Reference().Value))
}

func (v vdsNetworkProvider) assertNetworkInterfacesDNE(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine,
	networkName, interfaceName string) {

	netInterface := &netopv1alpha1.NetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
			Namespace: vm.Namespace,
		},
	}
	err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)
	Expect(err).To(HaveOccurred())
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

type nsxtNetworkProvider struct{}

func (n nsxtNetworkProvider) simulateInterfaceReconcile(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine,
	networkName, interfaceName string,
	idx int) {

	netInterface := &ncpv1alpha1.VirtualNetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.NCPCRName(vm.Name, networkName, interfaceName, false),
			Namespace: vm.Namespace,
		},
	}
	Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
	Expect(netInterface.Spec.VirtualNetwork).To(Equal(networkName))

	netInterface.Status.MacAddress = fmt.Sprintf("01-23-45-67-89-AB-CD-%X", idx)
	netInterface.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
		NsxLogicalSwitchID: builder.GetNsxTLogicalSwitchUUID(idx),
	}
	netInterface.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
		{
			IP:         fmt.Sprintf("192.168.1.11%d", idx),
			Gateway:    "192.168.1.1",
			SubnetMask: "255.255.255.0",
		},
	}
	netInterface.Status.Conditions = []ncpv1alpha1.VirtualNetworkCondition{
		{
			Type:   "Ready",
			Status: "True",
		},
	}
	Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
}

func (n nsxtNetworkProvider) assertEthCardBacking(
	ctx *builder.TestContextForVCSim,
	dev vimtypes.BaseVirtualDevice,
	idx int) {

	backing := dev.GetVirtualDevice().Backing
	backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
	Expect(ok).Should(BeTrue())
	Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRefs[idx].Reference().Value))
}

func (n nsxtNetworkProvider) assertNetworkInterfacesDNE(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine,
	networkName, interfaceName string) {

	netInterface := &ncpv1alpha1.VirtualNetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.NCPCRName(vm.Name, networkName, interfaceName, false),
			Namespace: vm.Namespace,
		},
	}
	err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)
	Expect(err).To(HaveOccurred())
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

type vpcNetworkProvider struct{}

func (v vpcNetworkProvider) simulateInterfaceReconcile(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine,
	networkName, interfaceName string,
	idx int) {

	subnetPort := &vpcv1alpha1.SubnetPort{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
			Namespace: vm.Namespace,
		},
	}
	Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
	Expect(subnetPort.Spec.Subnet).To(Equal(networkName))

	subnetPort.Status.NetworkInterfaceConfig.MACAddress = "01-23-45-67-89-AB-CD-EF"
	subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = builder.GetVPCTLogicalSwitchUUID(idx)
	subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
		{
			IPAddress: fmt.Sprintf("192.168.1.11%d/24", idx),
			Gateway:   "192.168.1.1",
		},
	}
	subnetPort.Status.Conditions = []vpcv1alpha1.Condition{
		{
			Type:   vpcv1alpha1.Ready,
			Status: corev1.ConditionTrue,
		},
	}
	Expect(ctx.Client.Status().Update(ctx, subnetPort)).To(Succeed())
}

func (v vpcNetworkProvider) assertEthCardBacking(
	ctx *builder.TestContextForVCSim,
	dev vimtypes.BaseVirtualDevice,
	idx int) {

	backing := dev.GetVirtualDevice().Backing
	backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
	Expect(ok).Should(BeTrue())
	Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRefs[idx].Reference().Value))
}

func (v vpcNetworkProvider) assertNetworkInterfacesDNE(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine,
	networkName, interfaceName string) {

	subnetPort := &vpcv1alpha1.SubnetPort{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
			Namespace: vm.Namespace,
		},
	}
	err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)
	Expect(err).To(HaveOccurred())
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

// vmE2ETests() tries to close the gap in the existing vmTests() have in the sense that we don't do e2e-like
// tests of the typical VM create/update workflow. This somewhat of a super-set of the vmTests() but those
// tests are already kind of unwieldy and in places, and until we switch over to v1a2, I don't want
// to disturb that file so keeping things in sync easier.
// For now, these tests focus on a real - VDS, NSX-T or VPC - network env w/ cloud init or sysprep,
// and we'll see how these need to evolve.
func vmE2ETests() {

	var (
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm              *vmopv1.VirtualMachine
		vmClass         *vmopv1.VirtualMachineClass
		cloudInitSecret *corev1.Secret
		sysprepSecret   *corev1.Secret
	)

	BeforeEach(func() {
		// Speed up tests until we Watch the network interface types. Sigh.
		network.RetryTimeout = 1 * time.Millisecond

		testConfig = builder.VCSimTestConfig{
			NumNetworks:        2,
			WithContentLibrary: true,
		}

		vm = builder.DummyBasicVirtualMachine("test-vm-e2e", "")
		vmClass = builder.DummyVirtualMachineClassGenName()
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.AsyncSignalEnabled = false
			config.MaxDeployThreadsOnProvider = 1
			config.Features.MutableNetworks = true
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
		Expect(ctx.Client.Status().Update(ctx, vmClass)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = ctx.ContentLibraryImageName
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = ctx.ContentLibraryImageName
		vm.Spec.StorageClass = ctx.StorageClassName
		if vm.Spec.Network != nil {
			vm.Spec.Network.Interfaces[0].Nameservers = []string{"1.1.1.1", "8.8.8.8"}
			vm.Spec.Network.Interfaces[0].SearchDomains = []string{"vmware.local"}
		}

		cloudInitSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cloud-init-secret",
				Namespace: nsInfo.Namespace,
			},
			Data: map[string][]byte{
				"user-value": []byte(""),
			},
		}

		sysprepSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-sysprep-secret",
				Namespace: nsInfo.Namespace,
			},
			Data: map[string][]byte{
				"unattend": []byte("foo"),
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}

		vm = nil
		vmClass = nil
		cloudInitSecret = nil
		sysprepSecret = nil
	})

	Context("Nil fields in Spec", func() {

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvVDS

			vm.Spec.Network = nil
			vm.Spec.Bootstrap = nil
			vm.Spec.Advanced = nil
			vm.Spec.Reserved = nil
		})

		It("DoIt without an NPE", func() {
			err := createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			err = createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			Expect(vm.Status.UniqueID).ToNot(BeEmpty())
			vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
			Expect(vcVM).ToNot(BeNil())
		})
	})

	Context("VM E2E", func() {

		const (
			networkName0   = "my-network-0"
			interfaceName0 = "eth0"
			networkName1   = "my-network-1"
			interfaceName1 = "eth1"

			bsCloudInit = "cloudInit"
			bsSysprep   = "sysPrep"
		)

		DescribeTableSubtree("Simulate VM operations",
			func(networkEnv builder.NetworkEnv, bootstrap string) {
				var np fakeNetworkProvider

				BeforeEach(func() {
					testConfig.WithNetworkEnv = networkEnv

					switch networkEnv {
					case builder.NetworkEnvVDS:
						np = vdsNetworkProvider{}
					case builder.NetworkEnvNSXT:
						np = nsxtNetworkProvider{}
					case builder.NetworkEnvVPC:
						np = vpcNetworkProvider{}
					}
				})

				JustBeforeEach(func() {
					switch bootstrap {
					case bsCloudInit:
						Expect(ctx.Client.Create(ctx, cloudInitSecret)).To(Succeed())
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								RawCloudConfig: &common.SecretKeySelector{
									Name: cloudInitSecret.Name,
								},
							},
						}
					case bsSysprep:
						Expect(ctx.Client.Create(ctx, sysprepSecret)).To(Succeed())
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{
									Name: sysprepSecret.Name,
									Key:  "unattend",
								},
							},
						}
					}

					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name: interfaceName0,
								Network: &common.PartialObjectRef{
									Name: networkName0,
								},
							},
						},
					}

					if networkEnv == builder.NetworkEnvVPC {
						vm.Spec.Network.Interfaces[0].Network.Kind = "Subnet"
						vm.Spec.Network.Interfaces[0].Network.APIVersion = "crd.nsx.vmware.com/v1alpha1"
					}
				})

				It("DoIt", func() {
					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
					Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())

					By("simulate successful network provider reconcile", func() {
						np.simulateInterfaceReconcile(ctx, vm, networkName0, interfaceName0, 0)
					})

					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

					By("has expected conditions", func() {
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
						if bootstrap != "" {
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						} else {
							Expect(conditions.Get(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeNil())
						}
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
					})

					Expect(vm.Status.UniqueID).ToNot(BeEmpty())
					vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)

					By("Has expected NIC backing", func() {
						devList, err := vcVM.Device(ctx)
						Expect(err).ToNot(HaveOccurred())
						l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
						Expect(l).To(HaveLen(1))

						dev0 := l[0].GetVirtualDevice()
						np.assertEthCardBacking(ctx, dev0, 0)
					})

					By("add network interface", func() {
						vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, vm.Spec.Network.Interfaces[0])
						vm.Spec.Network.Interfaces[1].Name = interfaceName1
						vm.Spec.Network.Interfaces[1].Network = ptr.To(*vm.Spec.Network.Interfaces[1].Network)
						vm.Spec.Network.Interfaces[1].Network.Name = networkName1
					})

					// NOTE: network changes only checked during power on
					By("power off and on VM", func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
					})

					By("simulate successful network provider reconcile on added interface", func() {
						np.simulateInterfaceReconcile(ctx, vm, networkName1, interfaceName1, 1)
					})

					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

					By("Added interface has expected NIC backing", func() {
						devList, err := vcVM.Device(ctx)
						Expect(err).ToNot(HaveOccurred())
						l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
						Expect(l).To(HaveLen(2))

						dev1 := l[1].GetVirtualDevice()
						np.assertEthCardBacking(ctx, dev1, 1)
					})

					By("remove just added network interface", func() {
						vm.Spec.Network.Interfaces = vm.Spec.Network.Interfaces[:1]
					})

					By("power off and on VM", func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					})

					By("interface has been removed", func() {
						devList, err := vcVM.Device(ctx)
						Expect(err).ToNot(HaveOccurred())
						l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
						Expect(l).To(HaveLen(1))

						dev0 := l[0].GetVirtualDevice()
						np.assertEthCardBacking(ctx, dev0, 0)

						By("network interface has been deleted", func() {
							np.assertNetworkInterfacesDNE(ctx, vm, networkName1, interfaceName1)
						})
					})
				})
			},
			Entry("VDS with CloudInit", builder.NetworkEnvVDS, bsCloudInit),
			Entry("NSX-T with CloudInit", builder.NetworkEnvNSXT, bsCloudInit),
			Entry("VPC with Sysprep", builder.NetworkEnvVPC, bsSysprep),
		)
	})
}
