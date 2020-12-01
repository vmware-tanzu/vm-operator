//go:generate mockgen -destination=../../../mocks/mock_probe.go -package=mocks github.com/vmware-tanzu/vm-operator/pkg/prober/probe Probe

// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package worker

import (
	goctx "context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgorecord "k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/mocks"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine readiness probes", func() {
	var (
		testWorker Worker
		vm         *vmopv1alpha1.VirtualMachine
		vmKey      types.NamespacedName
		ctx        *context.ProbeContext

		fakeClient   client.Client
		fakeRecorder record.Recorder
		fakeEvents   chan string

		mockCtrl  *gomock.Controller
		mockProbe *mocks.MockProbe
	)

	BeforeEach(func() {
		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ClassName: "dummy-vmclass",
			},
		}

		vmKey = client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}

		fakeClient, _ = builder.NewFakeClient()
		eventRecorder := clientgorecord.NewFakeRecorder(1024)
		fakeRecorder = record.New(eventRecorder)
		fakeEvents = eventRecorder.Events

		mockCtrl = gomock.NewController(GinkgoT())
		mockProbe = mocks.NewMockProbe(mockCtrl)
		ctx = &context.ProbeContext{
			Context:   goctx.Background(),
			Logger:    ctrl.Log.WithName("readiness-probe").WithValues("vmName", vm.NamespacedName()),
			ProbeType: "readiness",
			VM:        vm,
		}
		queue := workqueue.NewNamedDelayingQueue("test")
		prober := probe.NewProber()
		testWorker = NewReadinessWorker(queue, prober, fakeClient, fakeRecorder)
		prober.TCPProbe = mockProbe
	})

	getVMCondition := func(vm *vmopv1alpha1.VirtualMachine, conditionType vmopv1alpha1.VirtualMachineConditionType) metav1.ConditionStatus {
		for _, c := range vm.Status.Conditions {
			if c.Type != conditionType {
				continue
			}
			return c.Status
		}
		return ""
	}

	checkVirtualMachineCondition := func(c client.Client, objKey client.ObjectKey, expectedCondition metav1.ConditionStatus) {
		Expect(fakeClient.Get(ctx, objKey, vm)).Should(Succeed())
		status := getVMCondition(vm, vmopv1alpha1.VirtualMachineReady)
		Expect(status).Should(Equal(expectedCondition))
	}

	Context("VM has TCP readiness probe", func() {
		var (
			oldStatus vmopv1alpha1.VirtualMachineStatus
		)

		BeforeEach(func() {
			vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(10001)
			ctx.ProbeSpec = vm.Spec.ReadinessProbe
			Expect(fakeClient.Create(ctx, vm)).To(Succeed())
			Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
			oldStatus = vm.Status
		})

		AfterEach(func() {
			Expect(fakeClient.Delete(ctx, vm)).To(Succeed())
		})

		When("new VirtualMachineCondition is in a transition", func() {
			It("Should update the VirtualMachineCondition when probe succeeds", func() {
				mockProbe.EXPECT().Probe(ctx).Return(probe.Success, nil)

				err := testWorker.DoProbe(vm)
				Expect(err).ShouldNot(HaveOccurred())

				By("Should set VirtualMachineReady condition status as true", func() {
					checkVirtualMachineCondition(fakeClient, vmKey, metav1.ConditionTrue)
				})

				By("Conditions in VM status should be updated", func() {
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					Expect(vm.Status.Conditions).ShouldNot(Equal(oldStatus.Conditions))
					Expect(fakeEvents).Should(Receive(ContainSubstring(readyReason)))
				})
			})

			It("Should update the VirtualMachineCondition when probe fails", func() {
				mockProbe.EXPECT().Probe(ctx).Return(probe.Failure, nil)

				err := testWorker.DoProbe(vm)
				Expect(err).ShouldNot(HaveOccurred())

				By("Should set VirtualMachineReady condition value as false", func() {
					checkVirtualMachineCondition(fakeClient, vmKey, metav1.ConditionFalse)
				})

				By("Conditions in VM status should be updated", func() {
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					Expect(vm.Status.Conditions).ShouldNot(Equal(oldStatus.Conditions))
					Expect(fakeEvents).Should(Receive(ContainSubstring(notReadyReason)))
				})
			})

			When("new VirtualMachineCondition isn't in a transition", func() {
				It("Shouldn't update the VirtualMachineCondition in status", func() {
					vmReadyCondition := vmopv1alpha1.VirtualMachineCondition{
						Type:   vmopv1alpha1.VirtualMachineReady,
						Status: metav1.ConditionTrue,
						Reason: readyReason,
					}
					vm.Status.Conditions = append(vm.Status.Conditions, vmReadyCondition)
					Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					oldStatus = vm.Status

					mockProbe.EXPECT().Probe(ctx).Return(probe.Success, nil)

					err := testWorker.DoProbe(vm)
					Expect(err).ShouldNot(HaveOccurred())

					By("Conditions in VM status should not be updated", func() {
						Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
						Expect(vm.Status.Conditions).Should(Equal(oldStatus.Conditions))
						Expect(fakeEvents).ShouldNot(Receive(ContainSubstring(readyReason)))
					})
				})
			})
		})
	})
})

func TestReadinessProbeWorker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Readiness Workers")
}

func getVirtualMachineReadinessTCPProbe(port int) *vmopv1alpha1.Probe {
	return &vmopv1alpha1.Probe{
		TCPSocket: &vmopv1alpha1.TCPSocketAction{
			Port: intstr.FromInt(port),
		},
		PeriodSeconds: 1,
	}
}
