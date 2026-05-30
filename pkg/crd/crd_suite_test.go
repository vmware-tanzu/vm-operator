// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd_test

import (
	goruntime "runtime"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
	_ "github.com/vmware-tanzu/vm-operator/test/builder/log"
)

var (
	envTestEnv    *envtest.Environment
	envTestClient ctrlclient.Client
)

func TestCRD(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD Test Suite")
}

func envTestsEnabled() bool {
	return Label(testlabels.EnvTest).MatchesLabelFilter(GinkgoLabelFilter())
}

var _ = BeforeSuite(func() {
	if !envTestsEnabled() {
		return
	}

	rootDir := testutil.GetRootDirOrDie()
	envTestEnv = &envtest.Environment{
		BinaryAssetsDirectory: filepath.Join(rootDir, "hack", "tools", "bin", goruntime.GOOS+"_"+goruntime.GOARCH),
	}

	cfg, err := envTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	scheme := runtime.NewScheme()
	Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())

	envTestClient, err = ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if envTestEnv != nil {
		Expect(envTestEnv.Stop()).To(Succeed())
	}
})
