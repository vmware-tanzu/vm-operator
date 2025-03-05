// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	zonectrl "github.com/vmware-tanzu/vm-operator/controllers/infra/zone"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
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
		testlabels.API,
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
				builder.VCSimTestConfig{},
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
			var (
				oldPort string
			)

			JustBeforeEach(func() {
				vcSimCtx.Suite.StartErrExpected = false

				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeTrue())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(Equal(int32(1)))
				}).Should(Succeed())

				By("invalidate the port", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					oldPort = vcSimCtx.VCClientConfig.Port
					vcSimCtx.VCClientConfig.Port = "1"
				})

				By("log out the client session", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vsClient.Logout(vcSimCtx)
				})
			})

			Specify("the service should be restarted once the port is fixed", func() {
				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(vsClient.Valid()).To(BeFalse())
					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(2)))
				}).Should(Succeed())

				By("fix the port", func() {
					vsClientMu.Lock()
					defer vsClientMu.Unlock()

					vcSimCtx.VCClientConfig.Port = oldPort
				})

				Eventually(func(g Gomega) {
					vsClientMu.RLock()
					defer vsClientMu.RUnlock()

					g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(3)))
				}).Should(Succeed())

				Consistently(func(g Gomega) {
					g.Expect(vcSimCtx.Suite.StartErr()).ToNot(HaveOccurred())
				}).Should(Succeed())
			})
		})

		When("the zones do not have the finalizer", func() {
			Specify("the finalizer gets added", func() {
				Eventually(func(g Gomega) {
					zoneList := topologyv1.ZoneList{}
					err := vcSimCtx.Client.List(vcSimCtx, &zoneList, ctrlclient.InNamespace(vcSimCtx.NSInfo.Namespace))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(zoneList.Items).ToNot(BeEmpty())
					for _, zone := range zoneList.Items {
						g.Expect(zone.Finalizers).To(ConsistOf(zonectrl.Finalizer))
					}
				})
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

				When("a Zone that is being deleted", func() {
					When("the zone has the zone controller finalizer", func() {
						Specify("a reconcile request should be received", func() {
							By("verify that the zone has the finalizer added", func() {
								Eventually(func(g Gomega) {
									zoneList := topologyv1.ZoneList{}
									err := vcSimCtx.Client.List(vcSimCtx, &zoneList, ctrlclient.InNamespace(vcSimCtx.NSInfo.Namespace))
									g.Expect(err).NotTo(HaveOccurred())
									g.Expect(zoneList.Items).ToNot(BeEmpty())
									for _, zone := range zoneList.Items {
										g.Expect(zone.Finalizers).To(ContainElement(zonectrl.Finalizer))
									}
								}).Should(Succeed())
							})

							By("a reconcile request should be received", func() {
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

							By("add a custom finalizer and mark the zone for deletion", func() {
								zoneList := topologyv1.ZoneList{}
								err := vcSimCtx.Client.List(vcSimCtx, &zoneList, ctrlclient.InNamespace(vcSimCtx.NSInfo.Namespace))
								Expect(err).NotTo(HaveOccurred())
								Expect(zoneList.Items).ToNot(BeEmpty())
								for _, zone := range zoneList.Items {
									// Add a finalizer to each zone.
									controllerutil.AddFinalizer(&zone, "foo/bar")
									Expect(vcSimCtx.Client.Update(vcSimCtx, &zone)).To(Succeed())

									// Mark the zone for deletion.
									Expect(vcSimCtx.Client.Delete(vcSimCtx, &zone)).To(Succeed())
								}
							})

							By("restart the watcher", func() {
								Expect(watcher.Close(vcSimCtx)).Should(Succeed())

								Eventually(func(g Gomega) {
									vsClientMu.RLock()
									defer vsClientMu.RUnlock()

									g.Expect(vsClient.Valid()).To(BeTrue())
									g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(2)))
								}).Should(Succeed())
							})

							By("reconfigure the VM", func() {
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

							By("a reconcile request should still be received", func() {
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
						})
					})

					When("the zone does not have the zone controller finalizer", func() {
						Specify("a reconcile request should not be received", func() {
							By("wait for the watcher to start", func() {
								Eventually(func(g Gomega) {
									vsClientMu.RLock()
									defer vsClientMu.RUnlock()

									g.Expect(vsClient.Valid()).To(BeTrue())
									g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(1)))
								}).Should(Succeed())

							})

							By("verify that the zone has the finalizer added", func() {
								Eventually(func(g Gomega) {
									zoneList := topologyv1.ZoneList{}
									err := vcSimCtx.Client.List(vcSimCtx, &zoneList, ctrlclient.InNamespace(vcSimCtx.NSInfo.Namespace))
									g.Expect(err).NotTo(HaveOccurred())
									g.Expect(zoneList.Items).ToNot(BeEmpty())
									for _, zone := range zoneList.Items {
										g.Expect(zone.Finalizers).To(ContainElement(zonectrl.Finalizer))
									}
								}).Should(Succeed())
							})

							By("first reconcile event should be received", func() {
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

							By("add a custom finalizer to the VM, remove the controller finalizer and mark the zone for deletion", func() {
								zoneList := topologyv1.ZoneList{}
								err := vcSimCtx.Client.List(vcSimCtx, &zoneList, ctrlclient.InNamespace(vcSimCtx.NSInfo.Namespace))
								Expect(err).NotTo(HaveOccurred())
								Expect(zoneList.Items).ToNot(BeEmpty())
								for _, zone := range zoneList.Items {
									// Add a custom finalizer
									controllerutil.AddFinalizer(&zone, "foo/bar")
									controllerutil.RemoveFinalizer(&zone, zonectrl.Finalizer)
									Expect(vcSimCtx.Client.Update(vcSimCtx, &zone)).To(Succeed())

									// Mark the zone for deletion
									Expect(vcSimCtx.Client.Delete(vcSimCtx, &zone)).To(Succeed())
								}
							})

							By("close the watcher so all pending property update connections are closed", func() {
								Expect(watcher.Close(vcSimCtx)).Should(Succeed())

								Eventually(func(g Gomega) {
									vsClientMu.RLock()
									defer vsClientMu.RUnlock()

									g.Expect(vsClient.Valid()).To(BeTrue())
									g.Expect(atomic.LoadInt32(&numNewClientCalls)).To(BeNumerically(">=", int32(2)))
								}).Should(Succeed())
							})

							By("reconfigure the VM", func() {
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

							By("a reconcile should not be received", func() {
								chanSource := cource.FromContext(ctx, "VirtualMachine")
								Consistently(chanSource).ShouldNot(Receive())
							})
						})
					})
				})

				When("a bogus Zone Folder MoID", func() {
					BeforeEach(func() {
						outerInitEnvFn := initEnvFn

						initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
							outerInitEnvFn(ctx)

							zone := &topologyv1.Zone{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "bogus",
									Namespace: vcSimCtx.NSInfo.Namespace,
								},
								Spec: topologyv1.ZoneSpec{
									ManagedVMs: topologyv1.VSphereEntityInfo{
										FolderMoID: "group-4242424242",
									},
								},
							}
							Expect(ctx.Client.Create(ctx, zone)).To(Succeed())
						}
					})

					Specify("a reconcile request should still be received", func() {
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

						By("bogus zone should not have finalizer applied", func() {
							zone := topologyv1.Zone{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "bogus",
									Namespace: vcSimCtx.NSInfo.Namespace,
								},
							}
							Expect(vcSimCtx.Client.Get(vcSimCtx, ctrlclient.ObjectKeyFromObject(&zone), &zone)).To(Succeed())
							Expect(zone.Finalizers).To(BeEmpty())
						})
					})
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
