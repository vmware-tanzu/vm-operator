// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"path"
	"path/filepath"
	"strconv"

	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/vcenter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

type VSphereVmProviderTestConfig struct {
	VcCredsSecretName string
	*vsphere.VSphereVmProviderConfig
}

const (
	IntegrationContentLibraryItemName = "test-item"
	DefaultNamespace                  = "default"
	SecretName                        = "wcp-vmop-sa-vc-auth" // nolint:gosec
	ContentSourceName                 = "vmop-test-integration-cl"
)

var (
	ContentSourceID string
	log             = logf.Log.WithName("integration")
	Log             = log
	vmProvider      vmprovider.VirtualMachineProviderInterface
)

func setContentSourceID(id string) {
	ContentSourceID = id
}

func GetContentSourceID() string {
	return ContentSourceID
}

func NewIntegrationVmOperatorConfig(vcAddress string, vcPort int) *vsphere.VSphereVmProviderConfig {
	var dcMoId, rpMoId, folderMoId string
	for _, dc := range simulator.Map.All("Datacenter") {
		if dc.Entity().Name == "DC0" {
			dcMoId = dc.Reference().Value
			break
		}
	}
	for _, cl := range simulator.Map.All("ClusterComputeResource") {
		if cl.Entity().Name == "DC0_C0" {
			rpMoId = cl.(*simulator.ClusterComputeResource).ResourcePool.Reference().Value
			break
		}
	}
	for _, folder := range simulator.Map.All("Folder") {
		if folder.Entity().Name == "vm" {
			folderMoId = folder.Reference().Value
			break
		}
	}

	return &vsphere.VSphereVmProviderConfig{
		VcPNID:                      vcAddress,
		VcPort:                      strconv.Itoa(vcPort),
		VcCreds:                     NewIntegrationVmOperatorCredentials(),
		Datacenter:                  dcMoId,
		ResourcePool:                rpMoId,
		Datastore:                   "/DC0/datastore/LocalDS_0",
		Folder:                      folderMoId,
		UseInventoryAsContentSource: true,
		InsecureSkipTLSVerify:       true,
	}
}

func NewIntegrationVmOperatorCredentials() *vsphere.VSphereVmProviderCredentials {
	// User and password can be anything for vcSim
	return &vsphere.VSphereVmProviderCredentials{
		Username: "Administrator@vsphere.local",
		Password: "Admin!23",
	}
}

func enableDebugLogging() {
	strVal, ok := os.LookupEnv("ENABLE_DEBUG_MODE")
	if ok {
		stdlog.Println("Debug logging is enabled")
		klog.InitFlags(nil)
		dbgEnabled, err := strconv.ParseBool(strVal)
		if err != nil {
			stdlog.Fatalf("Failed to print ENABLE_DEBUG_MODE env variable '%s': %v", strVal, err)
		}
		if dbgEnabled {
			if err := flag.Set("alsologtostderr", "true"); err != nil {
				stdlog.Fatalf("failed to set klog logtostderr flag: %v", err)
			}
			if err := flag.Set("v", "4"); err != nil {
				stdlog.Fatalf("failed to set klog level flag: %v", err)
			}
			flag.Parse()
			logf.Log.Fulfill(klogr.New())
			return
		}
	}
	stdlog.Println("Debug logging is disabled")
	flag.Parse()
}

// GetCtrlRuntimeClient gets a vm-operator-api client
// This is separate from NewVMService so that a fake client can be injected for testing
// This should really take the Scheme as a param.
func GetCtrlRuntimeClient(config *rest.Config) (client.Client, error) {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = vmopv1alpha1.AddToScheme(s)
	_ = ncpv1alpha1.AddToScheme(s)
	_ = netopv1alpha1.AddToScheme(s)
	_ = cnsv1alpha1.SchemeBuilder.AddToScheme(s)
	controllerClient, err := client.New(config, client.Options{
		Scheme: s,
	})
	return controllerClient, err
}

func SetupIntegrationEnv(namespaces []string) (*envtest.Environment, *vsphere.VSphereVmProviderConfig, *rest.Config, *VcSimInstance, *vsphere.Session, vmprovider.VirtualMachineProviderInterface) {
	Expect(len(namespaces) > 0).To(BeTrue())
	enableDebugLogging()
	rootDir, err := testutil.GetRootDir()
	Expect(err).ToNot(HaveOccurred())

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(rootDir, "config", "crd", "bases"),
			filepath.Join(rootDir, "config", "crd", "external-crds"),
		},
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	stdlog.Print("setting up the integration test env...")
	// BMV: We should not use the global Scheme here. Need to plumb this down to the controller Manager.
	err = vmopv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = ncpv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = netopv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = cnsv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	ncpClient := ncpclientset.NewForConfigOrDie(cfg)
	k8sClient, err := GetCtrlRuntimeClient(cfg)
	Expect(err).NotTo(HaveOccurred())

	// Register the vSphere provider
	log.Info("setting up vSphere Provider")
	vmProvider = vsphere.NewVSphereVmProviderFromClients(ncpClient, k8sClient, scheme.Scheme)

	vcSim := NewVcSimInstance()

	address, port := vcSim.Start()
	vSphereConfig := NewIntegrationVmOperatorConfig(address, port)
	Expect(vSphereConfig).ToNot(BeNil())

	session, err := SetupVcSimEnv(vSphereConfig, k8sClient, vcSim, namespaces)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv(vsphere.EnvContentLibApiWaitSecs, "1")
	Expect(err).NotTo(HaveOccurred())

	return testEnv, vSphereConfig, cfg, vcSim, session, vmProvider
}

