// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/simulator"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimPlacement() {

	var (
		initObjects []client.Object
		parentCtx   context.Context
		ctx         *builder.TestContextForVCSim
		nsInfo      builder.WorkloadNamespaceInfo
		testConfig  builder.VCSimTestConfig

		vm          *vmopv1.VirtualMachine
		vmCtx       pkgctx.VirtualMachineContext
		configSpec  vimtypes.VirtualMachineConfigSpec
		constraints placement.Constraints
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		testConfig = builder.VCSimTestConfig{}

		vm = builder.DummyVirtualMachine()
		vm.Name = "placement-test"

		configSpec = vimtypes.VirtualMachineConfigSpec{
			Name: vm.Name,

			// Add a disk to prompt a datastore assignment.
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						CapacityInBytes: 1024 * 1024,
						VirtualDevice: vimtypes.VirtualDevice{
							Key:        -42,
							UnitNumber: ptr.To[int32](0),
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: ptr.To(true),
							},
						},
					},
				},
			},
		}

		constraints = placement.Constraints{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx,
			testConfig,
			initObjects...)

		sctx := ctx.SimulatorContext()
		for _, dsEnt := range sctx.Map.All("Datastore") {
			sctx.WithLock(
				dsEnt.Reference(),
				func() {
					ds := sctx.Map.Get(dsEnt.Reference()).(*simulator.Datastore)
					ds.Info.GetDatastoreInfo().SupportedVDiskFormats = []string{
						"native_512", "native_4k",
					}
				})
		}

		nsInfo = ctx.CreateWorkloadNamespace()

		vm.Namespace = nsInfo.Namespace

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	Describe("When WorkloadDomainIsolation capability enabled", func() {
		Context("Zone placement", func() {

			Context("zone already assigned", func() {
				zoneName := "in the zone"
				zone := &topologyv1.Zone{}

				JustBeforeEach(func() {
					vm.Labels[corev1.LabelTopologyZone] = zoneName
					obj := &topologyv1.Zone{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       zoneName,
							Finalizers: []string{"test"},
						},
					}
					Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: zoneName, Namespace: vm.Namespace}, zone)).To(Succeed())
				})

				It("returns success with same zone", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).ToNot(BeNil())
					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(Equal(zoneName))
					Expect(result.HostMoRef).To(BeNil())
					Expect(vm.Labels).To(HaveKeyWithValue(corev1.LabelTopologyZone, zoneName))

					// Current contract is the caller must look this up based on the pre-assigned zone but
					// we might want to change that later.
					Expect(result.PoolMoRef.Value).To(BeEmpty())
				})

				It("returns success even if assigned zone is being deleted", func() {
					Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).To(BeNil())
					Expect(result).NotTo(BeNil())
				})
			})

			Context("no placement candidates", func() {
				JustBeforeEach(func() {
					vm.Namespace = "does-not-exist"
				})

				It("returns an error", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).To(MatchError("no zones in specified namespace"))
					Expect(result).To(BeNil())
				})
			})

			It("no zone assigned, returns success", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
				Expect(result.PoolMoRef.Value).ToNot(BeEmpty())
				Expect(result.HostMoRef).To(BeNil())

				nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
			})

			Context("Only one zone exists", func() {
				BeforeEach(func() {
					testConfig.NumFaultDomains = 1
				})

				It("returns success", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(Equal(ctx.ZoneNames[0]))
					Expect(result.PoolMoRef.Value).ToNot(BeEmpty())
					Expect(result.HostMoRef).To(BeNil())

					nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
					Expect(nsRP).ToNot(BeNil())
					Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
				})

				It("returns no placement candidates error if the only zone is being deleted", func() {
					zone := &topologyv1.Zone{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ZoneNames[0], Namespace: vm.Namespace}, zone)).To(Succeed())
					zone.Finalizers = []string{"test"}
					Expect(ctx.Client.Update(ctx, zone)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).To(MatchError("no placement candidates available"))
					Expect(result).To(BeNil())
				})
			})

			Context("VM is in child RP via ResourcePolicy", func() {
				It("returns success", func() {
					resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
					Expect(resourcePolicy).ToNot(BeNil())
					childRPName := resourcePolicy.Spec.ResourcePool.Name
					Expect(childRPName).ToNot(BeEmpty())
					vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
						ResourcePolicyName: resourcePolicy.Name,
					}

					constraints.ChildRPName = childRPName
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))

					childRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, childRPName)
					Expect(childRP).ToNot(BeNil())
					Expect(result.PoolMoRef.Value).To(Equal(childRP.Reference().Value))
				})
			})

			Context("Allowed Zone Constraints", func() {
				Context("Only allowed zone does not exist", func() {
					It("returns error", func() {
						constraints.Zones = sets.New("bogus-zone")
						_, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
						Expect(err).To(MatchError("no placement candidates available after applying zone constraints: bogus-zone"))
					})
				})

				Context("Allowed zone exists", func() {
					It("returns success", func() {
						constraints.Zones = sets.New(ctx.ZoneNames[0])
						result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
						Expect(err).ToNot(HaveOccurred())
						Expect(result.ZoneName).To(Equal(ctx.ZoneNames[0]))
					})
				})
			})

			Context("Instance Storage Placement", func() {

				BeforeEach(func() {
					testConfig.WithInstanceStorage = true
					builder.AddDummyInstanceStorageVolume(vm)
					testConfig.NumFaultDomains = 1 // Only support for non-HA "HA"
				})

				When("host already assigned", func() {
					const hostMoID = "foobar-host-42"

					BeforeEach(func() {
						vm.Labels[corev1.LabelTopologyZone] = "my-zone"
						vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = hostMoID
					})

					It("returns success with same host", func() {
						result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
						Expect(err).ToNot(HaveOccurred())

						Expect(result.InstanceStoragePlacement).To(BeTrue())
						Expect(result.HostMoRef).ToNot(BeNil())
						Expect(result.HostMoRef.Value).To(Equal(hostMoID))
					})
				})

				It("returns success", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).ToNot(BeEmpty())

					Expect(result.InstanceStoragePlacement).To(BeTrue())
					Expect(result.HostMoRef).ToNot(BeNil())
					Expect(result.HostMoRef.Value).ToNot(BeEmpty())

					nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
					Expect(nsRP).ToNot(BeNil())
					Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
				})

				Context("VM is in child RP via ResourcePolicy", func() {
					It("returns success", func() {
						resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
						Expect(resourcePolicy).ToNot(BeNil())
						childRPName := resourcePolicy.Spec.ResourcePool.Name
						Expect(childRPName).ToNot(BeEmpty())
						vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
							ResourcePolicyName: resourcePolicy.Name,
						}

						constraints.ChildRPName = childRPName
						result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
						Expect(err).ToNot(HaveOccurred())

						Expect(result.ZonePlacement).To(BeTrue())
						Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))

						Expect(result.InstanceStoragePlacement).To(BeTrue())
						Expect(result.HostMoRef).ToNot(BeNil())
						Expect(result.HostMoRef.Value).ToNot(BeEmpty())

						childRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, childRPName)
						Expect(childRP).ToNot(BeNil())
						Expect(result.PoolMoRef.Value).To(Equal(childRP.Reference().Value))
					})
				})
			})
		})
	})

	Describe("When WorkloadDomainIsolation capability disabled", func() {
		BeforeEach(func() {
			testConfig = builder.VCSimTestConfig{
				WithoutWorkloadDomainIsolation: true,
			}
		})

		Context("Zone placement", func() {

			Context("zone already assigned", func() {
				zoneName := "in the zone"

				JustBeforeEach(func() {
					vm.Labels[corev1.LabelTopologyZone] = zoneName
				})

				It("returns success with same zone", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).ToNot(BeNil())
					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(Equal(zoneName))
					Expect(result.HostMoRef).To(BeNil())
					Expect(vm.Labels).To(HaveKeyWithValue(corev1.LabelTopologyZone, zoneName))

					// Current contract is the caller must look this up based on the pre-assigned zone but
					// we might want to change that later.
					Expect(result.PoolMoRef.Value).To(BeEmpty())
				})
			})

			Context("no placement candidates", func() {
				JustBeforeEach(func() {
					vm.Namespace = "does-not-exist"
				})

				It("returns an error", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).To(MatchError("no placement candidates available"))
					Expect(result).To(BeNil())
				})
			})

			It("returns success", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
				Expect(result.PoolMoRef.Value).ToNot(BeEmpty())
				Expect(result.HostMoRef).To(BeNil())

				nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
			})

			Context("Only one zone exists", func() {
				BeforeEach(func() {
					testConfig.NumFaultDomains = 1
				})

				It("returns success", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
					Expect(result.PoolMoRef.Value).ToNot(BeEmpty())
					Expect(result.HostMoRef).To(BeNil())

					nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
					Expect(nsRP).ToNot(BeNil())
					Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
				})
			})

			Context("VM is in child RP via ResourcePolicy", func() {
				It("returns success", func() {
					resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
					Expect(resourcePolicy).ToNot(BeNil())
					childRPName := resourcePolicy.Spec.ResourcePool.Name
					Expect(childRPName).ToNot(BeEmpty())
					vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
						ResourcePolicyName: resourcePolicy.Name,
					}

					constraints.ChildRPName = childRPName
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))

					childRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, childRPName)
					Expect(childRP).ToNot(BeNil())
					Expect(result.PoolMoRef.Value).To(Equal(childRP.Reference().Value))
				})
			})
		})

		Context("Allowed Zone Constraints", func() {
			Context("Only allowed zone does not exist", func() {
				It("returns error", func() {
					constraints.Zones = sets.New("bogus-zone")
					_, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).To(MatchError("no placement candidates available after applying zone constraints: bogus-zone"))
				})
			})

			Context("Allowed zone exists", func() {
				It("returns success", func() {
					constraints.Zones = sets.New(ctx.ZoneNames[0])
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())
					Expect(result.ZoneName).To(Equal(ctx.ZoneNames[0]))
				})
			})
		})

		Context("Instance Storage Placement", func() {

			BeforeEach(func() {
				testConfig.WithInstanceStorage = true
				builder.AddDummyInstanceStorageVolume(vm)
				testConfig.NumFaultDomains = 1 // Only support for non-HA "HA"
			})

			When("host already assigned", func() {
				const hostMoID = "foobar-host-42"

				BeforeEach(func() {
					vm.Labels[corev1.LabelTopologyZone] = "my-zone"
					vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = hostMoID
				})

				It("returns success with same host", func() {
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.InstanceStoragePlacement).To(BeTrue())
					Expect(result.HostMoRef).ToNot(BeNil())
					Expect(result.HostMoRef.Value).To(Equal(hostMoID))
				})
			})

			It("returns success", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).ToNot(BeEmpty())

				Expect(result.InstanceStoragePlacement).To(BeTrue())
				Expect(result.HostMoRef).ToNot(BeNil())
				Expect(result.HostMoRef.Value).ToNot(BeEmpty())

				nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
			})

			Context("VM is in child RP via ResourcePolicy", func() {
				It("returns success", func() {
					resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
					Expect(resourcePolicy).ToNot(BeNil())
					childRPName := resourcePolicy.Spec.ResourcePool.Name
					Expect(childRPName).ToNot(BeEmpty())
					vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
						ResourcePolicyName: resourcePolicy.Name,
					}

					constraints.ChildRPName = childRPName
					result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
					Expect(err).ToNot(HaveOccurred())

					Expect(result.ZonePlacement).To(BeTrue())
					Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))

					Expect(result.InstanceStoragePlacement).To(BeTrue())
					Expect(result.HostMoRef).ToNot(BeNil())
					Expect(result.HostMoRef.Value).ToNot(BeEmpty())

					childRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, childRPName)
					Expect(childRP).ToNot(BeNil())
					Expect(result.PoolMoRef.Value).To(Equal(childRP.Reference().Value))
				})
			})
		})
	})

	Describe("When FSS_WCP_VMSERVICE_FAST_DEPLOY enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
				config.Features.FastDeploy = true
			})
		})

		It("returns success", func() {
			result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.ZonePlacement).To(BeTrue())
			Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
			Expect(result.PoolMoRef).ToNot(BeZero())
			Expect(result.HostMoRef).To(BeNil())
			Expect(result.Datastores).ToNot(BeEmpty())
			Expect(result.Datastores[0].ForDisk).To(BeFalse())
			Expect(result.Datastores[0].DiskKey).To(BeZero())
			Expect(result.Datastores[0].Name).ToNot(BeEmpty())
			Expect(result.Datastores[0].MoRef).ToNot(BeZero())
			Expect(result.Datastores[0].URL).ToNot(BeZero())
			Expect(result.Datastores[0].DiskFormats).ToNot(BeEmpty())
			Expect(result.Datastores[1].ForDisk).To(BeTrue())
			Expect(result.Datastores[1].DiskKey).ToNot(BeZero())
			Expect(result.Datastores[1].Name).ToNot(BeEmpty())
			Expect(result.Datastores[1].MoRef).ToNot(BeZero())
			Expect(result.Datastores[1].URL).ToNot(BeZero())
			Expect(result.Datastores[1].DiskFormats).ToNot(BeEmpty())
		})

		Context("Only one zone exists", func() {
			BeforeEach(func() {
				testConfig.NumFaultDomains = 1
			})

			It("returns success", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
				Expect(result.PoolMoRef).ToNot(BeZero())
				Expect(result.HostMoRef).ToNot(BeNil())
				Expect(result.Datastores).ToNot(BeEmpty())
				Expect(result.Datastores[0].ForDisk).To(BeFalse())
				Expect(result.Datastores[0].DiskKey).To(BeZero())
				Expect(result.Datastores[0].Name).ToNot(BeEmpty())
				Expect(result.Datastores[0].MoRef).ToNot(BeZero())
				Expect(result.Datastores[0].URL).ToNot(BeZero())
				Expect(result.Datastores[0].DiskFormats).ToNot(BeEmpty())
			})
		})
	})

	// TODO(akutz): Delete when FSS_WCP_VMSERVICE_FAST_DEPLOY is enabled.
	Describe("When FSS_WCP_VMSERVICE_FAST_DEPLOY disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
				config.Features.FastDeploy = false
			})
		})

		It("returns success", func() {
			result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.ZonePlacement).To(BeTrue())
			Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
			Expect(result.PoolMoRef).ToNot(BeZero())
			Expect(result.HostMoRef).To(BeNil())
			Expect(result.Datastores).To(BeEmpty())
		})

		Context("Only one zone exists", func() {
			BeforeEach(func() {
				testConfig.NumFaultDomains = 1
			})

			It("returns success", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Finder, configSpec, constraints)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
				Expect(result.PoolMoRef).ToNot(BeZero())
				Expect(result.HostMoRef).To(BeNil())
				Expect(result.Datastores).To(BeEmpty())
			})
		})
	})
}
