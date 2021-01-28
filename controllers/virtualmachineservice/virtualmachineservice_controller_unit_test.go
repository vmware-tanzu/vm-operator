// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
	Describe("Invoking NSXT Reconcile", nsxtLBProviderTestsReconcile)
}

const LabelServiceProxyName = "service.kubernetes.io/service-proxy-name"

//nolint:dupl goconst
func unitTestsReconcile() {

	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController

		reconciler *virtualmachineservice.ReconcileVirtualMachineService
	)

	BeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachineservice.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Scheme,
			ctx.Recorder,
			providers.NoopLoadbalancerProvider{},
		)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	Describe("Reconcile VirtualMachineService", func() {

		Describe("Create Or Update k8s Service", func() {
			var (
				vmServiceName string
				vmService     *vmopv1alpha1.VirtualMachineService
				vmServiceCtx  *context.VirtualMachineServiceContext
				serviceKey    client.ObjectKey
			)

			BeforeEach(func() {
				vmServiceName = "dummy-service"
				vmService = getVmService(vmServiceName, ctx.Namespace)
				vmServiceCtx = &context.VirtualMachineServiceContext{
					Context:   ctx,
					Logger:    ctx.Logger,
					VMService: vmService,
				}

				service := getService(vmService.Name, vmService.Namespace)
				Expect(ctx.Client.Create(ctx, service)).To(Succeed())

				serviceKey = client.ObjectKey{Namespace: service.Namespace, Name: service.Name}
			})

			It("Should update the k8s Service to match with the VirtualMachineService", func() {
				service := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, serviceKey, service)).To(Succeed())

				// Modify the VirtualMachineService, corresponding Service should also be modified.
				externalName := "someExternalName"
				vmService.Spec.ExternalName = externalName
				loadBalancerIP := "1.1.1.1"
				vmService.Spec.LoadBalancerIP = loadBalancerIP
				vmService.Spec.LoadBalancerSourceRanges = []string{"1.1.1.0/24", "2.2.2.2/28"}
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
				vmService.Labels[LabelServiceProxyName] = providers.NSXTServiceProxy

				newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				Expect(newService.Spec.ExternalName).To(Equal(externalName))
				Expect(newService.Spec.LoadBalancerIP).To(Equal(loadBalancerIP))
				Expect(newService.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeLocal))
				Expect(newService.Labels[LabelServiceProxyName]).To(Equal(providers.NSXTServiceProxy)) // TODO: Don't fake NSX-T here
				Expect(newService.Spec.LoadBalancerSourceRanges).To(Equal([]string{"1.1.1.0/24", "2.2.2.2/28"}))
			})

			It("Should not clobber the nodePort while updating service", func() {
				service := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, serviceKey, service)).To(Succeed())

				nodePortPreUpdate := service.Spec.Ports[0].NodePort

				// Modify the VirtualMachineService to trigger an update in backing k8s service.
				externalName := "someExternalName"
				vmService.Spec.ExternalName = externalName

				newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))
				Expect(newService.Spec.Ports[0].NodePort).To(Equal(nodePortPreUpdate))
			})

			It("Should not update the k8s Service if there is no update", func() {
				service := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, serviceKey, service)).To(Succeed())

				_, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				// Channel should not receive an Update event
				Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))

				Eventually(func() bool {
					newService := &corev1.Service{}
					err = ctx.Client.Get(ctx, serviceKey, newService)
					return err == nil && apiEquality.Semantic.DeepEqual(service, newService)
				})
			})

			It("Should not update the k8s Service with change in VirtualMachine's selector", func() {
				service := &corev1.Service{}
				Expect(ctx.Client.Get(ctx, serviceKey, service)).To(Succeed())

				// Modify the VirtualMachineService's selector
				selector := map[string]string{"bar": "foo"}
				vmService.Spec.Selector = selector

				_, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				// Channel should not receive an Update event
				Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))

				Eventually(func() bool {
					newService := &corev1.Service{}
					err = ctx.Client.Get(ctx, serviceKey, newService)
					return err == nil && apiEquality.Semantic.DeepEqual(service, newService)
				})
			})
		})

		Describe("When VirtualMachineService selector matches virtual machines", func() {
			var (
				vm1       vmopv1alpha1.VirtualMachine
				vm2       vmopv1alpha1.VirtualMachine
				vmService vmopv1alpha1.VirtualMachineService
			)

			BeforeEach(func() {
				labels := map[string]string{"vm-match-selector": "true"}
				vm1 = getTestVirtualMachineWithLabels(ctx.Namespace, "dummy-vm-match-selector-1", labels)
				Expect(ctx.Client.Create(ctx, &vm1)).To(Succeed())
				vm2 = getTestVirtualMachine(ctx.Namespace, "dummy-vm-match-selector-2")
				Expect(ctx.Client.Create(ctx, &vm2)).To(Succeed())
				vmService = getTestVMServiceWithSelector(ctx.Namespace, "dummy-vm-service-match-selector", labels)
				Expect(ctx.Client.Create(ctx, &vmService)).To(Succeed())
			})

			It("Should use VirtualMachineService selector instead of Service selector", func() {
				vmList, err := reconciler.GetVirtualMachinesSelectedByVmService(ctx, &vmService)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(vmList.Items).To(HaveLen(1))
				Expect(vmList.Items[0].Name).To(Equal(vm1.Name))
			})
		})

		Describe("When Service has labels", func() {
			var (
				svcLabelKey   string
				svcLabelValue string
				serviceName   string
				service       *corev1.Service
				vmService     *vmopv1alpha1.VirtualMachineService
				vmServiceCtx  *context.VirtualMachineServiceContext
			)

			BeforeEach(func() {
				serviceName = "dummy-label-service"
				service = getService(serviceName, ctx.Namespace)
				svcLabelKey = "a.run.tanzu.vmware.com.label.for.lbapi"
				svcLabelValue = "a.label.value"
				service.Labels = map[string]string{svcLabelKey: svcLabelValue}
				Expect(ctx.Client.Create(ctx, service)).To(Succeed())

				vmService = getVmService(serviceName, ctx.Namespace)
				Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())

				vmServiceCtx = &context.VirtualMachineServiceContext{
					Context:   ctx,
					Logger:    ctx.Logger,
					VMService: vmService,
				}
			})

			Context("When there are existing labels on the Service, but none on the VMService", func() {
				It("Should preserve existing labels on the Service and shouldn't update the k8s Service", func() {
					newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Labels).To(HaveLen(1))
					Expect(newService.Labels).To(HaveKeyWithValue(svcLabelKey, svcLabelValue))

					// Channel shouldn't receive an Update event
					Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))
				})
			})

			Context("when both the VirtualMachineService and the Service have non-intersecting labels", func() {
				It("Should have union of labels from Service and VirtualMachineService and should update the k8s Service", func() {
					// Set label on VirtualMachineService that is non-conflicting with the Service.
					labelKey := "non-intersecting-label-key"
					labelValue := "label-value"
					vmService.Labels = map[string]string{labelKey: labelValue}

					newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Labels).To(HaveLen(2))
					Expect(newService.Labels).To(HaveKeyWithValue(svcLabelKey, svcLabelValue))
					Expect(newService.Labels).To(HaveKeyWithValue(labelKey, labelValue))

					// Channel should receive an Update event
					expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))
				})
			})

			Context("when both the VirtualMachineService and the Service have conflicting labels", func() {
				It("Should have union of labels from Service and VirtualMachineService, with VirtualMachineService "+
					"winning the conflict and should update the k8s Service", func() {
					// Set label on VirtualMachineService that is non-conflicting with the Service.
					vmService.Labels = make(map[string]string)
					labelValue := "label-value"
					vmService.Labels[svcLabelKey] = labelValue

					newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Labels).To(HaveLen(1))
					Expect(newService.Labels).To(HaveKeyWithValue(svcLabelKey, labelValue))

					// Channel should receive an Update event
					expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))
				})
			})
		})

		Describe("When Service has new annotations", func() {
			var (
				serviceName          string
				serviceAnnotationKey string
				serviceAnnotationVal string
				vmService            *vmopv1alpha1.VirtualMachineService
				vmServiceCtx         *context.VirtualMachineServiceContext
			)

			BeforeEach(func() {
				serviceName = "dummy-service"
				service := getService(serviceName, ctx.Namespace)

				annotations := service.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				serviceAnnotationKey = "dummy-service-annotation"
				serviceAnnotationVal = "blah"
				annotations[serviceAnnotationKey] = serviceAnnotationVal
				service.SetAnnotations(annotations)

				Expect(ctx.Client.Create(ctx, service)).To(Succeed())

				vmService = getVmService(serviceName, ctx.Namespace)
				Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())

				vmServiceCtx = &context.VirtualMachineServiceContext{
					Context:   ctx,
					Logger:    ctx.Logger,
					VMService: vmService,
				}
			})

			Context("When there are new annotations added to the VMSVC", func() {
				It("Should update the Service with new annotations", func() {
					newServiceAnnotationKey := "new-dummy-service-annotation"
					newServiceAnnotationVal := "ha"
					if vmService.Annotations == nil {
						vmService.Annotations = make(map[string]string)
					}
					vmService.Annotations[newServiceAnnotationKey] = newServiceAnnotationVal
					newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Annotations).To(HaveKeyWithValue(newServiceAnnotationKey, newServiceAnnotationVal))
					expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))
				})
			})

			Context("When there are annotations added to the Service", func() {
				It("Should preserve existing annotations on the Service and shouldn't update the k8s Service", func() {
					newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Annotations).To(HaveKeyWithValue(serviceAnnotationKey, serviceAnnotationVal))
					Expect(ctx.Events).ShouldNot(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))
				})
			})
		})

		Describe("UpdateVmStatus", func() {

			It("Should add VirtualMachineService Status and Annotations if they dont exist", func() {
				vmServiceName := "service-add-status-annotations"
				vmService := getVmService(vmServiceName, ctx.Namespace)
				Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())

				vmServiceCtx := &context.VirtualMachineServiceContext{
					Context:   ctx,
					Logger:    ctx.Logger,
					VMService: vmService,
				}

				service := &corev1.Service{}
				Expect(reconciler.UpdateVmService(vmServiceCtx, service)).To(Succeed())

				// Eventually, the VM service should be updated with the LB Ingress IPs and annotations.
				Eventually(func() bool {
					vmSvc := &vmopv1alpha1.VirtualMachineService{}
					vmSvcKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}
					Expect(ctx.Client.Get(ctx, vmSvcKey, vmSvc)).To(Succeed())

					vmSvcLBIngress := vmSvc.Status.LoadBalancer.Ingress
					svcLBIngress := service.Status.LoadBalancer.Ingress
					if len(svcLBIngress) != len(vmSvcLBIngress) {
						return false
					}

					for idx, ingress := range vmSvcLBIngress {
						if ingress.IP != svcLBIngress[idx].IP || ingress.Hostname != svcLBIngress[idx].Hostname {
							return false
						}
					}

					// Ensure that correct annotations are set.
					if val, ok := vmSvc.GetAnnotations()[pkg.VmOperatorVersionKey]; !ok || val != "v1" {
						return false
					}

					return true
				}).Should(BeTrue())
			})
		})

		Describe("When Updating endpoints", func() {
			var (
				serviceName  string
				service      *corev1.Service
				vmService    *vmopv1alpha1.VirtualMachineService
				vmServiceCtx *context.VirtualMachineServiceContext
			)

			BeforeEach(func() {
				serviceName = "dummy-endpoints-service"
				service = getService(serviceName, ctx.Namespace)

				vmService = getVmService(serviceName, ctx.Namespace)
				vmServiceCtx = &context.VirtualMachineServiceContext{
					Context:   ctx,
					Logger:    ctx.Logger,
					VMService: vmService,
				}

				err := ctx.Client.Create(ctx, service)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, service)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should update endpoints when it is not the same with existing endpoints", func() {
				// We need the VMService because the endpoints use the its UID as a OwnerReference
				Expect(ctx.Client.Create(ctx, vmService)).To(Succeed())

				currentEndpoints := &corev1.Endpoints{}

				// Dummy Service with updated fields.
				port := getServicePort("foo", "TCP", 44, 8080, 30007)
				changedService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      serviceName,
						Labels:    map[string]string{"foo": "bar"},
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{port},
					},
				}

				err := reconciler.UpdateEndpoints(vmServiceCtx, changedService)
				Expect(err).ShouldNot(HaveOccurred())

				// Wait for the Endpoint to be created.
				newEndpoints := &corev1.Endpoints{}
				Eventually(func() error {
					return ctx.Client.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: ctx.Namespace}, newEndpoints)
				}).Should(Succeed())

				Expect(currentEndpoints).NotTo(Equal(newEndpoints))
			})

			It("should not update endpoints when it is the same with existing endpoints", func() {
				endpoints := &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      serviceName,
					},
				}
				err := ctx.Client.Create(ctx, endpoints)
				Expect(err).ShouldNot(HaveOccurred())

				currentEndpoints := &corev1.Endpoints{}
				err = ctx.Client.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, currentEndpoints)
				Expect(err).NotTo(HaveOccurred())

				err = reconciler.UpdateEndpoints(vmServiceCtx, service)
				Expect(err).ShouldNot(HaveOccurred())

				newEndpoints := &corev1.Endpoints{}
				Eventually(func() error {
					return ctx.Client.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, newEndpoints)
				}).Should(Succeed())

				Expect(currentEndpoints).To(Equal(newEndpoints))
			})
		})
	})
}

