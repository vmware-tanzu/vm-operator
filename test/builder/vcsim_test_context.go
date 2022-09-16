// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package builder is a comment just to silence the linter
//
//nolint:goconst
package builder

import (
	goctx "context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	// Blank import to make govmomi client aware of these bindings.
	_ "github.com/vmware/govmomi/pbm/simulator"
	_ "github.com/vmware/govmomi/vapi/cluster/simulator"
	_ "github.com/vmware/govmomi/vapi/simulator"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

// VCSimTestConfig configures the vcsim environment.
type VCSimTestConfig struct {
	// WithFaultDomains enables the HA WCP_FAULTDOMAINS_FSS.
	WithFaultDomains bool

	// NumFaultDomains is the number of zones when WithFaultDomains is true.
	NumFaultDomains int

	// WithContentLibrary configures a Content Library, populated with one image's
	// name available in the TestContextForVCSim.ContentLibraryImageName.
	WithContentLibrary bool

	// WithInstanceStorage enables the WCP_INSTANCE_STORAGE FSS.
	WithInstanceStorage bool

	// WithoutStorageClass disables the storage class required, meaning that the
	// Datastore will be used instead. In WCP production the storage class is
	// always required; the Datastore is only needed for gce2e.
	WithoutStorageClass bool

	// WithJSONExtraConfig enables additional ExtraConfig that is included when
	// creating a VM.
	WithJSONExtraConfig string

	// WithDefaultNetwork string sets the default network VM NICs will use.
	// In WCP production this is never set; it only exists for current
	// limitations of gce2e.
	WithDefaultNetwork string

	// WithVMClassAsConfig enables the WCP_VM_CLASS_AS_CONFIG FSS.
	WithVMClassAsConfig bool

	// WithVMClassAsConfigDaynDate enables the WCP_VM_CLASS_AS_CONFIG_DAYNDATE FSS.
	WithVMClassAsConfigDaynDate bool
}

type TestContextForVCSim struct {
	// NOTE: Unit test in the context of test suite framework means we use
	// the fake k8s client, which is sufficient for our needs. Otherwise,
	// unit testing is a little misleading here since we're using vcsim.
	*UnitTestContext

	PodNamespace string
	VCClient     *govmomi.Client
	Finder       *find.Finder
	RestClient   *rest.Client
	Recorder     record.Recorder

	// When WithFaultDomains is true:
	ZoneCount       int
	ClustersPerZone int
	ZoneNames       []string

	// When WithContentLibrary is true:
	ContentLibraryImageName string
	ContentLibraryID        string

	// When WithoutStorageClass is false:
	StorageClassName string
	StorageProfileID string

	model             *simulator.Model
	server            *simulator.Server
	tlsServerCertPath string
	tlsServerKeyPath  string

	folder           *object.Folder
	datastore        *object.Datastore
	withFaultDomains bool

	singleCCR *object.ClusterComputeResource
	azCCRs    map[string][]*object.ClusterComputeResource
}

type WorkloadNamespaceInfo struct {
	Namespace string
	Folder    *object.Folder
}

const (
	// zoneCount is how many zones to create for HA.
	zoneCount = 3
	// clustersPerZone is how many clusters to create per zone.
	clustersPerZone = 1
)

func (s *TestSuite) NewTestContextForVCSim(
	config VCSimTestConfig,
	initObjects ...client.Object) *TestContextForVCSim {

	ctx := newTestContextForVCSim(config, initObjects)

	ctx.setupEnvFSS(config)
	ctx.setupVCSim(config)
	ctx.setupContentLibrary(config)
	ctx.setupK8sConfig(config)
	ctx.setupAZs(config)

	return ctx
}

func newTestContextForVCSim(
	config VCSimTestConfig,
	initObjects []client.Object) *TestContextForVCSim {

	fakeRecorder, _ := NewFakeRecorder()

	ctx := &TestContextForVCSim{
		UnitTestContext:  NewUnitTestContext(initObjects...),
		PodNamespace:     "vmop-pod-test",
		Recorder:         fakeRecorder,
		withFaultDomains: config.WithFaultDomains,
	}

	if ctx.withFaultDomains {
		if config.NumFaultDomains != 0 {
			ctx.ZoneCount = config.NumFaultDomains
		} else {
			ctx.ZoneCount = zoneCount
		}

		ctx.ClustersPerZone = clustersPerZone
	}

	return ctx
}

// AfterEach is a comment just to silence the linter
// TODO: Once we update ginkgo, this is more suitable as an AfterAll().
func (c *TestContextForVCSim) AfterEach() {
	if c.RestClient != nil {
		_ = c.RestClient.Logout(c)
	}
	if c.VCClient != nil {
		_ = c.VCClient.Logout(c)
	}
	if c.server != nil {
		c.server.Close()
	}
	if c.model != nil {
		c.model.Remove()
	}

	_ = os.Remove(c.tlsServerKeyPath)
	_ = os.Remove(c.tlsServerCertPath)

	c.UnitTestContext.AfterEach()
}

func (c *TestContextForVCSim) CreateWorkloadNamespace() WorkloadNamespaceInfo {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "workload-",
		},
	}
	Expect(c.Client.Create(c, ns)).To(Succeed())
	Expect(ns.Name).ToNot(BeEmpty())

	nsFolder, err := c.folder.CreateFolder(c, ns.Name)
	Expect(err).ToNot(HaveOccurred())

	if c.withFaultDomains {
		for _, azName := range c.ZoneNames {
			nsInfo := topologyv1.NamespaceInfo{
				FolderMoId: nsFolder.Reference().Value,
			}

			var nsRPs []*object.ResourcePool
			for _, ccr := range c.azCCRs[azName] {
				rp, err := ccr.ResourcePool(c)
				Expect(err).ToNot(HaveOccurred())

				nsRP, err := rp.Create(c, ns.Name, types.DefaultResourceConfigSpec())
				Expect(err).ToNot(HaveOccurred())

				nsRPs = append(nsRPs, nsRP)
			}
			Expect(nsRPs).To(HaveLen(c.ClustersPerZone))
			for _, rp := range nsRPs {
				nsInfo.PoolMoIDs = append(nsInfo.PoolMoIDs, rp.Reference().Value)
			}

			az := &topologyv1.AvailabilityZone{}
			Expect(c.Client.Get(c, client.ObjectKey{Name: azName}, az)).To(Succeed())
			if az.Spec.Namespaces == nil {
				az.Spec.Namespaces = map[string]topologyv1.NamespaceInfo{}
			}
			az.Spec.Namespaces[ns.Name] = nsInfo
			Expect(c.Client.Update(c, az)).To(Succeed())
		}
	} else {
		rp, err := c.singleCCR.ResourcePool(c)
		Expect(err).ToNot(HaveOccurred())

		nsRP, err := rp.Create(c, ns.Name, types.DefaultResourceConfigSpec())
		Expect(err).ToNot(HaveOccurred())

		ns.Annotations = map[string]string{
			"vmware-system-vm-folder":     nsFolder.Reference().Value,
			"vmware-system-resource-pool": nsRP.Reference().Value,
		}
		Expect(c.Client.Update(c, ns)).To(Succeed())
	}

	if clID := c.ContentLibraryID; clID != "" {
		csBinding := &vmopv1alpha1.ContentSourceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clID,
				Namespace: ns.Name,
			},
			ContentSourceRef: vmopv1alpha1.ContentSourceReference{
				APIVersion: vmopv1alpha1.SchemeGroupVersion.Group,
				Kind:       "ContentSource",
				Name:       clID,
			},
		}
		Expect(c.Client.Create(c, csBinding)).To(Succeed())
	}

	resourceQuota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy-resource-quota",
			Namespace: ns.Name,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceName(c.StorageClassName + ".storageclass.storage.k8s.io/persistentvolumeclaims"): resource.MustParse("1"),
			},
		},
	}
	Expect(c.Client.Create(c, resourceQuota)).To(Succeed())

	// Make trip through the Finder to populate InventoryPath.
	objRef, err := c.Finder.ObjectReference(c, nsFolder.Reference())
	Expect(err).ToNot(HaveOccurred())
	nsFolder, ok := objRef.(*object.Folder)
	Expect(ok).To(BeTrue())
	Expect(nsFolder.InventoryPath).ToNot(BeEmpty())

	return WorkloadNamespaceInfo{
		Namespace: ns.Name,
		Folder:    nsFolder,
	}
}

