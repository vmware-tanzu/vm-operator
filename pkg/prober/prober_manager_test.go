// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	goctx "context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgorecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
	fakeworker "github.com/vmware-tanzu/vm-operator/pkg/prober/fake/worker"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/worker"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine probes", func() {
	var (
		initObjects []client.Object
		ctx         goctx.Context

		testManager   *manager
		vm            *vmopv1alpha1.VirtualMachine
		vmKey         client.ObjectKey
		vmProbe       *vmopv1alpha1.Probe
		periodSeconds int32

		fakeClient   client.Client
		fakeRecorder record.Recorder
		fakeWorkerIf worker.Worker
		fakeWorker   *fakeworker.FakeWorker
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
		ctx = goctx.Background()
		initObjects = append(initObjects, vm)
	})

	JustBeforeEach(func() {
		fakeClient = builder.NewFakeClient(initObjects...)
		eventRecorder := clientgorecord.NewFakeRecorder(1024)
		fakeRecorder = record.New(eventRecorder)
		fakeVmProvider := &fake.FakeVmProvider{}
		testManagerIf := NewManger(fakeClient, fakeRecorder, fakeVmProvider)
		testManager = testManagerIf.(*manager)
		fakeWorkerIf = fakeworker.NewFakeWorker(testManager.readinessQueue)
		fakeWorker = fakeWorkerIf.(*fakeworker.FakeWorker)
	})

	AfterEach(func() {
		fakeWorker.Reset()
		fakeWorker = nil
		fakeClient = nil
		fakeRecorder = nil
		testManager = nil
		initObjects = nil
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
		JustBeforeEach(func() {
			testManager.readinessQueue.Add(vmKey)
		})

		When("VM doesn't specify a probe", func() {
			It("Should return immediately", func() {
				quit := testManager.processItemFromQueue(fakeWorker)
				Expect(quit).To(BeFalse())
				checkProbeQueueLenConsistently(2*periodSeconds, 0)
			})
		})

		When("VM specifies a probe", func() {
			BeforeEach(func() {
				vm.Spec.ReadinessProbe = vmProbe
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
				JustBeforeEach(func() {
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
					JustBeforeEach(func() {
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

	Context("Probe manager manages VMs when they are created, updated or deleted", func() {
		var err error
		BeforeEach(func() {
			vmProbe.PeriodSeconds = defaultPeriodSeconds
			vm.Spec.ReadinessProbe = vmProbe
		})

		JustBeforeEach(func() {
			vm.Status.PowerState = vmopv1alpha1.VirtualMachinePoweredOn
			Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())

			Expect(fakeClient.Get(ctx, vmKey, vm)).To(Succeed())
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should not add to prober manager if Probe spec is not specified", func() {
			vm.Spec.ReadinessProbe = nil
			testManager.AddToProberManager(vm)

			Expect(testManager.readinessQueue.Len()).To(Equal(0))
			testManager.readinessMutex.Lock()
			Expect(testManager.vmReadinessProbeList).ShouldNot(HaveKey(vm.NamespacedName()))
			testManager.readinessMutex.Unlock()
		})

		When("VM is first time being added to the prober manager", func() {
			It("Should add to the queue and list", func() {
				testManager.AddToProberManager(vm)

				Expect(testManager.readinessQueue.Len()).To(Equal(1))
				testManager.readinessMutex.Lock()
				Expect(testManager.vmReadinessProbeList).Should(HaveKey(vm.NamespacedName()))
				testManager.readinessMutex.Unlock()
			})
		})

		When("VM has already been added to the prober manager", func() {
			var newVM *vmopv1alpha1.VirtualMachine
			JustBeforeEach(func() {
				testManager.AddToProberManager(vm)
				Expect(testManager.readinessQueue.Len()).To(Equal(1))

				fakeWorker.DoProbeFn = func(ctx *context.ProbeContext) error {
					return nil
				}
				Expect(testManager.processItemFromQueue(fakeWorker)).To(BeFalse())

				newVM = &vmopv1alpha1.VirtualMachine{}
				Expect(fakeClient.Get(ctx, vmKey, newVM)).To(Succeed())
			})

			It("Should do nothing if VM probe is not updated", func() {
				testManager.AddToProberManager(newVM)
				Expect(testManager.readinessQueue.Len()).To(Equal(0))
			})

			It("Should add to queue immediately if the VM's probe spec is updated", func() {
				newVM.Spec.ReadinessProbe.PeriodSeconds = 0
				Expect(fakeClient.Update(ctx, newVM)).To(Succeed())

				testManager.AddToProberManager(newVM)

				Expect(testManager.readinessQueue.Len()).To(Equal(1))
			})

			It("Should remove from the manager if the VM's probe spec is changed to nil", func() {
				newVM.Spec.ReadinessProbe = nil
				Expect(fakeClient.Update(ctx, newVM)).To(Succeed())

				testManager.AddToProberManager(newVM)

				Expect(testManager.readinessQueue.Len()).To(Equal(0))
				testManager.readinessMutex.Lock()
				Expect(testManager.vmReadinessProbeList).ShouldNot(HaveKey(vm.NamespacedName()))
				testManager.readinessMutex.Unlock()
			})
		})
	})
})

func TestProberManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Prober Manager")
}
