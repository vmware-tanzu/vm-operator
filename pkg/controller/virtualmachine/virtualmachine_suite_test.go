
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package virtualmachine_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/test"

	"vmware.com/kubevsphere/pkg/apis"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	"vmware.com/kubevsphere/pkg/openapi"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	"vmware.com/kubevsphere/pkg/controller/virtualmachine"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset
var shutdown chan struct{}
var controller *virtualmachine.VirtualMachineController
var si *sharedinformers.SharedInformers

func TestVirtualMachine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachine Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	testenv = test.NewTestEnvironment()
	config = testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs = clientset.NewForConfigOrDie(config)

	shutdown = make(chan struct{})
	si = sharedinformers.NewSharedInformers(config, shutdown)
	controller = virtualmachine.NewVirtualMachineController(config, si)
	controller.Run(shutdown)
})

var _ = AfterSuite(func() {
	close(shutdown)
	testenv.Stop()
})
