// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configtarget_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/controllers/configtarget"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const clusterMoID = "domain-c9"

var _ = Describe("ConfigTarget Controller", Label(testlabels.Controller, testlabels.API), unitTests)

func unitTests() {
	var (
		ctx        context.Context
		client     ctrlclient.Client
		reconciler *configtarget.Reconciler
		provider   *providerfake.VMProvider
		obj        *vimv1.ConfigTarget
		objReq     ctrl.Request

		withObjs []ctrlclient.Object

		configTargetResult *vimtypes.ConfigTarget
		descriptorsResult  []vimtypes.VirtualMachineConfigOptionDescriptor
		queryErr           error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		provider = providerfake.NewVMProvider()

		withObjs = nil

		obj = &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterMoID,
			},
			Spec: vimv1.ConfigTargetSpec{
				ID: vimv1.ManagedObjectID{ID: clusterMoID},
			},
		}
		objReq = ctrl.Request{
			NamespacedName: types.NamespacedName{Name: obj.Name},
		}

		sevTrue := true
		configTargetResult = &vimtypes.ConfigTarget{
			NumCpus:                8,
			NumCpuCores:            4,
			NumNumaNodes:           2,
			MaxCpusPerHost:         64,
			MaxSimultaneousThreads: 2,
			SmcPresent:             false,
			MaxMemMBOptimalPerf:    4096,
			SupportedMaxMemMB:      8192,
			SevSupported:           &sevTrue,
		}
		descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
			{Key: "vmx-19", CreateSupported: true},
			{Key: "vmx-20", CreateSupported: true},
		}
		queryErr = nil
	})

	JustBeforeEach(func() {
		provider.GetVirtualMachineConfigTargetFn = func(
			_ context.Context, gotClusterMoID string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
			Expect(gotClusterMoID).To(Equal(clusterMoID))
			return configTargetResult, descriptorsResult, queryErr
		}

		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vimv1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&vimv1.ConfigTarget{}).
			WithObjects(withObjs...).
			Build()

		reconciler = configtarget.NewReconciler(
			ctx,
			client,
			log.Log.WithName("configtarget"),
			record.New(events.NewFakeRecorder(100)),
			provider)
	})

	When("the ConfigTarget does not exist", func() {
		It("returns without error", func() {
			result, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	When("the ConfigTarget exists", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, obj)
		})

		When("the vSphere queries succeed", func() {
			It("populates status, sets Ready=True, and fans out VirtualMachineConfigOptions", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())

				Expect(got.Status.NumCPUs).To(Equal(int32(8)))
				Expect(got.Status.NumCPUCores).To(Equal(int32(4)))
				Expect(got.Status.NumNumaNodes).To(Equal(int32(2)))
				Expect(got.Status.MaxCPUsPerVM).To(Equal(int32(64)))
				Expect(got.Status.MaxSimultaneousThreads).To(Equal(int32(2)))
				Expect(got.Status.SMCPresent).To(BeFalse())
				Expect(got.Status.SEVSupported).To(BeTrue())
				Expect(got.Status.SEVSNPSupported).To(BeFalse())
				Expect(got.Status.TDXSupported).To(BeFalse())
				Expect(got.Status.MaxMemOptimalPerf).ToNot(BeNil())
				Expect(got.Status.MaxMemOptimalPerf.Value()).To(Equal(int64(4096) * 1024 * 1024))
				Expect(got.Status.SupportedMaxMem).ToNot(BeNil())
				Expect(got.Status.SupportedMaxMem.Value()).To(Equal(int64(8192) * 1024 * 1024))

				// SR-IOV requires per-host attribution and is not supported
				// yet (vmop-3926, T066); the base configTargetResult
				// carries no other device inventory either -- device-mapping
				// fidelity is covered by the dedicated "mapping device
				// categories" cases below.
				Expect(got.Status.ConfigTargetDevices.SRIOV).To(BeEmpty())
				Expect(got.Status.ConfigTargetDevices.CDROM).To(BeEmpty())

				Expect(got.Status.MaxHardwareVersion).To(Equal("vmx-20"))

				Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
				Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())

				var vmcoList vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &vmcoList)).To(Succeed())
				Expect(vmcoList.Items).To(HaveLen(2))

				names := make([]string, len(vmcoList.Items))
				for i, item := range vmcoList.Items {
					names[i] = item.Name
					Expect(item.Spec.HardwareVersion).To(Equal(item.Name))
					Expect(item.OwnerReferences).To(HaveLen(1))
					Expect(item.OwnerReferences[0].UID).To(Equal(got.UID))
				}

				Expect(names).To(ConsistOf("vmx-19", "vmx-20"))
			})

			It("is idempotent on a second reconcile with unchanged vSphere state", func() {
				_, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())

				var before vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &before)).To(Succeed())

				resourceVersions := map[string]string{}
				for _, item := range before.Items {
					resourceVersions[item.Name] = item.ResourceVersion
				}

				_, err = reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())

				var after vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &after)).To(Succeed())
				Expect(after.Items).To(HaveLen(len(before.Items)))

				for _, item := range after.Items {
					Expect(item.ResourceVersion).To(Equal(resourceVersions[item.Name]),
						"expected no spurious patch for %s", item.Name)
				}
			})

			When("a hardware version is dropped from the descriptor list", func() {
				It("garbage-collects the corresponding VirtualMachineConfigOptions", func() {
					_, err := reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var before vimv1.VirtualMachineConfigOptionsList
					Expect(client.List(ctx, &before)).To(Succeed())
					Expect(before.Items).To(HaveLen(2))

					descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
						{Key: "vmx-20"},
					}
					provider.GetVirtualMachineConfigTargetFn = func(
						_ context.Context, _ string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
						return configTargetResult, descriptorsResult, nil
					}

					_, err = reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var after vimv1.VirtualMachineConfigOptionsList
					Expect(client.List(ctx, &after)).To(Succeed())
					Expect(after.Items).To(HaveLen(1))
					Expect(after.Items[0].Name).To(Equal("vmx-20"))
				})
			})

			When("a VirtualMachineConfigOptions is co-owned by another ConfigTarget", func() {
				var otherObj *vimv1.ConfigTarget

				BeforeEach(func() {
					otherObj = &vimv1.ConfigTarget{
						ObjectMeta: metav1.ObjectMeta{Name: "domain-c-other"},
						Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: "domain-c-other"}},
					}
					withObjs = append(withObjs, otherObj)
				})

				It("only removes its own owner reference and leaves the object when another owner remains", func() {
					// otherObj always reports both vmx-19 and vmx-20; obj's
					// live set is driven by descriptorsResult, mutated below.
					provider.GetVirtualMachineConfigTargetFn = func(
						_ context.Context, gotClusterMoID string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
						if gotClusterMoID == otherObj.Name {
							return configTargetResult, []vimtypes.VirtualMachineConfigOptionDescriptor{
								{Key: "vmx-19"}, {Key: "vmx-20"},
							}, nil
						}

						return configTargetResult, descriptorsResult, queryErr
					}

					// Reconcile the "other" cluster first so it takes an
					// owner reference on the shared vmx-19/vmx-20 objects.
					otherReconciler := configtarget.NewReconciler(
						ctx,
						client,
						log.Log.WithName("configtarget"),
						record.New(events.NewFakeRecorder(100)),
						provider)
					_, err := otherReconciler.Reconcile(ctx, ctrl.Request{
						NamespacedName: types.NamespacedName{Name: otherObj.Name},
					})
					Expect(err).ToNot(HaveOccurred())

					_, err = reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var vmco vimv1.VirtualMachineConfigOptions
					Expect(client.Get(ctx, types.NamespacedName{Name: "vmx-19"}, &vmco)).To(Succeed())
					Expect(vmco.OwnerReferences).To(HaveLen(2))

					// Now this ConfigTarget drops vmx-19 from its live set.
					descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
						{Key: "vmx-20"},
					}

					_, err = reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					// vmx-19 must still exist: otherObj still owns it.
					Expect(client.Get(ctx, types.NamespacedName{Name: "vmx-19"}, &vmco)).To(Succeed())
					Expect(vmco.OwnerReferences).To(HaveLen(1))
					Expect(vmco.OwnerReferences[0].UID).To(Equal(otherObj.UID))
				})
			})
		})

		When("the vSphere result includes device inventory data", func() {
			BeforeEach(func() {
				configTargetResult.CdRom = []vimtypes.VirtualMachineCdromInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "cdrom0"},
					Description:              "CD/DVD drive 0",
				}}
				configTargetResult.Floppy = []vimtypes.VirtualMachineFloppyInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "floppy0"},
				}}
				configTargetResult.Serial = []vimtypes.VirtualMachineSerialInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "serial0"},
				}}
				configTargetResult.Parallel = []vimtypes.VirtualMachineParallelInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "parallel0"},
				}}
				configTargetResult.Sound = []vimtypes.VirtualMachineSoundInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "sound0"},
				}}
				configTargetResult.Usb = []vimtypes.VirtualMachineUsbInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "usb0"},
					Description:              "USB device",
					Vendor:                   1,
					Product:                  2,
					PhysicalPath:             "/usb/0",
					Family:                   []string{"hid"},
					Speed:                    []string{"high"},
				}}
				configTargetResult.PciPassthrough = []vimtypes.BaseVirtualMachinePciPassthroughInfo{
					&vimtypes.VirtualMachinePciPassthroughInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "pci0"},
						PciDevice: vimtypes.HostPciDevice{
							Id:         "0000:03:00.0",
							DeviceName: "NIC",
							VendorName: "Intel",
						},
						SystemId: "host-1",
					},
					&vimtypes.VirtualMachineSriovInfo{
						VirtualMachinePciPassthroughInfo: vimtypes.VirtualMachinePciPassthroughInfo{
							VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "sriov0"},
							PciDevice:                vimtypes.HostPciDevice{Id: "0000:04:00.0"},
							SystemId:                 "host-1",
						},
						VirtualFunction: true,
					},
				}
				configTargetResult.Sriov = []vimtypes.VirtualMachineSriovInfo{{
					VirtualMachinePciPassthroughInfo: vimtypes.VirtualMachinePciPassthroughInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "sriov1"},
						PciDevice:                vimtypes.HostPciDevice{Id: "0000:05:00.0"},
						SystemId:                 "host-1",
					},
					VirtualFunction: true,
				}}
				configTargetResult.DynamicPassthrough = []vimtypes.VirtualMachineDynamicPassthroughInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "dyn0"},
					VendorName:               "NVIDIA",
					DeviceName:               "GPU",
					VendorId:                 1,
					DeviceId:                 2,
				}}
				configTargetResult.VgpuDeviceInfo = []vimtypes.VirtualMachineVgpuDeviceInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "vgpu0"},
					DeviceName:               "grid",
					DeviceVendorId:           1,
					MaxFbSizeInGib:           16,
					TimeSlicedCapable:        true,
					ComputeProfileCapable:    true,
				}}
				configTargetResult.VgpuProfileInfo = []vimtypes.VirtualMachineVgpuProfileInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "profile0"},
					ProfileName:              "grid_p1",
					DeviceVendorId:           1,
					FbSizeInGib:              8,
					ProfileSharing:           "time-sliced",
					ProfileClass:             "compute",
					StunTimeEstimates: []vimtypes.VirtualMachineVMotionStunTimeInfo{{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "stun0"},
						MigrationBW:              1000,
						StunTime:                 5,
					}},
				}}
				configTargetResult.SharedGpuPassthroughTypes = []vimtypes.VirtualMachinePciSharedGpuPassthroughInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "shared0"},
					Vgpu:                     "grid_p1",
				}}
				requireAttestation := true
				configTargetResult.SgxTargetInfo = &vimtypes.VirtualMachineSgxTargetInfo{
					VirtualMachineTargetInfo:    vimtypes.VirtualMachineTargetInfo{Name: "sgx"},
					MaxEpcSize:                  1024,
					FlcModes:                    []string{"unlocked"},
					LePubKeyHashes:              []string{"hash1"},
					RequireAttestationSupported: &requireAttestation,
				}
				configTargetResult.PrecisionClockInfo = []vimtypes.VirtualMachinePrecisionClockInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "ptp0"},
					SystemClockProtocol:      "ptp",
				}}
				configTargetResult.VendorDeviceGroupInfo = []vimtypes.VirtualMachineVendorDeviceGroupInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "vdg0"},
					DeviceGroupName:          "group1",
					DeviceGroupDescription:   "desc",
					ComponentDeviceInfo: []vimtypes.VirtualMachineVendorDeviceGroupInfoComponentDeviceInfo{{
						Type:           "DVX",
						VendorName:     "NVIDIA",
						DeviceName:     "GPU",
						IsConfigurable: true,
					}},
				}}
				configTargetResult.DvxClassInfo = []vimtypes.VirtualMachineDvxClassInfo{{
					DeviceClass: &vimtypes.ElementDescription{
						Description: vimtypes.Description{Label: "DVX NIC", Summary: "summary"},
						Key:         "dvxnic",
					},
					VendorName: "Mellanox",
					SriovNic:   true,
				}}
				configTargetResult.IdeDisk = []vimtypes.VirtualMachineIdeDiskDeviceInfo{{
					VirtualMachineDiskDeviceInfo: vimtypes.VirtualMachineDiskDeviceInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "ide0"},
						Capacity:                 1024,
					},
					PartitionTable: []vimtypes.VirtualMachineIdeDiskDevicePartitionInfo{{Id: 1, Capacity: 512}},
				}}
				configTargetResult.ScsiDisk = []vimtypes.VirtualMachineScsiDiskDeviceInfo{{
					VirtualMachineDiskDeviceInfo: vimtypes.VirtualMachineDiskDeviceInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "scsi0"},
						Capacity:                 2048,
					},
					Disk: &vimtypes.HostScsiDisk{
						ScsiLun: vimtypes.ScsiLun{
							HostDevice:    vimtypes.HostDevice{DeviceName: "/dev/sda", DeviceType: "disk"},
							Key:           "key1",
							Uuid:          "uuid1",
							CanonicalName: "naa.1",
							DisplayName:   "Disk1",
							LunType:       "disk",
							Vendor:        "VMW",
							Model:         "Virtual disk",
						},
						Capacity:   vimtypes.HostDiskDimensionsLba{BlockSize: 512, Block: 100},
						DevicePath: "/vmfs/devices/disks/naa.1",
					},
					TransportHint: "hint",
				}}
				configTargetResult.ScsiPassthrough = []vimtypes.VirtualMachineScsiPassthroughInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "scsipt0"},
					ScsiClass:                "tape",
					Vendor:                   "IBM",
					PhysicalUnitNumber:       1,
				}}
				configTargetResult.VFlashModule = []vimtypes.VirtualMachineVFlashModuleInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "vflash0"},
					VFlashModule: vimtypes.HostVFlashManagerVFlashCacheConfigInfoVFlashModuleConfigOption{
						VFlashModule:              "vfc",
						VFlashModuleVersion:       "1.0",
						MinSupportedModuleVersion: "1.0",
						CacheConsistencyType: vimtypes.ChoiceOption{
							ChoiceInfo: []vimtypes.BaseElementDescription{&vimtypes.ElementDescription{Key: "strong"}},
						},
						CacheMode: vimtypes.ChoiceOption{
							ChoiceInfo: []vimtypes.BaseElementDescription{&vimtypes.ElementDescription{Key: "write-back"}},
						},
						BlockSizeInKBOption:   vimtypes.LongOption{Min: 4, Max: 1024, DefaultValue: 64},
						ReservationInMBOption: vimtypes.LongOption{Min: 1, Max: 1024, DefaultValue: 100},
						MaxDiskSizeInKB:       1048576,
					},
				}}
			})

			It("maps every non-SR-IOV device category and excludes SR-IOV entirely", func() {
				_, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
				devices := got.Status.ConfigTargetDevices

				Expect(devices.SRIOV).To(BeEmpty(), "ct.Sriov must not be mapped")

				Expect(devices.CDROM).To(HaveLen(1))
				Expect(devices.CDROM[0].Name).To(Equal("cdrom0"))
				Expect(devices.CDROM[0].Description).To(Equal("CD/DVD drive 0"))

				Expect(devices.Floppy).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "floppy0"}))
				Expect(devices.Serial).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "serial0"}))
				Expect(devices.Parallel).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "parallel0"}))
				Expect(devices.Sound).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "sound0"}))

				Expect(devices.USB).To(HaveLen(1))
				Expect(devices.USB[0].Vendor).To(Equal(int32(1)))
				Expect(devices.USB[0].Family).To(ConsistOf("hid"))

				Expect(devices.PCIPassthrough).To(HaveLen(1), "the SR-IOV union entry must be excluded")
				Expect(devices.PCIPassthrough[0].Name).To(Equal("pci0"))
				Expect(devices.PCIPassthrough[0].ID).To(Equal("host-1:0000:03:00.0"))
				Expect(devices.PCIPassthrough[0].SystemID).To(Equal("host-1"))

				Expect(devices.DynamicPassthroughDevices).To(HaveLen(1))
				Expect(devices.DynamicPassthroughDevices[0].VendorName).To(Equal("NVIDIA"))
				Expect(devices.DynamicPassthroughDevices[0].VendorID).To(Equal(int32(1)))

				Expect(devices.VGPUDevice).To(HaveLen(1))
				Expect(devices.VGPUDevice[0].DeviceName).To(Equal("grid"))
				Expect(devices.VGPUDevice[0].MaxFbSizeInGib).To(Equal(int64(16)))

				Expect(devices.VGPUProfile).To(HaveLen(1))
				Expect(devices.VGPUProfile[0].ProfileName).To(Equal("grid_p1"))
				Expect(devices.VGPUProfile[0].StunTimeEstimates).To(HaveLen(1))
				Expect(devices.VGPUProfile[0].StunTimeEstimates[0].StunTime).To(Equal(int64(5)))

				Expect(devices.SharedGPUPassthroughTypes).To(HaveLen(1))
				Expect(devices.SharedGPUPassthroughTypes[0].VGPU).To(Equal("grid_p1"))

				Expect(devices.SGXTargetInfo).ToNot(BeNil())
				Expect(devices.SGXTargetInfo.MaxEpcSize).To(Equal(int64(1024)))
				Expect(devices.SGXTargetInfo.RequireAttestationSupported).To(BeTrue())

				Expect(devices.PrecisionClockInfo).To(HaveLen(1))
				Expect(devices.PrecisionClockInfo[0].SystemClockProtocol).To(Equal(vimv1.HostDateTimeInfoProtocol("ptp")))

				Expect(devices.VendorDeviceGroupInfo).To(HaveLen(1))
				Expect(devices.VendorDeviceGroupInfo[0].DeviceGroupName).To(Equal("group1"))
				Expect(devices.VendorDeviceGroupInfo[0].ComponentDeviceInfo).To(HaveLen(1))
				Expect(devices.VendorDeviceGroupInfo[0].ComponentDeviceInfo[0].Type).To(BeEquivalentTo("DVX"))

				Expect(devices.DVXClassInfo).To(HaveLen(1))
				Expect(devices.DVXClassInfo[0].DeviceClass.Key).To(Equal("dvxnic"))
				Expect(devices.DVXClassInfo[0].SriovNic).To(BeTrue())

				Expect(devices.IDEDisks).To(HaveLen(1))
				Expect(devices.IDEDisks[0].Capacity).ToNot(BeNil())
				Expect(devices.IDEDisks[0].Capacity.Value()).To(Equal(int64(1024) * 1024))
				Expect(devices.IDEDisks[0].PartitionTable).To(HaveLen(1))
				Expect(devices.IDEDisks[0].PartitionTable[0].ID).To(Equal(int32(1)))

				Expect(devices.SCSIDisks).To(HaveLen(1))
				Expect(devices.SCSIDisks[0].Capacity).ToNot(BeNil())
				Expect(devices.SCSIDisks[0].Capacity.Value()).To(Equal(int64(2048) * 1024))
				Expect(devices.SCSIDisks[0].Disk).ToNot(BeNil())
				Expect(devices.SCSIDisks[0].Disk.UUID).To(Equal("uuid1"))
				Expect(devices.SCSIDisks[0].Disk.DeviceName).To(Equal("/dev/sda"))

				Expect(devices.SCSIPassthrough).To(HaveLen(1))
				Expect(devices.SCSIPassthrough[0].SCSIClass).To(Equal("tape"))

				Expect(devices.VFlashModule).To(HaveLen(1))
				vfm := devices.VFlashModule[0].VFlashModule
				Expect(vfm.VFlashModule).To(Equal("vfc"))
				Expect(vfm.CacheMode.Choices).To(HaveLen(1))
				Expect(vfm.CacheMode.Choices[0].Key).To(Equal("write-back"))
				Expect(vfm.BlockSizeOption.Min.Value()).To(Equal(int64(4) * 1024))
				Expect(vfm.MaxDiskSize.Value()).To(Equal(int64(1048576) * 1024))
			})
		})

		When("computing MaxHardwareVersion", func() {
			When("descriptors have mixed CreateSupported values", func() {
				BeforeEach(func() {
					descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
						{Key: "vmx-15", CreateSupported: true},
						{Key: "vmx-21", CreateSupported: false},
						{Key: "vmx-19", CreateSupported: true},
					}
				})

				It("ignores descriptors where CreateSupported is false", func() {
					_, err := reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var got vimv1.ConfigTarget
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
					Expect(got.Status.MaxHardwareVersion).To(Equal("vmx-19"))
				})
			})

			When("there are no descriptors", func() {
				BeforeEach(func() {
					descriptorsResult = nil
				})

				It("leaves MaxHardwareVersion empty", func() {
					_, err := reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var got vimv1.ConfigTarget
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
					Expect(got.Status.MaxHardwareVersion).To(BeEmpty())
				})
			})

			When("a descriptor has a malformed key", func() {
				BeforeEach(func() {
					descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
						{Key: "not-a-version", CreateSupported: true},
						{Key: "vmx-18", CreateSupported: true},
					}
				})

				It("skips the malformed key without erroring", func() {
					_, err := reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var got vimv1.ConfigTarget
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
					Expect(got.Status.MaxHardwareVersion).To(Equal("vmx-18"))
				})
			})
		})

		When("the cluster cannot be resolved", func() {
			BeforeEach(func() {
				notFound := &vimtypes.ManagedObjectNotFound{
					Obj: vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: clusterMoID},
				}
				queryErr = fmt.Errorf("cluster %q not found: %w", clusterMoID, soap.WrapVimFault(notFound))
			})

			It("marks Ready=False with a ClusterNotFound-style reason and does not create VirtualMachineConfigOptions", func() {
				_, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).To(HaveOccurred())

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
				Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
				Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).To(Equal(configtarget.ClusterNotFoundReason))

				var vmcoList vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &vmcoList)).To(Succeed())
				Expect(vmcoList.Items).To(BeEmpty())
			})
		})

		When("a transient error occurs querying vSphere", func() {
			BeforeEach(func() {
				queryErr = errors.New("connection reset by peer")
			})

			It("marks Ready=False with a distinct reason, runs no GC, and signals a retry", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection reset by peer"))
				Expect(result).To(Equal(ctrl.Result{}))

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
				Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
				Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).To(Equal(configtarget.QueryConfigTargetFailedReason))
				Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).ToNot(Equal(configtarget.ClusterNotFoundReason))

				var vmcoList vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &vmcoList)).To(Succeed())
				Expect(vmcoList.Items).To(BeEmpty())
			})
		})
	})
}

