// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimagecache_test

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineimagecache"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineimagecache/internal"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	clprov "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	clsutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/library"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {

		const (
			fakeString = "fake"

			cndHdwReady = vmopv1.VirtualMachineImageCacheConditionHardwareReady
			cndFilReady = vmopv1.VirtualMachineImageCacheConditionFilesReady
			cndPrvReady = vmopv1.VirtualMachineImageCacheConditionProviderReady
			cndRdyReady = vmopv1.ReadyConditionType
		)

		getVMICacheObj := func(
			namespace,
			providerID,
			providerVersion string,
			locations ...vmopv1.VirtualMachineImageCacheLocationSpec) vmopv1.VirtualMachineImageCache {

			return vmopv1.VirtualMachineImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    namespace,
					GenerateName: "vmi-",
				},
				Spec: vmopv1.VirtualMachineImageCacheSpec{
					ProviderID:      providerID,
					ProviderVersion: providerVersion,
					Locations:       locations,
				},
			}
		}

		const (
			vmName = "DC0_C0_RP0_VM1"
		)

		var (
			ctx       context.Context
			vcSimCtx  *builder.IntegrationTestContextForVCSim
			provider  *providerfake.VMProvider
			initEnvFn builder.InitVCSimEnvFn
			nsInfo    builder.WorkloadNamespaceInfo

			itemIDOVF      string
			itemVersionOVF string

			itemIDVM      string
			itemVersionVM string

			faker fakeClient
		)

		assertCondNil := func(g Gomega, o pkgcond.Getter, t string) {
			g.ExpectWithOffset(1, pkgcond.Get(o, t)).To(BeNil())
		}

		assertCondTrue := func(g Gomega, o pkgcond.Getter, t string) {
			c := pkgcond.Get(o, t)
			g.ExpectWithOffset(1, c).ToNot(BeNil())
			g.ExpectWithOffset(1, c.Message).To(BeEmpty())
			g.ExpectWithOffset(1, c.Reason).To(Equal(string(metav1.ConditionTrue)))
			g.ExpectWithOffset(1, c.Status).To(Equal(metav1.ConditionTrue))
		}

		assertCondFalse := func(g Gomega, o pkgcond.Getter, t, reason, message string) {
			c := pkgcond.Get(o, t)
			g.ExpectWithOffset(1, c).ToNot(BeNil())
			g.ExpectWithOffset(1, c.Message).To(HavePrefix(message))
			g.ExpectWithOffset(1, c.Reason).To(Equal(reason))
			g.ExpectWithOffset(1, c.Status).To(Equal(metav1.ConditionFalse))
		}

		assertConfigMapOVF := func(g Gomega, key ctrlclient.ObjectKey) {
			var obj corev1.ConfigMap
			g.ExpectWithOffset(1, vcSimCtx.Client.Get(ctx, key, &obj)).To(Succeed())
			g.ExpectWithOffset(1, obj.Data["value"]).To(MatchYAML(ovfEnvelopeYAML))
		}

		assertLocationOVF := func(
			g Gomega,
			obj vmopv1.VirtualMachineImageCache,
			locationIndex int) {

			var (
				spec   = obj.Spec.Locations[locationIndex]
				status = obj.Status.Locations[locationIndex]
			)

			g.ExpectWithOffset(1, status.DatacenterID).To(Equal(spec.DatacenterID))
			g.ExpectWithOffset(1, status.DatastoreID).To(Equal(spec.DatastoreID))

			ds := object.NewDatastore(
				vcSimCtx.VCClient.Client,
				vimtypes.ManagedObjectReference{
					Type:  "Datastore",
					Value: spec.DatastoreID,
				})
			dsName, err := ds.ObjectName(ctx)
			g.ExpectWithOffset(1, err).ToNot(HaveOccurred())

			itemCacheDir := clsutil.GetCacheDirectory(
				dsName,
				obj.Name,
				spec.ProfileID,
				obj.Spec.ProviderVersion)

			vmdkFileName := clsutil.GetCachedFileName(
				"ttylinux-pc_i486-16.1-disk1.vmdk") + ".vmdk"
			vmdkFilePath := path.Join(itemCacheDir, vmdkFileName)

			nvramFileName := clsutil.GetCachedFileName(
				"ttylinux-pc_i486-16.1-2.nvram") + ".nvram"
			nvramFilePath := path.Join(itemCacheDir, nvramFileName)

			g.ExpectWithOffset(1, status.Files).To(HaveLen(2))
			g.ExpectWithOffset(1, status.Files[0].ID).To(Equal(vmdkFilePath))
			g.ExpectWithOffset(1, status.Files[0].Type).To(Equal(vmopv1.VirtualMachineImageCacheFileTypeDisk))
			g.ExpectWithOffset(1, status.Files[0].DiskType).To(Equal(vmopv1.VolumeTypeClassic))
			g.ExpectWithOffset(1, status.Files[1].ID).To(Equal(nvramFilePath))
			g.ExpectWithOffset(1, status.Files[1].Type).To(Equal(vmopv1.VirtualMachineImageCacheFileTypeOther))
			g.ExpectWithOffset(1, status.Files[1].DiskType).To(BeEmpty())
		}

		assertLocationVM := func(
			g Gomega,
			obj vmopv1.VirtualMachineImageCache,
			locationIndex int) {

			var (
				spec   = obj.Spec.Locations[locationIndex]
				status = obj.Status.Locations[locationIndex]
			)

			g.ExpectWithOffset(1, status.DatacenterID).To(Equal(spec.DatacenterID))
			g.ExpectWithOffset(1, status.DatastoreID).To(Equal(spec.DatastoreID))

			ds := object.NewDatastore(
				vcSimCtx.VCClient.Client,
				vimtypes.ManagedObjectReference{
					Type:  "Datastore",
					Value: spec.DatastoreID,
				})
			dsName, err := ds.ObjectName(ctx)
			g.ExpectWithOffset(1, err).ToNot(HaveOccurred())

			itemCacheDir := clsutil.GetCacheDirectory(
				dsName,
				obj.Name,
				spec.ProfileID,
				obj.Spec.ProviderVersion)

			vmdkFileName := clsutil.GetCachedFileName(
				"disk1.vmdk") + ".vmdk"
			vmdkFilePath := path.Join(itemCacheDir, vmdkFileName)

			nvramFileName := clsutil.GetCachedFileName(
				vmName+".nvram") + ".nvram"
			nvramFilePath := path.Join(itemCacheDir, nvramFileName)

			g.ExpectWithOffset(1, status.Files).To(HaveLen(2))
			g.ExpectWithOffset(1, status.Files[0].ID).To(Equal(vmdkFilePath))
			g.ExpectWithOffset(1, status.Files[0].Type).To(Equal(vmopv1.VirtualMachineImageCacheFileTypeDisk))
			g.ExpectWithOffset(1, status.Files[0].DiskType).To(Equal(vmopv1.VolumeTypeClassic))
			g.ExpectWithOffset(1, status.Files[1].ID).To(Equal(nvramFilePath))
			g.ExpectWithOffset(1, status.Files[1].Type).To(Equal(vmopv1.VirtualMachineImageCacheFileTypeOther))
			g.ExpectWithOffset(1, status.Files[1].DiskType).To(BeEmpty())
		}

		Context("Ordered", Ordered, func() {

			BeforeAll(func() {
				_ = internal.NewContentLibraryProviderContextKey

				ctx = context.Background()
				ctx = logr.NewContext(
					ctx,
					textlogger.NewLogger(textlogger.NewConfig(
						textlogger.Verbosity(5),
						textlogger.Output(GinkgoWriter),
					)))
				ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
				ctx = cource.WithContext(ctx)

				ctx = context.WithValue(
					ctx,
					internal.NewContentLibraryProviderContextKey,
					faker.newContentLibraryProviderFn)
				ctx = context.WithValue(
					ctx,
					internal.NewCacheStorageURIsClientContextKey,
					faker.newCacheStorageURIsClientFn)

				provider = providerfake.NewVMProvider()

				vcSimCtx = builder.NewIntegrationTestContextForVCSim(
					ctx,
					builder.VCSimTestConfig{
						WithContentLibrary: true,
					},
					virtualmachineimagecache.AddToManager,
					func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
						ctx.VMProvider = provider
						return nil
					},
					initEnvFn)
				Expect(vcSimCtx).ToNot(BeNil())

				vcSimCtx.BeforeEach()
				ctx = vcSimCtx

				// Get the info for the OVF-backed image.
				itemIDOVF = vcSimCtx.ContentLibraryItemID
				libMgr := library.NewManager(vcSimCtx.RestClient)
				item, err := libMgr.GetLibraryItem(ctx, itemIDOVF)
				Expect(err).ToNot(HaveOccurred())
				itemVersionOVF = item.ContentVersion

				// Get the info for the VM-backed image.
				vms := vcSimCtx.SimulatorContext().Map.All(
					string(vimtypes.ManagedObjectTypesVirtualMachine))
				for i := range vms {
					if vms[i].Entity().Name == vmName {
						itemIDVM = vms[i].Reference().Value
						break
					}
				}
				Expect(itemIDVM).ToNot(BeEmpty())
				itemVersionVM = ""

				// Create one namespace for all the ordered tests.
				nsInfo = vcSimCtx.CreateWorkloadNamespace()
			})

			BeforeEach(func() {
				provider.VSphereClientFn = func(ctx context.Context) (*vsclient.Client, error) {
					return vsclient.NewClient(ctx, vcSimCtx.VCClientConfig)
				}
			})

			AfterEach(func() {
				faker.reset()
			})

			AfterAll(func() {
				vcSimCtx.AfterEach()
			})

			tableFn := func(
				fn func() vmopv1.VirtualMachineImageCache,
				expHdwRdy bool, expHdwMsg string,
				expPrvRdy bool, expPrvMsg string,
				expFilRdy bool, expDskMsg, expLocMsg string,
				expRdyRdy bool, expRdyMsg string,
			) {
				obj := fn()
				Expect(vcSimCtx.Client.Create(ctx, &obj)).To(Succeed())
				key := ctrlclient.ObjectKey{
					Namespace: obj.Namespace,
					Name:      obj.Name,
				}

				type expCondResult struct {
					ready bool
					msg   string
				}

				conditionTypesAndExpVals := map[string]expCondResult{
					cndHdwReady: {
						ready: expHdwRdy,
						msg:   expHdwMsg,
					},
					cndPrvReady: {
						ready: expPrvRdy,
						msg:   expPrvMsg,
					},
					cndFilReady: {
						ready: expFilRdy,
						msg:   expDskMsg,
					},
					cndRdyReady: {
						ready: expRdyRdy,
						msg:   expRdyMsg,
					},
				}

				Eventually(func(g Gomega) {
					var obj vmopv1.VirtualMachineImageCache
					g.Expect(vcSimCtx.Client.Get(ctx, key, &obj)).To(Succeed())

					isOVF := !strings.HasPrefix(obj.Spec.ProviderID, "vm-")

					for t, v := range conditionTypesAndExpVals {

						switch {
						case v.ready:

							// Verify the specified condition is true.
							assertCondTrue(g, obj, t)

							switch t {
							case cndHdwReady:
								if isOVF {
									// Verify the OVF is cached and ready.
									assertConfigMapOVF(g, key)
								}

							case cndFilReady:

								// Verify the files are cached and ready.
								g.Expect(obj.Status.Locations).To(HaveLen(1))
								assertCondTrue(g, obj.Status.Locations[0], cndRdyReady)

								if isOVF {
									assertLocationOVF(g, obj, 0)
								} else {
									assertLocationVM(g, obj, 0)
								}

							}

						case v.msg == "",
							len(obj.Spec.Locations) == 0 &&
								(t == cndFilReady || t == cndPrvReady):

							// There will be no condition when the expected
							// message is empty OR spec.locations is empty and
							// the tested condition is FilesReady.
							assertCondNil(g, obj, t)

						case v.msg != "":

							// Expect a failure condition.
							assertCondFalse(g, obj, t, "Failed", v.msg)

							if t == cndFilReady && expLocMsg != "" {
								assertCondFalse(
									g,
									obj.Status.Locations[0],
									cndRdyReady,
									"Failed",
									expLocMsg)
							}
						}
					}

				}, 5*time.Second, 1*time.Second).Should(Succeed())
			}

			DescribeTable("Failures",
				tableFn,

				Entry(
					"spec.providerID is empty",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							"",
							fakeString)
					},
					false, "", // HardwareReady
					false, "", // ProviderReady
					false, "", "", // FilesReady
					false, "spec.providerID is empty", // Ready
				),

				Entry(
					"spec.providerVersion is empty",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							fakeString,
							"")
					},
					false, "", // HardwareReady
					false, "", // ProviderReady
					false, "", "", // FilesReady
					false, "spec.providerVersion is empty", // Ready
				),

				Entry(
					"failure to get vSphere client",
					func() vmopv1.VirtualMachineImageCache {
						provider.VSphereClientFn = func(ctx context.Context) (*vsclient.Client, error) {
							return nil, errors.New("fubar")
						}
						return getVMICacheObj(
							nsInfo.Namespace,
							fakeString,
							fakeString)
					},
					false, "", // HardwareReady
					false, "", // ProviderReady
					false, "", "", // FilesReady
					false, "failed to get vSphere client: fubar", // Ready
				),

				Entry(
					"library item does not exist",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							fakeString,
							fakeString)
					},
					false, "failed to create or patch ovf configmap: failed to retrieve ovf envelope:", // HardwareReady
					false, "", // ProviderReady
					false, "", "", // FilesReady
					false, "0 of 1 completed", // Ready
				),

				Entry(
					"library item (OVF) exists and location has invalid datacenter ID",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDOVF,
							itemVersionOVF,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: fakeString,
								DatastoreID:  vcSimCtx.Datastore.Reference().Value,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					false, "invalid datacenter ID: "+fakeString, "", // FilesReady
					false, "2 of 3 completed", // Ready
				),

				Entry(
					"library item (VM) exists and location has invalid datacenter ID",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDVM,
							itemVersionVM,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: fakeString,
								DatastoreID:  vcSimCtx.Datastore.Reference().Value,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					false, "invalid datacenter ID: "+fakeString, "", // FilesReady
					false, "2 of 3 completed", // Ready
				),

				Entry(
					"library item (OVF) exists and location has invalid datastore ID",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDOVF,
							itemVersionOVF,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: vcSimCtx.Datacenter.Reference().Value,
								DatastoreID:  fakeString,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					false, "invalid datastore ID: "+fakeString, "", // FilesReady
					false, "2 of 3 completed", // Ready
				),

				Entry(
					"library item (VM) exists and location has invalid datastore ID",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDVM,
							itemVersionVM,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: vcSimCtx.Datacenter.Reference().Value,
								DatastoreID:  fakeString,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					false, "invalid datastore ID: "+fakeString, "", // FilesReady
					false, "2 of 3 completed", // Ready
				),

				Entry(
					"library item (OVF) cannot cache storage uris",
					func() vmopv1.VirtualMachineImageCache {

						faker.fakeSRIClient = true
						faker.datastoreFileExistsFn = func(
							context.Context,
							string,
							*object.Datacenter) error {

							return errors.New("query file error")
						}

						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDOVF,
							itemVersionOVF,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: vcSimCtx.Datacenter.Reference().Value,
								DatastoreID:  vcSimCtx.Datastore.Reference().Value,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					false, "0 of 1 completed", "failed to cache storage items: failed to check if file exists: query file error", // FilesReady
					false, "2 of 3 completed", // Ready
				),

				Entry(
					"library item (VM) cannot cache storage uris",
					func() vmopv1.VirtualMachineImageCache {

						faker.fakeSRIClient = true
						faker.datastoreFileExistsFn = func(
							context.Context,
							string,
							*object.Datacenter) error {

							return errors.New("query file error")
						}

						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDVM,
							itemVersionVM,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: vcSimCtx.Datacenter.Reference().Value,
								DatastoreID:  vcSimCtx.Datastore.Reference().Value,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					false, "0 of 1 completed", "failed to cache storage items: failed to check if file exists: query file error", // FilesReady
					false, "2 of 3 completed", // Ready
				),
			)

			DescribeTable("Successes",
				tableFn,

				Entry(
					"library item (OVF) with no cache locations",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(nsInfo.Namespace, itemIDOVF, itemVersionOVF)
					},
					true, "", // HardwareReady
					false, "", // ProviderReady
					false, "", "", // FilesReady
					true, "", // Ready
				),

				Entry(
					"library item (VM) with no cache locations",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(nsInfo.Namespace, itemIDVM, itemVersionVM)
					},
					true, "", // HardwareReady
					false, "", // ProviderReady
					false, "", "", // FilesReady
					true, "", // Ready
				),

				Entry(
					"library item (OVF) with a single cache location",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDOVF,
							itemVersionOVF,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: vcSimCtx.Datacenter.Reference().Value,
								DatastoreID:  vcSimCtx.Datastore.Reference().Value,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					true, "", "", // FilesReady
					true, "", // Ready
				),

				Entry(
					"library item (VM) with a single cache location",
					func() vmopv1.VirtualMachineImageCache {
						return getVMICacheObj(
							nsInfo.Namespace,
							itemIDVM,
							itemVersionVM,
							vmopv1.VirtualMachineImageCacheLocationSpec{
								DatacenterID: vcSimCtx.Datacenter.Reference().Value,
								DatastoreID:  vcSimCtx.Datastore.Reference().Value,
								ProfileID:    vcSimCtx.StorageProfileID,
							})
					},
					true, "", // HardwareReady
					true, "", // ProviderReady
					true, "", "", // FilesReady
					true, "", // Ready
				),
			)
		})

	})

