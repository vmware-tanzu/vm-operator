// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizerName = "virtualmachineservice.vmoperator.vmware.com"
)

func intgTests() {

	var (
		ctx *builder.IntegrationTestContext
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {

		Describe("when creating/deleting a VM Service", func() {
			It("invoke the reconcile method", func() {
				vmService := getVmServiceOfType("foo-vm-service", ctx.Namespace, vmopv1alpha1.VirtualMachineServiceTypeClusterIP)
				vmServiceKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}

				// Add dummy labels and annotations to vmService
				dummyLabelKey := "dummy-label-key"
				dummyLabelVal := "dummy-label-val"
				dummyAnnotationKey := "dummy-annotation-key"
				dummyAnnotationVal := "dummy-annotation-val"
				vmService.Labels = map[string]string{
					dummyLabelKey: dummyLabelVal,
				}
				vmService.Annotations = map[string]string{
					dummyAnnotationKey: dummyAnnotationVal,
				}

				// Create the VM Service object and expect the Reconcile
				err := ctx.Client.Create(ctx, vmService)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() []string {
					vmService := &vmopv1alpha1.VirtualMachineService{}
					if err := ctx.Client.Get(ctx, vmServiceKey, vmService); err == nil {
						return vmService.GetFinalizers()
					}
					return nil
				}).Should(ContainElement(finalizerName))

				service := corev1.Service{}
				Eventually(func() error {
					serviceKey := client.ObjectKey{Name: vmService.Name, Namespace: vmService.Namespace}
					return ctx.Client.Get(ctx, serviceKey, &service)
				}).Should(Succeed())

				// Service should have label and annotations replicated on vmService create
				Expect(service.Labels).To(HaveKeyWithValue(dummyLabelKey, dummyLabelVal))
				Expect(service.Annotations).To(HaveKeyWithValue(dummyAnnotationKey, dummyAnnotationVal))

				// Delete the VM Service object and expect it to be deleted.
				Expect(ctx.Client.Delete(ctx, vmService)).To(Succeed())

				Eventually(func() []string {
					vmService := &vmopv1alpha1.VirtualMachineService{}
					if err := ctx.Client.Get(ctx, vmServiceKey, vmService); err == nil {
						return vmService.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizerName))
			})
		})

		Describe("When VirtualMachineService selector matches virtual machines with no IP", func() {
			var (
				vm1       vmopv1alpha1.VirtualMachine
				vm2       vmopv1alpha1.VirtualMachine
				vmService vmopv1alpha1.VirtualMachineService
			)

			BeforeEach(func() {
				// Create dummy VMs and rely on the the fact that they will not get an IP
				// since there is no VM controller to reconcile the VMs.
				vm1 = getTestVirtualMachine(ctx.Namespace, "dummy-vm-with-no-ip-1")
				vm2 = getTestVirtualMachine(ctx.Namespace, "dummy-vm-with-no-ip-2")
				vmService = getTestVMService(ctx.Namespace, "dummy-vm-service-no-ip")
				createObjects(ctx, ctx.Client, []runtime.Object{&vm1, &vm2, &vmService})
			})

			AfterEach(func() {
				deleteObjects(ctx, ctx.Client, []runtime.Object{&vm1, &vm2, &vmService})
			})

			It("Should create Service and Endpoints with no subsets", func() {
				assertServiceWithNoEndpointSubsets(ctx, ctx.Client, vmService.Namespace, vmService.Name)
			})
		})

		XDescribe("When VirtualMachineService selector matches virtual machines with no probe", func() {
			var (
				vm1       vmopv1alpha1.VirtualMachine
				vm2       vmopv1alpha1.VirtualMachine
				vmService vmopv1alpha1.VirtualMachineService
			)

			BeforeEach(func() {
				label := map[string]string{"no-probe": "true"}
				vm1 = getTestVirtualMachineWithLabels(ctx.Namespace, "dummy-vm-with-no-probe-1", label)
				vm2 = getTestVirtualMachineWithLabels(ctx.Namespace, "dummy-vm-with-no-probe-2", label)
				vmService = getTestVMServiceWithSelector(ctx.Namespace, "dummy-vm-service-no-probe", label)
				createObjects(ctx, ctx.Client, []runtime.Object{&vm1, &vm2, &vmService})

				// Since Status is a sub-resource, we need to update the status separately.
				vm1.Status.VmIp = "192.168.1.100"
				vm1.Status.Host = "10.0.0.100"
				vm2.Status.VmIp = "192.168.1.200"
				vm2.Status.Host = "10.0.0.200"
				updateObjectsStatus(ctx, ctx.Client, []runtime.Object{&vm1, &vm2})
			})

			AfterEach(func() {
				deleteObjects(ctx, ctx.Client, []runtime.Object{&vm1, &vm2, &vmService})
			})

			It("Should create Service and Endpoints with subsets", func() {
				subsets := assertServiceWithEndpointSubsets(ctx, ctx.Client, vmService.Namespace, vmService.Name)
				if subsets[0].Addresses[0].IP == vm1.Status.VmIp {
					Expect(subsets[0].Addresses[1].IP).To(Equal(vm2.Status.VmIp))
				} else {
					Expect(subsets[0].Addresses[0].IP).To(Equal(vm1.Status.VmIp))
				}
			})
		})

		Describe("When VirtualMachineService selector matches virtual machines with probe", func() {
			var (
				successVM         vmopv1alpha1.VirtualMachine
				failedVM          vmopv1alpha1.VirtualMachine
				vmService         vmopv1alpha1.VirtualMachineService
				readyCondition    *vmopv1alpha1.Condition
				notReadyCondition *vmopv1alpha1.Condition
			)

			BeforeEach(func() {
				// Setup two VM's one that will pass the readiness probe and one that will fail
				label := map[string]string{"with-probe": "true"}
				successVM = getTestVirtualMachineWithProbe(ctx.Namespace, "dummy-vm-with-success-probe", label, 10001)
				failedVM = getTestVirtualMachineWithProbe(ctx.Namespace, "dummy-vm-with-failure-probe", label, 10001)
				vmService = getTestVMServiceWithSelector(ctx.Namespace, "dummy-vm-service-with-probe", label)
				createObjects(ctx, ctx.Client, []runtime.Object{&successVM, &failedVM, &vmService})

				successVM.Status.VmIp = "192.168.1.100"
				successVM.Status.Host = "10.0.0.100"
				readyCondition = conditions.TrueCondition(vmopv1alpha1.ReadyCondition)
				conditions.Set(&successVM, readyCondition)
				failedVM.Status.VmIp = "192.168.1.200"
				failedVM.Status.Host = "10.0.0.200"
				notReadyCondition = conditions.FalseCondition(vmopv1alpha1.ReadyCondition, "notReady", vmopv1alpha1.ConditionSeverityInfo, "")
				conditions.Set(&failedVM, notReadyCondition)
				updateObjectsStatus(ctx, ctx.Client, []runtime.Object{&successVM, &failedVM})
			})

			It("Should create Service and Endpoints with VMs that pass the probe", func() {
				subsets := assertServiceWithEndpointSubsets(ctx, ctx.Client, vmService.Namespace, vmService.Name)
				Expect(subsets[0].Addresses[0].IP).To(Equal(successVM.Status.VmIp))
			})

			When("VMs are not in the Old Service endpoint subsets", func() {

				It("Should not add VM to endpoint subsets when Ready Condition is missing if readiness probe is set", func() {
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: failedVM.Name, Namespace: failedVM.Namespace}, &failedVM)).To(Succeed())
					conditions.Delete(&failedVM, vmopv1alpha1.ReadyCondition)
					updateObjectsStatus(ctx, ctx.Client, []runtime.Object{&failedVM})

					// This will race.
					time.Sleep(3 * time.Second)

					subsets := assertServiceWithEndpointSubsets(ctx, ctx.Client, vmService.Namespace, vmService.Name)
					Expect(subsets).To(HaveLen(1))
					Expect(subsets[0].Addresses[0].IP).To(Equal(successVM.Status.VmIp))
				})

				It("Should add VM to endpoint subsets if readiness probe is not set", func() {
					failedVM.Spec.ReadinessProbe = nil
					updateObjects(ctx, ctx.Client, []runtime.Object{&failedVM})

					// This will race.
					time.Sleep(3 * time.Second)

					subsets := assertServiceWithEndpointSubsets(ctx, ctx.Client, vmService.Namespace, vmService.Name)
					Expect(subsets).To(HaveLen(1))
					if subsets[0].Addresses[0].IP == successVM.Status.VmIp {
						Expect(subsets[0].Addresses[1].IP).To(Equal(failedVM.Status.VmIp))
					} else {
						Expect(subsets[0].Addresses[0].IP).To(Equal(failedVM.Status.VmIp))
					}
				})
			})

			XWhen("VMs are in the Old Service endpoint subsets", func() {
				It("Should not remove VM if its Ready condition is missing in the status", func() {
					conditions.Delete(&successVM, vmopv1alpha1.ReadyCondition)
					updateObjectsStatus(ctx, ctx.Client, []runtime.Object{&successVM})

					// This will race - will actually remove the VM if not fast enough.
					subsets := assertServiceWithEndpointSubsets(ctx, ctx.Client, vmService.Namespace, vmService.Name)
					Expect(subsets[0].Addresses[0].IP).To(Equal(successVM.Status.VmIp))
				})
			})

			AfterEach(func() {
				deleteObjects(ctx, ctx.Client, []runtime.Object{&successVM, &failedVM, &vmService})

				Eventually(func() bool {
					serviceKey := client.ObjectKey{Name: vmService.Name, Namespace: vmService.Namespace}
					err := ctx.Client.Get(ctx, serviceKey, &vmService)
					return errors.IsNotFound(err)
				}).Should(BeTrue())
			})
		})
	})
}