func TeardownIntegrationEnv(testEnv *envtest.Environment, vcSim *VcSimInstance) {
	TeardownVcSimEnv(vcSim)

	if testEnv != nil {
		stdlog.Print("stopping the test environment...")
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
}

func SetupVcSimEnv(vSphereConfig *vsphere.VSphereVmProviderConfig, client client.Client, vcSim *VcSimInstance, namespaces []string) (*vsphere.Session, error) {

	// Support for bootstrapping VM operator resource requirements in Kubernetes.
	// Generate a fake vsphere provider config that is suitable for the integration test environment.
	// Post the resultant config map to the API Master for consumption by the VM operator
	log.Info("Installing a bootstrap config map for use in integration tests.")

	// Configure the environment with the location of the vmop config.
	err := lib.SetVmOpNamespaceEnv(DefaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to install vm operator config: %v", err)
	}

	// Support for bootstrapping VM operator resource requirements in Kubernetes.
	// Generate a fake vsphere provider config that is suitable for the integration test environment.
	// Post the resultant config map to the API Master for consumption by the VM operator
	klog.Infof("Installing a bootstrap config map for use in integration tests.")
	err = vsphere.InstallVSphereVmProviderConfig(client, DefaultNamespace, vSphereConfig, SecretName)
	if err != nil {
		return nil, fmt.Errorf("failed to install vm operator config: %v", err)
	}

	// Setup content library once.  The first namespace is sufficient to use
	session, err := vmProvider.(vsphere.VSphereVmProviderGetSessionHack).GetSession(context.TODO(), namespaces[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %v", err)
	}

	if err := SetupContentLibrary(client, session); err != nil {
		return nil, fmt.Errorf("failed to setup the VC Simulator: %v", err)
	}

	// Configure each requested namespace to use CL as the content source
	for _, ns := range namespaces {
		err = vsphere.InstallVSphereVmProviderConfig(client,
			ns,
			NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port),
			SecretName,
		)
		Expect(err).NotTo(HaveOccurred())
	}

	return session, nil
}

func TeardownVcSimEnv(vcSim *VcSimInstance) {
	if vcSim != nil {
		vcSim.Stop()
	}
}

func CreateLibraryItem(ctx context.Context, session *vsphere.Session, name, kind, libraryId string) error {
	ovf := "ttylinux-pc_i486-16.1.ovf"
	imagePath := path.Join(testutil.GetRootDirOrDie(), "images", ovf)

	libraryItem := library.Item{
		Name:      name,
		Type:      kind,
		LibraryID: libraryId,
	}

	return session.CreateLibraryItem(ctx, libraryItem, imagePath)
}

// SetupContentLibrary creates ContentSource and CotentLibraryProvider resources for the vSphere content library.
func SetupContentLibrary(client client.Client, session *vsphere.Session) error {
	stdlog.Printf("Setting up ContentLibraryPrvider and ContentSource for integration tests")

	ctx := context.Background()

	libID, err := session.CreateLibrary(ctx, ContentSourceName)
	if err != nil {
		return err
	}

	if err := CreateLibraryItem(ctx, session, IntegrationContentLibraryItemName, "ovf", libID); err != nil {
		return err
	}

	// Assign ContentSourceID to be used for integration tests
	setContentSourceID(libID)

	clProvider := &vmopv1alpha1.ContentLibraryProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: libID,
		},
		Spec: vmopv1alpha1.ContentLibraryProviderSpec{
			UUID: libID,
		},
	}

	cs := &vmopv1alpha1.ContentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: libID,
		},
		Spec: vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				Name: clProvider.ObjectMeta.Name,
				Kind: "ContentLibraryProvider",
			},
		},
	}

	// Create ContentSource and ContentLibraryProvider resources for the content library.
	if err := client.Create(ctx, clProvider); err != nil {
		return err
	}

	if err := client.Create(ctx, cs); err != nil {
		return err
	}

	return nil
}

func CloneVirtualMachineToLibraryItem(ctx context.Context, config *vsphere.VSphereVmProviderConfig, s *vsphere.Session, src, name string) error {
	vm, err := s.Finder.VirtualMachine(ctx, src)
	if err != nil {
		return err
	}

	pool, err := vm.ResourcePool(ctx)
	if err != nil {
		return err
	}

	restClient := s.Client.RestClient()

	spec := vcenter.Template{
		Name:     name,
		Library:  GetContentSourceID(),
		SourceVM: vm.Reference().Value,
		Placement: &vcenter.Placement{
			Folder:       config.Folder,
			ResourcePool: pool.Reference().Value,
		},
	}

	id, err := vcenter.NewManager(restClient).CreateTemplate(ctx, spec)
	if err != nil {
		return err
	}
	stdlog.Printf("Created vmtx %s in library %s", id, GetContentSourceID())

	return nil
}