var _ = Describe("ConfigTarget Controller against vcsim",
	Label(testlabels.Controller, testlabels.API, testlabels.EnvTest, testlabels.VCSim),
	vcsimTests)

func vcsimTests() {
	var (
		vcsimCtx   *builder.TestContextForVCSim
		reconciler *configtarget.Reconciler
		obj        *vimv1.ConfigTarget
		objReq     ctrl.Request
	)

	BeforeEach(func() {
		vcsimCtx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		provider := vsphere.NewVSphereVMProviderFromClient(vcsimCtx, vcsimCtx.Client, vcsimCtx.Recorder)

		reconciler = configtarget.NewReconciler(
			vcsimCtx,
			vcsimCtx.Client,
			log.Log.WithName("configtarget"),
			vcsimCtx.Recorder,
			provider)
	})

	AfterEach(func() {
		vcsimCtx.AfterEach()
	})

	When("the ConfigTarget names a real vcsim cluster", func() {
		var ccr *object.ClusterComputeResource

		BeforeEach(func() {
			ccr = vcsimCtx.GetFirstClusterFromFirstZone()
			Expect(ccr).ToNot(BeNil())

			obj = &vimv1.ConfigTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name: ccr.Reference().Value,
				},
				Spec: vimv1.ConfigTargetSpec{
					ID: vimv1.ManagedObjectID{ID: ccr.Reference().Value},
				},
			}
			Expect(vcsimCtx.Client.Create(vcsimCtx, obj)).To(Succeed())

			objReq = ctrl.Request{
				NamespacedName: types.NamespacedName{Name: obj.Name},
			}
		})

		It("populates status and creates VirtualMachineConfigOptions from the real EnvironmentBrowser result", func() {
			_, err := reconciler.Reconcile(vcsimCtx, objReq)
			Expect(err).ToNot(HaveOccurred())

			var got vimv1.ConfigTarget
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
			Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(got.Status.NumCPUs).To(BeNumerically(">", 0))

			var vmcoList vimv1.VirtualMachineConfigOptionsList
			Expect(vcsimCtx.Client.List(vcsimCtx, &vmcoList)).To(Succeed())
			Expect(vmcoList.Items).ToNot(BeEmpty())

			// vcsim's own EnvironmentBrowser simulator derives each host's
			// hardware version from its ESXi build, takes the max across the
			// cluster's hosts (2 hosts by default per
			// builder.setupVCSim/vcModel.ClusterHost), and marks every
			// descriptor up to that max as CreateSupported -- so the highest
			// VirtualMachineConfigOptions key fanned out is real,
			// vcsim-computed proof of the same multi-host aggregation
			// Status.MaxHardwareVersion performs.
			Expect(got.Status.MaxHardwareVersion).ToNot(BeEmpty())
			maxHV, err := vimtypes.ParseHardwareVersion(got.Status.MaxHardwareVersion)
			Expect(err).ToNot(HaveOccurred())
			Expect(maxHV.IsValid()).To(BeTrue())

			for _, vmco := range vmcoList.Items {
				hv, err := vimtypes.ParseHardwareVersion(vmco.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(hv).To(BeNumerically("<=", maxHV),
					"VirtualMachineConfigOptions %q exceeds Status.MaxHardwareVersion %q", vmco.Name, got.Status.MaxHardwareVersion)
			}

			Expect(maxHV).To(Equal(highestOf(vmcoList.Items)))

			// SKIP: a GC-via-vcsim-mutation scenario (dropping a previously
			// reported hardware version key at runtime) is not exercised here.
			// vcsim's EnvironmentBrowser reports a fixed, version-derived set
			// of ConfigOptionDescriptor keys that cannot be shrunk without
			// restarting the simulator, so this behavior is only covered by
			// the fake-provider-backed unit test above.
		})

		It("maps device inventory from a QueryConfigTarget result fed through the real EnvironmentBrowser", func() {
			// vcsim's own QueryConfigTarget computation never populates the
			// device-category fields (only CPU/NUMA/datastore/network), so
			// device-mapping fidelity through the real vSphere SOAP client
			// -- as opposed to the fake-provider-backed unit test above,
			// which stubs the provider and never touches EnvironmentBrowser
			// at all -- can only be exercised by pre-seeding vcsim's
			// EnvironmentBrowser.QueryConfigTargetResponse override, which
			// short-circuits vcsim's own computation and hands this value
			// straight back to the client.
			envBrowserRef, err := ccr.EnvironmentBrowser(vcsimCtx)
			Expect(err).ToNot(HaveOccurred())

			simEnvBrowser, ok := vcsimCtx.SimulatorContext().Map.Get(envBrowserRef.Reference()).(*simulator.EnvironmentBrowser)
			Expect(ok).To(BeTrue())

			simEnvBrowser.QueryConfigTargetResponse.Returnval = &vimtypes.ConfigTarget{
				NumCpus: 1,
				CdRom: []vimtypes.VirtualMachineCdromInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "cdrom0"},
				}},
				Floppy: []vimtypes.VirtualMachineFloppyInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "floppy0"},
				}},
				Serial: []vimtypes.VirtualMachineSerialInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "serial0"},
				}},
				Parallel: []vimtypes.VirtualMachineParallelInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "parallel0"},
				}},
				Sound: []vimtypes.VirtualMachineSoundInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "sound0"},
				}},
				Usb: []vimtypes.VirtualMachineUsbInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "usb0"},
				}},
				PciPassthrough: []vimtypes.BaseVirtualMachinePciPassthroughInfo{
					&vimtypes.VirtualMachinePciPassthroughInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "pci0"},
						PciDevice:                vimtypes.HostPciDevice{Id: "0000:03:00.0", DeviceName: "NIC"},
						SystemId:                 "host-1",
					},
					&vimtypes.VirtualMachineSriovInfo{
						VirtualMachinePciPassthroughInfo: vimtypes.VirtualMachinePciPassthroughInfo{
							VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "sriov0"},
							PciDevice:                vimtypes.HostPciDevice{Id: "0000:04:00.0"},
							SystemId:                 "host-1",
						},
						VirtualFunction: true,
					},
				},
				DynamicPassthrough: []vimtypes.VirtualMachineDynamicPassthroughInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "dyn0"},
					VendorName:               "NVIDIA",
					DeviceName:               "GPU",
				}},
				VgpuDeviceInfo: []vimtypes.VirtualMachineVgpuDeviceInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "vgpu0"},
					DeviceName:               "grid",
				}},
				VgpuProfileInfo: []vimtypes.VirtualMachineVgpuProfileInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "profile0"},
					ProfileName:              "grid_p1",
				}},
				SharedGpuPassthroughTypes: []vimtypes.VirtualMachinePciSharedGpuPassthroughInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "shared0"},
					Vgpu:                     "grid_p1",
				}},
				SgxTargetInfo: &vimtypes.VirtualMachineSgxTargetInfo{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "sgx"},
					MaxEpcSize:               1024,
				},
				PrecisionClockInfo: []vimtypes.VirtualMachinePrecisionClockInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "ptp0"},
					SystemClockProtocol:      "ptp",
				}},
				VendorDeviceGroupInfo: []vimtypes.VirtualMachineVendorDeviceGroupInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "vdg0"},
					DeviceGroupName:          "group1",
				}},
				DvxClassInfo: []vimtypes.VirtualMachineDvxClassInfo{{
					DeviceClass: &vimtypes.ElementDescription{Key: "dvxnic"},
					VendorName:  "Mellanox",
					SriovNic:    true,
				}},
				IdeDisk: []vimtypes.VirtualMachineIdeDiskDeviceInfo{{
					VirtualMachineDiskDeviceInfo: vimtypes.VirtualMachineDiskDeviceInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "ide0"},
						Capacity:                 1024,
					},
				}},
				ScsiDisk: []vimtypes.VirtualMachineScsiDiskDeviceInfo{{
					VirtualMachineDiskDeviceInfo: vimtypes.VirtualMachineDiskDeviceInfo{
						VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "scsi0"},
						Capacity:                 2048,
					},
					Disk: &vimtypes.HostScsiDisk{
						ScsiLun: vimtypes.ScsiLun{
							HostDevice: vimtypes.HostDevice{DeviceName: "/dev/sda", DeviceType: "disk"},
							Uuid:       "uuid1",
						},
					},
				}},
				ScsiPassthrough: []vimtypes.VirtualMachineScsiPassthroughInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "scsipt0"},
					ScsiClass:                "tape",
				}},
				VFlashModule: []vimtypes.VirtualMachineVFlashModuleInfo{{
					VirtualMachineTargetInfo: vimtypes.VirtualMachineTargetInfo{Name: "vflash0"},
					VFlashModule: vimtypes.HostVFlashManagerVFlashCacheConfigInfoVFlashModuleConfigOption{
						VFlashModule:    "vfc",
						MaxDiskSizeInKB: 1048576,
					},
				}},
			}

			_, err = reconciler.Reconcile(vcsimCtx, objReq)
			Expect(err).ToNot(HaveOccurred())

			var got vimv1.ConfigTarget
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
			Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())

			devices := got.Status.ConfigTargetDevices
			Expect(devices.CDROM).To(ConsistOf(vimv1.VirtualMachineCdromInfo{
				VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: "cdrom0"},
			}))
			Expect(devices.Floppy).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "floppy0"}))
			Expect(devices.Serial).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "serial0"}))
			Expect(devices.Parallel).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "parallel0"}))
			Expect(devices.Sound).To(ConsistOf(vimv1.VirtualMachineTargetInfo{Name: "sound0"}))
			Expect(devices.USB).To(HaveLen(1))
			Expect(devices.USB[0].Name).To(Equal("usb0"))

			Expect(devices.SRIOV).To(BeEmpty(), "ct.Sriov must not be mapped")
			Expect(devices.PCIPassthrough).To(HaveLen(1), "the SR-IOV union entry must be excluded")
			Expect(devices.PCIPassthrough[0].Name).To(Equal("pci0"))
			Expect(devices.PCIPassthrough[0].SystemID).To(Equal("host-1"))

			Expect(devices.DynamicPassthroughDevices).To(HaveLen(1))
			Expect(devices.DynamicPassthroughDevices[0].VendorName).To(Equal("NVIDIA"))

			Expect(devices.VGPUDevice).To(HaveLen(1))
			Expect(devices.VGPUDevice[0].DeviceName).To(Equal("grid"))

			Expect(devices.VGPUProfile).To(HaveLen(1))
			Expect(devices.VGPUProfile[0].ProfileName).To(Equal("grid_p1"))

			Expect(devices.SharedGPUPassthroughTypes).To(HaveLen(1))
			Expect(devices.SharedGPUPassthroughTypes[0].VGPU).To(Equal("grid_p1"))

			Expect(devices.SGXTargetInfo).ToNot(BeNil())
			Expect(devices.SGXTargetInfo.MaxEpcSize).To(Equal(int64(1024)))

			Expect(devices.PrecisionClockInfo).To(HaveLen(1))
			Expect(devices.PrecisionClockInfo[0].SystemClockProtocol).To(Equal(vimv1.HostDateTimeInfoProtocol("ptp")))

			Expect(devices.VendorDeviceGroupInfo).To(HaveLen(1))
			Expect(devices.VendorDeviceGroupInfo[0].DeviceGroupName).To(Equal("group1"))

			Expect(devices.DVXClassInfo).To(HaveLen(1))
			Expect(devices.DVXClassInfo[0].DeviceClass.Key).To(Equal("dvxnic"))
			Expect(devices.DVXClassInfo[0].SriovNic).To(BeTrue())

			Expect(devices.IDEDisks).To(HaveLen(1))
			Expect(devices.IDEDisks[0].Capacity).ToNot(BeNil())
			Expect(devices.IDEDisks[0].Capacity.Value()).To(Equal(int64(1024) * 1024))

			Expect(devices.SCSIDisks).To(HaveLen(1))
			Expect(devices.SCSIDisks[0].Disk).ToNot(BeNil())
			Expect(devices.SCSIDisks[0].Disk.UUID).To(Equal("uuid1"))

			Expect(devices.SCSIPassthrough).To(HaveLen(1))
			Expect(devices.SCSIPassthrough[0].SCSIClass).To(Equal("tape"))

			Expect(devices.VFlashModule).To(HaveLen(1))
			Expect(devices.VFlashModule[0].VFlashModule.VFlashModule).To(Equal("vfc"))
			Expect(devices.VFlashModule[0].VFlashModule.MaxDiskSize.Value()).To(Equal(int64(1048576) * 1024))
		})
	})

	When("the ConfigTarget names a cluster that does not exist", func() {
		BeforeEach(func() {
			obj = &vimv1.ConfigTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name: "domain-c-does-not-exist",
				},
				Spec: vimv1.ConfigTargetSpec{
					ID: vimv1.ManagedObjectID{ID: "domain-c-does-not-exist"},
				},
			}
			Expect(vcsimCtx.Client.Create(vcsimCtx, obj)).To(Succeed())

			objReq = ctrl.Request{
				NamespacedName: types.NamespacedName{Name: obj.Name},
			}
		})

		It("marks Ready=False and does not panic", func() {
			Expect(func() {
				_, _ = reconciler.Reconcile(vcsimCtx, objReq)
			}).ToNot(Panic())

			var got vimv1.ConfigTarget
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
			Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
		})
	})
}

// highestOf returns the highest hardware version among the given
// VirtualMachineConfigOptions objects' names.
func highestOf(items []vimv1.VirtualMachineConfigOptions) vimtypes.HardwareVersion {
	var maxVer vimtypes.HardwareVersion

	for _, item := range items {
		hv := vimtypes.MustParseHardwareVersion(item.Name)
		if hv > maxVer {
			maxVer = hv
		}
	}

	return maxVer
}
