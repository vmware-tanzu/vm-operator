// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/nsx.vmware.com/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vsphere "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	cvmiKind = "ClusterVirtualMachineImage"
)

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
		// Speed up tests until we Watch the network vimtypes. Sigh.
		network.RetryTimeout = 1 * time.Millisecond

		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vm = builder.DummyBasicVirtualMachineA2("test-vm", "")
		vmClass = builder.DummyVirtualMachineClassA2()
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
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
		// used in VDS and VPC network
		cloudInitSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cloud-init-secret",
				Namespace: nsInfo.Namespace,
			},
			Data: map[string][]byte{
				"user-value": []byte(""),
			},
		}

		// used in NSX-T network
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
			err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
			Expect(err).ToNot(HaveOccurred())

			err = vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
			Expect(err).ToNot(HaveOccurred())

			Expect(vm.Status.UniqueID).ToNot(BeEmpty())
			vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
			Expect(vcVM).ToNot(BeNil())
		})
	})

	Context("VDS", func() {

		const (
			networkName   = "my-vds-network"
			interfaceName = "eth0"
		)

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvVDS
		})

		JustBeforeEach(func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							Name: networkName,
						},
					},
				},
			}
		})

		Context("CloudInit Bootstrap", func() {

			JustBeforeEach(func() {
				Expect(ctx.Client.Create(ctx, cloudInitSecret)).To(Succeed())
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &common.SecretKeySelector{
							Name: cloudInitSecret.Name,
						},
					},
				}

			})

			It("DoIt", func() {
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
				Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())

				By("simulate successful NetOP reconcile", func() {
					netInterface := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
					Expect(netInterface.Spec.NetworkName).To(Equal(networkName))

					netInterface.Status.NetworkID = ctx.NetworkRef.Reference().Value
					netInterface.Status.MacAddress = "" // NetOP doesn't set this.
					netInterface.Status.IPConfigs = []netopv1alpha1.IPConfig{
						{
							IP:         "192.168.1.110",
							IPFamily:   netopv1alpha1.IPv4Protocol,
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
				})

				err = vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				By("has expected conditions", func() {
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				})

				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				devList, err := vcVM.Device(ctx)
				Expect(err).ToNot(HaveOccurred())

				// For now just check the expected Nic backing.
				By("Has expected NIC backing", func() {
					l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					dev1 := l[0].GetVirtualDevice()
					backingInfo, ok := dev1.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).Should(BeTrue())
					Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRef.Reference().Value))
				})
			})
		})
	})

	Context("NSX-T", func() {

		const (
			networkName   = "my-nsxt-network"
			interfaceName = "eth0"
		)

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvNSXT
		})

		JustBeforeEach(func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							Name: networkName,
						},
					},
				},
			}
		})

		Context("Sysprep Bootstrap", func() {

			JustBeforeEach(func() {
				Expect(ctx.Client.Create(ctx, sysprepSecret)).To(Succeed())

				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						RawSysprep: &common.SecretKeySelector{
							Name: sysprepSecret.Name,
							Key:  "unattend",
						},
					},
				}
			})

			It("DoIt", func() {
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
				Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())

				By("simulate successful NCP reconcile", func() {
					netInterface := &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NCPCRName(vm.Name, networkName, interfaceName, false),
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
					Expect(netInterface.Spec.VirtualNetwork).To(Equal(networkName))

					netInterface.Status.MacAddress = "01-23-45-67-89-AB-CD-EF"
					netInterface.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: builder.NsxTLogicalSwitchUUID,
					}
					netInterface.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
						{
							IP:         "192.168.1.110",
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
				})

				err = vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				By("has expected conditions", func() {
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				})

				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				devList, err := vcVM.Device(ctx)
				Expect(err).ToNot(HaveOccurred())

				// For now just check the expected Nic backing.
				By("Has expected NIC backing", func() {
					l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					dev1 := l[0].GetVirtualDevice()
					backingInfo, ok := dev1.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).Should(BeTrue())
					Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRef.Reference().Value))
				})
			})
		})
	})

	Context("VPC", func() {

		const (
			networkName   = "my-vpc-network"
			interfaceName = "eth0"
		)

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvVPC
		})

		JustBeforeEach(func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							Name: networkName,
							TypeMeta: metav1.TypeMeta{
								Kind:       "Subnet",
								APIVersion: "nsx.vmware.com/v1alpha1",
							},
						},
					},
				},
			}
		})

		Context("CloudInit Bootstrap", func() {

			JustBeforeEach(func() {
				Expect(ctx.Client.Create(ctx, cloudInitSecret)).To(Succeed())
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &common.SecretKeySelector{
							Name: cloudInitSecret.Name,
						},
					},
				}
			})

			It("DoIt", func() {
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("subnetPort is not ready yet"))
				Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())

				By("simulate successful NSX Operator reconcile", func() {
					subnetPort := &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
					Expect(subnetPort.Spec.Subnet).To(Equal(networkName))

					subnetPort.Status.NetworkInterfaceConfig.MACAddress = "01-23-45-67-89-AB-CD-EF"
					subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = builder.VPCLogicalSwitchUUID
					subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
						{
							IPAddress: "192.168.1.110/24",
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
				})

				err = vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				By("has expected conditions", func() {
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				})

				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				devList, err := vcVM.Device(ctx)
				Expect(err).ToNot(HaveOccurred())

				// For now just check the expected Nic backing.
				By("Has expected NIC backing", func() {
					l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					dev1 := l[0].GetVirtualDevice()
					backingInfo, ok := dev1.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).Should(BeTrue())
					Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRef.Reference().Value))
				})
			})
		})
	})
}
