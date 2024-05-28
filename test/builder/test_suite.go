// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/controllers/util/remote"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	pkgmgrinit "github.com/vmware-tanzu/vm-operator/pkg/manager/init"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

// Reconciler is a base type for builder's reconcilers.
type Reconciler interface{}

// NewReconcilerFunc is a base type for functions that return a reconciler.
type NewReconcilerFunc func() Reconciler

func init() {
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klog.Background())
}

// TestSuite is used for unit and integration testing builder. Each TestSuite
// contains one independent test environment and a controller manager.
type TestSuite struct {
	context.Context

	flags                 testFlags
	envTest               envtest.Environment
	config                *rest.Config
	integrationTestClient client.Client

	// Cancel function that will be called to close the Done channel of the
	// Context, which will then stop the manager.
	cancelFuncMutex sync.Mutex
	cancelFunc      context.CancelFunc

	// Controller specific fields
	addToManagerFn      pkgmgr.AddToManagerFunc
	initProvidersFn     pkgmgr.InitializeProvidersFunc
	integrationTest     bool
	newCacheFn          ctrlcache.NewCacheFunc
	manager             pkgmgr.Manager
	managerRunning      bool
	managerRunningMutex sync.Mutex

	// Webhook specific fields
	webhookName string
	certDir     string
	validatorFn builder.ValidatorFunc
	mutatorFn   builder.MutatorFunc
	pki         pkiToolchain
	webhookYaml []byte
}

func (s *TestSuite) isWebhookTest() bool {
	return s.webhookName != ""
}

func (s *TestSuite) GetManager() pkgmgr.Manager {
	return s.manager
}

func (s *TestSuite) SetManagerNewCacheFunc(f ctrlcache.NewCacheFunc) {
	s.newCacheFn = f
}

func (s *TestSuite) GetEnvTestConfig() *rest.Config {
	return s.config
}

func (s *TestSuite) GetLogger() logr.Logger {
	return logf.Log
}

// NewTestSuite returns a new test suite used for unit and/or integration test.
func NewTestSuite() *TestSuite {
	return NewTestSuiteForController(
		pkgmgr.AddToManagerNoopFn,
		pkgmgr.InitializeProvidersNoopFn,
	)
}

// NewTestSuiteWithContext returns a new test suite used for unit and/or
// integration test.
func NewTestSuiteWithContext(ctx context.Context) *TestSuite {
	return NewTestSuiteForControllerWithContext(
		ctx,
		pkgmgr.AddToManagerNoopFn,
		pkgmgr.InitializeProvidersNoopFn,
	)
}

// NewFunctionalTestSuite returns a new test suite used for functional tests.
// The functional test starts all the controllers, and creates all the providers
// so it is a more fully functioning env than an integration test with a single
// controller running.
func NewFunctionalTestSuite(addToManagerFunc func(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error) *TestSuite {
	return NewTestSuiteForController(
		addToManagerFunc,
		pkgmgrinit.InitializeProviders,
	)
}

// NewTestSuiteForController returns a new test suite used for controller integration test.
func NewTestSuiteForController(addToManagerFn pkgmgr.AddToManagerFunc, initProvidersFn pkgmgr.InitializeProvidersFunc) *TestSuite {
	return NewTestSuiteForControllerWithContext(pkgcfg.NewContext(), addToManagerFn, initProvidersFn)
}

// NewTestSuiteForControllerWithContext returns a new test suite used for
// controller integration test.
func NewTestSuiteForControllerWithContext(
	ctx context.Context,
	addToManagerFn pkgmgr.AddToManagerFunc,
	initProvidersFn pkgmgr.InitializeProvidersFunc) *TestSuite {

	if addToManagerFn == nil {
		panic("addToManagerFn is nil")
	}
	if initProvidersFn == nil {
		panic("initProvidersFn is nil")
	}

	testSuite := &TestSuite{
		Context:         ctx,
		integrationTest: true,
		addToManagerFn:  addToManagerFn,
		initProvidersFn: initProvidersFn,
	}
	testSuite.init()

	return testSuite
}

// NewTestSuiteForValidatingWebhook returns a new test suite used for unit and
// integration testing validating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForValidatingWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(pkgcfg.NewContext(), addToManagerFn, newValidatorFn, nil, webhookName)
}

