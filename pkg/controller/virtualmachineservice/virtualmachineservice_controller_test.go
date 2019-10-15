// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"fmt"
	"sync"
	"time"

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
})
