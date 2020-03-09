// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineclass

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const timeout = time.Second * 5

var _ = Describe("VirtualMachineClass controller", func() {
	name := "foo-vm"
	ns := integration.DefaultNamespace

	var (
		instance                vmoperatorv1alpha1.VirtualMachineClass
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
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

	Describe("when creating/deleting a VM Class", func() {
		It("invoke the reconcile method", func() {
			instance = vmoperatorv1alpha1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineClassSpec{
					Hardware: vmoperatorv1alpha1.VirtualMachineClassHardware{
						Cpus:   4,
						Memory: resource.MustParse("1Mi"),
					},
					Policies: vmoperatorv1alpha1.VirtualMachineClassPolicies{
						Resources: vmoperatorv1alpha1.VirtualMachineClassResources{
							Requests: vmoperatorv1alpha1.VirtualMachineResourceSpec{
								Cpu:    resource.MustParse("1000Mi"),
								Memory: resource.MustParse("100Mi"),
							},
							Limits: vmoperatorv1alpha1.VirtualMachineResourceSpec{
								Cpu:    resource.MustParse("2000Mi"),
								Memory: resource.MustParse("200Mi"),
							},
						},
					},
				},
			}

			ctrlContext := &controllerContext.ControllerManagerContext{
				VmProvider: vmprovider.GetService().GetRegisteredVmProviderOrDie(),
			}

			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}
			recFn, requests, _, _ := integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
			Expect(add(mgr, recFn)).To(Succeed())
			// Create the VM Class object and expect the Reconcile
			err = c.Create(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
			// Delete the VM Class object and expect the Reconcile
			err = c.Delete(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})
})
