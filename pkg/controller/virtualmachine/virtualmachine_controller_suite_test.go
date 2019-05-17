// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	stdlog "log"
	"os"
	"sync"
	"testing"

	"github.com/vmware-tanzu/vm-operator/test/integration"
	"k8s.io/client-go/rest"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	"github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	stdlog.Printf("TestMain")

	t, err := suite.InstallLocalTestingAPIAggregationEnvironment("vmoperator.vmware.com", "v1alpha1")
	if err != nil {
		stdlog.Panic(err)
	}

	cfg = t.LoopbackClientConfig
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		stdlog.Panic(err)
	}

	stdlog.Print("setting up integration test env..")
	cleanupEnv, err := integration.SetupEnv()
	if err != nil {
		stdlog.Panic(err)
	}
	code := m.Run()
	cleanupEnv()

	stdlog.Print("stopping aggregated-apiserver..")
	if err := t.StopAggregatedAPIServer(); err != nil {
		stdlog.Panic(err)
		return
	}
	stdlog.Print("stopping kube-apiserver..")
	if err := t.KubeAPIServerEnvironment.Stop(); err != nil {
		stdlog.Panic(err)
		return
	}

	os.Exit(code)
}

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
func StartTestManager(mgr manager.Manager, g *gomega.GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	go func() {
		wg.Add(1)
		g.Expect(mgr.Start(stop)).NotTo(gomega.HaveOccurred())
		wg.Done()
	}()
	return stop, wg
}
