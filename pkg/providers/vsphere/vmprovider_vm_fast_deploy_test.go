// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

//nolint:gocyclo
func vmFastDeployTests() {

	type diskResult struct {
		cachedFileName  string
		fromCache       bool
		capacityInBytes int64
	}

	const (
		vmdkExt  = ".vmdk"
		nvramExt = ".nvram"

		oneMiB = 1 * 1024 * 1024
		oneGiB = oneMiB * 1024
	)

	var (
		parentCtx                context.Context
		initObjects              []client.Object
		testConfig               builder.VCSimTestConfig
		ctx                      *builder.TestContextForVCSim
		vmProvider               providers.VirtualMachineProviderInterface
		nsInfo                   builder.WorkloadNamespaceInfo
		libMgr                   *library.Manager
		useEncryptedStorageClass bool

		vm          *vmopv1.VirtualMachine
		vmClass     *vmopv1.VirtualMachineClass
		vcVM        *object.VirtualMachine
		moVM        mo.VirtualMachine
		extraConfig object.OptionValueList

		configSpec *vimtypes.VirtualMachineConfigSpec

		vmi             *vmopv1.ClusterVirtualMachineImage
		vmic            vmopv1.VirtualMachineImageCache
		vmicm           corev1.ConfigMap
		cachedDiskPaths []string
		cachedNvramPath string

		localLibItemID string
		libItemID      string
		libItemName    string
		libItemYAML    string
		libItemVersion string

		expectedDiskNamesFromCache []string
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
			config.Features.FastDeploy = true
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

		testConfig.WithNetworkEnv = builder.NetworkEnvNamed
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
		libMgr = library.NewManager(ctx.RestClient)

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		var encStorageClass storagev1.StorageClass
		Expect(ctx.Client.Get(
			ctx,
			client.ObjectKey{Name: ctx.EncryptedStorageClassName},
			&encStorageClass)).To(Succeed())
		Expect(kubeutil.MarkEncryptedStorageClass(
			ctx,
			ctx.Client,
			encStorageClass,
			true)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.StorageClass = ctx.StorageClassName
		if useEncryptedStorageClass {
			vm.Spec.StorageClass = ctx.EncryptedStorageClassName
		}

		vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
		vm.Spec.Network.Disabled = false
		vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
			{
				Name:    "eth0",
				Network: &vmopv1common.PartialObjectRef{Name: dvpgName},
			},
		}

		if configSpec != nil {
			var w bytes.Buffer
			enc := vimtypes.NewJSONEncoder(&w)
			Expect(enc.Encode(configSpec)).To(Succeed())

			vmClass.Spec.ConfigSpec = w.Bytes()
			Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())
		}

		h := xxhash.New()
		_, _ = h.Write([]byte(strconv.Itoa(int(time.Now().UnixMicro()))))
		data := h.Sum(nil)
		libItemName = fmt.Sprintf(
			"test-image-%x",
			data[len(data)-7:])
	})

	JustAfterEach(func() {
		Expect(client.IgnoreNotFound(ctx.Client.Delete(ctx, &vmicm))).To(Succeed())
		Expect(ctx.Client.Delete(ctx, &vmic)).To(Succeed())

		if localLibItemID != "" {
			Expect(libMgr.DeleteLibraryItem(ctx, &library.Item{ID: localLibItemID})).To(Succeed())
		}

		vmi = nil
		vmic = vmopv1.VirtualMachineImageCache{}
		vmicm = corev1.ConfigMap{}

		localLibItemID = ""
		libItemID = ""
		libItemName = ""
		libItemYAML = ""
		libItemVersion = ""

		cachedDiskPaths = nil
		cachedNvramPath = ""
	})

	AfterEach(func() {
		expectedDiskNamesFromCache = nil

		if vm != nil &&
			!pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {

			By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
				Expect(vm.Status.Crypto).To(BeNil())
			})
		}

		libMgr = nil
		configSpec = nil

		vsphere.SkipVMImageCLProviderCheck = false

		vmClass = nil
		vm = nil
		vcVM = nil
		moVM = mo.VirtualMachine{}
		extraConfig = nil
		useEncryptedStorageClass = false

		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	assertVMICNotReady := func(err error, msg, name, dcID, dsID string) {
		var e pkgerr.VMICacheNotReadyError
		ExpectWithOffset(1, errors.As(err, &e)).To(BeTrue())
		ExpectWithOffset(1, e.Message).To(Equal(msg))
		ExpectWithOffset(1, e.Name).To(Equal(name))
		ExpectWithOffset(1, e.DatacenterID).To(Equal(dcID))
		ExpectWithOffset(1, e.DatastoreID).To(Equal(dsID))
	}

	DescribeTableSubtree("vm",
		func(
			vmName string,
			expectedDisks map[string]diskResult,
			expectNvram bool) {

			var (
				simVM *simulator.VirtualMachine
			)

			BeforeEach(func() {
				for _, v := range expectedDisks {
					if v.fromCache {
						expectedDiskNamesFromCache = append(
							expectedDiskNamesFromCache, v.cachedFileName)
					}
				}
			})

			AfterEach(func() {
				simVM = nil
			})

			JustBeforeEach(func() {
				By("Finding simulated vm", func() {
					vms := ctx.SimulatorContext().Map.All(
						string(vimtypes.ManagedObjectTypeVirtualMachine))
					for _, obj := range vms {
						vm := obj.(*simulator.VirtualMachine)
						if vm.Name == vmName {
							simVM = vm
							break
						}
					}
					Expect(simVM).ToNot(BeNil())
					libItemID = simVM.Reference().Value

					devices := object.VirtualDeviceList(simVM.Config.Hardware.Device)
					disks := devices.SelectByType(&vimtypes.VirtualDisk{})
					Expect(disks).To(HaveLen(1))
					b, ok := disks[0].GetVirtualDevice().Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
					Expect(ok).To(BeTrue())
					vmdkFileName := b.FileName

					var nvramFileName string
					for _, f := range simVM.LayoutEx.File {
						if strings.HasSuffix(f.Name, ".nvram") {
							nvramFileName = f.Name
							break
						}
					}

					Expect(vmdkFileName).ToNot(BeEmpty())
					Expect(nvramFileName).ToNot(BeEmpty())

					cachedDiskPaths = append(cachedDiskPaths, vmdkFileName)
				})

				By("Creating vmi", func() {
					vmi = builder.DummyClusterVirtualMachineImage(libItemName)
					vmi.Spec.ProviderRef = &vmopv1common.LocalObjectRef{
						Kind: "ClusterContentLibraryItem",
					}
					Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
					vmi.Status.ProviderItemID = libItemID
					vmi.Status.ProviderContentVersion = libItemVersion
					vmi.Status.Type = "VM"
					for i := range cachedDiskPaths {
						vmi.Status.Disks = append(
							vmi.Status.Disks,
							vmopv1.VirtualMachineImageDiskInfo{
								Name:      cachedDiskPaths[i],
								Limit:     ptr.To(resource.MustParse("10Mi")),
								Requested: ptr.To(resource.MustParse("10Mi")),
							})
					}
					conditions.MarkTrue(vmi, vmopv1.ReadyConditionType)
					Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())

					vm.Spec.ImageName = vmi.Name
					vm.Spec.Image.Name = vmi.Name
					vm.Spec.Image.Kind = cvmiKind
				})

				By("Creating vmic", func() {
					vmicName := pkgutil.VMIName(libItemID)
					vmic = vmopv1.VirtualMachineImageCache{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: pkgcfg.FromContext(ctx).PodNamespace,
							Name:      vmicName,
						},
						Spec: vmopv1.VirtualMachineImageCacheSpec{
							ProviderID: libItemID,
						},
					}
					Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())
				})
			})

			createVM := func() error {
				var err error
				vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				if err != nil {
					return err
				}

				Expect(vcVM.Properties(
					ctx,
					vcVM.Reference(),
					[]string{
						"config.extraConfig",
						"config.vAppConfig",
						"config.hardware.device",
					},
					&moVM)).To(Succeed())

				extraConfig = object.OptionValueList(moVM.Config.ExtraConfig)

				ecKeyFuVal, _ := extraConfig.GetString("fu")
				Expect(ecKeyFuVal).To(Equal("bar"))

				By("Assert expected number of disks", func() {
					devices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
					disks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))
					Expect(disks).To(HaveLen(len(expectedDisks)))
				})

				return nil
			}

			assertCapacity := func() {
				devices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
				disks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))

				for _, bd := range disks {
					d := bd.(*vimtypes.VirtualDisk)
					bfb := d.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo)
					fb := bfb.GetVirtualDeviceFileBackingInfo()
					baseFileName := path.Base(fb.FileName)
					if edr, ok := expectedDisks[baseFileName]; ok {
						Expect(d.CapacityInBytes).To(Equal(edr.capacityInBytes))
					}
				}
			}

			assertFileNames := func() {
				devices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
				disks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))
				actualDiskNames := map[string]struct{}{}

				for _, bd := range disks {
					d := bd.(*vimtypes.VirtualDisk)
					bfb := d.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo)
					fb := bfb.GetVirtualDeviceFileBackingInfo()
					baseFileName := path.Base(fb.FileName)
					actualDiskNames[baseFileName] = struct{}{}
				}

				for expectedDiskName := range expectedDisks {
					Expect(actualDiskNames).To(HaveKey(expectedDiskName))
				}
			}

			assertKeptDisks := func() {
				v, ok := extraConfig.GetString(pkgconst.VMProvKeepDisksExtraConfigKey)
				Expect(ok).To(BeTrue())
				Expect(strings.Split(v, ",")).To(ConsistOf(expectedDiskNamesFromCache))
			}

			assertNoKeptDisks := func() {
				v, ok := extraConfig.GetString(pkgconst.VMProvKeepDisksExtraConfigKey)
				Expect(ok).To(BeFalse())
				Expect(v).To(BeEmpty())
			}

			When("hardware is not ready", func() {
				It("should fail", func() {
					assertVMICNotReady(
						createVM(),
						"hardware not ready",
						vmic.Name,
						"",
						"")
				})
			})

			When("hardware is ready", func() {

				JustBeforeEach(func() {
					vmic.Status = vmopv1.VirtualMachineImageCacheStatus{
						OVF: &vmopv1.VirtualMachineImageCacheOVFStatus{
							ConfigMapName:   vmic.Name,
							ProviderVersion: libItemVersion,
						},
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineImageCacheConditionHardwareReady,
								Status: metav1.ConditionTrue,
							},
						},
					}
					Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())
				})

				When("files are not ready", func() {
					It("should fail", func() {
						assertVMICNotReady(
							createVM(),
							"cached files not ready",
							vmic.Name,
							ctx.Datacenter.Reference().Value,
							ctx.Datastore.Reference().Value)
					})
				})

				When("files are ready", func() {

					BeforeEach(func() {
						// Ensure the VM has a UID so the VM path is stable.
						vm.UID = types.UID("123")

						configSpec := vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   "fu",
									Value: "bar",
								},
							},
						}

						var w bytes.Buffer
						enc := vimtypes.NewJSONEncoder(&w)
						Expect(enc.Encode(configSpec)).To(Succeed())

						vmClass.Spec.ConfigSpec = w.Bytes()
					})

					JustBeforeEach(func() {
						conditions.MarkTrue(
							&vmic,
							vmopv1.VirtualMachineImageCacheConditionFilesReady)
						cachedFiles := make([]vmopv1.VirtualMachineImageCacheFileStatus, len(cachedDiskPaths))
						for i := range cachedDiskPaths {
							cachedFiles[i] = vmopv1.VirtualMachineImageCacheFileStatus{
								ID:       cachedDiskPaths[i],
								Type:     vmopv1.VirtualMachineImageCacheFileTypeDisk,
								DiskType: vmopv1.VolumeTypeClassic,
								Name:     cachedDiskPaths[i],
							}
						}

						vmic.Status.Locations = []vmopv1.VirtualMachineImageCacheLocationStatus{
							{
								DatacenterID: ctx.Datacenter.Reference().Value,
								DatastoreID:  ctx.Datastore.Reference().Value,
								ProfileID:    ctx.StorageProfileID,
								Files:        cachedFiles,
								Conditions: []metav1.Condition{
									{
										Type:   vmopv1.ReadyConditionType,
										Status: metav1.ConditionTrue,
									},
								},
							},
							{
								DatacenterID: ctx.Datacenter.Reference().Value,
								DatastoreID:  ctx.Datastore.Reference().Value,
								ProfileID:    ctx.EncryptedStorageProfileID,
								Files:        cachedFiles,
								Conditions: []metav1.Condition{
									{
										Type:   vmopv1.ReadyConditionType,
										Status: metav1.ConditionTrue,
									},
								},
							},
						}
						Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())
					})

					When("global default is direct mode", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
								config.FastDeployMode = pkgconst.FastDeployModeDirect
							})
						})

						It("should succeed with direct mode", func() {
							Expect(createVM()).To(Succeed())
							By("Assert no kept disks", assertNoKeptDisks)
							By("Assert disk names", assertFileNames)
							By("Assert capacity", assertCapacity)
						})

						When("vm specifies linked mode via annotation", func() {
							BeforeEach(func() {
								vm.SetAnnotation(
									pkgconst.FastDeployAnnotationKey,
									pkgconst.FastDeployModeLinked)
							})

							It("should succeed with linked mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert kept disks", assertKeptDisks)
								By("Assert disk names", assertFileNames)
							})
						})
					})

					When("global default is linked mode", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
								config.FastDeployMode = pkgconst.FastDeployModeLinked
							})
						})

						It("should succeed with linked mode", func() {
							Expect(createVM()).To(Succeed())
							By("Assert kept disks", assertKeptDisks)
							By("Assert disk names", assertFileNames)
						})

						When("disk promotion is disabled", func() {
							JustBeforeEach(func() {
								vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled
							})
							It("should succeed with linked mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert kept disks", assertKeptDisks)
								By("Assert disk names", assertFileNames)
							})
						})

						When("vm uses encrypted storage class", func() {
							BeforeEach(func() {
								useEncryptedStorageClass = true
							})

							It("should succeed by falling back to direct mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert no kept disks", assertNoKeptDisks)
								By("Assert disk names", assertFileNames)
								By("Assert capacity", assertCapacity)
							})
						})

						When("vm specifies direct mode via annotation", func() {
							BeforeEach(func() {
								vm.SetAnnotation(
									pkgconst.FastDeployAnnotationKey,
									pkgconst.FastDeployModeDirect)
							})

							It("should succeed with direct mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert no kept disks", assertNoKeptDisks)
								By("Assert disk names", assertFileNames)
								By("Assert capacity", assertCapacity)
							})
						})
					})
				})
			})
		},

		Entry(
			"vm-1",
			"DC0_C0_RP0_VM1",
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "disk1.vmdk",
					fromCache:       true,
					capacityInBytes: 10 * oneGiB,
				},
			},
			true,
		),
	)

	DescribeTableSubtree("ovf",
		func(
			ovfPath string,
			cachedDiskNames []string,
			expectedDisks map[string]diskResult,
			expectNvram bool) {

			BeforeEach(func() {
				shuffleSlice(cachedDiskNames)

				for _, v := range expectedDisks {
					if v.fromCache {
						expectedDiskNamesFromCache = append(
							expectedDiskNamesFromCache, v.cachedFileName)
					}
				}
			})

			JustBeforeEach(func() {
				By("Creating library item", func() {
					ovaAbsPathParts := []string{
						testutil.GetRootDirOrDie(),
						"test", "builder", "testdata", "images"}
					ovaAbsPathParts = append(
						ovaAbsPathParts,
						strings.Split(ovfPath, "/")...)
					ovaAbsPath := path.Join(ovaAbsPathParts...)

					yamlAbsPath := strings.Replace(
						ovaAbsPath,
						".ova",
						".yaml", 1)

					localLibItem := library.Item{
						Name:      libItemName,
						Type:      library.ItemTypeOVF,
						LibraryID: ctx.LocalContentLibraryID,
					}

					localLibItemID = builder.CreateContentLibraryItem(
						ctx,
						libMgr,
						localLibItem,
						ovaAbsPath,
					)
					Expect(localLibItemID).ToNot(BeEmpty())
					{
						data, err := os.ReadFile(yamlAbsPath)
						Expect(err).ToNot(HaveOccurred())
						libItemYAML = string(data)
					}
					{
						li, err := libMgr.GetLibraryItem(ctx, localLibItemID)
						Expect(err).ToNot(HaveOccurred())
						Expect(li.Cached).To(BeTrue())
						Expect(li.ContentVersion).ToNot(BeEmpty())
					}

					Expect(libMgr.SyncLibrary(
						ctx,
						&library.Library{
							ID: ctx.ContentLibraryID,
						})).To(Succeed())

					subLibItems, err := libMgr.GetLibraryItems(ctx, ctx.ContentLibraryID)
					Expect(err).ToNot(HaveOccurred())
					for _, subLibItem := range subLibItems {
						if subLibItem.Name == libItemName {
							libItemID = subLibItem.ID
							libItemVersion = subLibItem.ContentVersion
							break
						}
					}

					Expect(libItemID).ToNot(BeEmpty())
					Expect(libItemVersion).ToNot(BeEmpty())

					cachedDiskPaths = make([]string, len(cachedDiskNames))

					subLibItemStor, err := libMgr.ListLibraryItemStorage(ctx, libItemID)
					Expect(err).ToNot(HaveOccurred())
					Expect(subLibItemStor).ToNot(BeEmpty())
					for _, s := range subLibItemStor {
						Expect(s.StorageURIs).ToNot(BeEmpty())
						for i := range s.StorageURIs {
							p := s.StorageURIs[i]
							base := path.Base(p)
							ext := strings.ToLower(path.Ext(base))
							switch ext {
							case vmdkExt, nvramExt:
								var moDS mo.Datastore
								Expect(ctx.Datastore.Properties(
									ctx,
									ctx.Datastore.Reference(),
									[]string{"name", "info.url"}, &moDS)).To(Succeed())
								p := strings.Replace(p, moDS.Info.GetDatastoreInfo().Url, "", 1)
								p = strings.TrimPrefix(p, "/")
								p = fmt.Sprintf("[%s] %s", moDS.Name, p)

								switch ext {
								case vmdkExt:
									for j := range cachedDiskNames {
										if cachedDiskNames[j] == base {
											cachedDiskPaths[j] = p
											break
										}
									}
								case nvramExt:
									cachedNvramPath = p
								default:
									panic("unexpected extension: " + ext)
								}
							}
						}
					}

					for i := range cachedDiskNames {
						Expect(cachedDiskPaths[i]).ToNot(BeEmpty())
					}

					if expectNvram {
						Expect(cachedNvramPath).ToNot(BeEmpty())
					}
				})

				By("Creating vmi", func() {
					vmi = builder.DummyClusterVirtualMachineImage(libItemName)
					vmi.Spec.ProviderRef = &vmopv1common.LocalObjectRef{
						Kind: "ClusterContentLibraryItem",
					}
					Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
					vmi.Status.ProviderItemID = libItemID
					vmi.Status.ProviderContentVersion = libItemVersion
					vmi.Status.Type = "OVF"
					for i := range cachedDiskNames {
						vmi.Status.Disks = append(
							vmi.Status.Disks,
							vmopv1.VirtualMachineImageDiskInfo{
								Name:      cachedDiskNames[i],
								Limit:     ptr.To(resource.MustParse("10Mi")),
								Requested: ptr.To(resource.MustParse("10Mi")),
							})
					}
					conditions.MarkTrue(vmi, vmopv1.ReadyConditionType)
					Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())

					vm.Spec.ImageName = vmi.Name
					vm.Spec.Image.Name = vmi.Name
					vm.Spec.Image.Kind = cvmiKind
				})

				By("Creating vmic", func() {
					vmicName := pkgutil.VMIName(libItemID)
					vmic = vmopv1.VirtualMachineImageCache{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: pkgcfg.FromContext(ctx).PodNamespace,
							Name:      vmicName,
						},
						Spec: vmopv1.VirtualMachineImageCacheSpec{
							ProviderID: libItemID,
						},
					}
					Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())

					vmicm = corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: vmic.Namespace,
							Name:      vmic.Name,
						},
						Data: map[string]string{
							"value": libItemYAML,
						},
					}
					Expect(ctx.Client.Create(ctx, &vmicm)).To(Succeed())
				})
			})

			createVM := func() error {
				var err error
				vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				if err != nil {
					return err
				}

				Expect(vcVM.Properties(
					ctx,
					vcVM.Reference(),
					[]string{
						"config.extraConfig",
						"config.vAppConfig",
						"config.hardware.device",
					},
					&moVM)).To(Succeed())

				extraConfig = object.OptionValueList(moVM.Config.ExtraConfig)

				ecKeyFuVal, _ := extraConfig.GetString("fu")
				Expect(ecKeyFuVal).To(Equal("bar"))

				By("Assert expected number of disks", func() {
					devices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
					disks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))
					Expect(disks).To(HaveLen(len(expectedDisks)))
				})

				return nil
			}

			assertCapacity := func() {
				devices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
				disks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))

				for _, bd := range disks {
					d := bd.(*vimtypes.VirtualDisk)
					bfb := d.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo)
					fb := bfb.GetVirtualDeviceFileBackingInfo()
					baseFileName := path.Base(fb.FileName)
					if edr, ok := expectedDisks[baseFileName]; ok {
						Expect(d.CapacityInBytes).To(Equal(edr.capacityInBytes))
					}
				}
			}

			assertFileNames := func() {
				devices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
				disks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))
				actualDiskNames := map[string]struct{}{}

				for _, bd := range disks {
					d := bd.(*vimtypes.VirtualDisk)
					bfb := d.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo)
					fb := bfb.GetVirtualDeviceFileBackingInfo()
					baseFileName := path.Base(fb.FileName)
					actualDiskNames[baseFileName] = struct{}{}
				}

				for expectedDiskName := range expectedDisks {
					Expect(actualDiskNames).To(HaveKey(expectedDiskName))
				}
			}

			assertKeptDisks := func() {
				v, ok := extraConfig.GetString(pkgconst.VMProvKeepDisksExtraConfigKey)
				Expect(ok).To(BeTrue())
				Expect(strings.Split(v, ",")).To(ConsistOf(expectedDiskNamesFromCache))
			}

			assertNoKeptDisks := func() {
				v, ok := extraConfig.GetString(pkgconst.VMProvKeepDisksExtraConfigKey)
				Expect(ok).To(BeFalse())
				Expect(v).To(BeEmpty())
			}

			When("hardware is not ready", func() {
				It("should fail", func() {
					assertVMICNotReady(
						createVM(),
						"hardware not ready",
						vmic.Name,
						"",
						"")
				})
			})

			When("hardware is ready", func() {

				JustBeforeEach(func() {
					vmic.Status = vmopv1.VirtualMachineImageCacheStatus{
						OVF: &vmopv1.VirtualMachineImageCacheOVFStatus{
							ConfigMapName:   vmic.Name,
							ProviderVersion: libItemVersion,
						},
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineImageCacheConditionHardwareReady,
								Status: metav1.ConditionTrue,
							},
						},
					}
					Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())
				})

				When("files are not ready", func() {
					It("should fail", func() {
						assertVMICNotReady(
							createVM(),
							"cached files not ready",
							vmic.Name,
							ctx.Datacenter.Reference().Value,
							ctx.Datastore.Reference().Value)
					})
				})

				When("files are ready", func() {

					BeforeEach(func() {
						// Ensure the VM has a UID so the VM path is stable.
						vm.UID = types.UID("123")

						configSpec := vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   "fu",
									Value: "bar",
								},
							},
						}

						var w bytes.Buffer
						enc := vimtypes.NewJSONEncoder(&w)
						Expect(enc.Encode(configSpec)).To(Succeed())

						vmClass.Spec.ConfigSpec = w.Bytes()
					})

					JustBeforeEach(func() {
						conditions.MarkTrue(
							&vmic,
							vmopv1.VirtualMachineImageCacheConditionFilesReady)
						cachedFiles := make([]vmopv1.VirtualMachineImageCacheFileStatus, len(cachedDiskPaths))
						for i := range cachedDiskPaths {
							cachedFiles[i] = vmopv1.VirtualMachineImageCacheFileStatus{
								ID:       cachedDiskPaths[i],
								Type:     vmopv1.VirtualMachineImageCacheFileTypeDisk,
								DiskType: vmopv1.VolumeTypeClassic,
								Name:     cachedDiskNames[i],
							}
						}

						vmic.Status.Locations = []vmopv1.VirtualMachineImageCacheLocationStatus{
							{
								DatacenterID: ctx.Datacenter.Reference().Value,
								DatastoreID:  ctx.Datastore.Reference().Value,
								ProfileID:    ctx.StorageProfileID,
								Files:        cachedFiles,
								Conditions: []metav1.Condition{
									{
										Type:   vmopv1.ReadyConditionType,
										Status: metav1.ConditionTrue,
									},
								},
							},
							{
								DatacenterID: ctx.Datacenter.Reference().Value,
								DatastoreID:  ctx.Datastore.Reference().Value,
								ProfileID:    ctx.EncryptedStorageProfileID,
								Files:        cachedFiles,
								Conditions: []metav1.Condition{
									{
										Type:   vmopv1.ReadyConditionType,
										Status: metav1.ConditionTrue,
									},
								},
							},
						}
						Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())

						libMgr := library.NewManager(ctx.RestClient)
						Expect(libMgr.SyncLibraryItem(ctx,
							&library.Item{ID: libItemID},
							true)).To(Succeed())
					})

					When("global default is direct mode", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
								config.FastDeployMode = pkgconst.FastDeployModeDirect
							})
						})

						It("should succeed with direct mode", func() {
							Expect(createVM()).To(Succeed())
							By("Assert no kept disks", assertNoKeptDisks)
							By("Assert disk names", assertFileNames)
							By("Assert capacity", assertCapacity)
						})

						When("vm specifies linked mode via annotation", func() {
							BeforeEach(func() {
								vm.SetAnnotation(
									pkgconst.FastDeployAnnotationKey,
									pkgconst.FastDeployModeLinked)
							})

							It("should succeed with linked mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert kept disks", assertKeptDisks)
								By("Assert disk names", assertFileNames)
							})
						})
					})

					When("global default is linked mode", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
								config.FastDeployMode = pkgconst.FastDeployModeLinked
							})
						})

						It("should succeed with linked mode", func() {
							Expect(createVM()).To(Succeed())
							By("Assert kept disks", assertKeptDisks)
							By("Assert disk names", assertFileNames)
						})

						When("disk promotion is disabled", func() {
							JustBeforeEach(func() {
								vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled
							})
							It("should succeed with linked mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert kept disks", assertKeptDisks)
								By("Assert disk names", assertFileNames)
							})
						})

						When("vm uses encrypted storage class", func() {
							BeforeEach(func() {
								useEncryptedStorageClass = true
							})

							It("should succeed by falling back to direct mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert no kept disks", assertNoKeptDisks)
								By("Assert disk names", assertFileNames)
								By("Assert capacity", assertCapacity)
							})
						})

						When("vm specifies direct mode via annotation", func() {
							BeforeEach(func() {
								vm.SetAnnotation(
									pkgconst.FastDeployAnnotationKey,
									pkgconst.FastDeployModeDirect)
							})

							It("should succeed with direct mode", func() {
								Expect(createVM()).To(Succeed())
								By("Assert no kept disks", assertNoKeptDisks)
								By("Assert disk names", assertFileNames)
								By("Assert capacity", assertCapacity)
							})
						})
					})
				})
			})
		},

		Entry(
			"ttylinux",
			"ttylinux-pc_i486-16.1.ova",
			[]string{
				"disk0.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "disk0.vmdk",
					fromCache:       true,
					capacityInBytes: 30 * oneMiB,
				},
			},
			true,
		),

		Entry(
			"uber",
			"uber.ova",
			[]string{
				"disk0.vmdk",
				"disk1.vmdk",
				"disk2.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "disk0.vmdk",
					fromCache:       true,
					capacityInBytes: 40 * oneMiB,
				},
				"disk-1.vmdk": {
					cachedFileName:  "disk1.vmdk",
					fromCache:       true,
					capacityInBytes: 50 * oneMiB,
				},
				"disk-2.vmdk": {
					cachedFileName:  "disk2.vmdk",
					fromCache:       true,
					capacityInBytes: 60 * oneMiB,
				},
				"test-vm.vmdk": {
					capacityInBytes: 20 * oneMiB,
				},
			},
			true,
		),

		Entry(
			"vcsa",
			"vmware/vcsa/VMware-vCenter-Server-Appliance-9.1.0.0.25262036_OVF10.ova",
			[]string{
				"VMware-vCenter-Server-Appliance-9.1.0.0.25262036-swap.vmdk",
				"VMware-vCenter-Server-Appliance-9.1.0.0.25262036-system.vmdk",
				"VMware-vCenter-Server-Appliance-9.1.0.0.25262036-cloud-components.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "VMware-vCenter-Server-Appliance-9.1.0.0.25262036-system.vmdk",
					fromCache:       true,
					capacityInBytes: 49728 * oneMiB,
				},
				"disk-1.vmdk": {
					cachedFileName:  "VMware-vCenter-Server-Appliance-9.1.0.0.25262036-cloud-components.vmdk",
					fromCache:       true,
					capacityInBytes: 7779 * oneMiB,
				},
				"test-vm.vmdk": {
					capacityInBytes: 25 * oneGiB,
				},
				"test-vm_1.vmdk": {
					capacityInBytes: 25 * oneGiB,
				},
				"test-vm_2.vmdk": {
					capacityInBytes: 50 * oneGiB,
				},
				"test-vm_3.vmdk": {
					capacityInBytes: 10 * oneGiB,
				},
				"test-vm_4.vmdk": {
					capacityInBytes: 15 * oneGiB,
				},
				"test-vm_5.vmdk": {
					capacityInBytes: 10 * oneGiB,
				},
				"test-vm_6.vmdk": {
					capacityInBytes: 1 * oneGiB,
				},
				"test-vm_7.vmdk": {
					capacityInBytes: 10 * oneGiB,
				},
				"test-vm_8.vmdk": {
					capacityInBytes: 10 * oneGiB,
				},
				"test-vm_9.vmdk": {
					capacityInBytes: 100 * oneGiB,
				},
				"test-vm_10.vmdk": {
					capacityInBytes: 50 * oneGiB,
				},
				"test-vm_11.vmdk": {
					capacityInBytes: 100 * oneGiB,
				},
				"test-vm_12.vmdk": {
					capacityInBytes: 150 * oneGiB,
				},
			},
			false,
		),

		Entry(
			"esx",
			"vmware/esx/esxvmovf-25262174.ova",
			[]string{
				"disk-0.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "disk-0.vmdk",
					fromCache:       true,
					capacityInBytes: 32 * oneGiB,
				},
			},
			false,
		),

		Entry(
			"nsx",
			"vmware/nsx/nsx-unified-appliance-9.1.0.0.25262063.ova",
			[]string{
				"nsx-unified-appliance.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "nsx-unified-appliance.vmdk",
					fromCache:       true,
					capacityInBytes: 200 * oneGiB,
				},
				"test-vm.vmdk": {
					capacityInBytes: 100 * oneGiB,
				},
			},
			false,
		),

		Entry(
			"o11n",
			"vmware/o11n/O11N_VA-9.1.0.0.25262048.ova",
			[]string{
				"O11N_VA-9.1.0.0.25262048-system.vmdk",
				"O11N_VA-9.1.0.0.25262048-logs.vmdk",
				"O11N_VA-9.1.0.0.25262048-home.vmdk",
				"O11N_VA-9.1.0.0.25262048-data.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "O11N_VA-9.1.0.0.25262048-system.vmdk",
					fromCache:       true,
					capacityInBytes: 50 * oneGiB,
				},
				"disk-1.vmdk": {
					cachedFileName:  "O11N_VA-9.1.0.0.25262048-logs.vmdk",
					fromCache:       true,
					capacityInBytes: 8 * oneGiB,
				},
				"disk-2.vmdk": {
					cachedFileName:  "O11N_VA-9.1.0.0.25262048-home.vmdk",
					fromCache:       true,
					capacityInBytes: 20 * oneGiB,
				},
				"disk-3.vmdk": {
					cachedFileName:  "O11N_VA-9.1.0.0.25262048-data.vmdk",
					fromCache:       true,
					capacityInBytes: 108 * oneGiB,
				},
			},
			false,
		),

		Entry(
			"opapp",
			"vmware/opapp/Operations-Appliance-9.1.0.0.25262067.ova",
			[]string{
				"Operations-Appliance-9.1.0.0.25262067-system.vmdk",
				"Operations-Appliance-9.1.0.0.25262067-data.vmdk",
				"Operations-Appliance-9.1.0.0.25262067-cloud-components.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "Operations-Appliance-9.1.0.0.25262067-system.vmdk",
					fromCache:       true,
					capacityInBytes: 20 * oneGiB,
				},
				"disk-1.vmdk": {
					cachedFileName:  "Operations-Appliance-9.1.0.0.25262067-data.vmdk",
					fromCache:       true,
					capacityInBytes: 250 * oneGiB,
				},
				"disk-2.vmdk": {
					cachedFileName:  "Operations-Appliance-9.1.0.0.25262067-cloud-components.vmdk",
					fromCache:       true,
					capacityInBytes: 4 * oneGiB,
				},
			},
			false,
		),

		Entry(
			"opcp",
			"vmware/opcp/Operations-Cloud-Proxy-9.1.0.0.25262086.ova",
			[]string{
				"Operations-Cloud-Proxy-9.1.0.0.25262086-system.vmdk",
				"Operations-Cloud-Proxy-9.1.0.0.25262086-data.vmdk",
				"Operations-Cloud-Proxy-9.1.0.0.25262086-cloud-components.vmdk",
				"Operations-Cloud-Proxy-9.1.0.0.25262086-unified.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "Operations-Cloud-Proxy-9.1.0.0.25262086-system.vmdk",
					fromCache:       true,
					capacityInBytes: 20 * oneGiB,
				},
				"disk-1.vmdk": {
					cachedFileName:  "Operations-Cloud-Proxy-9.1.0.0.25262086-data.vmdk",
					fromCache:       true,
					capacityInBytes: 120 * oneGiB,
				},
				"disk-2.vmdk": {
					cachedFileName:  "Operations-Cloud-Proxy-9.1.0.0.25262086-cloud-components.vmdk",
					fromCache:       true,
					capacityInBytes: 4 * oneGiB,
				},
			},
			false,
		),

		Entry(
			"vcfls",
			"vmware/vcfls/Vcf-License-Server-9.1.0.0.25262073.ova",
			[]string{
				"Vcf-License-Server-9.1.0.0.25262073-disk1.vmdk",
				"Vcf-License-Server-9.1.0.0.25262073-disk2.vmdk",
			},
			map[string]diskResult{
				"disk-0.vmdk": {
					cachedFileName:  "Vcf-License-Server-9.1.0.0.25262073-disk1.vmdk",
					fromCache:       true,
					capacityInBytes: 4 * oneGiB,
				},
				"disk-1.vmdk": {
					cachedFileName:  "Vcf-License-Server-9.1.0.0.25262073-disk2.vmdk",
					fromCache:       true,
					capacityInBytes: 4 * oneGiB,
				},
				"test-vm.vmdk": {
					capacityInBytes: 4 * oneGiB,
				},
			},
			false,
		),
	)
}

func shuffleSlice[T any](a []T) {
	rand.Shuffle(len(a), func(i, j int) {
		a[i], a[j] = a[j], a[i]
	})
}