type fakeClient struct {
	fakeCLSProvdr bool
	fakeSRIClient bool

	datastoreFileExistsFn func(
		ctx context.Context,
		name string,
		datacenter *object.Datacenter) error

	copyVirtualDiskFn func(
		ctx context.Context,
		srcName string, srcDatacenter *object.Datacenter,
		dstName string, dstDatacenter *object.Datacenter,
		dstSpec vimtypes.BaseVirtualDiskSpec, force bool) (*object.Task, error)

	copyDatastoreFileFn func(
		ctx context.Context,
		srcName string, srcDatacenter *object.Datacenter,
		dstName string, dstDatacenter *object.Datacenter,
		force bool) (*object.Task, error)

	makeDirectoryFn func(
		ctx context.Context,
		name string,
		datacenter *object.Datacenter,
		createParentDirectories bool) error

	waitForTaskFn func(
		ctx context.Context, task *object.Task) error

	getLibraryItemsFn func(
		ctx context.Context,
		libraryID string) ([]library.Item, error)

	getLibraryItemFn func(
		ctx context.Context,
		libraryID,
		itemName string,
		notFoundReturnErr bool) (*library.Item, error)

	getLibraryItemIDFn func(
		ctx context.Context,
		itemID string) (*library.Item, error)

	listLibraryItemsFn func(
		ctx context.Context,
		libraryID string) ([]string, error)

	updateLibraryItemFn func(
		ctx context.Context,
		itemID,
		newName string,
		newDescription *string) error

	retrieveOvfEnvelopeFromLibraryItemFn func(
		ctx context.Context,
		item *library.Item) (*ovf.Envelope, error)

	retrieveOvfEnvelopeByLibraryItemIDFn func(
		ctx context.Context,
		itemID string) (*ovf.Envelope, error)

	syncLibraryItemFn func(
		ctx context.Context,
		item *library.Item,
		force bool) error

	listLibraryItemStorageFn func(
		ctx context.Context,
		itemID string) ([]library.Storage, error)

	resolveLibraryItemStorageFn func(
		ctx context.Context,
		datacenter *object.Datacenter,
		storage []library.Storage) error

	createLibraryItemFn func(
		ctx context.Context,
		libraryItem library.Item,
		path string) error
}

