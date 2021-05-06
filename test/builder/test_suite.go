// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"

	admissionregv1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/controllers/util/remote"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	ctrlCtx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

// Reconciler is a base type for builder's reconcilers
type Reconciler interface{}

// NewReconcilerFunc is a base type for functions that return a reconciler
type NewReconcilerFunc func() Reconciler

func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klogr.New())
}

// TestSuite is used for unit and integration testing builder. Each TestSuite
// contains one independent test environment and a controller manager
type TestSuite struct {
	context.Context

	flags                 testFlags
	envTest               envtest.Environment
	config                *rest.Config
	integrationTestClient client.Client

	// Controller specific fields
	addToManagerFn      pkgmgr.AddToManagerFunc
	initProvidersFn     pkgmgr.InitializeProvidersFunc
	integrationTest     bool
	manager             pkgmgr.Manager
	managerDone         chan struct{}
	managerRunning      bool
	managerRunningMutex sync.Mutex

	// Webhook specific fields
	webhookName string
	certDir     string
	validatorFn builder.ValidatorFunc
	mutator     builder.Mutator
	pki         pkiToolchain
	webhookYaml []byte
}

func (s *TestSuite) isWebhookTest() bool {
	return s.webhookName != ""
}

func (s *TestSuite) GetEnvTestConfig() *rest.Config {
	return s.config
}

func getCrdPaths(crdPaths ...string) []string {
	rootDir := testutil.GetRootDirOrDie()

	return append(crdPaths,
		filepath.Join(rootDir, "config", "crd", "bases"),
		filepath.Join(rootDir, "config", "crd", "external-crds"),
	)
}

// NewTestSuite returns a new test suite used for unit and/or integration test
func NewTestSuite() *TestSuite {
	return NewTestSuiteForController(
		pkgmgr.AddToManagerNoopFn,
		pkgmgr.InitializeProvidersNoopFn,
	)
}

// NewFunctionalTestSuite returns a new test suite used for functional tests.
// The functional test starts all the controllers, and creates all the providers
// so it is a more fully functioning env than an integration test with a single
// controller running.
func NewFunctionalTestSuite(addToManagerFunc func(ctx *ctrlCtx.ControllerManagerContext, mgr manager.Manager) error) *TestSuite {
	return NewTestSuiteForController(
		addToManagerFunc,
		pkgmgr.InitializeProviders,
	)
}

// NewTestSuiteForController returns a new test suite used for controller integration test
func NewTestSuiteForController(addToManagerFn pkgmgr.AddToManagerFunc, initProvidersFn pkgmgr.InitializeProvidersFunc) *TestSuite {

	if addToManagerFn == nil {
		panic("addToManagerFn is nil")
	}
	if initProvidersFn == nil {
		panic("initProvidersFn is nil")
	}

	testSuite := &TestSuite{
		Context:         context.Background(),
		integrationTest: true,
		addToManagerFn:  addToManagerFn,
		initProvidersFn: initProvidersFn,
	}
	testSuite.init(getCrdPaths())

	return testSuite
}

// NewTestSuiteForValidatingWebhook returns a new test suite used for unit and
// integration testing validating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForValidatingWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(addToManagerFn, newValidatorFn, nil, webhookName)
}

// NewTestSuiteForMutatingWebhook returns a new test suite used for unit and
// integration testing mutating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForMutatingWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newMutatorFn func() builder.Mutator,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(addToManagerFn, nil, newMutatorFn, webhookName)
}

func newTestSuiteForWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	newMutatorFn func() builder.Mutator,
	webhookName string) *TestSuite {

	testSuite := &TestSuite{
		Context:         context.Background(),
		integrationTest: true,
		addToManagerFn:  addToManagerFn,
		initProvidersFn: pkgmgr.InitializeProvidersNoopFn,
		webhookName:     webhookName,
	}

	if newValidatorFn != nil {
		testSuite.validatorFn = newValidatorFn
	}
	if newMutatorFn != nil {
		testSuite.mutator = newMutatorFn()
	}

	// Create a temp directory for the certs needed for testing webhooks.
	certDir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		panic(errors.Wrap(err, "failed to create temp dir for certs"))
	}
	testSuite.certDir = certDir

	testSuite.init(
		getCrdPaths(),
		"--cert-dir="+certDir)

	return testSuite
}

func (s *TestSuite) init(crdPaths []string, additionalAPIServerFlags ...string) {
	// Initialize the test flags.
	s.flags = flags

	if s.flags.IntegrationTestsEnabled {
		apiServerFlags := append([]string{"--allow-privileged=true"}, envtest.DefaultKubeAPIServerFlags...)
		if len(additionalAPIServerFlags) > 0 {
			apiServerFlags = append(apiServerFlags, additionalAPIServerFlags...)
		}

		s.envTest = envtest.Environment{
			CRDDirectoryPaths:  crdPaths,
			KubeAPIServerFlags: apiServerFlags,
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

	if runIntegrationTestsFn == nil {
		s.flags.IntegrationTestsEnabled = false
	}
	if runUnitTestsFn == nil {
		s.flags.UnitTestsEnabled = false
	}

	if s.flags.IntegrationTestsEnabled {
		Describe("Integration tests", runIntegrationTestsFn)
	}
	if s.flags.UnitTestsEnabled {
		Describe("Unit tests", runUnitTestsFn)
	}

	if s.flags.IntegrationTestsEnabled {
		SetDefaultEventuallyTimeout(time.Second * 10)
		SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
		RunSpecsWithDefaultAndCustomReporters(t, name, []Reporter{printer.NewlineReporter{}})
	} else if s.flags.UnitTestsEnabled {
		RunSpecs(t, name)
	}
}

// NewUnitTestContextForController returns a new unit test context for this
// suite's reconciler.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForController(initObjects ...runtime.Object) *UnitTestContextForController {
	if s.flags.UnitTestsEnabled {
		ctx := NewUnitTestContextForController(initObjects)
		return ctx
	}
	return nil
}

// NewUnitTestContextForValidatingWebhook returns a new unit test context for this
// suite's validator.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForValidatingWebhook(
	obj, oldObj *unstructured.Unstructured,
	initObjects ...runtime.Object) *UnitTestContextForValidatingWebhook {

	if s.flags.UnitTestsEnabled {
		ctx := NewUnitTestContextForValidatingWebhook(s.validatorFn, obj, oldObj, initObjects...)
		return ctx
	}
	return nil
}

