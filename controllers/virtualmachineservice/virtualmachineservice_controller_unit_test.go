// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
	Describe("Invoking NSXT Reconcile", nsxtLBProviderTestsReconcile)
}

const LabelServiceProxyName = "service.kubernetes.io/service-proxy-name"

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler   *virtualmachineservice.ReconcileVirtualMachineService
		vmServiceCtx *vmopContext.VirtualMachineServiceContext

		vmService      *vmopv1alpha1.VirtualMachineService
		vmServicePort1 vmopv1alpha1.VirtualMachineServicePort
		vmServicePort2 vmopv1alpha1.VirtualMachineServicePort
		lbSourceRanges []string
		objKey         client.ObjectKey
	)

	const (
		externalName   = "my-external-name"
		clusterIP      = "192.168.100.42"
		loadBalancerIP = "1.1.1.42"

		annotationName1  = "my-annotation-1"
		annotationValue1 = "bar1"
		annotationName2  = "my-annotation-2"
		annotationValue2 = "bar2"
		labelName1       = "my-label-1"
		labelValue1      = "bar3"
		labelName2       = "my-label-2"
		labelValue2      = "bar4"
	)

	BeforeEach(func() {
		vmService = &vmopv1alpha1.VirtualMachineService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dummy-vm-service",
				Namespace:   "dummy-ns",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: vmopv1alpha1.VirtualMachineServiceSpec{
				Type:                     vmopv1alpha1.VirtualMachineServiceTypeLoadBalancer,
				Selector:                 map[string]string{},
				ExternalName:             externalName,
				ClusterIP:                clusterIP,
				LoadBalancerIP:           loadBalancerIP,
				LoadBalancerSourceRanges: lbSourceRanges,
			},
		}

		vmServicePort1 = vmopv1alpha1.VirtualMachineServicePort{
			Name:       "port1",
			Protocol:   "TCP",
			Port:       42,
			TargetPort: 142,
		}

		vmServicePort2 = vmopv1alpha1.VirtualMachineServicePort{
			Name:       "port2",
			Protocol:   "UDP",
			Port:       1042,
			TargetPort: 1142,
		}

		lbSourceRanges = []string{"1.1.1.0/24", "2.2.0.0/16"}

		objKey = client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachineservice.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Scheme,
			ctx.Recorder,
			providers.NoopLoadbalancerProvider{},
		)

		vmServiceCtx = &vmopContext.VirtualMachineServiceContext{
			Context:   ctx,
			Logger:    ctx.Logger.WithName(vmService.Name),
			VMService: vmService,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmServiceCtx = nil
		reconciler = nil
	})

	Context("ReconcileNormal", func() {
		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vmService.Finalizers = nil
			})

			It("will set finalizer", func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmServiceCtx.VMService.GetFinalizers()).To(ContainElement(finalizerName))
			})
		})

		It("will have finalizer set upon successful reconciliation", func() {
			err := reconciler.ReconcileNormal(vmServiceCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmServiceCtx.VMService.GetFinalizers()).To(ContainElement(finalizerName))
		})

		Context("Creates expected Service", func() {
			var service *corev1.Service

			BeforeEach(func() {
				service = &corev1.Service{}
			})

			JustBeforeEach(func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())

				Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpCreate)))
				Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
			})

			It("With Expected OwnerReference", func() {
				ownerRefs := service.GetOwnerReferences()
				Expect(ownerRefs).To(HaveLen(1))
				ownerRef := ownerRefs[0]
				Expect(ownerRef.Name).To(Equal(vmService.Name))
				Expect(ownerRef.Controller).To(Equal(pointer.BoolPtr(true)))
			})

			It("With Expected Spec", func() {
				Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				Expect(service.Spec.ExternalName).To(Equal(externalName))
				Expect(service.Spec.ClusterIP).To(Equal(clusterIP))
				Expect(service.Spec.LoadBalancerIP).To(Equal(loadBalancerIP))
				Expect(service.Spec.LoadBalancerSourceRanges).To(HaveLen(2))
				Expect(service.Spec.LoadBalancerSourceRanges).To(ContainElements(lbSourceRanges))
			})

			Context("With Expected Spec.Ports", func() {
				BeforeEach(func() {
					vmService.Spec.Ports = []vmopv1alpha1.VirtualMachineServicePort{
						vmServicePort1,
						vmServicePort2,
					}
				})

				It("Service ports", func() {
					ports := service.Spec.Ports
					Expect(ports).To(HaveLen(2))

					port := ports[0]
					Expect(port.Name).To(Equal(vmServicePort1.Name))
					Expect(port.Protocol).To(BeEquivalentTo(vmServicePort1.Protocol))
					Expect(port.Port).To(Equal(vmServicePort1.Port))
					Expect(port.TargetPort.IntValue()).To(Equal(int(vmServicePort1.TargetPort)))

					port = ports[1]
					Expect(port.Name).To(Equal(vmServicePort2.Name))
					Expect(port.Protocol).To(BeEquivalentTo(vmServicePort2.Protocol))
					Expect(port.Port).To(Equal(vmServicePort2.Port))
					Expect(port.TargetPort.IntValue()).To(Equal(int(vmServicePort2.TargetPort)))
				})
			})

			Context("Inherits Annotations and Labels", func() {
				BeforeEach(func() {
					vmService.Annotations[annotationName1] = annotationValue1
					vmService.Labels[labelName1] = labelValue1
				})

				It("Expected Labels and Annotations", func() {
					Expect(service.Annotations).To(HaveKeyWithValue(annotationName1, annotationValue1))
					Expect(service.Labels).To(HaveKeyWithValue(labelName1, labelValue1))
				})

				// TODO: These don't belong here. Sort out when the Provider interface is improved.
				Context("NCP specific Labels and Annotations", func() {
					BeforeEach(func() {
						vmService.Labels[LabelServiceProxyName] = providers.NSXTServiceProxy
					})

					It("Expected labels", func() {
						Expect(service.Labels[LabelServiceProxyName]).To(Equal(providers.NSXTServiceProxy))
					})
				})
			})

			// TODO: NCP Specific. Sort out when the Provider interface is improved.
			Context("ExternalTrafficPolicy Annotations", func() {
				BeforeEach(func() {
					vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
					vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "99"
				})

				It("Expected values", func() {
					Expect(service.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeLocal))
					Expect(service.Annotations).To(HaveKeyWithValue(utils.AnnotationServiceHealthCheckNodePortKey, "99"))
				})
			})
		})

		Context("Service Exists", func() {
			var service *corev1.Service

			BeforeEach(func() {
				service = &corev1.Service{}

				vmService.Annotations[annotationName1] = annotationValue1
				vmService.Labels[labelName1] = labelValue1
			})

			JustBeforeEach(func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())

				Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpCreate)))
				Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
			})

			It("No Update if VirtualMachineService didn't change", func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))

				service2 := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, objKey, service2)).To(Succeed())
				Expect(apiEquality.Semantic.DeepEqual(service, service2)).To(BeTrue())
			})

			Context("VirtualMachineService Spec is updated", func() {
				It("Service Spec is updated accordingly", func() {
					vmService.Spec.ExternalName = "new-external-name"
					vmService.Spec.LoadBalancerIP = "new-lb-ip"
					vmService.Spec.LoadBalancerSourceRanges = []string{"range42"}

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())

					Expect(service.Spec.ExternalName).To(Equal("new-external-name"))
					Expect(service.Spec.LoadBalancerIP).To(Equal("new-lb-ip"))
					Expect(service.Spec.LoadBalancerSourceRanges).To(HaveLen(1))
					Expect(service.Spec.LoadBalancerSourceRanges).To(ContainElements("range42"))
				})

				It("Spec Annotations and Labels are updated", func() {
					vmService.Annotations[annotationName1] = "new-bar1"
					vmService.Annotations[annotationName2] = "new-bar2"
					vmService.Labels[labelName1] = "new-bar3"
					vmService.Labels[labelName2] = "new-bar4"

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())

					Expect(service.Annotations).To(HaveLen(2))
					Expect(service.Annotations).To(HaveKeyWithValue(annotationName1, "new-bar1"))
					Expect(service.Annotations).To(HaveKeyWithValue(annotationName2, "new-bar2"))
					Expect(service.Labels).To(HaveLen(2))
					Expect(service.Labels).To(HaveKeyWithValue(labelName1, "new-bar3"))
					Expect(service.Labels).To(HaveKeyWithValue(labelName2, "new-bar4"))
				})

				It("VirtualMachineService Annotations and Labels takes precedence", func() {
					Expect(service.Annotations).To(HaveLen(1))
					service.Annotations[annotationName2] = "should-be-changed"
					Expect(service.Labels).To(HaveLen(1))
					service.Labels[labelName2] = "should-be-changed"
					Expect(ctx.Client.Update(ctx, service)).To(Succeed())

					newValue := "vmsvc-should-win"
					vmService.Annotations[annotationName2] = newValue
					vmService.Labels[labelName2] = newValue

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).ToNot(HaveOccurred())

					Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())

					Expect(service.Annotations).To(HaveKeyWithValue(annotationName2, newValue))
					Expect(service.Labels).To(HaveKeyWithValue(labelName2, newValue))
				})
			})

			Context("Preserves Service Annotations and Labels set elsewhere", func() {
				// NetOp can set additional annotations and labels that we shouldn't delete.

				It("Keeps Labels and Annotations", func() {
					Expect(service.Annotations).To(HaveLen(1))
					service.Annotations["netop-annotation"] = "bar42"
					Expect(service.Labels).To(HaveLen(1))
					service.Labels["netop-label"] = "bar43"
					Expect(ctx.Client.Update(ctx, service)).To(Succeed())

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))

					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
					Expect(service.Annotations).To(HaveLen(2))
					Expect(service.Annotations).To(HaveKeyWithValue("netop-annotation", "bar42"))
					Expect(service.Labels).To(HaveLen(2))
					Expect(service.Labels).To(HaveKeyWithValue("netop-label", "bar43"))
				})
			})

			Context("Preserves existing NodePort", func() {
				BeforeEach(func() {
					vmService.Spec.Ports = []vmopv1alpha1.VirtualMachineServicePort{
						vmServicePort1,
					}
				})

				It("Keeps NodePort", func() {
					Expect(service.Spec.Ports).To(HaveLen(1))
					service.Spec.Ports[0].NodePort = 10000
					Expect(ctx.Client.Update(ctx, service)).To(Succeed())

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))

					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
					ports := service.Spec.Ports
					Expect(ports).To(HaveLen(1))

					port := ports[0]
					Expect(port.Name).To(Equal(vmServicePort1.Name))
					Expect(port.Protocol).To(BeEquivalentTo(vmServicePort1.Protocol))
					Expect(port.Port).To(Equal(vmServicePort1.Port))
					Expect(port.TargetPort.IntValue()).To(Equal(int(vmServicePort1.TargetPort)))
					Expect(port.NodePort).To(BeNumerically("==", 10000))
				})
			})

			Context("VirtualMachineService Status Ingress", func() {
				It("Sets empty Ingress", func() {
					Expect(vmService.Status.LoadBalancer.Ingress).To(BeEmpty())
				})

				It("Sets Ingress from Service Status", func() {
					service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
						{
							IP: "ip1",
						},
						{
							Hostname: "hostname1",
						},
					}
					Expect(ctx.Client.Update(ctx, service)).To(Succeed())

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).ToNot(HaveOccurred())

					ingress := vmService.Status.LoadBalancer.Ingress
					Expect(ingress).To(HaveLen(2))
					Expect(ingress[0].IP).To(Equal("ip1"))
					Expect(ingress[0].Hostname).To(BeEmpty())
					Expect(ingress[1].IP).To(BeEmpty())
					Expect(ingress[1].Hostname).To(Equal("hostname1"))
				})
			})
		})

		Context("Creates expected Endpoints", func() {
			var endpoints *corev1.Endpoints
			var labelSelector map[string]string
			var vm1, vm2, vm3 *vmopv1alpha1.VirtualMachine

			BeforeEach(func() {
				endpoints = &corev1.Endpoints{}
				labelSelector = map[string]string{"my-app": "dummy-label"}

				vmService.Annotations[annotationName1] = "bar1"
				vmService.Labels[labelName1] = "bar2"
				vmService.Spec.Selector = labelSelector
				vmService.Spec.Ports = []vmopv1alpha1.VirtualMachineServicePort{
					vmServicePort1,
				}

				vm1 = &vmopv1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm1",
						Namespace: vmService.Namespace,
						Labels:    labelSelector,
					},
					Status: vmopv1alpha1.VirtualMachineStatus{
						VmIp: "1.1.1.1",
					},
				}

				vm2 = &vmopv1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm2",
						Namespace: vmService.Namespace,
						Labels:    labelSelector,
					},
					Status: vmopv1alpha1.VirtualMachineStatus{
						VmIp: "2.2.2.2",
					},
				}

				vm3 = &vmopv1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm3",
						Namespace: vmService.Namespace,
						Labels:    map[string]string{},
					},
					Status: vmopv1alpha1.VirtualMachineStatus{
						VmIp: "3.3.3.3",
					},
				}
			})

			JustBeforeEach(func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())

				Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpCreate)))
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
			})

			It("With Expected OwnerReference", func() {
				ownerRefs := endpoints.GetOwnerReferences()
				Expect(ownerRefs).To(HaveLen(1))
				ownerRef := ownerRefs[0]
				Expect(ownerRef.Name).To(Equal(vmService.Name))
				Expect(ownerRef.Controller).To(Equal(pointer.BoolPtr(true)))
			})

			It("With Expected Annotations and Labels", func() {
				Expect(endpoints.Annotations).To(HaveKeyWithValue(annotationName1, "bar1"))
				Expect(endpoints.Labels).To(HaveKeyWithValue(labelName1, "bar2"))
			})

			It("Empty Subsets when no VM matches", func() {
				Expect(endpoints.Subsets).To(BeEmpty())
			})

			Context("When one VM matches label selector", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vm1, vm3)
				})

				It("With Expected Subset", func() {
					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))

					subset := subsets[0]

					Expect(subset.Ports).To(HaveLen(1))
					ssPort := subset.Ports[0]
					Expect(ssPort.Name).To(Equal(vmServicePort1.Name))
					Expect(ssPort.Protocol).To(BeEquivalentTo(vmServicePort1.Protocol))
					Expect(ssPort.Port).To(Equal(vmServicePort1.TargetPort))

					Expect(subset.Addresses).To(HaveLen(1))
					ssAddr := subset.Addresses[0]
					Expect(ssAddr.IP).To(Equal(vm1.Status.VmIp))
					Expect(ssAddr.TargetRef).ToNot(BeNil())
					Expect(ssAddr.TargetRef.Name).To(Equal(vm1.Name))
					Expect(ssAddr.TargetRef.Namespace).To(Equal(vm1.Namespace))

					// NOTE: We do not assign this yet.
					Expect(subset.NotReadyAddresses).To(BeEmpty())
				})

				Context("When VM does not have IP", func() {
					BeforeEach(func() {
						vm1.Status.VmIp = ""
					})

					It("Is not included in Subsets", func() {
						Expect(endpoints.Subsets).To(BeEmpty())
					})
				})
			})

			Context("When multiple VMs match label selector", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vm1, vm2, vm3)
				})

				It("With Expected Subsets", func() {
					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))

					subset := subsets[0]

					Expect(subset.Ports).To(HaveLen(1))
					ssPort := subset.Ports[0]
					Expect(ssPort.Name).To(Equal(vmServicePort1.Name))
					Expect(ssPort.Protocol).To(BeEquivalentTo(vmServicePort1.Protocol))
					Expect(ssPort.Port).To(Equal(vmServicePort1.TargetPort))

					// NOTE: Hardcode our subset sorting order of VM1, VM2.
					Expect(subset.Addresses).To(HaveLen(2))

					ssAddr := subset.Addresses[0]
					Expect(ssAddr.IP).To(Equal(vm1.Status.VmIp))
					Expect(ssAddr.TargetRef).ToNot(BeNil())
					Expect(ssAddr.TargetRef.Name).To(Equal(vm1.Name))

					ssAddr = subset.Addresses[1]
					Expect(ssAddr.IP).To(Equal(vm2.Status.VmIp))
					Expect(ssAddr.TargetRef).ToNot(BeNil())
					Expect(ssAddr.TargetRef.Name).To(Equal(vm2.Name))

					// NOTE: We do not assign this yet.
					Expect(subset.NotReadyAddresses).To(BeEmpty())
				})

				When("Service has multiple ports", func() {
					BeforeEach(func() {
						vmService.Spec.Ports = append(vmService.Spec.Ports, vmServicePort2)
						Expect(vmService.Spec.Ports).To(HaveLen(2))
					})

					It("Expected subsets should be packed", func() {
						subsets := endpoints.Subsets
						Expect(subsets).To(HaveLen(1))

						subset := subsets[0]

						// NOTE: Hardcode our subset sorting order of Port1, Port2.
						Expect(subset.Ports).To(HaveLen(2))
						ssPort := subset.Ports[0]
						Expect(ssPort.Name).To(Equal(vmServicePort1.Name))
						Expect(ssPort.Protocol).To(BeEquivalentTo(vmServicePort1.Protocol))
						Expect(ssPort.Port).To(Equal(vmServicePort1.TargetPort))

						ssPort = subset.Ports[1]
						Expect(ssPort.Name).To(Equal(vmServicePort2.Name))
						Expect(ssPort.Protocol).To(BeEquivalentTo(vmServicePort2.Protocol))
						Expect(ssPort.Port).To(Equal(vmServicePort2.TargetPort))

						// NOTE: Hardcode our subset sorting order of VM1, VM2.
						Expect(subset.Addresses).To(HaveLen(2))

						ssAddr := subset.Addresses[0]
						Expect(ssAddr.IP).To(Equal(vm1.Status.VmIp))
						Expect(ssAddr.TargetRef).ToNot(BeNil())
						Expect(ssAddr.TargetRef.Name).To(Equal(vm1.Name))

						ssAddr = subset.Addresses[1]
						Expect(ssAddr.IP).To(Equal(vm2.Status.VmIp))
						Expect(ssAddr.TargetRef).ToNot(BeNil())
						Expect(ssAddr.TargetRef.Name).To(Equal(vm2.Name))

						// NOTE: We do not assign this yet.
						Expect(subset.NotReadyAddresses).To(BeEmpty())
					})
				})
			})

			Context("When VM has Readiness Probe", func() {
				BeforeEach(func() {
					vm1.Spec.ReadinessProbe = &vmopv1alpha1.Probe{}
					vm2.Spec.ReadinessProbe = &vmopv1alpha1.Probe{}
					vm3.Spec.ReadinessProbe = &vmopv1alpha1.Probe{}

					initObjects = append(initObjects, vm1, vm2, vm3)
				})

				It("VMs without Ready Condition are not included in Subsets", func() {
					Expect(endpoints.Subsets).To(BeEmpty())
				})

				Context("Unready VMs are not included in Subset", func() {
					BeforeEach(func() {
						conditions.MarkFalse(vm1, vmopv1alpha1.ReadyCondition, "reason", vmopv1alpha1.ConditionSeverityError, "")
					})

					It("Subsets is Empty", func() {
						Expect(endpoints.Subsets).To(BeEmpty())
					})
				})

				Context("Ready VM is included in Subset", func() {
					BeforeEach(func() {
						conditions.MarkTrue(vm1, vmopv1alpha1.ReadyCondition)
					})

					It("Ready VM is included in Subsets", func() {
						subsets := endpoints.Subsets
						Expect(subsets).To(HaveLen(1))

						subset := subsets[0]

						Expect(subset.Ports).To(HaveLen(1))
						ssPort := subset.Ports[0]
						Expect(ssPort.Name).To(Equal(vmServicePort1.Name))
						Expect(ssPort.Protocol).To(BeEquivalentTo(vmServicePort1.Protocol))
						Expect(ssPort.Port).To(Equal(vmServicePort1.TargetPort))

						Expect(subset.Addresses).To(HaveLen(1))
						ssAddr := subset.Addresses[0]
						Expect(ssAddr.IP).To(Equal(vm1.Status.VmIp))
						Expect(ssAddr.TargetRef).ToNot(BeNil())
						Expect(ssAddr.TargetRef.Name).To(Equal(vm1.Name))
						Expect(ssAddr.TargetRef.Namespace).To(Equal(vm1.Namespace))

						// NOTE: We do not assign this yet.
						Expect(subset.NotReadyAddresses).To(BeEmpty())
					})
				})
			})

			Context("Preserve VMs in Endpoints that have Probe but hasn't run yet", func() {
				BeforeEach(func() {
					vm1.UID = "abc"
					vm1.Spec.ReadinessProbe = &vmopv1alpha1.Probe{}
					vm2.UID = "xyz"
					vm2.Spec.ReadinessProbe = &vmopv1alpha1.Probe{}
					// Initial setup so that the first Reconcile will add the VM.
					conditions.MarkTrue(vm1, vmopv1alpha1.ReadyCondition)
					initObjects = append(initObjects, vm1, vm2)
				})

				It("VM is kept in Endpoints", func() {
					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))
					subset := subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))
					ssAddr := subset.Addresses[0]
					Expect(ssAddr.IP).To(Equal(vm1.Status.VmIp))

					// Remove Ready condition but keep the ReadinessProbe. This simulates the probe not
					// being run yet.
					vm1.Status.Conditions = nil
					Expect(ctx.Client.Status().Update(ctx, vm1)).To(Succeed())

					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

					// VM1 should still be present in the Endpoints.
					subsets = endpoints.Subsets
					Expect(subsets).To(HaveLen(1))
					subset = subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))
					ssAddr = subset.Addresses[0]
					Expect(ssAddr.IP).To(Equal(vm1.Status.VmIp))
				})
			})
		})
	})

	Context("ReconcileDelete", func() {
		BeforeEach(func() {
			vmService.Finalizers = []string{finalizerName}
		})

		It("will clear finalizer", func() {
			err := reconciler.ReconcileDelete(vmServiceCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmServiceCtx.VMService.GetFinalizers()).ToNot(ContainElement(finalizerName))
		})

		Context("When Endpoint and Service exists", func() {

			BeforeEach(func() {
				objectMeta := metav1.ObjectMeta{
					Name:      vmService.Name,
					Namespace: vmService.Namespace,
				}
				endpoint := &corev1.Endpoints{ObjectMeta: objectMeta}
				service := &corev1.Service{ObjectMeta: objectMeta}
				initObjects = append(initObjects, endpoint, service)
			})

			It("Deletes Endpoint and Service", func() {
				err := reconciler.ReconcileDelete(vmServiceCtx)
				Expect(err).ToNot(HaveOccurred())

				endpoint := &corev1.Endpoints{}
				err = ctx.Client.Get(ctx, objKey, endpoint)
				Expect(errors.IsNotFound(err)).To(BeTrue())

				service := &corev1.Service{}
				err = ctx.Client.Get(ctx, objKey, service)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})
	})
}

// This duplicates some of the above tests just to test bits of the NSX-T LB provider.
// We should really instead refactor the provider interface so we don't have logic for
// it in multiples places and can test it just in the existing provider tests. These
// tests as-is need some improvement.
func nsxtLBProviderTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		lbProvider   providers.LoadbalancerProvider
		reconciler   *virtualmachineservice.ReconcileVirtualMachineService
		vmServiceCtx *vmopContext.VirtualMachineServiceContext

		vmServicePort1 vmopv1alpha1.VirtualMachineServicePort
		vmService      *vmopv1alpha1.VirtualMachineService
		objKey         client.ObjectKey

		lbSourceRanges = []string{"1.1.1.0/24", "2.2.0.0/16"}
	)

	const (
		externalName   = "my-external-name"
		clusterIP      = "192.168.100.42"
		loadBalancerIP = "1.1.1.42"
	)

	BeforeEach(func() {
		vmServicePort1 = vmopv1alpha1.VirtualMachineServicePort{
			Name:       "port1",
			Protocol:   "TCP",
			Port:       42,
			TargetPort: 142,
		}

		vmService = &vmopv1alpha1.VirtualMachineService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dummy-vm-service",
				Namespace:   "dummy-ns",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: vmopv1alpha1.VirtualMachineServiceSpec{
				Type:                     vmopv1alpha1.VirtualMachineServiceTypeLoadBalancer,
				Selector:                 map[string]string{},
				ExternalName:             externalName,
				ClusterIP:                clusterIP,
				LoadBalancerIP:           loadBalancerIP,
				LoadBalancerSourceRanges: lbSourceRanges,
				Ports:                    []vmopv1alpha1.VirtualMachineServicePort{vmServicePort1},
			},
		}

		objKey = client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		lbProvider = providers.NsxtLoadBalancerProvider()
		reconciler = virtualmachineservice.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Scheme,
			ctx.Recorder,
			lbProvider,
		)

		vmServiceCtx = &vmopContext.VirtualMachineServiceContext{
			Context:   ctx,
			Logger:    ctx.Logger.WithName(vmService.Name),
			VMService: vmService,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmServiceCtx = nil
		reconciler = nil
	})

	Describe("ReconcileNormal", func() {
		var service *corev1.Service

		BeforeEach(func() {
			service = &corev1.Service{}
		})

		Describe("Create Or Update k8s Service", func() {
			JustBeforeEach(func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())

				Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpCreate)))
				Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
			})

			It("Should update the k8s Service to match with the VirtualMachineService", func() {
				// Modify the VirtualMachineService, corresponding Service should also be modified.
				newExternalName := "someExternalName"
				newLoadBalancerIP := "1.1.1.1"

				vmService.Spec.ExternalName = newExternalName
				vmService.Spec.LoadBalancerIP = newLoadBalancerIP
				vmService.Spec.LoadBalancerSourceRanges = []string{"1.1.1.0/24", "2.2.2.2/28"}
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
				vmService.Labels[LabelServiceProxyName] = providers.NSXTServiceProxy

				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				newService := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, objKey, newService)).To(Succeed())

				Expect(newService.Spec.ExternalName).To(Equal(newExternalName))
				Expect(newService.Spec.LoadBalancerIP).To(Equal(newLoadBalancerIP))
				Expect(newService.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeLocal))
				Expect(newService.Labels[LabelServiceProxyName]).To(Equal(providers.NSXTServiceProxy))
				Expect(newService.Spec.LoadBalancerSourceRanges).To(Equal([]string{"1.1.1.0/24", "2.2.2.2/28"}))
			})

			It("Should update the k8s Service to match with the VirtualMachineService when LoadBalancerSourceRanges is cleared", func() {
				vmService.Spec.LoadBalancerSourceRanges = []string{}

				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				newService := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, objKey, newService)).To(Succeed())
				Expect(newService.Spec.LoadBalancerSourceRanges).To(BeEmpty())
			})

			It("Should update the k8s Service to match with the VirtualMachineService when externalTrafficPolicy is cleared", func() {
				if service.Annotations == nil {
					service.Annotations = make(map[string]string)
				}
				if service.Labels == nil {
					service.Labels = make(map[string]string)
				}

				By("applying externalTrafficPolicy and healthCheckNodePort annotations")
				service.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				service.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
				By("applying service-proxy label")
				service.Labels[LabelServiceProxyName] = providers.NSXTServiceProxy
				service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
				Expect(ctx.Client.Update(ctx, service)).To(Succeed())

				delete(vmService.Annotations, utils.AnnotationServiceExternalTrafficPolicyKey)
				delete(vmService.Annotations, utils.AnnotationServiceHealthCheckNodePortKey)

				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				newService := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, objKey, newService)).To(Succeed())
				Expect(newService.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeCluster))
				Expect(newService.Annotations).ToNot(HaveKey(utils.AnnotationServiceExternalTrafficPolicyKey))
				Expect(newService.Annotations).ToNot(HaveKey(utils.AnnotationServiceHealthCheckNodePortKey))
				Expect(newService.Labels).ToNot(HaveKey(LabelServiceProxyName))
			})

			It("Should update the k8s Service to remove the provider specific annotations regarding healthCheckNodePort", func() {
				if service.Annotations == nil {
					service.Annotations = make(map[string]string)
				}

				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
				annotations, err := lbProvider.GetServiceAnnotations(ctx, vmService)
				Expect(err).ToNot(HaveOccurred())
				for k, v := range annotations {
					service.Annotations[k] = v
				}
				Expect(ctx.Client.Update(ctx, service)).To(Succeed())

				err = reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				newService := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, objKey, newService)).To(Succeed())
				By("ensuring the provider specific annotations are removed from the new service")
				annotationsToBeRemoved, err := lbProvider.GetToBeRemovedServiceAnnotations(ctx, vmService)
				Expect(err).ToNot(HaveOccurred())
				for k := range annotationsToBeRemoved {
					_, exist := newService.Annotations[k]
					Expect(exist).To(BeFalse())
				}
			})
		})
	})
}

func expectEvent(ctx *builder.UnitTestContextForController, matcher types.GomegaMatcher) {
	var event string
	EventuallyWithOffset(1, ctx.Events).Should(Receive(&event))
	ExpectWithOffset(1, event).To(matcher)
}