func (m *fakeClient) reset() {
	m.fakeCLSProvdr = false
	m.fakeSRIClient = false

	m.datastoreFileExistsFn = nil
	m.copyVirtualDiskFn = nil
	m.copyDatastoreFileFn = nil
	m.makeDirectoryFn = nil
	m.waitForTaskFn = nil
	m.getLibraryItemsFn = nil
	m.getLibraryItemFn = nil
	m.getLibraryItemIDFn = nil
	m.listLibraryItemsFn = nil
	m.updateLibraryItemFn = nil
	m.retrieveOvfEnvelopeFromLibraryItemFn = nil
	m.retrieveOvfEnvelopeByLibraryItemIDFn = nil
	m.syncLibraryItemFn = nil
	m.listLibraryItemStorageFn = nil
	m.resolveLibraryItemStorageFn = nil
	m.createLibraryItemFn = nil
}

func (m *fakeClient) newContentLibraryProviderFn(
	context.Context,
	*rest.Client) clprov.Provider {

	if m.fakeCLSProvdr {
		return m
	}
	return nil
}

func (m *fakeClient) newCacheStorageURIsClientFn(
	c *vim25.Client) clsutil.CacheStorageURIsClient {

	if m.fakeSRIClient {
		return m
	}
	return nil
}

func (m *fakeClient) DatastoreFileExists(
	ctx context.Context,
	name string,
	datacenter *object.Datacenter) error {

	if fn := m.datastoreFileExistsFn; fn != nil {
		return fn(ctx, name, datacenter)
	}
	return nil
}