// NewUnitTestContextForMutatingWebhook returns a new unit test context for this
// suite's mutator.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForMutatingWebhook(obj *unstructured.Unstructured) *UnitTestContextForMutatingWebhook {
	if s.flags.UnitTestsEnabled {
		ctx := NewUnitTestContextForMutatingWebhook(s.mutator, obj)
		return ctx
	}
	return nil
}

// BeforeSuite should be invoked by ginkgo.BeforeSuite.
func (s *TestSuite) BeforeSuite() {
	if s.flags.IntegrationTestsEnabled {
		s.beforeSuiteForIntegrationTesting()
	}
}

// AfterSuite should be invoked by ginkgo.AfterSuite.
func (s *TestSuite) AfterSuite() {
	if s.flags.IntegrationTestsEnabled {
		s.afterSuiteForIntegrationTesting()
	}
}

// Create a new Manager with default values
func (s *TestSuite) createManager() {
	var err error

	s.managerDone = make(chan struct{})
	s.manager, err = pkgmgr.New(pkgmgr.Options{
		KubeConfig:          s.config,
		MetricsAddr:         "0",
		AddToManager:        s.addToManagerFn,
		InitializeProviders: s.initProvidersFn,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(s.manager).ToNot(BeNil())
}

func (s *TestSuite) initializeManager() {
	// If one or more webhooks are being tested then go ahead and configure the webhook server.
	if s.isWebhookTest() {
		By("configuring webhook server", func() {
			s.manager.GetWebhookServer().Host = "127.0.0.1"
			s.manager.GetWebhookServer().Port = randomTCPPort()
			s.manager.GetWebhookServer().CertDir = s.certDir
		})
	}
}

// Set a flag to indicate that the manager is running or not
func (s *TestSuite) setManagerRunning(isRunning bool) {
	s.managerRunningMutex.Lock()
	s.managerRunning = isRunning
	s.managerRunningMutex.Unlock()
}

// Returns true if the manager is running, false otherwise
func (s *TestSuite) getManagerRunning() bool {
	s.managerRunningMutex.Lock()
	result := s.managerRunning
	s.managerRunningMutex.Unlock()
	return result
}

// Starts the manager and sets managerRunning
func (s *TestSuite) startManager() {
	go func() {
		defer GinkgoRecover()

		s.setManagerRunning(true)
		Expect(s.manager.Start(s.managerDone)).ToNot(HaveOccurred())
		s.setManagerRunning(false)
	}()
}

// Applies configuration to the Manager after it has started
func (s *TestSuite) postConfigureManager() {
	// If there's a configured certificate directory then it means one or more
	// webhooks are being tested. Go ahead and install the webhooks and wait
	// for the webhook server to come online.
	if s.isWebhookTest() {
		By("installing the webhook(s)", func() {
			// ASSERT that the file for validating webhook file exists.
			validatingWebhookFile := path.Join(testutil.GetRootDirOrDie(), "config", "webhook", "manifests.v1beta1.yaml")
			Expect(validatingWebhookFile).Should(BeAnExistingFile())

			// UNMARSHAL the contents of the validating webhook file into MutatingWebhookConfiguration and
			// ValidatingWebhookConfiguration.
			mutatingWebhookConfig, validatingWebhookConfig := parseWebhookConfig(validatingWebhookFile)

			// MARSHAL the webhook config back to YAML.
			if s.mutator != nil {
				By("installing the mutating webhook(s)")
				s.webhookYaml = updateMutatingWebhookConfig(mutatingWebhookConfig, s.webhookName, s.manager.GetWebhookServer().Host, s.manager.GetWebhookServer().Port, s.pki.publicKeyPEM)
			} else {
				By("installing the validating webhook(s)")
				s.webhookYaml = updateValidatingWebhookConfig(validatingWebhookConfig, s.webhookName, s.manager.GetWebhookServer().Host, s.manager.GetWebhookServer().Port, s.pki.publicKeyPEM)
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
			addr := net.JoinHostPort(s.manager.GetWebhookServer().Host, strconv.Itoa(s.manager.GetWebhookServer().Port))
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
			Expect(ioutil.WriteFile(path.Join(s.certDir, "tls.crt"), s.pki.publicKeyPEM, 0400)).To(Succeed())
			Expect(ioutil.WriteFile(path.Join(s.certDir, "tls.key"), s.pki.privateKeyPEM, 0400)).To(Succeed())
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
			close(s.managerDone)
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

func parseWebhookConfig(path string) (
	mutatingWebhookConfig admissionregv1.MutatingWebhookConfiguration,
	validatingWebhookConfig admissionregv1.ValidatingWebhookConfiguration) {

	const separator = "\n---\n"

	// READ the validating webhook file.
	yamlIn, err := ioutil.ReadFile(path)
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
