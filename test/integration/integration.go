// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"flag"
	"fmt"
	stdlog "log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/go-logr/zapr"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	govmomirest "github.com/vmware/govmomi/vapi/rest"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	vmoperator "github.com/vmware-tanzu/vm-operator"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
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
	vmProvider      vmprovider.VirtualMachineProviderInterface
	log             = logf.Log.WithName("integration")
)

func setContentSourceID(id string) {
	ContentSourceID = id
}

func GetContentSourceID() string {
	return ContentSourceID
}

func NewIntegrationVmOperatorConfig(vcAddress string, vcPort int, contentSource string) *vsphere.VSphereVmProviderConfig {
	var dcMoId string
	var rpMoId string
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

	return &vsphere.VSphereVmProviderConfig{
		VcPNID:                      vcAddress,
		VcPort:                      strconv.Itoa(vcPort),
		VcCreds:                     NewIntegrationVmOperatorCredentials(),
		Datacenter:                  dcMoId,
		ResourcePool:                rpMoId,
		Folder:                      "/DC0/vm",
		Datastore:                   "/DC0/datastore/LocalDS_0",
		ContentSource:               contentSource,
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

//nolint:golint,unused,deadcode
func EnableDebugLogging() {
	zapcfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.DebugLevel * 4),
		Development:      true,
		Encoding:         "console",
		EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
	zapLog, err := zapcfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		// Log but ignore failure to enable debug logging
		stdlog.Print(err)
	}
	logf.SetLogger(zapr.NewLogger(zapLog))
}

// Because of a hard coded path in buildAggregatedAPIServer, all integration tests that are using
// suite.InstallLocalTestingAPIAggregationEnvironment() must be at this level of the directory hierarchy.
// If you are calling this setup function, ensure that the tests are at the same "level" of the directory hierarchy
// as the other integration tests.
func SetupIntegrationEnv(namespaces []string) (*suite.Environment, *vsphere.VSphereVmProviderConfig, *rest.Config, *VcSimInstance, *vsphere.Session) {
	// Enable this function call in order to see more verbose logging as part of these integration tests
	EnableDebugLogging()

	stdlog.Print("setting up the local aggregated-apiserver for test env...")
	testEnv, err := suite.InstallLocalTestingAPIAggregationEnvironment("vmoperator.vmware.com", "v1alpha1")
	Expect(err).NotTo(HaveOccurred())

	flag.Parse()

	_, err = envtest.InstallCRDs(testEnv.KubeAPIServerEnvironment.Config, envtest.CRDInstallOptions{
		Paths: []string{
			filepath.Join(vmoperator.Rootpath, "config", "crds", "external-crds")},
	})
	Expect(err).NotTo(HaveOccurred())

	cfg := testEnv.LoopbackClientConfig

	stdlog.Print("setting up the integration test env..")
	err = apis.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = cnsv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	vmProvider, err = vsphere.RegisterVsphereVmProvider(cfg)
	Expect(err).NotTo(HaveOccurred())

	vcSim := NewVcSimInstance()

	address, port := vcSim.Start()
	vSphereConfig := NewIntegrationVmOperatorConfig(address, port, "")

	session, err := SetupVcsimEnv(vSphereConfig, cfg, vcSim, namespaces)
	Expect(err).NotTo(HaveOccurred())
	Expect(vSphereConfig).ShouldNot(Equal(nil))

	err = os.Setenv(vsphere.EnvContentLibApiWaitSecs, "1")
	Expect(err).NotTo(HaveOccurred())

	return testEnv, vSphereConfig, cfg, vcSim, session
}

func TeardownIntegrationEnv(testEnv *suite.Environment, vcSim *VcSimInstance) {
	TeardownVcsimEnv(vcSim)

	stdlog.Print("stopping the aggregated-apiserver..")
	err := testEnv.StopAggregatedAPIServer()
	Expect(err).NotTo(HaveOccurred())

	stdlog.Print("stopping the kube-apiserver..")
	err = testEnv.KubeAPIServerEnvironment.Stop()
	Expect(err).NotTo(HaveOccurred())
}

