/*
Copyright 2017 The Kubernetes Authors.

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

package patch2

import (
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Describe("Patch Helper", func() {
		It("Should patch an unstructured object", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "VirtualMachine",
					"apiVersion": "vmoperator.vmware.com/v1alpha1",
					"metadata": map[string]interface{}{
						"generateName": "test-bootstrap-",
						"namespace":    "default",
					},
				},
			}

			Context("adding an owner reference, preserving its status", func() {
				obj := obj.DeepCopy()

				By("Creating the unstructured object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()
				obj.Object["status"] = map[string]interface{}{
					"ready": true,
				}
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Modifying the OwnerReferences")
				refs := []metav1.OwnerReference{
					{
						APIVersion: "cluster.x-k8s.io/v1alpha3",
						Kind:       "Cluster",
						Name:       "test",
						UID:        types.UID("fake-uid"),
					},
				}
				obj.SetOwnerReferences(refs)

				By("Patching the unstructured object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
						return false
					}

					return reflect.DeepEqual(obj.GetOwnerReferences(), objAfter.GetOwnerReferences())
				}, timeout).Should(BeTrue())
			})
		})

		Describe("Should patch conditions", func() {
			Specify("on a corev1.Node object", func() {
				conditionTime := metav1.Date(2015, 1, 1, 12, 0, 0, 0, metav1.Now().Location())

				obj := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "node-patch-test-",
						Annotations: map[string]string{
							"test": "1",
						},
					},
				}

				By("Creating a Node object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.GetName()}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Appending a new condition")
				condition := corev1.NodeCondition{
					Type:               "CustomCondition",
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  conditionTime,
					LastTransitionTime: conditionTime,
					Reason:             "reason",
					Message:            "message",
				}
				obj.Status.Conditions = append(obj.Status.Conditions, condition)

				By("Patching the Node")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					Expect(ctx.Client.Get(ctx, key, objAfter)).To(Succeed())

					ok, _ := ContainElement(condition).Match(objAfter.Status.Conditions)
					return ok
				}, timeout).Should(BeTrue())
			})

			Describe("on a vmopv1.VirtualMachine object", func() {
				obj := &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-",
						Namespace:    "default",
					},
					Spec: vmopv1.VirtualMachineSpec{
						PowerState: vmopv1.VirtualMachinePowerStateOn,
					},
				}

				Specify("should mark it ready", func() {
					obj := obj.DeepCopy()

					By("Creating the object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
					}()

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, ctx.Client)
					Expect(err).NotTo(HaveOccurred())

					By("Marking Ready=True")
					conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj)).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
							return false
						}
						return cmp.Equal(obj.Status.Conditions, objAfter.Status.Conditions)
					}, timeout).Should(BeTrue())
				})

				Specify("should recover if there is a resolvable conflict", func() {
					obj := obj.DeepCopy()

					By("Creating the object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
					}()
					objCopy := obj.DeepCopy()

					By("Marking a custom condition to be false")
					conditions.MarkFalse(objCopy, "TestCondition", "reason", "message")
					Expect(ctx.Client.Status().Update(ctx, objCopy)).To(Succeed())

					By("Validating that the local object's resource version is behind")
					Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, ctx.Client)
					Expect(err).NotTo(HaveOccurred())

					By("Marking Ready=True")
					conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj)).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
							return false
						}

						testConditionCopy := conditions.Get(objCopy, "TestCondition")
						testConditionAfter := conditions.Get(objAfter, "TestCondition")

						readyBefore := conditions.Get(obj, vmopv1.ReadyConditionType)
						readyAfter := conditions.Get(objAfter, vmopv1.ReadyConditionType)

						return cmp.Equal(testConditionCopy, testConditionAfter) && cmp.Equal(readyBefore, readyAfter)
					}, timeout).Should(BeTrue())
				})

				Specify("should recover if there is a resolvable conflict, incl. patch spec and status", func() {
					obj := obj.DeepCopy()

					By("Creating the object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
					}()
					objCopy := obj.DeepCopy()

					By("Marking a custom condition to be false")
					conditions.MarkFalse(objCopy, "TestCondition", "reason", "message")
					Expect(ctx.Client.Status().Update(ctx, objCopy)).To(Succeed())

					By("Validating that the local object's resource version is behind")
					Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, ctx.Client)
					Expect(err).NotTo(HaveOccurred())

					By("Changing the object spec, status, and adding Ready=True condition")
					obj.Spec.ImageName = "foo-image"
					conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj)).To(Succeed())

					By("Validating the object has been updated")
					objAfter := obj.DeepCopy()
					Eventually(func() bool {
						if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
							return false
						}

						testConditionCopy := conditions.Get(objCopy, "TestCondition")
						testConditionAfter := conditions.Get(objAfter, "TestCondition")

						readyBefore := conditions.Get(obj, vmopv1.ReadyConditionType)
						readyAfter := conditions.Get(objAfter, vmopv1.ReadyConditionType)

						return cmp.Equal(testConditionCopy, testConditionAfter) && cmp.Equal(readyBefore, readyAfter) &&
							obj.Spec.ImageName == objAfter.Spec.ImageName
					}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
				})

				Specify("should return an error if there is an unresolvable conflict", func() {
					obj := obj.DeepCopy()

					By("Creating the object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
					}()
					objCopy := obj.DeepCopy()

					By("Marking a custom condition to be false")
					conditions.MarkFalse(objCopy, vmopv1.ReadyConditionType, "reason", "message")
					Expect(ctx.Client.Status().Update(ctx, objCopy)).To(Succeed())

					By("Validating that the local object's resource version is behind")
					Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, ctx.Client)
					Expect(err).NotTo(HaveOccurred())

					By("Marking Ready=True")
					conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj)).ToNot(Succeed())

					By("Validating the object has not been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
							return false
						}
						ok, _ := ContainElement(objCopy.Status.Conditions[0]).Match(objAfter.Status.Conditions)
						return ok
					}, timeout).Should(BeTrue())
				})

				Specify("should not return an error if there is an unresolvable conflict but the conditions is owned by the controller", func() {
					obj := obj.DeepCopy()

					By("Creating the object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
					}()
					objCopy := obj.DeepCopy()

					By("Marking a custom condition to be false")
					conditions.MarkFalse(objCopy, vmopv1.ReadyConditionType, "reason", "message")
					Expect(ctx.Client.Status().Update(ctx, objCopy)).To(Succeed())

					By("Validating that the local object's resource version is behind")
					Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, ctx.Client)
					Expect(err).NotTo(HaveOccurred())

					By("Marking Ready=True")
					conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj, WithOwnedConditions{Conditions: []string{vmopv1.ReadyConditionType}})).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
							return false
						}

						readyBefore := conditions.Get(obj, vmopv1.ReadyConditionType)
						readyAfter := conditions.Get(objAfter, vmopv1.ReadyConditionType)

						return cmp.Equal(readyBefore, readyAfter)
					}, timeout).Should(BeTrue())
				})

				Specify("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func() {
					obj := obj.DeepCopy()

					By("Creating the object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
					}()
					objCopy := obj.DeepCopy()

					By("Marking a custom condition to be false")
					conditions.MarkFalse(objCopy, vmopv1.ReadyConditionType, "reason", "message")
					Expect(ctx.Client.Status().Update(ctx, objCopy)).To(Succeed())

					By("Validating that the local object's resource version is behind")
					Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, ctx.Client)
					Expect(err).NotTo(HaveOccurred())

					By("Marking Ready=True")
					conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
							return false
						}

						readyBefore := conditions.Get(obj, vmopv1.ReadyConditionType)
						readyAfter := conditions.Get(objAfter, vmopv1.ReadyConditionType)

						return cmp.Equal(readyBefore, readyAfter)
					}, timeout).Should(BeTrue())
				})
			})
		})

		Describe("Should patch a vmopv1.VirtualMachine", func() {
			var obj *vmopv1.VirtualMachine

			BeforeEach(func() {
				obj = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-",
						Namespace:    ctx.Namespace,
					},
					Spec: vmopv1.VirtualMachineSpec{
						PowerState: vmopv1.VirtualMachinePowerStateOn,
					},
					Status: vmopv1.VirtualMachineStatus{
						BiosUUID:     "bios-uuid-foo",
						InstanceUUID: "instance-uuid-foo",
						UniqueID:     "unique-id",
						PowerState:   vmopv1.VirtualMachinePowerStateOn,
					},
				}
			})

			Specify("add a finalizers", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Adding a finalizer")
				obj.Finalizers = append(obj.Finalizers, "vm-finalizer")

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
						return false
					}

					return reflect.DeepEqual(obj.Finalizers, objAfter.Finalizers)
				}, timeout).Should(BeTrue())
			})

			Specify("removing finalizers", func() {
				obj := obj.DeepCopy()
				obj.Finalizers = append(obj.Finalizers, "vm-finalizer")

				By("Creating the object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Removing the finalizers")
				obj.SetFinalizers(nil)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
						return false
					}

					return len(objAfter.Finalizers) == 0
				}, timeout).Should(BeTrue())
			})

			Specify("updating spec", func() {
				obj := obj.DeepCopy()
				obj.ObjectMeta.Namespace = ctx.Namespace

				By("Creating the object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Updating the object spec")
				obj.Spec.ImageName = "image-name-42"

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
						return false
					}

					return obj.Spec.ImageName == objAfter.Spec.ImageName
				}, timeout).Should(BeTrue())
			})

			Specify("updating status", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Updating the object status")
				obj.Status.Host = "vm-host"

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
						return false
					}
					return reflect.DeepEqual(objAfter.Status, obj.Status)
				}, timeout).Should(BeTrue())
			})

			Specify("updating both spec, status, and adding a condition", func() {
				obj := obj.DeepCopy()
				obj.ObjectMeta.Namespace = ctx.Namespace

				By("Creating the object")
				Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, ctx.Client)
				Expect(err).NotTo(HaveOccurred())

				By("Updating the object spec")
				obj.Spec.ImageName = "image-name"

				By("Updating the object status")
				obj.Status.Host = "vm-host"

				By("Setting Ready condition")
				conditions.MarkTrue(obj, vmopv1.ReadyConditionType)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := ctx.Client.Get(ctx, key, objAfter); err != nil {
						return false
					}

					return obj.Status.Host == objAfter.Status.Host &&
						conditions.IsTrue(objAfter, vmopv1.ReadyConditionType) &&
						reflect.DeepEqual(obj.Spec, objAfter.Spec)
				}, timeout).Should(BeTrue())
			})
		})

		/*
			It("Should update Status.ObservedGeneration when using WithStatusObservedGeneration option", func() {
				obj := &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-ms",
						Namespace:    "test-namespace",
					},
					Spec: clusterv1.MachineSetSpec{
						ClusterName: "test1",
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								ClusterName: "test1",
							},
						},
					},
				}

				Context("when updating spec", func() {
					obj := obj.DeepCopy()

					By("Creating the MachineSet object")
					Expect(ctx.Client.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(testEnv.Delete(ctx, obj)).To(Succeed())
					}()

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, testEnv)
					Expect(err).NotTo(HaveOccurred())

					By("Updating the object spec")
					obj.Spec.Replicas = pointer.Int32(10)

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := testEnv.Get(ctx, key, objAfter); err != nil {
							return false
						}

						return reflect.DeepEqual(obj.Spec, objAfter.Spec) &&
							obj.GetGeneration() == objAfter.Status.ObservedGeneration
					}, timeout).Should(BeTrue())
				})

				Context("when updating spec, status, and metadata", func() {
					obj := obj.DeepCopy()

					By("Creating the MachineSet object")
					Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(testEnv.Delete(ctx, obj)).To(Succeed())
					}()

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, testEnv)
					Expect(err).NotTo(HaveOccurred())

					By("Updating the object spec")
					obj.Spec.Replicas = pointer.Int32(10)

					By("Updating the object status")
					obj.Status.AvailableReplicas = 6
					obj.Status.ReadyReplicas = 6

					By("Updating the object metadata")
					obj.ObjectMeta.Annotations = map[string]string{
						"test1": "annotation",
					}

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := testEnv.Get(ctx, key, objAfter); err != nil {
							return false
						}

						return reflect.DeepEqual(obj.Spec, objAfter.Spec) &&
							reflect.DeepEqual(obj.Status, objAfter.Status) &&
							obj.GetGeneration() == objAfter.Status.ObservedGeneration
					}, timeout).Should(BeTrue())
				})

				Context("without any changes", func() {
					obj := obj.DeepCopy()

					By("Creating the MachineSet object")
					Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
					key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
					defer func() {
						Expect(testEnv.Delete(ctx, obj)).To(Succeed())
					}()
					obj.Status.ObservedGeneration = obj.GetGeneration()
					lastGeneration := obj.GetGeneration()
					Expect(testEnv.Status().Update(ctx, obj))

					By("Creating a new patch helper")
					patcher, err := NewHelper(obj, testEnv)
					Expect(err).NotTo(HaveOccurred())

					By("Patching the object")
					Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

					By("Validating the object has been updated")
					Eventually(func() bool {
						objAfter := obj.DeepCopy()
						if err := testEnv.Get(ctx, key, objAfter); err != nil {
							return false
						}

						return lastGeneration == objAfter.Status.ObservedGeneration
					}, timeout).Should(BeTrue())
				})
			})
		*/
	})
}

func TestNewHelperNil(t *testing.T) {
	var x *appsv1.Deployment
	g := NewWithT(t)
	_, err := NewHelper(x, nil)
	g.Expect(err).ToNot(BeNil())
	_, err = NewHelper(nil, nil)
	g.Expect(err).ToNot(BeNil())
}
