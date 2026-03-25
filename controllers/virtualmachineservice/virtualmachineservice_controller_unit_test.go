// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.NSXT,
			testlabels.API,
		),
		nsxtLBProviderTestsReconcile,
	)
}

const LabelServiceProxyName = "service.kubernetes.io/service-proxy-name"

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler   *virtualmachineservice.ReconcileVirtualMachineService
		vmServiceCtx *pkgctx.VirtualMachineServiceContext

		vmService      *vmopv1.VirtualMachineService
		vmServicePort1 vmopv1.VirtualMachineServicePort
		vmServicePort2 vmopv1.VirtualMachineServicePort
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
		vmService = &vmopv1.VirtualMachineService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dummy-vm-service",
				Namespace:   "dummy-ns",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineServiceSpec{
				Type:                     vmopv1.VirtualMachineServiceTypeLoadBalancer,
				Selector:                 map[string]string{},
				ExternalName:             externalName,
				ClusterIP:                clusterIP,
				LoadBalancerIP:           loadBalancerIP,
				LoadBalancerSourceRanges: lbSourceRanges,
			},
		}

		vmServicePort1 = vmopv1.VirtualMachineServicePort{
			Name:       "port1",
			Protocol:   "TCP",
			Port:       42,
			TargetPort: 142,
		}

		vmServicePort2 = vmopv1.VirtualMachineServicePort{
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
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			providers.NoopLoadbalancerProvider{},
		)

		vmServiceCtx = &pkgctx.VirtualMachineServiceContext{
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
				Expect(ownerRef.Controller).To(Equal(ptr.To(true)))
			})

			It("With Expected Spec", func() {
				Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				Expect(service.Spec.ExternalName).To(Equal(externalName))
				Expect(service.Spec.ClusterIP).To(Equal(clusterIP))
				Expect(service.Spec.LoadBalancerIP).To(Equal(loadBalancerIP))
				Expect(service.Spec.LoadBalancerSourceRanges).To(HaveLen(2))
				Expect(service.Spec.LoadBalancerSourceRanges).To(ContainElements(lbSourceRanges))
				Expect(service.Spec.AllocateLoadBalancerNodePorts).ToNot(BeNil())
				Expect(*service.Spec.AllocateLoadBalancerNodePorts).To(BeFalse())
			})

			Context("IPFamilies and IPFamilyPolicy", func() {
				It("Copies IPFamilies to Service spec", func() {
					vmService.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}
					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
					Expect(service.Spec.IPFamilies).To(HaveLen(2))
					Expect(service.Spec.IPFamilies).To(ContainElements(corev1.IPv4Protocol, corev1.IPv6Protocol))
				})

				It("Copies IPFamilyPolicy to Service spec", func() {
					policy := corev1.IPFamilyPolicyPreferDualStack
					vmService.Spec.IPFamilyPolicy = &policy
					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
					Expect(service.Spec.IPFamilyPolicy).ToNot(BeNil())
					Expect(*service.Spec.IPFamilyPolicy).To(Equal(corev1.IPFamilyPolicyPreferDualStack))
				})

				It("Clears IPFamilies and IPFamilyPolicy for ExternalName services", func() {
					vmService.Spec.Type = vmopv1.VirtualMachineServiceTypeExternalName
					vmService.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
					policy := corev1.IPFamilyPolicySingleStack
					vmService.Spec.IPFamilyPolicy = &policy
					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
					Expect(service.Spec.IPFamilies).To(BeEmpty())
					Expect(service.Spec.IPFamilyPolicy).To(BeNil())
				})
			})

			Context("With Expected Spec.Ports", func() {
				BeforeEach(func() {
					vmService.Spec.Ports = []vmopv1.VirtualMachineServicePort{
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
					vmService.Spec.Ports = []vmopv1.VirtualMachineServicePort{
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
					Expect(ctx.Client.Status().Update(ctx, service)).To(Succeed())

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
			var labelSelector, vmLabels map[string]string
			var vm1, vm2, vm3 *vmopv1.VirtualMachine

			BeforeEach(func() {
				endpoints = &corev1.Endpoints{}
				labelSelector = map[string]string{"my-app": "dummy-label"}
				vmLabels = map[string]string{"my-app": "dummy-label", "other": "label"}

				vmService.Annotations[annotationName1] = "bar1"
				vmService.Labels[labelName1] = "bar2"
				vmService.Spec.Selector = labelSelector
				vmService.Spec.Ports = []vmopv1.VirtualMachineServicePort{
					vmServicePort1,
				}

				vm1 = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm1",
						Namespace: vmService.Namespace,
						Labels:    vmLabels,
					},
					Status: vmopv1.VirtualMachineStatus{
						Network: &vmopv1.VirtualMachineNetworkStatus{
							PrimaryIP4: "1.1.1.1",
						},
					},
				}

				vm2 = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm2",
						Namespace: vmService.Namespace,
						Labels:    vmLabels,
					},
					Status: vmopv1.VirtualMachineStatus{
						Network: &vmopv1.VirtualMachineNetworkStatus{
							PrimaryIP4: "2.2.2.2",
						},
					},
				}

				vm3 = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm3",
						Namespace: vmService.Namespace,
						Labels:    map[string]string{},
					},
					Status: vmopv1.VirtualMachineStatus{
						Network: &vmopv1.VirtualMachineNetworkStatus{
							PrimaryIP4: "3.3.3.3",
						},
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
				Expect(ownerRef.Controller).To(Equal(ptr.To(true)))
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

				It("With Expected Subsets", func() {
					Expect(endpoints.Subsets).To(HaveLen(1))
					subset := endpoints.Subsets[0]

					Expect(subset.Ports).To(HaveLen(1))
					assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)

					Expect(subset.Addresses).To(HaveLen(1))
					assertEPAddrFromVM(subset.Addresses[0], vm1)
					Expect(subset.NotReadyAddresses).To(BeEmpty())
				})

				Context("When VM does not have IP", func() {
					BeforeEach(func() {
						vm1.Status.Network.PrimaryIP4 = ""
					})

					It("Not included in Subsets", func() {
						Expect(endpoints.Subsets).To(BeEmpty())
					})
				})
			})

			Context("When multiple VMs match label selector", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vm1, vm2, vm3)
				})

				It("With Expected Subsets", func() {
					Expect(endpoints.Subsets).To(HaveLen(1))
					subset := endpoints.Subsets[0]

					Expect(subset.Ports).To(HaveLen(1))
					assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)

					Expect(subset.Addresses).To(HaveLen(2))
					assertEPAddrFromVM(subset.Addresses[0], vm1)
					assertEPAddrFromVM(subset.Addresses[1], vm2)
					Expect(subset.NotReadyAddresses).To(BeEmpty())
				})

				When("Service has multiple ports", func() {
					BeforeEach(func() {
						vmService.Spec.Ports = append(vmService.Spec.Ports, vmServicePort2)
						Expect(vmService.Spec.Ports).To(HaveLen(2))
					})

					It("Expected subsets should be packed", func() {
						Expect(endpoints.Subsets).To(HaveLen(1))
						subset := endpoints.Subsets[0]

						Expect(subset.Ports).To(HaveLen(2))
						assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)
						assertEPPortFromVMServicePort(subset.Ports[1], vmServicePort2)

						Expect(subset.Addresses).To(HaveLen(2))
						assertEPAddrFromVM(subset.Addresses[0], vm1)
						assertEPAddrFromVM(subset.Addresses[1], vm2)
						Expect(subset.NotReadyAddresses).To(BeEmpty())
					})
				})
			})

			Context("When VMs have Readiness Probe", func() {
				BeforeEach(func() {
					vm1.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						TCPSocket: &vmopv1.TCPSocketAction{},
					}
					vm2.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						TCPSocket: &vmopv1.TCPSocketAction{},
					}
					vm3.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						TCPSocket: &vmopv1.TCPSocketAction{},
					}

					initObjects = append(initObjects, vm1, vm2, vm3)
				})

				It("VMs without Ready Condition are included in NotReadyAddresses", func() {
					Expect(endpoints.Subsets).To(HaveLen(1))
					subset := endpoints.Subsets[0]

					Expect(subset.Ports).To(HaveLen(1))
					assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)

					Expect(subset.Addresses).To(BeEmpty())
					Expect(subset.NotReadyAddresses).To(HaveLen(2))
					assertEPAddrFromVM(subset.NotReadyAddresses[0], vm1)
					assertEPAddrFromVM(subset.NotReadyAddresses[1], vm2)
				})

				Context("Unready VM with false Ready condition", func() {
					BeforeEach(func() {
						conditions.MarkFalse(vm1, vmopv1.ReadyConditionType, "reason", "")
					})

					It("With expected Subsets", func() {
						Expect(endpoints.Subsets).To(HaveLen(1))
						subset := endpoints.Subsets[0]

						Expect(subset.Ports).To(HaveLen(1))
						assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)

						Expect(subset.Addresses).To(BeEmpty())
						Expect(subset.NotReadyAddresses).To(HaveLen(2))
						assertEPAddrFromVM(subset.NotReadyAddresses[0], vm1)
						assertEPAddrFromVM(subset.NotReadyAddresses[1], vm2)
					})
				})

				Context("Ready VM with true Ready condition", func() {
					BeforeEach(func() {
						conditions.MarkTrue(vm1, vmopv1.ReadyConditionType)
					})

					It("With expected Subsets", func() {
						Expect(endpoints.Subsets).To(HaveLen(1))
						subset := endpoints.Subsets[0]

						Expect(subset.Ports).To(HaveLen(1))
						assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)

						Expect(subset.Addresses).To(HaveLen(1))
						assertEPAddrFromVM(subset.Addresses[0], vm1)
						Expect(subset.NotReadyAddresses).To(HaveLen(1))
						assertEPAddrFromVM(subset.NotReadyAddresses[0], vm2)
					})
				})
			})

			Context("Endpoint filtering based on Service IPFamilies", func() {
				var dualStackVM *vmopv1.VirtualMachine
				var ipv4VM *vmopv1.VirtualMachine
				var ipv6VM *vmopv1.VirtualMachine

				BeforeEach(func() {
					dualStackVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dual-stack-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.1",
								PrimaryIP6: "2001:db8::1",
							},
						},
					}
					ipv4VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv4-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.2",
							},
						},
					}
					ipv6VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv6-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP6: "2001:db8::2",
							},
						},
					}
					initObjects = append(initObjects, dualStackVM, ipv4VM, ipv6VM)
				})

				It("IPv4-only Service only includes IPv4 endpoints", func() {
				vmService.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectAllEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElements("192.168.1.1", "192.168.1.2"))
				Expect(allAddresses).NotTo(ContainElements("2001:db8::1", "2001:db8::2"))
			})

			It("IPv6-only Service only includes IPv6 endpoints", func() {
				vmService.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv6Protocol}
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectAllEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElements("2001:db8::1", "2001:db8::2"))
				Expect(allAddresses).NotTo(ContainElements("192.168.1.1", "192.168.1.2"))
			})

			It("Dual-stack Service includes both IPv4 and IPv6 endpoints", func() {
				vmService.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectAllEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElements("192.168.1.1", "192.168.1.2", "2001:db8::1", "2001:db8::2"))
			})
			})

			Context("Endpoint filtering based on Service IPFamilyPolicy", func() {
				var dualStackVM *vmopv1.VirtualMachine
				var ipv4VM *vmopv1.VirtualMachine
				var ipv6VM *vmopv1.VirtualMachine
				var service *corev1.Service

				BeforeEach(func() {
					service = &corev1.Service{}
					dualStackVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dual-stack-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.1",
								PrimaryIP6: "2001:db8::1",
							},
						},
					}
					ipv4VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv4-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.2",
							},
						},
					}
					ipv6VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv6-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP6: "2001:db8::2",
							},
						},
					}
					initObjects = append(initObjects, dualStackVM, ipv4VM, ipv6VM)
				})

				It("SingleStack policy with IPv4 clusterIP only includes IPv4 endpoints", func() {
					policy := corev1.IPFamilyPolicySingleStack
					vmService.Spec.IPFamilyPolicy = &policy
					// Create service first to get clusterIP
					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())
					// Set IPv4 clusterIP
					service.Spec.ClusterIPs = []string{"10.0.0.1"}
					Expect(ctx.Client.Update(ctx, service)).To(Succeed())

				err = reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectReadyEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElements("192.168.1.1", "192.168.1.2"))
				Expect(allAddresses).NotTo(ContainElements("2001:db8::1", "2001:db8::2"))
			})

			It("PreferDualStack policy includes both IPv4 and IPv6 endpoints", func() {
				policy := corev1.IPFamilyPolicyPreferDualStack
				vmService.Spec.IPFamilyPolicy = &policy
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectReadyEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElements("192.168.1.1", "192.168.1.2", "2001:db8::1", "2001:db8::2"))
			})

			It("RequireDualStack policy includes both IPv4 and IPv6 endpoints", func() {
				policy := corev1.IPFamilyPolicyRequireDualStack
				vmService.Spec.IPFamilyPolicy = &policy
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectReadyEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElements("192.168.1.1", "192.168.1.2", "2001:db8::1", "2001:db8::2"))
			})
			})

			Context("VM created after VirtualMachineService", func() {
				var newVM *vmopv1.VirtualMachine

				BeforeEach(func() {
					vmService.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
					// Don't add VM to initObjects initially - simulate VM being created later
				})

				JustBeforeEach(func() {
					// First reconciliation: Service created, but no VMs yet
					err := reconciler.ReconcileNormal(vmServiceCtx)
					Expect(err).NotTo(HaveOccurred())
					// Service may be created or updated depending on test execution order
					// Drain any events (Create or Update) to ensure event channel is ready
					select {
					case <-ctx.Events:
					default:
					}
					Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
					// Initially, endpoints should be empty
					Expect(endpoints.Subsets).To(BeEmpty())

					// Now create a VM that matches the selector (simulating VM created after service)
					newVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.100",
								PrimaryIP6: "2001:db8::100",
							},
						},
					}
					Expect(ctx.Client.Create(ctx, newVM)).To(Succeed())
				})

			It("VM is added to endpoints with correct IP family filtering", func() {
				// Reconcile again after VM is created (simulating controller watching VM changes)
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

				allAddresses := collectReadyEndpointIPs(endpoints)
				Expect(allAddresses).To(ContainElement("192.168.1.100"))
				Expect(allAddresses).NotTo(ContainElement("2001:db8::100"))
			})
			})

			Context("Preserve VMs in Endpoints that have Probe but hasn't run yet", func() {
				BeforeEach(func() {
					vm1.UID = "abc"
					vm1.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						TCPSocket: &vmopv1.TCPSocketAction{},
					}
					vm2.UID = "xyz"
					vm2.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						TCPSocket: &vmopv1.TCPSocketAction{},
					}
					// Initial setup so that the first Reconcile will add the VM.
					conditions.MarkTrue(vm1, vmopv1.ReadyConditionType)
					initObjects = append(initObjects, vm1, vm2)
				})

				It("VM is kept in Endpoints", func() {
					subsets := endpoints.Subsets
					Expect(subsets).To(HaveLen(1))
					subset := subsets[0]
					Expect(subset.Addresses).To(HaveLen(1))
					assertEPAddrFromVM(subset.Addresses[0], vm1)
					Expect(subset.NotReadyAddresses).To(HaveLen(1))
					assertEPAddrFromVM(subset.NotReadyAddresses[0], vm2)

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
					assertEPAddrFromVM(subset.Addresses[0], vm1)
					Expect(subset.NotReadyAddresses).To(HaveLen(1))
					assertEPAddrFromVM(subset.NotReadyAddresses[0], vm2)
				})
			})

			Context("IPv6-only VM", func() {
				var ipv6VM *vmopv1.VirtualMachine

				BeforeEach(func() {
					ipv6VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv6-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP6: "2001:db8::1",
							},
						},
					}
					initObjects = append(initObjects, ipv6VM)
				})

				It("Endpoint should contain IPv6 address", func() {
					Expect(endpoints.Subsets).To(HaveLen(1))
					subset := endpoints.Subsets[0]

					Expect(subset.Ports).To(HaveLen(1))
					assertEPPortFromVMServicePort(subset.Ports[0], vmServicePort1)

					Expect(subset.Addresses).To(HaveLen(1))
					assertEPAddrFromVMWithIP(subset.Addresses[0], ipv6VM, "2001:db8::1")
					Expect(subset.NotReadyAddresses).To(BeEmpty())
				})

				It("Endpoint should NOT contain IPv4 address", func() {
					Expect(endpoints.Subsets).To(HaveLen(1))
					subset := endpoints.Subsets[0]

					for _, addr := range subset.Addresses {
						Expect(addr.IP).ToNot(Equal(""))
						Expect(addr.IP).To(Equal("2001:db8::1"))
					}
				})
			})

			Context("Dual-stack VM", func() {
				var dualStackVM *vmopv1.VirtualMachine

				BeforeEach(func() {
					dualStackVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dual-stack-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.100",
								PrimaryIP6: "2001:db8::100",
							},
						},
					}
					initObjects = append(initObjects, dualStackVM)
				})

				It("Endpoints should have TWO subsets (one for IPv4, one for IPv6)", func() {
					Expect(endpoints.Subsets).To(HaveLen(2))

					var ipv4Subset, ipv6Subset *corev1.EndpointSubset
					for i := range endpoints.Subsets {
						subset := &endpoints.Subsets[i]
						if len(subset.Addresses) > 0 {
							if subset.Addresses[0].IP == "192.168.1.100" {
								ipv4Subset = subset
							} else if subset.Addresses[0].IP == "2001:db8::100" {
								ipv6Subset = subset
							}
						}
					}

					Expect(ipv4Subset).ToNot(BeNil(), "Should have IPv4 subset")
					Expect(ipv6Subset).ToNot(BeNil(), "Should have IPv6 subset")
				})

				It("IPv4 subset contains IPv4 address", func() {
					var ipv4Subset *corev1.EndpointSubset
					for i := range endpoints.Subsets {
						subset := &endpoints.Subsets[i]
						if len(subset.Addresses) > 0 && subset.Addresses[0].IP == "192.168.1.100" {
							ipv4Subset = subset
							break
						}
					}

					Expect(ipv4Subset).ToNot(BeNil())
					Expect(ipv4Subset.Addresses).To(HaveLen(1))
					assertEPAddrFromVMWithIP(ipv4Subset.Addresses[0], dualStackVM, "192.168.1.100")
					Expect(ipv4Subset.Ports).To(HaveLen(1))
					assertEPPortFromVMServicePort(ipv4Subset.Ports[0], vmServicePort1)
				})

				It("IPv6 subset contains IPv6 address", func() {
					var ipv6Subset *corev1.EndpointSubset
					for i := range endpoints.Subsets {
						subset := &endpoints.Subsets[i]
						if len(subset.Addresses) > 0 && subset.Addresses[0].IP == "2001:db8::100" {
							ipv6Subset = subset
							break
						}
					}

					Expect(ipv6Subset).ToNot(BeNil())
					Expect(ipv6Subset.Addresses).To(HaveLen(1))
					assertEPAddrFromVMWithIP(ipv6Subset.Addresses[0], dualStackVM, "2001:db8::100")
					Expect(ipv6Subset.Ports).To(HaveLen(1))
					assertEPPortFromVMServicePort(ipv6Subset.Ports[0], vmServicePort1)
				})
			})

			Context("Mixed VMs (IPv4-only, IPv6-only, and dual-stack)", func() {
				var ipv4VM, ipv6VM, dualStackVM *vmopv1.VirtualMachine

				BeforeEach(func() {
					ipv4VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv4-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.10",
							},
						},
					}

					ipv6VM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ipv6-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP6: "2001:db8::10",
							},
						},
					}

					dualStackVM = &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dual-stack-vm",
							Namespace: vmService.Namespace,
							Labels:    vmLabels,
						},
						Status: vmopv1.VirtualMachineStatus{
							Network: &vmopv1.VirtualMachineNetworkStatus{
								PrimaryIP4: "192.168.1.20",
								PrimaryIP6: "2001:db8::20",
							},
						},
					}

					initObjects = append(initObjects, ipv4VM, ipv6VM, dualStackVM)
				})

			It("All VMs appear in appropriate subsets based on their IP families", func() {
				Expect(endpoints.Subsets).ToNot(BeEmpty())

				allIPs := collectReadyEndpointIPs(endpoints)
				// Should have: IPv4 from ipv4VM, IPv6 from ipv6VM, IPv4 and IPv6 from dualStackVM
				Expect(allIPs).To(ContainElement("192.168.1.10"), "Should contain IPv4-only VM IP")
				Expect(allIPs).To(ContainElement("2001:db8::10"), "Should contain IPv6-only VM IP")
				Expect(allIPs).To(ContainElement("192.168.1.20"), "Should contain dual-stack VM IPv4")
				Expect(allIPs).To(ContainElement("2001:db8::20"), "Should contain dual-stack VM IPv6")
				Expect(allIPs).To(HaveLen(4), "Should have 4 total IP addresses")
			})

			It("IPv4 addresses are grouped together", func() {
				allIPs := collectReadyEndpointIPs(endpoints)
				Expect(allIPs).To(ContainElement("192.168.1.10"))
				Expect(allIPs).To(ContainElement("192.168.1.20"))
			})

			It("IPv6 addresses are grouped together", func() {
				allIPs := collectReadyEndpointIPs(endpoints)
				Expect(allIPs).To(ContainElement("2001:db8::10"))
				Expect(allIPs).To(ContainElement("2001:db8::20"))
			})
			})
		})

		Context("Selectorless VirtualMachineService", func() {
			var vm1 *vmopv1.VirtualMachine
			var labelSelector, vmLabels map[string]string

			BeforeEach(func() {
				labelSelector = map[string]string{"my-app": "dummy-label"}
				vmLabels = map[string]string{"my-app": "dummy-label", "other": "label"}

				vmService.Annotations[annotationName1] = "bar1"
				vmService.Labels[labelName1] = "bar2"
				vmService.Spec.Ports = []vmopv1.VirtualMachineServicePort{
					vmServicePort1,
				}

				vm1 = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vm1",
						Namespace: vmService.Namespace,
						Labels:    vmLabels,
					},
					Status: vmopv1.VirtualMachineStatus{
						Network: &vmopv1.VirtualMachineNetworkStatus{
							PrimaryIP4: "1.1.1.1",
						},
					},
				}

				initObjects = append(initObjects, vm1)
			})

			JustBeforeEach(func() {
				err := reconciler.ReconcileNormal(vmServiceCtx)
				Expect(err).NotTo(HaveOccurred())

				Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpCreate)))
			})

			It("Creates Service but not Endpoints", func() {
				service := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, objKey, service)).To(Succeed())

				endpoints := &corev1.Endpoints{}
				err := ctx.Client.Get(ctx, objKey, endpoints)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})

			Context("When VirtualMachineService becomes selectorless", func() {
				BeforeEach(func() {
					vmService.Spec.Selector = labelSelector
				})

				/* This is to match the k8s Service behavior when it changes to selectorless. */
				It("Does not delete existing Endpoints", func() {
					endpoints := &corev1.Endpoints{}
					Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())

					vmServiceCtx.VMService.Spec.Selector = nil
					Expect(reconciler.ReconcileNormal(vmServiceCtx)).To(Succeed())
					Expect(ctx.Client.Get(ctx, objKey, endpoints)).To(Succeed())
					Expect(endpoints.Subsets).ToNot(BeEmpty())
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
		vmServiceCtx *pkgctx.VirtualMachineServiceContext

		vmServicePort1 vmopv1.VirtualMachineServicePort
		vmService      *vmopv1.VirtualMachineService
		objKey         client.ObjectKey

		lbSourceRanges = []string{"1.1.1.0/24", "2.2.0.0/16"}
	)

	const (
		externalName   = "my-external-name"
		clusterIP      = "192.168.100.42"
		loadBalancerIP = "1.1.1.42"
	)

	BeforeEach(func() {
		vmServicePort1 = vmopv1.VirtualMachineServicePort{
			Name:       "port1",
			Protocol:   "TCP",
			Port:       42,
			TargetPort: 142,
		}

		vmService = &vmopv1.VirtualMachineService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dummy-vm-service",
				Namespace:   "dummy-ns",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineServiceSpec{
				Type:                     vmopv1.VirtualMachineServiceTypeLoadBalancer,
				Selector:                 map[string]string{},
				ExternalName:             externalName,
				ClusterIP:                clusterIP,
				LoadBalancerIP:           loadBalancerIP,
				LoadBalancerSourceRanges: lbSourceRanges,
				Ports:                    []vmopv1.VirtualMachineServicePort{vmServicePort1},
			},
		}

		objKey = client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		lbProvider = providers.NsxtLoadBalancerProvider()
		reconciler = virtualmachineservice.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			lbProvider,
		)

		vmServiceCtx = &pkgctx.VirtualMachineServiceContext{
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

func assertEPPortFromVMServicePort(
	port corev1.EndpointPort,
	vmServicePort vmopv1.VirtualMachineServicePort) {

	ExpectWithOffset(1, port.Name).To(Equal(vmServicePort.Name))
	ExpectWithOffset(1, port.Protocol).To(BeEquivalentTo(vmServicePort.Protocol))
	ExpectWithOffset(1, port.Port).To(Equal(vmServicePort.TargetPort))
}

func assertEPAddrFromVM(
	addr corev1.EndpointAddress,
	vm *vmopv1.VirtualMachine) {

	ExpectWithOffset(1, vm.Status.Network).ToNot(BeNil())
	ExpectWithOffset(1, addr.IP).To(Equal(vm.Status.Network.PrimaryIP4))
	ExpectWithOffset(1, addr.TargetRef).ToNot(BeNil())
	ExpectWithOffset(1, addr.TargetRef.Name).To(Equal(vm.Name))
	ExpectWithOffset(1, addr.TargetRef.Namespace).To(Equal(vm.Namespace))
}

// assertEPAddrFromVMWithIP validates that the endpoint address matches the expected IP
// and belongs to the specified VM. Supports both IPv4 and IPv6.
func assertEPAddrFromVMWithIP(
	addr corev1.EndpointAddress,
	vm *vmopv1.VirtualMachine,
	expectedIP string) {

	ExpectWithOffset(1, vm.Status.Network).ToNot(BeNil())
	ExpectWithOffset(1, addr.IP).To(Equal(expectedIP))
	ExpectWithOffset(1, addr.TargetRef).ToNot(BeNil())
	ExpectWithOffset(1, addr.TargetRef.Name).To(Equal(vm.Name))
	ExpectWithOffset(1, addr.TargetRef.Namespace).To(Equal(vm.Namespace))
}

// collectReadyEndpointIPs returns all IPs from ready (Addresses) subsets of an Endpoints object.
func collectReadyEndpointIPs(endpoints *corev1.Endpoints) []string {
	var ips []string
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			ips = append(ips, addr.IP)
		}
	}
	return ips
}

// collectAllEndpointIPs returns all IPs from both ready and not-ready subsets of an Endpoints object.
func collectAllEndpointIPs(endpoints *corev1.Endpoints) []string {
	var ips []string
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			ips = append(ips, addr.IP)
		}
		for _, addr := range subset.NotReadyAddresses {
			ips = append(ips, addr.IP)
		}
	}
	return ips
}
