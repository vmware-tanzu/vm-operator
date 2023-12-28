// Copyright (c) 2020-2022 VMware, Inc. All Rights Reserved.
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
	"github.com/pkg/errors"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"

	vmopv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/controllers/util/remote"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	ctrlCtx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

const (
	VMICRDName = "virtualmachineimages.vmoperator.vmware.com"
)

// Reconciler is a base type for builder's reconcilers.
type Reconciler interface{}

// NewReconcilerFunc is a base type for functions that return a reconciler.
type NewReconcilerFunc func() Reconciler

func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klogr.New())
}

// TestSuite is used for unit and integration testing builder. Each TestSuite
// contains one independent test environment and a controller manager.
type TestSuite struct {
	context.Context

	flags                 testFlags
	envTest               envtest.Environment
	config                *rest.Config
	integrationTestClient ctrlclient.Client
	integrationTest       bool
	initProvidersFn       pkgmgr.InitializeProvidersFunc

	// Controller manager specific fields.
	manager             pkgmgr.Manager
	managerRunning      bool
	managerRunningMutex sync.Mutex

	// Cancel function that will be called to close the Done channel of the
	// Context, which will then stop the manager.
	cancelFuncMutex sync.Mutex
	cancelFunc      context.CancelFunc

	// Controller specific fields.
	controllers []pkgmgr.AddToManagerFunc

	// Webhook specific fields.
	webhook testSuiteWebhookConfig

	// Feature state switches.
	fssMap map[string]bool
}

type testSuiteWebhookConfig struct {
	certDir        string
	pki            pkiToolchain
	conversionOpts []TestSuiteConversionWebhookOptions
	conversionName []string
	mutationOpts   []TestSuiteMutationWebhookOptions
	mutationYAML   []byte
	validationOpts []TestSuiteValidationWebhookOptions
	validationYAML []byte
}

func (s *TestSuite) isControllerTest() bool {
	return len(s.controllers) > 0
}

func (s *TestSuite) isWebhookTest() bool {
	return s.isAdmissionWebhookTest() || s.isConversionWebhookTest()
}

func (s *TestSuite) isAdmissionWebhookTest() bool {
	return s.isMutationWebhookTest() || s.isValidationWebhookTest()
}

func (s *TestSuite) isConversionWebhookTest() bool {
	return len(s.webhook.conversionOpts) > 0
}

func (s *TestSuite) isMutationWebhookTest() bool {
	return len(s.webhook.mutationOpts) > 0
}

func (s *TestSuite) isValidationWebhookTest() bool {
	return len(s.webhook.validationOpts) > 0
}

func (s *TestSuite) GetEnvTestConfig() *rest.Config {
	return s.config
}

func (s *TestSuite) GetLogger() logr.Logger {
	return logf.Log
}

// NewTestSuite returns a new test suite used for unit and/or integration test.
func NewTestSuite() *TestSuite {
	return NewTestSuiteWithOptions(
		TestSuiteOptions{
			InitProviderFn: pkgmgr.InitializeProvidersNoopFn,
			Controllers:    []pkgmgr.AddToManagerFunc{pkgmgr.AddToManagerNoopFn},
		})
}

// NewFunctionalTestSuite returns a new test suite used for functional tests.
// The functional test starts all the controllers, and creates all the providers
// so it is a more fully functioning env than an integration test with a single
// controller running.
func NewFunctionalTestSuite(addToManagerFn pkgmgr.AddToManagerFunc) *TestSuite {
	return NewTestSuiteWithOptions(
		TestSuiteOptions{
			InitProviderFn: pkgmgr.InitializeProviders,
			Controllers:    []pkgmgr.AddToManagerFunc{addToManagerFn},
		})
}

