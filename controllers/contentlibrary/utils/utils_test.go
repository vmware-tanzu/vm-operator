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
