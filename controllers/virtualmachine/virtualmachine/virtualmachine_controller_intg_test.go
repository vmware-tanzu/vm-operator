// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	vmwatcher "github.com/vmware-tanzu/vm-operator/services/vm-watcher"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {

	var (
		ctx *builder.IntegrationTestContext

		vm    *vmopv1.VirtualMachine
		vmKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    "dummy-image",
				ClassName:    "dummy-class",
				StorageClass: "my-storage-class",
				PowerState:   vmopv1.VirtualMachinePowerStateOn,
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	waitForVirtualMachineFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func(g Gomega) {
			vm := getVirtualMachine(ctx, objKey)
			g.Expect(vm).ToNot(BeNil())
			g.Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
		}).Should(Succeed(), "waiting for VirtualMachine finalizer")
	}

	Context("Reconcile", func() {
		const dummyIPAddress = "1.2.3.4"
		dummyInstanceUUID := uuid.NewString()

		BeforeEach(func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					return nil
				},
			)
		})

		AfterEach(func() {
			By("Delete VirtualMachine", func() {
				if err := ctx.Client.Delete(ctx, vm); err == nil {
					vm := &vmopv1.VirtualMachine{}
					// If VM is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, vmKey, vm); err == nil && len(vm.Finalizers) > 0 {
						vm.Finalizers = nil
						_ = ctx.Client.Update(ctx, vm)
					}
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			var createAttempts int32

			By("Exceed number of allowed concurrent create operations", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						atomic.AddInt32(&createAttempts, 1)
						return providers.ErrTooManyCreates
					},
				)
			})

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})

			Eventually(func(g Gomega) {
				g.Expect(atomic.LoadInt32(&createAttempts)).To(BeNumerically(">=", int32(3)))
				g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())
			}, "5s").Should(
				Succeed(),
				"waiting for reconcile to be requeued at least three times")

			atomic.StoreInt32(&createAttempts, 0)

			By("Causing duplicate creates", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						atomic.AddInt32(&createAttempts, 1)
						return providers.ErrReconcileInProgress
					},
				)
			})

			Eventually(func(g Gomega) {
				g.Expect(atomic.LoadInt32(&createAttempts)).To(BeNumerically(">=", int32(3)))
				g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())
			}, "5s").Should(
				Succeed(),
				"waiting for reconcile to be requeued at least three times")

			By("Set InstanceUUID in CreateOrUpdateVM", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						// Just using InstanceUUID here for a field to update.
						vm.Status.InstanceUUID = dummyInstanceUUID
						return nil
					},
				)
			})

			By("VirtualMachine should have InstanceUUID set", func() {
				// This depends on CreateVMRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
				}, "4s").Should(Succeed(), "waiting for expected InstanceUUID")
			})

			By("Set Created condition in CreateOrUpdateVM", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
						conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
						return nil
					},
				)
			})

			By("VirtualMachine should have Created condition set", func() {
				// This depends on CreateVMRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for Created condition")
			})

			By("Set IP address in CreateOrUpdateVM", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
							PrimaryIP4: dummyIPAddress,
						}
						return nil
					},
				)
			})

			By("VirtualMachine should have IP address set", func() {
				// This depends on PoweredOnVMHasIPRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Network).ToNot(BeNil())
					g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
				}, "4s").Should(Succeed(), "waiting for IP address")
			})

			// Used with "gingko -v . -- -v 5" to verify that controller.go is
			// logging:
			//
			//     "Reconciler error" err="terminal error: helloworld"
			By("Reconciler returns a NoRequeue error with DoNotErr=false", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						conditions.MarkTrue(vm, "HelloWorld")
						return pkgerr.NoRequeueError{
							Message:  "helloworld",
							DoNotErr: false,
						}
					},
				)
				vm := getVirtualMachine(ctx, vmKey)
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations["hello"] = "world"
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, "HelloWorld")).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for HelloWorld condition")
			})

			// Used with "gingko -v . -- -v 5" to verify that controller.go is
			// not logging:
			//
			//     "Reconciler error" err="terminal error: fubar"
			By("Reconciler returns a NoRequeue error with DoNotErr=true", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						conditions.MarkTrue(vm, "Fubar")
						return pkgerr.NoRequeueError{
							Message:  "fubar",
							DoNotErr: true,
						}
					},
				)
				vm := getVirtualMachine(ctx, vmKey)
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations["fu"] = "bar"
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, "Fubar")).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for Fubar condition")
			})

			By("VirtualMachine should not be updated in steady-state", func() {
				vm := getVirtualMachine(ctx, vmKey)
				Expect(vm).ToNot(BeNil())
				rv := vm.GetResourceVersion()
				Expect(rv).ToNot(BeEmpty())
				expected := fmt.Sprintf("%s :: %d", rv, vm.GetGeneration())
				Consistently(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(fmt.Sprintf("%s :: %d", vm.GetResourceVersion(), vm.GetGeneration())).To(Equal(expected))
				}, "4s").Should(Succeed(), "no updates in steady state")
			})
		})

		When("VM is a vSphere Kubernetes node", func() {

			var (
				oldCapVal bool
				newCapVal bool
			)

			BeforeEach(func() {
				if vm.Labels == nil {
					vm.Labels = map[string]string{}
				}
				vm.Labels[vmopv1util.KubernetesNodeLabelKey] = ""
			})

			AfterEach(func() {
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.Features.WorkloadDomainIsolation = oldCapVal
				})
			})

			JustBeforeEach(func() {
				oldCapVal = pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.Features.WorkloadDomainIsolation = newCapVal
				})
			})

			When("WorkloadDomainIsolation capability is disabled", func() {

				BeforeEach(func() {
					newCapVal = false
				})

				It("Reconciles after VirtualMachine creation", func() {
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

					By("VirtualMachine should have finalizer added", func() {
						waitForVirtualMachineFinalizer(ctx, vmKey)
					})

					By("Set VM status", func() {
						providerfake.SetCreateOrUpdateFunction(
							ctx,
							intgFakeVMProvider,
							func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
								vm.Status.InstanceUUID = dummyInstanceUUID
								vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
								conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
								vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
									PrimaryIP4: dummyIPAddress,
								}
								return nil
							},
						)
					})

					By("VirtualMachine should be created", func() {
						// This depends on CreateVMRequeueDelay to timely reflect the update.
						Eventually(func(g Gomega) {
							vm := getVirtualMachine(ctx, vmKey)
							g.Expect(vm).ToNot(BeNil())
							g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
							g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
							g.Expect(vm.Status.Network).ToNot(BeNil())
							g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
						}, "5s").Should(Succeed(), "waiting for VM to be created")
					})
				})
			})

			When("WorkloadDomainIsolation capability is enabled", func() {

				BeforeEach(func() {
					newCapVal = false
				})

				It("Reconciles after VirtualMachine creation", func() {
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

					By("VirtualMachine should have finalizer added", func() {
						waitForVirtualMachineFinalizer(ctx, vmKey)
					})

					By("Set VM status", func() {
						providerfake.SetCreateOrUpdateFunction(
							ctx,
							intgFakeVMProvider,
							func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
								vm.Status.InstanceUUID = dummyInstanceUUID
								vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
								conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
								vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
									PrimaryIP4: dummyIPAddress,
								}
								return nil
							},
						)
					})

					By("VirtualMachine should be created", func() {
						// This depends on CreateVMRequeueDelay to timely reflect the update.
						Eventually(func(g Gomega) {
							vm := getVirtualMachine(ctx, vmKey)
							g.Expect(vm).ToNot(BeNil())
							g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
							g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
							g.Expect(vm.Status.Network).ToNot(BeNil())
							g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
						}, "5s").Should(Succeed(), "waiting for VM to be created")
					})
				})
			})

		})

		It("Reconciles after VirtualMachineClass change", func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Set this so requeueDelay() returns 0.
					conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
					return nil
				},
			)

			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			By("VirtualMachine should be reconciled", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				}).Should(Succeed())
			})

			instanceUUID := uuid.NewString()

			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.InstanceUUID = instanceUUID
					return nil
				},
			)

			vmClass := builder.DummyVirtualMachineClass(vm.Spec.ClassName)
			vmClass.Namespace = vm.Namespace
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			By("VirtualMachine should be reconciled because of class", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal(instanceUUID))
				}).Should(Succeed())
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

			By("VirtualMachine should be deleted", func() {
				Eventually(func(g Gomega) {
					g.Expect(getVirtualMachine(ctx, vmKey)).To(BeNil())
				}).Should(Succeed())
			})
		})

		When("Provider DeleteVM returns an error", func() {
			errMsg := "delete error"
			instanceUUID := uuid.NewString()

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Set InstanceUUID to know this was called.
					vm.Status.InstanceUUID = instanceUUID
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine still has finalizer", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

				By("Finalizer should still be present", func() {
					Eventually(func(g Gomega) {
						vm := getVirtualMachine(ctx, vmKey)
						Expect(vm).ToNot(BeNil())
						Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
						Expect(vm.Status.InstanceUUID).To(Equal(instanceUUID))
					})
				})
			})
		})
	})
}

