// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
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
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	//nolint
	. "github.com/onsi/ginkgo"
	//nolint
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	admissionregv1 "k8s.io/api/admissionregistration/v1beta1"

	//apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	//"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/util/remote"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
	//"gitlab.eng.vmware.com/core-build/guest-cluster-controller/webhooks/capi"
)

var (
	separator = "\n---\n"
)

func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klogr.New())
}

// TestSuite is used for unit and integration testing builder.
type TestSuite struct {
	context.Context
	addToManagerFn        manager.AddToManagerFunc
	rootDir               string
	certDir               string
	integrationTestClient client.Client
	config                *rest.Config
	done                  chan struct{}
	envTest               envtest.Environment
	flags                 TestFlags
	manager               manager.Manager
	pki                   pkiToolchain
	validator             builder.Validator
	mutator               builder.Mutator
	newReconcilerFn       builder.NewReconcilerFunc
	webhookName           string
	managerRunning        bool
	managerRunningMutex   sync.Mutex
	webhookYaml           []byte
	//capiWebhookValidator  capi.Validator
}

func (s *TestSuite) isWebhookTest() bool {
	return s.webhookName != ""
}

func (s *TestSuite) GetEnvTestConfig() *rest.Config {
	return s.config
}

// NewTestSuiteForController returns a new test suite used for unit and
// integration testing controllers created using the "pkg/builder"
// package.
func NewTestSuiteForController(
	addToManagerFn manager.AddToManagerFunc,
	newReconcilerFn builder.NewReconcilerFunc) *TestSuite {

	testSuite := &TestSuite{
		Context: context.Background(),
	}
	testSuite.init(addToManagerFn, newReconcilerFn)

	if testSuite.flags.UnitTestsEnabled {
		if newReconcilerFn == nil {
			panic("newReconcilerFn is nil")
		}
	}

	return testSuite
}

// NewTestSuiteForPackage returns a new test suite for envtest-based
// integration testing of packages that aren't controllers or webhooks.
func NewTestSuiteForPackage(
	addToManagerFn manager.AddToManagerFunc) *TestSuite {

	testSuite := &TestSuite{
		Context: context.Background(),
	}
	testSuite.init(addToManagerFn, nil)

	return testSuite
}

// NewTestSuiteForVirtualMachineWebhook returns a new test suite used for unit and
// integration testing validating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForVirtualMachineWebhook(
	addToManagerFn manager.AddToManagerFunc,
	//newValidatorFn func() capi.Validator,
	webhookName string) *TestSuite {

	testSuite := &TestSuite{
		Context: context.Background(),
		//capiWebhookValidator: newValidatorFn(),
		webhookName: webhookName,
	}

	// Create a temp directory for the certs needed for testing webhooks.
	certDir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		panic(errors.Wrap(err, "failed to create temp dir for certs"))
	}
	testSuite.certDir = certDir

	// Init the test suite with the flags to enable the validating admission
	// webhooks as well as a custom directory for where certificates are
	// located.
	testSuite.init(addToManagerFn,
		nil,
		"--admission-control=ValidatingAdmissionWebhook",
		"--cert-dir="+certDir)

	return testSuite
}

// NewTestSuiteForValidatingWebhook returns a new test suite used for unit and
// integration testing validating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForValidatingWebhook(
	addToManagerFn manager.AddToManagerFunc,
	newValidatorFn func() builder.Validator,
	webhookName string) *TestSuite {

	testSuite := &TestSuite{
		Context:     context.Background(),
		validator:   newValidatorFn(),
		webhookName: webhookName,
	}

	// Create a temp directory for the certs needed for testing webhooks.
	certDir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		panic(errors.Wrap(err, "failed to create temp dir for certs"))
	}
	testSuite.certDir = certDir

	// Init the test suite with the flags to enable the validating admission
	// webhooks as well as a custom directory for where certificates are
	// located.
	testSuite.init(addToManagerFn,
		nil,
		"--admission-control=ValidatingAdmissionWebhook",
		"--cert-dir="+certDir)

	return testSuite
}

// NewTestSuiteForMutatingWebhook returns a new test suite used for unit and
// integration testing mutating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForMutatingWebhook(
	addToManagerFn manager.AddToManagerFunc,
	newMutatorFn func() builder.Mutator,
	webhookName string) *TestSuite {

	testSuite := &TestSuite{
		Context:     context.Background(),
		mutator:     newMutatorFn(),
		webhookName: webhookName,
	}

	// Create a temp directory for the certs needed for testing webhooks.
	certDir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		panic(errors.Wrap(err, "failed to create temp dir for certs"))
	}
	testSuite.certDir = certDir

	// Init the test suite with the flags to enable the mutating admission
	// webhooks as well as a custom directory for where certificates are
	// located.
	testSuite.init(addToManagerFn,
		nil,
		"--admission-control=MutatingAdmissionWebhook",
		"--cert-dir="+certDir)

	return testSuite
}