// TODO: Get rid of runtime env checks so this isn't needed.
func (c *TestContextForVCSim) setupEnvFSS(config VCSimTestConfig) {
	Expect(lib.SetVMOpNamespaceEnv(c.PodNamespace)).To(Succeed())

	if config.WithContentLibrary {
		Expect(os.Setenv("CONTENT_API_WAIT_SECS", "1")).To(Succeed())
	}

	faultDomains := "false"
	if config.WithFaultDomains {
		faultDomains = "true"
	}
	Expect(os.Setenv(lib.WcpFaultDomainsFSS, faultDomains)).To(Succeed())

	instanceStorage := "false"
	if config.WithInstanceStorage {
		instanceStorage = "true"
	}
	Expect(os.Setenv(lib.InstanceStorageFSS, instanceStorage)).To(Succeed())

	vmClassAsConfig := "false"
	if config.WithVMClassAsConfig {
		vmClassAsConfig = "true"
	}
	Expect(os.Setenv(lib.VMClassAsConfigFSS, vmClassAsConfig)).To(Succeed())

	vmClassAsConfigDaynDate := "false"
	if config.WithVMClassAsConfigDaynDate {
		vmClassAsConfigDaynDate = "true"
	}
	Expect(os.Setenv(lib.VMClassAsConfigDaynDateFSS, vmClassAsConfigDaynDate)).To(Succeed())

	if config.WithJSONExtraConfig != "" {
		Expect(os.Setenv("JSON_EXTRA_CONFIG", config.WithJSONExtraConfig)).To(Succeed())
	} else {
		Expect(os.Unsetenv("JSON_EXTRA_CONFIG")).To(Succeed())
	}
}

