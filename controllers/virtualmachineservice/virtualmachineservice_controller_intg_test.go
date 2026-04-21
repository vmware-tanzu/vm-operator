// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizerName = "vmoperator.vmware.com/virtualmachineservice"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
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
							PowerState: vmopv1.VirtualMachinePowerStateOn,
							ReadinessProbe: &vmopv1.VirtualMachineReadinessProbeSpec{
								TCPSocket: &vmopv1.TCPSocketAction{},
							},
						},
					}
					Expect(ctx.Client.Create(ctx, notReadyVM)).To(Succeed())
					notReadyVM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{PrimaryIP4: vmIP1}
					conditions.MarkFalse(notReadyVM, vmopv1.ReadyConditionType, "not_ready", "VM should be skipped")
					Expect(ctx.Client.Status().Update(ctx, notReadyVM)).To(Succeed())
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
				Eventually(func(g Gomega) {
					vmService := &vmopv1.VirtualMachineService{}

					g.Expect(ctx.Client.Get(ctx, objKey, vmService)).To(Succeed())
					g.Expect(vmService.GetFinalizers()).To(ContainElement(finalizerName))
				}).Should(Succeed())

				By("Service should be created", func() {
					service := &corev1.Service{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
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
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
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
						Expect(notReadyVM.Status.Network).ToNot(BeNil())
						Expect(address.IP).To(Equal(notReadyVM.Status.Network.PrimaryIP4))
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
							PowerState: vmopv1.VirtualMachinePowerStateOn,
							ReadinessProbe: &vmopv1.VirtualMachineReadinessProbeSpec{
								GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
							},
						},
					}
					Expect(ctx.Client.Create(ctx, readyVM)).To(Succeed())

					readyVM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{PrimaryIP4: vmIP2}
					conditions.MarkTrue(readyVM, vmopv1.ReadyConditionType)
					Expect(ctx.Client.Status().Update(ctx, readyVM)).To(Succeed())
				})

				By("Ready VM should be added to Endpoints", func() {
					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).To(HaveLen(1))
						subset := endpoints.Subsets[0]
						g.Expect(subset.Addresses).ToNot(BeEmpty())
						g.Expect(subset.NotReadyAddresses).ToNot(BeEmpty())
					}).Should(Succeed())

					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))

					subset := subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))
					Expect(subset.NotReadyAddresses).To(HaveLen(1))

					address := subset.Addresses[0]
					Expect(readyVM.Status.Network).ToNot(BeNil())
					Expect(address.IP).To(Equal(readyVM.Status.Network.PrimaryIP4))
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

					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).To(HaveLen(1))
						g.Expect(endpoints.Subsets[0].NotReadyAddresses).To(BeEmpty())
					}).Should(Succeed(), "not ready VM should be removed from Endpoints")

					readyVM.Finalizers = append(readyVM.Finalizers, "dummy.test.finalizer")
					Expect(ctx.Client.Update(ctx, readyVM)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, readyVM)).To(Succeed())

					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).To(BeEmpty())
					}).Should(Succeed())

					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(notReadyVM), notReadyVM)).To(Succeed())
					notReadyVM.Finalizers = nil
					Expect(ctx.Client.Update(ctx, notReadyVM)).To(Succeed())

					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(readyVM), readyVM)).To(Succeed())
					readyVM.Finalizers = nil
					Expect(ctx.Client.Update(ctx, readyVM)).To(Succeed())
				})

				By("Delete VirtualMachineService and finalizer should be removed", func() {
					Expect(ctx.Client.Delete(ctx, vmService)).To(Succeed())

					Eventually(func(g Gomega) {
						got := &vmopv1.VirtualMachineService{}
						err := ctx.Client.Get(ctx, objKey, got)
						if apierrors.IsNotFound(err) {
							return
						}
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(got.GetFinalizers()).NotTo(ContainElement(finalizerName))
					}).Should(Succeed())
				})
			})
		})

		Context("Endpoints reflect Service IP family selection", func() {
			var vmForIPFamilyTest *vmopv1.VirtualMachine

			AfterEach(func() {
				if vmForIPFamilyTest != nil {
					Expect(client.IgnoreNotFound(ctx.Client.Delete(ctx, vmForIPFamilyTest))).To(Succeed())
					vmForIPFamilyTest = nil
				}
			})

			Context("Dual-stack VM with SingleStack IPv4 service", func() {
				BeforeEach(func() {
					vmServiceName = "test-vm-service-dual-stack"
				})

				It("includes only IPv4 when VM has both primary IPv4 and IPv6", func() {
					vmForIPFamilyTest = createVMWithNetwork(ctx, "dual-stack-vm", vmLabels, "192.168.1.100", "2001:db8::100")
					vmService := createVMService(ctx, vmServiceName, selector, vmServicePort,
						[]corev1.IPFamily{corev1.IPv4Protocol}, nil)

					objKey := client.ObjectKeyFromObject(vmService)
					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).ToNot(BeEmpty())
					}).Should(Succeed())

					allIPs := collectEndpointIPs(endpoints)
					Expect(allIPs).To(ContainElement("192.168.1.100"))
					Expect(allIPs).NotTo(ContainElement("2001:db8::100"))
				})
			})

			Context("IPv6-only VM with SingleStack IPv4 service", func() {
				BeforeEach(func() {
					vmServiceName = "test-vm-service-ipv6-only"
				})

				It("has empty Endpoints because IPv4 service cannot route to IPv6-only VM", func() {
					vmForIPFamilyTest = createVMWithNetwork(ctx, "ipv6-vm", vmLabels, "", "2001:db8::200")
					vmService := createVMService(ctx, vmServiceName, selector, vmServicePort,
						[]corev1.IPFamily{corev1.IPv4Protocol}, ptr.To(corev1.IPFamilyPolicySingleStack))

					objKey := client.ObjectKeyFromObject(vmService)
					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
					}).Should(Succeed())

					Expect(endpoints.Subsets).To(BeEmpty())
				})
			})

			Context("PreferDualStack with empty ipFamilies", func() {
				BeforeEach(func() {
					vmServiceName = "test-vm-service-prefer-empty-ipf"
				})

				// This should succeed since envtest is single stack.
				It("creates child Service and IPv4 Endpoints", func() {
					vmForIPFamilyTest = createVMWithNetwork(ctx, "vm-prefer-empty-ipf", vmLabels, "192.168.6.1", "2001:db8::601")
					vmService := createVMService(ctx, vmServiceName, selector, vmServicePort,
						nil, ptr.To(corev1.IPFamilyPolicyPreferDualStack))

					svcKey := client.ObjectKeyFromObject(vmService)
					svc := &corev1.Service{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, svcKey, svc)).To(Succeed())
					}).Should(Succeed())

					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, svcKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).ToNot(BeEmpty())
					}).Should(Succeed())

					Expect(collectEndpointIPs(endpoints)).To(ContainElement("192.168.6.1"))
				})
			})

			// This should fail since envtest is single stack.

			Context("RequireDualStack with empty ipFamilies", func() {
				BeforeEach(func() {
					vmServiceName = "test-vm-service-require-empty-ipf"
				})

				It("does not create a child Service", func() {
					vmForIPFamilyTest = createVMWithNetwork(ctx, "vm-require-empty-ipf", vmLabels, "192.168.7.1", "2001:db8::701")
					vmService := createVMService(ctx, vmServiceName, selector, vmServicePort,
						nil, ptr.To(corev1.IPFamilyPolicyRequireDualStack))

					svcKey := client.ObjectKeyFromObject(vmService)
					svc := &corev1.Service{}
					Consistently(func(g Gomega) {
						err := ctx.Client.Get(ctx, svcKey, svc)
						g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
					}).Should(Succeed())
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
							Name:      "ready-vm",
							Namespace: ctx.Namespace,
							Labels:    vmLabels,
						},
						Spec: vmopv1.VirtualMachineSpec{
							PowerState: vmopv1.VirtualMachinePowerStateOn,
							ReadinessProbe: &vmopv1.VirtualMachineReadinessProbeSpec{
								TCPSocket: &vmopv1.TCPSocketAction{},
							},
						},
					}
					Expect(ctx.Client.Create(ctx, readyVM)).To(Succeed())
					readyVM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{PrimaryIP4: vmIP1}
					conditions.MarkTrue(readyVM, vmopv1.ReadyConditionType)
					Expect(ctx.Client.Status().Update(ctx, readyVM)).To(Succeed())
				})

				vmService := &vmopv1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      vmServiceName,
						Namespace: ctx.Namespace,
					},
					Spec: vmopv1.VirtualMachineServiceSpec{
						Type:       vmopv1.VirtualMachineServiceTypeLoadBalancer,
						Ports:      []vmopv1.VirtualMachineServicePort{vmServicePort},
						Selector:   selector,
						IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol},
					},
				}

				objKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}

				By("Create VirtualMachineService", func() {
					Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())
				})

				By("Ready VM should be added to Endpoints", func() {
					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).ToNot(BeEmpty())
					}).Should(Succeed())

					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))

					subset := subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))

					address := subset.Addresses[0]
					Expect(address.IP).To(Equal(readyVM.Status.Network.PrimaryIP4))

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
					endpoints := &corev1.Endpoints{}
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
						g.Expect(endpoints.Subsets).To(BeEmpty())
					}).Should(Succeed())
				})
			})
		})
	})
}

