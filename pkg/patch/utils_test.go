/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package patch

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func unitTests() {
	Context("ToUnstructured", func() {

		Context("with a typed object", func() {
			It("should succeed", func() {
				// Test with a typed object.
				obj := &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster-1",
						Namespace: "namespace-1",
					},
					Spec: vmopv1.VirtualMachineSpec{
						ImageName: "foo",
					},
				}
				newObj, err := toUnstructured(obj)
				Expect(err).ToNot(HaveOccurred())
				Expect(newObj.GetName()).To(Equal(obj.Name))
				Expect(newObj.GetNamespace()).To(Equal(obj.Namespace))

				// Change a spec field and validate that it stays the same in the incoming object.
				Expect(unstructured.SetNestedField(newObj.Object, false, "spec", "paused")).To(Succeed())
				Expect(obj.Spec.ImageName).To(Equal("foo"))
			})
		})
	})

	Context("with an unstructured object", func() {
		It("should succeed", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "test.x.y.z/v1",
					"metadata": map[string]interface{}{
						"name":      "test-1",
						"namespace": "namespace-1",
					},
					"spec": map[string]interface{}{
						"paused": true,
					},
				},
			}

			newObj, err := toUnstructured(obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(newObj.GetName()).To(Equal(obj.GetName()))
			Expect(newObj.GetNamespace()).To(Equal(obj.GetNamespace()))

			// Validate that the maps point to different addresses.
			Expect(obj.Object).ToNot(BeIdenticalTo(newObj.Object))

			// Change a spec field and validate that it stays the same in the incoming object.
			Expect(unstructured.SetNestedField(newObj.Object, false, "spec", "paused")).To(Succeed())
			pausedValue, _, err := unstructured.NestedBool(obj.Object, "spec", "paused")
			Expect(err).ToNot(HaveOccurred())
			Expect(pausedValue).To(BeTrue())

			// Change the name of the new object and make sure it doesn't change it the old one.
			newObj.SetName("test-2")
			Expect(obj.GetName()).To(Equal("test-1"))
		})
	})

	Context("UnsafeFocusedUnstructured", func() {

		Context("focus=spec", func() {

			It("should only return spec and common fields", func() {
				obj := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test.x.y.z/v1",
						"kind":       "TestCluster",
						"metadata": map[string]interface{}{
							"name":      "test-1",
							"namespace": "namespace-1",
						},
						"spec": map[string]interface{}{
							"paused": true,
						},
						"status": map[string]interface{}{
							"infrastructureReady": true,
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Ready",
									"status": "True",
								},
							},
						},
					},
				}

				newObj := unsafeUnstructuredCopy(obj, specPatch, true)

				// Validate that common fields are always preserved.
				Expect(newObj.Object["apiVersion"]).To(Equal(obj.Object["apiVersion"]))
				Expect(newObj.Object["kind"]).To(Equal(obj.Object["kind"]))
				Expect(newObj.Object["metadata"]).To(Equal(obj.Object["metadata"]))

				// Validate that the spec has been preserved.
				Expect(newObj.Object["spec"]).To(Equal(obj.Object["spec"]))

				// Validate that the status is nil, but preserved in the original object.
				Expect(newObj.Object["status"]).To(BeNil())
				Expect(obj.Object["status"]).ToNot(BeNil())
				Expect(obj.Object["status"].(map[string]interface{})["conditions"]).ToNot(BeNil())
			})
		})

		Context("focus=status w/ condition-setter object", func() {

			It("should only return status (without conditions) and common fields", func() {
				obj := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test.x.y.z/v1",
						"kind":       "TestCluster",
						"metadata": map[string]interface{}{
							"name":      "test-1",
							"namespace": "namespace-1",
						},
						"spec": map[string]interface{}{
							"paused": true,
						},
						"status": map[string]interface{}{
							"infrastructureReady": true,
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Ready",
									"status": "True",
								},
							},
						},
					},
				}

				newObj := unsafeUnstructuredCopy(obj, statusPatch, true)

				// Validate that common fields are always preserved.
				Expect(newObj.Object["apiVersion"]).To(Equal(obj.Object["apiVersion"]))
				Expect(newObj.Object["kind"]).To(Equal(obj.Object["kind"]))
				Expect(newObj.Object["metadata"]).To(Equal(obj.Object["metadata"]))

				// Validate that spec is nil in the new object, but still exists in the old copy.
				Expect(newObj.Object["spec"]).To(BeNil())
				Expect(obj.Object["spec"]).To(Equal(map[string]interface{}{
					"paused": true,
				}))

				// Validate that the status has been copied, without conditions.
				Expect(newObj.Object["status"]).To(HaveLen(1))
				Expect(newObj.Object["status"].(map[string]interface{})["infrastructureReady"]).To(Equal(true))
				Expect(newObj.Object["status"].(map[string]interface{})["conditions"]).To(BeNil())

				// When working with conditions, the inner map is going to be removed from the original object.
				Expect(obj.Object["status"].(map[string]interface{})["conditions"]).To(BeNil())
			})
		})

		Context("focus=status w/o condition-setter object", func() {

			It("should only return status and common fields", func() {
				obj := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test.x.y.z/v1",
						"kind":       "TestCluster",
						"metadata": map[string]interface{}{
							"name":      "test-1",
							"namespace": "namespace-1",
						},
						"spec": map[string]interface{}{
							"paused": true,
							"other":  "field",
						},
						"status": map[string]interface{}{
							"infrastructureReady": true,
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Ready",
									"status": "True",
								},
							},
						},
					},
				}

				newObj := unsafeUnstructuredCopy(obj, statusPatch, false)

				// Validate that spec is nil in the new object, but still exists in the old copy.
				Expect(newObj.Object["spec"]).To(BeNil())
				Expect(obj.Object["spec"]).To(Equal(map[string]interface{}{
					"paused": true,
					"other":  "field",
				}))

				// Validate that common fields are always preserved.
				Expect(newObj.Object["apiVersion"]).To(Equal(obj.Object["apiVersion"]))
				Expect(newObj.Object["kind"]).To(Equal(obj.Object["kind"]))
				Expect(newObj.Object["metadata"]).To(Equal(obj.Object["metadata"]))

				// Validate that the status has been copied, without conditions.
				Expect(newObj.Object["status"]).To(HaveLen(2))
				Expect(newObj.Object["status"]).To(Equal(obj.Object["status"]))

				// Make sure that we didn't modify the incoming object if this object isn't a condition setter.
				Expect(obj.Object["status"].(map[string]interface{})["conditions"]).ToNot(BeNil())
			})
		})
	})
}