func (c *TestContextForVCSim) setupVCSim(config VCSimTestConfig) {
	c.tlsServerKeyPath, c.tlsServerCertPath = generateSelfSignedCert()
	tlsCert, err := tls.LoadX509KeyPair(c.tlsServerCertPath, c.tlsServerKeyPath)
	Expect(err).NotTo(HaveOccurred())

	vcModel := simulator.VPX()
	// By Default, the Model being used by vcsim has two ResourcePools (one for the cluster
	// and host each). Setting Model.Host=0 ensures we only have one ResourcePool, making it
	// easier to pick the ResourcePool without having to look up using a hardcoded path.
	vcModel.Host = 0
	if config.WithFaultDomains {
		vcModel.Cluster = c.ZoneCount * c.ClustersPerZone
		vcModel.ClusterHost = 2
	}

	Expect(vcModel.Create()).To(Succeed())

	vcModel.Service.RegisterEndpoints = true
	vcModel.Service.TLS = &tls.Config{
		Certificates: []tls.Certificate{
			tlsCert,
		},
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
	}

	c.model = vcModel
	c.server = c.model.Service.NewServer()

	vcClient, err := govmomi.NewClient(c, c.server.URL, true)
	Expect(err).ToNot(HaveOccurred())
	c.VCClient = vcClient

	c.RestClient = rest.NewClient(c.VCClient.Client)
	Expect(c.RestClient.Login(c, simulator.DefaultLogin)).To(Succeed())

	c.Finder = find.NewFinder(vcClient.Client)

	folder, err := c.Finder.DefaultFolder(c)
	Expect(err).ToNot(HaveOccurred())
	c.folder = folder

	datastore, err := c.Finder.DefaultDatastore(c)
	Expect(err).ToNot(HaveOccurred())
	c.datastore = datastore

	if !config.WithFaultDomains {
		ccrs, err := c.Finder.ClusterComputeResourceList(c, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(ccrs).To(HaveLen(1))
		c.singleCCR = ccrs[0]
	}

	if config.WithInstanceStorage {
		// Instance storage (because of CSI) apparently needs the hosts' FQDN to be populated.
		systems := simulator.Map.AllReference("HostNetworkSystem")
		Expect(systems).ToNot(BeEmpty())
		for _, s := range systems {
			hns, ok := s.(*simulator.HostNetworkSystem)
			Expect(ok).To(BeTrue())
			Expect(hns.Host).ToNot(BeNil())

			hns.DnsConfig = &types.HostDnsConfig{
				HostName:   hns.Host.Reference().Value,
				DomainName: "vmop.vmware.com",
			}
		}
	}
}

func (c *TestContextForVCSim) setupContentLibrary(config VCSimTestConfig) {
	if !config.WithContentLibrary {
		return
	}

	libMgr := library.NewManager(c.RestClient)

	libSpec := library.Library{
		Name: "vmop-content-library",
		Type: "LOCAL",
		Storage: []library.StorageBackings{
			{
				DatastoreID: c.datastore.Reference().Value,
				Type:        "DATASTORE",
			},
		},
	}

	clID, err := libMgr.CreateLibrary(c, libSpec)
	Expect(err).ToNot(HaveOccurred())
	Expect(clID).ToNot(BeEmpty())
	c.ContentLibraryID = clID

	clProvider := &vmopv1alpha1.ContentLibraryProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: clID,
		},
		Spec: vmopv1alpha1.ContentLibraryProviderSpec{
			UUID: clID,
		},
	}
	Expect(c.Client.Create(c, clProvider)).To(Succeed())

	cs := &vmopv1alpha1.ContentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: clID,
		},
		Spec: vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				Name: clProvider.Name,
				Kind: "ContentLibraryProvider",
			},
		},
	}
	Expect(c.Client.Create(c, cs)).To(Succeed())

	Expect(controllerutil.SetOwnerReference(cs, clProvider, c.Client.Scheme())).To(Succeed())
	Expect(c.Client.Update(c, clProvider)).To(Succeed())

	libraryItem := library.Item{
		Name:      "test-image-ovf",
		Type:      "ovf",
		LibraryID: clID,
	}
	c.ContentLibraryImageName = libraryItem.Name

	vmImage := DummyVirtualMachineImage(c.ContentLibraryImageName)
	Expect(controllerutil.SetOwnerReference(clProvider, vmImage, c.Client.Scheme())).To(Succeed())
	Expect(c.Client.Create(c, vmImage)).To(Succeed())

	createContentLibraryItem(libMgr, libraryItem,
		path.Join(testutil.GetRootDirOrDie(), "images", "ttylinux-pc_i486-16.1.ovf"))
}

