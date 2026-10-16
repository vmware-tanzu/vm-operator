// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	// mixedDisksImageDisplayName is the display name of the OVA the spec
	// deploys. It must be published to the content library the suite reads its
	// images from. See mixedDisksExpected for the image's layout.
	mixedDisksImageDisplayName = "mixed-disks-vm"

	// mixedDisksMarkerPrefix is the prefix of the marker written at the start
	// of every file-backed disk in the image, in the form
	// MIXED-DISKS-MARKER:<file>:<size>MiB.
	mixedDisksMarkerPrefix = "MIXED-DISKS-MARKER:"
)

// mixedDisksOVFFileOrder is the order in which the image's OVF disks
// reference their files.
var mixedDisksOVFFileOrder = []string{"z-disk.vmdk", "m-disk.vmdk", "a-disk.vmdk"}

// mixedDisksExpected describes the image's disks in OVF order. An empty marker
// means the OVF disk has no file reference and is created empty. The disks are
// in the reverse of their files' alphabetical order, with the empty disks
// interleaved, and every capacity is unique so a mismatch is visible.
var mixedDisksExpected = []struct {
	capacityBytes int64
	marker        string
}{
	{capacityBytes: 12 << 20, marker: mixedDisksMarkerPrefix + "z-disk:12MiB"},
	{capacityBytes: 6 << 20},
	{capacityBytes: 8 << 20, marker: mixedDisksMarkerPrefix + "m-disk:8MiB"},
	{capacityBytes: 4 << 20, marker: mixedDisksMarkerPrefix + "a-disk:4MiB"},
	{capacityBytes: 10 << 20},
}

// VMFastDeployDisksSpecInput is the input for the fast deploy disks spec.
type VMFastDeployDisksSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// VMFastDeployDisksSpec validates that fast deploy binds each cached disk file
// to the right VM disk, and leaves disks without a file reference empty.
//
// Fast deploy used to match the cached disk files to the VM's disks by index,
// which is only correct when Content Library lists an item's files in the same
// order as the OVF's disks. The spec deploys an OVF whose disks are in a
// different order than the files Content Library lists, and then reads the
// content of each deployed disk back from the datastore.
//
// The spec requires the mixed-disks-vm image and a datastore that exposes the VMDK flat files, such as VMFS or NFS.
func VMFastDeployDisksSpec(ctx context.Context, inputGetter func() VMFastDeployDisksSpecInput) {
	const specName = "vm-fastdeploy-disks"

	var (
		input           VMFastDeployDisksSpecInput
		config          *e2eConfig.E2EConfig
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		vCenterClient   *vim25.Client
		vmClassName     string
		storageClass    string
		imageName       string
	)

	BeforeEach(func() {
		input = inputGetter()

		Expect(input.Config).NotTo(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).NotTo(BeNil(),
			"Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).NotTo(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).NotTo(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0o755)).To(Succeed(),
			"Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			clusterProxy.GetRESTConfig(),
			filepath.Join(input.ArtifactFolder, specName),
		)
		DeferCleanup(cancelPodWatches)

		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(func() {
			vcenter.LogoutVimClient(vCenterClient)
		})

		clusterResources := config.InfraConfig.ManagementClusterConfig.Resources
		vmClassName = clusterResources.VMClassName
		storageClass = clusterResources.StorageClassName

		imageName = vmoperator.WaitForVirtualMachineImageName(
			ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, mixedDisksImageDisplayName)
		vmoperator.WaitForOVFVirtualMachineImageReady(
			ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, imageName)
	})

	for _, mode := range []string{"direct", "linked"} {
		It(fmt.Sprintf("binds each cached file to its own disk in %s mode", mode),
			Label("extended-functional", "experimental"), func() {

				vmName := fmt.Sprintf("%s-%s-%s", specName, mode, capiutil.RandomString(4))
				vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

				By("Creating a fast deploy VM in " + mode + " mode")
				vm := &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      vmName,
						Namespace: input.WCPNamespaceName,
						Annotations: map[string]string{
							"vmoperator.vmware.com/fast-deploy": mode,
						},
					},
					Spec: vmopv1.VirtualMachineSpec{
						ClassName:    vmClassName,
						ImageName:    imageName,
						StorageClass: storageClass,
						PowerState:   vmopv1.VirtualMachinePowerStateOff,
						// Promoting the disks would flatten the linked clones
						// before they are inspected.
						PromoteDisksMode: vmopv1.VirtualMachinePromoteDisksModeDisabled,
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							Disabled: true,
						},
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
				vmoperator.DeferCleanupVirtualMachine(config, svClusterClient, clusterProxy.GetKubeconfigPath(),
					vm.Namespace, vm.Name, input.SkipCleanup)

				By("Waiting for the image cache to be ready, which only happens when fast deploy is used")
				vmoperator.WaitForVirtualMachineImageCacheReady(
					ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
				vmoperator.WaitForVirtualMachineConditionCreated(
					ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

				By("Checking Content Library listed the files in a different order than the OVF")
				cachedOrder := cachedDiskFileOrder(ctx, svClusterClient,
					config.GetVariable("VMOPNamespace"), imageName)
				if equalStrings(cachedOrder, mixedDisksOVFFileOrder) {
					Skip(fmt.Sprintf(
						"Content Library listed the files in the OVF's order (%v), "+
							"so matching disks by index would also pass", cachedOrder))
				}
				By(fmt.Sprintf("OVF order %v, cache order %v", mixedDisksOVFFileOrder, cachedOrder))

				By("Reading the disks of the deployed VM")
				vmMoID := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
				var vmMO mo.VirtualMachine
				Expect(property.DefaultCollector(vCenterClient).RetrieveOne(
					ctx,
					vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID},
					[]string{"config.hardware.device"},
					&vmMO)).To(Succeed())
				Expect(vmMO.Config).NotTo(BeNil())

				disks := virtualDisksByUnit(vmMO.Config.Hardware.Device)
				Expect(disks).To(HaveLen(len(mixedDisksExpected)),
					"expected one VM disk per OVF disk")

				for i, expected := range mixedDisksExpected {
					disk := disks[i]

					By(fmt.Sprintf("Checking disk %d (unit %d)", i+1, *disk.UnitNumber))
					Expect(disk.CapacityInBytes).To(Equal(expected.capacityBytes),
						"disk %d has the wrong capacity", i+1)

					content := readDiskHead(ctx, vCenterClient, disk)
					if expected.marker == "" {
						Expect(content).NotTo(HavePrefix(mixedDisksMarkerPrefix),
							"disk %d has no file in the OVF but holds the data of another disk", i+1)
					} else {
						Expect(content).To(Equal(expected.marker),
							"disk %d holds the wrong disk's data", i+1)
					}
				}
			})
	}
}

