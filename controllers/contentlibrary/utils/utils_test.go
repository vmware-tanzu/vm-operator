// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
)

const fake = "fake"

var _ = Describe("DummyClusterContentLibraryItem", func() {
	It("should succeed", func() {
		obj := utils.DummyClusterContentLibraryItem(fake)
		Expect(obj).ToNot(BeNil())
		Expect(obj.Name).To(Equal(fake))
		Expect(obj.Namespace).To(BeEmpty())
	})
})

var _ = Describe("DummyContentLibraryItem", func() {
	It("should succeed", func() {
		obj := utils.DummyContentLibraryItem(fake, fake+fake)
		Expect(obj).ToNot(BeNil())
		Expect(obj.Name).To(Equal(fake))
		Expect(obj.Namespace).To(Equal(fake + fake))
	})
})

var _ = Describe("FilterServicesTypeLabels", func() {
	It("should succeed", func() {
		filtered := utils.FilterServicesTypeLabels(map[string]string{
			"hello":                     "world",
			"type.services.vmware.com/": "bar"})
		Expect(filtered).To(HaveLen(1))
		Expect(filtered).To(HaveKeyWithValue("type.services.vmware.com/", ""))
	})
})

var _ = Describe("IsItemReady", func() {
	It("should succeed", func() {
		Expect(utils.IsItemReady(nil)).To(BeFalse())
		Expect(utils.IsItemReady(imgregv1a1.Conditions{})).To(BeFalse())
		Expect(utils.IsItemReady(imgregv1a1.Conditions{
			{
				Type: imgregv1a1.ConditionType(fake),
			},
		})).To(BeFalse())
		Expect(utils.IsItemReady(imgregv1a1.Conditions{
			{
				Type: imgregv1a1.ReadyCondition,
			},
		})).To(BeFalse())
		Expect(utils.IsItemReady(imgregv1a1.Conditions{
			{
				Type:   imgregv1a1.ReadyCondition,
				Status: corev1.ConditionFalse,
			},
		})).To(BeFalse())
		Expect(utils.IsItemReady(imgregv1a1.Conditions{
			{
				Type:   imgregv1a1.ReadyCondition,
				Status: corev1.ConditionTrue,
			},
		})).To(BeTrue())
	})
})

var _ = DescribeTable("GetImageFieldNameFromItem",
	func(in, expOut, expErr string) {
		out, err := utils.GetImageFieldNameFromItem(in)
		if expErr != "" {
			Expect(err).To(MatchError(expErr))
			Expect(out).To(BeEmpty())
		} else {
			Expect(out).To(Equal(expOut))
		}
	},
	Entry("missing prefix", "123", "", "item name does not start with \"clitem\""),
	Entry("missing identifier", "clitem", "", "item name does not have an identifier after clitem-"),
	Entry("valid", "clitem-123", "vmi-123", ""),
)

var _ = Describe("AddContentLibRefToAnnotation", func() {

	var obj client.Object

	BeforeEach(func() {
		obj = &vmopv1.ClusterVirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{},
		}
	})

	When("the CL ref is missing", func() {
		Expect(utils.AddContentLibraryRefToAnnotation(obj, nil)).To(Succeed())
	})

	When("there is a valid CL ref", func() {
		ref := &imgregv1a1.NameAndKindRef{
			Kind: "FooKind",
			Name: "foo",
		}

		When("annotation gets correctly set", func() {
			It("should have expected result", func() {
				Expect(utils.AddContentLibraryRefToAnnotation(obj, ref)).To(Succeed())
				Expect(obj.GetAnnotations()).To(HaveLen(1))
				assertAnnotation(obj)
			})
		})

		When("the new annotation does not override existing annotations", func() {
			It("should have expected result", func() {
				obj.SetAnnotations(map[string]string{"bar": "baz"})
				Expect(utils.AddContentLibraryRefToAnnotation(obj, ref)).To(Succeed())
				Expect(len(obj.GetAnnotations())).To(BeNumerically(">=", 1))
				assertAnnotation(obj)
			})
		})

	})
})

func assertAnnotation(obj client.Object) {
	ExpectWithOffset(1, obj.GetAnnotations()).To(HaveKey(vmopv1.VMIContentLibRefAnnotation))

	val := obj.GetAnnotations()[vmopv1.VMIContentLibRefAnnotation]
	coreRef := corev1.TypedLocalObjectReference{}
	ExpectWithOffset(1, json.Unmarshal([]byte(val), &coreRef)).To(Succeed())

	ExpectWithOffset(1, coreRef.APIGroup).To(gstruct.PointTo(Equal(imgregv1a1.GroupVersion.Group)))
	ExpectWithOffset(1, coreRef.Kind).To(Equal("FooKind"))
	ExpectWithOffset(1, coreRef.Name).To(Equal("foo"))
}
