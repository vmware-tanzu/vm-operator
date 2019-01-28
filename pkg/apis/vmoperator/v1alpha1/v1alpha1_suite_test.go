/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1_test

import (
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"vmware.com/kubevsphere/pkg/apis"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	"vmware.com/kubevsphere/pkg/openapi"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset

func TestV1alpha1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "v1 Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	testenv = test.NewTestEnvironment()
	config = testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs = clientset.NewForConfigOrDie(config)
})

var _ = AfterSuite(func() {
	testenv.Stop()
})
