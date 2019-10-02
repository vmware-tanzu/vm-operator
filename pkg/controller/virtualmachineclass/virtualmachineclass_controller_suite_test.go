// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineclass

import (
	stdlog "log"
	"sync"
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"

	"github.com/vmware-tanzu/vm-operator/test/integration"
	"k8s.io/client-go/rest"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestVirtualMachineClass(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineClass Suite", []Reporter{test.NewlineReporter{}})
}

var (
	cfg           *rest.Config
	vcSim         *integration.VcSimInstance
	testEnv       *suite.Environment
	err           error
	vSphereConfig *vsphere.VSphereVmProviderConfig
)

var _ = BeforeSuite(func() {
	stdlog.Print("setting up local aggregated-apiserver for test env...")
	testEnv, err = suite.InstallLocalTestingAPIAggregationEnvironment("vmoperator.vmware.com", "v1alpha1")
	Expect(err).NotTo(HaveOccurred())

	cfg = testEnv.LoopbackClientConfig

	err = apis.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	stdlog.Print("setting up integration test env..")
	vcSim = integration.NewVcSimInstance()

	vSphereConfig, err = integration.SetupEnv(vcSim)
	Expect(err).NotTo(HaveOccurred())
	Expect(vSphereConfig).ShouldNot(Equal(nil))
})

var _ = AfterSuite(func() {
	integration.CleanupEnv(vcSim)

	stdlog.Print("stopping aggregated-apiserver..")
	err = testEnv.StopAggregatedAPIServer()
	Expect(err).NotTo(HaveOccurred())

	stdlog.Print("stopping kube-apiserver..")
	err = testEnv.KubeAPIServerEnvironment.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})
	return fn, requests
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