func (m *fakeClient) CopyVirtualDisk(
	ctx context.Context,
	srcName string, srcDatacenter *object.Datacenter,
	dstName string, dstDatacenter *object.Datacenter,
	dstSpec vimtypes.BaseVirtualDiskSpec, force bool) (*object.Task, error) {

	if fn := m.copyVirtualDiskFn; fn != nil {
		return fn(ctx, srcName, srcDatacenter, dstName, dstDatacenter, dstSpec, force)
	}
	return nil, nil
}

func (m *fakeClient) CopyDatastoreFile(
	ctx context.Context,
	srcName string, srcDatacenter *object.Datacenter,
	dstName string, dstDatacenter *object.Datacenter,
	force bool) (*object.Task, error) {

	if fn := m.copyDatastoreFileFn; fn != nil {
		return fn(ctx, srcName, srcDatacenter, dstName, dstDatacenter, force)
	}
	return nil, nil
}

func (m *fakeClient) MakeDirectory(
	ctx context.Context,
	name string,
	datacenter *object.Datacenter,
	createParentDirectories bool) error {

	if fn := m.makeDirectoryFn; fn != nil {
		return fn(ctx, name, datacenter, createParentDirectories)
	}
	return nil
}

func (m *fakeClient) WaitForTask(
	ctx context.Context, task *object.Task) error {

	if fn := m.waitForTaskFn; fn != nil {
		return fn(ctx, task)
	}
	return nil
}

