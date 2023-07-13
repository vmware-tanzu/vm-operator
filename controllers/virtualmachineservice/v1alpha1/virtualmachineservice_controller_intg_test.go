// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizerName = "virtualmachineservice.vmoperator.vmware.com"
)

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext

		selector      map[string]string
		vmLabels      map[string]string
		vmIP1, vmIP2  string
		vmServicePort vmopv1.VirtualMachineServicePort
		vmServiceName string
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		selector = map[string]string{"vmservice-intg-test": "selector"}
		vmLabels = map[string]string{"vmservice-intg-test": "selector", "other": "label"}
		vmIP1, vmIP2 = "10.100.101.1", "10.100.101.2"

		vmServicePort = vmopv1.VirtualMachineServicePort{
			Name:       "port1",
			Protocol:   "TCP",
			Port:       42,
			TargetPort: 142,
		}
	})

	AfterEach(func() {
		By("Object cleanup", func() {
			objMeta := metav1.ObjectMeta{Name: vmServiceName, Namespace: ctx.Namespace}

			vmService := &vmopv1.VirtualMachineService{ObjectMeta: objMeta}
			err := ctx.Client.Delete(ctx, vmService)
			Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

			service := &corev1.Service{ObjectMeta: objMeta}
			err = ctx.Client.Delete(ctx, service)
			Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

			endpoints := &corev1.Endpoints{ObjectMeta: objMeta}
			err = ctx.Client.Delete(ctx, endpoints)
			Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
		})

		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		Context("Reconciles after VirtualMachineService creation", func() {
			BeforeEach(func() {
				vmServiceName = "test-vm-service"
			})

			It("Simulate workflow", func() {
				dummyAnnotationKey, dummyAnnotationVal := "dummy-annotation-key", "dummy-annotation-val"
				dummyLabelKey, dummyLabelVal := "dummy-label-key", "dummy-label-val"

				notReadyVM := &vmopv1.VirtualMachine{}
				By("Create not ready VM with selected labels", func() {
					notReadyVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "not-ready-vm",
							Namespace: ctx.Namespace,
							Labels:    vmLabels,
						},
						Spec: vmopv1.VirtualMachineSpec{
							PowerState:     vmopv1.VirtualMachinePoweredOn,
							ReadinessProbe: &vmopv1.Probe{},
						},
					}
					Expect(ctx.Client.Create(ctx, notReadyVM)).To(Succeed())
					notReadyVM.Status.VmIp = vmIP1
					conditions.MarkFalse(notReadyVM, vmopv1.ReadyCondition, "not ready",
						vmopv1.ConditionSeverityError, "VM should be skipped")
					Expect(ctx.Client.Status().Update(ctx, notReadyVM))
				})

				vmService := &vmopv1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      vmServiceName,
						Namespace: ctx.Namespace,
						Annotations: map[string]string{
							dummyAnnotationKey: dummyAnnotationVal,
						},
						Labels: map[string]string{
							dummyLabelKey: dummyLabelVal,
						},
					},
					Spec: vmopv1.VirtualMachineServiceSpec{
						Type:     vmopv1.VirtualMachineServiceTypeLoadBalancer,
						Ports:    []vmopv1.VirtualMachineServicePort{vmServicePort},
						Selector: selector,
					},
				}

				objKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}

				By("Create VirtualMachineService", func() {
					Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())
				})

				By("VirtualMachineService finalizer should get set")
				Eventually(func() []string {
					vmService := &vmopv1.VirtualMachineService{}
					if err := ctx.Client.Get(ctx, objKey, vmService); err == nil {
						return vmService.GetFinalizers()
					}
					return nil
				}).Should(ContainElement(finalizerName))

				By("Service should be created", func() {
					service := &corev1.Service{}
					Eventually(func() error {
						return ctx.Client.Get(ctx, objKey, service)
					}).Should(Succeed())

					// Service should have label and annotations replicated on vmService create
					Expect(service.Labels).To(HaveKeyWithValue(dummyLabelKey, dummyLabelVal))
					Expect(service.Annotations).To(HaveKeyWithValue(dummyAnnotationKey, dummyAnnotationVal))

					Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
					Expect(service.Spec.Selector).To(BeEmpty())
					Expect(service.Status.LoadBalancer.Ingress).To(BeEmpty())
				})

				By("Endpoints should be created", func() {
					endpoints := &corev1.Endpoints{}
					Eventually(func() error {
						return ctx.Client.Get(ctx, objKey, endpoints)
					}).Should(Succeed())

					Expect(endpoints.Labels).To(HaveKeyWithValue(dummyLabelKey, dummyLabelVal))
					Expect(endpoints.Annotations).To(HaveKeyWithValue(dummyAnnotationKey, dummyAnnotationVal))

					Expect(endpoints.Subsets).To(HaveLen(1))
					subset := endpoints.Subsets[0]
					Expect(subset.Addresses).To(BeEmpty())

					By("Not ready VM should be included to Endpoints", func() {
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(notReadyVM), notReadyVM)).To(Succeed())

						Expect(subset.NotReadyAddresses).To(HaveLen(1))
						address := subset.NotReadyAddresses[0]
						Expect(address.IP).To(Equal(notReadyVM.Status.VmIp))
						Expect(address.TargetRef).ToNot(BeNil())
						Expect(address.TargetRef.Name).To(Equal(notReadyVM.Name))
						Expect(address.TargetRef.Namespace).To(Equal(notReadyVM.Namespace))
						Expect(address.TargetRef.UID).To(Equal(notReadyVM.UID))
						Expect(address.TargetRef.Kind).ToNot(BeEmpty())
						Expect(address.TargetRef.APIVersion).ToNot(BeEmpty())
					})
				})

				readyVM := &vmopv1.VirtualMachine{}
				By("Create ready VM with selected labels,", func() {
					readyVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ready-vm",
							Namespace: ctx.Namespace,
							Labels:    vmLabels,
						},
						Spec: vmopv1.VirtualMachineSpec{
							PowerState:     vmopv1.VirtualMachinePoweredOn,
							ReadinessProbe: &vmopv1.Probe{},
						},
					}
					Expect(ctx.Client.Create(ctx, readyVM)).To(Succeed())

					readyVM.Status.VmIp = vmIP2
					conditions.MarkTrue(readyVM, vmopv1.ReadyCondition)
					Expect(ctx.Client.Status().Update(ctx, readyVM)).To(Succeed())
				})

				By("Ready VM should be added to Endpoints", func() {
					endpoints := &corev1.Endpoints{}
					Eventually(func() bool {
						if err := ctx.Client.Get(ctx, objKey, endpoints); err == nil && len(endpoints.Subsets) == 1 {
							subset := endpoints.Subsets[0]
							return len(subset.Addresses) != 0 && len(subset.NotReadyAddresses) != 0
						}
						return false
					}).Should(BeTrue())

					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))

					subset := subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))
					Expect(subset.NotReadyAddresses).To(HaveLen(1))

					address := subset.Addresses[0]
					Expect(address.IP).To(Equal(readyVM.Status.VmIp))
					Expect(address.TargetRef).ToNot(BeNil())
					Expect(address.TargetRef.Name).To(Equal(readyVM.Name))
					Expect(address.TargetRef.Namespace).To(Equal(readyVM.Namespace))
					Expect(address.TargetRef.UID).To(Equal(readyVM.UID))
					Expect(address.TargetRef.Kind).ToNot(BeEmpty())
					Expect(address.TargetRef.APIVersion).ToNot(BeEmpty())

					Expect(subset.Ports).To(HaveLen(1))
					port := subset.Ports[0]
					Expect(port.Name).To(Equal(port.Name))
					Expect(port.Port).To(BeEquivalentTo(vmServicePort.TargetPort))
					Expect(port.Protocol).To(BeEquivalentTo(corev1.ProtocolTCP))
				})

				By("Deleted VM should be removed from Endpoints", func() {
					// Must add finalizer here so that the VM does not get deleted immediately, as our
					// VM mapping function assumes that the VM exists. This is a bug, and should have
					// a similar solution as the XIt() test below. In practice, this should be hard to
					// hit because of the VirtualMachine controller finalizer.
					notReadyVM.Finalizers = append(notReadyVM.Finalizers, "dummy.test.finalizer")
					Expect(ctx.Client.Update(ctx, notReadyVM)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, notReadyVM)).To(Succeed())

					Eventually(func() bool {
						endpoints := &corev1.Endpoints{}
						if err := ctx.Client.Get(ctx, objKey, endpoints); err == nil {
							return len(endpoints.Subsets) == 1 && len(endpoints.Subsets[0].NotReadyAddresses) == 0
						}
						return false
					}).Should(BeTrue(), "not ready VM should be removed from Endpoints")

					readyVM.Finalizers = append(readyVM.Finalizers, "dummy.test.finalizer")
					Expect(ctx.Client.Update(ctx, readyVM)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, readyVM)).To(Succeed())

					Eventually(func() bool {
						endpoints := &corev1.Endpoints{}
						if err := ctx.Client.Get(ctx, objKey, endpoints); err == nil {
							return len(endpoints.Subsets) == 0
						}
						return false
					}).Should(BeTrue())

					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(notReadyVM), notReadyVM)).To(Succeed())
					notReadyVM.Finalizers = nil
					Expect(ctx.Client.Update(ctx, notReadyVM)).To(Succeed())

					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(readyVM), readyVM)).To(Succeed())
					readyVM.Finalizers = nil
					Expect(ctx.Client.Update(ctx, readyVM)).To(Succeed())
				})

				By("Delete VirtualMachineService and finalizer should be removed", func() {
					Expect(ctx.Client.Delete(ctx, vmService)).To(Succeed())

					Eventually(func() []string {
						vmService := &vmopv1.VirtualMachineService{}
						if err := ctx.Client.Get(ctx, objKey, vmService); err == nil {
							return vmService.GetFinalizers()
						}
						return nil
					}).ShouldNot(ContainElement(finalizerName))
				})
			})
		})

		Context("Reconciles after VirtualMachineService after VMs labels no longer match", func() {
			BeforeEach(func() {
				vmServiceName = "test-vm-service-unmatched-vm"
			})

			// The VM mapping function needs to be fixed for this to work.
			XIt("Simulate workflow", func() {
				readyVM := &vmopv1.VirtualMachine{}
				By("Create ready VM with selected labels", func() {
					readyVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "not-ready-vm",
							Namespace: ctx.Namespace,
							Labels:    vmLabels,
						},
						Spec: vmopv1.VirtualMachineSpec{
							PowerState:     vmopv1.VirtualMachinePoweredOn,
							ReadinessProbe: &vmopv1.Probe{},
						},
					}
					Expect(ctx.Client.Create(ctx, readyVM)).To(Succeed())
					readyVM.Status.VmIp = vmIP1
					conditions.MarkTrue(readyVM, vmopv1.ReadyCondition)
					Expect(ctx.Client.Status().Update(ctx, readyVM))
				})

				vmService := &vmopv1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      vmServiceName,
						Namespace: ctx.Namespace,
					},
					Spec: vmopv1.VirtualMachineServiceSpec{
						Type:     vmopv1.VirtualMachineServiceTypeLoadBalancer,
						Ports:    []vmopv1.VirtualMachineServicePort{vmServicePort},
						Selector: selector,
					},
				}

				objKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}

				By("Create VirtualMachineService", func() {
					Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())
				})

				By("Ready VM should be added to Endpoints", func() {
					endpoints := &corev1.Endpoints{}
					Eventually(func() bool {
						if err := ctx.Client.Get(ctx, objKey, endpoints); err == nil {
							return len(endpoints.Subsets) != 0
						}
						return false
					}).Should(BeTrue())

					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))

					subset := subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))

					address := subset.Addresses[0]
					Expect(address.IP).To(Equal(readyVM.Status.VmIp))

					Expect(subset.Ports).To(HaveLen(1))
					port := subset.Ports[0]
					Expect(port.Name).To(Equal(port.Name))
					Expect(port.Port).To(BeEquivalentTo(vmServicePort.TargetPort))
					Expect(port.Protocol).To(BeEquivalentTo(corev1.ProtocolTCP))
				})

				By("Change VM Labels so it no longer matches selector", func() {
					readyVM.Labels = map[string]string{"some-other": "label"}
					Expect(ctx.Client.Update(ctx, readyVM)).To(Succeed())
				})

				By("VM should be removed from Endpoints", func() {
					Eventually(func() bool {
						endpoints := &corev1.Endpoints{}
						if err := ctx.Client.Get(ctx, objKey, endpoints); err == nil {
							return len(endpoints.Subsets) == 0
						}
						return false
					}).Should(BeTrue())
				})
			})
		})
	})
}
