// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineinframigration_test

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
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

	// maxConditionMessageLen is the most characters that the message of a
	// condition can have.
	maxConditionMessageLen = 32768
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

// mirroredPrefix is the prefix of the type of the VirtualMachine condition that
// mirrors the Ready condition of a VirtualMachineInfraMigration. Only that
// condition is mirrored, so checking for the prefix also catches any other type
// of condition being written.
const mirroredPrefix = "VirtualMachineInfraMigration"

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
// were written by this controller.
func mirroredTypes(vm *vmopv1.VirtualMachine) []string {
	var types []string
	for _, c := range vm.Status.Conditions {
		if strings.HasPrefix(c.Type, mirroredPrefix) {
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

		// logs holds what the reconcile logged, at up to logVerbosity.
		logs         []string
		logVerbosity int

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
		logs = nil
		logVerbosity = 0
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
			ctx.Logger)

		logger := funcr.New(
			func(_, args string) { logs = append(logs, args) },
			funcr.Options{Verbosity: logVerbosity})

		_, err = reconciler.Reconcile(
			logr.NewContext(context.Background(), logger),
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

		When("the VirtualMachine has the mirrored condition", func() {
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
				}
			})
			It("should remove it, and keep the other conditions and the VolumeAttributesClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMStatusVAC()).To(Equal("vac-gold"))

				obj := getVM()
				Expect(mirroredTypes(obj)).To(BeEmpty())
				Expect(conditions.IsTrue(
					obj,
					vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
			})

			When("the VirtualMachine is being deleted", func() {
				BeforeEach(func() {
					now := metav1.Now()
					vm.DeletionTimestamp = &now
					vm.Finalizers = []string{"vmoperator.vmware.com/virtualmachine"}
				})
				It("should not patch the VirtualMachine", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(mirroredTypes(getVM())).To(ConsistOf(
						vmopv1.VirtualMachineConditionInfraMigrationReady))
				})
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

	When("the VirtualMachineInfraMigration has no Ready condition", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				newCondition("Validated", metav1.ConditionTrue, 1),
			}
		})
		It("should not mirror any condition", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(mirroredTypes(getVM())).To(BeEmpty())
		})
	})

	When("the VirtualMachineInfraMigration has several conditions", func() {
		const failure = "StorageInSync: volume pvc-1 is not in sync"

		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				newCondition("Validated", metav1.ConditionTrue, 1),
				newCondition("StorageInSync", metav1.ConditionFalse, 1),
				newCondition("ComputeInSync", metav1.ConditionUnknown, 1),
				readyCondition(metav1.ConditionFalse, 1),
			}
			vmim.Status.Conditions[3].Reason = "StorageOutOfSync"
			vmim.Status.Conditions[3].Message = failure
		})
		It("should mirror only the Ready condition, with its status, reason, and message", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(mirroredTypes(getVM())).To(ConsistOf(
				vmopv1.VirtualMachineConditionInfraMigrationReady))

			c := getVMCondition()
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal("StorageOutOfSync"))
			Expect(c.Message).To(Equal(mirroredMessage(failure)))
		})

		It("should not copy the observed generation", func() {
			Expect(err).ToNot(HaveOccurred())
			for _, c := range getVM().Status.Conditions {
				Expect(c.ObservedGeneration).To(BeZero(), c.Type)
			}
		})

		When("the VirtualMachine has a mirrored Ready condition that changed", func() {
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
						Reason:             "Stale",
						LastTransitionTime: metav1.Now(),
					},
				}
			})
			It("should update it and keep the other conditions", func() {
				Expect(err).ToNot(HaveOccurred())

				obj := getVM()
				Expect(mirroredTypes(obj)).To(ConsistOf(
					vmopv1.VirtualMachineConditionInfraMigrationReady))
				Expect(conditions.IsFalse(
					obj,
					vmopv1.VirtualMachineConditionInfraMigrationReady)).To(BeTrue())
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
			It("should not change the VolumeAttributesClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).ToNot(BeNil())
				Expect(getVMStatusVAC()).To(Equal("vac-old"))
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
			It("should not record a VolumeAttributesClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).ToNot(BeNil())
				Expect(getVMStatusVAC()).To(BeEmpty())
			})
		})
	})

	When("the Ready condition describes a previous generation", func() {
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
			It("should not record a VolumeAttributesClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition()).To(BeNil())
				Expect(getVMStatusVAC()).To(BeEmpty())
			})
		})

		When("other conditions describe the current generation", func() {
			BeforeEach(func() {
				vmim.Status.Conditions = []metav1.Condition{
					readyCondition(metav1.ConditionTrue, 1),
					newCondition("Validated", metav1.ConditionTrue, 2),
				}
			})
			It("should still not mirror anything, since only Ready is mirrored", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mirroredTypes(getVM())).To(BeEmpty())
			})
		})

		When("the Ready condition has caught up with the generation", func() {
			BeforeEach(func() {
				vmim.Status.Conditions = []metav1.Condition{
					readyCondition(metav1.ConditionTrue, 2),
					newCondition("Validated", metav1.ConditionTrue, 1),
				}
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-gold"
				initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
			})
			It("should mirror it, whatever the other conditions, and record the VolumeAttributesClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mirroredTypes(getVM())).To(ConsistOf(
					vmopv1.VirtualMachineConditionInfraMigrationReady))
				Expect(getVMStatusVAC()).To(Equal("vac-gold"))
			})
		})
	})

	When("the Ready condition has no observed generation", func() {
		BeforeEach(func() {
			vmim.Status.Conditions = []metav1.Condition{
				readyCondition(metav1.ConditionTrue, 0),
				newCondition("Validated", metav1.ConditionTrue, 1),
			}
		})
		It("should not mirror it, since it does not describe the generation", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(mirroredTypes(getVM())).To(BeEmpty())
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
					It("should set the condition and record nothing", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
					})
				})

				When("the spec has no VolumeAttributesClass", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))
					})
					It("should set the condition and record nothing", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
					})
				})

				When("no VolumeAttributesClass refers to the policy", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = "vac-gold"
					})
					It("should set the condition and record nothing", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(getVMCondition()).ToNot(BeNil())
						Expect(getVMStatusVAC()).To(BeEmpty())
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

			When("the message is long", func() {
				const mark = "..."
				suffix := " @ VirtualMachineInfraMigration/" + dummyName

				When("the message and the reference are exactly at the limit", func() {
					var message string
					BeforeEach(func() {
						message = strings.Repeat(
							"a", maxConditionMessageLen-len(suffix))
						vmim.Status.Conditions[0].Message = message
					})
					It("should not truncate the message", func() {
						Expect(err).ToNot(HaveOccurred())

						c := getVMCondition()
						Expect(c).ToNot(BeNil())
						Expect(c.Message).To(Equal(message + suffix))
						Expect(utf8.RuneCountInString(c.Message)).To(
							Equal(maxConditionMessageLen))
					})
				})

				When("the message and the reference are one character over the limit", func() {
					BeforeEach(func() {
						vmim.Status.Conditions[0].Message = strings.Repeat(
							"a", maxConditionMessageLen-len(suffix)+1)
					})
					It("should truncate the message so that the reference fits", func() {
						Expect(err).ToNot(HaveOccurred())

						c := getVMCondition()
						Expect(c).ToNot(BeNil())
						Expect(utf8.RuneCountInString(c.Message)).To(
							Equal(maxConditionMessageLen))
						Expect(c.Message).To(Equal(
							strings.Repeat(
								"a", maxConditionMessageLen-len(suffix)-len(mark)) +
								mark + suffix))
					})
				})

				When("the message has multi-byte characters and is far over the limit", func() {
					BeforeEach(func() {
						vmim.Status.Conditions[0].Message = strings.Repeat(
							"é", 2*maxConditionMessageLen)
					})
					It("should truncate it on a character boundary", func() {
						Expect(err).ToNot(HaveOccurred())

						c := getVMCondition()
						Expect(c).ToNot(BeNil())
						Expect(utf8.ValidString(c.Message)).To(BeTrue())
						Expect(utf8.RuneCountInString(c.Message)).To(
							Equal(maxConditionMessageLen))
						Expect(c.Message).To(HaveSuffix(mark + suffix))
					})
				})
			})

			for _, status := range []metav1.ConditionStatus{
				metav1.ConditionTrue,
				metav1.ConditionFalse,
				metav1.ConditionUnknown,
			} {
				When("the Ready condition is "+string(status)+" and has no reason", func() {
					BeforeEach(func() {
						ready := readyCondition(status, 1)
						ready.Reason = ""
						vmim.Status.Conditions = []metav1.Condition{ready}
					})
					It("should use the status as the reason", func() {
						Expect(err).ToNot(HaveOccurred())

						c := getVMCondition()
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(status))
						Expect(c.Reason).To(Equal(string(status)))
					})
				})
			}

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

	Describe("logging", func() {
		const (
			mirroredLog    = "Mirrored infra migration condition to virtual machine"
			changedLog     = "Infra migration status changed on virtual machine"
			notRecordedLog = "Infra migration home storage policy is not " +
				"referenced by the virtual machine's volume attributes class"
		)

		logged := func() string { return strings.Join(logs, "\n") }

		// mirroredReady returns the condition of the VirtualMachine that
		// mirrors the migration's Ready condition, with the status and
		// message provided.
		mirroredReady := func(
			status metav1.ConditionStatus,
			message string) metav1.Condition {

			return metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionInfraMigrationReady,
				Status:             status,
				Reason:             readyReason,
				Message:            message,
				LastTransitionTime: metav1.Now(),
			}
		}

		When("the VirtualMachine does not have the condition yet", func() {
			It("should log the status change, and not the mirroring", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(logged()).To(ContainSubstring(changedLog))
				Expect(logged()).To(ContainSubstring(
					vmopv1.VirtualMachineConditionInfraMigrationReady +
						": <none> -> True"))
				Expect(logged()).ToNot(ContainSubstring(mirroredLog))
			})
		})

		When("the status of the condition changes", func() {
			BeforeEach(func() {
				vm.Status.Conditions = []metav1.Condition{
					mirroredReady(
						metav1.ConditionFalse, mirroredMessage(readyMessage)),
				}
			})
			It("should log the change", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(logged()).To(ContainSubstring(changedLog))
				Expect(logged()).To(ContainSubstring(
					vmopv1.VirtualMachineConditionInfraMigrationReady +
						": False -> True"))
			})
		})

		When("only the message of the condition changes", func() {
			BeforeEach(func() {
				// The mobility operator rewrites a failing condition's message
				// with the time left to the next retry.
				vm.Status.Conditions = []metav1.Condition{
					mirroredReady(
						metav1.ConditionTrue,
						mirroredMessage("a retry in 10m26s")),
				}
			})
			It("should not log a status change", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMCondition().Message).To(Equal(
					mirroredMessage(readyMessage)))
				Expect(logged()).ToNot(ContainSubstring(changedLog))
				Expect(logged()).ToNot(ContainSubstring(mirroredLog))
			})

			When("the verbosity is 4", func() {
				BeforeEach(func() {
					logVerbosity = 4
				})
				It("should log the mirroring, and still not a status change", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(logged()).To(ContainSubstring(mirroredLog))
					Expect(logged()).ToNot(ContainSubstring(changedLog))
				})
			})
		})

		When("nothing changes", func() {
			BeforeEach(func() {
				vm.Status.Conditions = []metav1.Condition{
					mirroredReady(
						metav1.ConditionTrue, mirroredMessage(readyMessage)),
				}
			})
			It("should log neither at the default verbosity", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(logged()).ToNot(ContainSubstring(changedLog))
				Expect(logged()).ToNot(ContainSubstring(mirroredLog))
			})

			When("the verbosity is 4", func() {
				BeforeEach(func() {
					logVerbosity = 4
				})
				It("should log the mirroring, and not a status change", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(logged()).To(ContainSubstring(mirroredLog))
					Expect(logged()).ToNot(ContainSubstring(changedLog))
				})
			})
		})

		When("the spec's VolumeAttributesClass is not the one for the home policy", func() {
			BeforeEach(func() {
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-silver"
				initObjects = append(initObjects,
					newVAC("vac-gold", homePolicyID),
					newVAC("vac-silver", "22222222-aaaa-bbbb-cccc-222222222222"))
			})
			It("should not log why nothing was recorded at the default verbosity", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMStatusVAC()).To(BeEmpty())
				Expect(logged()).ToNot(ContainSubstring(notRecordedLog))
			})

			When("the verbosity is 2", func() {
				BeforeEach(func() {
					logVerbosity = 2
				})
				It("should log why nothing was recorded", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(getVMStatusVAC()).To(BeEmpty())
					Expect(logged()).To(ContainSubstring(notRecordedLog))
					Expect(logged()).To(ContainSubstring(
						`"storagePolicyID"="` + homePolicyID + `"`))
					Expect(logged()).To(ContainSubstring(
						`"specVolumeAttributesClassName"="vac-silver"`))
				})
			})
		})

		When("the migration does not exist", func() {
			BeforeEach(func() {
				vmim = nil
				vm.Status.Conditions = []metav1.Condition{
					mirroredReady(
						metav1.ConditionFalse, mirroredMessage(readyMessage)),
				}
			})
			It("should log the condition that was removed", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(logged()).To(ContainSubstring(changedLog))
				Expect(logged()).To(ContainSubstring(
					vmopv1.VirtualMachineConditionInfraMigrationReady +
						": False -> <none>"))
			})
		})

		When("the migration no longer has a Ready condition for its generation", func() {
			BeforeEach(func() {
				vmim.Status.Conditions = nil
				vm.Status.Conditions = []metav1.Condition{
					mirroredReady(
						metav1.ConditionTrue, mirroredMessage(readyMessage)),
				}
			})
			It("should log the condition that was removed", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(logged()).To(ContainSubstring(changedLog))
				Expect(logged()).To(ContainSubstring(
					vmopv1.VirtualMachineConditionInfraMigrationReady +
						": True -> <none>"))
			})
		})

		When("the VolumeAttributesClass is recorded", func() {
			BeforeEach(func() {
				vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
					HomeStoragePolicyID: homePolicyID,
				}
				vm.Spec.VolumeAttributesClassName = "vac-gold"
				initObjects = append(initObjects, newVAC("vac-gold", homePolicyID))

				// The condition is the same, so only the status field changes.
				vm.Status.Conditions = []metav1.Condition{
					mirroredReady(
						metav1.ConditionTrue, mirroredMessage(readyMessage)),
				}
			})
			It("should log it, and no condition", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(getVMStatusVAC()).To(Equal("vac-gold"))
				Expect(logged()).To(ContainSubstring(changedLog))
				Expect(logged()).To(ContainSubstring(
					`"volumeAttributesClassName"="vac-gold"`))
				Expect(logged()).ToNot(ContainSubstring(`"condition"`))
			})

			When("the status already has it", func() {
				BeforeEach(func() {
					vm.Status.VolumeAttributesClassName = "vac-gold"
				})
				It("should not log a status change", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(logged()).ToNot(ContainSubstring(changedLog))
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

		It("should mirror the Ready condition as the migration reports it", func() {
			Consistently(vmMirroredTypes).Should(BeEmpty())

			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 1),
				readyCondition(metav1.ConditionFalse, 1))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionFalse))
			Consistently(vmMirroredTypes).Should(ConsistOf(
				vmopv1.VirtualMachineConditionInfraMigrationReady))

			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 1),
				newCondition("StorageInSync", metav1.ConditionTrue, 1),
				readyCondition(metav1.ConditionTrue, 1))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))

			By("removing it when the migration stops reporting it")
			setVMIMConditions(newCondition("Validated", metav1.ConditionTrue, 1))
			Eventually(vmMirroredTypes).Should(BeEmpty())
		})

		It("should mirror a message at the API limit together with the reference", func() {
			ready := readyCondition(metav1.ConditionTrue, 1)
			ready.Message = strings.Repeat("a", maxConditionMessageLen)
			setVMIMConditions(ready)
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))

			var obj vmopv1.VirtualMachine
			Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
			c := conditions.Get(
				&obj, vmopv1.VirtualMachineConditionInfraMigrationReady)
			Expect(c).ToNot(BeNil())
			Expect(utf8.RuneCountInString(c.Message)).To(
				Equal(maxConditionMessageLen))
			Expect(c.Message).To(HaveSuffix(
				"... @ VirtualMachineInfraMigration/" + dummyName))
		})

		It("should not keep a Ready condition that describes a previous generation", func() {
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

			By("mirroring it again once it describes the new generation")
			setVMIMConditions(
				newCondition("Validated", metav1.ConditionTrue, 2),
				readyCondition(metav1.ConditionFalse, 2))
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionFalse))
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

				By("removing the condition, and keeping the field, after the migration is deleted")
				Expect(ctx.Client.Delete(ctx, vmim)).To(Succeed())
				Eventually(vmReadyStatus).Should(BeEmpty())
				Consistently(getStatusVAC).Should(Equal(vac.Name))
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

	When("the VolumeAttributesClass for the home storage policy is created after the migration is ready", func() {
		var (
			vac      *storagev1.VolumeAttributesClass
			policyID string
		)

		getStatusVAC := func(g Gomega) string {
			var obj vmopv1.VirtualMachine
			g.Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
			return obj.Status.VolumeAttributesClassName
		}

		BeforeEach(func() {
			policyID = uuid.NewString()
			vac = newVAC("later-vac-"+uuid.NewString(), policyID)
			vmim.Spec.TargetStorage = &mobilityv1a4.TargetStorageSpec{
				HomeStoragePolicyID: policyID,
			}
			vm.Spec.VolumeAttributesClassName = vac.Name
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vmim)).To(Succeed())
		})

		AfterEach(func() {
			Expect(ctrlclient.IgnoreNotFound(
				ctx.Client.Delete(ctx, vac))).To(Succeed())
		})

		It("should record it without another VirtualMachine or migration event", func() {
			setVMIMReady(metav1.ConditionTrue)
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))
			Consistently(getStatusVAC).Should(BeEmpty())

			Expect(ctx.Client.Create(ctx, vac)).To(Succeed())
			Eventually(getStatusVAC).Should(Equal(vac.Name))
		})
	})

	When("the VirtualMachine has the mirrored condition but there is no migration", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
		})

		It("should remove the condition", func() {
			var obj vmopv1.VirtualMachine
			Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
			conditions.Set(&obj, &metav1.Condition{
				Type:    vmopv1.VirtualMachineConditionInfraMigrationReady,
				Status:  metav1.ConditionFalse,
				Reason:  readyReason,
				Message: mirroredMessage(readyMessage),
			})
			Expect(ctx.Client.Status().Update(ctx, &obj)).To(Succeed())

			Eventually(vmReadyStatus).Should(BeEmpty())
		})
	})

	When("something else removes a mirrored condition", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vmim)).To(Succeed())
		})

		It("should restore it", func() {
			setVMIMReady(metav1.ConditionTrue)
			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))

			var obj vmopv1.VirtualMachine
			Expect(ctx.Client.Get(ctx, vmKey, &obj)).To(Succeed())
			conditions.Delete(
				&obj, vmopv1.VirtualMachineConditionInfraMigrationReady)
			Expect(ctx.Client.Status().Update(ctx, &obj)).To(Succeed())

			Eventually(vmReadyStatus).Should(Equal(metav1.ConditionTrue))
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
