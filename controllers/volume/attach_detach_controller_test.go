// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package volume

import (
	stdlog "log"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	storagetypev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

const (
	timeout          = time.Second * 10
	storageClassName = "foo-class"
)

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

var _ = Describe("Volume Attach Detach Controller", func() {

	var (
		classInstance   vmoperatorv1alpha1.VirtualMachineClass
		storageClass    *storagetypev1.StorageClass
		resourceQuota   *corev1.ResourceQuota
		instance        vmoperatorv1alpha1.VirtualMachine
		expectedRequest reconcile.Request
		recFn           reconcile.Reconciler
		requests        chan reconcile.Request
		stopMgr         chan struct{}
		mgrStopped      *sync.WaitGroup
		mgr             manager.Manager
		c               client.Client
		err             error
		ns              = integration.DefaultNamespace
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
		mgr, err = manager.New(cfg, manager.Options{
			SyncPeriod:         &syncPeriod,
			Scheme:             scheme.Scheme,
			MetricsBindAddress: "0",
		})
		Expect(err).NotTo(HaveOccurred())

		c = mgr.GetClient()

		ctrlContext := &controllerContext.ControllerManagerContext{}

		storageClass = generateStorageClass(ns)

		err = c.Create(context.TODO(), storageClass)
		Expect(err).ShouldNot(HaveOccurred())

		err = c.Create(context.TODO(), &classInstance)
		Expect(err).ShouldNot(HaveOccurred())

		recFn, requests, _, _ = integration.SetupTestReconcile(newReconciler(mgr))
		Expect(add(ctrlContext, mgr, recFn)).To(Succeed())

		stopMgr, mgrStopped = integration.StartTestManager(mgr)

		Expect(vmProvider.(vsphere.VSphereVmProviderGetSessionHack).SetContentLibrary(context.TODO(), integration.GetContentSourceID())).To(Succeed())
	})

	AfterEach(func() {

		stdlog.Printf("Cleaning up after test")
		close(stopMgr)
		mgrStopped.Wait()

		ctx := context.Background()

		// Don't validate errors so that we clean up as much as possible.  Delete with Background propagation in order to trigger immediate delete
		// Otherwise, the API server will place a finalizer on the resource expected the GC to remove, which leads to subsequent resource creation issues.
		err = c.Delete(ctx, storageClass, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))
		err = c.Delete(ctx, &classInstance, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))
		err = c.Delete(ctx, resourceQuota, client.PropagationPolicy(metav1.DeletePropagationBackground), client.GracePeriodSeconds(0))

		stdlog.Printf("Cleaned up after test")
	})

	// Assert the volume status updates are expected
	assertVmVolumeStatusUpdates := func(updatedVm *vmoperatorv1alpha1.VirtualMachine) {
		Eventually(func() bool {
			vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
			Expect(c.Get(context.TODO(), client.ObjectKey{Name: updatedVm.Name, Namespace: updatedVm.Namespace}, vmRetrieved)).To(Succeed())

			cnsNodeVmAttachmentList := &cnsv1alpha1.CnsNodeVmAttachmentList{}
			Expect(c.List(context.TODO(), cnsNodeVmAttachmentList, client.InNamespace(updatedVm.Namespace))).To(Succeed())

			// The number of vm.Status.Volumes should always be the same as the number of CNS CRs
			return len(vmRetrieved.Status.Volumes) == len(cnsNodeVmAttachmentList.Items)
		}, 60*time.Second).Should(BeTrue())
	}

	Context("when updating a VM object by attaching/detaching volumes", func() {
		// TODO:  Figure out a way to mock the client error in order to cover more corner cases
		It("invoke the reconcile method", func() {

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
					Volumes: []vmoperatorv1alpha1.VirtualMachineVolume{
						{Name: "fake-volume-1", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-1"}},
					},
				},
			}

			// Create the VM Object
			Expect(c.Create(context.TODO(), &instance)).To(Succeed())

			// Should not receive a reconcile until BiosUUID is set
			Eventually(requests, time.Second).ShouldNot(Receive(Equal(expectedRequest)))

			// Update Status.BiosUUID then expect Reconcile
			instance.Status = vmoperatorv1alpha1.VirtualMachineStatus{
				BiosUUID: "1-2-3-4",
			}
			Expect(c.Status().Update(context.Background(), &instance)).To(Succeed())

			// Expect one reconcile for the VM creation
			Eventually(requests, time.Second*10).Should(Receive(Equal(expectedRequest)))

			// VM Operator will create a CnsNodeVmAttachment instance for each volume.  Expect a reconcile for the
			// creation of the CnsNodeVmAttachment resource.
			Eventually(requests, time.Second*10).Should(Receive(Equal(expectedRequest)))

			// VirtualMachine volume status is expected to be updated properly
			stdlog.Printf("Testing first status update with %v", instance)
			assertVmVolumeStatusUpdates(&instance)

			// Update the volumes under virtual machine spec
			vmToUpdate := &vmoperatorv1alpha1.VirtualMachine{}

			// It is possible c.Update fails due to the reconciliation running in background has updated the vm instance
			// So we put the c.Update in Eventually block
			Eventually(func() error {
				Expect(c.Get(context.TODO(), client.ObjectKey{Name: vmName, Namespace: ns}, vmToUpdate)).Should(Succeed())
				vmToUpdate.Spec.Volumes = []vmoperatorv1alpha1.VirtualMachineVolume{
					{Name: "fake-volume-4", PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fake-pvc-4"}},
				}
				return c.Update(context.TODO(), vmToUpdate)
			}, timeout).Should(Succeed())

			// We must receive a subsequent reconcile for the VM update
			Eventually(requests, 30*time.Second).Should(Receive(Equal(expectedRequest)))

			// We must receive a subsequent reconcile for the CnsNodeVmAttachment associated with the new volume.
			Eventually(requests, 30*time.Second).Should(Receive(Equal(expectedRequest)))

			// VirtualMachine volume status is expected to be updated properly
			stdlog.Printf("Testing second status update with %v", vmToUpdate)
			assertVmVolumeStatusUpdates(vmToUpdate)

			// Check the volume names in status are the same as in spec
			Eventually(func() bool {
				vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
				Expect(c.Get(context.TODO(), client.ObjectKey{Name: vmName, Namespace: ns}, vmRetrieved)).NotTo(HaveOccurred())
				return checkVolumeNamesConsistency(vmRetrieved)
			}, 3*timeout).Should(BeTrue())

			// Delete the VM Object then expect Reconcile
			Expect(c.Delete(context.TODO(), &instance)).To(Succeed())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

		})
	})
})