func (m *fakeClient) GetLibraryItems(
	ctx context.Context,
	libraryID string) ([]library.Item, error) {

	if fn := m.getLibraryItemsFn; fn != nil {
		return fn(ctx, libraryID)
	}
	return nil, nil
}

func (m *fakeClient) GetLibraryItem(
	ctx context.Context,
	libraryID,
	itemName string,
	notFoundReturnErr bool) (*library.Item, error) {

	if fn := m.getLibraryItemFn; fn != nil {
		return fn(ctx, libraryID, itemName, notFoundReturnErr)
	}
	return nil, nil
}

func (m *fakeClient) GetLibraryItemID(
	ctx context.Context,
	itemID string) (*library.Item, error) {

	if fn := m.getLibraryItemIDFn; fn != nil {
		return fn(ctx, itemID)
	}
	return nil, nil
}

func (m *fakeClient) ListLibraryItems(
	ctx context.Context,
	libraryID string) ([]string, error) {

	if fn := m.listLibraryItemsFn; fn != nil {
		return fn(ctx, libraryID)
	}
	return nil, nil
}

func (m *fakeClient) UpdateLibraryItem(
	ctx context.Context,
	itemID,
	newName string,
	newDescription *string) error {

	if fn := m.updateLibraryItemFn; fn != nil {
		return fn(ctx, itemID, newName, newDescription)
	}
	return nil
}

