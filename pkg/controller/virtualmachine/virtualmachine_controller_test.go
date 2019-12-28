// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"fmt"
	stdlog "log"
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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	vmrecord "github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
	"gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
)

var c client.Client

const (
	timeout          = time.Second * 10
	storageClassName = "foo-class"
)

func generateDefaultResourceQuota() *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		TypeMeta: metav1.TypeMeta{
			Kind: "ResourceQuota",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rq-for-unit-test",
			Namespace: integration.DefaultNamespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				storageClassName + ".storageclass.storage.k8s.io/persistentvolumeclaims": resource.MustParse("1"),
				"simple-class" + ".storageclass.storage.k8s.io/persistentvolumeclaims":   resource.MustParse("1"),
				"limits.cpu":    resource.MustParse("2"),
				"limits.memory": resource.MustParse("2Gi"),
			},
		},
	}
}

func generateStorageClass(ns string) *storagetypev1.StorageClass {
	parameters := make(map[string]string)
	parameters["storagePolicyID"] = "foo"

	return &storagetypev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      storageClassName,
		},
		Provisioner: "foo",
		Parameters:  parameters,
	}
}

// Checking if the spec and status of virtual machine volumes are eventually consistent
// The following function returns true if the vm.Spec.Volumes equal to vm.Status.Volumes
func checkVolumeNamesConsistency(vm *vmoperatorv1alpha1.VirtualMachine) bool {
	set := make(map[string]bool)
	for _, volume := range vm.Spec.Volumes {
		set[volume.Name] = true
	}
	for _, volume := range vm.Status.Volumes {
		if set[volume.Name] {
			delete(set, volume.Name)
		} else {
			return false
		}
	}
	return len(set) == 0
}

// Assert the volume status updates are expected
func assertVmVolumeStatusUpdates(updatedVm *vmoperatorv1alpha1.VirtualMachine) {
	Eventually(func() bool {
		vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
		Expect(c.Get(context.TODO(), client.ObjectKey{Name: updatedVm.Name, Namespace: updatedVm.Namespace}, vmRetrieved)).To(Succeed())

		cnsNodeVmAttachmentList := &v1alpha1.CnsNodeVmAttachmentList{}
		Expect(c.List(context.TODO(), &client.ListOptions{Namespace: updatedVm.Namespace}, cnsNodeVmAttachmentList)).To(Succeed())

		// The number of vm.Status.Volumes should always be the same as the number of CNS CRs
		return len(vmRetrieved.Status.Volumes) == len(cnsNodeVmAttachmentList.Items)
	}, 60*time.Second).Should(BeTrue())
}

