// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package tag contains E2E tests for the Tag CRD's admission webhook,
// exercising its derived-name rule directly against a privileged client.
package tag

import (
	"context"
	"fmt"

	"github.com/cespare/xxhash/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	capiutil "sigs.k8s.io/cluster-api/util"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// vCenterTagName returns the vCenter tag name for a label key/value pair:
// "<key>:<value>". Byte-for-byte what
// pkg/providers/vsphere/virtualmachine.TagName computes; reimplemented
// here rather than imported so this e2e module does not pick up that
// package's production dependency graph for two one-line derivations.
func vCenterTagName(key, value string) string {
	return key + ":" + value
}

// tagResourceName returns the derived name of the Tag resource for a
// namespace and label key/value pair: "tag-" followed by the 16 hex
// character XXHash64 digest of "<namespace>:<key>:<value>".
func tagResourceName(namespace, key, value string) string {
	return fmt.Sprintf("tag-%016x", xxhash.Sum64String(namespace+":"+vCenterTagName(key, value)))
}

// newTag returns a Tag for the given label key/value pair, named per its
// derived name.
func newTag(namespace, key, value string) *vspherepolv1.Tag {
	return &vspherepolv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tagResourceName(namespace, key, value),
			Namespace: namespace,
		},
		Spec: vspherepolv1.TagSpec{
			Key:   key,
			Value: value,
		},
	}
}

// randomLabelPair returns a unique label key/value pair for a scenario, so
// concurrent or repeated runs of this suite never collide on the same
// derived Tag resource name.
func randomLabelPair(scenario string) (key, value string) {
	suffix := capiutil.RandomString(6)
	return fmt.Sprintf("e2e-tag-%s-%s", scenario, suffix), "v1"
}

// SpecInput is the input to Spec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPNamespaceName string
}

// Spec exercises the Tag validating webhook directly, managing Tags
// through a privileged (admin) client.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "tag-cr"

	var (
		input          SpecInput
		svClusterProxy *common.VMServiceClusterProxy

		adminProxy  *common.VMServiceClusterProxy
		adminClient ctrlclient.Client
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		svClusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, svClusterProxy, consts.VMHardAffinityDuringExecutionCapabilityName)

		var err error
		adminProxy, err = svClusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to get admin cluster proxy for Tag creation")
		DeferCleanup(func() { adminProxy.Dispose(ctx) })

		adminClient, err = adminProxy.GetAdminClient()
		Expect(err).ToNot(HaveOccurred(), "failed to get admin client for Tag creation")
	})

	Context("When a privileged client manages a Tag directly", func() {
		It("rejects a Tag whose name does not equal the name derived from spec.key and spec.value",
			Label("core-functional", "experimental"), func() {
				key, value := randomLabelPair("badname")
				tagObj := newTag(input.WCPNamespaceName, key, value)
				tagObj.Name = "tag-does-not-match-derived-name"

				err := adminClient.Create(ctx, tagObj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must equal"))
			})

	})

	Context("When a CSP admin diagnoses affinity tagging", func() {
		It("surfaces the label key and value as default 'kubectl get' columns",
			Label("core-functional", "experimental"), func() {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				Expect(adminClient.Get(ctx, ctrlclient.ObjectKey{Name: "tags.vsphere.policy.vmware.com"}, crd)).To(Succeed())

				var version *apiextensionsv1.CustomResourceDefinitionVersion
				for i := range crd.Spec.Versions {
					if crd.Spec.Versions[i].Name == vspherepolv1.GroupVersion.Version {
						version = &crd.Spec.Versions[i]
						break
					}
				}
				Expect(version).ToNot(BeNil(), "expected a %q version on the Tag CRD", vspherepolv1.GroupVersion.Version)

				var haveKey, haveValue bool
				for _, col := range version.AdditionalPrinterColumns {
					switch {
					case col.Name == "Key" && col.JSONPath == ".spec.key":
						haveKey = true
					case col.Name == "Value" && col.JSONPath == ".spec.value":
						haveValue = true
					}
				}
				Expect(haveKey).To(BeTrue(), "expected a 'Key' printer column sourced from .spec.key")
				Expect(haveValue).To(BeTrue(), "expected a 'Value' printer column sourced from .spec.value")
			})
	})
}
