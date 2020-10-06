// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	clientgorecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/pkg"
	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const timeout = time.Second * 30

var _ = Describe("VirtualMachineService controller", func() {
	SetDefaultEventuallyTimeout(timeout)

	var (
		ctx              context.Context
		recFn            reconcile.Reconciler
		requests         chan reconcile.Request
		reconcileResult  chan reconcile.Result
		reconcileErr     chan error
		stopMgr          chan struct{}
		mgrStopped       *sync.WaitGroup
		mgr              manager.Manager
		err              error
		r                *ReconcileVirtualMachineService
		serviceNamespace string
	)

	BeforeEach(func() {
		serviceNamespace = integration.DefaultNamespace

		ctrlContext := &controllerContext.ControllerManagerContext{
			Logger: ctrllog.Log.WithName("test"),
		}

		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		mgr, err = manager.New(cfg, manager.Options{
			SyncPeriod:         &syncPeriod,
			MetricsBindAddress: "0",
		})
		Expect(err).NotTo(HaveOccurred())

		r, err = newReconciler(mgr)
		Expect(err).NotTo(HaveOccurred())
		recFn, requests, reconcileResult, reconcileErr = integration.SetupTestReconcile(r)

		Expect(add(ctrlContext, mgr, recFn, r)).To(Succeed())

		stopMgr, mgrStopped = integration.StartTestManager(mgr)

		// Context for tests to use.
		ctx = context.TODO()

		// Set up a client for tests to use that will talk to API server directly. We can not use the maanger client here because it talks
		// to the cache which might take some time to sync after we write objects to it.
		c, err = client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
		Expect(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("when creating/deleting a VM Service", func() {
		It("invoke the reconcile method", func() {
			name := "foo-vmservice"
			vmService := getVmServiceOfType(name, serviceNamespace, vmoperatorv1alpha1.VirtualMachineServiceTypeClusterIP)

			// Add dummy labels and annotations to vmService
			dummyLabelKey := "dummy-label-key"
			dummyLabelVal := "dummy-label-val"
			dummyAnnotationKey := "dummy-annotation-key"
			dummyAnnotationVal := "dummy-annotation-val"
			vmService.Labels = make(map[string]string)
			vmService.Annotations = make(map[string]string)
			vmService.Labels[dummyLabelKey] = dummyLabelVal
			vmService.Annotations[dummyAnnotationKey] = dummyAnnotationVal

			// Create the VM Service object and expect the Reconcile
			err := c.Create(ctx, vmService)
			Expect(err).ShouldNot(HaveOccurred())
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: serviceNamespace, Name: name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Service should have been created with same name
			serviceKey := client.ObjectKey{Name: vmService.Name, Namespace: vmService.Namespace}
			service := corev1.Service{}
			Eventually(func() error {
				return c.Get(ctx, serviceKey, &service)
			}).Should(Succeed())

			// Service should have label and annotations replicated on vmService create
			Expect(service.Labels).To(HaveKeyWithValue(dummyLabelKey, dummyLabelVal))
			Expect(service.Annotations).To(HaveKeyWithValue(dummyAnnotationKey, dummyAnnotationVal))

			// Delete the VM Service object and expect it to be deleted.
			err = c.Delete(ctx, vmService)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			Eventually(func() bool {
				vmService := vmoperatorv1alpha1.VirtualMachineService{}
				err = c.Get(ctx, serviceKey, &vmService)
				if errors.IsNotFound(err) {
					return true
				}
				if err != nil {
					GinkgoWriter.Write([]byte(fmt.Sprintf("Unexpected error: %#v\n", err)))
				}
				return false
			}).Should(BeTrue())
		})
	})

	Describe("When VirtualMachineService selector matches virtual machines with no IP", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			// Create dummy VMs and rely on the the fact that they will not get an IP since there is no VM controller to reconcile the VMs.
			vm1 = getTestVirtualMachine(serviceNamespace, "dummy-vm-with-no-ip-1")
			vm2 = getTestVirtualMachine(serviceNamespace, "dummy-vm-with-no-ip-2")
			vmService = getTestVMService(serviceNamespace, "dummy-vm-service-no-ip")
			createObjects(ctx, c, []runtime.Object{&vm1, &vm2, &vmService})
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: serviceNamespace, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should create Service and Endpoints with no subsets", func() {
			assertServiceWithNoEndpointSubsets(ctx, c, vmService.Namespace, vmService.Name)
		})
		AfterEach(func() {
			deleteObjects(ctx, c, []runtime.Object{&vm1, &vm2, &vmService})
		})
	})

	Describe("When VirtualMachineService selector matches virtual machines with no probe", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			label := map[string]string{"no-probe": "true"}
			vm1 = getTestVirtualMachineWithLabels(serviceNamespace, "dummy-vm-with-no-probe-1", label)
			vm2 = getTestVirtualMachineWithLabels(serviceNamespace, "dummy-vm-with-no-probe-2", label)
			vmService = getTestVMServiceWithSelector(serviceNamespace, "dummy-vm-service-no-probe", label)
			createObjects(ctx, c, []runtime.Object{&vm1, &vm2, &vmService})

			// Since Status is a sub-resource, we need to update the status separately.
			vm1.Status.VmIp = "192.168.1.100"
			vm1.Status.Host = "10.0.0.100"
			vm2.Status.VmIp = "192.168.1.200"
			vm2.Status.Host = "10.0.0.200"
			updateObjectsStatus(ctx, c, []runtime.Object{&vm1, &vm2})

			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: serviceNamespace, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})

		It("Should create Service and Endpoints with subsets", func() {
			subsets := assertServiceWithEndpointSubsets(ctx, c, vmService.Namespace, vmService.Name)
			if subsets[0].Addresses[0].IP == vm1.Status.VmIp {
				Expect(subsets[0].Addresses[1].IP).To(Equal(vm2.Status.VmIp))
			} else {
				Expect(subsets[0].Addresses[0].IP).To(Equal(vm1.Status.VmIp))
			}
		})

		AfterEach(func() {
			deleteObjects(ctx, c, []runtime.Object{&vm1, &vm2, &vmService})
		})
	})

	Describe("When VirtualMachineService selector matches virtual machines with probe", func() {
		var (
			successVM  vmoperatorv1alpha1.VirtualMachine
			failedVM   vmoperatorv1alpha1.VirtualMachine
			vmService  vmoperatorv1alpha1.VirtualMachineService
			testServer *httptest.Server
			testHost   string
		)
		BeforeEach(func() {
			// A test server that will simulate a VM's readiness probe endpoint
			var testPort int
			testServer, testHost, testPort = setupTestServer()
			// Setup two VM's one that will pass the readiness probe and one that will fail
			label := map[string]string{"with-probe": "true"}
			successVM = getTestVirtualMachineWithProbe(serviceNamespace, "dummy-vm-with-success-probe", label, testPort)
			failedVM = getTestVirtualMachineWithProbe(serviceNamespace, "dummy-vm-with-failure-probe", label, 10001)
			vmService = getTestVMServiceWithSelector(serviceNamespace, "dummy-vm-service-with-probe", label)
			createObjects(ctx, c, []runtime.Object{&successVM, &failedVM, &vmService})
			// Since Status is a sub-resource, we need to update the status separately.
			successVM.Status.VmIp = testHost
			successVM.Status.Host = "10.0.0.100"
			failedVM.Status.VmIp = "192.168.1.200"
			failedVM.Status.Host = "10.0.0.200"
			updateObjectsStatus(ctx, c, []runtime.Object{&successVM, &failedVM})

			// Wait for the reconciliation to happen before asserting
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: serviceNamespace, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should create Service and Endpoints with VMs that pass the probe", func() {
			// Note: Ideally we would expect the successVM with local IP 127.0.0.1 to be part of the endpoint. However,
			// kubernetes doesn't allow endpoints to have an IP address in the loopback range. As a result,
			// we end up with an error trying to create the endpoints for the VMService. The fact that we tried to add
			// an endpoint with the loopback address from the test server asserts that the endpoint passed the
			// readiness probe.
			// subsets := assertServiceWithEndpointSubsets(ctx, c, vmService.Namespace, vmService.Name)
			// Expect(subsets[0].Addresses[0].IP).To(Equal(successVM.Status.VmIp))
			Eventually(reconcileErr, timeout).Should(Receive(
				MatchError(fmt.Sprintf("Endpoints \"dummy-vm-service-with-probe\" is invalid: subsets[0].addresses[0].ip: "+
					"Invalid value: \"%s\": may not be in the loopback range (127.0.0.0/8)", testHost))))

		})
		AfterEach(func() {
			testServer.Close()
			deleteObjects(ctx, c, []runtime.Object{&successVM, &failedVM, &vmService})
		})
	})

	Describe("When VirtualMachineService selector matches VM's and all of them fail probe", func() {
		var (
			failedVM  vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			label := map[string]string{"with-probe-requeue": "true"}
			failedVM = getTestVirtualMachineWithProbe(serviceNamespace, "dummy-vm-with-failure-probe-requeue", label, 10002)
			vmService = getTestVMServiceWithSelector(serviceNamespace, "dummy-vm-service-with-probe-requeue", label)
			createObjects(ctx, c, []runtime.Object{&failedVM, &vmService})
			// Since Status is a sub-resource, we need to update the status separately.
			failedVM.Status.VmIp = "192.168.1.300"
			failedVM.Status.Host = "10.0.0.300"
			updateObjectsStatus(ctx, c, []runtime.Object{&failedVM})

			// Wait for the reconciliation to happen before asserting
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: serviceNamespace, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should requeue", func() {
			expectedResult := reconcile.Result{RequeueAfter: probeFailureRequeueTime}
			Eventually(reconcileResult, timeout).Should(Receive(Equal(expectedResult)))
		})
		AfterEach(func() {
			deleteObjects(ctx, c, []runtime.Object{&failedVM, &vmService})
		})
	})

	Describe("When VirtualMachineService selector matches virtual machines", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			labels := map[string]string{"vm-match-selector": "true"}
			vm1 = getTestVirtualMachineWithLabels(serviceNamespace, "dummy-vm-match-selector-1", labels)
			vm2 = getTestVirtualMachine(serviceNamespace, "dummy-vm-match-selector-2")
			vmService = getTestVMServiceWithSelector(serviceNamespace, "dummy-vm-service-match-selector", labels)
			createObjects(ctx, c, []runtime.Object{&vm1, &vm2, &vmService})
		})
		It("Should use VirtualMachineService selector instead of Service selector", func() {
			vmList := &vmoperatorv1alpha1.VirtualMachineList{}
			var err error
			Eventually(func() int {
				vmList, err = r.getVirtualMachinesSelectedByVmService(ctx, &vmService)
				Expect(err).ShouldNot(HaveOccurred())
				return len(vmList.Items)
			}).Should(BeNumerically("==", 1))
			Expect(vmList.Items[0].Name).To(Equal(vm1.Name))
		})
		AfterEach(func() {
			deleteObjects(ctx, c, []runtime.Object{&vm1, &vm2, &vmService})
		})
	})

	Describe("Reconcile k8s Service", func() {
		var (
			serviceName   string
			service       *corev1.Service
			vmServiceName string
			vmService     *vmoperatorv1alpha1.VirtualMachineService
			recorder      record.Recorder
			events        chan string
		)

		BeforeEach(func() {
			recorder, events = newFakeRecorder()
			r.recorder = recorder
		})

		Describe("Create Or Update k8s Service", func() {
			BeforeEach(func() {
				serviceName = "dummy-service"
				service = getService(serviceName, serviceNamespace)

				vmServiceName = "dummy-service"
				vmService = getVmService(vmServiceName, serviceNamespace)

				err := c.Create(ctx, service)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, service)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should update the k8s Service to match with the VirtualMachineService", func() {
				svc := corev1.Service{}
				Expect(c.Get(ctx, client.ObjectKey{Namespace: serviceNamespace, Name: serviceName}, &svc)).To(Succeed())

				// Modify the VirtualMachineService, corresponding Service should also be modified.
				externalName := "someExternalName"
				vmService.Spec.ExternalName = externalName

				newService, err := r.createOrUpdateService(ctx, vmService)
				Expect(err).ShouldNot(HaveOccurred())

				// Channel should receive an Update event
				Expect(events).Should(Receive(ContainSubstring(OpUpdate)))

				Expect(newService.Spec.ExternalName).To(Equal(externalName))
			})

			It("Should not clobber the nodePort while updating service", func() {
				svc := corev1.Service{}
				Expect(c.Get(ctx, client.ObjectKey{Namespace: serviceNamespace, Name: serviceName}, &svc)).To(Succeed())
				nodePortPreUpdate := svc.Spec.Ports[0].NodePort

				// Modify the VirtualMachineService to trigger an update in backing k8s service.
				externalName := "someExternalName"
				vmService.Spec.ExternalName = externalName

				newService, err := r.createOrUpdateService(ctx, vmService)
				Expect(err).ShouldNot(HaveOccurred())

				// Channel should receive an Update event
				Expect(events).Should(Receive(ContainSubstring(OpUpdate)))

				Expect(newService.Spec.Ports[0].NodePort).To(Equal(nodePortPreUpdate))
			})

			It("Should not update the k8s Service if there is no update", func() {
				currentService := &corev1.Service{}
				err = c.Get(ctx, types.NamespacedName{service.Namespace, service.Name}, currentService)
				Expect(err).ShouldNot(HaveOccurred())

				_, err := r.createOrUpdateService(ctx, vmService)
				Expect(err).ShouldNot(HaveOccurred())

				//Channel should not receive an Update event
				Expect(events).ShouldNot(Receive(ContainSubstring(OpUpdate)))

				// Expect(apiEquality.Semantic.DeepEqual(newService, currentService)).To(BeTrue())
				// TypeMeta is not filled in the client go returned struct. Compare Spec and ObjectMeta explicitly.
				// https://github.com/kubernetes/client-go/issues/308
				// Expect(newService.ObjectMeta).To(Equal(currentService.ObjectMeta))
				Eventually(func() bool {
					newService := corev1.Service{}
					err = c.Get(ctx, types.NamespacedName{service.Namespace, service.Name}, &newService)

					return err == nil && apiEquality.Semantic.DeepEqual(newService, currentService)
				})
			})

			It("Should not update the k8s Service with change in VirtualMachine's selector", func() {
				currentService := &corev1.Service{}
				err = c.Get(ctx, types.NamespacedName{service.Namespace, service.Name}, currentService)
				Expect(err).ShouldNot(HaveOccurred())

				// Modify the VirtualMachineService's selector
				selector := map[string]string{"bar": "foo"}
				vmService.Spec.Selector = selector

				_, err := r.createOrUpdateService(ctx, vmService)
				Expect(err).ShouldNot(HaveOccurred())

				//Channel should not receive an Update event
				Expect(events).ShouldNot(Receive(ContainSubstring(OpUpdate)))

				Eventually(func() bool {
					newService := corev1.Service{}
					err = c.Get(ctx, types.NamespacedName{service.Namespace, service.Name}, &newService)

					return err == nil && apiEquality.Semantic.DeepEqual(newService, currentService)
				})
			})
		})

		Describe("When Service has labels", func() {
			var (
				svcLabelKey   string
				svcLabelValue string
				serviceName   string
				service       *corev1.Service
				vmService     *vmoperatorv1alpha1.VirtualMachineService
			)

			BeforeEach(func() {
				serviceName = "dummy-label-service"
				service = getService(serviceName, serviceNamespace)

				svcLabelKey = "a.run.tanzu.vmware.com.label.for.lbapi"
				svcLabelValue = "a.label.value"
				service.Labels = make(map[string]string)
				service.Labels[svcLabelKey] = svcLabelValue

				err := c.Create(ctx, service)
				Expect(err).NotTo(HaveOccurred())

				vmService = getVmService(serviceName, serviceNamespace)
				err = c.Create(ctx, vmService)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, service)
				Expect(err).NotTo(HaveOccurred())

				err = c.Delete(ctx, vmService)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("When there are existing labels on the Service, but none on the VMService", func() {
				It("Should preserve existing labels on the Service and shouldn't update the k8s Service", func() {
					newService, err := r.createOrUpdateService(ctx, vmService)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Labels).To(HaveLen(1))
					Expect(newService.Labels).To(HaveKeyWithValue(svcLabelKey, svcLabelValue))

					// Channel shouldn't receive an Update event
					Expect(events).ShouldNot(Receive(ContainSubstring(OpUpdate)))
				})
			})

			Context("when both the VirtualMachineService and the Service have non-intersecting labels", func() {
				It("Should have union of labels from Service and VirtualMachineService and should update the k8s Service", func() {
					// Set label on VirtualMachineService that is non-conflicting with the Service.
					vmService.Labels = make(map[string]string)
					labelKey := "non-intersecting-label-key"
					labelValue := "label-value"
					vmService.Labels[labelKey] = labelValue

					newService, err := r.createOrUpdateService(ctx, vmService)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Labels).To(HaveLen(2))
					Expect(newService.Labels).To(HaveKeyWithValue(svcLabelKey, svcLabelValue))
					Expect(newService.Labels).To(HaveKeyWithValue(labelKey, labelValue))

					// Channel should receive an Update event
					Expect(events).Should(Receive(ContainSubstring(OpUpdate)))
				})
			})

			Context("when both the VirtualMachineService and the Service have conflicting labels", func() {
				It("Should have union of labels from Service and VirtualMachineService, with VirtualMachineService "+
					"winning the conflict and should update the k8s Service", func() {
					// Set label on VirtualMachineService that is non-conflicting with the Service.
					vmService.Labels = make(map[string]string)
					labelValue := "label-value"
					vmService.Labels[svcLabelKey] = labelValue

					newService, err := r.createOrUpdateService(ctx, vmService)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Labels).To(HaveLen(1))
					Expect(newService.Labels).To(HaveKeyWithValue(svcLabelKey, labelValue))

					// Channel should receive an Update event
					Expect(events).Should(Receive(ContainSubstring(OpUpdate)))
				})
			})
		})
		Describe("When Service has new annotations", func() {
			var (
				serviceName          string
				service              *corev1.Service
				serviceAnnotationKey string
				serviceAnnotationVal string
				vmService            *vmoperatorv1alpha1.VirtualMachineService
			)

			BeforeEach(func() {
				serviceName = "dummy-service"
				service = getService(serviceName, serviceNamespace)

				annotations := service.ObjectMeta.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				serviceAnnotationKey = "dummy-service-annotation"
				serviceAnnotationVal = "blah"

				annotations[serviceAnnotationKey] = serviceAnnotationVal
				service.ObjectMeta.SetAnnotations(annotations)

				err := c.Create(ctx, service)
				Expect(err).NotTo(HaveOccurred())

				vmService = getVmService(serviceName, serviceNamespace)
				err = c.Create(ctx, vmService)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, service)
				Expect(err).NotTo(HaveOccurred())

				err = c.Delete(ctx, vmService)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("When there are annotations added to the Service", func() {
				It("Should preserve existing annotations on the Service and shouldn't update the k8s Service", func() {
					newService, err := r.createOrUpdateService(ctx, vmService)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(newService.Annotations).To(HaveKeyWithValue(serviceAnnotationKey, serviceAnnotationVal))

					// Channel should receive an Update event
					Expect(events).ShouldNot(Receive(ContainSubstring(OpUpdate)))
				})
			})
		})

		Describe("UpdateVmStatus", func() {
			It("Should add VirtualMachineService Status and Annotations if they dont exist", func() {
				Expect(c.Create(ctx, vmService)).To(Succeed())
				Expect(r.updateVmService(ctx, vmService, service)).To(Succeed())

				// Eventually, the VM service should be updated with the LB Ingress IPs and annotations.
				Eventually(func() bool {
					vmSvc := &vmoperatorv1alpha1.VirtualMachineService{}
					vmSvcKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}
					Expect(c.Get(ctx, vmSvcKey, vmSvc)).To(Succeed())

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
				}, timeout).Should(BeTrue())

			})
		})

		Describe("When Updating endpoints", func() {
			var (
				serviceName string
				service     *corev1.Service
				vmService   *vmoperatorv1alpha1.VirtualMachineService
			)

			BeforeEach(func() {
				serviceName = "dummy-endpoints-service"
				service = getService(serviceName, serviceNamespace)
				vmService = getVmService(serviceName, serviceNamespace)

				err := c.Create(ctx, service)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, service)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should update endpoints when it is not the same with existing endpoints", func() {
				// We need the VMService becasue the endpoints use the its UID as a OwnerReference
				Expect(c.Create(ctx, vmService)).To(Succeed())

				currentEndpoints := &corev1.Endpoints{}

				// Dummy Service with updated fields.
				port := getServicePort("foo", "TCP", 44, 8080, 30007)
				changedService := &corev1.Service{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Service",
						APIVersion: "core/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: serviceNamespace,
						Name:      serviceName,
						Labels:    map[string]string{"foo": "bar"},
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{port},
					},
				}

				err = r.updateEndpoints(ctx, vmService, changedService)
				Expect(err).ShouldNot(HaveOccurred())

				// Wait for the Endpoint to be created.
				newEndpoints := &corev1.Endpoints{}
				Eventually(func() error {
					err := c.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: serviceNamespace}, newEndpoints)
					return err
				}).Should(Succeed())

				Expect(currentEndpoints).NotTo(Equal(newEndpoints))
			})

			It("should not update endpoints when it is the same with existing endpoints", func() {
				endpoints := &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: serviceNamespace,
						Name:      serviceName,
					},
					Subsets: nil,
				}
				err = c.Create(ctx, endpoints)
				Expect(err).ShouldNot(HaveOccurred())

				currentEndpoints := &corev1.Endpoints{}
				err = c.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, currentEndpoints)
				Expect(err).NotTo(HaveOccurred())

				err = r.updateEndpoints(ctx, vmService, service)
				Expect(err).ShouldNot(HaveOccurred())

				newEndpoints := &corev1.Endpoints{}
				Eventually(func() error {
					return c.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, newEndpoints)
				}).Should(Succeed())

				Expect(currentEndpoints).To(Equal(newEndpoints))
			})
		})
	})
})