var _ = Describe(
	"ChanSource",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	), func() {

		const (
			vmName = "my-vm-1"
		)

		var (
			ctx                    context.Context
			vcSimCtx               *builder.IntegrationTestContextForVCSim
			provider               *providerfake.VMProvider
			initEnvFn              builder.InitVCSimEnvFn
			vm                     *object.VirtualMachine
			numCreateOrUpdateCalls int32
			obj                    *vmopv1.VirtualMachine
			objKey                 client.ObjectKey
		)

		BeforeEach(func() {
			numCreateOrUpdateCalls = 0
			ctx = context.Background()
			ctx = logr.NewContext(ctx, testutil.GinkgoLogr(4))
			obj = &vmopv1.VirtualMachine{}
		})

		JustBeforeEach(func() {
			ctx = logr.NewContext(
				context.Background(),
				textlogger.NewLogger(textlogger.NewConfig(
					textlogger.Verbosity(2),
					textlogger.Output(GinkgoWriter),
				)))

			ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
			ctx = pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.AsyncCreateEnabled = true
					config.AsyncSignalEnabled = true
				},
			)
			ctx = cource.WithContext(ctx)
			ctx = watcher.WithContext(ctx)
			ctx = ovfcache.WithContext(ctx)

			provider = providerfake.NewVMProvider()
			provider.VSphereClientFn = func(ctx context.Context) (*vsclient.Client, error) {
				return vsclient.NewClient(ctx, vcSimCtx.VCClientConfig)
			}
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				provider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					atomic.AddInt32(&numCreateOrUpdateCalls, 1)
					return nil
				},
			)

			vcSimCtx = builder.NewIntegrationTestContextForVCSim(
				ctx,
				builder.VCSimTestConfig{},
				func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
					if err := vmwatcher.AddToManager(ctx, mgr); err != nil {
						return err
					}
					return virtualmachine.AddToManager(ctx, mgr)
				},
				func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
					ctx.VMProvider = provider
					return nil
				},
				initEnvFn)
			Expect(vcSimCtx).ToNot(BeNil())

			vcSimCtx.BeforeEach()

			objKey = client.ObjectKey{
				Namespace: vcSimCtx.NSInfo.Namespace,
				Name:      vmName,
			}

			ctx = vcSimCtx
		})

		BeforeEach(func() {
			initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
				vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
				Expect(err).ToNot(HaveOccurred())
				Expect(vmList).ToNot(BeEmpty())
				vm = vmList[0]

				By("creating vm in k8s", func() {
					obj = builder.DummyBasicVirtualMachine(
						vmName,
						ctx.NSInfo.Namespace)
					Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
					obj.Status.UniqueID = vm.Reference().Value
					Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
				})

				By("adding namespacedName to vm's extraConfig", func() {
					t, err := vm.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "vmservice.namespacedName",
								Value: ctx.NSInfo.Namespace + "/" + vmName,
							},
						},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(t).ToNot(BeNil())
					Expect(t.Wait(ctx)).To(Succeed())
				})

				By("moving vm into the zone's folder", func() {
					t, err := vm.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
						Folder: ptr.To(vcSimCtx.NSInfo.Folder.Reference()),
					}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
					Expect(err).ToNot(HaveOccurred())
					Expect(t).ToNot(BeNil())
					Expect(t.Wait(ctx)).To(Succeed())
				})
			}
		})

		AfterEach(func() {
			vcSimCtx.AfterEach()
		})

		JustBeforeEach(func() {
			By("wait for VM to have finalizer", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					g.Expect(obj.Finalizers).To(HaveLen(1))
				}).Should(Succeed())
			})

			By("wait for vm to be reconciled once due to the controller starting", func() {
				Eventually(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(1)))
				Consistently(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(1)))
			})
		})

		Specify("vm should be reconciled when change happens on vSphere", func() {
			By("update the vm's extraConfig", func() {
				t, err := vm.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   "guestinfo.ipaddr",
							Value: "1.2.3.4",
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(t).ToNot(BeNil())
				Expect(t.Wait(ctx)).To(Succeed())
			})

			By("wait for vm to be reconciled again, this time by the watcher", func() {
				Eventually(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(2)))
				Consistently(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(2)))
			})
		})

		When("VM has skip-platform-delete annotation", func() {
			JustBeforeEach(func() {
				By("add the skip-platform-delete annotation", func() {
					Eventually(func(g Gomega) {
						g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
						if obj.Annotations == nil {
							obj.Annotations = map[string]string{}
						}
						obj.Annotations[pkgconst.SkipDeletePlatformResourceKey] = ""
						g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
					}).Should(Succeed())
				})
				By("delete the VM", func() {
					Eventually(func(g Gomega) {
						g.Expect(vcSimCtx.Client.Delete(ctx, obj)).To(Succeed())
					}).Should(Succeed())
				})
			})
			It("will delete the Kube VirtualMachine object but not the vSphere VM", func() {
				By("kube vm should be deleted", func() {
					Eventually(func(g Gomega) {
						err := vcSimCtx.Client.Get(ctx, objKey, obj)
						g.Expect(err).To(HaveOccurred())
						g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
					}).Should(Succeed())
				})
				By("vsphere vm should not be deleted", func() {
					Consistently(func(g Gomega) {
						name, err := vm.ObjectName(vcSimCtx)
						Expect(err).ToNot(HaveOccurred())
						Expect(name).ToNot(BeEmpty())
					}).Should(Succeed())
				})
			})
		})
	})
