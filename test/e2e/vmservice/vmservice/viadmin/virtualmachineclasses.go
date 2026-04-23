// Copyright (c) 2020 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package viadmin

import (
	"context"
	errpkg "errors"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/kubectl"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	config "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type VIAdminVMClassSpecInput struct {
	ClusterProxy wcpframework.WCPClusterProxyInterface
	Config       *config.E2EConfig
	WCPClient    wcp.WorkloadManagementAPI
}

const VMClassInvalidArg = "Server error: com.vmware.vapi.std.errors.InvalidArgument"

func VIAdminVMClassSpec(ctx context.Context, inputGetter func() VIAdminVMClassSpecInput) {
	var (
		input                                                               VIAdminVMClassSpecInput
		wcpClient                                                           wcp.WorkloadManagementAPI
		createSpecE2eTestBestEffortSmall, createSpecE2eTestGuaranteedXSmall wcp.VMClassSpec
		createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs                wcp.VMClassSpec
		vmClassAsConfigDaynDateFssEnabled                                   bool
		namespacedVMClassFSSEnabled                                         bool
		vmImageRegistryEnabled                                              bool
		config                                                              *config.E2EConfig
	)

	BeforeEach(func() {
		input = inputGetter()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		wcpClient = input.WCPClient
		config = input.Config
		clusterProxy := input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient := clusterProxy.GetClient()
		namespacedVMClassFSSEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSNamespacedVMClass"))
		vmImageRegistryEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMImageRegistry"))
		vmClassAsConfigDaynDateFssEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMClassAsConfigDaynDate"))
		createSpecE2eTestBestEffortSmall = vmservice.CreateSpecE2eTestBestEffortSmall()
		createSpecE2eTestGuaranteedXSmall = vmservice.CreateSpecE2eTestGuaranteedXSmall()

		vmservice.VerifyVMClassCreate(wcpClient, createSpecE2eTestBestEffortSmall, createSpecE2eTestBestEffortSmall)
		vmservice.VerifyVMClassCreate(wcpClient, createSpecE2eTestGuaranteedXSmall, createSpecE2eTestGuaranteedXSmall)
	})

	AfterEach(func() {
		vmservice.VerifyVMClassDeletion(wcpClient, createSpecE2eTestBestEffortSmall.ID)
		vmservice.VerifyVMClassDeletion(wcpClient, createSpecE2eTestGuaranteedXSmall.ID)
	})

	It("VI Admin should have privs to view and edit resources in all namespaces by default", Label("smoke"), func() {
		viAdminKubeConfig := config.InfraConfig.KubeconfigPath
		if namespacedVMClassFSSEnabled {
			kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachineclass", "-A")
		} else {
			// VM Class is cluster scoped resource when WCP_Namespaced_VM_Class disabled
			kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachineclass")
			kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachineclassbinding", "-A")
		}

		if vmImageRegistryEnabled {
			kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "clustervirtualmachineimage")
		}

		kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachineimage", "-A")
		kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachine", "-A")
		kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachineservices", "-A")
		kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "virtualmachinepublishrequests", "-A")
		kubectl.AssertKubectlUserCan(ctx, viAdminKubeConfig, "get", "webconsolerequests", "-A")
	})

	Context("When testing VMClass workflow with valid params", func() {
		It("Should update vmClass with new cpuCount", func() {
			cpuCount := 4
			updateSpec := wcp.VMClassSpec{ID: createSpecE2eTestBestEffortSmall.ID, CPUCount: &cpuCount}
			expectedSpec := createSpecE2eTestBestEffortSmall
			expectedSpec.CPUCount = updateSpec.CPUCount
			VerifyVMClassUpdate(wcpClient, expectedSpec, updateSpec)
		})

		It("Should update vmClass with new description", func() {
			description := "Added description for vmClass"
			updateSpec := wcp.VMClassSpec{ID: createSpecE2eTestBestEffortSmall.ID, Description: &description}
			expectedSpec := createSpecE2eTestBestEffortSmall
			expectedSpec.Description = updateSpec.Description
			VerifyVMClassUpdate(wcpClient, expectedSpec, updateSpec)
		})

		// Add a new block here so we can skip vGPU related tests by setting TEST_SKIP to vGPU.
		Context("VM classes with vGPU", func() {
			BeforeEach(func() {
				// Configure vGPUs on ESX hosts if configuration is available
				esxConfig := vmservice.NewESXConfigFromEnv()
				if esxConfig != nil {
					err := vmservice.EnsureVGPUConfiguration(*esxConfig)
					Expect(err).ToNot(HaveOccurred(), "failed to configure vGPUs on ESX hosts")
				} else {
					e2eframework.Logf("ESX configuration not provided, proceeding with vGPU VM class tests without ESX vGPU setup")
				}

				createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs = vmservice.CreateSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs()

				expectedSpec := createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs
				if vmClassAsConfigDaynDateFssEnabled {
					expectedSpec.ConfigSpec = &types.VirtualMachineConfigSpec{
						DeviceChange: []types.BaseVirtualDeviceConfigSpec{
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
											Vgpu: "mockup-vmiop",
										},
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
											Vgpu: "mockup-vmiop",
										},
									},
								},
							},
						},
					}
				}

				vmservice.VerifyVMClassCreate(wcpClient, createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs, expectedSpec)
			})

			AfterEach(func() {
				vmservice.VerifyVMClassDeletion(wcpClient, createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs.ID)
			})

			It("Should update vmClass with new devices", func() {
				vgpuDevices := []wcp.VGPUDevice{
					{
						ProfileName: "mockup-vmiop",
					},
				}
				virtualDevices := wcp.VirtualDevices{
					VGPUDevices: vgpuDevices,
				}
				updateSpec := wcp.VMClassSpec{ID: createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs.ID, Devices: virtualDevices}
				expectedSpec := createSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs

				expectedSpec.Devices = virtualDevices
				if vmClassAsConfigDaynDateFssEnabled {
					expectedSpec.ConfigSpec = &types.VirtualMachineConfigSpec{
						DeviceChange: []types.BaseVirtualDeviceConfigSpec{
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
											Vgpu: "mockup-vmiop",
										},
									},
								},
							},
						},
					}
				}

				VerifyVMClassUpdate(wcpClient, expectedSpec, updateSpec)
			})
		})

		It("Should list all the vmClasses created in the VC", func() {
			expectedVMClasses := []wcp.VMClassSpec{createSpecE2eTestBestEffortSmall, createSpecE2eTestGuaranteedXSmall}
			VerifyListVMClass(wcpClient, expectedVMClasses)
		})
	})
	Context("When testing VMClasses workflow with invalid params", func() {
		It("Invalid name", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid_name"
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Zero CPU Count", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "zero-cpu-count"
			invalidSpec.CPUCount = new(0)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Zero Memory", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "zero-memory-count"
			invalidSpec.MemoryMB = new(0)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Invalid Memory", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-memory-count"
			invalidSpec.MemoryMB = new(3)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Invalid CPU Reservation", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-cpu-reservation"
			invalidSpec.CPUReservation = new(101)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Negative CPU Reservation", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-cpu-reservation"
			invalidSpec.CPUReservation = new(-1)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Invalid Memory Reservation", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-memory-reservation"
			invalidSpec.MemoryReservation = new(101)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		It("Negative Memory Reservation", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-memory-reservation"
			invalidSpec.MemoryReservation = new(-1)
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})

		// These validation are currently disabled. See https://p4swarm.eng.vmware.com/perforce_1666/changes/12922354.
		XIt("Invalid vGPU Profile", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-vgpu-profile"
			invalidSpec.MemoryReservation = new(100)
			invalidSpec.Devices.VGPUDevices = []wcp.VGPUDevice{
				{
					ProfileName: "dummy-profile",
				},
			}
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
		XIt("Invalid DDPIO Device", func() {
			invalidSpec := createSpecE2eTestBestEffortSmall
			invalidSpec.ID = "invalid-ddpio-devices"
			invalidSpec.MemoryReservation = new(100)
			invalidSpec.Devices.DynamicDirectPathIODevices = []wcp.DynamicDirectPathIODevice{
				{
					DeviceID: 1,
					VendorID: 2,
				},
			}
			VerifyVMClassCreateWithInvalidParams(wcpClient, invalidSpec)
		})
	})
}

