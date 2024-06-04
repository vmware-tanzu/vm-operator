// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgorecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	proberctx "github.com/vmware-tanzu/vm-operator/pkg/prober/context"
	fakeworker "github.com/vmware-tanzu/vm-operator/pkg/prober/fake/worker"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/worker"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine probes", func() {
	var (
		initObjects []client.Object
		ctx         context.Context

		testManager   *manager
		vm            *vmopv1.VirtualMachine
		vmKey         client.ObjectKey
		periodSeconds int32

		fakeCtrlManager pkgmgr.Manager
		fakeClient      client.Client
		fakeRecorder    record.Recorder
		fakeWorkerIf    worker.Worker
		fakeWorker      *fakeworker.FakeWorker
		fakeVMProvider  providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		ctx = context.Background()
		periodSeconds = 1

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName: "dummy-vmclass",
				ReadinessProbe: &vmopv1.VirtualMachineReadinessProbeSpec{
					TCPSocket: &vmopv1.TCPSocketAction{
						Port: intstr.FromInt(10001),
					},
					PeriodSeconds: periodSeconds,
				},
			},
		}
		vmKey = client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}

		initObjects = append(initObjects, vm)
	})

	JustBeforeEach(func() {
		fakeClient = builder.NewFakeClient(initObjects...)
		eventRecorder := clientgorecord.NewFakeRecorder(1024)
		fakeRecorder = record.New(eventRecorder)
		fakeVMProvider = &fake.VMProvider{}
		fakeCtrlManager = &fakeManager{client: fakeClient}
		testManagerIf := NewManager(fakeClient, fakeRecorder, fakeVMProvider)
		testManager = testManagerIf.(*manager)
		fakeWorkerIf = fakeworker.NewFakeWorker(testManager.readinessQueue)
		fakeWorker = fakeWorkerIf.(*fakeworker.FakeWorker)
	})

	AfterEach(func() {
		fakeWorker.Reset()
		fakeWorker = nil
		fakeClient = nil
		fakeRecorder = nil
		fakeCtrlManager = nil
		fakeVMProvider = nil
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

	Specify("Adding prober to controller manager should succeed", func() {
		m, err := AddToManager(fakeCtrlManager, fakeVMProvider)
		Expect(err).ToNot(HaveOccurred())
		Expect(m).ToNot(BeNil())
	})

	Specify("Starting probe manager should succeed", func() {
		m, err := AddToManager(fakeCtrlManager, fakeVMProvider)
		Expect(err).ToNot(HaveOccurred())
		Expect(m).ToNot(BeNil())

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		Expect(m.Start(ctx)).To(Succeed())
	})

	Specify("Removing probe from prober should succeed", func() {
		testManager.RemoveFromProberManager(&vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "hello",
				Name:      "world",
			},
		})
	})

	Context("Probe manager processes items from the queue", func() {
		JustBeforeEach(func() {
			testManager.readinessQueue.Add(vmKey)
		})

		When("VM doesn't specify a probe", func() {
			BeforeEach(func() {
				vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{}
			})

			It("Should return immediately", func() {
				quit := testManager.processItemFromQueue(fakeWorker)
				Expect(quit).To(BeFalse())
				checkProbeQueueLenConsistently(2*periodSeconds, 0)
			})
		})

		When("VM specifies a probe", func() {

			It("Should not run probes when VM is in deleting phase", func() {
				Expect(fakeClient.Delete(ctx, vm)).To(Succeed())

				quit := testManager.processItemFromQueue(fakeWorker)
				Expect(quit).To(BeFalse())
				checkProbeQueueLenConsistently(2*periodSeconds, 0)
			})

			It("Should set probe result as failed if the VM is powered off", func() {
				vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
				fakeWorker.ProcessProbeResultFn = func(ctx *proberctx.ProbeContext, res probe.Result, err error) error {
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
					vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
				})

				It("Should immediately add to the queue if DoProbe returns error", func() {
					fakeWorker.DoProbeFn = func(ctx *proberctx.ProbeContext) error {
						return fmt.Errorf("dummy error")
					}
					quit := testManager.processItemFromQueue(fakeWorker)
					Expect(quit).To(BeFalse())

					Expect(testManager.readinessQueue.Len()).To(Equal(1))
				})

				When("DoProbe succeeds", func() {
					JustBeforeEach(func() {
						fakeWorker.DoProbeFn = func(ctx *proberctx.ProbeContext) error {
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

		JustBeforeEach(func() {
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(fakeClient.Status().Update(ctx, vm)).To(Succeed())
		})

		It("Should not add to prober manager if Probe spec is not specified", func() {
			vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{}
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
			var newVM *vmopv1.VirtualMachine
			JustBeforeEach(func() {
				testManager.AddToProberManager(vm)
				Expect(testManager.readinessQueue.Len()).To(Equal(1))

				fakeWorker.DoProbeFn = func(ctx *proberctx.ProbeContext) error {
					return nil
				}
				Expect(testManager.processItemFromQueue(fakeWorker)).To(BeFalse())

				newVM = &vmopv1.VirtualMachine{}
				Expect(fakeClient.Get(ctx, vmKey, newVM)).To(Succeed())
			})

			It("Should do nothing if VM probe is not updated", func() {
				newVM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{}
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
				newVM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{}
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

type fakeManager struct {
	pkgmgr.Manager
	scheme *runtime.Scheme
	client client.Client
}

func (f fakeManager) GetEventRecorderFor(name string) clientgorecord.EventRecorder {
	return nil
}

func (f fakeManager) GetScheme() *runtime.Scheme {
	return f.scheme
}

func (f fakeManager) GetClient() client.Client {
	return f.client
}

func (f fakeManager) Add(_ ctrlmgr.Runnable) error {
	return nil
}