// NewTestSuiteForValidatingWebhookWithContext returns a new test suite used for unit and
// integration testing validating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForValidatingWebhookWithContext(
	ctx context.Context,
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(ctx, addToManagerFn, newValidatorFn, nil, webhookName)
}

// NewTestSuiteForMutatingWebhook returns a new test suite used for unit and
// integration testing mutating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForMutatingWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newMutatorFn builder.MutatorFunc,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(pkgcfg.NewContext(), addToManagerFn, nil, newMutatorFn, webhookName)
}

// NewTestSuiteForMutatingWebhookWithContext returns a new test suite used for unit and
// integration testing mutating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForMutatingWebhookWithContext(
	ctx context.Context,
	addToManagerFn pkgmgr.AddToManagerFunc,
	newMutatorFn builder.MutatorFunc,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(ctx, addToManagerFn, nil, newMutatorFn, webhookName)
}

func newTestSuiteForWebhook(
	ctx context.Context,
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	newMutatorFn builder.MutatorFunc,
	webhookName string) *TestSuite {

	testSuite := &TestSuite{
		Context:         ctx,
		integrationTest: true,
		addToManagerFn:  addToManagerFn,
		initProvidersFn: pkgmgr.InitializeProvidersNoopFn,
		webhookName:     webhookName,
	}

	if newValidatorFn != nil {
		testSuite.validatorFn = newValidatorFn
	}
	if newMutatorFn != nil {
		testSuite.mutatorFn = newMutatorFn
	}

	// Create a temp directory for the certs needed for testing webhooks.
	certDir, err := os.MkdirTemp(os.TempDir(), "")
	if err != nil {
		panic(fmt.Errorf("failed to create temp dir for certs: %w", err))
	}
	testSuite.certDir = certDir

	testSuite.init()

	return testSuite
}

func (s *TestSuite) init() {
	// Initialize the test flags.
	s.flags = flags

	rootDir := testutil.GetRootDirOrDie()

	crds, err := LoadCRDs(filepath.Join(rootDir, "config", "crd", "bases"))
	if err != nil {
		panic(err)
	}

	if envTestsEnabled() {
		s.envTest = envtest.Environment{
			CRDs: s.applyFeatureStatesToCRDs(crds),
			CRDDirectoryPaths: []string{
				filepath.Join(rootDir, "config", "crd", "external-crds"),
			},
			BinaryAssetsDirectory: filepath.Join(testutil.GetRootDirOrDie(), "hack", "tools", "bin", goruntime.GOOS+"_"+goruntime.GOARCH),
		}
	}
}

// Register should be invoked by the function to which *testing.T is passed.
//
// Use runUnitTestsFn to pass a function that will be invoked if unit testing
// is enabled with Describe("Unit tests", runUnitTestsFn).
//
// Use runIntegrationTestsFn to pass a function that will be invoked if
// integration testing is enabled with
// Describe("Unit tests", runIntegrationTestsFn).
func (s *TestSuite) Register(t *testing.T, name string, runIntegrationTestsFn, runUnitTestsFn func()) {
	RegisterFailHandler(Fail)

	if runIntegrationTestsFn != nil && envTestsEnabled() {
		Describe("Integration tests", runIntegrationTestsFn)
		SetDefaultEventuallyTimeout(time.Second * 10)
		SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	}
	if runUnitTestsFn != nil {
		Describe("Unit tests", runUnitTestsFn)
	}

	RunSpecs(t, name)
}

// NewUnitTestContextForController returns a new unit test context for this
// suite's reconciler.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForController(initObjects ...client.Object) *UnitTestContextForController {
	return NewUnitTestContextForController(initObjects)
}

// NewUnitTestContextForValidatingWebhook returns a new unit test context for this
// suite's validator.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForValidatingWebhook(
	obj, oldObj *unstructured.Unstructured,
	initObjects ...client.Object) *UnitTestContextForValidatingWebhook {

	return NewUnitTestContextForValidatingWebhook(s.validatorFn, obj, oldObj, initObjects...)
}

