// +build integration

/* **********************************************************
 * Copyright 2019-2020 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachinesetresourcepolicy

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	controllercontext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const timeout = time.Second * 30

var _ = Describe("VirtualMachineSetResourcePolicy controller", func() {
	ns := integration.DefaultNamespace

	var (
		stopMgr    chan struct{}
		mgrStopped *sync.WaitGroup
		mgr        manager.Manager
		err        error
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("when creating/deleting a VirtualMachineSetResourcePolicy", func() {
		var resourcePolicyNameSuffix = 0
		var (
			resourcePolicyName     string
			resourcePolicyInstance vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
			expectedRequest        reconcile.Request
		)

		BeforeEach(func() {
			resourcePolicyName = "resourcepolicy-name" + strconv.Itoa(resourcePolicyNameSuffix)
			resourcePolicyNameSuffix++
			resourcePolicyInstance = getResourcePolicyInstance(resourcePolicyName, ns)
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: resourcePolicyName, Namespace: ns}}
		})

		Context("a valid spec is used", func() {
			It("should create the VirtualMachineSetResource", func() {
				ctrlContext := &controllercontext.ControllerManagerContext{
					VmProvider: vmProvider,
				}
				recFn, requests, _, _ := integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
				Expect(add(mgr, recFn)).To(Succeed())

				// Create the VirtualMachineSetResourcePolicy object and expect the Reconcile
				err = c.Create(context.TODO(), &resourcePolicyInstance)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

				// Delete the VirtualMachineSetResourcePolicy object and expect the Reconcile
				err = c.Delete(context.TODO(), &resourcePolicyInstance)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
			})
		})

		Context("deleting a VirtualMachineSetResourcePolicy", func() {
			var (
				vmInstance     vmoperatorv1alpha1.VirtualMachine
				requests       chan reconcile.Request
				recFn          reconcile.Reconciler
				reconcileError chan error
			)

			BeforeEach(func() {
				ctrlContext := &controllercontext.ControllerManagerContext{
					VmProvider: vmProvider,
				}
				recFn, requests, _, reconcileError = integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
				Expect(add(mgr, recFn)).To(Succeed())

				vmInstance = getVirtualMachineInstance("virtualmachine-name", ns)
				vmInstance.Spec.ResourcePolicyName = resourcePolicyName
			})

			Context("when a VM is referencing it", func() {
				It("should fail to delete", func() {
					err = c.Create(context.TODO(), &resourcePolicyInstance)
					Expect(err).NotTo(HaveOccurred())
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					err = c.Create(context.TODO(), &vmInstance)
					Expect(err).NotTo(HaveOccurred())
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					err = c.Delete(context.TODO(), &resourcePolicyInstance)
					Expect(err).ShouldNot(HaveOccurred())
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					expectedError := fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'",
						vmInstance.NamespacedName(), resourcePolicyInstance.NamespacedName())
					Eventually(reconcileError, timeout).Should(Receive(Equal(expectedError)))
				})
			})

			Context("when no VMs are referencing it", func() {
				It("should successfully delete", func() {

					resourcePolicyInstance.Name = "resource-policy-name-1"
					err = c.Create(context.TODO(), &resourcePolicyInstance)
					Expect(err).NotTo(HaveOccurred())
					expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: resourcePolicyInstance.Name, Namespace: ns}}
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					// Create a different VM since the cache might not have synced yet
					vmInstance.Name = "virtualmachine-name-1"
					err = c.Create(context.TODO(), &vmInstance)
					Expect(err).NotTo(HaveOccurred())

					err = c.Delete(context.TODO(), &vmInstance)
					Expect(err).NotTo(HaveOccurred())

					err = c.Delete(context.TODO(), &resourcePolicyInstance)
					Expect(err).NotTo(HaveOccurred())
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
				})
			})
		})
	})
})

func getResourcePolicyInstance(name, namespace string) vmoperatorv1alpha1.VirtualMachineSetResourcePolicy {
	return vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmoperatorv1alpha1.ResourcePoolSpec{
				Name: "name-resourcepool",
				Reservations: vmoperatorv1alpha1.VirtualMachineResourceSpec{
					Cpu:    resource.MustParse("1000Mi"),
					Memory: resource.MustParse("100Mi"),
				},
				Limits: vmoperatorv1alpha1.VirtualMachineResourceSpec{
					Cpu:    resource.MustParse("2000Mi"),
					Memory: resource.MustParse("200Mi"),
				},
			},
			Folder: vmoperatorv1alpha1.FolderSpec{
				Name: "name-folder",
			},
			ClusterModules: []vmoperatorv1alpha1.ClusterModuleSpec{
				{GroupName: "ControlPlane"},
				{GroupName: "NodeGroup1"},
			},
		},
	}
}

func getVirtualMachineInstance(name, namespace string) vmoperatorv1alpha1.VirtualMachine {
	return vmoperatorv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSpec{
			ImageName:  "DC0_H0_VM0", // Default govcsim image name
			ClassName:  "xsmall",
			PowerState: "poweredOn",
			Ports:      []vmoperatorv1alpha1.VirtualMachinePort{},
			// StorageClass: storageClass,
		},
	}
}
