// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
)

func TestVirtualMachineImageConversion(t *testing.T) {
	t.Run("VirtualMachineImage hub-spoke", func(t *testing.T) {
		g := NewWithT(t)

		hub := &vmopv1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-image",
				Namespace: "my-namespace",
				Annotations: map[string]string{
					"fizz": "buzz",
				},
			},
			Spec: vmopv1.VirtualMachineImageSpec{
				ProviderRef: &vmopv1common.LocalObjectRef{
					APIVersion: "vmware.com/v1",
					Kind:       "ImageProvider",
					Name:       "my-image",
				},
			},
			Status: vmopv1.VirtualMachineImageStatus{
				VMwareSystemProperties: []vmopv1common.KeyValuePair{
					{
						Key:   sysAnnotationKey("foo"),
						Value: "foo-val",
					},
					{
						Key:   sysAnnotationKey("bar"),
						Value: "bar-val",
					},
				},
			},
		}

		spoke := &vmopv1a1.VirtualMachineImage{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

		g.Expect(spoke.Spec.ProviderRef.APIVersion).To(Equal("vmware.com/v1"))
		g.Expect(spoke.Spec.ProviderRef.Kind).To(Equal("ImageProvider"))
		g.Expect(spoke.Spec.ProviderRef.Name).To(Equal("my-image"))
		g.Expect(spoke.Spec.ProviderRef.Namespace).To(Equal("my-namespace"))

		g.Expect(spoke.Annotations).To(HaveLen(3))
		g.Expect(spoke.Annotations).To(HaveKeyWithValue(sysAnnotationKey("foo"), "foo-val"))
		g.Expect(spoke.Annotations).To(HaveKeyWithValue(sysAnnotationKey("bar"), "bar-val"))
	})

	t.Run("VirtualMachineImage spoke-hub", func(t *testing.T) {
		g := NewWithT(t)

		nextVerCVMI := &vmopv1.ClusterVirtualMachineImage{}
		spoke := &vmopv1a1.ClusterVirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				Annotations: map[string]string{
					sysAnnotationKey("foo"): "foo-val",
					sysAnnotationKey("bar"): "bar-val",
					"fizz":                  "buzz",
				},
			},
		}

		g.Expect(spoke.ConvertTo(nextVerCVMI)).To(Succeed())
		g.Expect(nextVerCVMI.Annotations).To(HaveLen(1))

		props := nextVerCVMI.Status.VMwareSystemProperties
		g.Expect(props).To(HaveLen(2))

		kvMap := map[string]string{}
		for _, prop := range props {
			kvMap[prop.Key] = prop.Value
		}
		g.Expect(kvMap).To(HaveKeyWithValue(sysAnnotationKey("foo"), "foo-val"))
		g.Expect(kvMap).To(HaveKeyWithValue(sysAnnotationKey("bar"), "bar-val"))
	})

	t.Run("VirtualMachineImage spoke-hub Conditions", func(t *testing.T) {
		now := metav1.Now()
		later := metav1.NewTime(now.AddDate(1, 0, 0))

		t.Run("Ready true", func(t *testing.T) {
			g := NewWithT(t)

			spoke := &vmopv1a1.VirtualMachineImage{
				Status: vmopv1a1.VirtualMachineImageStatus{
					Conditions: []vmopv1a1.Condition{
						{
							Type:               vmopv1a1.VirtualMachineImageSyncedCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: now,
						},
						{
							Type:               vmopv1a1.VirtualMachineImageProviderReadyCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: later,
						},
						{
							Type:               vmopv1a1.VirtualMachineImageProviderSecurityComplianceCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: now,
						},
					},
				},
			}

			hub := &vmopv1.VirtualMachineImage{}
			g.Expect(spoke.ConvertTo(hub)).To(Succeed())

			g.Expect(hub.Status.Conditions).To(HaveLen(1))
			c := hub.Status.Conditions[0]
			g.Expect(c.Type).To(Equal(vmopv1.ReadyConditionType))
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
			g.Expect(c.LastTransitionTime).To(Equal(later))
		})

		t.Run("Ready false", func(t *testing.T) {
			g := NewWithT(t)

			spoke := &vmopv1a1.VirtualMachineImage{
				Status: vmopv1a1.VirtualMachineImageStatus{
					Conditions: []vmopv1a1.Condition{
						{
							Type:               vmopv1a1.VirtualMachineImageSyncedCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: now,
						},
						{
							Type:               vmopv1a1.VirtualMachineImageProviderReadyCondition,
							Status:             corev1.ConditionFalse,
							Reason:             "ProviderNotReady",
							Message:            "Message",
							LastTransitionTime: later,
						},
						{
							Type:               vmopv1a1.VirtualMachineImageProviderSecurityComplianceCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: now,
						},
					},
				},
			}

			hub := &vmopv1.VirtualMachineImage{}
			g.Expect(spoke.ConvertTo(hub)).To(Succeed())

			g.Expect(hub.Status.Conditions).To(HaveLen(1))
			c := hub.Status.Conditions[0]
			g.Expect(c.Type).To(Equal(vmopv1.ReadyConditionType))
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal("ProviderNotReady"))
			g.Expect(c.Message).To(Equal("Message"))
			g.Expect(c.LastTransitionTime).To(Equal(later))
		})
	})

	t.Run("VirtualMachineImage hub-spoke Conditions", func(t *testing.T) {
		now := metav1.Now()

		t.Run("Ready true", func(t *testing.T) {
			g := NewWithT(t)

			hub := &vmopv1.VirtualMachineImage{
				Status: vmopv1.VirtualMachineImageStatus{
					Conditions: []metav1.Condition{
						{
							Type:               vmopv1.ReadyConditionType,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
						},
					},
				},
			}

			spoke := &vmopv1a1.VirtualMachineImage{}
			g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

			g.Expect(spoke.Status.Conditions).To(HaveLen(3))
			c := spoke.Status.Conditions[0]
			g.Expect(c.Type).To(Equal(vmopv1a1.VirtualMachineImageProviderSecurityComplianceCondition))
			g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
			g.Expect(c.LastTransitionTime).To(Equal(now))
			c = spoke.Status.Conditions[1]
			g.Expect(c.Type).To(Equal(vmopv1a1.VirtualMachineImageProviderReadyCondition))
			g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
			g.Expect(c.LastTransitionTime).To(Equal(now))
			c = spoke.Status.Conditions[2]
			g.Expect(c.Type).To(Equal(vmopv1a1.VirtualMachineImageSyncedCondition))
			g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
			g.Expect(c.LastTransitionTime).To(Equal(now))
		})

		t.Run("Ready false", func(t *testing.T) {
			g := NewWithT(t)

			hub := &vmopv1.VirtualMachineImage{
				Status: vmopv1.VirtualMachineImageStatus{
					Conditions: []metav1.Condition{
						{
							Type:               vmopv1.ReadyConditionType,
							Status:             metav1.ConditionFalse,
							Reason:             vmopv1.VirtualMachineImageNotSyncedReason,
							LastTransitionTime: now,
						},
					},
				},
			}

			spoke := &vmopv1a1.VirtualMachineImage{}
			g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

			g.Expect(spoke.Status.Conditions).To(HaveLen(3))
			c := spoke.Status.Conditions[0]
			g.Expect(c.Type).To(Equal(vmopv1a1.VirtualMachineImageProviderSecurityComplianceCondition))
			g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
			g.Expect(c.LastTransitionTime).To(Equal(now))
			c = spoke.Status.Conditions[1]
			g.Expect(c.Type).To(Equal(vmopv1a1.VirtualMachineImageProviderReadyCondition))
			g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
			g.Expect(c.LastTransitionTime).To(Equal(now))
			c = spoke.Status.Conditions[2]
			g.Expect(c.Type).To(Equal(vmopv1a1.VirtualMachineImageSyncedCondition))
			g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(c.LastTransitionTime).To(Equal(now))
		})
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
		vmopv1.VMIContentLibRefAnnotation: string(refData),
	}

	t.Run("CVMI hub-spoke sets up content library ref in status", func(t *testing.T) {
		g := NewWithT(t)

		// setting up the annotation is performed by the nextver controllers
		nextVerCVMI := vmopv1.ClusterVirtualMachineImage{ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Annotations: annotations,
		}}

		cvmiAfter := vmopv1a1.ClusterVirtualMachineImage{}
		g.Expect(cvmiAfter.ConvertFrom(&nextVerCVMI)).To(Succeed())
		g.Expect(cvmiAfter.Status.ContentLibraryRef).ToNot(BeNil())
		g.Expect(cvmiAfter.Status.ContentLibraryRef.APIGroup).To(gstruct.PointTo(Equal(apiGroup)))
		g.Expect(cvmiAfter.Status.ContentLibraryRef.Kind).To(Equal("FooKind"))
		g.Expect(cvmiAfter.Status.ContentLibraryRef.Name).To(Equal("foo-ref"))

		t.Run("when conversion annotation is unset", func(t *testing.T) {
			g := NewWithT(t)
			cvmiWithoutLibraryRef := vmopv1.ClusterVirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       vmopv1.VirtualMachineImageSpec{},
				Status:     vmopv1.VirtualMachineImageStatus{},
			}

			cvmiAfter := vmopv1a1.ClusterVirtualMachineImage{}
			g.Expect(cvmiAfter.ConvertFrom(&cvmiWithoutLibraryRef)).To(Succeed())
			g.Expect(cvmiAfter.Status.ContentLibraryRef).To(BeNil())
		})
	})

	t.Run("VMI hub-spoke sets up content library ref in status", func(t *testing.T) {
		g := NewWithT(t)

		// setting up the annotation is performed by the nextver controllers
		nextVerVMI := vmopv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   "default",
			Annotations: annotations,
		}}

		vmiAfter := vmopv1a1.VirtualMachineImage{}
		g.Expect(vmiAfter.ConvertFrom(&nextVerVMI)).To(Succeed())
		g.Expect(vmiAfter.Status.ContentLibraryRef).ToNot(BeNil())
		g.Expect(vmiAfter.Status.ContentLibraryRef.APIGroup).To(gstruct.PointTo(Equal(apiGroup)))
		g.Expect(vmiAfter.Status.ContentLibraryRef.Kind).To(Equal("FooKind"))
		g.Expect(vmiAfter.Status.ContentLibraryRef.Name).To(Equal("foo-ref"))

		t.Run("when conversion annotation is unset", func(t *testing.T) {
			g := NewWithT(t)
			vmiWithoutLibraryRef := vmopv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       vmopv1.VirtualMachineImageSpec{},
				Status:     vmopv1.VirtualMachineImageStatus{},
			}

			vmiAfter := vmopv1a1.VirtualMachineImage{}
			g.Expect(vmiAfter.ConvertFrom(&vmiWithoutLibraryRef)).To(Succeed())
			g.Expect(vmiAfter.Status.ContentLibraryRef).To(BeNil())
		})
	})
}

func sysAnnotationKey(item string) string {
	return fmt.Sprintf("vmware-system-%s", item)
}
