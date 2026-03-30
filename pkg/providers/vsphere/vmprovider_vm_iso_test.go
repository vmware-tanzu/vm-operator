// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmISOTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	var (
		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm-iso", "")

		// Reduce diff from old tests: by default don't create an NIC.
		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		// Add required objects to get CD-ROM backing file name.
		cvmiName := "vmi-iso"
		objs := builder.DummyImageAndItemObjectsForCdromBacking(cvmiName, "", cvmiKind, "test-file.iso", ctx.ContentLibraryIsoItemID, true, true, resource.MustParse("100Mi"), true, true, "ISO")
		for _, obj := range objs {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		}

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = cvmiName
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = cvmiName
		vm.Spec.StorageClass = ctx.StorageClassName
		vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
			Cdrom: []vmopv1.VirtualMachineCdromSpec{{
				Name: "cdrom0",
				Image: vmopv1.VirtualMachineImageRef{
					Name: cvmiName,
					Kind: cvmiKind,
				},
			}},
		}

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
	})

	Context("return config", func() {
		JustBeforeEach(func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		})

		It("return config.files", func() {
			vmPathName := "config.files.vmPathName"
			props, err := vmProvider.GetVirtualMachineProperties(ctx, vm, []string{vmPathName})
			Expect(err).NotTo(HaveOccurred())
			var path object.DatastorePath
			path.FromString(props[vmPathName].(string))
			Expect(path.Datastore).NotTo(BeEmpty())
		})
	})

	When("Fast Deploy is enabled", func() {

		var (
			vmic vmopv1.VirtualMachineImageCache
		)

		BeforeEach(func() {
			testConfig.WithContentLibrary = true
			pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
				config.Features.FastDeploy = true
			})
			// Ensure the VM has a UID to verify the VM directory path
			// is different from the VM's Kubernetes UID on vSAN.
			vm.UID = "test-vm-iso-uid"
		})

		JustBeforeEach(func() {
			vmicName := pkgutil.VMIName(ctx.ContentLibraryIsoItemID)
			vmic = vmopv1.VirtualMachineImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: pkgcfg.FromContext(ctx).PodNamespace,
					Name:      vmicName,
				},
			}
			Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())
		})

		assertVMICNotReady := func(err error, msg, name, dcID, dsID string) {
			var e pkgerr.VMICacheNotReadyError
			ExpectWithOffset(1, errors.As(err, &e)).To(BeTrue())
			ExpectWithOffset(1, e.Message).To(Equal(msg))
			ExpectWithOffset(1, e.Name).To(Equal(name))
			ExpectWithOffset(1, e.DatacenterID).To(Equal(dcID))
			ExpectWithOffset(1, e.DatastoreID).To(Equal(dsID))
		}

		When("cache files are not ready", func() {
			It("should fail", func() {
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				assertVMICNotReady(
					err,
					"cached files not ready",
					vmic.Name,
					ctx.Datacenter.Reference().Value,
					ctx.Datastore.Reference().Value)
			})
		})

		When("cache files are ready", func() {
			JustBeforeEach(func() {
				// Simulate vSAN datastore with TopLevelDirectoryCreateSupported disabled.
				sctx := ctx.SimulatorContext()
				for _, dsEnt := range sctx.Map.All("Datastore") {
					sctx.WithLock(
						dsEnt.Reference(),
						func() {
							ds := sctx.Map.Get(dsEnt.Reference()).(*simulator.Datastore)
							ds.Capability.TopLevelDirectoryCreateSupported = ptr.To(false)
							ds.Summary.Type = string(vimtypes.HostFileSystemVolumeFileSystemTypeVsan)
						})
				}

				// Set required fields for ISO VM creation in the VMIC.
				conditions.MarkTrue(
					&vmic,
					vmopv1.VirtualMachineImageCacheConditionFilesReady)
				vmic.Status.Locations = []vmopv1.VirtualMachineImageCacheLocationStatus{
					{
						DatacenterID: ctx.Datacenter.Reference().Value,
						DatastoreID:  ctx.Datastore.Reference().Value,
						ProfileID:    ctx.StorageProfileID,
						Files:        []vmopv1.VirtualMachineImageCacheFileStatus{},
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
				Expect(libMgr.SyncLibraryItem(ctx, &library.Item{ID: ctx.ContentLibraryIsoItemID}, true)).To(Succeed())
			})

			It("should successfully create the ISO VM in a different UUID-based directory", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).NotTo(HaveOccurred())

				var moVM mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

				var p object.DatastorePath
				p.FromString(moVM.Config.Files.VmPathName)
				Expect(p.Datastore).NotTo(BeEmpty())

				// The VM path should be something like: <uuid>/test-vm-iso.vmx
				// When TopLevelDirectoryCreateSupported is false,
				// DatastoreNamespaceManager.CreateDirectory creates a
				// new UUID-based directory that is different from the
				// VM's Kubernetes UID.
				pathParts := strings.Split(p.Path, "/")
				Expect(pathParts).To(HaveLen(2))
				_, err = uuid.Parse(pathParts[0])
				Expect(err).NotTo(HaveOccurred(), "expected directory to be a UUID, got: %s", pathParts[0])
				Expect(pathParts[0]).NotTo(Equal(string(vm.UID)), "expected directory to be different from VM's K8s UID")
			})
		})
	})
}