func createVMWithNetwork(ctx *builder.IntegrationTestContext, name string, labels map[string]string, ip4, ip6 string) *vmopv1.VirtualMachine {
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ctx.Namespace,
			Labels:    labels,
		},
		Spec: vmopv1.VirtualMachineSpec{
			PowerState: vmopv1.VirtualMachinePowerStateOn,
		},
	}
	Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

	vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
		PrimaryIP4: ip4,
		PrimaryIP6: ip6,
	}
	Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
	return vm
}

func createVMService(
	ctx *builder.IntegrationTestContext,
	name string,
	selector map[string]string,
	port vmopv1.VirtualMachineServicePort,
	ipFamilies []corev1.IPFamily,
	policy *corev1.IPFamilyPolicy,
) *vmopv1.VirtualMachineService {
	vmService := &vmopv1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ctx.Namespace,
		},
		Spec: vmopv1.VirtualMachineServiceSpec{
			Type:           vmopv1.VirtualMachineServiceTypeClusterIP,
			Ports:          []vmopv1.VirtualMachineServicePort{port},
			Selector:       selector,
			IPFamilies:     ipFamilies,
			IPFamilyPolicy: policy,
		},
	}
	Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())
	return vmService
}

func collectEndpointIPs(endpoints *corev1.Endpoints) []string {
	var ips []string
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			ips = append(ips, addr.IP)
		}
	}
	return ips
}
