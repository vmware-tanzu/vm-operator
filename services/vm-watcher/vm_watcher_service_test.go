// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmwatcher_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	vmwatcher "github.com/vmware-tanzu/vm-operator/services/vm-watcher"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"Start", Label(
		testlabels.EnvTest,
		testlabels.Service,
		testlabels.V1Alpha3,
	),
	func() {

		var (
			ctx               context.Context
			vcSimCtx          *builder.IntegrationTestContextForVCSim
			provider          *providerfake.VMProvider
			initEnvFn         builder.InitVCSimEnvFn
			vsClientMu        sync.RWMutex
			vsClient          *vsclient.Client
			numNewClientCalls int32
		)

		BeforeEach(func() {
			numNewClientCalls = 0
			vsClient = nil
			vsClientMu = sync.RWMutex{}
			ctx = logr.NewContext(
				context.Background(),
				textlogger.NewLogger(textlogger.NewConfig(
					textlogger.Verbosity(5),
					textlogger.Output(GinkgoWriter),
				)))
		})

		JustBeforeEach(func() {
			ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
			ctx = pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.Features.WorkloadDomainIsolation = true
				},
			)
			ctx = cource.WithContext(ctx)
			ctx = watcher.WithContext(ctx)

			provider = providerfake.NewVMProvider()
			provider.VSphereClientFn = func(ctx context.Context) (*vsclient.Client, error) {
				vsClientMu.Lock()
				defer vsClientMu.Unlock()

				atomic.AddInt32(&numNewClientCalls, 1)
				var err error
				vsClient, err = vsclient.NewClient(ctx, vcSimCtx.VCClientConfig)
				return vsClient, err
			}

			vcSimCtx = builder.NewIntegrationTestContextForVCSim(
				ctx,
				builder.VCSimTestConfig{
					WithWorkloadIsolation: pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation,
				},
				vmwatcher.AddToManager,
				func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
					ctx.VMProvider = provider
					return nil
				},
				initEnvFn)
			Expect(vcSimCtx).ToNot(BeNil())

			vcSimCtx.BeforeEach()
		})

		AfterEach(func() {
			vcSimCtx.AfterEach()
		})

		When("the client is no longer authenticated", func() {
			var (
				oldVSClient *vsclient.Client
			)
			BeforeEach(func() {
				oldVSClient = nil
			})
			JustBeforeEach(func() {
				func() {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					oldVSClient = vsClient
				}()

				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeTrue())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(Equal(int32(1)))
				}).Should(Succeed())

				By("log out the client session", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vsClient.Logout(vcSimCtx)
				})
			})
			Specify("the service should be restarted", func() {
				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeTrue())
					g.Expect(vsClient).ToNot(BeIdenticalTo(oldVSClient))
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(Equal(int32(2)))
				}).Should(Succeed())
			})
		})

		When("the credentials are rotated", func() {
			var (
				oldUser     string
				oldPass     string
				oldVSClient *vsclient.Client
			)
			BeforeEach(func() {
				oldUser = ""
				oldPass = ""
				oldVSClient = nil
			})
			JustBeforeEach(func() {
				By("store the old client", func() {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					oldVSClient = vsClient
				})

				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeTrue())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(Equal(int32(1)))
				}).Should(Succeed())

				By("invalidate the credentials", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					oldUser = vcSimCtx.VCClientConfig.Username
					oldPass = vcSimCtx.VCClientConfig.Password
					vcSimCtx.VCClientConfig.Username = ""
					vcSimCtx.VCClientConfig.Password = ""
				})

				By("log out the client session", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vsClient.Logout(vcSimCtx)
				})
			})

			Specify("the service should be restarted once the password is rotated", func() {
				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeFalse())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(2)))
				}).Should(Succeed())

				By("fix the credentials", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vcSimCtx.VCClientConfig.Username = oldUser
					vcSimCtx.VCClientConfig.Password = oldPass
				})

				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeTrue())
					g.Expect(vsClient).ToNot(BeIdenticalTo(oldVSClient))
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(3)))
				}).Should(Succeed())
			})
		})

		When("the port is invalid", func() {
			JustBeforeEach(func() {
				vcSimCtx.Suite.StartErrExpected = true

				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeTrue())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(Equal(int32(1)))
				}).Should(Succeed())

				By("invalidate the port", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vcSimCtx.VCClientConfig.Port = "1"
				})

				By("log out the client session", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vsClient.Logout(vcSimCtx)
				})
			})

			Specify("the service should fail", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Suite.StartErr()).To(HaveOccurred())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(Equal(int32(2)))
				}).Should(Succeed())
			})
		})

		When("there is a vm in the zone's folder", func() {

			const (
				vmName = "my-vm-1"
			)

			When("the vm has the namespacedName in extraConfig", func() {
				var (
					vm *object.VirtualMachine
				)

				BeforeEach(func() {
					initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
						vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
						Expect(err).ToNot(HaveOccurred())
						Expect(vmList).ToNot(BeEmpty())
						vm = vmList[0]

						By("creating vm in k8s", func() {
							By("creating vm in k8s", func() {
								obj := builder.DummyBasicVirtualMachine(
									vmName,
									ctx.NSInfo.Namespace)
								Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
							})
						})

						By("moving vm into zone's folder", func() {
							t, err := vm.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
								Folder: ptr.To(ctx.NSInfo.Folder.Reference()),
							}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
							Expect(err).ToNot(HaveOccurred())
							Expect(t).ToNot(BeNil())
							Expect(t.Wait(ctx)).To(Succeed())
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
					}
				})

				Specify("a reconcile request should be received", func() {
					chanSource := cource.FromContext(ctx, "VirtualMachine")
					var e event.GenericEvent
					Eventually(chanSource).Should(Receive(&e, Equal(event.GenericEvent{
						Object: &vmopv1.VirtualMachine{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: vcSimCtx.NSInfo.Namespace,
								Name:      vmName,
							},
						},
					})))
				})

				When("the vm is reconfigured", func() {

					Specify("a second reconcile request should be received", func() {
						chanSource := cource.FromContext(ctx, "VirtualMachine")
						var e1 event.GenericEvent
						Eventually(chanSource).Should(Receive(&e1, Equal(event.GenericEvent{
							Object: &vmopv1.VirtualMachine{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: vcSimCtx.NSInfo.Namespace,
									Name:      vmName,
								},
							},
						})))

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

						var e2 event.GenericEvent
						Eventually(chanSource).Should(Receive(&e2, Equal(event.GenericEvent{
							Object: &vmopv1.VirtualMachine{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: vcSimCtx.NSInfo.Namespace,
									Name:      vmName,
								},
							},
						})))
					})

					When("the namespacedName has an invalid namespace", func() {
						Specify("a second reconcile request should not be received", func() {
							chanSource := cource.FromContext(ctx, "VirtualMachine")
							var e1 event.GenericEvent
							Eventually(chanSource).Should(Receive(&e1, Equal(event.GenericEvent{
								Object: &vmopv1.VirtualMachine{
									ObjectMeta: metav1.ObjectMeta{
										Namespace: vcSimCtx.NSInfo.Namespace,
										Name:      vmName,
									},
								},
							})))

							t, err := vm.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: []vimtypes.BaseOptionValue{
									&vimtypes.OptionValue{
										Key:   "guestinfo.ipaddr",
										Value: "1.2.3.4",
									},
									&vimtypes.OptionValue{
										Key:   "vmservice.namespacedName",
										Value: "invalid/" + vmName,
									},
								},
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(t).ToNot(BeNil())
							Expect(t.Wait(ctx)).To(Succeed())

							Consistently(chanSource).ShouldNot(Receive())
						})
					})
				})
			})

			When("the vm does not have the namespacedName in extraConfig", func() {
				When("the vm's status.uniqueID is in the manager's cache", func() {
					BeforeEach(func() {
						initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
							vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
							Expect(err).ToNot(HaveOccurred())
							Expect(vmList).ToNot(BeEmpty())
							vm := vmList[0]

							By("creating vm in k8s", func() {
								obj := builder.DummyBasicVirtualMachine(
									vmName,
									ctx.NSInfo.Namespace)
								Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
								obj.Status.UniqueID = vm.Reference().Value
								Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
							})

							By("moving vm into zone's folder", func() {
								t, err := vm.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
									Folder: ptr.To(ctx.NSInfo.Folder.Reference()),
								}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
								Expect(err).ToNot(HaveOccurred())
								Expect(t).ToNot(BeNil())
								Expect(t.Wait(ctx)).To(Succeed())
							})
						}
					})

					Specify("a reconcile request should not be received because the VM entered the watcher's scope already verified", func() {
						chanSource := cource.FromContext(ctx, "VirtualMachine")
						Consistently(chanSource, time.Second*5).ShouldNot(Receive())
					})

					When("the vm's k8s object is being deleted", func() {
						BeforeEach(func() {
							initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
								vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
								Expect(err).ToNot(HaveOccurred())
								Expect(vmList).ToNot(BeEmpty())
								vm := vmList[0]
								obj := builder.DummyBasicVirtualMachine(
									vmName,
									ctx.NSInfo.Namespace)

								By("creating vm in k8s", func() {
									Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
									obj.Status.UniqueID = vm.Reference().Value
									Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
								})

								By("deleting vm in k8s", func() {
									// Add a fake finalizer to prevent the VM
									// from being removed entirely. We want the
									// VM to exist with a non-zero deletion
									// time stamp.
									obj.Finalizers = []string{"fake.com/finalizer"}
									Expect(ctx.Client.Update(ctx, obj)).To(Succeed())
									Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
								})

								By("moving vm into zone's folder", func() {
									t, err := vm.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
										Folder: ptr.To(ctx.NSInfo.Folder.Reference()),
									}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
									Expect(err).ToNot(HaveOccurred())
									Expect(t).ToNot(BeNil())
									Expect(t.Wait(ctx)).To(Succeed())
								})
							}
						})

						Specify("a reconcile request should not be received because the VM is being deleted", func() {
							chanSource := cource.FromContext(ctx, "VirtualMachine")
							Consistently(chanSource, time.Second*5).ShouldNot(Receive())
						})
					})
				})

				When("the vm's status.uniqueID is not in the manager's cache", func() {
					BeforeEach(func() {
						initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
							vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
							Expect(err).ToNot(HaveOccurred())
							Expect(vmList).ToNot(BeEmpty())
							vm := vmList[0]

							By("creating vm in k8s", func() {
								obj := builder.DummyBasicVirtualMachine(
									vmName,
									ctx.NSInfo.Namespace)
								Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
							})

							By("moving vm into zone's folder", func() {
								t, err := vm.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
									Folder: ptr.To(ctx.NSInfo.Folder.Reference()),
								}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
								Expect(err).ToNot(HaveOccurred())
								Expect(t).ToNot(BeNil())
								Expect(t.Wait(ctx)).To(Succeed())
							})
						}
					})
					Specify("a reconcile request should not be received", func() {
						chanSource := cource.FromContext(ctx, "VirtualMachine")
						Consistently(chanSource, time.Second*5).ShouldNot(Receive())
					})
				})
			})
		})
	})