// NewUnitTestContextForMutatingWebhook returns a new unit test context for this
// suite's mutator.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForMutatingWebhook(obj *unstructured.Unstructured) *UnitTestContextForMutatingWebhook {
	return NewUnitTestContextForMutatingWebhook(s.mutatorFn, obj)
}

// BeforeSuite should be invoked by ginkgo.BeforeSuite.
func (s *TestSuite) BeforeSuite() {
	if envTestsEnabled() {
		s.beforeSuiteForIntegrationTesting()
	}
}

// AfterSuite should be invoked by ginkgo.AfterSuite.
func (s *TestSuite) AfterSuite() {
	if envTestsEnabled() {
		s.afterSuiteForIntegrationTesting()
	}
}

// Create a new Manager with default values.
func (s *TestSuite) createManager() {
	var err error

	opts := pkgmgr.Options{
		KubeConfig:          s.config,
		MetricsAddr:         "0",
		AddToManager:        s.addToManagerFn,
		InitializeProviders: s.initProvidersFn,
		NewCache:            s.newCacheFn,
	}

	opts.Scheme = runtime.NewScheme()
	s.manager, err = pkgmgr.New(s, opts)

	Expect(err).NotTo(HaveOccurred())
	Expect(s.manager).ToNot(BeNil())
}

func (s *TestSuite) initializeManager() {
	// If one or more webhooks are being tested then go ahead and configure the webhook server.
	if s.isWebhookTest() {
		By("configuring webhook server", func() {
			svr := s.manager.GetWebhookServer().(*webhook.DefaultServer)
			svr.Options.Host = "127.0.0.1"
			svr.Options.Port = randomTCPPort()
			svr.Options.CertDir = s.certDir
		})
	}
}

// Set a flag to indicate that the manager is running or not.
func (s *TestSuite) setManagerRunning(isRunning bool) {
	s.managerRunningMutex.Lock()
	s.managerRunning = isRunning
	s.managerRunningMutex.Unlock()
}

// Returns true if the manager is running, false otherwise.
func (s *TestSuite) getManagerRunning() bool {
	s.managerRunningMutex.Lock()
	result := s.managerRunning
	s.managerRunningMutex.Unlock()
	return result
}

// Starts the manager and sets managerRunning.
func (s *TestSuite) startManager() {
	ctx, cancel := context.WithCancel(s.Context)
	s.cancelFuncMutex.Lock()
	s.cancelFunc = cancel
	s.cancelFuncMutex.Unlock()

	go func() {
		defer GinkgoRecover()

		s.setManagerRunning(true)
		Expect(s.manager.Start(ctx)).ToNot(HaveOccurred())
		s.setManagerRunning(false)
	}()
}

// Applies configuration to the Manager after it has started.
func (s *TestSuite) postConfigureManager() {
	// If there's a configured certificate directory then it means one or more
	// webhooks are being tested. Go ahead and install the webhooks and wait
	// for the webhook server to come online.
	if s.isWebhookTest() {
		By("installing the webhook(s)", func() {
			// ASSERT that the file for validating webhook file exists.
			validatingWebhookFile := path.Join(testutil.GetRootDirOrDie(), "config", "webhook", "manifests.yaml")
			Expect(validatingWebhookFile).Should(BeAnExistingFile())

			// UNMARSHAL the contents of the validating webhook file into MutatingWebhookConfiguration and
			// ValidatingWebhookConfiguration.
			mutatingWebhookConfig, validatingWebhookConfig := parseWebhookConfig(validatingWebhookFile)

			// MARSHAL the webhook config back to YAML.
			if s.mutatorFn != nil {
				By("installing the mutating webhook(s)")
				svr := s.manager.GetWebhookServer().(*webhook.DefaultServer)
				s.webhookYaml = updateMutatingWebhookConfig(mutatingWebhookConfig, s.webhookName, svr.Options.Host, svr.Options.Port, s.pki.publicKeyPEM)
			} else {
				By("installing the validating webhook(s)")
				svr := s.manager.GetWebhookServer().(*webhook.DefaultServer)
				s.webhookYaml = updateValidatingWebhookConfig(validatingWebhookConfig, s.webhookName, svr.Options.Host, svr.Options.Port, s.pki.publicKeyPEM)
			}

			// ASSERT that eventually the webhook config gets successfully
			// applied to the API server.
			Eventually(func() error {
				return remote.ApplyYAML(s, s.integrationTestClient, s.webhookYaml)
			}).Should(Succeed())
		})

		// It can take a few seconds for the webhook server to come online.
		// This step blocks until the webserver can be successfully accessed.
		By("waiting for the webhook server to come online", func() {
			svr := s.manager.GetWebhookServer().(*webhook.DefaultServer)
			addr := net.JoinHostPort(svr.Options.Host, strconv.Itoa(svr.Options.Port))
			dialer := &net.Dialer{Timeout: time.Second}
			//nolint:gosec
			tlsConfig := &tls.Config{InsecureSkipVerify: true}
			Eventually(func() error {
				conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
				if err != nil {
					return err
				}
				_ = conn.Close()
				return nil
			}).Should(Succeed())
		})
	}
}

