// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineinframigration_test

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/mobility/virtualmachineinframigration"
	mobilityv1a4 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	dummyNamespace = "dummy-ns"
	dummyName      = "dummy-vm"

	readyReason  = "MigrationConverged"
	readyMessage = "all storage is in sync"

	homePolicyID = "11111111-aaaa-bbbb-cccc-111111111111"
)

// newVAC returns a VolumeAttributesClass that refers to the policy ID.
func newVAC(name, policyID string) *storagev1.VolumeAttributesClass {
	return &storagev1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		DriverName: "fake",
		Parameters: map[string]string{"storagePolicyID": policyID},
	}
}

func newCondition(
	t string,
	status metav1.ConditionStatus,
	observedGeneration int64) metav1.Condition {

	return metav1.Condition{
		Type:               t,
		Status:             status,
		Reason:             readyReason,
		Message:            readyMessage,
		ObservedGeneration: observedGeneration,
		LastTransitionTime: metav1.Now(),
	}
}

func readyCondition(
	status metav1.ConditionStatus,
	observedGeneration int64) metav1.Condition {

	return newCondition("Ready", status, observedGeneration)
}

// mirroredType returns the type of the VirtualMachine condition that mirrors the
// VirtualMachineInfraMigration condition with the provided type.
func mirroredType(t string) string {
	return vmopv1.VirtualMachineConditionInfraMigrationPrefix + t
}

// mirroredMessage returns the message of a mirrored condition, which is the
// message of the VirtualMachineInfraMigration's condition followed by a
// reference to the VirtualMachineInfraMigration.
func mirroredMessage(message string) string {
	ref := "VirtualMachineInfraMigration/" + dummyName
	if message == "" {
		return ref
	}
	return message + " @ " + ref
}