func getTestVirtualMachine(namespace, name string) vmopv1alpha1.VirtualMachine {
	return getTestVirtualMachineWithLabels(namespace, name, map[string]string{"foo": "bar"})
}

func getTestVirtualMachineWithLabels(namespace, name string, labels map[string]string) vmopv1alpha1.VirtualMachine {
	return vmopv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: vmopv1alpha1.VirtualMachineSpec{
			PowerState: "poweredOn",
			ImageName:  "test",
			ClassName:  "TEST",
		},
	}
}

func getTestVirtualMachineWithProbe(namespace, name string, labels map[string]string, port int) vmopv1alpha1.VirtualMachine {
	vm := getTestVirtualMachineWithLabels(namespace, name, labels)
	vm.Spec.Ports = []vmopv1alpha1.VirtualMachinePort{
		{
			Protocol: corev1.ProtocolTCP,
			Port:     port,
			Name:     "my-service",
		},
	}
	vm.Spec.ReadinessProbe = &vmopv1alpha1.Probe{
		TCPSocket: &vmopv1alpha1.TCPSocketAction{
			Port: intstr.FromInt(port),
		},
	}
	return vm
}

func getTestVMService(namespace, name string) vmopv1alpha1.VirtualMachineService {
	return getTestVMServiceWithSelector(namespace, name, map[string]string{"foo": "bar"})
}

