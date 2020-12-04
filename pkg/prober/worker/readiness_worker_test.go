// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -destination=../../../mocks/mock_probe.go -package=mocks github.com/vmware-tanzu/vm-operator/pkg/prober/probe Probe

package worker

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgorecord "k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/mock/gomock"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/mocks"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
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
		queue := workqueue.NewNamedDelayingQueue("test")
		prober := probe.NewProber()
		testWorker = NewReadinessWorker(queue, prober, fakeClient, fakeRecorder)
		prober.TCPProbe = mockProbe
	})

	checkVirtualMachineCondition := func(c client.Client, objKey client.ObjectKey, expectedCondition corev1.ConditionStatus) {
		Expect(c.Get(ctx, objKey, vm)).Should(Succeed())
		condition := conditions.Get(vm, vmopv1alpha1.ReadyCondition)
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).Should(Equal(expectedCondition))
	}

	Context("VM has TCP readiness probe", func() {
		var (
			oldStatus vmopv1alpha1.VirtualMachineStatus
		)

		BeforeEach(func() {
			vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(10001)
			Expect(fakeClient.Create(ctx, vm)).To(Succeed())
			Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
			var err error
			ctx, err = testWorker.CreateProbeContext(vm)
			Expect(err).ShouldNot(HaveOccurred())
			oldStatus = vm.Status
		})

		AfterEach(func() {
			Expect(fakeClient.Delete(ctx, vm)).To(Succeed())
		})

		When("new VirtualMachineCondition is in a transition", func() {
			It("Should update the VirtualMachineCondition when probe succeeds", func() {
				mockProbe.EXPECT().Probe(gomock.Any()).Return(probe.Success, nil)

				err := testWorker.DoProbe(ctx)
				Expect(err).ShouldNot(HaveOccurred())

				By("Should set ReadyCondition status as true", func() {
					checkVirtualMachineCondition(fakeClient, vmKey, corev1.ConditionTrue)
				})

				By("Conditions in VM status should be updated", func() {
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					Expect(vm.Status.Conditions).ShouldNot(Equal(oldStatus.Conditions))
					Expect(fakeEvents).Should(Receive(ContainSubstring(readyReason)))
				})
			})

			It("Should update the VirtualMachineCondition when probe fails", func() {
				mockProbe.EXPECT().Probe(gomock.Any()).Return(probe.Failure, nil)

				err := testWorker.DoProbe(ctx)
				Expect(err).ShouldNot(HaveOccurred())

				By("Should set ReadyCondition value as false", func() {
					checkVirtualMachineCondition(fakeClient, vmKey, corev1.ConditionFalse)
				})

				By("Conditions in VM status should be updated", func() {
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					Expect(vm.Status.Conditions).ShouldNot(Equal(oldStatus.Conditions))
					Expect(fakeEvents).Should(Receive(ContainSubstring(notReadyReason)))
				})
			})

			When("new VirtualMachineCondition isn't in a transition", func() {
				It("Shouldn't update the Condition in status", func() {
					vmReadyCondition := conditions.TrueCondition(vmopv1alpha1.ReadyCondition)
					vm.Status.Conditions = append(vm.Status.Conditions, *vmReadyCondition)
					Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					oldStatus = vm.Status

					mockProbe.EXPECT().Probe(gomock.Any()).Return(probe.Success, nil)

					err := testWorker.DoProbe(ctx)
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