func (c *TestContextForVCSim) ContentLibraryItemTemplate(srcVMName, templateName string) {
	clID := c.ContentLibraryID
	Expect(clID).ToNot(BeEmpty())

	vm, err := c.Finder.VirtualMachine(c, srcVMName)
	Expect(err).ToNot(HaveOccurred())

	folder, err := c.Finder.DefaultFolder(c)
	Expect(err).ToNot(HaveOccurred())

	rp, err := vm.ResourcePool(c)
	Expect(err).ToNot(HaveOccurred())

	spec := vcenter.Template{
		Name:     templateName,
		Library:  clID,
		SourceVM: vm.Reference().Value,
		Placement: &vcenter.Placement{
			Folder:       folder.Reference().Value,
			ResourcePool: rp.Reference().Value,
		},
	}

	_, err = vcenter.NewManager(c.RestClient).CreateTemplate(c, spec)
	Expect(err).ToNot(HaveOccurred())

	// Create the expected VirtualMachineImage for the template.
	vmImage := DummyVirtualMachineImage(templateName)
	cl := &vmopv1alpha1.ContentLibraryProvider{}
	Expect(c.Client.Get(c, client.ObjectKey{Name: clID}, cl)).To(Succeed())
	Expect(controllerutil.SetOwnerReference(cl, vmImage, c.Client.Scheme())).To(Succeed())
	Expect(c.Client.Create(c, vmImage)).To(Succeed())
}

func createContentLibraryItem(
	libMgr *library.Manager,
	libraryItem library.Item,
	itemPath string) {

	ctx := goctx.Background()

	itemID, err := libMgr.CreateLibraryItem(ctx, libraryItem)
	Expect(err).ToNot(HaveOccurred())

	sessionID, err := libMgr.CreateLibraryItemUpdateSession(ctx, library.Session{LibraryItemID: itemID})
	Expect(err).ToNot(HaveOccurred())

	uploadFunc := func(path string) error {
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()

		fi, err := f.Stat()
		if err != nil {
			return err
		}

		info := library.UpdateFile{
			Name:       filepath.Base(path),
			SourceType: "PUSH",
			Size:       fi.Size(),
		}

		update, err := libMgr.AddLibraryItemFile(ctx, sessionID, info)
		if err != nil {
			return err
		}

		u, err := url.Parse(update.UploadEndpoint.URI)
		if err != nil {
			return err
		}

		p := soap.DefaultUpload
		p.ContentLength = info.Size

		return libMgr.Client.Upload(ctx, f, u, &p)
	}
	Expect(uploadFunc(itemPath)).To(Succeed())
	Expect(libMgr.CompleteLibraryItemUpdateSession(ctx, sessionID)).To(Succeed())
}

