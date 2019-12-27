// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	vmrecord "github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const timeout = time.Second * 30

var _ = Describe("VirtualMachineService controller", func() {
	ns := integration.DefaultNamespace
	// here "name" must be confine the a DNS-1035 style which must consist of lower case alphanumeric characters or '-',
	// regex used for validation is '[a-z]([-a-z0-9]*[a-z0-9])?')
	name := "foo-vm"

	var (
		recFn                   reconcile.Reconciler
		requests                chan reconcile.Request
		reconcileErr            chan error
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
		r                       ReconcileVirtualMachineService
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		leaderElectionConfigMap = fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod,
			LeaderElection:          true,
			LeaderElectionID:        leaderElectionConfigMap,
			LeaderElectionNamespace: ns})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()
		r = ReconcileVirtualMachineService{mgr.GetClient(), mgr.GetScheme(), nil}
		// Setup the reconciler for all the tests
		recFn, requests, reconcileErr = integration.SetupTestReconcile(newReconciler(mgr))
		Expect(add(mgr, recFn)).To(Succeed())
		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      leaderElectionConfigMap,
			},
		}

		err := c.Delete(context.Background(), configMap)
		Expect(err).NotTo(HaveOccurred())
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

			fakeRecorder := vmrecord.GetRecorder().(*record.FakeRecorder)

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
			//err = c.Get(context.TODO(), serviceKey, &service)
			//Expect(errors.IsNotFound(err)).Should(BeTrue())

			Eventually(func() bool {
				reasonMap := vmrecord.ReadEvents(fakeRecorder)
				if (len(reasonMap) != 2) || (reasonMap[vmrecord.Success+OpCreate] != 1) ||
					(reasonMap[vmrecord.Success+OpDelete] != 1) {
					GinkgoWriter.Write([]byte(fmt.Sprintf("reasonMap =  %v", reasonMap)))
					return false
				}
				return true
			}, timeout).Should(BeTrue())
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
		It("Should create service and endpoints with subsets", func() {
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

	Describe("When vmservice selector matches virtual machines", func() {
		var (
			vm1       vmoperatorv1alpha1.VirtualMachine
			vm2       vmoperatorv1alpha1.VirtualMachine
			vmService vmoperatorv1alpha1.VirtualMachineService
		)
		BeforeEach(func() {
			label := map[string]string{"vm-match-selector": "true"}
			vm1 = getTestVirtualMachineWithLabels(ns, "dummy-vm-match-selector-1", label)
			vm2 = getTestVirtualMachine(ns, "dummy-vm-match-selector-2")
			vmService = getTestVMServiceWithSelector(ns, "dummy-vm-service-match-selector", label)
			createObjects(context.TODO(), c, []runtime.Object{&vm1, &vm2, &vmService})

		})
		It("Should use vmservice selector instead of service selector", func() {
			vmList, err := r.getVMServiceSelectedVirtualMachines(context.TODO(), &vmService)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(vmList.Items)).To(Equal(1))
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

				newService, err := r.createOrUpdateService(context.TODO(), changedVMService, service, "test-lb")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newService).NotTo(Equal(currentService))
			})

			It("should not update service when it is the same service", func() {
				currentService := &corev1.Service{}
				err = c.Get(context.TODO(), types.NamespacedName{service.Namespace, service.Name}, currentService)
				Expect(err).ShouldNot(HaveOccurred())

				newService, err := r.createOrUpdateService(context.TODO(), vmService, currentService, "test-lb")
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

				newService, err := r.createOrUpdateService(context.TODO(), changedVMService, service, "test-lb")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newService).NotTo(Equal(currentService))
			})
		})

		Describe("when update vm service", func() {
			It("should update vm service when it is not the same with exist vm service", func() {
				err := c.Create(context.TODO(), vmService)
				Expect(err).ShouldNot(HaveOccurred())

				currentVMService := &vmoperatorv1alpha1.VirtualMachineService{}
				Eventually(func() error {
					return c.Get(context.TODO(), types.NamespacedName{vmService.Namespace, vmService.Name}, currentVMService)
				}).Should(Succeed())

				changedVMService := &vmoperatorv1alpha1.VirtualMachineService{
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

				newVMService, err := r.updateVmServiceStatus(context.TODO(), changedVMService, service)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newVMService).NotTo(Equal(currentVMService))
			})

			It("should not update vm service when it is the same with exist vm service", func() {
				currentVMService := &vmoperatorv1alpha1.VirtualMachineService{}
				err = c.Get(context.TODO(), types.NamespacedName{vmService.Namespace, vmService.Name}, currentVMService)
				Expect(err).ShouldNot(HaveOccurred())

				newVMService, err := r.updateVmServiceStatus(context.TODO(), currentVMService, service)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(newVMService).To(Equal(currentVMService))
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
