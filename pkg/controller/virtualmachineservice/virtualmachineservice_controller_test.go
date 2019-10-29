// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"fmt"
	"sync"
	"time"

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

const timeout = time.Second * 5

var _ = Describe("VirtualMachineService controller", func() {
	ns := integration.DefaultNamespace
	// here "name" must be confine the a DNS-1035 style which must consist of lower case alphanumeric characters or '-',
	// regex used for validation is '[a-z]([-a-z0-9]*[a-z0-9])?')
	name := "foo-vm"

	var (
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
		stopMgr, mgrStopped = StartTestManager(mgr)
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

			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
			recFn, requests := SetupTestReconcile(newReconciler(mgr))
			Expect(add(mgr, recFn)).To(Succeed())

			fakeRecorder := vmrecord.GetRecorder().(*record.FakeRecorder)

			// Create the VM Service object and expect the Reconcile
			err := c.Create(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Service should have been created with same name
			serviceKey := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			service := corev1.Service{}
			err = c.Get(context.TODO(), serviceKey, &service)
			Expect(err).ShouldNot(HaveOccurred())

			// Delete the VM Service object and expect it to be deleted.
			err = c.Delete(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			Eventually(func() bool {
				vmService := vmoperatorv1alpha1.VirtualMachineService{}
				err = c.Get(context.TODO(), serviceKey, &vmService)
				return errors.IsNotFound(err)
			}).Should(BeTrue())

			// Expect the Service to be deleted too but not yet. In this testenv framework, the kube-controller is
			// not running so this won't be garbage collected.
			//err = c.Get(context.TODO(), serviceKey, &service)
			//Expect(errors.IsNotFound(err)).Should(BeTrue())

			reasonMap := vmrecord.ReadEvents(fakeRecorder)
			Expect(len(reasonMap)).Should(Equal(2))
			Expect(reasonMap[vmrecord.Success+OpCreate]).Should(Equal(1))
			Expect(reasonMap[vmrecord.Success+OpDelete]).Should(Equal(1))
		})
	})

	Describe("When listing virtual machines", func() {
		BeforeEach(func() {
			virtualMachine1 := vmoperatorv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-1",
					Namespace: ns,
					Labels:    map[string]string{"foo": "bar"},
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSpec{
					PowerState: "poweredOn",
					ImageName:  "test",
					ClassName:  "TEST",
				},
			}
			virtualMachine2 := vmoperatorv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-2",
					Namespace: ns,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSpec{
					PowerState: "poweredOn",
					ImageName:  "test",
					ClassName:  "TEST",
				},
			}
			err := c.Create(context.TODO(), &virtualMachine1)
			Expect(err).ShouldNot(HaveOccurred())
			err = c.Create(context.TODO(), &virtualMachine2)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should use vmservice selector instead of service selector", func() {
			vmService := &vmoperatorv1alpha1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "dummy-vm-service",
				},
				Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
					Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
					Selector: map[string]string{"foo": "bar"},
				},
			}
			vmList, err := r.getVMServiceSelectedVirtualMachines(context.TODO(), vmService)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(vmList.Items)).To(Equal(1))
			Expect(vmList.Items[0].Name).To(Equal("dummy-vm-1"))
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
				err = c.Get(context.TODO(), types.NamespacedName{service.Namespace, service.Name}, currentService)
				Expect(err).ShouldNot(HaveOccurred())

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
				err = c.Get(context.TODO(), types.NamespacedName{vmService.Namespace, vmService.Name}, currentVMService)
				Expect(err).ShouldNot(HaveOccurred())

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
				err = c.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, currentEndpoints)
				Expect(err).ShouldNot(HaveOccurred())

				err = r.updateEndpoints(context.TODO(), vmService, service)
				Expect(err).ShouldNot(HaveOccurred())

				newEndpoints := &corev1.Endpoints{}
				err = c.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, newEndpoints)
				Expect(err).ShouldNot(HaveOccurred())

				Expect(currentEndpoints).To(Equal(newEndpoints))
			})
		})
	})
})