func (s *TestSuite) init(addToManagerFn manager.AddToManagerFunc, newReconcilerFn builder.NewReconcilerFunc, additionalAPIServerFlags ...string) {
	var err error

	s.flags = GetTestFlags()
	s.newReconcilerFn = newReconcilerFn
	s.rootDir, err = testutil.GetRootDir()
	if err != nil {
		panic(fmt.Sprintf("GetRootDir failed: %v", err))
	}

	if s.flags.IntegrationTestsEnabled {
		if addToManagerFn == nil {
			panic("addToManagerFn is nil")
		}

		apiServerFlags := append([]string{"--allow-privileged=true"}, envtest.DefaultKubeAPIServerFlags...)
		if len(additionalAPIServerFlags) > 0 {
			apiServerFlags = append(apiServerFlags, additionalAPIServerFlags...)
		}

		s.addToManagerFn = addToManagerFn
		s.envTest = envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join(s.rootDir, "config", "crd", "bases"),
				filepath.Join(s.rootDir, "config", "crd", "external-crds"),
				//filepath.Join(s.rootDir, "test", "stubcrds"),
				//filepath.Join(testutil.FindModuleDir("gitlab.eng.vmware.com/core-build/cluster-api-provider-wcp"), "config", "crd", "bases"),
			},
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
		SetDefaultEventuallyTimeout(time.Second * 30)
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
		ctx := NewUnitTestContextForController(s.newReconcilerFn, initObjects)
		reconcileNormalAndExpectSuccess(ctx)
		return ctx
	}
	return nil
}

// NewUnitTestContextForControllerWithVirtualMachine returns a new unit test context for this
// suite's reconciler.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForControllerWithVirtualMachine(vm *vmopv1.VirtualMachine,
	initObjects []runtime.Object, positiveTestCase bool) *UnitTestContextForController {

	if s.flags.UnitTestsEnabled {
		ctx := NewUnitTestContextForController(s.newReconcilerFn, initObjects)
		if positiveTestCase {
			reconcileNormalAndExpectSuccess(ctx)
			// Update the VirtualMachine and its status in the fake client.
			//Expect(ctx.Client.Update(ctx, ctx.Cluster)).To(Succeed())
			//Expect(ctx.Client.Status().Update(ctx, ctx.Cluster)).To(Succeed())
		}
		return ctx
	}
	return nil
}

func reconcileNormalAndExpectSuccess(ctx *UnitTestContextForController) {
	// Manually invoke the reconciliation. This is poor design, but in order
	// to support unit testing with a minimum set of dependencies that does
	// not include the Kubernetes envtest package, this is required.
	Expect(ctx.ReconcileNormal()).ShouldNot(HaveOccurred())
}