func (s *TestSuite) beforeSuiteForIntegrationTesting() {
	var err error

	By("bootstrapping test environment", func() {
		s.config, err = s.envTest.Start()
		Expect(err).ToNot(HaveOccurred())
		Expect(s.config).ToNot(BeNil())
	})

	// If one or more webhooks are being tested then go ahead and generate a
	// PKI toolchain to use with the webhook server.
	if s.isWebhookTest() {
		By("generating the pki toolchain", func() {
			s.pki, err = generatePKIToolchain()
			Expect(err).ToNot(HaveOccurred())
			// Write the CA pub key and cert pub and private keys to the cert dir.
			Expect(os.WriteFile(path.Join(s.certDir, "tls.crt"), s.pki.publicKeyPEM, 0400)).To(Succeed())
			Expect(os.WriteFile(path.Join(s.certDir, "tls.key"), s.pki.privateKeyPEM, 0400)).To(Succeed())
		})
	}

	if s.integrationTest {
		By("setting up a new manager", func() {
			s.createManager()
			s.initializeManager()
		})

		s.integrationTestClient, err = client.New(s.manager.GetConfig(), client.Options{Scheme: s.manager.GetScheme()})
		Expect(err).NotTo(HaveOccurred())

		By("create pod namespace", func() {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: s.manager.GetContext().Namespace,
				},
			}
			Expect(s.integrationTestClient.Create(s, namespace)).To(Succeed())
		})

		By("create availability zone", func() {
			Expect(s.integrationTestClient.Create(s, DummyAvailabilityZone())).To(Succeed())
		})

		By("starting the manager", func() {
			s.startManager()
		})

		By("configuring the manager", func() {
			s.postConfigureManager()
		})
	}
}

func (s *TestSuite) afterSuiteForIntegrationTesting() {
	if s.integrationTest {
		By("tearing down the manager", func() {
			s.cancelFuncMutex.Lock()
			if s.cancelFunc != nil {
				s.cancelFunc()
			}
			s.cancelFuncMutex.Unlock()

			Eventually(s.getManagerRunning).Should(BeFalse())

			if s.webhookYaml != nil {
				Eventually(func() error {
					return remote.DeleteYAML(s, s.integrationTestClient, s.webhookYaml)
				}).Should(Succeed())
			}
		})
	}

	By("tearing down the test environment", func() {
		Expect(s.envTest.Stop()).To(Succeed())
	})
}

func (s *TestSuite) applyFeatureStatesToCRDs(
	in []*apiextensionsv1.CustomResourceDefinition) []*apiextensionsv1.CustomResourceDefinition {

	out := make([]*apiextensionsv1.CustomResourceDefinition, 0)
	for i := range in {
		crd := applyFeatureStateFnsToCRD(
			s,
			*in[i])
		out = append(out, &crd)
	}
	return out
}

func (s *TestSuite) GetInstalledCRD(crdName string) *apiextensionsv1.CustomResourceDefinition {
	for _, crd := range s.envTest.CRDs {
		if crd.Name == crdName {
			return crd
		}
	}

	return nil
}

