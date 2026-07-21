// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	mopv1a2 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/csi"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// VMHardwareSpecInput is the input for the VM Hardware test spec.
type VMHardwareSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
	SkipCleanup      bool
}

func createPvcsFromSpec(
	input VMHardwareSpecInput,
	prefix string,
	spec manifestbuilders.PVC,
	count int,
) []manifestbuilders.PVC {
	pvcs := []manifestbuilders.PVC{}

	for i := range count {
		volumePrefix := fmt.Sprintf("%s-pvc-%s-%d",
			strings.ToLower(prefix), capiutil.RandomString(4), i)
		pvcs = append(pvcs, manifestbuilders.PVC{
			Namespace:           input.WCPNamespaceName,
			VolumeName:          fmt.Sprintf("%s-volume", volumePrefix),
			ClaimName:           fmt.Sprintf("%s-claim", volumePrefix),
			StorageClassName:    spec.StorageClassName,
			RequestSize:         "1Mi",
			ControllerType:      spec.ControllerType,
			ControllerBusNumber: spec.ControllerBusNumber,
			SharingMode:         spec.SharingMode,
			AccessModes:         spec.AccessModes,
			VolumeMode:          spec.VolumeMode,
			ApplicationType:     spec.ApplicationType,
			UnitNumber:          spec.UnitNumber,
		})
	}

	return pvcs
}

type testSpec struct {
	pvcs     []manifestbuilders.PVC
	hardware vmopv1.VirtualMachineHardwareSpec
}

func waitForVMAndBatchAttach(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	svClusterClient ctrlclient.Client,
	vmSvcNamespace,
	vmPrefix string,
	expectedVolumes []string,
) *vmopv1.VirtualMachine {
	By("Waiting for the Virtual Machine to be created")
	vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, vmSvcNamespace, vmPrefix)

	By("Waiting for the batch attachment volumes to be attached")
	csi.WaitForBatchAttachVolumesToBeAttached(ctx, config, svClusterClient, vmSvcNamespace, vmPrefix, expectedVolumes)

	By("Waiting for Virtual Machine to Power On")
	vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmPrefix, "PoweredOn")

	vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmPrefix)
	Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
	Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")

	return vm
}

// getBackfilledVolumes returns a list of backfilled volume names when the "all disks are PVCs"
// capability is enabled. If the capability is not enabled, it returns an empty list.
func getBackfilledVolumes(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	svClusterClient ctrlclient.Client,
	vmSvcNamespace,
	vmName string,
	allDisksArePVCapabilityEnabled bool,
) []string {
	if !allDisksArePVCapabilityEnabled {
		return []string{}
	}

	// Wait for both conditions
	conditions := []metav1.Condition{
		{
			Type:   consts.VMUnmanagedVolumesBackfilledCondition,
			Status: metav1.ConditionTrue,
		},
		{
			Type:   consts.VMUnmanagedVolumesRegisteredCondition,
			Status: metav1.ConditionTrue,
		},
	}
	for _, condition := range conditions {
		vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, vmName, condition)
	}

	// Get the VM and extract backfilled volumes
	var backfilledVolumes []string

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

		backfilledVolumes = []string{}

		for _, vol := range vm.Spec.Volumes {
			// When VirtualMachineUnmanagedVolumesRegistered is true, all volumes should have PVC references
			g.Expect(vol.PersistentVolumeClaim).ToNot(BeNil(),
				"Volume %s should have PersistentVolumeClaim when VirtualMachineUnmanagedVolumesRegistered is true", vol.Name)

			// Backfilled volumes have removable: false
			// Removable defaults to true, so we only include volumes where it's explicitly false
			if vol.Removable != nil && !*vol.Removable {
				backfilledVolumes = append(backfilledVolumes, vol.Name)
			}
		}

		// Verify all backfilled volumes are in vm.Status.Volumes with type Managed
		for _, backfilledVolName := range backfilledVolumes {
			found := false

			for _, volStatus := range vm.Status.Volumes {
				if volStatus.Name == backfilledVolName {
					g.Expect(volStatus.Type).To(Equal(vmopv1.VolumeTypeManaged),
						"Expected backfilled volume %s to have type Managed", backfilledVolName)

					found = true

					break
				}
			}

			g.Expect(found).To(BeTrue(), "Expected backfilled volume %s to be found in vm.Status.Volumes", backfilledVolName)
		}
	}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
		Should(Succeed(), "Timed out waiting to get VirtualMachine %s and extract backfilled volumes", vmName)

	return backfilledVolumes
}

func verifyCreatedControllersCount(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	svClusterClient ctrlclient.Client,
	vmSvcNamespace,
	vmName string,
	expectedControllersCount map[vmopv1.VirtualControllerType]int,
) {
	Eventually(func(g Gomega) bool {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		createdControllersCount := make(map[vmopv1.VirtualControllerType]int)
		for _, controller := range vm.Status.Hardware.Controllers {
			createdControllersCount[controller.Type]++
		}

		pass := true

		for controllerType, expectedCount := range expectedControllersCount {
			actualCount, ok := createdControllersCount[controllerType]
			if !ok {
				actualCount = 0
			}

			if actualCount != expectedCount {
				pass = false

				e2eframework.Logf("unexpected number of %s controllers: expected %d, got %d",
					controllerType, expectedCount, actualCount)
			}
		}

		return pass
	}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
		Should(BeTrue(), "Timed out waiting for VirtualMachines %s to be updated", vmName)
}

// addSharedMultiWriterDisk cold-adds a ParaVirtual SCSI controller with
// physical bus sharing (bus 1) and a 10 MB multi-writer shared disk backed by
// storagePolicyID to vm. The VM must be powered off; multi-writer shared disks
// require a cold reconfigure to avoid "Incompatible device backing" errors.
func addSharedMultiWriterDisk(ctx context.Context, vm *object.VirtualMachine, storagePolicyID string) error {
	var moVM mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"config.hardware.device", "datastore"}, &moVM); err != nil {
		return err
	}

	Expect(moVM.Datastore).ToNot(BeEmpty(), "VM has no datastores")
	datastoreRef := moVM.Datastore[0]

	sharedScsiController := &vimtypes.ParaVirtualSCSIController{
		VirtualSCSIController: vimtypes.VirtualSCSIController{
			SharedBus: vimtypes.VirtualSCSISharingPhysicalSharing,
			VirtualController: vimtypes.VirtualController{
				BusNumber: 1,
				VirtualDevice: vimtypes.VirtualDevice{
					Key: -1,
				},
			},
		},
	}

	sharedDisk := &vimtypes.VirtualDisk{
		VirtualDevice: vimtypes.VirtualDevice{
			Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
				DiskMode: string(vimtypes.VirtualDiskModePersistent),
				Sharing:  string(vimtypes.VirtualDiskSharingSharingMultiWriter),
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					Datastore: &datastoreRef,
				},
			},
			ControllerKey: sharedScsiController.Key,
			UnitNumber:    vimtypes.NewInt32(0),
		},
		CapacityInBytes: 10 * 1024 * 1024,
	}

	configSpec := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    sharedScsiController,
			},
			// EZT (Eager Zeroed Thick) allocation avoids "Incompatible device backing"
			// errors that occur with thin provisioning on multi-writer shared disks.
			&vimtypes.VirtualDeviceConfigSpec{
				Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
				FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
				Device:        sharedDisk,
				Profile: []vimtypes.BaseVirtualMachineProfileSpec{
					&vimtypes.VirtualMachineDefinedProfileSpec{
						ProfileId: storagePolicyID,
					},
				},
			},
		},
		ExtraConfig: []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{
				Key:   "test.shared.disk.import",
				Value: "true",
			},
		},
	}

	task, err := vm.Reconfigure(ctx, configSpec)
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