// NewTestSuiteForController returns a new test suite used for controller
// integration test.
func NewTestSuiteForController(
	addToManagerFn pkgmgr.AddToManagerFunc,
	initProvidersFn pkgmgr.InitializeProvidersFunc) *TestSuite {

	return NewTestSuiteWithOptions(
		TestSuiteOptions{
			InitProviderFn: initProvidersFn,
			Controllers:    []pkgmgr.AddToManagerFunc{addToManagerFn},
			FeatureStates:  map[string]bool{},
		})
}

// NewTestSuiteForControllerWithFSS returns a new test suite used for controller
// integration test with FSS set.
func NewTestSuiteForControllerWithFSS(
	addToManagerFn pkgmgr.AddToManagerFunc,
	initProvidersFn pkgmgr.InitializeProvidersFunc,
	fssMap map[string]bool) *TestSuite {

	return NewTestSuiteWithOptions(
		TestSuiteOptions{
			InitProviderFn: initProvidersFn,
			Controllers:    []pkgmgr.AddToManagerFunc{addToManagerFn},
			FeatureStates:  fssMap,
		})
}

type TestSuiteOptions struct {
	InitProviderFn     pkgmgr.InitializeProvidersFunc
	FeatureStates      map[string]bool
	Controllers        []pkgmgr.AddToManagerFunc
	ValidationWebhooks []TestSuiteValidationWebhookOptions
	MutationWebhooks   []TestSuiteMutationWebhookOptions
	ConversionWebhooks []TestSuiteConversionWebhookOptions
}

type TestSuiteValidationWebhookOptions struct {
	// Name is the unique ID of the validation webhook, ex.
	// default.validating.virtualmachine.v1alpha1.vmoperator.vmware.com.
	Name string

	// AddToManagerFn is the function that adds the webhook to the controller
	// manager.
	AddToManagerFn pkgmgr.AddToManagerFunc

	// ValidatorFn is used to unit testing.
	ValidatorFn builder.ValidatorFunc
}

type TestSuiteMutationWebhookOptions struct {
	// Name is the unique ID of the mutation webhook, ex.
	// default.mutating.virtualmachine.v1alpha1.vmoperator.vmware.com.
	Name string

	// AddToManagerFn is the function that adds the webhook to the controller
	// manager.
	AddToManagerFn pkgmgr.AddToManagerFunc

	// MutatorFn is used to unit testing.
	MutatorFn builder.MutatorFunc
}

type TestSuiteConversionWebhookOptions struct {
	// Name is the resource to which the conversion webhook applies. For
	// example, the name of the VirtualMachine resource is
	// virtualmachines.vmoperator.vmware.com.
	Name string

	// AddToManagerFn is a list of the functions that add the webhook(s) to the
	// controller manager. There is a distinct function for each version of the
	// resource specified by Name.
	AddToManagerFn []func(ctrl.Manager) error
}

// NewTestSuiteWithOptions returns a new test suite used for controller integration test with FSS set.
func NewTestSuiteWithOptions(opts TestSuiteOptions) *TestSuite {

	if len(opts.Controllers) == 0 &&
		len(opts.ValidationWebhooks) == 0 &&
		len(opts.MutationWebhooks) == 0 &&
		len(opts.ConversionWebhooks) == 0 {

		panic("there are no addToManager functions")
	}
	if opts.InitProviderFn == nil {
		panic("initProvidersFn is nil")
	}

	testSuite := &TestSuite{
		Context:         context.Background(),
		integrationTest: true,
		controllers:     opts.Controllers,
		initProvidersFn: opts.InitProviderFn,
		fssMap:          opts.FeatureStates,
		webhook: testSuiteWebhookConfig{
			conversionOpts: opts.ConversionWebhooks,
			mutationOpts:   opts.MutationWebhooks,
			validationOpts: opts.ValidationWebhooks,
		},
	}

	if testSuite.isWebhookTest() {
		// Create a temp directory for the certs needed for testing webhooks.
		certDir, err := os.MkdirTemp(os.TempDir(), "")
		if err != nil {
			panic(errors.Wrap(err, "failed to create temp dir for certs"))
		}
		testSuite.webhook.certDir = certDir
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

	return newTestSuiteForWebhook(addToManagerFn, newValidatorFn, nil, webhookName, map[string]bool{})
}

// NewTestSuiteForValidatingWebhookwithFSS returns a new test suite used for unit and
// integration testing validating webhooks created using the "pkg/builder"
// package with FSS set.
func NewTestSuiteForValidatingWebhookwithFSS(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	webhookName string,
	fssMap map[string]bool) *TestSuite {

	return newTestSuiteForWebhook(addToManagerFn, newValidatorFn, nil, webhookName, fssMap)
}

// NewTestSuiteForMutatingWebhook returns a new test suite used for unit and
// integration testing mutating webhooks created using the "pkg/builder"
// package.
func NewTestSuiteForMutatingWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newMutatorFn builder.MutatorFunc,
	webhookName string) *TestSuite {

	return newTestSuiteForWebhook(addToManagerFn, nil, newMutatorFn, webhookName, map[string]bool{})
}