// mirroredTypes returns the types of the conditions of the VirtualMachine that
// mirror a VirtualMachineInfraMigration condition.
func mirroredTypes(vm *vmopv1.VirtualMachine) []string {
	var types []string
	for _, c := range vm.Status.Conditions {
		if strings.HasPrefix(
			c.Type,
			vmopv1.VirtualMachineConditionInfraMigrationPrefix) {

			types = append(types, c.Type)
		}
	}
	return types
}

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []ctrlclient.Object
		funcs       interceptor.Funcs
		ctx         *builder.UnitTestContextForController

		reconciler *virtualmachineinframigration.Reconciler
		vmim       *mobilityv1a4.VirtualMachineInfraMigration
		vm         *vmopv1.VirtualMachine

		err error
	)

	getVM := func() *vmopv1.VirtualMachine {
		var obj vmopv1.VirtualMachine
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			ctrlclient.ObjectKey{Namespace: dummyNamespace, Name: dummyName},
			&obj)).To(Succeed())
		return &obj
	}

	getVMCondition := func() *metav1.Condition {
		return conditions.Get(
			getVM(), vmopv1.VirtualMachineConditionInfraMigrationReady)
	}

	getVMStatusVAC := func() string {
		var obj vmopv1.VirtualMachine
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			ctrlclient.ObjectKey{Namespace: dummyNamespace, Name: dummyName},
			&obj)).To(Succeed())
		return obj.Status.VolumeAttributesClassName
	}

	BeforeEach(func() {
		err = nil
		initObjects = nil
		funcs = interceptor.Funcs{}

		vmim = &mobilityv1a4.VirtualMachineInfraMigration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:  dummyNamespace,
				Name:       dummyName,
				Generation: 1,
			},
			Spec: mobilityv1a4.VirtualMachineInfraMigrationSpec{
				TargetResourcePool: "resgroup-1",
			},
			Status: mobilityv1a4.VirtualMachineInfraMigrationStatus{
				Conditions: []metav1.Condition{
					readyCondition(metav1.ConditionTrue, 1),
				},
			},
		}

		vm = builder.DummyBasicVirtualMachine(dummyName, dummyNamespace)
	})

	JustBeforeEach(func() {
		if vmim != nil {
			initObjects = append(initObjects, vmim)
		}
		if vm != nil {
			initObjects = append(initObjects, vm)
		}
		ctx = suite.NewUnitTestContextForControllerWithFuncs(funcs, initObjects...)
		reconciler = virtualmachineinframigration.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder)

		_, err = reconciler.Reconcile(
			context.Background(),
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: dummyNamespace,
					Name:      dummyName,
				},
			})
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	When("the VirtualMachineInfraMigration does not exist", func() {
		BeforeEach(func() {
			vmim = nil
		})
		It("should not return an error or patch the VirtualMachine", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(getVMCondition()).To(BeNil())
		})

		When("there is no VirtualMachine", func() {
			BeforeEach(func() {
				vm = nil
			})
			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("the VirtualMachine has mirrored conditions", func() {
			BeforeEach(func() {
				vm.Status.VolumeAttributesClassName = "vac-gold"
				vm.Status.Conditions = []metav1.Condition{
					{
						Type:               vmopv1.VirtualMachineConditionCreated,
						Status:             metav1.ConditionTrue,
						Reason:             string(metav1.ConditionTrue),
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               vmopv1.VirtualMachineConditionInfraMigrationReady,
						Status:             metav1.ConditionTrue,
						Reason:             readyReason,
						Message:            readyMessage,
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               mirroredType("Validated"),
						Status:             metav1.ConditionFalse,
						Reason:             readyReason,
						Message:            readyMessage,
						LastTransitionTime: metav1.Now(),
					},
				}
			})
			It("should leave the VirtualMachine unchanged", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMStatusVAC()).To(Equal("vac-gold"))

				obj := getVM()
				Expect(mirroredTypes(obj)).To(ConsistOf(
					vmopv1.VirtualMachineConditionInfraMigrationReady,
					mirroredType("Validated")))
				Expect(conditions.IsTrue(
					obj,
					vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
			})
		})
	})

	When("the VirtualMachineInfraMigration has no conditions", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = nil
		})
		It("should not mirror any condition", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(mirroredTypes(getVM())).To(BeEmpty())
		})

		When("the VirtualMachine has mirrored conditions", func() {
			BeforeEach(func() {
				vm.Status.Conditions = []metav1.Condition{
					{
						Type:               vmopv1.VirtualMachineConditionCreated,
						Status:             metav1.ConditionTrue,
						Reason:             string(metav1.ConditionTrue),
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               vmopv1.VirtualMachineConditionInfraMigrationReady,
						Status:             metav1.ConditionTrue,
						Reason:             readyReason,
						LastTransitionTime: metav1.Now(),
					},
				}
			})
			It("should remove them and keep the other conditions", func() {
				Expect(err).ToNot(HaveOccurred())

				obj := getVM()
				Expect(mirroredTypes(obj)).To(BeEmpty())
				Expect(conditions.IsTrue(
					obj,
					vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
			})
		})
	})

	When("the VirtualMachineInfraMigration has a different condition only", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				newCondition("Validated", metav1.ConditionTrue, 1),
			}
		})
		It("should mirror it, and not set the Ready condition", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(getVMCondition()).To(BeNil())

			c := conditions.Get(getVM(), mirroredType("Validated"))
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionTrue))
			Expect(c.Reason).To(Equal(readyReason))
			Expect(c.Message).To(Equal(mirroredMessage(readyMessage)))
		})
	})

	When("the VirtualMachineInfraMigration has several conditions", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				newCondition("Validated", metav1.ConditionTrue, 1),
				newCondition("StorageInSync", metav1.ConditionFalse, 1),
				newCondition("ComputeInSync", metav1.ConditionUnknown, 1),
				readyCondition(metav1.ConditionFalse, 1),
			}
			vmim.Status.Conditions[1].Reason = "StorageOutOfSync"
			vmim.Status.Conditions[1].Message = "volume pvc-1 is not in sync"
		})
		It("should mirror every one of them with its status, reason, and message", func() {
			Expect(err).ToNot(HaveOccurred())

			obj := getVM()
			Expect(mirroredTypes(obj)).To(ConsistOf(
				mirroredType("Validated"),
				mirroredType("StorageInSync"),
				mirroredType("ComputeInSync"),
				mirroredType("Ready")))

			for _, expected := range vmim.Status.Conditions {
				c := conditions.Get(obj, mirroredType(expected.Type))
				Expect(c).ToNot(BeNil(), expected.Type)
				Expect(c.Status).To(Equal(expected.Status), expected.Type)
				Expect(c.Reason).To(Equal(expected.Reason), expected.Type)
				Expect(c.Message).To(Equal(mirroredMessage(expected.Message)), expected.Type)
			}

			Expect(getVMCondition()).To(Equal(
				conditions.Get(obj, mirroredType("Ready"))))
		})

		It("should not copy the observed generation", func() {
			Expect(err).ToNot(HaveOccurred())
			for _, c := range getVM().Status.Conditions {
				Expect(c.ObservedGeneration).To(BeZero(), c.Type)
			}
		})

		When("the VirtualMachine has mirrored conditions that changed or are gone", func() {
			BeforeEach(func() {
				vm.Status.Conditions = []metav1.Condition{
					{
						Type:               vmopv1.VirtualMachineConditionCreated,
						Status:             metav1.ConditionTrue,
						Reason:             string(metav1.ConditionTrue),
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               mirroredType("Validated"),
						Status:             metav1.ConditionFalse,
						Reason:             "Stale",
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               mirroredType("NoLongerReported"),
						Status:             metav1.ConditionTrue,
						Reason:             readyReason,
						LastTransitionTime: metav1.Now(),
					},
				}
			})
			It("should update the former, remove the latter, and keep the others", func() {
				Expect(err).ToNot(HaveOccurred())

				obj := getVM()
				Expect(mirroredTypes(obj)).To(ConsistOf(
					mirroredType("Validated"),
					mirroredType("StorageInSync"),
					mirroredType("ComputeInSync"),
					mirroredType("Ready")))
				Expect(conditions.IsTrue(
					obj,
					mirroredType("Validated"))).To(BeTrue())
				Expect(conditions.IsTrue(
					obj,
					vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
			})
		})
	})

	When("the Ready condition is False", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				readyCondition(metav1.ConditionFalse, 1),
			}
		})
		It("should mirror the condition as False", func() {
			Expect(err).ToNot(HaveOccurred())

			c := getVMCondition()
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(readyReason))
			Expect(c.Message).To(Equal(mirroredMessage(readyMessage)))
		})

		When("the VirtualMachine has the condition as True", func() {
			BeforeEach(func() {
				vm.Status.Conditions = []metav1.Condition{
					{
						Type:               vmopv1.VirtualMachineConditionInfraMigrationReady,
						Status:             metav1.ConditionTrue,
						Reason:             readyReason,
						LastTransitionTime: metav1.Now(),
					},
				}
			})
			It("should change it to False", func() {
				Expect(err).ToNot(HaveOccurred())

				c := getVMCondition()
				Expect(c).ToNot(BeNil())
				Expect(c.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		When("the migration changes the home storage policy", func() {
			BeforeEach(func() {
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-gold"
				vm.Status.VolumeAttributesClassName = "vac-old"
				initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
			})
			It("should not change the VolumeAttributesClass or warn", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).ToNot(BeNil())
				Expect(getVMStatusVAC()).To(Equal("vac-old"))
				Expect(ctx.Events).To(BeEmpty())
			})
		})
	})

	When("the Ready condition is Unknown", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				readyCondition(metav1.ConditionUnknown, 1),
			}
		})
		It("should mirror the condition as Unknown", func() {
			Expect(err).ToNot(HaveOccurred())

			c := getVMCondition()
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionUnknown))
		})

		When("the migration changes the home storage policy", func() {
			BeforeEach(func() {
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-gold"
				initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
			})
			It("should not record a VolumeAttributesClass or warn", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).ToNot(BeNil())
				Expect(getVMStatusVAC()).To(BeEmpty())
				Expect(ctx.Events).To(BeEmpty())
			})
		})
	})

	When("a condition describes a previous generation", func() {
		BeforeEach(func() {
			vmim.Generation = 2
			vmim.Status.Conditions = []metav1.Condition{
				readyCondition(metav1.ConditionTrue, 1),
			}
		})
		It("should not mirror the condition", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(getVMCondition()).To(BeNil())
			Expect(mirroredTypes(getVM())).To(BeEmpty())
		})

		When("the VirtualMachine mirrors the condition from the previous generation", func() {
			BeforeEach(func() {
				vm.Status.Conditions = []metav1.Condition{
					{
						Type:               vmopv1.VirtualMachineConditionCreated,
						Status:             metav1.ConditionTrue,
						Reason:             string(metav1.ConditionTrue),
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               vmopv1.VirtualMachineConditionInfraMigrationReady,
						Status:             metav1.ConditionTrue,
						Reason:             readyReason,
						Message:            mirroredMessage(readyMessage),
						LastTransitionTime: metav1.Now(),
					},
				}
			})
			It("should remove it and keep the other conditions", func() {
				Expect(err).ToNot(HaveOccurred())

				obj := getVM()
				Expect(mirroredTypes(obj)).To(BeEmpty())
				Expect(conditions.IsTrue(
					obj,
					vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
			})
		})

		When("the migration changes the home storage policy", func() {
			BeforeEach(func() {
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-gold"
				initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
			})
			It("should not record a VolumeAttributesClass or warn", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).To(BeNil())
				Expect(getVMStatusVAC()).To(BeEmpty())
				Expect(ctx.Events).To(BeEmpty())
			})
		})

		When("other conditions describe the current generation", func() {
			BeforeEach(func() {
				vmim.Status.Conditions = []metav1.Condition{
					readyCondition(metav1.ConditionTrue, 1),
					newCondition("Validated", metav1.ConditionTrue, 2),
					newCondition("StorageInSync", metav1.ConditionFalse, 1),
				}
			})
			It("should mirror only the ones that do", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mirroredTypes(getVM())).To(ConsistOf(
					mirroredType("Validated")))
			})
		})

		When("the conditions have caught up with the generation", func() {
			BeforeEach(func() {
				vmim.Status.Conditions = []metav1.Condition{
					readyCondition(metav1.ConditionTrue, 2),
					newCondition("Validated", metav1.ConditionTrue, 2),
				}
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-gold"
				initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
			})
			It("should mirror them and record the VolumeAttributesClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mirroredTypes(getVM())).To(ConsistOf(
					mirroredType("Ready"),
					mirroredType("Validated")))
				Expect(getVMStatusVAC()).To(Equal("vac-gold"))
			})
		})
	})

	When("a condition has no observed generation", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				readyCondition(metav1.ConditionTrue, 0),
				newCondition("Validated", metav1.ConditionTrue, 1),
			}
		})
		It("should not mirror it, since it does not describe the generation", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(mirroredTypes(getVM())).To(ConsistOf(
				mirroredType("Validated")))
		})
	})

	When("the Ready condition is True for the current generation", func() {
		When("there is no VirtualMachine", func() {
			BeforeEach(func() {
				vm = nil
			})
			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
				err := ctx.Client.Get(
					ctx,
					ctrlclient.ObjectKey{
						Namespace: dummyNamespace,
						Name:      dummyName,
					},
					&vmopv1.VirtualMachine{})
				Expect(err).To(HaveOccurred())
			})
		})

		When("the VirtualMachine is being deleted", func() {
			BeforeEach(func() {
				now := metav1.Now()
				vm.DeletionTimestamp = &now
				vm.Finalizers = []string{"vmoperator.vmware.com/virtualmachine"}
			})
			It("should not patch the VirtualMachine", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).To(BeNil())
			})
		})

		When("the VirtualMachine exists", func() {
			It("should set the condition on the VirtualMachine", func() {
				Expect(err).ToNot(HaveOccurred())

				c := getVMCondition()
				Expect(c).ToNot(BeNil())
				Expect(c.Status).To(Equal(metav1.ConditionTrue))
				Expect(c.Reason).To(Equal(readyReason))
				Expect(c.Message).To(Equal(mirroredMessage(readyMessage)))
			})

			When("the migration does not change the home storage policy", func() {
				BeforeEach(func() {
					vm.Spec.VolumeAttributesClassName = "vac-gold"
					initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
				})
				It("should not record a VolumeAttributesClass", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(getVMCondition()).ToNot(BeNil())
					Expect(getVMStatusVAC()).To(BeEmpty())
					Expect(ctx.Events).To(BeEmpty())
				})
			})

			When("the migration changes the home storage policy", func() {
				BeforeEach(func() {
					vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
						HomeStoragePolicyID: homePolicyID,
					}
				})

				When("the spec names the VolumeAttributesClass for the policy", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = "vac-gold"
						initObjects = append(initObjects,
							newVAC("vac-gold", homePolicyID),
							newVAC("vac-silver", "22222222-aaaa-bbbb-cccc-222222222222"))
					})
					It("should record it", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(Equal("vac-gold"))
						Expect(ctx.Events).To(BeEmpty())
					})

					When("the status holds a previous VolumeAttributesClass", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = "vac-old"
						})
						It("should replace it", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(getVMStatusVAC()).To(Equal("vac-gold"))
						})
					})
				})

				When("several VolumeAttributesClasses refer to the policy", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = "vac-b"
						initObjects = append(initObjects,
							newVAC("vac-a", homePolicyID),
							newVAC("vac-b", homePolicyID))
					})
					It("should record the one in the spec", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMStatusVAC()).To(Equal("vac-b"))
					})
				})

				When("the spec names a VolumeAttributesClass for another policy", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = "vac-silver"
						initObjects = append(initObjects,
							newVAC("vac-gold", homePolicyID),
							newVAC("vac-silver", "22222222-aaaa-bbbb-cccc-222222222222"))
					})
					It("should set the condition, record nothing, and warn", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
						Eventually(ctx.Events).Should(Receive(ContainSubstring(
							"VolumeAttributesClassMismatch")))
					})
				})

				When("the spec has no VolumeAttributesClass", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
					})
					It("should set the condition, record nothing, and warn", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
						Eventually(ctx.Events).Should(Receive(ContainSubstring(
							"VolumeAttributesClassMismatch")))
					})
				})

				When("no VolumeAttributesClass refers to the policy", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = "vac-gold"
					})
					It("should set the condition, record nothing, and warn", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
						Eventually(ctx.Events).Should(Receive(ContainSubstring(
							"VolumeAttributesClassMismatch")))
					})
				})

				When("listing the VolumeAttributesClasses fails", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = "vac-gold"
						funcs.List = func(
							ctx context.Context,
							client ctrlclient.WithWatch,
							list ctrlclient.ObjectList,
							opts ...ctrlclient.ListOption) error {

							if _, ok := list.(*storagev1.VolumeAttributesClassList); ok {
								return apierrors.NewInternalError(errors.New("fake"))
							}
							return client.List(ctx, list, opts...)
						}
					})
					It("should return the error and not patch the VirtualMachine", func() {
						Expect(err).To(MatchError(ContainSubstring("fake")))
						Expect(getVMCondition()).To(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
					})
				})
			})

			When("a condition has no message", func() {
				BeforeEach(func() {
					vmim.Status.Conditions[0].Message = ""
				})
				It("should set only the reference to the migration as the message", func() {
					Expect(err).ToNot(HaveOccurred())

					c := getVMCondition()
					Expect(c).ToNot(BeNil())
					Expect(c.Message).To(Equal("VirtualMachineInfraMigration/" + dummyName))
				})
			})

			When("a condition has no reason", func() {
				BeforeEach(func() {
					ready := readyCondition(metav1.ConditionTrue, 1)
					ready.Reason = ""
					validated := newCondition("Validated", metav1.ConditionFalse, 1)
					validated.Reason = ""
					compute := newCondition("ComputeInSync", metav1.ConditionUnknown, 1)
					compute.Reason = ""
					vmim.Status.Conditions = []metav1.Condition{
						ready, validated, compute,
					}
				})
				It("should still set a valid condition", func() {
					Expect(err).ToNot(HaveOccurred())

					obj := getVM()

					c := conditions.Get(obj, mirroredType("Ready"))
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))

					c = conditions.Get(obj, mirroredType("Validated"))
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(string(metav1.ConditionFalse)))

					c = conditions.Get(obj, mirroredType("ComputeInSync"))
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionUnknown))
					Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				})
			})

			When("the VirtualMachine has other conditions", func() {
				BeforeEach(func() {
					vm.Status.Conditions = []metav1.Condition{
						{
							Type:               vmopv1.VirtualMachineConditionCreated,
							Status:             metav1.ConditionTrue,
							Reason:             string(metav1.ConditionTrue),
							LastTransitionTime: metav1.Now(),
						},
					}
				})
				It("should preserve them", func() {
					Expect(err).ToNot(HaveOccurred())

					var obj vmopv1.VirtualMachine
					Expect(ctx.Client.Get(
						ctx,
						ctrlclient.ObjectKeyFromObject(vm),
						&obj)).To(Succeed())

					Expect(conditions.IsTrue(
						&obj,
						vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
					Expect(conditions.IsTrue(
						&obj,
						vmopv1.VirtualMachineConditionInfraMigrationReady)).To(BeTrue())
				})
			})

			When("the VirtualMachine already has the condition", func() {
				var transitionTime metav1.Time

				BeforeEach(func() {
					transitionTime = metav1.NewTime(
						time.Now().Add(-time.Hour).Truncate(time.Second))
					vm.Status.Conditions = []metav1.Condition{
						{
							Type:               vmopv1.VirtualMachineConditionInfraMigrationReady,
							Status:             metav1.ConditionTrue,
							Reason:             readyReason,
							Message:            mirroredMessage(readyMessage),
							LastTransitionTime: transitionTime,
						},
					}
				})
				It("should not change the last transition time", func() {
					Expect(err).ToNot(HaveOccurred())

					c := getVMCondition()
					Expect(c).ToNot(BeNil())
					Expect(c.LastTransitionTime.Time.Equal(
						transitionTime.Time)).To(BeTrue())
				})

				When("the migration's condition changed", func() {
					BeforeEach(func() {
						vmim.Status.Conditions[0].Message = "changed"
					})
					It("should update the message and the last transition time", func() {
						Expect(err).ToNot(HaveOccurred())

						c := getVMCondition()
						Expect(c).ToNot(BeNil())
						Expect(c.Message).To(Equal(mirroredMessage("changed")))
						Expect(c.LastTransitionTime.Time.After(
							transitionTime.Time)).To(BeTrue())
					})
				})
			})
		})
	})
}

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	var (
		ctx  *builder.IntegrationTestContext
		vmim *mobilityv1a4.VirtualMachineInfraMigration
		vm   *vmopv1.VirtualMachine

		vmKey ctrlclient.ObjectKey
	)

	// setVMIMConditions sets the conditions on the VirtualMachineInfraMigration
	// as the mobility operator would.
	setVMIMConditions := func(c ...metav1.Condition) {
		var obj mobilityv1a4.VirtualMachineInfraMigration
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			ctrlclient.ObjectKeyFromObject(vmim),
			&obj)).To(Succeed())
		obj.Status.Conditions = c
		ExpectWithOffset(1, ctx.Client.Status().Update(ctx, &obj)).To(Succeed())
	}

	// setVMIMReady sets only the Ready condition on the
	// VirtualMachineInfraMigration.
	setVMIMReady := func(status metav1.ConditionStatus) {
		var obj mobilityv1a4.VirtualMachineInfraMigration
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			ctrlclient.ObjectKeyFromObject(vmim),
			&obj)).To(Succeed())
		setVMIMConditions(readyCondition(status, obj.Generation))
	}

	// vmConditionStatus returns the status of the VirtualMachine condition with
	// the provided type, or an empty status if it does not have one.
	vmConditionStatus := func(t string) func(g Gomega) metav1.ConditionStatus {
		return func(g Gomega) metav1.ConditionStatus {
			var obj vmopv1.VirtualMachine
			g.Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
			if c := conditions.Get(&obj, t); c != nil {
				return c.Status
			}
			return ""
		}
	}

	vmReadyStatus := vmConditionStatus(
		vmopv1.VirtualMachineConditionInfraMigrationReady)

	vmMirroredTypes := func(g Gomega) []string {
		var obj vmopv1.VirtualMachine
		g.Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
		return mirroredTypes(&obj)
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = builder.DummyBasicVirtualMachine(dummyName, ctx.Namespace)
		vmKey = ctrlclient.ObjectKeyFromObject(vm)

		vmim = &mobilityv1a4.VirtualMachineInfraMigration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      dummyName,
			},
			Spec: mobilityv1a4.VirtualMachineInfraMigrationSpec{
				TargetResourcePool: "resgroup-1",
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	When("the VirtualMachine exists before the migration is ready", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vmim)).To(Succeed())
		})

		It("should mirror the conditions as the migration reports them", func() {
			Consistently(vmMirroredTypes).Should(BeEmpty())

			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 1),
				readyCondition(metav1.ConditionFalse, 1))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionFalse))
			Eventually(vmConditionStatus(mirroredType("Validated"))).Should(
				Equal(metav1.ConditionTrue))

			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 1),
				newCondition("StorageInSync", metav1.ConditionTrue, 1),
				readyCondition(metav1.ConditionTrue, 1))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))
			Eventually(vmMirroredTypes).Should(ConsistOf(
				mirroredType("Validated"),
				mirroredType("StorageInSync"),
				mirroredType("Ready")))

			By("removing a mirrored condition the migration stops reporting")
			setVMIMConditions(readyCondition(metav1.ConditionTrue, 1))
			Eventually(vmMirroredTypes).Should(ConsistOf(mirroredType("Ready")))
		})

		It("should not keep conditions that describe a previous generation", func() {
			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 1),
				readyCondition(metav1.ConditionTrue, 1))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))

			By("changing the migration's spec, which advances its generation")
			var obj mobilityv1a4.VirtualMachineInfraMigration
			Expect(ctx.Client.Get(
				ctx,
				ctrlclient.ObjectKeyFromObject(vmim),
				&obj)).To(Succeed())
			obj.Spec.TargetResourcePool = "resgroup-2"
			Expect(ctx.Client.Update(ctx, &obj)).To(Succeed())
			Expect(obj.Generation).To(BeEquivalentTo(2))
			Eventually(vmMirroredTypes).Should(BeEmpty())

			By("mirroring them again once they describe the new generation")
			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 2),
				readyCondition(metav1.ConditionFalse, 2))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionFalse))
			Eventually(vmConditionStatus(mirroredType("Validated"))).Should(
				Equal(metav1.ConditionTrue))
		})
	})

	When("the migration changes the home storage policy", func() {
		var (
			policyID string
			vac      *storagev1.VolumeAttributesClass
		)

		getStatusVAC := func(g Gomega) string {
			var obj vmopv1.VirtualMachine
			g.Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
			return obj.Status.VolumeAttributesClassName
		}

		BeforeEach(func() {
			policyID = uuid.NewString()
			vac = newVAC("", policyID)
			vac.GenerateName = "my-vac-"
			vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
				HomeStoragePolicyID: policyID,
			}
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vac)).To(Succeed())
		})

		AfterEach(func() {
			Expect(ctx.Client.Delete(ctx, vac)).To(Succeed())
		})

		When("the spec names the VolumeAttributesClass for the policy", func() {
			JustBeforeEach(func() {
				vm.Spec.VolumeAttributesClassName = vac.Name
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vmim)).To(Succeed())
			})

			It("should record the VolumeAttributesClass once the migration is ready", func() {
				Consistently(getStatusVAC).Should(BeEmpty())

				By("mirroring a migration that is not ready without recording it")
				setVMIMReady(metav1.ConditionFalse)
				Eventually(vmReadyStatus).Should(Equal(metav1.ConditionFalse))
				Consistently(getStatusVAC).Should(BeEmpty())

				setVMIMReady(metav1.ConditionTrue)
				Eventually(getStatusVAC).Should(Equal(vac.Name))
				Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))

				By("keeping it, and the conditions, after the migration is deleted")
				Expect(ctx.Client.Delete(ctx, vmim)).To(Succeed())
				Consistently(getStatusVAC).Should(Equal(vac.Name))
				Consistently(vmReadyStatus).Should(Equal(metav1.ConditionTrue))
			})
		})

		When("the spec names a different VolumeAttributesClass", func() {
			JustBeforeEach(func() {
				vm.Spec.VolumeAttributesClassName = "some-other-vac"
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vmim)).To(Succeed())
			})

			It("should mirror the condition but not record a VolumeAttributesClass", func() {
				setVMIMReady(metav1.ConditionTrue)
				Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))
				Consistently(getStatusVAC).Should(BeEmpty())
			})
		})
	})

	When("the migration is ready before the VirtualMachine exists", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vmim)).To(Succeed())
			setVMIMReady(metav1.ConditionTrue)
		})

		It("should mirror the conditions once the VirtualMachine is created", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))
		})
	})
}