func VMHardwareSpec(ctx context.Context, inputGetter func() VMHardwareSpecInput) {
	const (
		specName              = "vm-hardware"
		eztStorageProfileName = "vmservice-ezt-storage-profile"
		isoImageDisplayName   = "ubuntu-24.04-live-server-amd64"
	)

	var (
		input                          VMHardwareSpecInput
		vmSvcNamespace                 string
		config                         *e2eConfig.E2EConfig
		clusterProxy                   *common.VMServiceClusterProxy
		svClusterClient                ctrlclient.Client
		clusterResources               *e2eConfig.Resources
		vCenterClient                  *vim25.Client
		vmYamls                        [][]byte
		pvcsYamls                      [][]byte
		vmName                         string
		isoSupportFSSEnabled           bool
		allDisksArePVCapabilityEnabled bool
		linuxImageDisplayName          string
		linuxVMIName                   string
		eztStoragePolicyID             string
	)

	Context("VMs with attached hardware", Ordered, func() {
		BeforeAll(func() {
			input = inputGetter()
			Expect(input.Config).ToNot(BeNil(),
				"Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
			Expect(input.Config.InfraConfig).ToNot(BeNil(),
				"Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
			Expect(input.Config.InfraConfig.ManagementClusterConfig).ToNot(BeNil(),
				"Invalid argument. input.E2EConfig.InfraConfig.ManagementClusterConfig can't be nil when calling %s spec",
				specName)
			clusterResources = input.Config.InfraConfig.ManagementClusterConfig.Resources

			Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(),
				"Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

			Expect(input.ClusterProxy).ToNot(BeNil(),
				"Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)

			Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
				"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
			vmSvcNamespace = input.WCPNamespaceName

			config = input.Config

			wcpClient = input.WCPClient
			kubeconfigPath := input.ClusterProxy.GetKubeconfigPath()

			clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
			svClusterClient = clusterProxy.GetClient()
			svClientSet := clusterProxy.GetClientSet()

			cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, svClientSet, filepath.Join(input.ArtifactFolder, specName))
			DeferCleanup(cancelPodWatches)

			linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)

			linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, vmSvcNamespace, linuxImageDisplayName)

			isoSupportFSSEnabled = utils.IsFssEnabled(ctx,
				svClusterClient,
				config.GetVariable("VMOPNamespace"),
				config.GetVariable("VMOPDeploymentName"),
				config.GetVariable("VMOPManagerCommand"),
				config.GetVariable("EnvFSSIsoSupport"),
			)

			asyncSupervisorFSSEnabled, err := utils.CheckSupervisorCapabilitiesCRDSupport(ctx, svClusterClient)
			Expect(err).NotTo(HaveOccurred())

			allDisksArePVCapabilityEnabled = utils.IsSupervisorCapabilityEnabled(
				ctx,
				clusterProxy.GetClientSet(),
				clusterProxy.GetDynamicClient(),
				consts.AllDisksArePVCapabilityName,
				asyncSupervisorFSSEnabled)

			vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, kubeconfigPath)
			defer vcenter.LogoutVimClient(vCenterClient)

			By("Creating EZT storage policy for multi-writer PVCs")
			// Get the base WCP storage class to extract its policy ID.
			storageClass, err := svClientSet.StorageV1().StorageClasses().
				Get(ctx, clusterResources.StorageClassName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to get storage class %s", clusterResources.StorageClassName)

			basePolicyID := storageClass.Parameters["storagePolicyID"]
			Expect(basePolicyID).NotTo(BeEmpty(), "Storage class %s does not have storagePolicyID parameter",
				clusterResources.StorageClassName)

			// Create or get the EZT storage policy.
			eztStoragePolicyID, err = vcenter.GetOrCreateEZTStoragePolicy(ctx, vCenterClient, eztStorageProfileName,
				basePolicyID)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to create EZT storage policy")
			Expect(eztStoragePolicyID).ShouldNot(BeEmpty(), "EZT storage policy ID is empty")
			e2eframework.Logf("EZT storage policy created/found with ID: %s", eztStoragePolicyID)

			By("Configure namespace with EZT storage policy")

			details, err := wcpClient.GetNamespace(vmSvcNamespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to get namespace %s", vmSvcNamespace)

			// Check if EZT policy is already in the namespace
			policyExists := slices.ContainsFunc(details.VMStorageSpec, func(spec wcp.StorageSpec) bool {
				return spec.Policy == eztStoragePolicyID
			})

			if !policyExists {
				details.VMStorageSpec = append(details.VMStorageSpec, wcp.StorageSpec{Policy: eztStoragePolicyID})
				Expect(wcpClient.SetNamespaceStorageSpecs(vmSvcNamespace, details.VMStorageSpec)).
					Should(Succeed(), "Failed to set storage specs for namespace %s", vmSvcNamespace)
				wcp.WaitForNamespaceReady(wcpClient, vmSvcNamespace)
				e2eframework.Logf("EZT storage policy added to namespace %s", vmSvcNamespace)
			} else {
				e2eframework.Logf("EZT storage policy already exists in namespace %s", vmSvcNamespace)
			}

			By("Ensure EZT storage class is available in namespace")

			podVMOnStretchedSupervisorEnabled := utils.IsFssEnabled(ctx, svClusterClient,
				config.GetVariable("VMOPNamespace"),
				config.GetVariable("VMOPDeploymentName"),
				config.GetVariable("VMOPManagerCommand"),
				config.GetVariable("EnvFSSPodVMOnStretchedSupervisor"))
			utils.EnsureStorageClassInNamespace(ctx, svClusterClient, vmSvcNamespace,
				eztStorageProfileName, podVMOnStretchedSupervisorEnabled, *config)
			e2eframework.Logf(
				"EZT storage class %s is available in namespace %s",
				eztStorageProfileName, vmSvcNamespace,
			)

			skipper.SkipUnlessInfraIs(config.InfraConfig.InfraName, consts.WCP)
			skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.SharedDisksCapabilityName)
		})

		BeforeEach(func() {
			vmYamls = [][]byte{}
			pvcsYamls = [][]byte{}
			vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		})

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				By("Logging Virtual Machines and Batch Attachments after failure")

				for _, vmYaml := range vmYamls {
					var virtualMachine manifestbuilders.VirtualMachineYaml
					if err := yaml.Unmarshal(vmYaml, &virtualMachine); err != nil {
						e2eframework.Logf("Failed to parse VM yaml: %s", err)
						continue
					}

					vmName := virtualMachine.Name

					stdout, err := framework.KubectlGet(ctx,
						clusterProxy.GetKubeconfigPath(),
						"virtualmachine", vmName,
						"-n", virtualMachine.Namespace,
						"-oyaml")
					if err != nil {
						e2eframework.Logf("Failed to get VirtualMachine %s: %s", vmName, err)
					} else {
						e2eframework.Logf("VirtualMachine %s:\n%s", vmName, stdout)
					}

					stdout, err = framework.KubectlGet(ctx,
						clusterProxy.GetKubeconfigPath(),
						"batchattach", vmName,
						"-n", virtualMachine.Namespace,
						"-oyaml")
					if err != nil {
						e2eframework.Logf("Failed to get batch attachment: %s", err)
					} else {
						e2eframework.Logf("Batch Attachment %s:\n%s", vmName, stdout)
					}
				}

				stdout, err := framework.KubectlGet(ctx, clusterProxy.GetKubeconfigPath(),
					"pvc",
					"-n", vmSvcNamespace,
					"-oyaml")
				if err != nil {
					e2eframework.Logf("Failed to get PVCs: %s", err)
				} else {
					e2eframework.Logf("PVCs:\n%s", stdout)
				}
			}

			for _, vmYaml := range vmYamls {
				_ = clusterProxy.DeleteWithArgs(ctx, vmYaml)
			}

			for _, pvcYaml := range pvcsYamls {
				_ = clusterProxy.DeleteWithArgs(ctx, pvcYaml)
			}
		})

		DescribeTable("Virtual Machine with PVCs Power On",
			func(specGetter func() testSpec) {
				spec := specGetter()

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             spec.pvcs,
					Hardware:         &spec.hardware,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine")

				By("Waiting for the VirtualMachine to be created")

				volumeNames := make([]string, len(spec.pvcs))
				for i, pvc := range spec.pvcs {
					volumeNames[i] = pvc.VolumeName
				}
				// Get backfilled volumes and add them to volumeNames
				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				vm := waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Waiting on virtual machine conditions to become true")

				conditions := []metav1.Condition{
					{
						Type:   "VirtualMachineHardwareControllersVerified",
						Status: metav1.ConditionTrue,
					},
					{
						Type:   "VirtualMachineHardwareDeviceConfigVerified",
						Status: metav1.ConditionTrue,
					},
					{
						Type:   "VirtualMachineHardwareVolumesVerified",
						Status: metav1.ConditionTrue,
					},
				}

				for _, condition := range conditions {
					vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, vmName, condition)
				}

				statusControllerDevices := make(map[vmopv1.VirtualControllerType]map[int32]int)
				for _, controller := range vm.Status.Hardware.Controllers {
					if _, ok := statusControllerDevices[controller.Type]; !ok {
						statusControllerDevices[controller.Type] = make(map[int32]int)
					}

					statusControllerDevices[controller.Type][controller.BusNumber] = len(controller.Devices)
				}

				By("verifying 2 IDE controllers")
				// The mutation webhook always creates 2 IDE controllers.
				Expect(statusControllerDevices[vmopv1.VirtualControllerTypeIDE]).To(HaveLen(2), "Expected 2 IDE controllers")

				By("verifying explicit controllers")

				expectedScsiControllerDevicesCount := 0
				expectedDeviceCount := make(map[vmopv1.VirtualControllerType]map[int32]int)

				for _, pvc := range spec.pvcs {
					if pvc.ControllerType == nil || pvc.ControllerBusNumber == nil {
						// Unassigned PVCs are attached to SCSI controllers.
						expectedScsiControllerDevicesCount++
						continue
					}

					if *pvc.ControllerType == vmopv1.VirtualControllerTypeSCSI {
						expectedScsiControllerDevicesCount++
					}

					if _, ok := expectedDeviceCount[*pvc.ControllerType]; !ok {
						expectedDeviceCount[*pvc.ControllerType] = make(map[int32]int)
					}

					expectedDeviceCount[*pvc.ControllerType][*pvc.ControllerBusNumber]++
				}

				for controllerType, busNumberToDeviceCount := range expectedDeviceCount {
					for busNumber, deviceCount := range busNumberToDeviceCount {
						devices, ok := statusControllerDevices[controllerType][busNumber]
						Expect(ok).To(BeTrue(), "Expected controller %s:%d to be found in status",
							controllerType, busNumber)
						Expect(devices).To(BeNumerically(">=", deviceCount),
							"Expected at least %d devices for controller %s:%d in status, got %d",
							deviceCount, controllerType, busNumber, devices)
					}
				}

				totalStatusScsiControllerDevicesCount := 0

				for _, controller := range vm.Status.Hardware.Controllers {
					if controller.Type == vmopv1.VirtualControllerTypeSCSI {
						totalStatusScsiControllerDevicesCount += len(controller.Devices)
					}
				}

				By("verifying implicit PVCs are attached to the SCSI controller")
				// We check for greater than or equal because the image may have additional devices.
				Expect(totalStatusScsiControllerDevicesCount).To(BeNumerically(">=", expectedScsiControllerDevicesCount),
					"Expected at least %d devices for SCSI controller in status, got %d",
					expectedScsiControllerDevicesCount, totalStatusScsiControllerDevicesCount)

				By("verifying the volume fields")

				expectedVolumes := make(map[string]manifestbuilders.PVC)
				for _, pvc := range spec.pvcs {
					expectedVolumes[pvc.VolumeName] = pvc
				}

				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					g.Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

					bootDiskFound := false

					for _, pvc := range vm.Status.Volumes {
						g.Expect(pvc.DiskUUID).To(Not(BeEmpty()), "Expected volume %s to have a disk UUID", pvc.Name)
						g.Expect(pvc.Name).To(Not(BeEmpty()), "Expected volume name to be not empty")
						g.Expect(pvc.Attached).To(BeTrue(), "Expected volume %s to be attached", pvc.Name)
						g.Expect(pvc.ControllerType).To(Not(BeEmpty()), "Expected volume %s to have a controller type", pvc.Name)
						g.Expect(pvc.ControllerBusNumber).To(Not(BeNil()), "Expected volume %s to have a controller bus number", pvc.Name)
						g.Expect(pvc.UnitNumber).To(Not(BeNil()), "Expected volume %s to have a unit number", pvc.Name)

						g.Expect(pvc.Limit).To(Not(BeNil()), "Expected volume %s to have a limit", pvc.Name)
						_, ok := pvc.Limit.AsInt64()
						g.Expect(ok).To(BeTrue(), "Expected volume %s to have a limit", pvc.Name)

						g.Expect(pvc.Requested).To(Not(BeNil()), "Expected volume %s to have a requested", pvc.Name)
						_, ok = pvc.Requested.AsInt64()
						g.Expect(ok).To(BeTrue(), "Expected volume %s to have a requested", pvc.Name)

						g.Expect(pvc.Used).To(Not(BeNil()), "Expected volume %s to have a used", pvc.Name)
						_, ok = pvc.Used.AsInt64()
						g.Expect(ok).To(BeTrue(), "Expected volume %s to have a used", pvc.Name)

						expectedPvc, ok := expectedVolumes[pvc.Name]

						if *pvc.ControllerBusNumber == 0 && *pvc.UnitNumber == 0 {
							bootDiskFound = true

							if allDisksArePVCapabilityEnabled {
								g.Expect(pvc.Type).To(Equal(vmopv1.VolumeTypeManaged),
									"Expected volume %s to have a type of %s", pvc.Name, vmopv1.VolumeTypeManaged)
							} else {
								// boot disk is not expect going to be in the manifest.
								g.Expect(pvc.Type).To(Equal(vmopv1.VolumeTypeClassic),
									"Expected volume %s to have a type of %s", pvc.Name, vmopv1.VolumeTypeClassic)
							}
						} else {
							g.Expect(ok).To(BeTrue(), "Expected volume %s to be found in expected volumes", pvc.Name)
							g.Expect(pvc.Type).To(Equal(vmopv1.VolumeTypeManaged),
								"Expected volume %s to have a type of %s", pvc.Name, vmopv1.VolumeTypeManaged)

							if expectedPvc.ControllerType != nil {
								g.Expect(pvc.ControllerType).To(Equal(*expectedPvc.ControllerType),
									"Expected volume %s to have a controller type of %s", pvc.Name, *expectedPvc.ControllerType)
							}

							if expectedPvc.ControllerBusNumber != nil {
								g.Expect(pvc.ControllerBusNumber).To(Equal(expectedPvc.ControllerBusNumber),
									"Expected volume %s to have a controller bus number of %d", pvc.Name, *expectedPvc.ControllerBusNumber)
							}

							if expectedPvc.UnitNumber != nil {
								g.Expect(pvc.UnitNumber).To(Equal(expectedPvc.UnitNumber),
									"Expected volume %s to have a unit number of %d", pvc.Name, *expectedPvc.UnitNumber)
							}
						}
					}

					g.Expect(bootDiskFound).To(BeTrue(), "Expected boot disk to be found")
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for the volume fields to be updated")
			},
			Entry("create a virtual machine with a single PVC", Label("smoke"), func() testSpec {
				return testSpec{
					pvcs: createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
						StorageClassName: clusterResources.StorageClassName,
					}, 1),
				}
			}),
			Entry("create a virtual machine with a combination of placements, controller types, and sharing modes", func() testSpec {
				pvcs := []manifestbuilders.PVC{}

				// We are adding 5 PVCs without explicit assignment.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 5)...)

				// We are adding a PVC with a persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModePersistent)),
				}, 1)...)

				// We are adding a PVC with a independent persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModeIndependentPersistent)),
				}, 1)...)

				// We are adding a PVC with a independent non persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModeIndependentNonPersistent)),
				}, 1)...)

				// We are adding a PVC with a non persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModeNonPersistent)),
				}, 1)...)

				// We are adding a PVC explicitly assigned to a SCSI:1:1.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    clusterResources.StorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(1)),
					UnitNumber:          ptr.To(int32(1)),
				}, 1)...)

				// We are adding a PVC explicitly assigned to SCSI:1 and without a unit number.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    clusterResources.StorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(1)),
				}, 1)...)

				// TODO(Faisal A): Enable this after fixing the issues with the NVME controller status.
				// We are adding a PVC explicitly assigned to NVME:1 and without a unit number.
				// pvcs = append(pvcs, createPvcsFromSpec(input, vmPrefix, manifestbuilders.PVC{
				// 	StorageClassName:    clusterResources.StorageClassName,
				// 	ControllerType:      ptr.To(vmopv1.VirtualControllerTypeNVME),
				// 	ControllerBusNumber: ptr.To(int32(1)),
				// }, 1)...)

				// // We are adding a PVC explicitly assigned to SATA:1 and without a unit number.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    clusterResources.StorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSATA),
					ControllerBusNumber: ptr.To(int32(1)),
				}, 1)...)

				return testSpec{
					pvcs: pvcs,
					hardware: vmopv1.VirtualMachineHardwareSpec{
						SCSIControllers: []vmopv1.SCSIControllerSpec{
							{
								BusNumber: 1,
								Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
							},
							{
								BusNumber: 2,
								Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
							},
							{
								BusNumber:   3,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
						},
						NVMEControllers: []vmopv1.NVMEControllerSpec{
							{
								BusNumber: 1,
							},
						},
						SATAControllers: []vmopv1.SATAControllerSpec{
							{
								BusNumber: 1,
							},
						},
					},
				}
			}),
			Entry("create a virtual machine with a combination of placements, controller types, and sharing modes with ezt", func() testSpec {
				vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vCenterClient)

				isVSANEnabled, err := vcenter.IsVSANEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())
				isVSANDEnabled, err := vcenter.IsVSANDEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())

				if isVSANDEnabled || isVSANEnabled {
					Skip("Skipping EZT storage profile tests as VSAN Datastore is present")
				}

				pvcs := []manifestbuilders.PVC{}

				// We are adding 5 PVCs without explicit assignment.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 5)...)

				// We are adding a PVC with a persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModePersistent)),
				}, 1)...)

				// We are adding a PVC with a independent persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModeIndependentPersistent)),
				}, 1)...)

				// We are adding a PVC with a independent non persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModeIndependentNonPersistent)),
				}, 1)...)

				// We are adding a PVC with a non persistent disk mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
					DiskMode:         ptr.To(string(vmopv1.VolumeDiskModeNonPersistent)),
				}, 1)...)

				// We are adding a PVC explicitly assigned to a SCSI:1:1.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    clusterResources.StorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(1)),
					UnitNumber:          ptr.To(int32(1)),
				}, 1)...)

				// We are adding a PVC explicitly assigned to a SCSI:2:2 with a multi-writer sharing mode.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    eztStorageProfileName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(2)),
					UnitNumber:          ptr.To(int32(2)),
					SharingMode:         ptr.To(string(vmopv1.VolumeSharingModeMultiWriter)),
					AccessModes:         []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:          ptr.To(corev1.PersistentVolumeBlock),
				}, 1)...)

				// We are adding a PVC explicitly assigned to SCSI:1 and without a unit number.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    clusterResources.StorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(1)),
				}, 1)...)

				// We are adding a PVC explicitly assigned to SCSI:3:0 with a SCSI controller
				// that has physical sharing mode defined in the VM below.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    eztStorageProfileName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(3)),
					UnitNumber:          ptr.To(int32(0)),
					AccessModes:         []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:          ptr.To(corev1.PersistentVolumeBlock),
				}, 1)...)

				// We are adding a PVC with only application type set to OracleRAC.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: eztStorageProfileName,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:       ptr.To(corev1.PersistentVolumeBlock),
					ApplicationType:  vmopv1.VolumeApplicationTypeOracleRAC,
				}, 1)...)

				// We are adding a PVC with only application type set to MicrosoftWSFC.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    eztStorageProfileName,
					AccessModes:         []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:          ptr.To(corev1.PersistentVolumeBlock),
					ApplicationType:     vmopv1.VolumeApplicationTypeMicrosoftWSFC,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(3)),
				}, 1)...)

				// TODO(Faisal A): Enable this after fixing the issues with the NVME controller status.
				// We are adding a PVC explicitly assigned to NVME:1 and without a unit number.
				// pvcs = append(pvcs, createPvcsFromSpec(input, vmPrefix, manifestbuilders.PVC{
				// 	StorageClassName:    clusterResources.StorageClassName,
				// 	ControllerType:      ptr.To(vmopv1.VirtualControllerTypeNVME),
				// 	ControllerBusNumber: ptr.To(int32(1)),
				// }, 1)...)

				// // We are adding a PVC explicitly assigned to SATA:1 and without a unit number.
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    clusterResources.StorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSATA),
					ControllerBusNumber: ptr.To(int32(1)),
				}, 1)...)

				return testSpec{
					pvcs: pvcs,
					hardware: vmopv1.VirtualMachineHardwareSpec{
						SCSIControllers: []vmopv1.SCSIControllerSpec{
							{
								BusNumber: 1,
								Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
							},
							{
								BusNumber: 2,
								Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
							},
							{
								BusNumber:   3,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
						},
						NVMEControllers: []vmopv1.NVMEControllerSpec{
							{
								BusNumber: 1,
							},
						},
						SATAControllers: []vmopv1.SATAControllerSpec{
							{
								BusNumber: 1,
							},
						},
					},
				}
			}),
			Entry("creating multiple implicit PVCs", func() testSpec {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 64+1) // Full bus number slots plus one.

				return testSpec{
					pvcs: pvcs,
				}
			}),
		)

		Describe("Controller lifecycle", func() {
			DescribeTable("Adding/Deleting controllers",
				func(vmPowerState vmopv1.VirtualMachinePowerState) {
					hardware := vmopv1.VirtualMachineHardwareSpec{
						SCSIControllers: []vmopv1.SCSIControllerSpec{
							{
								BusNumber: 0,
							},
							{
								BusNumber: 1,
								Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
							},
							{
								BusNumber: 2,
								Type:      vmopv1.SCSIControllerTypeLsiLogic,
							},
						},
						NVMEControllers: []vmopv1.NVMEControllerSpec{
							{
								BusNumber: 1,
							},
							{
								BusNumber:   2,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
						},
						SATAControllers: []vmopv1.SATAControllerSpec{
							{
								BusNumber: 1,
							},
						},
						// We do not create IDE controllers because they are created by default.
					}

					vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
						Namespace:        vmSvcNamespace,
						Name:             vmName,
						ImageName:        linuxVMIName,
						VMClassName:      clusterResources.VMClassName,
						StorageClassName: clusterResources.StorageClassName,
						PowerState:       string(vmPowerState),
						Hardware:         &hardware,
					})
					vmYamls = append(vmYamls, vmYaml)

					By("Create and wait for VM to power on")
					Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")
					vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, string(vmPowerState))

					By("Verifying the controllers are created")
					verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, vmName,
						map[vmopv1.VirtualControllerType]int{
							vmopv1.VirtualControllerTypeSCSI: len(hardware.SCSIControllers),
							vmopv1.VirtualControllerTypeSATA: len(hardware.SATAControllers),
							vmopv1.VirtualControllerTypeNVME: len(hardware.NVMEControllers),
							vmopv1.VirtualControllerTypeIDE:  2,
						},
					)

					By("Deleting controller from each type")

					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
					Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")
					vmPatch := vm.DeepCopy()
					vmPatch.Spec.Hardware.SCSIControllers = vmPatch.Spec.Hardware.SCSIControllers[:len(vmPatch.Spec.Hardware.SCSIControllers)-1]
					vmPatch.Spec.Hardware.SATAControllers = vmPatch.Spec.Hardware.SATAControllers[:len(vmPatch.Spec.Hardware.SATAControllers)-1]
					vmPatch.Spec.Hardware.NVMEControllers = vmPatch.Spec.Hardware.NVMEControllers[:len(vmPatch.Spec.Hardware.NVMEControllers)-1]

					Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
						To(Succeed(), "failed to patch virtualmachine")
					verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, vmName,
						map[vmopv1.VirtualControllerType]int{
							vmopv1.VirtualControllerTypeSCSI: len(vmPatch.Spec.Hardware.SCSIControllers),
							vmopv1.VirtualControllerTypeSATA: len(vmPatch.Spec.Hardware.SATAControllers),
							vmopv1.VirtualControllerTypeNVME: len(vmPatch.Spec.Hardware.NVMEControllers),
							vmopv1.VirtualControllerTypeIDE:  2,
						})

					By("Creating new controller for each type")

					vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
					Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")
					vmPatch = vm.DeepCopy()
					vmPatch.Spec.Hardware.SCSIControllers = append(vmPatch.Spec.Hardware.SCSIControllers, hardware.SCSIControllers[len(hardware.SCSIControllers)-1])
					vmPatch.Spec.Hardware.SATAControllers = append(vmPatch.Spec.Hardware.SATAControllers, hardware.SATAControllers[len(hardware.SATAControllers)-1])
					vmPatch.Spec.Hardware.NVMEControllers = append(vmPatch.Spec.Hardware.NVMEControllers, hardware.NVMEControllers[len(hardware.NVMEControllers)-1])

					Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
						To(Succeed(), "failed to patch virtualmachine")
					verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, vmName,
						map[vmopv1.VirtualControllerType]int{
							vmopv1.VirtualControllerTypeSCSI: len(vmPatch.Spec.Hardware.SCSIControllers),
							vmopv1.VirtualControllerTypeSATA: len(vmPatch.Spec.Hardware.SATAControllers),
							vmopv1.VirtualControllerTypeNVME: len(vmPatch.Spec.Hardware.NVMEControllers),
							vmopv1.VirtualControllerTypeIDE:  2,
						})
				},
				Entry("while VM is powered on", vmopv1.VirtualMachinePowerStateOn),
				Entry("while VM is powered off", vmopv1.VirtualMachinePowerStateOff),
			)

			It("Updating controller while VM is powered off should succeed", func() {
				hardware := vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber: 0,
						},
					},
					NVMEControllers: []vmopv1.NVMEControllerSpec{
						{
							BusNumber: 1,
						},
					},
				}

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PowerState:       string(vmopv1.VirtualMachinePowerStateOff),
					Hardware:         &hardware,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, string(vmopv1.VirtualMachinePowerStateOff))

				By("Verifying the controllers are created")
				verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					map[vmopv1.VirtualControllerType]int{
						vmopv1.VirtualControllerTypeSCSI: len(hardware.SCSIControllers),
						vmopv1.VirtualControllerTypeNVME: len(hardware.NVMEControllers),
					})

				By("Updating the controllers")

				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
				Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")
				vmPatch := vm.DeepCopy()
				vmPatch.Spec.Hardware.SCSIControllers[0].SharingMode = vmopv1.VirtualControllerSharingModePhysical
				vmPatch.Spec.Hardware.SCSIControllers[0].Type = vmopv1.SCSIControllerTypeLsiLogic

				vmPatch.Spec.Hardware.NVMEControllers[0].SharingMode = vmopv1.VirtualControllerSharingModePhysical

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine")

				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					metav1.Condition{
						Type:   "VirtualMachineHardwareControllersVerified",
						Status: metav1.ConditionTrue,
					},
				)
			})

			It("Updating controller while VM is powered on should fail", func() {
				hardware := vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber: 0,
						},
					},
				}

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PowerState:       string(vmopv1.VirtualMachinePowerStateOn),
					Hardware:         &hardware,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, string(vmopv1.VirtualMachinePowerStateOn))

				By("Verifying the controllers are created")
				verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					map[vmopv1.VirtualControllerType]int{
						vmopv1.VirtualControllerTypeSCSI: len(hardware.SCSIControllers),
					})

				By("Updating the controllers")

				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
				Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")
				vmPatch := vm.DeepCopy()
				vmPatch.Spec.Hardware.SCSIControllers[0].SharingMode = vmopv1.VirtualControllerSharingModePhysical

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(MatchError(ContainSubstring("spec.hardware.scsiControllers[0].sharingMode: Forbidden")),
						"expected error when updating controller while VM is powered on")
			})
		})

		Describe("Multiple VMs sharing MultiWriter PVCs", func() {
			numberOfVMs := 3

			AfterEach(func() {
				// We delete the VMs one after the other instead of relying on
				// deleting the entire yaml because the PVCs are shared between the VMs
				// and we need to ensure that VMs are deleted before PVCs.
				for i := range numberOfVMs {
					vmName := fmt.Sprintf("%s-%d", vmName, i)

					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					if err != nil {
						e2eframework.Logf("failed to get VirtualMachine %s: %v", vmName, err)
						continue
					}

					err = svClusterClient.Delete(ctx, vm)
					if err != nil {
						e2eframework.Logf("failed to delete VirtualMachine %s: %v", vmName, err)
					}
				}
			})

			It("VMs should power on successfully", func() {
				vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vCenterClient)

				isVSANEnabled, err := vcenter.IsVSANEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())
				isVSANDEnabled, err := vcenter.IsVSANDEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())

				if isVSANDEnabled || isVSANEnabled {
					Skip("Skipping EZT storage profile tests as VSAN Datastore is present")
				}

				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: eztStorageProfileName,
					SharingMode:      ptr.To(string(vmopv1.VolumeSharingModeMultiWriter)),
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:       ptr.To(corev1.PersistentVolumeBlock),
				}, 3)
				pvcs = append(pvcs, createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: eztStorageProfileName,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:       ptr.To(corev1.PersistentVolumeBlock),
					ApplicationType:  vmopv1.VolumeApplicationTypeOracleRAC,
				}, 1)...)

				var vms []string
				for i := range numberOfVMs {
					vms = append(vms, fmt.Sprintf("%s-%d", vmName, i))
				}

				for _, vmName := range vms {
					By("Creating Virtual Machine: " + vmName)
					vmYaml := manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
						Namespace:        vmSvcNamespace,
						Name:             vmName,
						ImageName:        linuxVMIName,
						VMClassName:      clusterResources.VMClassName,
						StorageClassName: clusterResources.StorageClassName,
						PVCs:             pvcs,
					})
					vmYamls = append(vmYamls, vmYaml)

					Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")
				}

				for _, vmName := range vms {
					By("Waiting for the Virtual Machine to be created: " + vmName)
					vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, vmSvcNamespace, vmName)

					By("Waiting for the batch attachment volumes to be attached: " + vmName)

					volumeNames := make([]string, len(pvcs))
					for i, pvc := range pvcs {
						volumeNames[i] = pvc.VolumeName
					}
					// Get backfilled volumes and add them to volumeNames
					backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
					volumeNames = append(volumeNames, backfilledVolumes...)
					csi.WaitForBatchAttachVolumesToBeAttached(ctx, config, svClusterClient, vmSvcNamespace, vmName,
						volumeNames)

					By("Waiting for Virtual Machine to Power On: " + vmName)
					vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName,
						"PoweredOn")
				}
			})
		})

		Describe("VM with volumes day two actions", func() {
			It("Hot attach and detaching PVs should succeed", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 3)

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             pvcs,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				volumeNames := make([]string, len(pvcs))
				for i, pvc := range pvcs {
					volumeNames[i] = pvc.VolumeName
				}
				// Get backfilled volumes and add them to volumeNames
				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				vm := waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Detaching one of the PVCs from the Virtual Machine")

				volumeNameToDetach := pvcs[len(pvcs)-1].VolumeName
				volumesAfterDetach := make(map[string]bool)

				for _, vol := range pvcs {
					// Exclude the volume being detached
					if vol.VolumeName != volumeNameToDetach {
						volumesAfterDetach[vol.VolumeName] = true
					}
				}
				// Include backfilled volumes as they should remain attached
				for _, backfilledVol := range backfilledVolumes {
					volumesAfterDetach[backfilledVol] = true
				}

				e2eframework.Logf("Detaching volume %s from VM %s",
					volumeNameToDetach, vmName)

				vmPatch := vm.DeepCopy()
				vmPatch.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

				var detachedVol vmopv1.VirtualMachineVolume

				for _, vol := range vm.Spec.Volumes {
					if vol.Name == volumeNameToDetach {
						detachedVol = vol
					} else {
						vmPatch.Spec.Volumes = append(vmPatch.Spec.Volumes, vol)
					}
				}

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine")

				// Convert map to sorted slice
				volumeNamesList := make([]string, 0, len(volumesAfterDetach))
				for volumeName := range volumesAfterDetach {
					volumeNamesList = append(volumeNamesList, volumeName)
				}

				vm = waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					volumeNamesList)

				By("Attaching the PVC back to the Virtual Machine")

				vmPatch = vm.DeepCopy()
				vmPatch.Spec.Volumes = append(vmPatch.Spec.Volumes, detachedVol)

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine")

				By("Waiting for the batch attachment volumes to be attached")
				waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					volumeNames)
			})

			It("PVC resize should succeed", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 1)

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             pvcs,
				})
				vmYamls = append(vmYamls, vmYaml)

				By("Creating the Virtual Machine")
				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				By("Waiting for the Virtual Machine to be created")

				volumeNames := make([]string, len(pvcs))
				for i, pvc := range pvcs {
					volumeNames[i] = pvc.VolumeName
				}
				// Get backfilled volumes and add them to volumeNames
				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				vm := waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Verify persistent volume capacity is 1MB")

				found := false

				for _, volStatus := range vm.Status.Volumes {
					if volStatus.Name == pvcs[0].VolumeName {
						Expect(volStatus.Limit.String()).To(Equal("1Mi"))

						found = true

						break
					}
				}

				Expect(found).To(BeTrue(), "Expected volume %s to be found in the VirtualMachine status",
					pvcs[0].VolumeName, vm.Status.Volumes)

				By("Resizing the PVC: " + pvcs[0].ClaimName)

				pvc := &corev1.PersistentVolumeClaim{}
				key := types.NamespacedName{
					Namespace: vmSvcNamespace,
					Name:      pvcs[0].ClaimName,
				}
				err := svClusterClient.Get(ctx, key, pvc)
				Expect(err).ToNot(HaveOccurred(), "failed to get PVC")
				Expect(pvc).ToNot(BeNil(), "PVC is nil")

				pvcPatch := pvc.DeepCopy()
				pvcPatch.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("5Mi")

				Expect(clusterProxy.GetClient().Patch(ctx, pvcPatch, ctrlclient.MergeFrom(pvc))).
					To(Succeed(), "failed to update PVC")

				By("Waiting for the Virtual Machine to be resized")
				Eventually(func(g Gomega) bool {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					if err != nil {
						e2eframework.Logf("retry due to: %v", err)
						return false
					}

					for _, volStatus := range vm.Status.Volumes {
						if volStatus.Name == pvcs[0].VolumeName {
							return g.Expect(volStatus.Limit.String()).To(Equal("5Mi"))
						}
					}

					return false
				}, config.GetIntervals("default", "wait-virtual-machine-resize")...).
					Should(BeTrue(), "Timed out waiting for VirtualMachines %s to be resized to 5MB", vmName)
			})

			It("Updating attached volume claim should succeed", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 2)
				// Updating the request sizes to test swapping the claims.
				pvcs[0].RequestSize = "1Mi"
				pvcs[1].RequestSize = "2Mi"

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             pvcs,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				By("Waiting for the Virtual Machine to be created")

				volumeNames := make([]string, len(pvcs))
				for i, pvc := range pvcs {
					volumeNames[i] = pvc.VolumeName
				}
				// Get backfilled volumes and add them to volumeNames
				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				vm := waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Swapping the persistent volume claim order")

				vmPatch := vm.DeepCopy()
				firstClaimName := pvcs[0].ClaimName
				secondClaimName := pvcs[1].ClaimName
				swapped := 0
				// We have to find and swap by name because order is not guaranteed in the spec.
				for i, vol := range vmPatch.Spec.Volumes {
					switch vol.Name {
					case pvcs[0].VolumeName:
						vmPatch.Spec.Volumes[i].PersistentVolumeClaim.ClaimName = secondClaimName
						swapped++
					case pvcs[1].VolumeName:
						vmPatch.Spec.Volumes[i].PersistentVolumeClaim.ClaimName = firstClaimName
						swapped++
					}
				}

				Expect(swapped).To(Equal(2), "Expected to swap 2 volumes, got %d", swapped)

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine")

				By("Waiting for the batch attachment volumes to be updated")
				waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					volumeNames)

				By("Verify persistent volume claim order is swapped")
				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					if err != nil {
						e2eframework.Logf("retry due to: %v", err)
						g.Expect(err).ToNot(HaveOccurred())
					}

					foundCount := 0

					for _, vol := range vm.Status.Volumes {
						switch vol.Name {
						case pvcs[0].VolumeName:
							g.Expect(vol.Requested.String()).To(Equal("2Mi"))

							foundCount++
						case pvcs[1].VolumeName:
							foundCount++

							g.Expect(vol.Requested.String()).To(Equal("1Mi"))
						}
					}

					g.Expect(foundCount).
						To(Equal(2), "Expected to find 2 volumes in the VirtualMachine status, got %d", foundCount)
				}, config.GetIntervals("default", "wait-virtual-machine-resize")...).
					Should(Succeed(),
						"Timed out waiting for VirtualMachines %s to have the persistent volume claim order swapped",
						vmName)
			})

			It("Powering VM on/off and deleting VM should succeed", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 1)

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             pvcs,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				By("Waiting for the Virtual Machine to be created and powered on")

				volumeNames := make([]string, len(pvcs))
				for i, pvc := range pvcs {
					volumeNames[i] = pvc.VolumeName
				}
				// Get backfilled volumes and add them to volumeNames
				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				vm := waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Powering off the Virtual Machine")

				vmPatch := vm.DeepCopy()
				vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine")
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, "PoweredOff")

				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
				Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")

				By("Powering on the Virtual Machine")

				vmPatch = vm.DeepCopy()
				vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine")
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, "PoweredOn")

				By("Deleting the Virtual Machine")
				Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmSvcNamespace, vmName)

				By("Verifying the CnsNodeVMBatchAttachment is deleted")
				csi.WaitForCnsNodeVMBatchAttachmentToBeDeleted(ctx, config, svClusterClient, vmSvcNamespace, vmName)
			})

			It("Detaching and reattaching all disks", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: clusterResources.StorageClassName,
				}, 3)

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             pvcs,
				})
				vmYamls = append(vmYamls, vmYaml)

				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				volumeNames := make([]string, len(pvcs))
				for i, pvc := range pvcs {
					volumeNames[i] = pvc.VolumeName
				}
				// Get backfilled volumes and add them to volumeNames
				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Waiting on virtual machine conditions to become true")

				conditions := []metav1.Condition{
					{
						Type:   "VirtualMachineHardwareVolumesVerified",
						Status: metav1.ConditionTrue,
					},
				}
				for _, condition := range conditions {
					vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, vmName, condition)
				}

				By("Storing the initial volume spec and status")
				// Re-fetch to get the latest resource version before patching.
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
				Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")

				savedSpecVolumes := make([]vmopv1.VirtualMachineVolume, len(vm.Spec.Volumes))
				copy(savedSpecVolumes, vm.Spec.Volumes)

				savedStatusVolumes := make([]vmopv1.VirtualMachineVolumeStatus, len(vm.Status.Volumes))
				copy(savedStatusVolumes, vm.Status.Volumes)
				vmopv1.SortVirtualMachineVolumeStatuses(savedStatusVolumes)

				// Build a lookup from DiskUUID -> saved status for field-by-field comparison.
				savedStatusByDiskUUID := make(map[string]vmopv1.VirtualMachineVolumeStatus, len(savedStatusVolumes))
				for _, vol := range savedStatusVolumes {
					savedStatusByDiskUUID[vol.DiskUUID] = vol
				}

				By("Powering off the VM and setting all volumes to removable")

				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

				vmPatch := vm.DeepCopy()

				vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				for i := range vmPatch.Spec.Volumes {
					vmPatch.Spec.Volumes[i].Removable = ptr.To(true)
				}

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine removable fields")
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, "PoweredOff")

				By("Detaching all volumes")

				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

				vmPatch = vm.DeepCopy()
				vmPatch.Spec.Volumes = []vmopv1.VirtualMachineVolume{}
				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine to detach all volumes")

				By("Waiting for status.volumes to be empty after detach")
				Eventually(func(g Gomega) {
					vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					g.Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
					g.Expect(vm.Status.Volumes).To(BeEmpty(),
						"Expected status.volumes to be empty after detaching all volumes")
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for status.volumes to be empty after detach")

				By("Waiting on virtual machine conditions to become true after detach")

				for _, condition := range conditions {
					vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, vmName, condition)
				}

				By("Reattaching all previously detached volumes")

				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

				vmPatch = vm.DeepCopy()
				vmPatch.Spec.Volumes = savedSpecVolumes
				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine to reattach volumes")

				By("Waiting for the batch attachment volumes to be attached")
				csi.WaitForBatchAttachVolumesToBeAttached(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Waiting on virtual machine conditions to become true after reattach")

				for _, condition := range conditions {
					vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, vmName, condition)
				}

				By("Powering on the Virtual Machine after reattaching volumes")

				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

				vmPatch = vm.DeepCopy()
				vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to patch virtualmachine power state to on")
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, "PoweredOn")

				By("Verifying status.volumes matches the originally recorded state")
				Eventually(func(g Gomega) {
					vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					g.Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")

					actualStatusVolumes := make([]vmopv1.VirtualMachineVolumeStatus, len(vm.Status.Volumes))
					copy(actualStatusVolumes, vm.Status.Volumes)
					vmopv1.SortVirtualMachineVolumeStatuses(actualStatusVolumes)

					g.Expect(actualStatusVolumes).To(HaveLen(len(savedStatusVolumes)),
						"Expected %d volumes in status, got %d", len(savedStatusVolumes), len(actualStatusVolumes))

					for _, cur := range actualStatusVolumes {
						saved, ok := savedStatusByDiskUUID[cur.DiskUUID]
						g.Expect(ok).To(BeTrue(), "volume with DiskUUID %q not found in saved status", cur.DiskUUID)

						g.Expect(cur.Name).To(Equal(saved.Name), "volume %q: Name mismatch", cur.DiskUUID)
						g.Expect(cur.Type).To(Equal(saved.Type), "volume %q: Type mismatch", cur.DiskUUID)
						g.Expect(cur.Attached).To(Equal(saved.Attached), "volume %q: Attached mismatch", cur.DiskUUID)
						g.Expect(cur.ControllerType).To(Equal(saved.ControllerType), "volume %q: ControllerType mismatch", cur.DiskUUID)
						g.Expect(cur.ControllerBusNumber).To(Equal(saved.ControllerBusNumber), "volume %q: ControllerBusNumber mismatch", cur.DiskUUID)
						g.Expect(&cur.UnitNumber).To(Equal(&saved.UnitNumber), "volume %q: UnitNumber mismatch", cur.DiskUUID)
						g.Expect(cur.DiskMode).To(Equal(saved.DiskMode), "volume %q: DiskMode mismatch", cur.DiskUUID)
						g.Expect(cur.SharingMode).To(Equal(saved.SharingMode), "volume %q: SharingMode mismatch", cur.DiskUUID)
						g.Expect(&cur.Limit).To(Equal(&saved.Limit), "volume %q: Limit mismatch", cur.DiskUUID)
						g.Expect(&cur.Requested).To(Equal(&saved.Requested), "volume %q: Requested mismatch", cur.DiskUUID)
						g.Expect(&cur.Crypto).To(Equal(&saved.Crypto), "volume %q: Crypto mismatch", cur.DiskUUID)
						g.Expect(&cur.Error).To(Equal(&saved.Error), "volume %q: Error mismatch", cur.DiskUUID)

						// Used represents bytes-on-disk and may differ between attach cycles;
						// just verify it is still populated if it was originally.
						if saved.Used != nil {
							g.Expect(cur.Used).ToNot(BeNil(), "volume %q: Used should still be populated after reattach", cur.Name)
						}
					}
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for status.volumes to match the original state after reattach")
			})
		})

		DescribeTable("VM with CD-ROM",
			func(cdrom vmopv1.VirtualMachineCdromSpec) {
				if !isoSupportFSSEnabled {
					Skip("ISO Support FSS is not enabled")
					return
				}

				By("Get the ISO-type image CR name")

				isoImageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, vmSvcNamespace, isoImageDisplayName)

				cdrom.Image = vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: isoImageName,
				}

				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					GuestID:          "ubuntu64Guest",
					StorageClassName: clusterResources.StorageClassName,
					Hardware: &vmopv1.VirtualMachineHardwareSpec{
						Cdrom: []vmopv1.VirtualMachineCdromSpec{
							cdrom,
						},
					},
				})
				vmYamls = append(vmYamls, vmYaml)

				By("Creating the Virtual Machine")
				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				By("Waiting for the Virtual Machine to be created and power on")
				vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, vmSvcNamespace, vmName)
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, vmName, "PoweredOn")

				By("Verifying the CD-ROMs are attached to the first IDE controller")

				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine")
				Expect(vm).ToNot(BeNil(), "VirtualMachine is nil")

				found := false

				for _, controller := range vm.Status.Hardware.Controllers {
					if controller.Type == vmopv1.VirtualControllerTypeIDE && controller.BusNumber == 0 {
						Expect(controller.Devices).To(HaveLen(1), "Expected 1 device on the controller")
						Expect(controller.Devices[0].Type).To(Equal(vmopv1.VirtualDeviceTypeCDROM), "Expected device type to be CDROM")

						if cdrom.UnitNumber != nil {
							Expect(controller.Devices[0].UnitNumber).To(Equal(*cdrom.UnitNumber), "Expected device unit number to be %d", *cdrom.UnitNumber)
						} else {
							Expect(controller.Devices[0].UnitNumber).To(BeNumerically(">=", 0), "Expected device unit number to be >= 0")
						}

						found = true

						break
					}
				}

				Expect(found).To(BeTrue(), "Expected to find the first IDE controller")
			},
			Entry("VM with implicit CD-ROM placement", vmopv1.VirtualMachineCdromSpec{
				Name:              "cdrom1",
				Connected:         ptr.To(true),
				AllowGuestControl: ptr.To(true),
			}),
			Entry("VM with explicit CD-ROM placement", vmopv1.VirtualMachineCdromSpec{
				Name:                "cdrom1",
				Connected:           ptr.To(true),
				AllowGuestControl:   ptr.To(true),
				ControllerBusNumber: ptr.To(int32(0)),
				ControllerType:      vmopv1.VirtualControllerTypeIDE,
				UnitNumber:          ptr.To(int32(0)),
			}),
		)

		Describe("VM with boot disk as PVC", func() {
			BeforeEach(func() {
				skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.AllDisksArePVCapabilityName)
			})

			It("Boot disk PVC lifecycle operations should succeed", func() {
				vmYaml = manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             []manifestbuilders.PVC{},
				})
				vmYamls = append(vmYamls, vmYaml)

				By("Creating the Virtual Machine")
				e2eframework.Logf("Creating the Virtual Machine with yaml: %s", string(vmYaml))
				Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply virtualmachine")

				waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, []string{})

				bootDiskVolName, currentVM := vmoperator.WaitForBootDiskPVC(ctx, config, svClusterClient, vmSvcNamespace, vmName)
				volumeNames := make([]string, 0, len(currentVM.Spec.Volumes))
				for _, vol := range currentVM.Spec.Volumes {
					volumeNames = append(volumeNames, vol.Name)
				}

				By("Verify volumes in batch attachment CRD")
				csi.WaitForBatchAttachVolumesToBeAttached(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Verifying the boot disk PVC exists")

				bootDiskPVC := &corev1.PersistentVolumeClaim{}
				bootDiskPVCKey := types.NamespacedName{
					Namespace: vmSvcNamespace,
					Name:      bootDiskVolName,
				}

				Eventually(func(g Gomega) {
					err := svClusterClient.Get(ctx, bootDiskPVCKey, bootDiskPVC)
					g.Expect(err).ToNot(HaveOccurred(), "failed to get boot disk PVC")
					g.Expect(bootDiskPVC).ToNot(BeNil(), "boot disk PVC is nil")
					g.Expect(bootDiskPVC.Status.Phase).To(Equal(corev1.ClaimBound),
						"Expected boot disk PVC to be bound")
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for boot disk PVC to be found in status")

				By("Verifying the boot disk becomes managed PVC")
				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					g.Expect(err).ToNot(HaveOccurred())

					var bootDiskVolume *vmopv1.VirtualMachineVolumeStatus

					for i, volStatus := range vm.Status.Volumes {
						if volStatus.Name == bootDiskVolName {
							bootDiskVolume = &vm.Status.Volumes[i]
							break
						}
					}

					g.Expect(bootDiskVolume).ToNot(BeNil(), "Expected to find boot disk volume in status")
					g.Expect(bootDiskVolume.Type).To(Equal(vmopv1.VolumeTypeManaged),
						"Expected boot disk to be of type Managed (PVC)")
					g.Expect(bootDiskVolume.Name).ToNot(BeEmpty(), "Expected boot disk volume to have a name")
					g.Expect(bootDiskVolume.Attached).To(BeTrue(), "Expected boot disk to be attached")
					g.Expect(bootDiskVolume.DiskUUID).ToNot(BeEmpty(), "Expected boot disk to have a disk UUID")

					g.Expect(bootDiskVolume.Limit).ToNot(BeNil(), "Expected volume %s to have a limit", bootDiskVolume.Name)
					_, ok := bootDiskVolume.Limit.AsInt64()
					g.Expect(ok).To(BeTrue(), "Expected volume %s limit to be convertible to int64", bootDiskVolume.Name)

					g.Expect(bootDiskVolume.Requested).ToNot(BeNil(), "Expected volume %s to have a requested", bootDiskVolume.Name)
					_, ok = bootDiskVolume.Requested.AsInt64()
					g.Expect(ok).To(BeTrue(), "Expected volume %s requested to be convertible to int64", bootDiskVolume.Name)

					g.Expect(bootDiskVolume.Used).ToNot(BeNil(), "Expected volume %s to have a used", bootDiskVolume.Name)
					_, ok = bootDiskVolume.Used.AsInt64()
					g.Expect(ok).To(BeTrue(), "Expected volume %s used to be convertible to int64", bootDiskVolume.Name)
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for boot disk to become managed PVC")

				By("Creating a new PVC with a different storage class and larger size")
				vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vCenterClient)

				isVSANEnabled, err := vcenter.IsVSANEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())
				isVSANDEnabled, err := vcenter.IsVSANDEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())

				if isVSANDEnabled || isVSANEnabled {
					Skip("Skipping boot disk storage class replacement: EZT storage profile requires non-VSAN datastore")
				}

				// Create a new PVC with a different storage class and larger size.
				newBootDiskPVCName := fmt.Sprintf("%s-new-boot-disk", vmName)
				newBootDiskPVCSize := bootDiskPVC.Spec.Resources.Requests[corev1.ResourceStorage]
				newBootDiskPVCSize.Add(resource.MustParse("1Gi"))
				newBootDiskPVC := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      newBootDiskPVCName,
						Namespace: vmSvcNamespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: newBootDiskPVCSize,
							},
						},
						StorageClassName: ptr.To(eztStorageProfileName),
					},
				}
				Expect(svClusterClient.Create(ctx, newBootDiskPVC)).
					To(Succeed(), "failed to create new boot disk PVC with different storage class")

				By("Updating the VM to use the new PVC with different storage class")

				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
				Expect(err).ToNot(HaveOccurred(), "failed to get VM")

				vmPatch := vm.DeepCopy()
				for i, vol := range vmPatch.Spec.Volumes {
					if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == bootDiskVolName {
						vmPatch.Spec.Volumes[i].PersistentVolumeClaim.ClaimName = newBootDiskPVCName
						break
					}
				}

				Expect(clusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
					To(Succeed(), "failed to update VM with new PVC")

				By("Waiting for the new PVC to be attached and reflected in VM status")
				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, vmName)
					g.Expect(err).ToNot(HaveOccurred())

					verifiedInStatus := false

					for _, volStatus := range vm.Status.Volumes {
						if volStatus.Name == bootDiskVolName {
							verifiedInStatus = true

							g.Expect(volStatus.Attached).To(BeTrue())
							g.Expect(volStatus.Limit.Cmp(newBootDiskPVCSize) >= 0).To(BeTrue())
						}
					}

					g.Expect(verifiedInStatus).To(BeTrue(), "Expected to find the new boot disk in status")
				}, config.GetIntervals("default", "wait-virtual-machine-resize")...).
					Should(Succeed(), "Timed out waiting for new PVC to be attached")
			})
		})

		Describe("Brownfield VM Import", func() {
			var (
				vCenterAdminClient *vim25.Client
				clusterMoID        string
				brownfieldVMName   string
				brownfieldVMMoID   string
				importOperation    *mopv1a2.ImportOperation
			)

			BeforeEach(func() {
				// Get vCenter client with admin credentials.
				kubeconfigPath := clusterProxy.GetKubeconfigPath()
				vCenterHostname := vcenter.GetVCPNIDFromKubeconfigFile(ctx, kubeconfigPath)

				var err error

				vCenterAdminClient, err = vcenter.NewVimClient(vCenterHostname, testbed.AdminUsername, testbed.AdminPassword)
				Expect(err).ToNot(HaveOccurred(), "Failed to create vCenter client")

				clusterMoID = GetClusterMoIDForNamespace(ctx, svClusterClient, vmSvcNamespace)
				brownfieldVMName = fmt.Sprintf("brownfield-vm-hardware-%s", capiutil.RandomString(4))
			})

			AfterEach(func() {
				CleanupBrownfieldVM(ctx, BrownfieldVMCleanupInput{
					VCenterAdminClient: vCenterAdminClient,
					BrownfieldVMName:   brownfieldVMName,
					BrownfieldVMMoID:   brownfieldVMMoID,
					ImportOperation:    importOperation,
					SVClusterClient:    svClusterClient,
				})
				vcenter.LogoutVimClient(vCenterAdminClient)
			})

			It("Import brownfield VM with hardware should succeed", func() {
				By("Creating a brownfield VM by deploying from content library template using govmomi")

				result := ImportBrownfieldVM(ImportBrownfieldVMInput{
					Ctx:                ctx,
					Config:             config,
					VCenterAdminClient: vCenterAdminClient,
					SVClusterClient:    svClusterClient,
					Namespace:          vmSvcNamespace,
					ClusterMoID:        clusterMoID,
					BrownfieldVMName:   brownfieldVMName,
					StorageClassName:   clusterResources.StorageClassName,
					VMClassName:        clusterResources.VMClassName,
					ImportOpName:       fmt.Sprintf("import-%s", vmName),
					BeforePowerOn: func(ctx context.Context, brownfieldVM *object.VirtualMachine) error {
						By("Adding additional hardware to the brownfield VM before power-on and import")

						var moVM mo.VirtualMachine
						if err := brownfieldVM.Properties(ctx, brownfieldVM.Reference(),
							[]string{"config.hardware.device", "datastore"}, &moVM); err != nil {
							return fmt.Errorf("failed to get VM properties: %w", err)
						}

						var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

						// Add a ParaVirtual SCSI controller (bus 1).
						pvscsiController := &vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								SharedBus: vimtypes.VirtualSCSISharingNoSharing,
								VirtualController: vimtypes.VirtualController{
									BusNumber: 1,
									VirtualDevice: vimtypes.VirtualDevice{
										Key: -1,
									},
								},
							},
						}
						deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device:    pvscsiController,
						})
						e2eframework.Logf("Adding ParaVirtual SCSI controller (bus 1)")

						// Add a SATA controller (bus 0).
						sataController := &vimtypes.VirtualAHCIController{
							VirtualSATAController: vimtypes.VirtualSATAController{
								VirtualController: vimtypes.VirtualController{
									BusNumber: 0,
									VirtualDevice: vimtypes.VirtualDevice{
										Key: -2,
									},
								},
							},
						}
						deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device:    sataController,
						})
						e2eframework.Logf("Adding SATA controller (bus 0)")

						// Add a 5MB disk attached to the new SCSI controller.
						if len(moVM.Datastore) == 0 {
							return fmt.Errorf("VM has no datastores")
						}
						datastoreRef := moVM.Datastore[0]
						disk := &vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									DiskMode:        string(vimtypes.VirtualDiskModePersistent),
									ThinProvisioned: new(true),
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										Datastore: &datastoreRef,
									},
								},
								ControllerKey: pvscsiController.Key,
								UnitNumber:    vimtypes.NewInt32(0),
							},
							CapacityInBytes: 5 * 1024 * 1024, // 5MB
						}
						deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
							Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
							FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
							Device:        disk,
						})
						e2eframework.Logf("Adding 5MB disk on new SCSI controller")

						configSpec := vimtypes.VirtualMachineConfigSpec{
							DeviceChange: deviceChanges,
							ExtraConfig: []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   "test.brownfield.import",
									Value: "true",
								},
							},
						}
						reconfigTask, err := brownfieldVM.Reconfigure(ctx, configSpec)
						if err != nil {
							return fmt.Errorf("failed to reconfigure VM with new hardware: %w", err)
						}
						if err := reconfigTask.Wait(ctx); err != nil {
							return fmt.Errorf("failed to wait for VM reconfiguration: %w", err)
						}
						e2eframework.Logf("Successfully added hardware to brownfield VM")
						return nil
					},
				})
				brownfieldVMMoID = result.BrownfieldVMMoID
				importedVMName := result.ImportedVMName

				// Retrieve the ImportOperation CR for cleanup.
				importOperation = &mopv1a2.ImportOperation{}
				_ = svClusterClient.Get(ctx, ctrlclient.ObjectKey{
					Namespace: vmSvcNamespace,
					Name:      fmt.Sprintf("import-%s", vmName),
				}, importOperation)

				vmYamls = append(vmYamls, manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace: vmSvcNamespace,
					Name:      importedVMName,
				}))

				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, "PoweredOn")

				By("Verifying controllers in status")
				verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, map[vmopv1.VirtualControllerType]int{
					vmopv1.VirtualControllerTypeIDE:  2,
					vmopv1.VirtualControllerTypeSCSI: 2,
					vmopv1.VirtualControllerTypeSATA: 1,
				})

				By("Waiting on virtual machine conditions to become true")

				conditions := []metav1.Condition{
					{
						Type:   consts.VMUnmanagedVolumesBackfilledCondition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   consts.VMUnmanagedVolumesRegisteredCondition,
						Status: metav1.ConditionTrue,
					},
				}
				for _, condition := range conditions {
					vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, condition)
				}

				By("Verifying volumes in VM status")

				volumeNames := make([]string, 0)

				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, importedVMName)
					g.Expect(err).ToNot(HaveOccurred())

					volumeNames = make([]string, 0)

					for i, vol := range vm.Status.Volumes {
						volName := vol.Name
						volumeNames = append(volumeNames, volName)
						g.Expect(volName).ToNot(BeEmpty(), "Expected volume, volumes[%d] to have a name", i)
						g.Expect(vol).ToNot(BeNil(), "Expected to find volume %s in status", volName)
						g.Expect(vol.Type).To(Equal(vmopv1.VolumeTypeManaged),
							"Expected volume to be of type Managed (PVC)")
						g.Expect(vol.Attached).To(BeTrue(), "Expected volume %s to be attached", volName)
						g.Expect(vol.DiskUUID).ToNot(BeEmpty(), "Expected volume %s to have a disk UUID", volName)

						g.Expect(vol.Limit).ToNot(BeNil(), "Expected volume %s to have a limit", volName)
						_, ok := vol.Limit.AsInt64()
						g.Expect(ok).To(BeTrue(), "Expected volume %s to have a limit", volName)

						g.Expect(vol.Requested).ToNot(BeNil(), "Expected volume %s to have a requested", volName)
						_, ok = vol.Requested.AsInt64()
						g.Expect(ok).To(BeTrue(), "Expected volume %s to have a requested", volName)

						g.Expect(vol.Used).ToNot(BeNil(), "Expected volume %s to have a used", volName)
						_, ok = vol.Used.AsInt64()
						g.Expect(ok).To(BeTrue(), "Expected volume %s to have a used", volName)
					}
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for volumes to be found in status")

				if allDisksArePVCapabilityEnabled {
					By("Verify volumes in batch attachment CRD")
					csi.WaitForBatchAttachVolumesToBeAttached(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, volumeNames)

					By("Verifying volumes have a PVC")

					for _, volName := range volumeNames {
						pvc := &corev1.PersistentVolumeClaim{}
						pvcKey := ctrlclient.ObjectKey{
							Namespace: vmSvcNamespace,
							Name:      volName,
						}
						err := svClusterClient.Get(ctx, pvcKey, pvc)
						Expect(err).ToNot(HaveOccurred(), "Failed to get PVC %s", volName)
						Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound),
							"Expected PVC %s to be bound", volName)
					}
				}
			})

			It("Import brownfield VM with shared SCSI controller and shared disk should succeed", Label("experimental"), func() {
				skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.MultiWriterDiskVMotionCapabilityName)

				vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vCenterClient)

				isVSANEnabled, err := vcenter.IsVSANEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())
				isVSANDEnabled, err := vcenter.IsVSANDEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())

				if isVSANDEnabled || isVSANEnabled {
					Skip("Skipping EZT storage profile tests as VSAN Datastore is present")
				}

				importOpName := fmt.Sprintf("import-shared-disk-%s", capiutil.RandomString(4))

				result := ImportBrownfieldVM(ImportBrownfieldVMInput{
					Ctx:                ctx,
					Config:             config,
					VCenterAdminClient: vCenterAdminClient,
					SVClusterClient:    svClusterClient,
					Namespace:          vmSvcNamespace,
					ClusterMoID:        clusterMoID,
					BrownfieldVMName:   brownfieldVMName,
					StorageClassName:   eztStorageProfileName,
					ImportOpName:       importOpName,
					BeforePowerOn: func(ctx context.Context, vm *object.VirtualMachine) error {
						By("Adding shared SCSI controller with physical sharing mode and shared disk")
						if err := addSharedMultiWriterDisk(ctx, vm, eztStoragePolicyID); err != nil {
							return err
						}
						e2eframework.Logf("Successfully added shared SCSI controller and shared disk to brownfield VM")
						return nil
					},
				})

				brownfieldVMMoID = result.BrownfieldVMMoID
				importedVMName := result.ImportedVMName

				// Retrieve the ImportOperation CR so AfterEach can clean it up.
				importOperation = &mopv1a2.ImportOperation{}
				_ = svClusterClient.Get(ctx, ctrlclient.ObjectKey{
					Namespace: vmSvcNamespace,
					Name:      importOpName,
				}, importOperation)

				vmYamls = append(vmYamls, manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace: vmSvcNamespace,
					Name:      importedVMName,
				}))

				By("Verifying VM is imported and powered on")
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, "PoweredOn")
				e2eframework.Logf("Imported VM %s is powered on", importedVMName)

				By("Verifying shared SCSI controller in VM status")
				verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, map[vmopv1.VirtualControllerType]int{
					vmopv1.VirtualControllerTypeIDE:  2,
					vmopv1.VirtualControllerTypeSCSI: 2,
				})

				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, importedVMName)
					g.Expect(err).ToNot(HaveOccurred())

					idx := slices.IndexFunc(vm.Spec.Hardware.SCSIControllers, func(c vmopv1.SCSIControllerSpec) bool {
						return c.BusNumber == 1 && c.SharingMode == vmopv1.VirtualControllerSharingModePhysical
					})
					g.Expect(idx).To(BeNumerically(">=", 0), "Expected to find shared SCSI controller with physical sharing mode")
					if idx >= 0 {
						c := vm.Spec.Hardware.SCSIControllers[idx]
						e2eframework.Logf("Found shared SCSI controller: bus=%d, sharingMode=%s", c.BusNumber, c.SharingMode)
					}
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for shared controller verification")

				conditions := []metav1.Condition{
					{
						Type:   consts.VMUnmanagedVolumesBackfilledCondition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   consts.VMUnmanagedVolumesRegisteredCondition,
						Status: metav1.ConditionTrue,
					},
				}
				for _, condition := range conditions {
					vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, vmSvcNamespace, importedVMName, condition)
				}

				By("Verifying shared disk volume has MultiWriter sharing mode")
				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, importedVMName)
					g.Expect(err).ToNot(HaveOccurred())

					idx := slices.IndexFunc(vm.Status.Volumes, func(v vmopv1.VirtualMachineVolumeStatus) bool {
						return v.ControllerType == vmopv1.VirtualControllerTypeSCSI &&
							*v.ControllerBusNumber == 1 &&
							*v.UnitNumber == 0
					})
					g.Expect(idx).To(BeNumerically(">=", 0), "Expected to find shared disk on SCSI bus 1 unit 0")
					if idx >= 0 {
						vol := vm.Status.Volumes[idx]
						e2eframework.Logf("Found shared disk volume: name=%s, sharingMode=%s, controllerBus=%d, unit=%d",
							vol.Name, vol.SharingMode, *vol.ControllerBusNumber, *vol.UnitNumber)
						g.Expect(vol.SharingMode).To(Equal(vmopv1.VolumeSharingModeMultiWriter),
							"Expected shared disk to have sharingMode=MultiWriter")
					}
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for shared disk volume verification")

				By("Verifying PVC for shared disk is created and bound")
				Eventually(func(g Gomega) {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmSvcNamespace, importedVMName)
					g.Expect(err).ToNot(HaveOccurred())

					idx := slices.IndexFunc(vm.Status.Volumes, func(v vmopv1.VirtualMachineVolumeStatus) bool {
						return v.ControllerType == vmopv1.VirtualControllerTypeSCSI &&
							*v.ControllerBusNumber == 1 &&
							*v.UnitNumber == 0
					})
					g.Expect(idx).To(BeNumerically(">=", 0), "Expected to find shared disk volume for PVC verification")
					if idx >= 0 {
						vol := vm.Status.Volumes[idx]
						pvc := &corev1.PersistentVolumeClaim{}
						g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{
							Namespace: vmSvcNamespace,
							Name:      vol.Name,
						}, pvc)).To(Succeed(), "Failed to get PVC %s", vol.Name)

						e2eframework.Logf("PVC %s annotations: %v", vol.Name, pvc.Annotations)
						e2eframework.Logf("PVC %s spec: storageClass=%s, volumeMode=%v, accessModes=%v",
							vol.Name, *pvc.Spec.StorageClassName, *pvc.Spec.VolumeMode, pvc.Spec.AccessModes)

						g.Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound),
							"Expected PVC %s to be bound", vol.Name)
					}
				}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).
					Should(Succeed(), "Timed out waiting for PVC verification")

				e2eframework.Logf("Successfully verified shared disk import with MultiWriter sharing mode")
			})
		})

		Describe("Multi-Writer and Encryption Validation Webhook", Label("extended-functional", "experimental"), func() {
			BeforeAll(func() {
				By("Ensuring encryption storage policy and class in test namespace (shared with VMEncryptionSpec)")
				vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				DeferCleanup(vcenter.LogoutVimClient, vCenterClient)

				Expect(utils.EnsureE2EEncryptionStorageInNamespace(ctx, vCenterClient, wcpClient,
					clusterProxy.GetClientSet(), svClusterClient, *config,
					vmSvcNamespace, clusterResources.StorageClassName)).To(Succeed(),
					"failed to ensure encryption storage in namespace %s", vmSvcNamespace)
			})

			It("should reject a VM whose encrypted PVC uses MultiWriter sharing mode", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName: utils.E2EEncryptionStorageClassName,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:       ptr.To(corev1.PersistentVolumeBlock),
				}, 1)
				pvcSpec := pvcs[0]

				By("Creating the encrypted PVC before the VM")
				Expect(clusterProxy.ApplyWithArgs(ctx, manifestbuilders.GetPersistentVolumeClaimYaml(pvcSpec))).
					To(Succeed(), "failed to create encrypted PVC %s", pvcSpec.ClaimName)
				DeferCleanup(func() {
					_ = svClusterClient.Delete(ctx, &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pvcSpec.ClaimName,
							Namespace: vmSvcNamespace,
						},
					})
				})

				By("Attempting to create VM with encrypted PVC in MultiWriter mode (should be webhook-rejected)")
				vm := &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      vmName,
						Namespace: vmSvcNamespace,
					},
					Spec: vmopv1.VirtualMachineSpec{
						ClassName:    clusterResources.VMClassName,
						ImageName:    linuxVMIName,
						StorageClass: clusterResources.StorageClassName,
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: pvcSpec.VolumeName,
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: pvcSpec.ClaimName,
										},
									},
								},
								SharingMode: vmopv1.VolumeSharingModeMultiWriter,
							},
						},
					},
				}

				err := svClusterClient.Create(ctx, vm)
				Expect(err).To(HaveOccurred(),
					"expected webhook to reject VM with encrypted PVC in MultiWriter sharing mode")
				Expect(err.Error()).To(ContainSubstring("MultiWriter disk sharing is not supported for encrypted volumes"),
					"expected webhook rejection message about encrypted MultiWriter disks")
			})

			It("should reject a VM whose encrypted PVC is on a physical-sharing SCSI controller", func() {
				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    utils.E2EEncryptionStorageClassName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(1)),
					AccessModes:         []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:          ptr.To(corev1.PersistentVolumeBlock),
				}, 1)
				pvcSpec := pvcs[0]

				By("Creating the encrypted PVC before the VM")
				Expect(clusterProxy.ApplyWithArgs(ctx, manifestbuilders.GetPersistentVolumeClaimYaml(pvcSpec))).
					To(Succeed(), "failed to create encrypted PVC %s", pvcSpec.ClaimName)
				DeferCleanup(func() {
					_ = svClusterClient.Delete(ctx, &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pvcSpec.ClaimName,
							Namespace: vmSvcNamespace,
						},
					})
				})

				vm := &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      vmName,
						Namespace: vmSvcNamespace,
					},
					Spec: vmopv1.VirtualMachineSpec{
						ClassName:    clusterResources.VMClassName,
						ImageName:    linuxVMIName,
						StorageClass: clusterResources.StorageClassName,
						Hardware: &vmopv1.VirtualMachineHardwareSpec{
							SCSIControllers: []vmopv1.SCSIControllerSpec{
								{
									BusNumber:   1,
									Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
									SharingMode: vmopv1.VirtualControllerSharingModePhysical,
								},
							},
						},
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: pvcSpec.VolumeName,
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: pvcSpec.ClaimName,
										},
									},
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(1)),
							},
						},
					},
				}

				By("Attempting to create VM with encrypted PVC on physical-sharing controller (should be webhook-rejected)")
				err := svClusterClient.Create(ctx, vm)
				Expect(err).To(HaveOccurred(),
					"expected webhook to reject VM with encrypted PVC on physical-sharing SCSI controller")
				Expect(err.Error()).To(ContainSubstring("not supported for encrypted volumes"),
					"expected webhook rejection message about encrypted volumes and controller sharing")
			})

			It("should allow a non-encrypted VM with a physical-sharing SCSI controller", func() {
				vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vCenterClient)

				isVSANEnabled, err := vcenter.IsVSANEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())
				isVSANDEnabled, err := vcenter.IsVSANDEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
				Expect(err).To(BeNil())

				if isVSANDEnabled || isVSANEnabled {
					Skip("Skipping EZT storage profile tests as VSAN Datastore is present")
				}

				pvcs := createPvcsFromSpec(input, vmName, manifestbuilders.PVC{
					StorageClassName:    eztStorageProfileName,
					ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
					ControllerBusNumber: ptr.To(int32(1)),
					UnitNumber:          ptr.To(int32(0)),
					AccessModes:         []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeMode:          ptr.To(corev1.PersistentVolumeBlock),
				}, 1)

				hardware := vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					},
				}

				vmYaml := manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             vmName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             pvcs,
					Hardware:         &hardware,
				})
				vmYamls = append(vmYamls, vmYaml)

				By("Creating non-encrypted VM with physical-sharing SCSI controller")
				Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(),
					"expected webhook to accept VM with physical-sharing controller and non-encrypted storage")

				By("Waiting for the VM to power on with shared PVCs attached")
				volumeNames := make([]string, len(pvcs))
				for i, pvc := range pvcs {
					volumeNames[i] = pvc.VolumeName
				}

				backfilledVolumes := getBackfilledVolumes(ctx, config, svClusterClient, vmSvcNamespace, vmName, allDisksArePVCapabilityEnabled)
				volumeNames = append(volumeNames, backfilledVolumes...)
				waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, vmName, volumeNames)

				By("Verifying the physical-sharing controller is reflected in VM status")
				verifyCreatedControllersCount(ctx, config, svClusterClient, vmSvcNamespace, vmName,
					map[vmopv1.VirtualControllerType]int{
						vmopv1.VirtualControllerTypeSCSI: 2,
						vmopv1.VirtualControllerTypeIDE:  2,
					},
				)
			})
		})

		Describe("VM with late-binding PVC powers on", Label("experimental", "core-functional"), func() {
			const wffcSpecName = "wffc-poweron"

			var (
				wffcPVC    manifestbuilders.PVC
				wffcVMName string
				wffcSCName string
			)

			BeforeAll(func() {
				skipper.SkipUnlessInfraIs(config.InfraConfig.InfraName, consts.WCP)

				wffcSCName = clusterResources.StorageClassName + "-latebinding"

				By("Verifying the late-binding storage class exists and uses WaitForFirstConsumer mode")
				svClientSet := clusterProxy.GetClientSet()
				sc, err := svClientSet.StorageV1().StorageClasses().
					Get(ctx, wffcSCName, metav1.GetOptions{})
				if err != nil {
					Skip(fmt.Sprintf("storage class %q not found — skipping WFFC late-binding test: %v",
						wffcSCName, err))
				}
				if sc.VolumeBindingMode == nil ||
					*sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
					Skip(fmt.Sprintf(
						"storage class %q has binding mode %v (not WaitForFirstConsumer) — skipping WFFC test",
						wffcSCName, sc.VolumeBindingMode))
				}

				By("Ensuring the WFFC storage class is associated with the test namespace")
				podVMOnStretchedSupervisorEnabled := utils.IsFssEnabled(ctx, svClusterClient,
					config.GetVariable("VMOPNamespace"),
					config.GetVariable("VMOPDeploymentName"),
					config.GetVariable("VMOPManagerCommand"),
					config.GetVariable("EnvFSSPodVMOnStretchedSupervisor"))
				utils.EnsureStorageClassInNamespace(ctx, svClusterClient, vmSvcNamespace,
					wffcSCName, podVMOnStretchedSupervisorEnabled, *config)
			})

			BeforeEach(func() {
				wffcVMName = fmt.Sprintf("%s-%s", wffcSpecName, capiutil.RandomString(4))

				volumePrefix := fmt.Sprintf("%s-pvc-%s", wffcVMName, capiutil.RandomString(4))
				wffcPVC = manifestbuilders.PVC{
					Namespace:        vmSvcNamespace,
					VolumeName:       fmt.Sprintf("%s-volume", volumePrefix),
					ClaimName:        fmt.Sprintf("%s-claim", volumePrefix),
					StorageClassName: wffcSCName,
					RequestSize:      "1Mi",
				}
			})

			It("should bind the PVC and power on the VM within a reasonable timeout", func() {
				By("Pre-creating a standalone PVC backed by the WFFC storage class")
				pvcYaml := manifestbuilders.GetPersistentVolumeClaimYaml(wffcPVC)
				Expect(clusterProxy.ApplyWithArgs(ctx, pvcYaml)).To(Succeed(),
					"failed to pre-create WFFC PVC %q", wffcPVC.ClaimName)
				pvcsYamls = append(pvcsYamls, pvcYaml)

				By("Verifying the PVC remains Pending before any consumer is created")
				Consistently(func(g Gomega) {
					pvc := &corev1.PersistentVolumeClaim{}
					g.Expect(svClusterClient.Get(ctx, types.NamespacedName{
						Namespace: vmSvcNamespace,
						Name:      wffcPVC.ClaimName,
					}, pvc)).To(Succeed(), "failed to get PVC %q", wffcPVC.ClaimName)
					g.Expect(pvc.Status.Phase).To(Equal(corev1.ClaimPending),
						"WFFC PVC %q should remain Pending until a consumer is created, but got phase: %s",
						wffcPVC.ClaimName, pvc.Status.Phase)
				}, config.GetIntervals("default", "consistent-virtual-machine-condition")...).
					Should(Succeed(), "WFFC PVC %q unexpectedly bound before VM was created", wffcPVC.ClaimName)

				By("Creating a VirtualMachine that references the WFFC PVC")
				wffcVMYaml := manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             wffcVMName,
					ImageName:        linuxVMIName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PVCs:             []manifestbuilders.PVC{wffcPVC},
				})
				vmYamls = append(vmYamls, wffcVMYaml)
				Expect(clusterProxy.ApplyWithArgs(ctx, wffcVMYaml)).To(Succeed(),
					"failed to create VirtualMachine %q with WFFC PVC", wffcVMName)

				backfilledVolumes := getBackfilledVolumes(ctx, config,
					svClusterClient, vmSvcNamespace, wffcVMName,
					allDisksArePVCapabilityEnabled)
				expectedVolumes := append([]string{wffcPVC.VolumeName}, backfilledVolumes...)
				waitForVMAndBatchAttach(ctx, config, svClusterClient, vmSvcNamespace, wffcVMName, expectedVolumes)

				By("Verifying the WFFC PVC is bound after the VM powers on")
				pvc := &corev1.PersistentVolumeClaim{}
				err := svClusterClient.Get(ctx, types.NamespacedName{
					Namespace: vmSvcNamespace,
					Name:      wffcPVC.ClaimName,
				}, pvc)
				Expect(err).ToNot(HaveOccurred(), "failed to get PVC %q", wffcPVC.ClaimName)
				// This assertion should not fail if the VM has powered on with late-binding PVCs.
				Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound),
					"expected PVC %q to be bound, but got phase: %s",
					wffcPVC.ClaimName, pvc.Status.Phase)
			})
		})
	})
}
