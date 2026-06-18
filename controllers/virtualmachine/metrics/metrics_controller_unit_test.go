// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmmetrics "github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcile,
	)
	Describe(
		"vmMetricsFieldsChanged",
		Label(
			testlabels.Controller,
		),
		unitTestsPredicateFieldsChanged,
	)
}

func intgTests() {
	// No integration tests for the metrics controller.
}

func unitTestsReconcile() {
	const (
		namespace = "default"
		name      = "test-vm"
	)

	var (
		ctx        *builder.UnitTestContextForController
		reconciler *vmmetrics.Reconciler
		vm         *vmopv1.VirtualMachine
		withObjects []ctrlclient.Object
	)

	BeforeEach(func() {
		vm = builder.DummyBasicVirtualMachine(name, namespace)
		withObjects = nil
	})

	JustBeforeEach(func() {
		withObjects = append(withObjects, vm)
		ctx = suite.NewUnitTestContextForController(withObjects...)
		reconciler = vmmetrics.NewReconciler(
			ctx,
			ctx.Client,
		)
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	doReconcile := func() (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: namespace,
				Name:      name,
			},
		})
	}

	getVM := func() *vmopv1.VirtualMachine {
		obj := &vmopv1.VirtualMachine{}
		ExpectWithOffset(1, ctx.Client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, obj)).To(Succeed())
		return obj
	}

	When("the VM does not exist", func() {
		JustBeforeEach(func() {
			ctx = suite.NewUnitTestContextForController()
			reconciler = vmmetrics.NewReconciler(
				ctx,
				ctx.Client,
			)
		})

		It("should return nil (no-op)", func() {
			result, err := doReconcile()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	When("the VM exists without the finalizer", func() {
		It("should add the finalizer", func() {
			result, err := doReconcile()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			obj := getVM()
			Expect(controllerutil.ContainsFinalizer(obj, "vmoperator.vmware.com/vm-metrics")).To(BeTrue())
		})
	})

	When("the VM exists with the finalizer", func() {
		BeforeEach(func() {
			controllerutil.AddFinalizer(vm, "vmoperator.vmware.com/vm-metrics")
		})

		It("should register metrics without error", func() {
			result, err := doReconcile()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	When("the VM is being deleted", func() {
		BeforeEach(func() {
			controllerutil.AddFinalizer(vm, "vmoperator.vmware.com/vm-metrics")
			// Also add the main controller's finalizer so the object isn't
			// immediately garbage collected by the fake client.
			controllerutil.AddFinalizer(vm, "vmoperator.vmware.com/virtualmachine")
			now := metav1.Now()
			vm.DeletionTimestamp = &now
		})

		It("should remove the finalizer", func() {
			result, err := doReconcile()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			obj := getVM()
			Expect(controllerutil.ContainsFinalizer(obj, "vmoperator.vmware.com/vm-metrics")).To(BeFalse())
		})
	})

	When("the VM is being deleted but has no metrics finalizer", func() {
		BeforeEach(func() {
			controllerutil.AddFinalizer(vm, "vmoperator.vmware.com/virtualmachine")
			now := metav1.Now()
			vm.DeletionTimestamp = &now
		})

		It("should be a no-op", func() {
			result, err := doReconcile()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
}

func unitTestsPredicateFieldsChanged() {
	var oldVM, newVM *vmopv1.VirtualMachine

	BeforeEach(func() {
		oldVM = builder.DummyBasicVirtualMachine("test-vm", "default")
		oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		oldVM.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		oldVM.Status.Conditions = []metav1.Condition{
			{
				Type:   vmopv1.VirtualMachineConditionCreated,
				Status: metav1.ConditionTrue,
				Reason: "Created",
			},
		}
		oldVM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
			PrimaryIP4: "10.0.0.1",
		}
		oldVM.Finalizers = []string{"vmoperator.vmware.com/vm-metrics"}

		newVM = oldVM.DeepCopy()
	})

	When("nothing has changed", func() {
		It("should return false", func() {
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeFalse())
		})
	})

	When("Spec.PowerState changes", func() {
		It("should return true", func() {
			newVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("Status.PowerState changes", func() {
		It("should return true", func() {
			newVM.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("a condition Status changes", func() {
		It("should return true", func() {
			newVM.Status.Conditions[0].Status = metav1.ConditionFalse
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("a condition Reason changes", func() {
		It("should return true", func() {
			newVM.Status.Conditions[0].Reason = "SomethingElse"
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("a condition is added", func() {
		It("should return true", func() {
			newVM.Status.Conditions = append(newVM.Status.Conditions, metav1.Condition{
				Type:   vmopv1.VirtualMachineConditionClassReady,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("only condition LastTransitionTime changes", func() {
		It("should return false", func() {
			newVM.Status.Conditions[0].LastTransitionTime = metav1.Now()
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeFalse())
		})
	})

	When("only condition Message changes", func() {
		It("should return false", func() {
			newVM.Status.Conditions[0].Message = "some new message"
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeFalse())
		})
	})

	When("IP goes from present to absent", func() {
		It("should return true", func() {
			newVM.Status.Network = nil
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("IP goes from absent to present", func() {
		BeforeEach(func() {
			oldVM.Status.Network = nil
		})
		It("should return true", func() {
			newVM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				PrimaryIP4: "10.0.0.1",
			}
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("IP address value changes but remains present", func() {
		It("should return false", func() {
			newVM.Status.Network.PrimaryIP4 = "10.0.0.2"
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeFalse())
		})
	})

	When("DeletionTimestamp is set", func() {
		It("should return true", func() {
			now := metav1.Now()
			newVM.DeletionTimestamp = &now
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("finalizers change", func() {
		It("should return true", func() {
			newVM.Finalizers = append(newVM.Finalizers, "some-other-finalizer")
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeTrue())
		})
	})

	When("an unrelated status field changes", func() {
		It("should return false", func() {
			newVM.Status.BiosUUID = "new-bios-uuid"
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeFalse())
		})
	})

	When("an unrelated spec field changes", func() {
		It("should return false", func() {
			newVM.Spec.ClassName = "new-class"
			Expect(vmmetrics.VMMetricsFieldsChanged(oldVM, newVM)).To(BeFalse())
		})
	})
}