// NewTestSuiteForMutatingWebhookwithFSS returns a new test suite used for unit and
// integration testing mutating webhooks created using the "pkg/builder"
// package with FSS set.
func NewTestSuiteForMutatingWebhookwithFSS(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newMutatorFn builder.MutatorFunc,
	webhookName string,
	fssMap map[string]bool) *TestSuite {

	return newTestSuiteForWebhook(addToManagerFn, nil, newMutatorFn, webhookName, fssMap)
}

func newTestSuiteForWebhook(
	addToManagerFn pkgmgr.AddToManagerFunc,
	newValidatorFn builder.ValidatorFunc,
	newMutatorFn builder.MutatorFunc,
	webhookName string,
	fssMap map[string]bool) *TestSuite {

	opts := TestSuiteOptions{
		InitProviderFn: pkgmgr.InitializeProvidersNoopFn,
		FeatureStates:  fssMap,
	}

	if newMutatorFn != nil {
		opts.MutationWebhooks = []TestSuiteMutationWebhookOptions{
			{
				Name:           webhookName,
				MutatorFn:      newMutatorFn,
				AddToManagerFn: addToManagerFn,
			},
		}
	}
	if newValidatorFn != nil {
		opts.ValidationWebhooks = []TestSuiteValidationWebhookOptions{
			{
				Name:           webhookName,
				ValidatorFn:    newValidatorFn,
				AddToManagerFn: addToManagerFn,
			},
		}
	}

	return NewTestSuiteWithOptions(opts)
}

func (s *TestSuite) init() {
	// Initialize the test flags.
	s.flags = flags

	rootDir := testutil.GetRootDirOrDie()

	crds, err := LoadCRDs(filepath.Join(rootDir, "config", "crd", "bases"))
	if err != nil {
		panic(err)
	}

	if s.flags.IntegrationTestsEnabled {
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
	}

	RunSpecs(t, name)
}

