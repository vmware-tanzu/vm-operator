// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	storagetypev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	vmrecord "github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const (
	timeout      = time.Second * 5
	storageClass = "foo-class"
)

var _ = Describe("VirtualMachine controller", func() {
	ns := integration.DefaultNamespace
	name := "fooVm"

	var (
		classInstance           vmoperatorv1alpha1.VirtualMachineClass
		sc                      storagetypev1.StorageClass
		instance                vmoperatorv1alpha1.VirtualMachine
		expectedRequest         reconcile.Request
		recFn                   reconcile.Reconciler
		requests                chan reconcile.Request
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
	)

	BeforeEach(func() {
		classInstance = vmoperatorv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
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

		sc.Provisioner = "foo"
		sc.Parameters = make(map[string]string)
		sc.Parameters["storagePolicyID"] = "foo"
		sc.Name = storageClass

		err = c.Create(context.TODO(), &sc)

		recFn, requests = SetupTestReconcile(newReconciler(mgr))
		Expect(add(mgr, recFn)).To(Succeed())

		stopMgr, mgrStopped = StartTestManager(mgr)

		err = c.Create(context.TODO(), &classInstance)
		Expect(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err = c.Delete(context.TODO(), &classInstance)
		Expect(err).ShouldNot(HaveOccurred())

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

	Describe("when creating/deleting a VM object", func() {
		Context("from inventory", func() {
			It("invoke the reconcile method", func() {
				provider := vmprovider.GetVmProviderOrDie()
				p := provider.(*vsphere.VSphereVmProvider)
				session, err := p.GetSession(context.TODO(), ns)
				Expect(err).NotTo(HaveOccurred())

				//Configure to use inventory
				vSphereConfig.ContentSource = ""
				err = session.ConfigureContent(context.TODO(), vSphereConfig.ContentSource)
				Expect(err).NotTo(HaveOccurred())

				vmName := "fooVM"
				expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmName}}
				imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
				err = c.List(context.TODO(), &client.ListOptions{Namespace: ns}, imageList)
				/* TODO() This List call does not seem to not pass along the namespace
				Expect(err).ShouldNot(HaveOccurred())
				imageName := imageList.Items[0].Name
				*/
				imageName := "DC0_H0_VM0"
				instance = vmoperatorv1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      vmName,
					},
					Spec: vmoperatorv1alpha1.VirtualMachineSpec{
						ImageName:    imageName,
						ClassName:    classInstance.Name,
						PowerState:   "poweredOn",
						Ports:        []vmoperatorv1alpha1.VirtualMachinePort{},
						StorageClass: storageClass,
					},
				}

				fakeRecorder := vmrecord.GetRecorder().(*record.FakeRecorder)

				// Create the VM Object then expect Reconcile
				err = c.Create(context.TODO(), &instance)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
				// Delete the VM Object then expect Reconcile
				err = c.Delete(context.TODO(), &instance)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

				reasonMap := vmrecord.ReadEvents(fakeRecorder)
				Expect(reasonMap[vmrecord.Success+OpCreate]).Should(Equal(1))
				Expect(reasonMap[vmrecord.Success+OpDelete]).Should(Equal(1))
			})
		})

		Context("from Content Library", func() {
			It("invoke the reconcile method", func() {
				provider := vmprovider.GetVmProviderOrDie()
				p := provider.(*vsphere.VSphereVmProvider)
				session, err := p.GetSession(context.TODO(), ns)
				Expect(err).NotTo(HaveOccurred())

				//Configure to use Content Library
				vSphereConfig.ContentSource = integration.ContentSourceName
				err = session.ConfigureContent(context.TODO(), vSphereConfig.ContentSource)
				Expect(err).NotTo(HaveOccurred())

				vmName := "CL-DeployedVM"
				expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmName}}
				imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
				err = c.List(context.TODO(), &client.ListOptions{Namespace: ns}, imageList)
				/* TODO() This List call does not seem to not pass along the namespace
				Expect(err).ShouldNot(HaveOccurred())
				imageName := imageList.Items[0].Name
				*/

				imageName := "test-item"
				instance = vmoperatorv1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      vmName,
					},
					Spec: vmoperatorv1alpha1.VirtualMachineSpec{
						ImageName:    imageName,
						ClassName:    classInstance.Name,
						PowerState:   "poweredOn",
						Ports:        []vmoperatorv1alpha1.VirtualMachinePort{},
						StorageClass: storageClass,
					},
				}

				fakeRecorder := vmrecord.GetRecorder().(*record.FakeRecorder)

				// Create the VM Object then expect Reconcile
				err = c.Create(context.TODO(), &instance)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
				// Delete the VM Object then expect Reconcile
				err = c.Delete(context.TODO(), &instance)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

				reasonMap := vmrecord.ReadEvents(fakeRecorder)
				Expect(reasonMap[vmrecord.Success+OpCreate]).Should(Equal(1))
				Expect(reasonMap[vmrecord.Success+OpDelete]).Should(Equal(1))
			})
		})
	})
})
