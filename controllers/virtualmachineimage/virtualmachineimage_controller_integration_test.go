// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

const timeout = time.Second * 30

func imageExistsFunc(imageList *vmoperatorv1alpha1.VirtualMachineImageList, name string) bool {
	for _, image := range imageList.Items {
		if image.Name == name {
			return true
		}
	}
	return false
}

func getVmConfigArgs(namespace, name string) vmprovider.VmConfigArgs {
	vmClass := getVMClassInstance(name, namespace)

	return vmprovider.VmConfigArgs{
		VmClass:          *vmClass,
		ResourcePolicy:   nil,
		VmMetadata:       &vmprovider.VmMetadata{},
		StorageProfileID: "foo",
	}
}

func getVMClassInstance(vmName, namespace string) *vmoperatorv1alpha1.VirtualMachineClass {
	return &vmoperatorv1alpha1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-class", vmName),
		},
		Spec: vmoperatorv1alpha1.VirtualMachineClassSpec{
			Hardware: vmoperatorv1alpha1.VirtualMachineClassHardware{
				Cpus:   4,
				Memory: resource.MustParse("1Mi"),
			},
		},
	}
}

func getVirtualMachineInstance(name, namespace, imageName, className string) *vmoperatorv1alpha1.VirtualMachine {
	return &vmoperatorv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSpec{
			ImageName:  imageName,
			ClassName:  className,
			PowerState: vmoperatorv1alpha1.VirtualMachinePoweredOn,
			Ports:      []vmoperatorv1alpha1.VirtualMachinePort{},
		},
	}
}