// NewUnitTestContextForController returns a new unit test context for this
// suite's reconciler.
//
// Returns nil if unit testing is disabled.
func (s *TestSuite) NewUnitTestContextForController(initObjects ...ctrlclient.Object) *UnitTestContextForController {
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
	initObjects ...ctrlclient.Object) *UnitTestContextForValidatingWebhook {

	if s.flags.UnitTestsEnabled {
		ctx := NewUnitTestContextForValidatingWebhook(s.webhook.validationOpts[0].ValidatorFn, obj, oldObj, initObjects...)
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
		ctx := NewUnitTestContextForMutatingWebhook(s.webhook.mutationOpts[0].MutatorFn, obj)
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

// Create a new Manager with default values.
func (s *TestSuite) createManager() {
	var err error

	opts := pkgmgr.Options{
		KubeConfig:          s.config,
		MetricsAddr:         "0",
		InitializeProviders: s.initProvidersFn,
		AddToManager: func(
			ctx *ctrlCtx.ControllerManagerContext,
			m manager.Manager) error {

			if s.isControllerTest() {
				By("registering controllers")
				for i := range s.controllers {
					if err := s.controllers[i](ctx, m); err != nil {
						return err
					}
				}
			}

			if s.isConversionWebhookTest() {
				By("registering conversion webhooks")
				for i := range s.webhook.conversionOpts {
					for j := range s.webhook.conversionOpts[i].AddToManagerFn {
						if err := s.webhook.conversionOpts[i].AddToManagerFn[j](m); err != nil {
							return err
						}
					}
				}
			}

			if s.isMutationWebhookTest() {
				By("registering mutation webhooks")
				for i := range s.webhook.mutationOpts {
					if err := s.webhook.mutationOpts[i].AddToManagerFn(ctx, m); err != nil {
						return err
					}
				}
			}

			if s.isValidationWebhookTest() {
				By("registering validation webhooks")
				for i := range s.webhook.validationOpts {
					if err := s.webhook.validationOpts[i].AddToManagerFn(ctx, m); err != nil {
						return err
					}
				}
			}

			return nil
		},
	}

	if enabled, ok := s.fssMap[lib.VMServiceV1Alpha2FSS]; ok && enabled {
		opts.Scheme = runtime.NewScheme()
		_ = vmopv1alpha2.AddToScheme(opts.Scheme)
	}

	s.manager, err = pkgmgr.New(opts)

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
			svr.Options.CertDir = s.webhook.certDir
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
		svr := s.manager.GetWebhookServer().(*webhook.DefaultServer)

		if s.isConversionWebhookTest() {
			updateConversionWebhookConfig(
				s, s.integrationTestClient,
				s.webhook.conversionOpts,
				svr.Options.Host, svr.Options.Port,
				s.webhook.pki.publicKeyPEM,
				&s.webhook.conversionName)
		}

		if s.isAdmissionWebhookTest() {
			// ASSERT the admission webhook manifest file exists.
			admissionWebhookManifestFilePath := path.Join(
				testutil.GetRootDirOrDie(),
				"config", "webhook", "manifests.yaml")
			Expect(admissionWebhookManifestFilePath).Should(BeAnExistingFile())

			// UNMARSHAL the contents of the admission webhook manifest file.
			mutationWebhookConfig, validationWebhookConfig := parseAdmissionWebhookManifestFile(
				admissionWebhookManifestFilePath)

			updateMutationWebhookConfig(
				s, s.integrationTestClient,
				s.webhook.mutationOpts, mutationWebhookConfig,
				svr.Options.Host, svr.Options.Port,
				s.webhook.pki.publicKeyPEM,
				&s.webhook.mutationYAML)

			updateValidationWebhookConfig(
				s, s.integrationTestClient,
				s.webhook.validationOpts, validationWebhookConfig,
				svr.Options.Host, svr.Options.Port,
				s.webhook.pki.publicKeyPEM,
				&s.webhook.validationYAML)
		}

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

	By("updating CRD scope", func() {
		if enabled, ok := s.fssMap[lib.VMImageRegistryFSS]; ok {
			crd := s.GetInstalledCRD(VMICRDName)
			Expect(crd).ToNot(BeNil())
			scope := string(crd.Spec.Scope)
			if enabled && scope == "Cluster" {
				s.UpdateCRDScope(crd, "Namespaced")
			} else if !enabled && scope == "Namespaced" {
				s.UpdateCRDScope(crd, "Cluster")
			}
		}
		// TODO: Include NamespacedVMClass related changes
	})

	// If one or more webhooks are being tested then go ahead and generate a
	// PKI toolchain to use with the webhook server.
	if s.isWebhookTest() {
		By("generating the pki toolchain", func() {
			s.webhook.pki, err = generatePKIToolchain()
			Expect(err).ToNot(HaveOccurred())
			// Write the CA pub key and cert pub and private keys to the cert dir.
			tlsCrtPath := path.Join(s.webhook.certDir, "tls.crt")
			tlsKeyPath := path.Join(s.webhook.certDir, "tls.key")
			Expect(os.WriteFile(tlsCrtPath, s.webhook.pki.publicKeyPEM, 0400)).To(Succeed())
			Expect(os.WriteFile(tlsKeyPath, s.webhook.pki.privateKeyPEM, 0400)).To(Succeed())
		})
	}

	if s.integrationTest {
		By("setting up a new manager", func() {
			s.createManager()
			s.initializeManager()
		})

		s.integrationTestClient, err = ctrlclient.New(s.manager.GetConfig(), ctrlclient.Options{Scheme: s.manager.GetScheme()})
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

			if data := s.webhook.conversionName; len(data) > 0 {
				By("tearing down conversion webhooks")
				for i := range data {
					name := data[i]
					crd := apiextensionsv1.CustomResourceDefinition{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "apiextensions.k8s.io/v1",
							Kind:       "CustomResourceDefinition",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: name,
						},
					}
					Expect(s.integrationTestClient.Get(s, ctrlclient.ObjectKey{Name: name}, &crd)).To(Succeed())
					Expect(crd.Spec.Conversion).ToNot(BeNil())
					crd.Spec.Conversion = nil
					Expect(s.integrationTestClient.Update(s, &crd)).To(Succeed())
					Expect(s.integrationTestClient.Get(s, ctrlclient.ObjectKey{Name: name}, &crd)).To(Succeed())
					Expect(crd.Spec.Conversion).ToNot(BeNil())
					Expect(crd.Spec.Conversion.Strategy).To(Equal(apiextensionsv1.NoneConverter))
					Expect(crd.Spec.Conversion.Webhook).To(BeNil())
				}
			}

			if data := s.webhook.mutationYAML; len(data) > 0 {
				By("tearing down mutation webhooks")
				Eventually(func() error {
					return remote.DeleteYAML(s, s.integrationTestClient, data)
				}).Should(Succeed())
			}

			if data := s.webhook.validationYAML; len(data) > 0 {
				By("tearing down validation webhooks")
				Eventually(func() error {
					return remote.DeleteYAML(s, s.integrationTestClient, data)
				}).Should(Succeed())
			}
		})
	}

	By("tearing down the test environment", func() {
		Expect(s.envTest.Stop()).To(Succeed())
	})
}