// NewUnitTestContextForValidatingWebhook returns a new unit test context for this
// suite's validator.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForValidatingWebhook(
	obj, oldObj *unstructured.Unstructured,
	initObjects ...runtime.Object) *UnitTestContextForValidatingWebhook {

	if s.flags.UnitTestsEnabled {
		ctx := NewUnitTestContextForValidatingWebhook(s.validator, obj, oldObj, initObjects...)
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
	s.done = make(chan struct{})
	s.manager, err = manager.New(manager.Options{
		KubeConfig: s.config,
		// Create a new Scheme for each controller. Don't use a global scheme otherwise manager reset
		// will try to reinitialize the global scheme which causes errors
		Scheme:      runtime.NewScheme(),
		MetricsAddr: "0",
		NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			syncPeriod := 1 * time.Second
			opts.Resync = &syncPeriod
			return cache.New(config, opts)
		},
		AddToManager: s.addToManagerFn,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(s.manager).ToNot(BeNil())
	s.integrationTestClient = s.manager.GetClient()
}

func (s *TestSuite) initializeManager() {
	// If one or more webhooks are being tested then go ahead and configure the
	// webhook server.
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
	var result bool
	s.managerRunningMutex.Lock()
	result = s.managerRunning
	s.managerRunningMutex.Unlock()
	return result
}

// Starts the manager and sets managerRunning
func (s *TestSuite) startManager() {
	go func() {
		defer GinkgoRecover()

		s.setManagerRunning(true)
		Expect(s.manager.Start(s.done)).ToNot(HaveOccurred())
		s.setManagerRunning(false)
	}()
}

// Blocks until the manager has stopped running
// Removes state applied in postConfigureManager()
func (s *TestSuite) stopManager() {
	close(s.done)
	Eventually(func() bool {
		return s.getManagerRunning()
	}).Should(BeFalse())
	if s.webhookYaml != nil {
		Eventually(func() error {
			return remote.DeleteYAML(s, s.integrationTestClient, s.webhookYaml)
		}).Should(Succeed())
	}
}

// Applies configuration to the Manager after it has started
func (s *TestSuite) postConfigureManager() {
	// If there's a configured certificate directory then it means one or more
	// webhooks are being tested. Go ahead and install the webhooks and wait
	// for the webhook server to come online.
	if s.isWebhookTest() {
		By("installing the webhook(s)", func() {
			// ASSERT that the file for validating webhook file exists.
			validatingWebhookFile := path.Join(s.rootDir, "config", "webhook", "manifests.yaml")
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
			addr := fmt.Sprintf("%s:%d",
				s.manager.GetWebhookServer().Host,
				s.manager.GetWebhookServer().Port)
			dialer := &net.Dialer{Timeout: time.Second}
			//nolint:gosec
			tlsConfig := &tls.Config{InsecureSkipVerify: true}
			Eventually(func() error {
				conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
				if err != nil {
					return err
				}
				conn.Close()
				return nil
			}).Should(Succeed())
		})
	}
}

// Start a whole new manager in the current context
// Optional to stop the manager before starting a new one
func (s *TestSuite) startNewManager(ctx *IntegrationTestContext) {
	if s.getManagerRunning() {
		s.stopManager()
	}
	s.createManager()
	s.initializeManager()
	s.startManager()
	s.postConfigureManager()
	ctx.Client = s.integrationTestClient
}

func (s *TestSuite) beforeSuiteForIntegrationTesting() {
	By("bootstrapping test environment", func() {
		var err error
		s.config, err = s.envTest.Start()
		Expect(err).ToNot(HaveOccurred())
		Expect(s.config).ToNot(BeNil())
	})

	// If one or more webhooks are being tested then go ahead and generate a
	// PKI toolchain to use with the webhook server.
	if s.isWebhookTest() {
		By("generating the pki toolchain", func() {
			var err error
			s.pki, err = generatePKIToolchain()
			Expect(err).ToNot(HaveOccurred())
			// Write the CA pub key and cert pub and private keys to the cert dir.
			Expect(ioutil.WriteFile(path.Join(s.certDir, "tls.crt"), s.pki.publicKeyPEM, 0400)).To(Succeed())
			Expect(ioutil.WriteFile(path.Join(s.certDir, "tls.key"), s.pki.privateKeyPEM, 0400)).To(Succeed())
		})
	}

	By("setting up a new manager", func() {
		s.createManager()
		s.initializeManager()
	})

	By("starting the manager", func() {
		s.startManager()
	})

	By("configuring the manager", func() {
		s.postConfigureManager()
	})

	// If the test is about capi validation webhook then create a config map and set environment variable
	/*
		if s.capiWebhookValidator != nil {
			testCMName, testNS := "test-cm", "test-namespace"

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCMName,
					Namespace: testNS,
				},
				Data: map[string]string{
					"system.unsecured": "true",
				},
			}
			Expect(s.integrationTestClient.Create(s, cm)).To(BeNil())
		}
	*/
}

func (s *TestSuite) afterSuiteForIntegrationTesting() {
	By("tearing down the test environment", func() {
		close(s.done)
		Expect(s.envTest.Stop()).To(Succeed())
	})
}

func parseWebhookConfig(path string) (mutatingWebhookConfig admissionregv1.MutatingWebhookConfiguration, validatingWebhookConfig admissionregv1.ValidatingWebhookConfiguration) {
	// READ the validating webhook file.
	yamlIn, err := ioutil.ReadFile(path)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(yamlIn).ShouldNot(BeEmpty())

	sep := len([]byte(separator))
	i := bytes.Index(yamlIn, []byte(separator))
	j := bytes.LastIndex(yamlIn, []byte(separator))
	mbytes := yamlIn[i+sep : j]
	vbytes := yamlIn[j+sep:]
	Expect(yaml.Unmarshal(mbytes, &mutatingWebhookConfig)).To(Succeed())
	Expect(yaml.Unmarshal(vbytes, &validatingWebhookConfig)).To(Succeed())
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
			url := fmt.Sprintf("https://%s:%d%s", host, port,
				*webhook.ClientConfig.Service.Path)
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
			url := fmt.Sprintf("https://%s:%d%s", host, port,
				*webhook.ClientConfig.Service.Path)
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