var _ = Describe("VirtualMachineImageDiscoverer", func() {

	var (
		c          client.Client
		stopMgr    chan struct{}
		mgrStopped *sync.WaitGroup
		mgr        manager.Manager
		err        error
		ctx        context.Context
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.

		syncPeriod := 5 * time.Second
		mgr, err = manager.New(restConfig, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		ctrlContext := &controllerContext.ControllerManagerContext{
			Logger:     ctrllog.Log.WithName("test"),
			VmProvider: vmProvider,
		}

		err = AddWithOptions(ctrlContext, mgr, VirtualMachineImageDiscovererOptions{
			initialDiscoveryFrequency:    1 * time.Second,
			continuousDiscoveryFrequency: 5 * time.Second})
		Expect(err).NotTo(HaveOccurred())

		stopMgr, mgrStopped = integration.StartTestManager(mgr)

		ctx = context.Background()

		// Reinitialize the session and client in case it is invalidated by other tests.
		session, err = vmProvider.(vsphere.VSphereVmProviderGetSessionHack).GetSession(context.TODO(), integration.DefaultNamespace)
		Expect(err).NotTo(HaveOccurred())

		Expect(vmProvider.(vsphere.VSphereVmProviderGetSessionHack).SetContentLibrary(ctx, integration.GetContentSourceID())).To(Succeed())
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	// The default integration test env is setup using the ConfigMap based approach.
	// The following test creates a ContentSource which points to a Content Library, uploads an image to the
	// library and expects the VM image to show up in the ListVirtualMachineImages call.
	// Once we move to the ContentSource based approach entirely, we can modify the default integration test env to use
	// content sources and get rid of this test.

	Context("with a ContentSource pointing to a content library", func() {
		var (
			libID            string
			clName           string
			contentSource    *vmoperatorv1alpha1.ContentSource
			contentLibrary   *vmoperatorv1alpha1.ContentLibraryProvider
			imageToAddNameCs string
		)

		// Function to create a content library
		aContentLibraryProvider := func(name string, libID string) *vmoperatorv1alpha1.ContentLibraryProvider {
			return &vmoperatorv1alpha1.ContentLibraryProvider{
				TypeMeta: metav1.TypeMeta{
					Kind: "ContentLibraryProvider",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: vmoperatorv1alpha1.ContentLibraryProviderSpec{
					UUID: libID,
				},
			}
		}

		// Function to create a content source with a ProviderRef to a content library with the same name.
		aContentSource := func(name string, contentLibrary *vmoperatorv1alpha1.ContentLibraryProvider) *vmoperatorv1alpha1.ContentSource {
			return &vmoperatorv1alpha1.ContentSource{
				TypeMeta: metav1.TypeMeta{
					Kind: "ContentSource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: vmoperatorv1alpha1.ContentSourceSpec{
					vmoperatorv1alpha1.ContentProviderReference{
						APIVersion: contentLibrary.APIVersion,
						Kind:       contentLibrary.Kind,
						Name:       contentLibrary.Name,
					},
				},
			}
		}

		BeforeEach(func() {
			os.Setenv("FSS_WCP_VMSERVICE", "true")

			clName = "content-library-name"
			// Create a content library to back the ContentSource
			libID, err = session.CreateLibrary(ctx, clName)
			Expect(err).NotTo(HaveOccurred())

			imageToAddNameCs = "contentsource-image-contentsource"
			Expect(integration.CreateLibraryItem(ctx, session, imageToAddNameCs, "ovf", libID)).To(Succeed())

			contentLibrary = aContentLibraryProvider(clName, libID)
			Expect(c.Create(context.TODO(), contentLibrary)).To(Succeed())

			contentSource = aContentSource(clName, contentLibrary)
			Expect(c.Create(context.TODO(), contentSource)).To(Succeed())

			// Wait for the image to show up in list VM images.
			Eventually(func() bool {
				imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
				err := c.List(context.TODO(), imageList)
				Expect(err).ShouldNot(HaveOccurred())
				return imageExistsFunc(imageList, imageToAddNameCs)
			}, timeout).Should(BeTrue())
		})

		AfterEach(func() {
			os.Unsetenv("FSS_WCP_VMSERVICE")

			// Delete the ContentSource resource
			Expect(c.Delete(context.TODO(), contentSource)).To(Succeed())
			Eventually(func() error {
				cs := &vmoperatorv1alpha1.ContentSource{}
				csObjectKey := types.NamespacedName{Name: contentSource.Name, Namespace: contentSource.Namespace}

				err := c.Get(context.TODO(), csObjectKey, cs)
				return err
			}, timeout).ShouldNot(Succeed())

			// Delete the ContentLibraryProvider resouce. Ideally this should not be needed since ContentLibraryProvider has an
			// OwnerReference to the ContentSource. We delete explicitly here so we do not have to wait for the reconcile.
			Expect(c.Delete(context.TODO(), contentLibrary)).To(Succeed())
			Eventually(func() error {
				cl := &vmoperatorv1alpha1.ContentLibraryProvider{}
				clObjectKey := types.NamespacedName{Name: contentLibrary.Name, Namespace: contentLibrary.Namespace}

				err := c.Get(context.TODO(), clObjectKey, cl)
				return err
			}, timeout).ShouldNot(Succeed())

			// Delete the Content Library created by the test
			Expect(session.DeleteContentLibrary(context.TODO(), libID)).To(Succeed())

			session, err = vmProvider.(vsphere.VSphereVmProviderGetSessionHack).GetSession(context.TODO(), "")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with an image in the ContentSource pointed library", func() {

			It("should only list the image from the ContentSource library", func() {
				// Tested by the BeforeEach/AfterEach block.
			})

			It("should successfully be able to clone a VirtualMachine", func() {
				testNamespace := "vmservice-test-clone-ns"
				ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
				Expect(c.Create(context.TODO(), ns)).To(Succeed())

				// Configure the test to use the test namespace. This will make sure that the session is using content library from
				// ContentSource resources.
				session, err = vmProvider.(vsphere.VSphereVmProviderGetSessionHack).GetSession(context.TODO(), testNamespace)
				Expect(err).NotTo(HaveOccurred())

				vmName := "vmservice-vm"
				vmConfigArgs := getVmConfigArgs(testNamespace, vmName)
				vm := getVirtualMachineInstance(vmName, testNamespace, imageToAddNameCs, vmConfigArgs.VmClass.Name)

				clonedVM, err := session.CloneVirtualMachine(context.TODO(), vm, vmConfigArgs, vmProvider.(vsphere.VSphereVmProviderGetSessionHack).GetContentLibrary())
				Expect(err).NotTo(HaveOccurred())
				Expect(clonedVM.Name).Should(Equal(vmName))
			})
		})
	})

	Describe("when the session's content library is deleted", func() {
		BeforeEach(func() {
			// update the session's content source to point to a CL that we will delete. This is done so we don't delete the content library being used by the
			// integration test framework.
			tempSessionCL, err := session.CreateLibrary(ctx, "temporary-content-library")
			Expect(err).NotTo(HaveOccurred())

			Expect(vmProvider.(vsphere.VSphereVmProviderGetSessionHack).SetContentLibrary(ctx, tempSessionCL)).To(Succeed())

			// Delete the content library pointed by the session
			Expect(session.DeleteContentLibrary(ctx, tempSessionCL)).To(Succeed())
		})

		It("returns empty list of images", func() {
			Eventually(func() bool {
				imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
				err = c.List(ctx, imageList)
				Expect(err).ShouldNot(HaveOccurred())
				return len(imageList.Items) == 0
			}, timeout).Should(BeTrue())
		})
	})

	Describe("with VM images in inventory", func() {

		Context("with an initial image in the CL", func() {

			BeforeEach(func() {
				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(ctx, imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return len(imageList.Items) == 1 && imageList.Items[0].Name == integration.IntegrationContentLibraryItemName
				}, timeout).Should(BeTrue())
			})

			It("should list the VM images", func() {
				// tested in BeforeEach
			})

			It("should update the image metadata if it is out of date", func() {
				var clImage vmoperatorv1alpha1.VirtualMachineImage
				imageKey := client.ObjectKey{Name: integration.IntegrationContentLibraryItemName}
				Expect(c.Get(ctx, imageKey, &clImage)).To(Succeed())

				// Remove annotations from the clImage.
				prevOSInfo := clImage.Spec.OSInfo
				clImage.Spec.OSInfo = vmoperatorv1alpha1.VirtualMachineImageOSInfo{}
				Expect(c.Update(ctx, &clImage)).To(Succeed())

				// Eventually, image sync should have updated the objects with correct metadata.
				Eventually(func() bool {
					image := vmoperatorv1alpha1.VirtualMachineImage{}
					Expect(c.Get(ctx, imageKey, &image)).To(Succeed())

					return reflect.DeepEqual(image.Spec.OSInfo, prevOSInfo)
				}, timeout).Should(BeTrue())
			})
		})

		Context("with a Content Library configured", func() {
			var err error

			It("should add a VirtualMachineImage to if an item is added to the library", func() {
				var imageToAddName = "image-to-add"

				err = integration.CreateLibraryItem(ctx, session, imageToAddName, "ovf", integration.ContentSourceID)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), imageList)
					Expect(err).ShouldNot(HaveOccurred())
					return imageExistsFunc(imageList, imageToAddName)
				}, timeout).Should(BeTrue())
			})

			It("should not fail fast with invalid VirtualMachineImage image name added to the library", func() {
				var imageToAddName1 = "image-to-add2"
				var imageToAddName2 = "image_not_to_add"

				err = integration.CreateLibraryItem(ctx, session, imageToAddName1, "ovf", integration.ContentSourceID)
				Expect(err).ShouldNot(HaveOccurred())

				err = integration.CreateLibraryItem(ctx, session, imageToAddName2, "ovf", integration.ContentSourceID)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), imageList)
					Expect(err).ShouldNot(HaveOccurred())
					return imageExistsFunc(imageList, imageToAddName1) && !imageExistsFunc(imageList, imageToAddName2)
				}, timeout).Should(BeTrue())
			})

			// TODO: Need Jira for CL item removal
			XIt("should remove a VirtualMachineImage if one is removed from the library", func() {
				var imageToRemoveName = "imageToRemove"

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), imageList)
					Expect(err).ShouldNot(HaveOccurred())
					return imageExistsFunc(imageList, imageToRemoveName)
				}, timeout).Should(BeFalse())
			})

			It("should remove an extra image if one was added to the control plane", func() {
				var strayImageName = "stray-image"

				strayImage := vmoperatorv1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: strayImageName,
					},
					Spec: vmoperatorv1alpha1.VirtualMachineImageSpec{
						Type:            "ovf",
						ImageSourceType: "Content Library",
					},
				}

				err = c.Create(ctx, &strayImage)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return imageExistsFunc(imageList, strayImageName)
				}, timeout).Should(BeFalse())
			})

			It("should re-create if image is deleted", func() {
				var imageToRecreate = "image-to-recreate"

				err = integration.CreateLibraryItem(ctx, session, imageToRecreate, "ovf", integration.ContentSourceID)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return imageExistsFunc(imageList, imageToRecreate)
				}, timeout).Should(BeTrue())

				existingImage := vmoperatorv1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: imageToRecreate,
					},
				}
				err = c.Delete(ctx, &existingImage)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return imageExistsFunc(imageList, imageToRecreate)
				}, timeout).Should(BeTrue())
			})
		})
	})
})

