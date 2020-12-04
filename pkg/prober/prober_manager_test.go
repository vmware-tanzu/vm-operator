// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	goctx "context"
	"fmt"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgorecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/worker"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine probes", func() {
	var (
		testManager   *manager
		vm            *vmopv1alpha1.VirtualMachine
		vmKey         client.ObjectKey
		vmProbe       *vmopv1alpha1.Probe
		periodSeconds int32

		fakeClient   client.Client
		fakeRecorder record.Recorder
		fakeWorkerIf worker.Worker
		fakeWorker   *fake.FakeWorker
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
		periodSeconds = 1
		vmProbe = &vmopv1alpha1.Probe{
			TCPSocket: &vmopv1alpha1.TCPSocketAction{
				Port: intstr.FromInt(10001),
			},
			PeriodSeconds: periodSeconds,
		}

		fakeClient, _ = builder.NewFakeClient()
		eventRecorder := clientgorecord.NewFakeRecorder(1024)
		fakeRecorder = record.New(eventRecorder)
		testManagerIf := NewManger(fakeClient, fakeRecorder)
		testManager = testManagerIf.(*manager)
		fakeWorkerIf = fake.NewFakeWorker(testManager.readinessQueue)
		fakeWorker = fakeWorkerIf.(*fake.FakeWorker)
	})

	AfterEach(func() {
		fakeWorker.Reset()
		fakeWorker = nil
		fakeClient = nil
		fakeRecorder = nil
		testManager = nil
	})

	checkProbeQueueLenEventually := func(interval int32, expectedLen int) {
		Eventually(func() int {
			return testManager.readinessQueue.Len()
		}, interval).Should(Equal(expectedLen))
	}

	checkProbeQueueLenConsistently := func(interval int32, expectedLen int) {
		Consistently(func() int {
			return testManager.readinessQueue.Len()
		}, interval).Should(Equal(expectedLen))
	}

	Context("Probe manager processes items from the queue", func() {
		ctx := goctx.Background()
		BeforeEach(func() {
			testManager.readinessQueue.Add(vmKey)
		})

		AfterEach(func() {
			err := fakeClient.Delete(ctx, vm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("VM doesn't specify a probe", func() {
			It("Should return immediately", func() {
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())
				quit := testManager.processItemFromQueue(fakeWorker)
				Expect(quit).To(BeFalse())
				checkProbeQueueLenConsistently(2*periodSeconds, 0)
			})
		})

		When("VM specifies a probe", func() {
			BeforeEach(func() {
				vm.Spec.ReadinessProbe = vmProbe
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())
			})

			It("Should not run probes when VM is in deleting phase", func() {
				Expect(fakeClient.Delete(ctx, vm)).To(Succeed())

				quit := testManager.processItemFromQueue(fakeWorker)
				Expect(quit).To(BeFalse())
				checkProbeQueueLenConsistently(2*periodSeconds, 0)
			})

			It("Should set probe result as failed if the VM is powered off", func() {
				vm.Status.PowerState = vmopv1alpha1.VirtualMachinePoweredOff
				Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
				fakeWorker.ProcessProbeResultFn = func(ctx *context.ProbeContext, res probe.Result, err error) error {
					if res != probe.Failure {
						return fmt.Errorf("dummy error")
					}
					return nil
				}
				quit := testManager.processItemFromQueue(fakeWorker)
				Expect(quit).To(BeFalse())

				By("Should add to queue after a time period", func() {
					checkProbeQueueLenEventually(2*periodSeconds, 1)
				})
			})

			When("VM is powered on", func() {
				BeforeEach(func() {
					vm.Status.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
					Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
				})

				It("Should immediately add to the queue if DoProbe returns error", func() {
					fakeWorker.DoProbeFn = func(ctx *context.ProbeContext) error {
						return fmt.Errorf("dummy error")
					}
					quit := testManager.processItemFromQueue(fakeWorker)
					Expect(quit).To(BeFalse())

					Expect(testManager.readinessQueue.Len()).To(Equal(1))
				})

				When("DoProbe succeeds", func() {
					BeforeEach(func() {
						fakeWorker.DoProbeFn = func(ctx *context.ProbeContext) error {
							return nil
						}
					})

					It("Should add to the queue after default value if periodSeconds is not set in Probe spec", func() {
						vm.Spec.ReadinessProbe.PeriodSeconds = 0
						Expect(fakeClient.Update(ctx, vm)).To(Succeed())
						quit := testManager.processItemFromQueue(fakeWorker)
						Expect(quit).To(BeFalse())

						By("Should add to queue after a time period", func() {
							checkProbeQueueLenEventually(2*defaultPeriodSeconds, 1)
						})
					})

					It("Should add to queue after specific time period if periodSeconds is set in probe spec", func() {
						quit := testManager.processItemFromQueue(fakeWorker)
						Expect(quit).To(BeFalse())

						By("Should add to queue after a time period", func() {
							checkProbeQueueLenEventually(2*periodSeconds, 1)
						})
					})
				})
			})
		})
	})
})

func TestProberManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Prober Manager")
}