// This duplicates too much of the unitTestsReconcile() but hard to combine. Need to fix
// the LBProvider interface changes made in b56b10e. These tests kind of stink.
//nolint:dupl goconst
func nsxtLBProviderTestsReconcile() {

	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController

		lbProvider providers.LoadbalancerProvider
		reconciler *virtualmachineservice.ReconcileVirtualMachineService
	)

	BeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		lbProvider = providers.NsxtLoadBalancerProvider()
		reconciler = virtualmachineservice.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Scheme,
			ctx.Recorder,
			lbProvider,
		)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	Describe("Reconcile k8s Service", func() {

		var (
			serviceName   string
			service       *corev1.Service
			vmServiceName string
			vmService     *vmopv1alpha1.VirtualMachineService
			vmServiceCtx  *context.VirtualMachineServiceContext
		)

		Describe("Create Or Update k8s Service", func() {
			BeforeEach(func() {
				vmServiceName = "nsxt-dummy-service"
				vmService = getVmService(vmServiceName, ctx.Namespace)

				vmServiceCtx = &context.VirtualMachineServiceContext{
					Context:   ctx,
					Logger:    ctx.Logger,
					VMService: vmService,
				}

				serviceName = vmServiceName
				service = getService(serviceName, ctx.Namespace)

				err := ctx.Client.Create(ctx, service)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, service)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should update the k8s Service to match with the VirtualMachineService", func() {
				svc := corev1.Service{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: ctx.Namespace, Name: serviceName}, &svc)).To(Succeed())

				// Modify the VirtualMachineService, corresponding Service should also be modified.
				externalName := "someExternalName"
				vmService.Spec.ExternalName = externalName
				loadBalancerIP := "1.1.1.1"
				vmService.Spec.LoadBalancerIP = loadBalancerIP
				vmService.Spec.LoadBalancerSourceRanges = []string{"1.1.1.0/24", "2.2.2.2/28"}
				if vmService.Annotations == nil {
					vmService.Annotations = make(map[string]string)
				}
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
				if vmService.Labels == nil {
					vmService.Labels = make(map[string]string)
				}
				vmService.Labels[LabelServiceProxyName] = providers.NSXTServiceProxy

				newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				Expect(newService.Spec.ExternalName).To(Equal(externalName))
				Expect(newService.Spec.LoadBalancerIP).To(Equal(loadBalancerIP))
				Expect(newService.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeLocal))
				Expect(newService.Labels[LabelServiceProxyName]).To(Equal(providers.NSXTServiceProxy))
				Expect(newService.Spec.LoadBalancerSourceRanges).To(Equal([]string{"1.1.1.0/24", "2.2.2.2/28"}))
			})

			It("Should update the k8s Service to match with the VirtualMachineService when LoadBalancerSourceRanges is cleared", func() {
				service.Spec.LoadBalancerSourceRanges = []string{"1.1.1.0/24", "2.2.2.2/28"}
				Expect(ctx.Client.Update(ctx, service)).To(Succeed())

				vmService.Spec.LoadBalancerSourceRanges = []string{}

				newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))
				Expect(newService.Spec.LoadBalancerSourceRanges).To(HaveLen(0))
			})

			It("Should update the k8s Service to match with the VirtualMachineService when externalTrafficPolicy is cleared", func() {
				By("applying externalTrafficPolicy and healthCheckNodePort annotations")
				if service.Annotations == nil {
					service.Annotations = make(map[string]string)
				}
				service.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				service.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"

				By("applying service-proxy label")
				if service.Labels == nil {
					service.Labels = make(map[string]string)
				}
				service.Labels[LabelServiceProxyName] = providers.NSXTServiceProxy
				service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
				Expect(ctx.Client.Update(ctx, service)).To(Succeed())

				delete(vmService.Annotations, utils.AnnotationServiceExternalTrafficPolicyKey)
				delete(vmService.Annotations, utils.AnnotationServiceHealthCheckNodePortKey)

				newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				// Channel should receive an Update event
				Expect(ctx.Events).Should(Receive(ContainSubstring(virtualmachineservice.OpUpdate)))

				Expect(newService.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeCluster))
				_, exist := newService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey]
				Expect(exist).To(BeFalse())
				_, exist = newService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey]
				Expect(exist).To(BeFalse())
				_, exist = newService.Labels[LabelServiceProxyName]
				Expect(exist).To(BeFalse())
			})

			It("Should update the k8s Service to remove the provider specific annotations regarding healthCheckNodePort", func() {
				By("adding provider specific healthCheckNodePort annotations to the Service")
				if vmService.Annotations == nil {
					vmService.Annotations = make(map[string]string)
				}
				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"

				annotations, err := lbProvider.GetServiceAnnotations(ctx, vmService)
				for k, v := range annotations {
					service.Annotations[k] = v
				}
				Expect(ctx.Client.Update(ctx, service)).To(Succeed())

				By("deleting the healthCheckNodePort annotation from vm service")
				delete(vmService.Annotations, utils.AnnotationServiceHealthCheckNodePortKey)

				By("reconciling the Service")
				newService, err := reconciler.CreateOrUpdateService(vmServiceCtx)
				Expect(err).ShouldNot(HaveOccurred())

				// Channel should receive an Update event
				expectEvent(ctx, ContainSubstring(virtualmachineservice.OpUpdate))

				By("ensuring the provider specific annotations are removed from the new service")
				annotationsToBeRemoved, err := lbProvider.GetToBeRemovedServiceAnnotations(ctx, vmService)
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
