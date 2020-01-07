// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachinesetresourcepolicy

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

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const timeout = time.Second * 5

var _ = Describe("VirtualMachineSetResourcePolicy controller", func() {
	ns := integration.DefaultNamespace

	var (
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

	Describe("when creating/deleting a VirtualMachineSetResourcePolicy", func() {
		var (
			resourcePolicyName     string
			resourcePolicyInstance vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
			expectedRequest        reconcile.Request
		)

		BeforeEach(func() {
			resourcePolicyName = "resourcePolicyName"
			resourcePolicyInstance = getResourcePolicyInstance(resourcePolicyName, ns)
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: resourcePolicyName, Namespace: ns}}
		})

		Context("invalid spec is used", func() {
			It("should fail to create a VirtualMachineSetResource", func() {
				By("Update the ResourcePolicy spec to make it invalid")
				resourcePolicyInstance.Spec.ResourcePool.Reservations.Memory = resource.MustParse("2000Mi")
				resourcePolicyInstance.Spec.ResourcePool.Limits.Memory = resource.MustParse("1000Mi")

				err = c.Create(context.TODO(), &resourcePolicyInstance)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("a valid spec is used", func() {
			It("should create the VirtualMachineSetResource", func() {
				recFn, requests, _ := integration.SetupTestReconcile(newReconciler(mgr))
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
				recFn, requests, reconcileError = integration.SetupTestReconcile(newReconciler(mgr))
				Expect(add(mgr, recFn)).To(Succeed())

				vmInstance = getVirtualMachineInstance("virtualMachineName", ns)
				vmInstance.Spec.ResourcePolicyName = resourcePolicyName
			})

			Context("when a VM is referencing it", func() {
				It("should fail to delete", func() {
					err = c.Create(context.TODO(), &resourcePolicyInstance)
					Expect(err).NotTo(HaveOccurred())
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					err = c.Create(context.TODO(), &vmInstance)
					Expect(err).NotTo(HaveOccurred())

					err = c.Delete(context.TODO(), &resourcePolicyInstance)
					Expect(err).ShouldNot(HaveOccurred())
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					expectedError := fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'",
						vmInstance.NamespacedName(), resourcePolicyInstance.NamespacedName())
					Eventually(reconcileError).Should(Receive(Equal(expectedError)))
				})
			})

			Context("when no VMs are referencing it", func() {
				It("should successfully delete", func() {

					resourcePolicyInstance.Name = "resourcePolicyName-1"
					err = c.Create(context.TODO(), &resourcePolicyInstance)
					Expect(err).NotTo(HaveOccurred())
					expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: resourcePolicyInstance.Name, Namespace: ns}}
					Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

					// Create a different VM since the cache might not have synced yet
					vmInstance.Name = "virtualMachineName-1"
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
			//StorageClass: storageClass,
		},
	}
}