func getTestVMServiceWithSelector(namespace, name string, selector map[string]string) vmopv1alpha1.VirtualMachineService {
	return vmopv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmopv1alpha1.VirtualMachineServiceSpec{
			Type:     vmopv1alpha1.VirtualMachineServiceTypeClusterIP,
			Selector: selector,
			Ports: []vmopv1alpha1.VirtualMachineServicePort{
				{
					Name:       "test",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 80,
				},
			},
		},
	}
}

func getServicePort(name string, protocol corev1.Protocol, port, targetPort int32, nodePort int32) corev1.ServicePort {
	return corev1.ServicePort{
		Name:     name,
		Protocol: protocol,
		Port:     port,
		TargetPort: intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: targetPort,
		},
		NodePort: nodePort,
	}
}

func getVmServicePort(name, protocol string, port, targetPort int32) vmopv1alpha1.VirtualMachineServicePort {
	return vmopv1alpha1.VirtualMachineServicePort{
		Name:       name,
		Protocol:   protocol,
		Port:       port,
		TargetPort: targetPort,
	}
}

func getService(name, namespace string) *corev1.Service {
	// Get ServicePort with dummy values.
	port := getServicePort("foo", "TCP", 42, 42, 30007)

	// Get annotations
	const VmOperatorVersionKey string = "vmoperator.vmware.com/version"
	annotations := make(map[string]string)
	annotations[VmOperatorVersionKey] = "v1"

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
			Ports:                 []corev1.ServicePort{port},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP:       "10.0.0.1",
						Hostname: "TEST",
					},
				},
			},
		},
	}
}