func (m *fakeClient) RetrieveOvfEnvelopeFromLibraryItem(
	ctx context.Context,
	item *library.Item) (*ovf.Envelope, error) {

	if fn := m.retrieveOvfEnvelopeFromLibraryItemFn; fn != nil {
		return fn(ctx, item)
	}
	return nil, nil
}

func (m *fakeClient) RetrieveOvfEnvelopeByLibraryItemID(
	ctx context.Context,
	itemID string) (*ovf.Envelope, error) {

	if fn := m.retrieveOvfEnvelopeByLibraryItemIDFn; fn != nil {
		return fn(ctx, itemID)
	}
	return nil, nil
}

func (m *fakeClient) SyncLibraryItem(
	ctx context.Context,
	item *library.Item,
	force bool) error {

	if fn := m.syncLibraryItemFn; fn != nil {
		return fn(ctx, item, force)
	}
	return nil
}

func (m *fakeClient) ListLibraryItemStorage(
	ctx context.Context,
	itemID string) ([]library.Storage, error) {

	if fn := m.listLibraryItemStorageFn; fn != nil {
		return fn(ctx, itemID)
	}
	return nil, nil
}

func (m *fakeClient) ResolveLibraryItemStorage(
	ctx context.Context,
	datacenter *object.Datacenter,
	storage []library.Storage) error {

	if fn := m.resolveLibraryItemStorageFn; fn != nil {
		return fn(ctx, datacenter, storage)
	}
	return nil
}

func (m *fakeClient) CreateLibraryItem(
	ctx context.Context,
	item library.Item,
	path string) error {

	if fn := m.createLibraryItemFn; fn != nil {
		return fn(ctx, item, path)
	}
	return nil
}

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
- href: ttylinux-pc_i486-16.1-2.nvram
  id: file2
  size: 8684
virtualSystem:
  id: vm
  info: A virtual machine
  name: ttylinux-pc_i486-16.1
  operatingSystemSection:
    id: 36
    info: The kind of installed guest operating system
    osType: otherLinuxGuest
  productSection:
  - fullVersion: v1.0.0
    product: Linux
    property:
    - default: "1.15"
      key: vmware-system.tkr.os-version
      type: string
    - default: dummy-value
      key: dummy-key-configurable
      type: string
      userConfigurable: true
    - default: dummy-value
      key: dummy-key-not-configurable
      type: string
      userConfigurable: false
    - default: dummy-value
      key: dummy-key-not-configurable
      type: string
      userConfigurable: false
    vendor: LinuxVendor
    version: v1
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
    extraConfig:
    - key: vmservice.vmi.labels
      required: false
      value: example.com/hello:world,fu.bar:,another.example.com:true
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
