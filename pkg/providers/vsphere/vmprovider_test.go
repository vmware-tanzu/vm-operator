// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vsphere "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func cpuFreqTests() {

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vmProvider = nil
	})

	Context("ComputeCPUMinFrequency", func() {
		It("returns success", func() {
			Expect(vmProvider.ComputeCPUMinFrequency(ctx)).To(Succeed())
		})
	})
}

var _ = Describe("SyncVirtualMachineImage", func() {
	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig.WithContentLibrary = true
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	When("content library item is an unexpected K8s object type", func() {
		It("should return an error", func() {
			err := vmProvider.SyncVirtualMachineImage(ctx, &imgregv1a1.ContentLibrary{}, &vmopv1.VirtualMachineImage{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("unexpected content library item K8s object type %T", &imgregv1a1.ContentLibrary{})))
		})
	})

	When("content library item is not an OVF type", func() {
		It("should return early without updating VM Image status", func() {
			isoItem := &imgregv1a1.ContentLibraryItem{
				Status: imgregv1a1.ContentLibraryItemStatus{
					Type: imgregv1a1.ContentLibraryItemTypeIso,
				},
			}
			var vmi vmopv1.VirtualMachineImage
			Expect(vmProvider.SyncVirtualMachineImage(ctx, isoItem, &vmi)).To(Succeed())
			Expect(vmi.Status).To(Equal(vmopv1.VirtualMachineImageStatus{}))
		})
	})

	When("content library item is an OVF type", func() {
		// TODO(akutz) Promote this block when the FSS WCP_VMService_FastDeploy is
		//             removed.
		When("FSS WCP_VMService_FastDeploy is enabled", func() {

			var (
				err        error
				cli        imgregv1a1.ContentLibraryItem
				vmi        vmopv1.VirtualMachineImage
				vmic       vmopv1.VirtualMachineImageCache
				vmicm      corev1.ConfigMap
				createVMIC bool
			)

			BeforeEach(func() {
				pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
				})

				cli = imgregv1a1.ContentLibraryItem{
					Spec: imgregv1a1.ContentLibraryItemSpec{
						UUID: types.UID(ctx.ContentLibraryItemID),
					},
					Status: imgregv1a1.ContentLibraryItemStatus{
						ContentVersion: "v1",
						Type:           imgregv1a1.ContentLibraryItemTypeOvf,
					},
				}

				vmi = vmopv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-namespace",
						Name:      "my-vmi",
					},
				}

				createVMIC = true
				vmicName := util.VMIName(ctx.ContentLibraryItemID)
				vmic = vmopv1.VirtualMachineImageCache{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: pkgcfg.FromContext(ctx).PodNamespace,
						Name:      vmicName,
					},
					Status: vmopv1.VirtualMachineImageCacheStatus{
						OVF: &vmopv1.VirtualMachineImageCacheOVFStatus{
							ConfigMapName:   vmicName,
							ProviderVersion: "v1",
						},
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineImageCacheConditionOVFReady,
								Status: metav1.ConditionTrue,
							},
						},
					},
				}

				vmicm = corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmic.Namespace,
						Name:      vmic.Name,
					},
					Data: map[string]string{
						"value": ovfEnvelopeYAML,
					},
				}
				Expect(ctx.Client.Create(ctx, &vmicm)).To(Succeed())
			})

			JustBeforeEach(func() {
				if createVMIC {
					status := vmic.Status.DeepCopy()
					Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())
					vmic.Status = *status
					Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())
				}
				err = vmProvider.SyncVirtualMachineImage(ctx, &cli, &vmi)
			})

			When("it fails to createOrPatch the VMICache resource", func() {
				// TODO(akutz) Add interceptors to the vcSim test context so
				//             this can be tested.
				XIt("should return an error", func() {

				})
			})

			assertVMICExists := func(namespace, name string) {
				var (
					obj vmopv1.VirtualMachineImageCache
					key = ctrlclient.ObjectKey{
						Namespace: namespace,
						Name:      name,
					}
				)
				ExpectWithOffset(1, ctx.Client.Get(ctx, key, &obj)).To(Succeed())
			}

			assertVMICNotReady := func(err error, name string) {
				var e pkgerr.VMICacheNotReadyError
				ExpectWithOffset(1, errors.As(err, &e)).To(BeTrue())
				ExpectWithOffset(1, e.Name).To(Equal(name))
			}

			When("OVF condition is False", func() {
				BeforeEach(func() {
					vmic.Status.Conditions[0].Status = metav1.ConditionFalse
					vmic.Status.Conditions[0].Message = "fubar"
				})
				It("should return an error", func() {
					Expect(err).To(MatchError("failed to get ovf: fubar: cache not ready"))
				})
			})

			When("OVF is not ready", func() {
				When("condition is missing", func() {
					BeforeEach(func() {
						createVMIC = false
					})
					It("should return ErrVMICacheNotReady", func() {
						assertVMICExists(vmic.Namespace, vmic.Name)
						assertVMICNotReady(err, vmic.Name)
					})
				})
				When("condition is unknown", func() {
					BeforeEach(func() {
						vmic.Status.Conditions[0].Status = metav1.ConditionUnknown
					})
					It("should return ErrVMICacheNotReady", func() {
						assertVMICNotReady(err, vmic.Name)
					})
				})
				When("status.ovf is nil", func() {
					BeforeEach(func() {
						vmic.Status.OVF = nil
					})
					It("should return ErrVMICacheNotReady", func() {
						assertVMICNotReady(err, vmic.Name)
					})
				})
				When("status.ovf.providerVersion does not match expected version", func() {
					BeforeEach(func() {
						vmic.Status.OVF.ProviderVersion = ""
					})
					It("should return ErrVMICacheNotReady", func() {
						assertVMICNotReady(err, vmic.Name)
					})
				})
				When("configmap is missing", func() {
					BeforeEach(func() {
						Expect(ctx.Client.Delete(ctx, &vmicm)).To(Succeed())
					})
					It("should return ErrVMICacheNotReady", func() {
						assertVMICNotReady(err, vmic.Name)
					})
				})
			})

			When("OVF is ready", func() {
				When("marshaled ovf data is invalid", func() {
					BeforeEach(func() {
						vmicm.Data["value"] = "invalid"
						Expect(ctx.Client.Update(ctx, &vmicm)).To(Succeed())
					})
					It("should return an error", func() {
						Expect(err).To(MatchError("failed to unmarshal ovf yaml into envelope: " +
							"error unmarshaling JSON: while decoding JSON: " +
							"json: cannot unmarshal string into Go value of type ovf.Envelope"))
					})
				})
				When("marshaled ovf data is valid", func() {
					It("should return success and update VM Image status accordingly", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vmi.Status.Firmware).To(Equal("efi"))
						Expect(vmi.Status.HardwareVersion).NotTo(BeNil())
						Expect(*vmi.Status.HardwareVersion).To(Equal(int32(9)))
						Expect(vmi.Status.OSInfo.ID).To(Equal("36"))
						Expect(vmi.Status.OSInfo.Type).To(Equal("otherLinuxGuest"))
						Expect(vmi.Status.Disks).To(HaveLen(1))
						Expect(vmi.Status.Disks[0].Capacity.String()).To(Equal("30Mi"))
						Expect(vmi.Status.Disks[0].Size.String()).To(Equal("18304Ki"))
					})
				})

			})
		})

		// TODO(akutz) Remove this block when the FSS WCP_VMService_FastDeploy is
		//             removed.
		When("FSS WCP_VMService_FastDeploy is disabled", func() {

			BeforeEach(func() {
				pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = false
				})
			})

			When("it fails to get the OVF envelope", func() {
				It("should return an error", func() {
					cli := &imgregv1a1.ContentLibraryItem{
						Spec: imgregv1a1.ContentLibraryItemSpec{
							// Use an invalid item ID to fail to get the OVF envelope.
							UUID: "invalid-library-ID",
						},
						Status: imgregv1a1.ContentLibraryItemStatus{
							Type: imgregv1a1.ContentLibraryItemTypeOvf,
						},
					}
					err := vmProvider.SyncVirtualMachineImage(ctx, cli, &vmopv1.VirtualMachineImage{})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get OVF envelope for library item \"invalid-library-ID\""))
				})
			})

			When("OVF envelope is nil", func() {
				It("should return an error", func() {
					ovfItem := &imgregv1a1.ContentLibraryItem{
						Spec: imgregv1a1.ContentLibraryItemSpec{
							UUID: types.UID(ctx.ContentLibraryIsoItemID),
						},
						Status: imgregv1a1.ContentLibraryItemStatus{
							Type: imgregv1a1.ContentLibraryItemTypeOvf,
						},
					}
					err := vmProvider.SyncVirtualMachineImage(ctx, ovfItem, &vmopv1.VirtualMachineImage{})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("OVF envelope is nil for library item %q", ctx.ContentLibraryIsoItemID)))
				})
			})

			When("there is a valid OVF envelope", func() {
				It("should return success and update VM Image status accordingly", func() {
					cli := &imgregv1a1.ContentLibraryItem{
						Spec: imgregv1a1.ContentLibraryItemSpec{
							UUID: types.UID(ctx.ContentLibraryItemID),
						},
						Status: imgregv1a1.ContentLibraryItemStatus{
							Type: imgregv1a1.ContentLibraryItemTypeOvf,
						},
					}
					var vmi vmopv1.VirtualMachineImage
					Expect(vmProvider.SyncVirtualMachineImage(ctx, cli, &vmi)).To(Succeed())
					Expect(vmi.Status.Firmware).To(Equal("efi"))
					Expect(vmi.Status.HardwareVersion).NotTo(BeNil())
					Expect(*vmi.Status.HardwareVersion).To(Equal(int32(9)))
					Expect(vmi.Status.OSInfo.ID).To(Equal("36"))
					Expect(vmi.Status.OSInfo.Type).To(Equal("otherLinuxGuest"))
					Expect(vmi.Status.Disks).To(HaveLen(1))
					Expect(vmi.Status.Disks[0].Capacity.String()).To(Equal("30Mi"))
					Expect(vmi.Status.Disks[0].Size.String()).To(Equal("18304Ki"))
				})
			})
		})
	})
})