func getVmService(name, namespace string) *vmopv1alpha1.VirtualMachineService {
	// Get VirtualMachineServicePort with dummy values.
	vmServicePort := getVmServicePort("foo", "TCP", 42, 42)

	return &vmopv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmopv1alpha1.VirtualMachineServiceSpec{
			Type:     vmopv1alpha1.VirtualMachineServiceTypeLoadBalancer,
			Ports:    []vmopv1alpha1.VirtualMachineServicePort{vmServicePort},
			Selector: map[string]string{"foo": "bar"},
		},
	}
}

func getVmServiceOfType(name, namespace string, serviceType vmopv1alpha1.VirtualMachineServiceType) *vmopv1alpha1.VirtualMachineService {
	// Get VirtualMachineServicePort with dummy values.
	vmServicePort := getVmServicePort("foo", "TCP", 42, 42)

	return &vmopv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmopv1alpha1.VirtualMachineServiceSpec{
			Type:     serviceType,
			Ports:    []vmopv1alpha1.VirtualMachineServicePort{vmServicePort},
			Selector: map[string]string{"foo": "bar"},
		},
	}
}

func createObjects(ctx context.Context, ctrlClient client.Client, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Create(ctx, obj)).To(Succeed())
	}
}

func updateObjects(ctx context.Context, ctrlClient client.Client, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Update(ctx, obj)).To(Succeed())
	}
}

func updateObjectsStatus(ctx context.Context, ctrlClient client.StatusClient, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Status().Update(ctx, obj)).To(Succeed())
	}
}

func deleteObjects(ctx context.Context, ctrlClient client.Client, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Delete(ctx, obj)).To(Succeed())
	}
}

func assertEventuallyExistsInNamespace(ctx context.Context, ctrlClient client.Client, namespace, name string, obj runtime.Object) {
	EventuallyWithOffset(2, func() error {
		key := client.ObjectKey{Namespace: namespace, Name: name}
		return ctrlClient.Get(ctx, key, obj)
	}).Should(Succeed())
}

func assertService(ctx context.Context, ctrlClient client.Client, namespace, name string) {
	service := &corev1.Service{}
	assertEventuallyExistsInNamespace(ctx, ctrlClient, namespace, name, service)
}

func assertServiceWithNoEndpointSubsets(ctx context.Context, ctrlClient client.Client, namespace, name string) {
	assertService(ctx, ctrlClient, namespace, name)
	endpoints := &corev1.Endpoints{}
	assertEventuallyExistsInNamespace(ctx, ctrlClient, namespace, name, endpoints)
	Expect(endpoints.Subsets).To(BeEmpty())
}

func assertServiceWithEndpointSubsets(ctx context.Context, ctrlClient client.Client, namespace, name string) []corev1.EndpointSubset {
	assertService(ctx, ctrlClient, namespace, name)
	endpoints := &corev1.Endpoints{}
	assertEventuallyExistsInNamespace(ctx, ctrlClient, namespace, name, endpoints)
	Expect(endpoints.Subsets).NotTo(BeEmpty())
	return endpoints.Subsets
}
