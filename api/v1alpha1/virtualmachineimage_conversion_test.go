// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	nextver "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	nextver_common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
)

func TestVirtualMachineImageConversion(t *testing.T) {
	g := NewWithT(t)

	t.Run("VirtualMachineImage hub-spoke-hub", func(t *testing.T) {
		hub := &nextver.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-image",
				Namespace: "my-namespace",
			},
			Spec: nextver.VirtualMachineImageSpec{
				ProviderRef: nextver_common.LocalObjectRef{
					APIVersion: "vmware.com/v1",
					Kind:       "ImageProvider",
					Name:       "my-image",
				},
			},
		}

		spoke := &v1alpha1.VirtualMachineImage{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

		g.Expect(spoke.Spec.ProviderRef.APIVersion).To(Equal("vmware.com/v1"))
		g.Expect(spoke.Spec.ProviderRef.Kind).To(Equal("ImageProvider"))
		g.Expect(spoke.Spec.ProviderRef.Name).To(Equal("my-image"))
		g.Expect(spoke.Spec.ProviderRef.Namespace).To(Equal("my-namespace"))
	})
}

func Test_Status_ContentLibraryRef(t *testing.T) {
	apiGroup := "foo.bar.com/v1"
	refData, err := json.Marshal(corev1.TypedLocalObjectReference{
		APIGroup: &apiGroup,
		Kind:     "FooKind",
		Name:     "foo-ref",
	})
	NewWithT(t).Expect(err).ToNot(HaveOccurred())
	annotations := map[string]string{
		nextver.VMIContentLibRefAnnotation: string(refData),
	}

	t.Run("CVMI hub-spoke sets up content library ref in status", func(t *testing.T) {
		g := NewWithT(t)

		// setting up the annotation is performed by the v1a2 controllers
		nextVerCVMI := nextver.ClusterVirtualMachineImage{ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Annotations: annotations,
		}}

		cvmiAfter := v1alpha1.ClusterVirtualMachineImage{}
		g.Expect(cvmiAfter.ConvertFrom(&nextVerCVMI)).To(Succeed())
		g.Expect(cvmiAfter.Status.ContentLibraryRef).ToNot(BeNil())
		g.Expect(cvmiAfter.Status.ContentLibraryRef.APIGroup).To(gstruct.PointTo(Equal(apiGroup)))
		g.Expect(cvmiAfter.Status.ContentLibraryRef.Kind).To(Equal("FooKind"))
		g.Expect(cvmiAfter.Status.ContentLibraryRef.Name).To(Equal("foo-ref"))

		t.Run("when conversion annotation is unset", func(t *testing.T) {
			g := NewWithT(t)
			cvmiWithoutLibraryRef := nextver.ClusterVirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       nextver.VirtualMachineImageSpec{},
				Status:     nextver.VirtualMachineImageStatus{},
			}

			cvmiAfter := v1alpha1.ClusterVirtualMachineImage{}
			g.Expect(cvmiAfter.ConvertFrom(&cvmiWithoutLibraryRef)).To(Succeed())
			g.Expect(cvmiAfter.Status.ContentLibraryRef).To(BeNil())
		})
	})

	t.Run("VMI hub-spoke sets up content library ref in status", func(t *testing.T) {
		g := NewWithT(t)

		// setting up the annotation is performed by the v1a2 controllers
		nextVerVMI := nextver.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   "default",
			Annotations: annotations,
		}}

		vmiAfter := v1alpha1.VirtualMachineImage{}
		g.Expect(vmiAfter.ConvertFrom(&nextVerVMI)).To(Succeed())
		g.Expect(vmiAfter.Status.ContentLibraryRef).ToNot(BeNil())
		g.Expect(vmiAfter.Status.ContentLibraryRef.APIGroup).To(gstruct.PointTo(Equal(apiGroup)))
		g.Expect(vmiAfter.Status.ContentLibraryRef.Kind).To(Equal("FooKind"))
		g.Expect(vmiAfter.Status.ContentLibraryRef.Name).To(Equal("foo-ref"))

		t.Run("when conversion annotation is unset", func(t *testing.T) {
			g := NewWithT(t)
			vmiWithoutLibraryRef := nextver.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       nextver.VirtualMachineImageSpec{},
				Status:     nextver.VirtualMachineImageStatus{},
			}

			vmiAfter := v1alpha1.VirtualMachineImage{}
			g.Expect(vmiAfter.ConvertFrom(&vmiWithoutLibraryRef)).To(Succeed())
			g.Expect(vmiAfter.Status.ContentLibraryRef).To(BeNil())
		})
	})
}
