// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"encoding/json"
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmTests() {

	var (
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  vmprovider.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	Context("PlaceVirtualMachine", func() {

		It("noop when placement isn't needed", func() {
			vm := builder.DummyBasicVirtualMachine("test-vm", nsInfo.Namespace)
			vmConfigArgs := vmprovider.VMConfigArgs{}

			err := vmProvider.PlaceVirtualMachine(ctx, vm, vmConfigArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(vm.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
			Expect(vm.Annotations).ToNot(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
		})

		Context("When fault domains is enabled", func() {
			BeforeEach(func() {
				testConfig.WithFaultDomains = true
			})

			It("places VM in zone", func() {
				vm := builder.DummyBasicVirtualMachine("test-vm", nsInfo.Namespace)
				vmConfigArgs := vmprovider.VMConfigArgs{}

				err := vmProvider.PlaceVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				zoneName, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
				Expect(ok).To(BeTrue())
				Expect(zoneName).To(BeElementOf(ctx.ZoneNames))
			})
		})

		Context("When instance storage is enabled", func() {
			BeforeEach(func() {
				testConfig.WithInstanceStorage = true
			})

			It("places VM on a host", func() {
				storageClass := builder.DummyStorageClass()
				Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())

				vm := builder.DummyBasicVirtualMachine("test-vm", nsInfo.Namespace)
				builder.AddDummyInstanceStorageVolume(vm)

				vmConfigArgs := vmprovider.VMConfigArgs{}

				err := vmProvider.PlaceVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
				Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
			})
		})
	})

	Context("Create/Update/Delete/Exists VirtualMachine", func() {
		var (
			vmConfigArgs vmprovider.VMConfigArgs
			vm           *vmopv1alpha1.VirtualMachine
		)

		BeforeEach(func() {
			testConfig.WithContentLibrary = true
			vm = builder.DummyBasicVirtualMachine("test-vm", "")
		})

		AfterEach(func() {
			vm = nil
			vmConfigArgs = vmprovider.VMConfigArgs{}
		})

		JustBeforeEach(func() {
			vmClass := builder.DummyVirtualMachineClass()
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			var vmImage *vmopv1alpha1.VirtualMachineImage
			if testConfig.WithContentLibrary {
				vmImage = builder.DummyVirtualMachineImage(ctx.ContentLibraryImageName)
			} else {
				// Use VM created by default by vcsim.
				vmImage = builder.DummyVirtualMachineImage("DC0_C0_RP0_VM0")
			}
			Expect(ctx.Client.Create(ctx, vmImage)).To(Succeed())

			vmConfigArgs.VMClass = *vmClass
			vmConfigArgs.VMImage = vmImage
			vmConfigArgs.ContentLibraryUUID = ctx.ContentLibraryID
			vmConfigArgs.StorageProfileID = ctx.StorageProfileID

			vm.Namespace = nsInfo.Namespace
			vm.Spec.ClassName = vmClass.Name
			vm.Spec.ImageName = vmImage.Name
		})

		Context("Create VM", func() {

			It("Basic Create VM", func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Status.Phase).To(Equal(vmopv1alpha1.Created))

				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())

				Expect(vcVM.InventoryPath).To(HaveSuffix(fmt.Sprintf("/%s/%s", nsInfo.Namespace, vm.Name)))

				rp, err := vcVM.ResourcePool(ctx)
				Expect(err).ToNot(HaveOccurred())
				nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))

				// TODO: More assertions!
			})

			It("Create VM from VMTX in ContentLibrary", func() {
				imageName := "test-vm-vmtx"

				ctx.ContentLibraryItemTemplate("DC0_C0_RP0_VM0", imageName)
				vm.Spec.ImageName = imageName

				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("When fault domains is enabled", func() {
				BeforeEach(func() {
					testConfig.WithFaultDomains = true
				})

				It("creates VM in assigned zone", func() {
					azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))] //nolint:gosec
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = azName

					err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
					Expect(err).ToNot(HaveOccurred())

					vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
					Expect(vcVM).ToNot(BeNil())

					By("VM is created in the zone's ResourcePool", func() {
						rp, err := vcVM.ResourcePool(ctx)
						Expect(err).ToNot(HaveOccurred())
						nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName)
						Expect(nsRP).ToNot(BeNil())
						Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
					})
				})
			})

			Context("StorageClass is required but none specified", func() {
				It("create VM fails", func() {
					vmConfigArgs.StorageProfileID = ""
					err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
					Expect(err).To(MatchError("storage class is required but not specified"))
				})
			})
		})

		Context("Does VM Exist", func() {

			It("returns true when VM exists", func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				exists, err := vmProvider.DoesVirtualMachineExist(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})

			It("returns false when VM does not exist", func() {
				Expect(vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())

				exists, err := vmProvider.DoesVirtualMachineExist(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		Context("Update VM", func() {
			var vcVM *object.VirtualMachine

			JustBeforeEach(func() {
				// Likely "soon" we'll just fold the Placement, Create and Update VM calls into
				// a single CreateOrUpdateVM() method.
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				// Handle current quirk that VM reconfigure is done in off -> on transition.
				vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
				Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())

				vcVM = ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())
			})

			AfterEach(func() {
				vcVM = nil
			})

			It("Basic VM Status fields", func() {
				Expect(vm.Status.Phase).To(Equal(vmopv1alpha1.Created))
				Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
				Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))
				Expect(vm.Status.Host).ToNot(BeEmpty())

				vmClass := vmConfigArgs.VMClass
				Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
				Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
			})

			Context("VM Metadata", func() {

				Context("ExtraConfig Transport", func() {
					var ec map[string]interface{}

					BeforeEach(func() {
						vm.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
							Transport: vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
						}
						vmConfigArgs.VMMetadata = vmprovider.VMMetadata{
							Data: map[string]string{
								"foo.bar":       "should-be-ignored",
								"guestinfo.Foo": "foo",
							},
							Transport: vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
						}
					})

					JustBeforeEach(func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						ec = map[string]interface{}{}
						for _, option := range o.Config.ExtraConfig {
							if val := option.GetOptionValue(); val != nil {
								ec[val.Key] = val.Value.(string)
							}
						}
					})

					AfterEach(func() {
						ec = nil
					})

					It("Metadata data is included in ExtraConfig", func() {
						Expect(ec).ToNot(HaveKey("foo.bar"))
						Expect(ec).To(HaveKeyWithValue("guestinfo.Foo", "foo"))

						By("Should include default keys and values", func() {
							Expect(ec).To(HaveKeyWithValue("disk.enableUUID", "TRUE"))
							Expect(ec).To(HaveKeyWithValue("vmware.tools.gosc.ignoretoolscheck", "TRUE"))
						})
					})

					Context("JSON_EXTRA_CONFIG is specified", func() {
						BeforeEach(func() {
							b, err := json.Marshal(
								struct {
									Foo string
									Bar string
								}{
									Foo: "f00",
									Bar: "42",
								},
							)
							Expect(err).ToNot(HaveOccurred())
							testConfig.WithJSONExtraConfig = string(b)
						})

						It("Global config is included in ExtraConfig", func() {
							Expect(ec).To(HaveKeyWithValue("Foo", "f00"))
							Expect(ec).To(HaveKeyWithValue("Bar", "42"))
						})
					})
				})
			})

			Context("Network", func() {
				// This really only tests functionality that is used in gce2e.

				It("Should have a single nic", func() {
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					l := devList.SelectByType(&types.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					backing1, ok := l[0].GetVirtualDevice().Backing.(*types.VirtualEthernetCardNetworkBackingInfo)
					Expect(ok).Should(BeTrue())
					Expect(backing1.DeviceName).To(Equal("VM Network"))
				})

				Context("Multiple NICs are specified", func() {
					BeforeEach(func() {
						vm.Spec.NetworkInterfaces = []vmopv1alpha1.VirtualMachineNetworkInterface{
							{
								NetworkName:      "VM Network",
								EthernetCardType: "e1000",
							},
							{
								NetworkName: "DC0_DVPG0",
							},
						}
					})

					It("Has expected devices", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						devList := object.VirtualDeviceList(o.Config.Hardware.Device)
						l := devList.SelectByType(&types.VirtualEthernetCard{})
						Expect(l).To(HaveLen(2))

						dev1 := l[0].GetVirtualDevice()
						backing1, ok := dev1.Backing.(*types.VirtualEthernetCardNetworkBackingInfo)
						Expect(ok).Should(BeTrue())
						Expect(backing1.DeviceName).To(Equal("VM Network"))

						dev2 := l[1].GetVirtualDevice()
						backing2, ok := dev2.Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
						Expect(ok).Should(BeTrue())
						_, dvpg := getDVPG(ctx, "DC0_DVPG0")
						Expect(backing2.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
					})
				})

				Context("When global default Network is specified", func() {
					BeforeEach(func() {
						testConfig.WithContentLibrary = false
						testConfig.WithDefaultNetwork = "DC0_DVPG0"
					})

					It("Has expected devices", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						devList := object.VirtualDeviceList(o.Config.Hardware.Device)
						l := devList.SelectByType(&types.VirtualEthernetCard{})
						Expect(l).To(HaveLen(1))

						_, dvpg := getDVPG(ctx, "DC0_DVPG0")
						backing, ok := l[0].GetVirtualDevice().Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
						Expect(ok).Should(BeTrue())
						Expect(backing.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
					})
				})
			})

			Context("Disks", func() {

				Context("VM has thin provisioning", func() {
					BeforeEach(func() {
						vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
							DefaultVolumeProvisioningOptions: &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
								ThinProvisioned: pointer.Bool(true),
							},
						}
					})

					It("Succeeds", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.ThinProvisioned).To(PointTo(BeTrue()))
					})
				})

				XContext("VM has thick provisioning", func() {
					BeforeEach(func() {
						vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
							DefaultVolumeProvisioningOptions: &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
								ThinProvisioned: pointer.Bool(false),
							},
						}
					})

					It("Succeeds", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						/* vcsim CL deploy has "thick" but that isn't reflected for this disk. */
						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.ThinProvisioned).To(PointTo(BeFalse()))
					})
				})

				XContext("VM has eager zero provisioning", func() {
					BeforeEach(func() {
						vm.Spec.AdvancedOptions = &vmopv1alpha1.VirtualMachineAdvancedOptions{
							DefaultVolumeProvisioningOptions: &vmopv1alpha1.VirtualMachineVolumeProvisioningOptions{
								EagerZeroed: pointer.Bool(true),
							},
						}
					})

					It("Succeeds", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						/* vcsim CL deploy has "eagerZeroedThick" but that isn't reflected for this disk. */
						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.EagerlyScrub).To(PointTo(BeTrue()))
					})
				})

				Context("Should resize root disk", func() {
					newSize := resource.MustParse("4242Gi")

					It("Succeeds", func() {
						vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOff
						Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						disk, _ := getVMHomeDisk(ctx, vcVM, o)
						Expect(disk.CapacityInBytes).ToNot(BeEquivalentTo(newSize.Value()))
						// This is almost always 203 but sometimes it isn't for some reason, so fetch it.
						deviceKey := int(disk.Key)

						vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
							{
								Name: "this-api-stinks",
								VsphereVolume: &vmopv1alpha1.VsphereVolumeSource{
									Capacity: corev1.ResourceList{
										corev1.ResourceEphemeralStorage: newSize,
									},
									DeviceKey: &deviceKey,
								},
							},
						}

						vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
						Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())

						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						disk, _ = getVMHomeDisk(ctx, vcVM, o)
						Expect(disk.CapacityInBytes).To(BeEquivalentTo(newSize.Value()))
					})
				})
			})

			Context("CNS Volumes", func() {
				cnsVolumeName := "cns-volume-1"

				It("CSI Volumes workflow", func() {
					vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOff
					Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1alpha1.VirtualMachinePoweredOff))

					vm.Spec.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
					By("Add CNS volume to VM", func() {
						vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{
							{
								Name: cnsVolumeName,
								PersistentVolumeClaim: &vmopv1alpha1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "pvc-volume-1",
									},
								},
							},
						}

						err := vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status update pending for persistent volume: %s on VM", cnsVolumeName)))
						Expect(vm.Status.PowerState).To(Equal(vmopv1alpha1.VirtualMachinePoweredOff))
					})

					By("CNS volume is not attached", func() {
						errMsg := "blah blah blah not attached"

						vm.Status.Volumes = []vmopv1alpha1.VirtualMachineVolumeStatus{
							{
								Name:     cnsVolumeName,
								Attached: false,
								Error:    errMsg,
							},
						}

						err := vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("persistent volume: %s not attached to VM", cnsVolumeName)))
						Expect(vm.Status.PowerState).To(Equal(vmopv1alpha1.VirtualMachinePoweredOff))
					})

					By("CNS volume is attached", func() {
						vm.Status.Volumes = []vmopv1alpha1.VirtualMachineVolumeStatus{
							{
								Name:     cnsVolumeName,
								Attached: true,
							},
						}
						Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
						Expect(vm.Status.PowerState).To(Equal(vmopv1alpha1.VirtualMachinePoweredOn))
					})
				})
			})

			Context("When fault domains is enabled", func() {
				const zoneName = "az-1"

				BeforeEach(func() {
					testConfig.WithFaultDomains = true
					// Explicitly place the VM into one of the zones that the test context will create.
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
				})

				It("Reverse lookups existing VM into correct zone", func() {
					Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
					Expect(vm.Status.Zone).To(Equal(zoneName))
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

					Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
					Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
					Expect(vm.Status.Zone).To(Equal(zoneName))
				})
			})
		})

		Context("VM SetResourcePolicy", func() {
			var resourcePolicy *vmopv1alpha1.VirtualMachineSetResourcePolicy

			JustBeforeEach(func() {
				resourcePolicyName := "test-policy"
				resourcePolicy = getVirtualMachineSetResourcePolicy(resourcePolicyName, nsInfo.Namespace)
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				vmConfigArgs.ResourcePolicy = resourcePolicy

				vm.Annotations["vsphere-cluster-module-group"] = resourcePolicy.Spec.ClusterModules[0].GroupName
				vm.Spec.ResourcePolicyName = resourcePolicy.Name
				Expect(vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
				Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
			})

			AfterEach(func() {
				resourcePolicy = nil
			})

			It("Cluster Modules", func() {
				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())

				members, err := cluster.NewManager(ctx.RestClient).ListModuleMembers(ctx, resourcePolicy.Status.ClusterModules[0].ModuleUuid)
				Expect(err).ToNot(HaveOccurred())
				Expect(members).To(ContainElements(vcVM.Reference()))
			})
		})

		Context("Delete VM", func() {
			JustBeforeEach(func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())
			})

			It("when the VM is off", func() {
				uniqueID := vm.Status.UniqueID
				Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})

			It("when the VM is on", func() {
				uniqueID := vm.Status.UniqueID

				vcVM := ctx.GetVMFromMoID(uniqueID)
				Expect(vcVM).ToNot(BeNil())
				task, err := vcVM.PowerOn(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())

				// This checks that we power off the VM prior to deletion.
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})

			It("returns NotFound when VM does not exist", func() {
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				err := vmProvider.DeleteVirtualMachine(ctx, vm)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})

			Context("When fault domains is enabled", func() {
				const zoneName = "az-1"

				BeforeEach(func() {
					testConfig.WithFaultDomains = true
					// Explicitly place the VM into one of the zones that the test context will create.
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
				})

				It("Reverse lookups existing VM into correct zone", func() {
					uniqueID := vm.Status.UniqueID
					Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

					Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

					Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
					Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
				})
			})
		})

		Context("Guest Heartbeat", func() {
			JustBeforeEach(func() {
				Expect(vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
				Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
			})

			It("return guest heartbeat", func() {
				heartbeat, err := vmProvider.GetVirtualMachineGuestHeartbeat(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				// Just testing for property query: field not set in vcsim.
				Expect(heartbeat).To(BeEmpty())
			})
		})

		Context("Web console ticket", func() {
			JustBeforeEach(func() {
				Expect(vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
				Expect(vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
			})

			It("return ticket", func() {
				// vcsim doesn't implement this yet so expect an error.
				_, err := vmProvider.GetVirtualMachineWebMKSTicket(ctx, vm, "foo")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not implement: AcquireTicket"))
			})
		})

		Context("ResVMToVirtualMachineImage", func() {
			JustBeforeEach(func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())
			})

			// ResVMToVirtualMachineImage isn't actually used.
			It("returns a VirtualMachineImage", func() {
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())

				// TODO: Need to convert this VM to a vApp (and back).
				// annotations := map[string]string{}
				// annotations[versionKey] = versionVal

				image, err := vsphere.ResVMToVirtualMachineImage(ctx, vcVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).ToNot(BeEmpty())
				Expect(image.Name).Should(Equal(vcVM.Name()))
				// Expect(image.Annotations).ToNot(BeEmpty())
				// Expect(image.Annotations).To(HaveKeyWithValue(versionKey, versionVal))
			})
		})
	})
}