var _ = Describe("SetLBProvider", func() {
	var origIsT1PerNamespaceEnabled func() bool
	var origLBProvider string
	BeforeEach(func() {
		origIsT1PerNamespaceEnabled = lib.IsT1PerNamespaceEnabled
		origLBProvider = providers.LBProvider
		providers.LBProvider = ""
	})
	AfterEach(func() {
		lib.IsT1PerNamespaceEnabled = origIsT1PerNamespaceEnabled
		providers.LBProvider = origLBProvider
	})
	It("Should create No-Op load-balancer when WCP_T1_PERNAMESPACE is true", func() {
		lib.IsT1PerNamespaceEnabled = func() bool {
			return true
		}
		providers.SetLBProvider()
		Expect(providers.LBProvider).To(Equal(providers.NoOpLoadBalancer))
	})
	It("Should create NSX-T load-balancer when WCP_T1_PERNAMESPACE is false", func() {
		lib.IsT1PerNamespaceEnabled = func() bool {
			return false
		}
		providers.SetLBProvider()
		Expect(providers.LBProvider).To(Equal(providers.NSXTLoadBalancer))
	})
})

func getTestVirtualMachine(namespace, name string) vmoperatorv1alpha1.VirtualMachine {
	return getTestVirtualMachineWithLabels(namespace, name, map[string]string{"foo": "bar"})
}