func VerifyVMClassCreateWithInvalidParams(wcpClient wcp.WorkloadManagementAPI, createSpec wcp.VMClassSpec) {
	err := wcpClient.CreateVMClass(createSpec)
	Expect(err).Should(HaveOccurred())

	var dcliErr wcp.DcliError
	Expect(errpkg.As(err, &dcliErr)).Should(BeTrue())
	Expect(dcliErr.Response()).Should(ContainSubstring(VMClassInvalidArg))
}

func VerifyVMClassUpdate(wcpClient wcp.WorkloadManagementAPI, expectedSpec, updateSpec wcp.VMClassSpec) {
	err := wcpClient.UpdateVMClass(updateSpec)
	Expect(err).ShouldNot(HaveOccurred())

	updatedVMClass := wcp.VMClassInfo{}

	Eventually(func(g Gomega) {
		updatedVMClass, err = wcpClient.GetVMClassInfo(expectedSpec.ID)
		g.Expect(err).ToNot(HaveOccurred())
	}, 30*time.Second, 3*time.Second).Should(Succeed())
	vmservice.VerifyVMClassSpec(updatedVMClass.VMClassSpec, expectedSpec)
}

func VerifyListVMClass(wcpClient wcp.WorkloadManagementAPI, expectedVMClassSpecs []wcp.VMClassSpec) {
	listVMClasses, err := wcpClient.ListVMClasses()
	Expect(err).ToNot(HaveOccurred())

	var foundClassNames []string

	for _, vmClass := range listVMClasses {
		idx := slices.IndexFunc(expectedVMClassSpecs, func(c wcp.VMClassSpec) bool { return c.ID == vmClass.ID })
		if idx >= 0 {
			Expect(foundClassNames).ToNot(ContainElement(vmClass.ID))
			foundClassNames = append(foundClassNames, vmClass.ID)

			vmservice.VerifyVMClassSpec(vmClass.VMClassSpec, expectedVMClassSpecs[idx])
		}
	}

	Expect(expectedVMClassSpecs).To(HaveLen(len(foundClassNames)))
}