const ovfEnvelopeYAML = `
diskSection:
  disk:
  - capacity: "30"
    capacityAllocationUnits: byte * 2^20
    diskId: vmdisk1
    fileRef: file1
    format: http://www.vmware.com/interfaces/specifications/vmdk.html#streamOptimized
    populatedSize: 18743296
  info: Virtual disk information
networkSection:
  info: The list of logical networks
  network:
  - description: The nat network
    name: nat
references:
- href: ttylinux-pc_i486-16.1-disk1.vmdk
  id: file1
  size: 10595840
virtualSystem:
  id: vm
  info: A virtual machine
  name: ttylinux-pc_i486-16.1
  operatingSystemSection:
    id: 36
    info: The kind of installed guest operating system
    osType: otherLinuxGuest
  virtualHardwareSection:
  - config:
    - key: firmware
      required: false
      value: efi
    - key: powerOpInfo.powerOffType
      required: false
      value: soft
    - key: powerOpInfo.resetType
      required: false
      value: soft
    - key: powerOpInfo.suspendType
      required: false
      value: soft
    - key: tools.syncTimeWithHost
      required: false
      value: "true"
    - key: tools.toolsUpgradePolicy
      required: false
      value: upgradeAtPowerCycle
    id: null
    info: Virtual hardware requirements
    item:
    - allocationUnits: hertz * 10^6
      description: Number of Virtual CPUs
      elementName: 1 virtual CPU(s)
      instanceID: "1"
      resourceType: 3
      virtualQuantity: 1
    - allocationUnits: byte * 2^20
      description: Memory Size
      elementName: 32MB of memory
      instanceID: "2"
      resourceType: 4
      virtualQuantity: 32
    - address: "0"
      description: IDE Controller
      elementName: ideController0
      instanceID: "3"
      resourceType: 5
    - addressOnParent: "0"
      elementName: disk0
      hostResource:
      - ovf:/disk/vmdisk1
      instanceID: "4"
      parent: "3"
      resourceType: 17
    - addressOnParent: "1"
      automaticAllocation: true
      config:
      - key: wakeOnLanEnabled
        required: false
        value: "false"
      connection:
      - nat
      description: E1000 ethernet adapter on "nat"
      elementName: ethernet0
      instanceID: "5"
      resourceSubType: E1000
      resourceType: 10
    - automaticAllocation: false
      elementName: video
      instanceID: "6"
      required: false
      resourceType: 24
    - automaticAllocation: false
      elementName: vmci
      instanceID: "7"
      required: false
      resourceSubType: vmware.vmci
      resourceType: 1
    system:
      elementName: Virtual Hardware Family
      instanceID: "0"
      virtualSystemIdentifier: ttylinux-pc_i486-16.1
      virtualSystemType: vmx-09`