func (c *TestContextForVCSim) setupK8sConfig(config VCSimTestConfig) {
	password, _ := simulator.DefaultLogin.Password()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vmop-vcsim-dummy-creds",
			Namespace: c.PodNamespace,
		},
		Data: map[string][]byte{
			"username": []byte(simulator.DefaultLogin.Username()),
			"password": []byte(password),
		},
	}

	Expect(c.Client.Create(c, secret)).To(Succeed())

	dc, err := c.Finder.DefaultDatacenter(c)
	Expect(err).ToNot(HaveOccurred())

	data := map[string]string{}
	data["VcPNID"] = c.server.URL.Hostname()
	data["VcPort"] = c.server.URL.Port()
	data["VcCredsSecretName"] = secret.Name
	data["Datacenter"] = dc.Reference().Value
	data["CAFilePath"] = c.tlsServerCertPath
	data["InsecureSkipTLSVerify"] = "false"

	if !config.WithFaultDomains {
		rp, err := c.singleCCR.ResourcePool(c)
		Expect(err).ToNot(HaveOccurred())
		data["ResourcePool"] = rp.Reference().Value
	}

	if config.WithoutStorageClass {
		// Only used in gce2e (LocalDS_0)
		data["StorageClassRequired"] = "false"
		data["Datastore"] = c.datastore.Name()
	} else {
		data["StorageClassRequired"] = "true"

		c.StorageClassName = "vcsim-default-storageclass"
		// Use the hardcoded vcsim profile ID.
		c.StorageProfileID = "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"

		storageClass := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.StorageClassName,
			},
			Parameters: map[string]string{
				"storagePolicyID": c.StorageProfileID,
			},
		}
		Expect(c.Client.Create(c, storageClass)).To(Succeed())
	}

	if !config.WithContentLibrary {
		data["UseInventoryAsContentSource"] = "true"
	}

	if config.WithDefaultNetwork != "" {
		data["Network"] = config.WithDefaultNetwork
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vsphere.provider.config.vmoperator.vmware.com",
			Namespace: c.PodNamespace,
		},
		Data: data,
	}

	Expect(c.Client.Create(c, cm)).To(Succeed())

	networkCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vmoperator-network-config",
			Namespace: c.PodNamespace,
		},
		Data: map[string]string{
			"nameservers": "1.1.1.1 1.0.0.1",
		},
	}

	Expect(c.Client.Create(c, networkCM)).To(Succeed())
}

func (c *TestContextForVCSim) setupAZs(config VCSimTestConfig) {
	if !config.WithFaultDomains {
		return
	}

	ccrs, err := c.Finder.ClusterComputeResourceList(c, "*")
	Expect(err).ToNot(HaveOccurred())
	Expect(ccrs).To(HaveLen(c.ZoneCount * c.ClustersPerZone))
	c.azCCRs = map[string][]*object.ClusterComputeResource{}

	for i := 0; i < c.ZoneCount; i++ {
		idx := i * c.ClustersPerZone
		clusters := ccrs[idx : idx+c.ClustersPerZone]

		az := &topologyv1.AvailabilityZone{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("az-%d", i),
			},
		}
		for _, c := range clusters {
			az.Spec.ClusterComputeResourceMoIDs = append(az.Spec.ClusterComputeResourceMoIDs, c.Reference().Value)
		}

		Expect(c.Client.Create(c, az)).To(Succeed())
		c.ZoneNames = append(c.ZoneNames, az.Name)
		c.azCCRs[az.Name] = clusters
	}
}

func (c *TestContextForVCSim) GetSingleClusterCompute() *object.ClusterComputeResource {
	Expect(c.withFaultDomains).To(BeFalse())
	Expect(c.singleCCR).ToNot(BeNil())

	return c.singleCCR
}

func (c *TestContextForVCSim) GetAZClusterComputes(azName string) []*object.ClusterComputeResource {
	Expect(c.withFaultDomains).To(BeTrue())

	ccrs, ok := c.azCCRs[azName]
	Expect(ok).To(BeTrue())
	return ccrs
}

