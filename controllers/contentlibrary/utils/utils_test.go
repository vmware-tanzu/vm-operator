// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
)

const fake = "fake"

func TestDummyClusterContentLibraryItem(t *testing.T) {
	g := NewWithT(t)
	obj := utils.DummyClusterContentLibraryItem(fake)
	g.Expect(obj).ToNot(BeNil())
	g.Expect(obj.Name).To(Equal(fake))
	g.Expect(obj.Namespace).To(BeEmpty())
}

func TestDummyContentLibraryItem(t *testing.T) {
	g := NewWithT(t)
	obj := utils.DummyContentLibraryItem(fake, fake+fake)
	g.Expect(obj).ToNot(BeNil())
	g.Expect(obj.Name).To(Equal(fake))
	g.Expect(obj.Namespace).To(Equal(fake + fake))
}

func TestFilterServicesTypeLabels(t *testing.T) {
	filtered := utils.FilterServicesTypeLabels(map[string]string{
		"hello":                     "world",
		"type.services.vmware.com/": "bar"})
	g := NewWithT(t)
	g.Expect(filtered).To(HaveLen(1))
	g.Expect(filtered).To(HaveKeyWithValue("type.services.vmware.com/", ""))
}

func TestIsItemReady(t *testing.T) {
	g := NewWithT(t)
	g.Expect(utils.IsItemReady(nil)).To(BeFalse())
	g.Expect(utils.IsItemReady(imgregv1a1.Conditions{})).To(BeFalse())
	g.Expect(utils.IsItemReady(imgregv1a1.Conditions{
		{
			Type: imgregv1a1.ConditionType(fake),
		},
	})).To(BeFalse())
	g.Expect(utils.IsItemReady(imgregv1a1.Conditions{
		{
			Type: imgregv1a1.ReadyCondition,
		},
	})).To(BeFalse())
	g.Expect(utils.IsItemReady(imgregv1a1.Conditions{
		{
			Type:   imgregv1a1.ReadyCondition,
			Status: corev1.ConditionFalse,
		},
	})).To(BeFalse())
	g.Expect(utils.IsItemReady(imgregv1a1.Conditions{
		{
			Type:   imgregv1a1.ReadyCondition,
			Status: corev1.ConditionTrue,
		},
	})).To(BeTrue())
}

func TestGetImageFieldNameFromItem(t *testing.T) {
	testCases := []struct {
		name        string
		in          string
		expectedErr string
		expectedOut string
	}{
		{
			name:        "missing prefix",
			in:          "123",
			expectedErr: "item name does not start with \"clitem\"",
		},
		{
			name:        "missing identifier",
			in:          "clitem",
			expectedErr: "item name does not have an identifier after clitem-",
		},
		{
			name:        "valid",
			in:          "clitem-123",
			expectedOut: "vmi-123",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			out, err := utils.GetImageFieldNameFromItem(tc.in)
			if tc.expectedErr != "" {
				g.Expect(err).To(MatchError(tc.expectedErr))
				g.Expect(out).To(BeEmpty())
			} else {
				g.Expect(out).To(Equal(tc.expectedOut))
			}
		})
	}
}

func Test_AddContentLibRefToAnnotation(t *testing.T) {
	obj := &vmopv1.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{},
	}

	t.Run("when the CL ref is missing", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(utils.AddContentLibraryRefToAnnotation(obj, nil)).To(Succeed())
	})

	t.Run("with a valid CL ref", func(t *testing.T) {
		g := NewWithT(t)
		ref := &imgregv1a1.NameAndKindRef{
			Kind: "FooKind",
			Name: "foo",
		}

		t.Run("annotation gets correctly set", func(t *testing.T) {
			g.Expect(utils.AddContentLibraryRefToAnnotation(obj, ref)).To(Succeed())
			g.Expect(obj.Annotations).To(HaveLen(1))
			assertAnnotation(g, obj)
		})

		t.Run("the new annotation does not override existing annotations", func(t *testing.T) {
			obj.Annotations = map[string]string{"bar": "baz"}
			g.Expect(utils.AddContentLibraryRefToAnnotation(obj, ref)).To(Succeed())
			g.Expect(len(obj.Annotations)).To(BeNumerically(">=", 1))
			assertAnnotation(g, obj)
		})

	})
}

func assertAnnotation(g *WithT, obj client.Object) {
	g.Expect(obj.GetAnnotations()).To(HaveKey(vmopv1.VMIContentLibRefAnnotation))

	val := obj.GetAnnotations()[vmopv1.VMIContentLibRefAnnotation]
	coreRef := corev1.TypedLocalObjectReference{}
	g.Expect(json.Unmarshal([]byte(val), &coreRef)).To(Succeed())

	g.Expect(coreRef.APIGroup).To(gstruct.PointTo(Equal(imgregv1a1.GroupVersion.Group)))
	g.Expect(coreRef.Kind).To(Equal("FooKind"))
	g.Expect(coreRef.Name).To(Equal("foo"))
}