func (s *TestSuite) applyFeatureStatesToCRDs(in []*apiextensionsv1.CustomResourceDefinition) []*apiextensionsv1.CustomResourceDefinition {
	out := make([]*apiextensionsv1.CustomResourceDefinition, 0)
	for i := range in {
		crd := applyFeatureStateFnsToCRD(
			*in[i],
			s.fssMap,
			applyV1Alpha2FSSToCRD)
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

func parseAdmissionWebhookManifestFile(path string) (
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

func updateConversionWebhookConfig(
	ctx context.Context,
	client ctrlclient.Client,
	webhookOption []TestSuiteConversionWebhookOptions,
	host string,
	port int,
	key []byte,
	addrOfName *[]string) {

	if len(webhookOption) == 0 {
		return
	}

	By("installing conversion webhooks")

	for _, in := range webhookOption {
		crd := apiextensionsv1.CustomResourceDefinition{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apiextensions.k8s.io/v1",
				Kind:       "CustomResourceDefinition",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: in.Name,
			},
		}
		Expect(client.Get(ctx, ctrlclient.ObjectKey{Name: in.Name}, &crd)).To(Succeed())

		crd.Spec.Conversion = &apiextensionsv1.CustomResourceConversion{
			Strategy: apiextensionsv1.WebhookConverter,
			Webhook: &apiextensionsv1.WebhookConversion{
				ConversionReviewVersions: []string{"v1", "v1beta1"},
				ClientConfig:             &apiextensionsv1.WebhookClientConfig{},
			},
		}
		updateAPIExtensionWebhookConfig(
			host, port, "/convert",
			key, crd.Spec.Conversion.Webhook.ClientConfig)

		Expect(client.Update(ctx, &crd)).To(Succeed())

		*addrOfName = append(*addrOfName, in.Name)
	}
}

func updateMutationWebhookConfig(
	ctx context.Context,
	client ctrlclient.Client,
	webhookOption []TestSuiteMutationWebhookOptions,
	webhookConfig admissionregv1.MutatingWebhookConfiguration,
	host string,
	port int,
	key []byte,
	addrOfYAML *[]byte) {

	if len(webhookOption) == 0 {
		return
	}

	By("installing mutation webhooks")

	webhookConfigToInstall := webhookConfig.DeepCopy()
	webhookConfigToInstall.Webhooks = nil

	for _, in := range webhookOption {
		for _, out := range webhookConfig.Webhooks {
			if in.Name == out.Name {
				updateAdmissionWebhookConfig(
					host, port, *out.ClientConfig.Service.Path, key,
					&out.ClientConfig)
				webhookConfigToInstall.Webhooks = append(
					webhookConfigToInstall.Webhooks,
					out)
			}
		}
	}

	if len(webhookConfigToInstall.Webhooks) == 0 {
		return
	}

	By(fmt.Sprintf("installing %d mutation webhooks",
		len(webhookConfigToInstall.Webhooks)))

	data, err := yaml.Marshal(webhookConfigToInstall)
	Expect(err).ShouldNot(HaveOccurred())
	*addrOfYAML = data

	// ASSERT that eventually the webhook config gets successfully
	// applied to the API server.
	Eventually(func() error {
		return remote.ApplyYAML(ctx, client, data)
	}).Should(Succeed())
}

func updateValidationWebhookConfig(
	ctx context.Context,
	client ctrlclient.Client,
	webhookOption []TestSuiteValidationWebhookOptions,
	webhookConfig admissionregv1.ValidatingWebhookConfiguration,
	host string,
	port int,
	key []byte,
	addrOfYAML *[]byte) {

	if len(webhookOption) == 0 {
		return
	}

	By("installing validation webhooks")

	webhookConfigToInstall := webhookConfig.DeepCopy()
	webhookConfigToInstall.Webhooks = nil

	for _, in := range webhookOption {
		for _, out := range webhookConfig.Webhooks {
			if in.Name == out.Name {
				updateAdmissionWebhookConfig(
					host, port, *out.ClientConfig.Service.Path, key,
					&out.ClientConfig)
				webhookConfigToInstall.Webhooks = append(
					webhookConfigToInstall.Webhooks,
					out)
			}
		}
	}

	if len(webhookConfigToInstall.Webhooks) == 0 {
		return
	}

	By(fmt.Sprintf("installing %d validation webhooks",
		len(webhookConfigToInstall.Webhooks)))

	data, err := yaml.Marshal(webhookConfigToInstall)
	Expect(err).ShouldNot(HaveOccurred())
	*addrOfYAML = data

	// ASSERT that eventually the webhook config gets successfully
	// applied to the API server.
	Eventually(func() error {
		return remote.ApplyYAML(ctx, client, data)
	}).Should(Succeed())
}

func updateAdmissionWebhookConfig(
	host string, port int, path string, key []byte,
	webhookConfig *admissionregv1.WebhookClientConfig) {

	webhookConfig.CABundle = key
	webhookConfig.Service = nil
	webhookConfig.URL = addrOf(fmt.Sprintf("https://%s:%d%s", host, port, path))
}

func updateAPIExtensionWebhookConfig(
	host string, port int, path string, key []byte,
	webhookConfig *apiextensionsv1.WebhookClientConfig) {

	webhookConfig.CABundle = key
	webhookConfig.Service = nil
	webhookConfig.URL = addrOf(fmt.Sprintf("https://%s:%d%s", host, port, path))
}