// cachedDiskFileOrder returns the original names of the disk files in the
// order the first location of the image's VirtualMachineImageCache lists them.
func cachedDiskFileOrder(
	ctx context.Context,
	client ctrlclient.Client,
	vmopNamespace, imageName string) []string {

	cache := &vmopv1.VirtualMachineImageCache{}
	Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: vmopNamespace, Name: imageName}, cache)).
		To(Succeed(), "failed to get the image cache for image %s", imageName)
	Expect(cache.Status.Locations).NotTo(BeEmpty(), "image cache has no locations")

	var names []string
	for _, f := range cache.Status.Locations[0].Files {
		if f.Type == vmopv1.VirtualMachineImageCacheFileTypeDisk {
			names = append(names, f.Name)
		}
	}
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// virtualDisksByUnit returns the VM's disks sorted by unit number.
func virtualDisksByUnit(devices []vimtypes.BaseVirtualDevice) []*vimtypes.VirtualDisk {
	var disks []*vimtypes.VirtualDisk
	for _, d := range devices {
		if disk, ok := d.(*vimtypes.VirtualDisk); ok && disk.UnitNumber != nil {
			disks = append(disks, disk)
		}
	}
	sort.Slice(disks, func(i, j int) bool {
		return *disks[i].UnitNumber < *disks[j].UnitNumber
	})
	return disks
}

// readDiskHead returns the leading text of the disk's data, which ends at the
// first NUL byte, without surrounding whitespace. For a linked clone the data is in the disk's
// root parent, not in the delta that the VM's disk points at.
func readDiskHead(ctx context.Context, c *vim25.Client, disk *vimtypes.VirtualDisk) string {
	GinkgoHelper()

	var dsPath object.DatastorePath
	Expect(dsPath.FromString(rootBackingFileName(disk))).To(BeTrue(),
		"failed to parse the datastore path of disk %s", disk.DeviceInfo.GetDescription().Label)

	finder := find.NewFinder(c)
	dc, err := finder.DefaultDatacenter(ctx)
	Expect(err).NotTo(HaveOccurred())
	finder.SetDatacenter(dc)
	ds, err := finder.Datastore(ctx, dsPath.Datastore)
	Expect(err).NotTo(HaveOccurred())

	// The disk's data is in the flat extent next to the descriptor.
	flat := strings.TrimSuffix(dsPath.Path, path.Ext(dsPath.Path)) + "-flat.vmdk"
	rc, _, err := ds.Download(ctx, flat, &soap.DefaultDownload)
	Expect(err).NotTo(HaveOccurred(), "failed to download [%s] %s", dsPath.Datastore, flat)
	defer rc.Close()

	buf := make([]byte, 64)
	n, err := io.ReadFull(rc, buf)
	Expect(err).To(Or(BeNil(), MatchError(io.ErrUnexpectedEOF)))

	buf = buf[:n]
	if i := strings.IndexByte(string(buf), 0); i >= 0 {
		buf = buf[:i]
	}
	return strings.TrimSpace(string(buf))
}

// rootBackingFileName returns the file name of the last disk in the disk's
// backing chain.
func rootBackingFileName(disk *vimtypes.VirtualDisk) string {
	GinkgoHelper()

	switch b := disk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		for b.Parent != nil {
			b = b.Parent
		}
		return b.FileName
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		for b.Parent != nil {
			b = b.Parent
		}
		return b.FileName
	default:
		Fail(fmt.Sprintf("unsupported disk backing type %T", disk.Backing))
		return ""
	}
}