var _ = Describe("VirtualMachine controller", func() {

	var (
		classInstance           vmoperatorv1alpha1.VirtualMachineClass
		storageClass            *storagetypev1.StorageClass
		resourceQuota           *corev1.ResourceQuota
		instance                vmoperatorv1alpha1.VirtualMachine
		expectedRequest         reconcile.Request
		recFn                   reconcile.Reconciler
		requests                chan reconcile.Request
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
		ns                      = integration.DefaultNamespace
	)

	BeforeEach(func() {
		classInstance = vmoperatorv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "small",
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

		syncPeriod := 5 * time.Second
		leaderElectionConfigMap = fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod,
			LeaderElection:          true,
			LeaderElectionID:        leaderElectionConfigMap,
			LeaderElectionNamespace: ns})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		storageClass = generateStorageClass(ns)

		err = c.Create(context.TODO(), storageClass)
		Expect(err).ShouldNot(HaveOccurred())

		resourceQuota = generateDefaultResourceQuota()
		err = c.Create(context.TODO(), resourceQuota)
		Expect(err).ShouldNot(HaveOccurred())

		err = c.Create(context.TODO(), &classInstance)
		Expect(err).ShouldNot(HaveOccurred())

		recFn, requests, _ = integration.SetupTestReconcile(newReconciler(mgr))
		Expect(add(mgr, recFn)).To(Succeed())

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {

		stdlog.Printf("Cleaning up after test")
		close(stopMgr)
		mgrStopped.Wait()

		ctx := context.Background()

		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      leaderElectionConfigMap,
			},
		}

		// Don't validate errors so that we clean up as much as possible.  Delete with Background propagation in order to trigger immmediate delete
		// Otherwise, the API server will place a finalizer on the resource expected the GC to remove, which leads to subsequent resource creation issues.
		err = c.Delete(ctx, storageClass, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))
		err = c.Delete(ctx, configMap, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))
		err = c.Delete(ctx, &classInstance, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))
		err = c.Delete(ctx, resourceQuota, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))

		stdlog.Printf("Cleaned up after test")
	})

	Context("when creating/deleting a VM object from Inventory", func() {
		It("invoke the reconcile method with valid storage class", func() {
			provider := vmprovider.GetVmProviderOrDie()

			err = vsphere.InstallVSphereVmProviderConfig(kubernetes.NewForConfigOrDie(cfg),
				integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, ""),
				integration.SecretName)
			Expect(err).NotTo(HaveOccurred())

			images, err := provider.ListVirtualMachineImages(context.TODO(), ns)

			for _, image := range images {
				stdlog.Printf("image %s", image.Name)
			}

			vmName := "fooVM"
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmName}}

			instance = vmoperatorv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      vmName,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineSpec{
					ImageName:    "DC0_H0_VM0", // Default govcsim image name
					ClassName:    classInstance.Name,
					PowerState:   "poweredOn",
					Ports:        []vmoperatorv1alpha1.VirtualMachinePort{},
					StorageClass: storageClassName,
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
			Eventually(requests, time.Second*10).Should(Receive(Equal(expectedRequest)))

			Eventually(func() bool {
				reasonMap := vmrecord.ReadEvents(fakeRecorder)
				if (reasonMap[vmrecord.Failure+OpCreate] != 0) || (reasonMap[vmrecord.Failure+OpDelete] != 0) ||
					(reasonMap[vmrecord.Success+OpDelete] != 1) || (reasonMap[vmrecord.Success+OpCreate] != 1) {
					GinkgoWriter.Write([]byte(fmt.Sprintf("reasonMap =  %v", reasonMap)))
					return false
				}
				return true
			}, timeout).Should(BeTrue())
		})
	})

	Context("when creating/deleting a VM object from Content Library", func() {
		It("invoke the reconcile method", func() {
			err = vsphere.InstallVSphereVmProviderConfig(kubernetes.NewForConfigOrDie(cfg),
				integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
				integration.SecretName)
			Expect(err).NotTo(HaveOccurred())

			vmName := "cl-deployed-vm"
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmName}}

			// Use the CL image setup by the integration framework
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
					StorageClass: storageClassName,
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
			Eventually(func() bool {
				reasonMap := vmrecord.ReadEvents(fakeRecorder)
				if (reasonMap[vmrecord.Success+OpDelete] != 1) || (reasonMap[vmrecord.Success+OpCreate] != 1) {
					GinkgoWriter.Write([]byte(fmt.Sprintf("reasonMap =  %v", reasonMap)))
					return false
				}
				return true
			}, timeout).Should(BeTrue())
		})
	})

	Context("when updating a VM object by attaching/detaching volumes", func() {
		// TODO:  Figure out a way to mock the client error in order to cover more corner cases
		It("invoke the reconcile method", func() {

			provider := vmprovider.GetVmProviderOrDie()

			p := provider.(*vsphere.VSphereVmProvider)
			session, err := p.GetSession(context.TODO(), ns)
			Expect(err).NotTo(HaveOccurred())

			//Configure to use Content Library
			vSphereConfig.ContentSource = integration.GetContentSourceID()
			Expect(session.ConfigureContent(context.TODO(), vSphereConfig.ContentSource)).To(Succeed())

			vmName := "volume-test-vm"
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vmName}}
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
					StorageClass: storageClassName,
					Volumes: []vmoperatorv1alpha1.VirtualMachineVolumes{
						{Name: "fake-volume-1", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-1"}},
						{Name: "fake-volume-2", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-2"}},
						{Name: "fake-volume-3", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-3"}},
					},
				},
			}

			// Create the VM Object then expect Reconcile
			Expect(c.Create(context.TODO(), &instance)).To(Succeed())
			// vmop will create CnsNodeVmAttachment instances, however they might not be created and available in one reconcile loop.
			// there might have several reconcile loops, and the maximum number of loops are the same as the number of volumes
			for range instance.Spec.Volumes {
				Eventually(requests, time.Second*10).Should(Receive(Equal(expectedRequest)))
			}

			// VirtualMachine volume status is expected to be updated properly
			assertVmVolumeStatusUpdates(&instance)

			// Update the volumes under virtual machine spec
			vmToUpdate := &vmoperatorv1alpha1.VirtualMachine{}
			// It is possible c.Update fails due to the reconciliation running in background has updated the vm instance
			// So we put the c.Update in Eventually block
			updateAttempts := 0
			Eventually(func() error {
				updateAttempts++
				Expect(c.Get(context.TODO(), client.ObjectKey{Name: vmName, Namespace: ns}, vmToUpdate)).Should(Succeed())
				vmToUpdate.Spec.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumes{
					{Name: "fake-volume-1", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-1"}},
					{Name: "fake-volume-4", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-4"}},
				}
				return c.Update(context.TODO(), vmToUpdate)
			}, timeout).Should(Succeed())
			// If c.Update has been executed multiple times, we expect to see the expectedRequest gets requeued
			for i := 0; i < updateAttempts; i++ {
				Eventually(requests, 30*time.Second).Should(Receive(Equal(expectedRequest)))
			}

			// VirtualMachine volume status is expected to be updated properly
			assertVmVolumeStatusUpdates(vmToUpdate)

			// Check the volume names in status are the same as in spec
			vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
			Expect(c.Get(context.TODO(), client.ObjectKey{Name: vmName, Namespace: ns}, vmRetrieved)).NotTo(HaveOccurred())
			Expect(checkVolumeNamesConsistency(vmRetrieved)).To(BeTrue())

			// Delete the VM Object then expect Reconcile
			Expect(c.Delete(context.TODO(), &instance)).To(Succeed())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

		})
	})
})