func SetupVcsimEnv(vSphereConfig *vsphere.VSphereVmProviderConfig, cfg *rest.Config, vcSim *VcSimInstance, namespaces []string) (*vsphere.Session, error) {

	// Support for bootstrapping VM operator resource requirements in Kubernetes.
	// Generate a fake vsphere provider config that is suitable for the integration test environment.
	// Post the resultant config map to the API Master for consumption by the VM operator
	log.Info("Installing a bootstrap config map for use in integration tests.")

	// Configure the environment with the location of the vmop config.
	err := lib.SetVmOpNamespaceEnv(DefaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to install vm operator config: %v", err)
	}

	err = vsphere.InstallVSphereVmProviderConfig(kubernetes.NewForConfigOrDie(cfg), DefaultNamespace, vSphereConfig, SecretName)
	if err != nil {
		return nil, fmt.Errorf("failed to install vm operator config: %v", err)
	}

	vsphereVmProvider := vmProvider.(*vsphere.VSphereVmProvider)

	// Setup content library once.  The first namespace is sufficient to use
	session, err := vsphereVmProvider.GetSession(context.TODO(), namespaces[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %v", err)
	}

	err = setupVcSimContent(vSphereConfig, vcSim, session)
	if err != nil {
		return nil, fmt.Errorf("failed to setup content library: %v", err)
	}

	// Configure each requested namespace to use CL as the content source
	for _, ns := range namespaces {
		err = vsphere.InstallVSphereVmProviderConfig(kubernetes.NewForConfigOrDie(cfg),
			ns,
			NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, GetContentSourceID()),
			SecretName)
		Expect(err).NotTo(HaveOccurred())
	}

	// return the last session for use
	return session, nil
}

func TeardownVcsimEnv(vcSim *VcSimInstance) {
	if vcSim != nil {
		vcSim.Stop()
	}

	if vmProvider != nil {
		vmprovider.UnregisterVmProviderOrDie(vmProvider)
	}
}

func setupVcSimContent(config *vsphere.VSphereVmProviderConfig, vcSim *VcSimInstance, session *vsphere.Session) (err error) {
	ctx := context.TODO()

	c, err := vcSim.NewClient(ctx)
	if err != nil {
		stdlog.Printf("Failed to create client %v", err)
		return
	}

	rClient := govmomirest.NewClient(c.Client)
	userInfo := url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)

	err = rClient.Login(ctx, userInfo)
	if err != nil {
		stdlog.Printf("Failed to login %v", err)
		return
	}
	defer func() {
		err = rClient.Logout(ctx)
	}()

	return SetupContentLibrary(ctx, config, session)
}

func CreateLibraryItem(ctx context.Context, session *vsphere.Session, name, kind, libraryId string) error {
	// The 'Rootpath'/images directory is created and populated with ova content for CL related integration tests
	// and cleaned up right after.
	imagesDir := "images/"
	ovf := "ttylinux-pc_i486-16.1.ovf"

	imagePath := vmoperator.Rootpath + "/" + imagesDir + ovf

	libraryItem := library.Item{
		Name:      name,
		Type:      kind,
		LibraryID: libraryId,
	}

	return session.CreateLibraryItem(ctx, libraryItem, imagePath)
}

func SetupContentLibrary(ctx context.Context, config *vsphere.VSphereVmProviderConfig, session *vsphere.Session) error {
	stdlog.Printf("Setting up Content Library: %+v\n", *config)

	libID, err := session.CreateLibrary(ctx, ContentSourceName)
	if err != nil {
		return err
	}

	// Assign ContentSourceID to be used for integration tests
	setContentSourceID(libID)
	config.ContentSource = libID

	err = CreateLibraryItem(ctx, session, IntegrationContentLibraryItemName, "ovf", libID)
	if err != nil {
		return err
	}

	return nil
}

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished. Additionally, it also writes any error from the
// reconcile function.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request, chan error) {
	requests := make(chan reconcile.Request)
	// Make errors a buffered channel so we don't block even if there are no interested receivers. This is needed for
	// the sake of tests that don't want to make any assertions on the errors. The buffer size should be more than
	// sufficient given the channels are setup for each test.
	errors := make(chan error, 100)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		errors <- err
		return result, err
	})
	return fn, requests, errors
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		Expect(mgr.Start(stop)).To(Succeed())
		wg.Done()
	}()
	cache := mgr.GetCache()
	result := cache.WaitForCacheSync(stop)
	Expect(result).Should(BeTrue(), "Cache should have synced")
	return stop, wg
}
