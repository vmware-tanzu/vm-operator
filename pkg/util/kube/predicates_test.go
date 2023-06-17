// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("Predicates", func() {
	Context("VMForControllerPredicate", func() {
		var (
			client                  ctrlclient.Client
			log                     logr.Logger
			vm                      *unstructured.Unstructured
			vmClass                 *unstructured.Unstructured
			vmClassControllerName   string
			predicateControllerName string
			opts                    kubeutil.VMForControllerPredicateOptions
			predicate               ctrlpredicate.Predicate
		)

		BeforeEach(func() {
			vm = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vmoperator.vmware.com/v1alpha1",
					"kind":       "VirtualMachine",
					"metadata": map[string]interface{}{
						"name":      "my-vm",
						"namespace": "my-namespace",
					},
					"spec": map[string]interface{}{
						"className": "my-vmclass",
					},
				},
			}
			vmClass = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vmoperator.vmware.com/v1alpha1",
					"kind":       "VirtualMachineClass",
					"metadata": map[string]interface{}{
						"name": "my-vmclass",
					},
					"spec": map[string]interface{}{},
				},
			}
			log = getLogger()
		})
		JustBeforeEach(func() {
			if vmClassControllerName != "" {
				Expect(unstructured.SetNestedField(
					vmClass.Object,
					vmClassControllerName,
					"spec", "controllerName")).To(Succeed())
			}
			withObjects := []ctrlclient.Object{}
			if vm != nil {
				withObjects = append(withObjects, vm)
			}
			if vmClass != nil {
				withObjects = append(withObjects, vmClass)
			}
			client = fake.NewClientBuilder().WithObjects(withObjects...).Build()
			predicate = kubeutil.VMForControllerPredicate(
				client, log, predicateControllerName, opts)
		})

		assertEvents := func(
			newObj, oldObj ctrlclient.Object,
			create, update, delete, generic bool) {

			By("CreateEvent")
			ExpectWithOffset(1, predicate.Create(
				event.CreateEvent{Object: newObj})).To(Equal(create))

			By("UpdateEvent")
			ExpectWithOffset(1, predicate.Update(
				event.UpdateEvent{ObjectNew: newObj, ObjectOld: oldObj})).To(Equal(update))

			By("DeleteEvent")
			ExpectWithOffset(1, predicate.Delete(
				event.DeleteEvent{Object: newObj})).To(Equal(delete))

			By("GenericEvent")
			ExpectWithOffset(1, predicate.Generic(
				event.GenericEvent{Object: newObj})).To(Equal(generic))
		}

		When("the vm class does not exist", func() {
			BeforeEach(func() {
				vmClass = nil
			})
			When("opts.MatchIfVMClassNotFound is false", func() {
				BeforeEach(func() {
					opts.MatchIfVMClassNotFound = false
				})
				It("should return false", func() {
					assertEvents(vm, vm, false, false, false, false)
				})
			})
			When("opts.MatchIfVMClassNotFound is true", func() {
				BeforeEach(func() {
					opts.MatchIfVMClassNotFound = true
				})
				It("should return true", func() {
					assertEvents(vm, vm, true, true, true, true)
				})
			})
		})

		When("the vm class's spec.controllerName field is undefined", func() {
			When("opts.MatchIfControllerNameFieldMissing is false", func() {
				BeforeEach(func() {
					opts.MatchIfControllerNameFieldMissing = false
				})
				It("should return false", func() {
					assertEvents(vm, vm, false, false, false, false)
				})
			})
			When("opts.MatchIfControllerNameFieldMissing is true", func() {
				BeforeEach(func() {
					opts.MatchIfControllerNameFieldMissing = true
				})
				It("should return true", func() {
					assertEvents(vm, vm, true, true, true, true)
				})
			})
		})

		When("the vm class's spec.controllerName field is empty", func() {
			BeforeEach(func() {
				Expect(unstructured.SetNestedField(
					vmClass.Object, "",
					"spec", "controllerName")).To(Succeed())
			})
			When("opts.MatchIfControllerNameFieldEmpty is false", func() {
				BeforeEach(func() {
					opts.MatchIfControllerNameFieldEmpty = false
				})
				It("should return false", func() {
					assertEvents(vm, vm, false, false, false, false)
				})
			})
			When("opts.MatchIfControllerNameFieldEmpty is true", func() {
				BeforeEach(func() {
					opts.MatchIfControllerNameFieldEmpty = true
				})
				It("should return true", func() {
					assertEvents(vm, vm, true, true, true, true)
				})
			})
		})

		When("the vm class's spec.controllerName field is non-empty", func() {
			BeforeEach(func() {
				vmClassControllerName = "my-controller"
			})
			When("the predicate's controllerName is empty", func() {
				BeforeEach(func() {
					predicateControllerName = ""
				})
				It("should return false", func() {
					assertEvents(vm, vm, false, false, false, false)
				})
			})
			When("the predicate's controllerName matches", func() {
				BeforeEach(func() {
					predicateControllerName = vmClassControllerName
				})
				It("should return true", func() {
					assertEvents(vm, vm, true, true, true, true)
				})
			})
			When("the predicate's controllerName does not match", func() {
				BeforeEach(func() {
					predicateControllerName = "not-" + vmClassControllerName
				})
				It("should return false", func() {
					assertEvents(vm, vm, false, false, false, false)
				})
			})
		})
	})
})