func getTestVirtualMachineWithLabels(namespace, name string, labels map[string]string) vmoperatorv1alpha1.VirtualMachine {
	return vmoperatorv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSpec{
			PowerState: "poweredOn",
			ImageName:  "test",
			ClassName:  "TEST",
		},
	}
}

func getTestVirtualMachineWithProbe(namespace, name string, labels map[string]string, port int) vmoperatorv1alpha1.VirtualMachine {
	vm := getTestVirtualMachineWithLabels(namespace, name, labels)
	vm.Spec.Ports = []vmoperatorv1alpha1.VirtualMachinePort{
		{
			Protocol: corev1.ProtocolTCP,
			Port:     port,
			Name:     "my-service",
		},
	}
	vm.Spec.ReadinessProbe = &vmoperatorv1alpha1.Probe{
		TCPSocket: &vmoperatorv1alpha1.TCPSocketAction{
			Port: intstr.FromInt(port),
		},
	}
	return vm
}

func getTestVMService(namespace, name string) vmoperatorv1alpha1.VirtualMachineService {
	return getTestVMServiceWithSelector(namespace, name, map[string]string{"foo": "bar"})
}

func getTestVMServiceWithSelector(namespace, name string, selector map[string]string) vmoperatorv1alpha1.VirtualMachineService {
	return vmoperatorv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
			Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeClusterIP,
			Selector: selector,
			Ports: []vmoperatorv1alpha1.VirtualMachineServicePort{
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

func setupTestServer() (*httptest.Server, string, int) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	host, port, err := net.SplitHostPort(s.Listener.Addr().String())
	Expect(err).NotTo(HaveOccurred())
	portInt, err := strconv.Atoi(port)
	Expect(err).NotTo(HaveOccurred())
	return s, host, portInt
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

func getVmServicePort(name, protocol string, port, targetPort int32) vmoperatorv1alpha1.VirtualMachineServicePort {
	return vmoperatorv1alpha1.VirtualMachineServicePort{
		Name:       name,
		Protocol:   protocol,
		Port:       port,
		TargetPort: targetPort,
	}
}

func getFakeLastAppliedConfiguration(vmService *vmoperatorv1alpha1.VirtualMachineService) string {
	service, err := json.Marshal(vmService)
	if err != nil {
		return ""
	}

	return string(service)
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
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{port},
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

func getVmService(name, namespace string) *vmoperatorv1alpha1.VirtualMachineService {
	// Get VirtualMachineServicePort with dummy values.
	vmServicePort := getVmServicePort("foo", "TCP", 42, 42)

	return &vmoperatorv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
			Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
			Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{vmServicePort},
			Selector: map[string]string{"foo": "bar"},
		},
	}

}

func getVmServiceOfType(name, namespace string, serviceType vmoperatorv1alpha1.VirtualMachineServiceType) *vmoperatorv1alpha1.VirtualMachineService {
	// Get VirtualMachineServicePort with dummy values.
	vmServicePort := getVmServicePort("foo", "TCP", 42, 42)

	return &vmoperatorv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
			Type:     serviceType,
			Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{vmServicePort},
			Selector: map[string]string{"foo": "bar"},
		},
	}
}

func newFakeRecorder() (record.Recorder, chan string) {
	fakeEventRecorder := clientgorecord.NewFakeRecorder(1024)
	recorder := record.New(fakeEventRecorder)
	return recorder, fakeEventRecorder.Events
}