func (c *TestContextForVCSim) CreateVirtualMachineSetResourcePolicy(
	name string,
	nsInfo WorkloadNamespaceInfo) (*vmopv1alpha1.VirtualMachineSetResourcePolicy, *object.Folder) {

	resourcePolicy := DummyVirtualMachineSetResourcePolicy2(name, nsInfo.Namespace)
	Expect(c.Client.Create(c, resourcePolicy)).To(Succeed())

	var rps []*object.ResourcePool

	if c.withFaultDomains {
		for _, ccrs := range c.azCCRs {
			for _, ccr := range ccrs {
				rp, err := ccr.ResourcePool(c)
				Expect(err).ToNot(HaveOccurred())
				rps = append(rps, rp)
			}
		}
	} else {
		rp, err := c.singleCCR.ResourcePool(c)
		Expect(err).ToNot(HaveOccurred())
		rps = append(rps, rp)
	}

	si := object.NewSearchIndex(c.VCClient.Client)
	for _, rp := range rps {
		objRef, err := si.FindChild(c, rp.Reference(), nsInfo.Namespace)
		Expect(err).ToNot(HaveOccurred())
		Expect(objRef).ToNot(BeNil())
		nsRP, ok := objRef.(*object.ResourcePool)
		Expect(ok).To(BeTrue())

		_, err = nsRP.Create(c, resourcePolicy.Spec.ResourcePool.Name, types.DefaultResourceConfigSpec())
		Expect(err).ToNot(HaveOccurred())
	}

	folder, err := nsInfo.Folder.CreateFolder(c, resourcePolicy.Spec.Folder.Name)
	Expect(err).ToNot(HaveOccurred())

	return resourcePolicy, folder
}

func (c *TestContextForVCSim) GetVMFromMoID(moID string) *object.VirtualMachine {
	objRef, err := c.Finder.ObjectReference(c, types.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
	if err != nil {
		return nil
	}

	vm, ok := objRef.(*object.VirtualMachine)
	Expect(ok).To(BeTrue())
	return vm
}

func (c *TestContextForVCSim) GetResourcePoolForNamespace(namespace, azName, childName string) *object.ResourcePool {
	var ccr *object.ClusterComputeResource

	if c.withFaultDomains {
		Expect(azName).ToNot(BeEmpty())
		Expect(c.ClustersPerZone).To(Equal(1)) // TODO: Deal with Zones w/ multiple CCRs later

		ccrs := c.GetAZClusterComputes(azName)
		ccr = ccrs[0]
	} else {
		ccr = c.GetSingleClusterCompute()
	}

	rp, err := ccr.ResourcePool(c)
	Expect(err).ToNot(HaveOccurred())

	// Make trip through the Finder to populate InventoryPath.
	objRef, err := c.Finder.ObjectReference(c, rp.Reference())
	Expect(err).ToNot(HaveOccurred())
	rp, ok := objRef.(*object.ResourcePool)
	Expect(ok).To(BeTrue())

	nsRP, err := c.Finder.ResourcePool(c, path.Join(rp.InventoryPath, namespace, childName))
	Expect(err).ToNot(HaveOccurred())

	return nsRP
}

func generatePrivateKey() *rsa.PrivateKey {
	reader := rand.Reader
	bitSize := 2048

	// Based on https://golang.org/src/crypto/tls/generate_cert.go
	privateKey, err := rsa.GenerateKey(reader, bitSize)
	Expect(err).ToNot(HaveOccurred())
	return privateKey
}

func generateSelfSignedCert() (string, string) {
	priv := generatePrivateKey()
	now := time.Now()
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	Expect(err).NotTo(HaveOccurred())

	template := x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		SerialNumber:          serialNumber,
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	Expect(err).NotTo(HaveOccurred())
	certOut, err := ioutil.TempFile("", "cert.pem")
	Expect(err).NotTo(HaveOccurred())
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	Expect(err).NotTo(HaveOccurred())
	err = certOut.Close()
	Expect(err).NotTo(HaveOccurred())

	keyOut, err := ioutil.TempFile("", "key.pem")
	Expect(err).NotTo(HaveOccurred())
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	Expect(err).NotTo(HaveOccurred())
	err = pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	Expect(err).NotTo(HaveOccurred())
	err = keyOut.Close()
	Expect(err).NotTo(HaveOccurred())

	return keyOut.Name(), certOut.Name()
}