// getVMHomeDisk gets the VM's "home" disk. It makes some assumptions about the backing and disk name.
func getVMHomeDisk(
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	o mo.VirtualMachine) (*types.VirtualDisk, *types.VirtualDiskFlatVer2BackingInfo) {

	ExpectWithOffset(1, vcVM.Name()).ToNot(BeEmpty())
	ExpectWithOffset(1, o.Datastore).ToNot(BeEmpty())
	var dso mo.Datastore
	ExpectWithOffset(1, vcVM.Properties(ctx, o.Datastore[0], nil, &dso)).To(Succeed())

	devList := object.VirtualDeviceList(o.Config.Hardware.Device)
	l := devList.SelectByBackingInfo(&types.VirtualDiskFlatVer2BackingInfo{
		VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
			FileName: fmt.Sprintf("[%s] %s/disk-0.vmdk", dso.Name, vcVM.Name()),
		},
	})
	ExpectWithOffset(1, l).To(HaveLen(1))

	disk := l[0].(*types.VirtualDisk)
	backing := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)

	return disk, backing
}

func getDVPG(
	ctx *builder.TestContextForVCSim,
	path string) (object.NetworkReference, *object.DistributedVirtualPortgroup) {

	network, err := ctx.Finder.Network(ctx, path)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	dvpg, ok := network.(*object.DistributedVirtualPortgroup)
	ExpectWithOffset(1, ok).To(BeTrue())

	return network, dvpg
}
