// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	goctx "context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgorecord "k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
	fakeprobe "github.com/vmware-tanzu/vm-operator/pkg/prober2/fake/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine readiness probes", func() {
	var (
		testWorker Worker

		vm    *vmopv1.VirtualMachine
		vmKey client.ObjectKey
		ctx   *context.ProbeContext

		fakeClient         client.Client
		fakeRecorder       record.Recorder
		fakeEvents         chan string
		fakeTCPProbe       *fakeprobe.FakeProbe
		fakeHeartbeatProbe *fakeprobe.FakeProbe
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName: "dummy-vmclass",
			},
		}

		vmKey = client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}

		fakeClient = builder.NewFakeClient()
		eventRecorder := clientgorecord.NewFakeRecorder(1024)
		fakeRecorder = record.New(eventRecorder)
		fakeEvents = eventRecorder.Events

		queue := workqueue.NewNamedDelayingQueue("test")
		fakeTCPProbe = fakeprobe.NewFakeProbe().(*fakeprobe.FakeProbe)
		fakeHeartbeatProbe = fakeprobe.NewFakeProbe().(*fakeprobe.FakeProbe)
		prober := &probe.Prober{
			TCPProbe:       fakeTCPProbe,
			GuestHeartbeat: fakeHeartbeatProbe,
		}
		testWorker = NewReadinessWorker(queue, prober, fakeClient, fakeRecorder)
	})

	checkReadyCondition := func(c client.Client, objKey client.ObjectKey, expectedCondition metav1.ConditionStatus) {
		Expect(c.Get(ctx, objKey, vm)).Should(Succeed())
		condition := conditions.Get(vm, vmopv1.ReadyConditionType)
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).Should(Equal(expectedCondition))
	}

	Context("VM has TCP readiness probe", func() {
		var (
			oldStatus vmopv1.VirtualMachineStatus
		)

		BeforeEach(func() {
			vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(10001)
			Expect(fakeClient.Create(goctx.Background(), vm)).Should(Succeed())
			Expect(fakeClient.Get(goctx.Background(), vmKey, vm)).Should(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			ctx, err = testWorker.CreateProbeContext(vm)
			Expect(err).ShouldNot(HaveOccurred())
			oldStatus = vm.Status
		})

		When("new ReadyCondition is in a transition", func() {
			It("Should update ReadyCondition when probe succeeds", func() {
				fakeTCPProbe.ProbeFn = func(ctx *context.ProbeContext) (probe.Result, error) {
					return probe.Success, nil
				}

				Expect(testWorker.DoProbe(ctx)).Should(Succeed())

				By("Should set ReadyCondition status as true", func() {
					checkReadyCondition(fakeClient, vmKey, metav1.ConditionTrue)
				})

				By("Conditions in VM status should be updated", func() {
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					Expect(vm.Status.Conditions).ShouldNot(Equal(oldStatus.Conditions))
					Expect(fakeEvents).Should(Receive(ContainSubstring(readyReason)))
				})
			})

			It("Should update ReadyCondition when probe fails", func() {
				fakeTCPProbe.ProbeFn = func(ctx *context.ProbeContext) (probe.Result, error) {
					return probe.Failure, nil
				}

				Expect(testWorker.DoProbe(ctx)).Should(Succeed())

				By("Should set ReadyCondition value as false", func() {
					checkReadyCondition(fakeClient, vmKey, metav1.ConditionFalse)
				})

				By("Conditions in VM status should be updated", func() {
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					Expect(vm.Status.Conditions).ShouldNot(Equal(oldStatus.Conditions))
					Expect(fakeEvents).Should(Receive(ContainSubstring(notReadyReason)))
				})
			})

			When("new ReadyCondition isn't in a transition", func() {

				BeforeEach(func() {
					vmReadyCondition := conditions.TrueCondition(vmopv1.ReadyConditionType)
					vm.Status.Conditions = append(vm.Status.Conditions, *vmReadyCondition)
					Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
					Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
					oldStatus = vm.Status
				})

				It("Shouldn't update the Condition in status", func() {

					fakeTCPProbe.ProbeFn = func(ctx *context.ProbeContext) (probe.Result, error) {
						return probe.Success, nil
					}

					Expect(testWorker.DoProbe(ctx)).Should(Succeed())

					By("Conditions in VM status should not be updated", func() {
						Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
						Expect(vm.Status.Conditions).Should(Equal(oldStatus.Conditions))
						Expect(fakeEvents).ShouldNot(Receive(ContainSubstring(readyReason)))
					})
				})
			})
		})
	})

	Context("Guest heartbeat Probe", func() {

		BeforeEach(func() {
			vm.Spec.ReadinessProbe = getVirtualMachineHeartbeatProbe()
			Expect(fakeClient.Create(goctx.Background(), vm)).Should(Succeed())
			Expect(fakeClient.Get(goctx.Background(), vmKey, vm)).Should(Succeed())
			var err error
			ctx, err = testWorker.CreateProbeContext(vm)
			Expect(err).ShouldNot(HaveOccurred())
		})

		// Just need to test for probe selection.
		It("Should update ReadyCondition when probe fails", func() {
			fakeHeartbeatProbe.ProbeFn = func(ctx *context.ProbeContext) (probe.Result, error) {
				return probe.Failure, fmt.Errorf("heartbeat error")
			}

			Expect(testWorker.DoProbe(ctx)).Should(Succeed())
			Expect(fakeClient.Get(ctx, vmKey, vm)).Should(Succeed())
			condition := conditions.Get(vm, vmopv1.ReadyConditionType)
			Expect(condition).ToNot(BeNil())
			Expect(condition.Message).To(ContainSubstring("heartbeat error"))
		})
	})
})

func TestReadinessProbeWorker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Readiness Workers")
}

func getVirtualMachineReadinessTCPProbe(port int) vmopv1.VirtualMachineReadinessProbeSpec {
	return vmopv1.VirtualMachineReadinessProbeSpec{
		TCPSocket: &vmopv1.TCPSocketAction{
			Port: intstr.FromInt(port),
		},
		PeriodSeconds: 1,
	}
}

func getVirtualMachineHeartbeatProbe() vmopv1.VirtualMachineReadinessProbeSpec {
	return vmopv1.VirtualMachineReadinessProbeSpec{
		GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
		PeriodSeconds:  1,
	}
}