func (s *TestSuite) UpdateCRDScope(oldCrd *apiextensionsv1.CustomResourceDefinition, newScope string) {
	// crd.spec.scope is immutable, uninstall first
	err := envtest.UninstallCRDs(s.envTest.Config, envtest.CRDInstallOptions{
		CRDs: []*apiextensionsv1.CustomResourceDefinition{oldCrd},
	})
	Expect(err).ShouldNot(HaveOccurred())

	crds := make([]*apiextensionsv1.CustomResourceDefinition, 0)
	crdName := oldCrd.Name
	Eventually(func() error {
		newCrd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:        crdName,
				Annotations: oldCrd.Annotations,
			},
			Spec: oldCrd.Spec,
		}
		newCrd.Spec.Scope = apiextensionsv1.ResourceScope(newScope)
		crds, err = envtest.InstallCRDs(s.envTest.Config, envtest.CRDInstallOptions{
			CRDs: []*apiextensionsv1.CustomResourceDefinition{newCrd},
		})
		return err
	}).ShouldNot(HaveOccurred())
	s.envTest.CRDs = append(s.envTest.CRDs, crds...)
}

func parseWebhookConfig(path string) (
	mutatingWebhookConfig admissionregv1.MutatingWebhookConfiguration,
	validatingWebhookConfig admissionregv1.ValidatingWebhookConfiguration) {

	// A recent update to controller-tools means manifests are no longer
	// generated with a leading newline character, so this check must now
	// simply be a ---\n.
	const separator = "---\n"

	// READ the validating webhook file.
	yamlIn, err := os.ReadFile(filepath.Clean(path))
	Expect(err).ShouldNot(HaveOccurred())
	Expect(yamlIn).ShouldNot(BeEmpty())

	// Assumes mutating and then validating are present.
	sep := len([]byte(separator))
	i := bytes.Index(yamlIn, []byte(separator))
	j := bytes.LastIndex(yamlIn, []byte(separator))
	mBytes := yamlIn[i+sep : j]
	vBytes := yamlIn[j+sep:]
	Expect(yaml.Unmarshal(mBytes, &mutatingWebhookConfig)).To(Succeed())
	Expect(yaml.Unmarshal(vBytes, &validatingWebhookConfig)).To(Succeed())
	return mutatingWebhookConfig, validatingWebhookConfig
}

func updateValidatingWebhookConfig(webhookConfig admissionregv1.ValidatingWebhookConfiguration, webhookName, host string, port int, key []byte) []byte {
	webhookConfigToInstall := webhookConfig.DeepCopy()
	// ITERATE over all of the defined webhooks and find the webhook
	// the test suite is testing and update its client config to point
	// to the test webhook server.
	//   1. Use the test CA
	//   2. Use the test webhook endpoint
	for _, webhook := range webhookConfig.Webhooks {
		if webhook.Name == webhookName {
			url := fmt.Sprintf("https://%s:%d%s", host, port, *webhook.ClientConfig.Service.Path)
			webhook.ClientConfig.CABundle = key
			webhook.ClientConfig.Service = nil
			webhook.ClientConfig.URL = &url
			webhookConfigToInstall.Webhooks = []admissionregv1.ValidatingWebhook{webhook}
		}
	}
	result, err := yaml.Marshal(webhookConfigToInstall)
	Expect(err).ShouldNot(HaveOccurred())
	return result
}

func updateMutatingWebhookConfig(webhookConfig admissionregv1.MutatingWebhookConfiguration, webhookName, host string, port int, key []byte) []byte {
	webhookConfigToInstall := webhookConfig.DeepCopy()
	// ITERATE over all of the defined webhooks and find the webhook
	// the test suite is testing and update its client config to point
	// to the test webhook server.
	//   1. Use the test CA
	//   2. Use the test webhook endpoint
	for _, webhook := range webhookConfig.Webhooks {
		if webhook.Name == webhookName {
			url := fmt.Sprintf("https://%s:%d%s", host, port, *webhook.ClientConfig.Service.Path)
			webhook.ClientConfig.CABundle = key
			webhook.ClientConfig.Service = nil
			webhook.ClientConfig.URL = &url
			webhookConfigToInstall.Webhooks = []admissionregv1.MutatingWebhook{webhook}
		}
	}
	result, err := yaml.Marshal(webhookConfigToInstall)
	Expect(err).ShouldNot(HaveOccurred())
	return result
}

func envTestsEnabled() bool {
	return Label(testlabels.EnvTest).MatchesLabelFilter(GinkgoLabelFilter())
}