var _ = Describe("ReconcileVirtualMachineImage", func() {

	var (
		c               client.Client
		stopMgr         chan struct{}
		mgrStopped      *sync.WaitGroup
		mgr             manager.Manager
		expectedRequest reconcile.Request
		recFn           reconcile.Reconciler
		requests        chan reconcile.Request
		err             error
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.

		syncPeriod := 5 * time.Second
		mgr, err = manager.New(restConfig, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		ctrlContext := &controllerContext.ControllerManagerContext{
			Logger:     ctrllog.Log.WithName("test"),
			VmProvider: vmProvider,
		}

		recFn, requests, _, _ = integration.SetupTestReconcile(newReconciler(ctrlContext, mgr, VirtualMachineImageDiscovererOptions{
			initialDiscoveryFrequency:    1 * time.Second,
			continuousDiscoveryFrequency: 5 * time.Second}))
		Expect(add(mgr, recFn)).To(Succeed())

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("Reconcile", func() {
		It("Should receive reconcile event and do nothing", func() {
			// Create the VM Image Object then expect Reconcile
			image := vmoperatorv1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image-to-reconcile",
				},
			}
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "image-to-reconcile"}}

			err := c.Create(context.TODO(), &image)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})

})

var _ = Describe("ReconcileVMOpConfigMap", func() {

	var (
		c               client.Client
		stopMgr         chan struct{}
		mgrStopped      *sync.WaitGroup
		mgr             manager.Manager
		expectedRequest reconcile.Request
		recFn           reconcile.Reconciler
		requests        chan reconcile.Request
		err             error
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.

		syncPeriod := 5 * time.Second
		mgr, err = manager.New(restConfig, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		ctrlContext := &controllerContext.ControllerManagerContext{
			Logger:     ctrllog.Log.WithName("test"),
			VmProvider: vmProvider,
		}

		recFn, requests, _, _ = integration.SetupTestReconcile(newCMReconciler(ctrlContext, mgr))
		Expect(addCM(mgr, recFn)).To(Succeed())

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("Reconcile", func() {
		It("invoke the reconcile method while updating VM Operator ConfigMap", func() {
			vmOperatorConfigNamespacedName := types.NamespacedName{
				Name:      vsphere.VSphereConfigMapName,
				Namespace: integration.DefaultNamespace,
			}

			expectedRequest = reconcile.Request{NamespacedName: vmOperatorConfigNamespacedName}

			// Delete Provider ConfigMap created for integration
			providerConfigMap := vsphere.ProviderConfigToConfigMap(integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, ""),
				integration.SecretName)
			err = c.Delete(context.TODO(), providerConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).ShouldNot(Receive(Equal(expectedRequest)))

			// Recreate Provider ConfigMap created for integration
			providerConfigMap = vsphere.ProviderConfigToConfigMap(integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, ""),
				integration.SecretName)
			err = c.Create(context.TODO(), providerConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Get ProviderConfigMap with a changed Content source
			providerConfigMap = vsphere.ProviderConfigToConfigMap(integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
				integration.SecretName)

			// Call update on the ConfigMap
			err = c.Update(context.TODO(), providerConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})
})
