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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	// "k8s.io/client-go/tools/record"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg"
	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/integration"
	// vmrecord "github.com/vmware-tanzu/vm-operator/pkg/record"
)

var c client.Client

const timeout = time.Second * 30

var _ = Describe("VirtualMachineService controller", func() {
	ns := integration.DefaultNamespace
	// here "name" must be confine the a DNS-1035 style which must consist of lower case alphanumeric characters or '-',
	// regex used for validation is '[a-z]([-a-z0-9]*[a-z0-9])?')
	name := "foo-vm"

	var (
		recFn           reconcile.Reconciler
		requests        chan reconcile.Request
		reconcileResult chan reconcile.Result
		reconcileErr    chan error
		stopMgr         chan struct{}
		mgrStopped      *sync.WaitGroup
		mgr             manager.Manager
		err             error
		r               *ReconcileVirtualMachineService
	)

	BeforeEach(func() {
		ctrlContext := &controllerContext.ControllerManagerContext{
			Logger: ctrllog.Log.WithName("test"),
		}
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()
		r, err = newReconciler(mgr)
		Expect(err).NotTo(HaveOccurred())
		recFn, requests, reconcileResult, reconcileErr = integration.SetupTestReconcile(r)
		Expect(add(ctrlContext, mgr, recFn, r)).To(Succeed())
		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("when creating/deleting a VM Service", func() {
		It("invoke the reconcile method", func() {
			port := vmoperatorv1alpha1.VirtualMachineServicePort{
				Name:       "foo",
				Protocol:   "TCP",
				Port:       42,
				TargetPort: 42,
			}

			instance := vmoperatorv1alpha1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      name,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
					Type:     "ClusterIP",
					Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{port},
					Selector: map[string]string{"foo": "bar"},
				},
			}

			// fakeRecorder := vmrecord.GetRecorder().(*record.FakeRecorder)

			// Create the VM Service object and expect the Reconcile
			err := c.Create(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Service should have been created with same name
			serviceKey := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			service := corev1.Service{}
			Eventually(func() error {
				return c.Get(context.TODO(), serviceKey, &service)
			}).Should(Succeed())

			// Delete the VM Service object and expect it to be deleted.
			err = c.Delete(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			Eventually(func() bool {
				vmService := vmoperatorv1alpha1.VirtualMachineService{}
				err = c.Get(context.TODO(), serviceKey, &vmService)
				if errors.IsNotFound(err) {
					return true
				}
				if err != nil {
					GinkgoWriter.Write([]byte(fmt.Sprintf("Unexpected error: %#v\n", err)))
				}
				return false
			}, 3*timeout).Should(BeTrue())

			// Expect the Service to be deleted too but not yet. In this testenv framework, the kube-controller is
			// not running so this won't be garbage collected.
			// err = c.Get(context.TODO(), serviceKey, &service)
			// Expect(errors.IsNotFound(err)).Should(BeTrue())

			/*
				Eventually(func() bool {
					reasonMap := vmrecord.ReadEvents(fakeRecorder)
					if (len(reasonMap) != 2) || (reasonMap[vmrecord.Success+OpCreate] != 1) ||
						(reasonMap[vmrecord.Success+OpDelete] != 1) {
						GinkgoWriter.Write([]byte(fmt.Sprintf("reasonMap =  %v", reasonMap)))
						return false
					}
					return true
				}, timeout).Should(BeTrue())
			*/
		})
	})

	Describe("When vmservice selector matches virtual machines with no IP", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			vm1 = getTestVirtualMachine(ns, "dummy-vm-with-no-ip-1")
			vm2 = getTestVirtualMachine(ns, "dummy-vm-with-no-ip-2")
			vmService = getTestVMService(ns, "dummy-vm-service-no-ip")
			createObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})
			// Wait for the reconciliation to happen before asserting
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should create service and endpoints with no subsets", func() {
			assertServiceWithNoEndpointSubsets(context.TODO(), c, vmService.Namespace, vmService.Name)
		})
		AfterEach(func() {
			deleteObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})
		})
	})

	Describe("When vmservice selector matches virtual machines with no probe", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			label := map[string]string{"no-probe": "true"}
			vm1 = getTestVirtualMachineWithLabels(ns, "dummy-vm-with-no-probe-1", label)
			vm2 = getTestVirtualMachineWithLabels(ns, "dummy-vm-with-no-probe-2", label)
			vmService = getTestVMServiceWithSelector(ns, "dummy-vm-service-no-probe", label)
			createObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})
			// Since Status is a sub-resource, we need to update the status separately.
			vm1.Status.VmIp = "192.168.1.100"
			vm1.Status.Host = "10.0.0.100"
			vm2.Status.VmIp = "192.168.1.200"
			vm2.Status.Host = "10.0.0.200"
			updateObjectsStatus(context.TODO(), c, []runtime.Object{&vm1, &vm2})
			// Wait for the reconciliation to happen before asserting
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should create service and endpoints with subsets", func() {
			subsets := assertServiceWithEndpointSubsets(context.TODO(), c, vmService.Namespace, vmService.Name)
			if subsets[0].Addresses[0].IP == vm1.Status.VmIp {
				Expect(subsets[0].Addresses[1].IP).To(Equal(vm2.Status.VmIp))
			} else {
				Expect(subsets[0].Addresses[0].IP).To(Equal(vm1.Status.VmIp))
			}
		})
		AfterEach(func() {
			deleteObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})
		})
	})

	Describe("When vmservice selector matches virtual machines with probe", func() {
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
			successVM = getTestVirtualMachineWithProbe(ns, "dummy-vm-with-success-probe", label, testPort)
			failedVM = getTestVirtualMachineWithProbe(ns, "dummy-vm-with-failure-probe", label, 10001)
			vmService = getTestVMServiceWithSelector(ns, "dummy-vm-service-with-probe", label)
			createObjects(context.TODO(), c, []runtime.Object{&successVM, &failedVM, &vmService})
			// Since Status is a sub-resource, we need to update the status separately.
			successVM.Status.VmIp = testHost
			successVM.Status.Host = "10.0.0.100"
			failedVM.Status.VmIp = "192.168.1.200"
			failedVM.Status.Host = "10.0.0.200"
			updateObjectsStatus(context.TODO(), c, []runtime.Object{&successVM, &failedVM})

			// Wait for the reconciliation to happen before asserting
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should create service and endpoints with VM's that pass the probe", func() {
			// Note: Ideally we would expect the successVM with local IP 127.0.0.1 to be part of the endpoint. However,
			// kubernetes doesn't allow endpoints to have an IP address in the loopback range. As a result,
			// we end up with an error trying to create the endpoints for the VMService. The fact that we tried to add
			// an endpoint with the loopback address from the test server asserts that the endpoint passed the
			// readiness probe.
			// subsets := assertServiceWithEndpointSubsets(context.TODO(), c, vmService.Namespace, vmService.Name)
			// Expect(subsets[0].Addresses[0].IP).To(Equal(successVM.Status.VmIp))
			Eventually(reconcileErr, timeout).Should(Receive(
				MatchError(fmt.Sprintf("Endpoints \"dummy-vm-service-with-probe\" is invalid: subsets[0].addresses[0].ip: "+
					"Invalid value: \"%s\": may not be in the loopback range (127.0.0.0/8)", testHost))))

		})
		AfterEach(func() {
			testServer.Close()
			deleteObjects(context.TODO(), c, []runtime.Object{&successVM, &failedVM, &vmService})
		})
	})

	Describe("When vmservice selector matches VM's and all of them fail probe", func() {
		var (
			failedVM  vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			label := map[string]string{"with-probe-requeue": "true"}
			failedVM = getTestVirtualMachineWithProbe(ns, "dummy-vm-with-failure-probe-requeue", label, 10002)
			vmService = getTestVMServiceWithSelector(ns, "dummy-vm-service-with-probe-requeue", label)
			createObjects(context.TODO(), c, []runtime.Object{&failedVM, &vmService})
			// Since Status is a sub-resource, we need to update the status separately.
			failedVM.Status.VmIp = "192.168.1.300"
			failedVM.Status.Host = "10.0.0.300"
			updateObjectsStatus(context.TODO(), c, []runtime.Object{&failedVM})

			// Wait for the reconciliation to happen before asserting
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmService.Name}}
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
		It("Should requeue", func() {
			expectedResult := reconcile.Result{RequeueAfter: probeFailureRequeueTime}
			Eventually(reconcileResult, timeout).Should(Receive(Equal(expectedResult)))
		})
		AfterEach(func() {
			deleteObjects(context.TODO(), c, []runtime.Object{&failedVM, &vmService})
		})
	})

	Describe("When vmservice selector matches virtual machines", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			labels := map[string]string{"vm-match-selector": "true"}
			vm1 = getTestVirtualMachineWithLabels(ns, "dummy-vm-match-selector-1", labels)
			vm2 = getTestVirtualMachine(ns, "dummy-vm-match-selector-2")
			vmService = getTestVMServiceWithSelector(ns, "dummy-vm-service-match-selector", labels)
			createObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})
		})
		It("Should use vmservice selector instead of service selector", func() {
			vmList := &vmoperatorv1alpha1.VirtualMachineList{}
			var err error
			Eventually(func() int {
				vmList, err = r.getVirtualMachinesSelectedByVmService(context.TODO(), &vmService)
				Expect(err).ShouldNot(HaveOccurred())
				return len(vmList.Items)
			}).Should(BeNumerically("==", 1))
			Expect(vmList.Items[0].Name).To(Equal(vm1.Name))
		})
		AfterEach(func() {
			deleteObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})
		})
	})

	Describe("when update k8s objects", func() {
		var (
			port = corev1.ServicePort{
				Name:     "foo",
				Protocol: "TCP",
				Port:     42,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 8080,
					StrVal: "",
				},
			}

			vmPort = vmoperatorv1alpha1.VirtualMachineServicePort{
				Name:       "foo",
				Protocol:   "TCP",
				Port:       42,
				TargetPort: 42,
			}

			service = &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "core/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   ns,
					Name:        "dummy-service",
					Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: `{"apiVersion":"vmoperator.vmware.com/v1alpha1","kind":"VirtualMachineService","metadata":{"annotations":{},"name":"dummy-service","namespace":"default"},"spec":{"ports":[{"name":"foo","port":42,"protocol":"TCP","targetPort":42}],"selector":{"foo":"bar"},"type":"LoadBalancer"}}`},
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
			vmService = &vmoperatorv1alpha1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "dummy-service",
				},
				Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
					Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
					Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{vmPort},
					Selector: map[string]string{"foo": "bar"},
				},
			}
		)

		Describe("when update service", func() {
			It("should update service when it is not the same service", func() {
				err := c.Create(context.TODO(), service)
				Expect(err).ShouldNot(HaveOccurred())
				currentService := &corev1.Service{}
				Eventually(func() error {
					return c.Get(context.TODO(), types.NamespacedName{service.Namespace, service.Name}, currentService)
				}).Should(Succeed())

				changedVMService := &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "dummy-service",
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type: vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
						Ports: []vmoperatorv1alpha1.VirtualMachineServicePort{
							{
								Name:       "test",
								Protocol:   "TCP",
								Port:       80,
								TargetPort: 80,
							},
						},
						Selector: map[string]string{"foo": "bar"},
					},
				}

				newService, err := r.createOrUpdateService(context.TODO(), changedVMService, service)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newService).NotTo(Equal(currentService))
			})

			It("should not update service when it is the same service", func() {
				currentService := &corev1.Service{}
				err = c.Get(context.TODO(), types.NamespacedName{service.Namespace, service.Name}, currentService)
				Expect(err).ShouldNot(HaveOccurred())

				newService, err := r.createOrUpdateService(context.TODO(), vmService, currentService)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newService).To(Equal(currentService))
			})

			It("should update service for invalid format service annotation", func() {
				currentService := &corev1.Service{}
				err = c.Get(context.TODO(), types.NamespacedName{service.Namespace, service.Name}, currentService)
				Expect(err).ShouldNot(HaveOccurred())

				changedVMService := &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "dummy-service-invalid-format",
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type: vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
						Ports: []vmoperatorv1alpha1.VirtualMachineServicePort{
							{
								Name:       "test",
								Protocol:   "TCP",
								Port:       80,
								TargetPort: 80,
							},
						},
						Selector: map[string]string{"foo": "bar"},
					},
				}
				currentService.Annotations[corev1.LastAppliedConfigAnnotation] = `{"d"}`

				newService, err := r.createOrUpdateService(context.TODO(), changedVMService, service)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newService).NotTo(Equal(currentService))
			})

		})
		Describe("when handling labels while reconciling a VirtualMachineService", func() {
			var (
				virtualMachineServiceToReconcile *vmoperatorv1alpha1.VirtualMachineService
				serviceReconciledFromVMService   *corev1.Service
				port                             vmoperatorv1alpha1.VirtualMachineServicePort
				// Labels that exist on the VMService.
				vmServiceLabels map[string]string
				// Labels that are expected on the Service.
				expectedServiceLabels map[string]string
				testVMServiceName     string
				testVMServicePort     int32
			)
			JustBeforeEach(func() {
				port = vmoperatorv1alpha1.VirtualMachineServicePort{
					Name:       "foo",
					Protocol:   "TCP",
					Port:       testVMServicePort,
					TargetPort: testVMServicePort,
				}

				virtualMachineServiceToReconcile = &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      testVMServiceName,
						Labels:    vmServiceLabels,
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:     "ClusterIP",
						Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{port},
						Selector: map[string]string{"foo": "bar"},
					},
				}

				Expect(c.Create(context.TODO(), virtualMachineServiceToReconcile)).To(Succeed())

				// Make sure the reconcile happened. The 'requests' channel is sent to after a reconcile
				//  is completed.
				Eventually(requests, timeout).Should(Receive(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      virtualMachineServiceToReconcile.Name,
						Namespace: virtualMachineServiceToReconcile.Namespace}})))

				// Each test gets a handle to the underlying service, so set it up.
				Eventually(func() error {
					serviceReconciledFromVMService = &corev1.Service{}
					return c.Get(context.TODO(), types.NamespacedName{
						Name:      virtualMachineServiceToReconcile.Name,
						Namespace: virtualMachineServiceToReconcile.Namespace}, serviceReconciledFromVMService)
				}, 30*time.Second, 1*time.Second).Should(Succeed(), "Expected service to be created from VirtualMachineService")
			})
			JustAfterEach(func() {
				// Make the assertions about the labels on the service.
				Eventually(requests, timeout).Should(Receive(Equal(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      virtualMachineServiceToReconcile.Name,
						Namespace: virtualMachineServiceToReconcile.Namespace}})))

				Eventually(func() map[string]string {
					reconciledService := &corev1.Service{}
					err := c.Get(context.TODO(), types.NamespacedName{
						Name:      virtualMachineServiceToReconcile.Name,
						Namespace: virtualMachineServiceToReconcile.Namespace}, reconciledService)
					if err != nil {
						return nil
					}
					return reconciledService.Labels
				}, 30*time.Second, 2*time.Second).Should(Equal(expectedServiceLabels))
			})
			AfterEach(func() {
				// BMV: The controller is already stopped by now, so this won't actually be deleted (unless
				// we remove the finalizer now).
				Expect(c.Delete(context.TODO(), virtualMachineServiceToReconcile)).To(Succeed())
			})
			Context("when there are existing labels on the Service, but none on the VMService", func() {
				BeforeEach(func() {
					testVMServiceName = "test-vmservice-without-labels"
					testVMServicePort = 50
				})
				It("should preserve existing labels on the Service created from the VMService", func() {
					// Generate a deep copy to use for patching.
					newServiceCopy := serviceReconciledFromVMService.DeepCopy()

					// Patch the service with a fake label to simulate what NetOperator does.
					// Add some fake labels to the 'current' service. They should be preserved
					// when the createOrUpdate() method returns.
					testLabelKey := "a.run.tanzu.vmware.com.label.for.lbapi"
					testLabelValue := "a.label.value"
					serviceReconciledFromVMService.Labels = make(map[string]string)
					serviceReconciledFromVMService.Labels[testLabelKey] = testLabelValue
					serviceReconciledFromVMService.Annotations = make(map[string]string)

					// Use a patch to add the label. This is how an external controller might behave.
					Eventually(func() error {
						patch := client.MergeFrom(newServiceCopy)
						return c.Patch(context.TODO(), serviceReconciledFromVMService, patch)
					}, 30*time.Second, 2*time.Second).Should(Succeed(), "Should eventually be able to patch the Service")

					expectedServiceLabels = map[string]string{
						testLabelKey: testLabelValue,
					}

					// Wait for two reconcile cycles to validate that the label was not removed.
					// The second one is just in case the first one happened mid-patch.
					Eventually(requests, timeout).Should(Receive())
					Eventually(requests, timeout).Should(Receive())

					s := &corev1.Service{}
					err = c.Get(context.TODO(), types.NamespacedName{Name: serviceReconciledFromVMService.Name,
						Namespace: serviceReconciledFromVMService.Namespace}, s)
					Expect(err).NotTo(HaveOccurred())

					Expect(s.Labels).To(HaveKeyWithValue(testLabelKey, testLabelValue), "Label should never be removed from the Service")
				})
			})
			Context("when both the VirtualMachineService and the Service have labels", func() {
				BeforeEach(func() {
					testVMServiceName = "vm-service-with-labels"
					testVMServicePort = 51
					vmServiceLabels = map[string]string{"a.vmservice.label": "a.vmservice.label.value"}
				})
				It("should merge existing labels on the Service with those on the VMService", func() {
					// Patch the service with a fake label to simulate what NetOperator does.
					// Add some fake labels to the 'current' service. They should be preserved
					// when the createOrUpdate() method returns.
					testLabelKey := "a.run.tanzu.vmware.com.label.for.lbapi"
					testLabelValue := "a.label.value"
					// The regular client.MergeFrom() patch overwrites the existing labels, presumably because it's
					//  a merge patch. Use a JSON patch to ensure we don't overwrite the existing labels.
					Eventually(func() error {
						patchData := []byte(fmt.Sprintf(`[{"op": "add",
						"path": "/metadata/labels/%s",
						"value": "%s"}]`, testLabelKey, testLabelValue))
						patch := client.ConstantPatch(types.JSONPatchType, patchData)
						return c.Patch(context.TODO(), serviceReconciledFromVMService, patch)
					}, 30*time.Second, 2*time.Second).Should(Succeed(), "Should eventually be able to patch the Service")

					expectedServiceLabels = map[string]string{
						testLabelKey:        testLabelValue,
						"a.vmservice.label": "a.vmservice.label.value",
					}
				})
			})
			Context("when both the VirtualMachineService and the Service have conflicting labels", func() {
				BeforeEach(func() {
					testVMServiceName = "vm-service-conflict-labels"
					testVMServicePort = 52
					vmServiceLabels = map[string]string{"a.vmservice.label": "a.vmservice.label.value"}
				})
				It("should prefer the label on the VMService", func() {
					// Patch the service with a fake label to simulate what NetOperator does.
					// Add some fake labels to the 'current' service. They should be preserved
					// when the createOrUpdate() method returns.
					testLabelKey := "a.vmservice.label"
					testLabelValue := "a.label.value"
					// The regular client.MergeFrom() patch overwrites the existing labels, presumably because it's
					//  a merge patch. Use a JSON patch to ensure we don't overwrite the existing labels.
					Eventually(func() error {
						patchData := []byte(fmt.Sprintf(`[{"op": "add",
						"path": "/metadata/labels/%s",
						"value": "%s"}]`, testLabelKey, testLabelValue))
						patch := client.ConstantPatch(types.JSONPatchType, patchData)
						return c.Patch(context.TODO(), serviceReconciledFromVMService, patch)
					}, 30*time.Second, 2*time.Second).Should(Succeed(), "Should eventually be able to patch the Service")

					expectedServiceLabels = map[string]string{
						"a.vmservice.label": "a.vmservice.label.value",
					}
				})
			})
		})
		Describe("UpdateVmStatus", func() {
			It("Should add VirtualMachineService Status and Annotations if they dont exist", func() {
				Expect(c.Create(context.TODO(), vmService)).To(Succeed())
				Expect(r.updateVmService(context.TODO(), vmService, service)).To(Succeed())

				// Eventually, the VM service should be updated with the LB Ingress IPs and annotations.
				Eventually(func() bool {
					vmSvc := &vmoperatorv1alpha1.VirtualMachineService{}
					vmSvcKey := client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name}
					Expect(c.Get(context.TODO(), vmSvcKey, vmSvc)).To(Succeed())

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

		Describe("when update endpoints", func() {
			It("should update endpoints when it is not the same with exist endpoints", func() {
				currentEndpoints := &corev1.Endpoints{}
				err = c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: integration.DefaultNamespace}, currentEndpoints)
				Expect(err).ShouldNot(HaveOccurred())

				changedService := &corev1.Service{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Service",
						APIVersion: "core/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: integration.DefaultNamespace,
						Name:      name,
						Labels:    map[string]string{"foo": "bar"},
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{port},
					},
				}
				err = r.updateEndpoints(context.TODO(), vmService, changedService)
				Expect(err).ShouldNot(HaveOccurred())

				newEndpoints := &corev1.Endpoints{}
				err = c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: integration.DefaultNamespace}, currentEndpoints)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(currentEndpoints).NotTo(Equal(newEndpoints))
			})

			It("should not update endpoints when it is the same with exist endpoints", func() {
				endpoints := &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "dummy-service",
					},
					Subsets: nil,
				}
				err = c.Create(context.TODO(), endpoints)
				Expect(err).ShouldNot(HaveOccurred())

				currentEndpoints := &corev1.Endpoints{}
				Eventually(func() error {
					return c.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, currentEndpoints)
				}).Should(Succeed())

				err = r.updateEndpoints(context.TODO(), vmService, service)
				Expect(err).ShouldNot(HaveOccurred())

				newEndpoints := &corev1.Endpoints{}
				Eventually(func() error {
					return c.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, newEndpoints)
				}).Should(Succeed())

				Expect(currentEndpoints).To(Equal(newEndpoints))
			})
		})
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
