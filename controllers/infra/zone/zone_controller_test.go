// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package zone_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/zone"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	vmwatcher "github.com/vmware-tanzu/vm-operator/services/vm-watcher"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {

		var (
			ctx       context.Context
			vcSimCtx  *builder.IntegrationTestContextForVCSim
			provider  *providerfake.VMProvider
			initEnvFn builder.InitVCSimEnvFn
		)

		BeforeEach(func() {
			ctx = context.Background()
			ctx = logr.NewContext(ctx, testutil.GinkgoLogr(4))
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
				return vsclient.NewClient(ctx, vcSimCtx.VCClientConfig)
			}

			vcSimCtx = builder.NewIntegrationTestContextForVCSim(
				ctx,
				builder.VCSimTestConfig{},
				func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
					if err := vmwatcher.AddToManager(ctx, mgr); err != nil {
						return err
					}
					return zone.AddToManager(ctx, mgr)
				},
				func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
					ctx.VMProvider = provider
					return nil
				},
				initEnvFn)
			Expect(vcSimCtx).ToNot(BeNil())

			vcSimCtx.BeforeEach()

			ctx = vcSimCtx
		})

		AfterEach(func() {
			vcSimCtx.AfterEach()
			vcSimCtx = nil
		})

		When("new zones are added", func() {
			var (
				nsInfo builder.WorkloadNamespaceInfo
			)

			JustBeforeEach(func() {
				nsInfo = vcSimCtx.CreateWorkloadNamespace()

				By("ensure all zones have finalizers", func() {
					Eventually(func(g Gomega) {
						var obj topologyv1.ZoneList
						g.Expect(vcSimCtx.Client.List(ctx, &obj, ctrlclient.InNamespace(nsInfo.Namespace))).To(Succeed())
						g.Expect(obj.Items).To(HaveLen(vcSimCtx.ZoneCount))
						g.Expect(obj.Items).ToNot(BeEmpty())
						for i := range obj.Items {
							g.Expect(obj.Items[i].Finalizers).To(ConsistOf([]string{zone.Finalizer}))
						}
					}).Should(Succeed())
				})
			})

			When("no vms exist in the zone's vm service folder", func() {

				const (
					vmName = "my-vm-1"
				)

				var (
					vm *object.VirtualMachine
				)

				BeforeEach(func() {
					initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
						vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
						Expect(err).ToNot(HaveOccurred())
						Expect(vmList).ToNot(BeEmpty())
						vm = vmList[0]

						dcFolders, err := ctx.Datacenter.Folders(ctx)
						Expect(err).ToNot(HaveOccurred())
						Expect(dcFolders).ToNot(BeNil())
						Expect(dcFolders.VmFolder).ToNot(BeNil())

						By("creating vm in k8s", func() {
							obj := builder.DummyBasicVirtualMachine(
								vmName,
								ctx.NSInfo.Namespace)
							Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
						})

						By("moving vm into the root folder", func() {
							t, err := dcFolders.VmFolder.MoveInto(
								ctx,
								[]vimtypes.ManagedObjectReference{vm.Reference()})
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

				Specify("no reconcile requests should be enqueued for vms", func() {
					chanSource := cource.FromContext(ctx, "VirtualMachine")
					Consistently(chanSource).ShouldNot(Receive())
				})

				When("the vm is relocated into the zone's folder", func() {
					JustBeforeEach(func() {
						t, err := nsInfo.Folder.MoveInto(
							ctx,
							[]vimtypes.ManagedObjectReference{vm.Reference()})
						Expect(err).ToNot(HaveOccurred())
						Expect(t).ToNot(BeNil())
						Expect(t.Wait(ctx)).To(Succeed())
					})
					Specify("a reconcile request should be enqueued for vm", func() {
						chanSource := cource.FromContext(ctx, "VirtualMachine")

						var e event.GenericEvent
						Eventually(chanSource).Should(Receive(&e))
						Expect(e.Object.GetNamespace()).To(Equal(vcSimCtx.NSInfo.Namespace))
						Expect(e.Object.GetName()).To(Equal(vmName))
					})

					When("a single zone with the vm's folder is removed", func() {
						JustBeforeEach(func() {
							var list topologyv1.ZoneList
							Expect(vcSimCtx.Client.List(
								ctx,
								&list,
								ctrlclient.InNamespace(nsInfo.Namespace))).To(Succeed())

							for i := range list.Items {
								z := &list.Items[i]
								if z.Spec.ManagedVMs.FolderMoID == nsInfo.Folder.Reference().Value {
									Expect(vcSimCtx.Client.Delete(ctx, z)).To(Succeed())
									Eventually(func(g Gomega) {
										key := ctrlclient.ObjectKeyFromObject(z)
										g.Expect(vcSimCtx.Client.Get(vcSimCtx, key, z)).ToNot(Succeed())
									}).Should(Succeed())
									break
								}
							}
						})

						Specify("a change to the vm should cause it to be reconciled", func() {
							// Pull any events off of the source channel for 10
							// seconds. This should give the zone enough time to be
							// removed.
							chanSource := cource.FromContext(ctx, "VirtualMachine")
							Eventually(func(g Gomega) {
								g.Expect(chanSource).ToNot(Receive())
							}, time.Second*10).Should(Succeed())

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

							var e event.GenericEvent
							Eventually(chanSource).Should(Receive(&e))
							Expect(e.Object.GetNamespace()).To(Equal(vcSimCtx.NSInfo.Namespace))
							Expect(e.Object.GetName()).To(Equal(vmName))
						})
					})

					When("all zones with the vm's folder are removed", func() {
						JustBeforeEach(func() {
							var list topologyv1.ZoneList
							Expect(vcSimCtx.Client.List(
								ctx,
								&list,
								ctrlclient.InNamespace(nsInfo.Namespace))).To(Succeed())

							for i := range list.Items {
								z := &list.Items[i]
								if z.Spec.ManagedVMs.FolderMoID == nsInfo.Folder.Reference().Value {
									Expect(vcSimCtx.Client.Delete(ctx, z)).To(Succeed())
									Eventually(func(g Gomega) {
										key := ctrlclient.ObjectKeyFromObject(z)
										g.Expect(vcSimCtx.Client.Get(vcSimCtx, key, z)).ToNot(Succeed())
									}).Should(Succeed())
								}
							}
						})

						Specify("a change to the vm should not cause it to be reconciled", func() {
							// Pull any events off of the source channel for 10
							// seconds. This should give the zones enough time to be
							// removed.
							chanSource := cource.FromContext(ctx, "VirtualMachine")
							Eventually(func(g Gomega) {
								g.Expect(chanSource).ToNot(Receive())
							}, time.Second*10).Should(Succeed())

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

							Consistently(chanSource).ShouldNot(Receive())
						})
					})
				})
			})
		})
	})

var _ = Describe(
	"ReconcileNormal",
	Label(
		testlabels.Controller,
		testlabels.API,
	),
	func() {
		const (
			zoneName      = "az-0"
			zoneNamespace = "test-ns"
			clusterMoID1  = "domain-c1"
			clusterMoID2  = "domain-c2"
		)

		var (
			ctx            context.Context
			fakeClient     ctrlclient.Client
			reconciler     *zone.Reconciler
			zoneClusterIDs []string
			featureEnabled bool
		)

		newZone := func() *topologyv1.Zone {
			return &topologyv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:       zoneName,
					Namespace:  zoneNamespace,
					Finalizers: []string{zone.Finalizer},
				},
				Spec: topologyv1.ZoneSpec{
					ManagedVMs: topologyv1.VSphereEntityInfo{
						ClusterMoIDs: zoneClusterIDs,
					},
				},
			}
		}

		BeforeEach(func() {
			featureEnabled = true
			zoneClusterIDs = []string{clusterMoID1}
		})

		JustBeforeEach(func() {
			fakeClient = builder.NewFakeClient(newZone())

			ctx = pkgcfg.WithContext(context.Background(), pkgcfg.Default())
			ctx = pkgcfg.UpdateContext(ctx, func(cfg *pkgcfg.Config) {
				cfg.Features.VirtualMachineConfigPolicy = featureEnabled
			})

			reconciler = zone.NewReconciler(context.Background(), fakeClient, logr.Discard(), nil)
		})

		When("the VirtualMachineConfigPolicy feature is disabled", func() {
			BeforeEach(func() {
				featureEnabled = false
			})

			It("does not create ConfigTarget or VirtualMachineConfigPolicy objects", func() {
				_, err := reconciler.ReconcileNormal(ctx, newZone())
				Expect(err).ToNot(HaveOccurred())

				var ctList vimv1.ConfigTargetList
				Expect(fakeClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).To(BeEmpty())

				var policyList vimv1.VirtualMachineConfigPolicyList
				Expect(fakeClient.List(ctx, &policyList, ctrlclient.InNamespace(zoneNamespace))).To(Succeed())
				Expect(policyList.Items).To(BeEmpty())
			})
		})

		When("the zone has a single cluster MoID", func() {
			It("creates a ConfigTarget for the cluster MoID", func() {
				_, err := reconciler.ReconcileNormal(ctx, newZone())
				Expect(err).ToNot(HaveOccurred())

				var ct vimv1.ConfigTarget
				Expect(fakeClient.Get(ctx, ctrlclient.ObjectKey{Name: clusterMoID1}, &ct)).To(Succeed())
				Expect(ct.Spec.ID.ID).To(Equal(clusterMoID1))
			})

			It("creates a VirtualMachineConfigPolicy for the zone", func() {
				_, err := reconciler.ReconcileNormal(ctx, newZone())
				Expect(err).ToNot(HaveOccurred())

				var policy vimv1.VirtualMachineConfigPolicy
				Expect(fakeClient.Get(ctx,
					ctrlclient.ObjectKey{Name: zoneName, Namespace: zoneNamespace},
					&policy)).To(Succeed())
				Expect(policy.Spec.Zone).To(Equal(zoneName))
				Expect(policy.Spec.SyncMode).To(Equal(vimv1.VirtualMachineConfigPolicySyncModeConfigTarget))
			})

			It("is idempotent across multiple reconciles", func() {
				for range 3 {
					_, err := reconciler.ReconcileNormal(ctx, newZone())
					Expect(err).ToNot(HaveOccurred())
				}

				var ctList vimv1.ConfigTargetList
				Expect(fakeClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).To(HaveLen(1))
				Expect(ctList.Items[0].Spec.ID.ID).To(Equal(clusterMoID1))
			})

			It("does not overwrite SyncMode on an existing policy", func() {
				existing := &vimv1.VirtualMachineConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      zoneName,
						Namespace: zoneNamespace,
					},
					Spec: vimv1.VirtualMachineConfigPolicySpec{
						Zone:     zoneName,
						SyncMode: vimv1.VirtualMachineConfigPolicySyncModeDisabled,
					},
				}
				Expect(fakeClient.Create(ctx, existing)).To(Succeed())

				_, err := reconciler.ReconcileNormal(ctx, newZone())
				Expect(err).ToNot(HaveOccurred())

				var policy vimv1.VirtualMachineConfigPolicy
				Expect(fakeClient.Get(ctx,
					ctrlclient.ObjectKey{Name: zoneName, Namespace: zoneNamespace},
					&policy)).To(Succeed())
				Expect(policy.Spec.SyncMode).To(Equal(vimv1.VirtualMachineConfigPolicySyncModeDisabled))
			})
		})

		When("the zone has multiple cluster MoIDs", func() {
			BeforeEach(func() {
				zoneClusterIDs = []string{clusterMoID1, clusterMoID2}
			})

			It("creates one ConfigTarget per cluster MoID", func() {
				_, err := reconciler.ReconcileNormal(ctx, newZone())
				Expect(err).ToNot(HaveOccurred())

				var ct1 vimv1.ConfigTarget
				Expect(fakeClient.Get(ctx, ctrlclient.ObjectKey{Name: clusterMoID1}, &ct1)).To(Succeed())
				Expect(ct1.Spec.ID.ID).To(Equal(clusterMoID1))

				var ct2 vimv1.ConfigTarget
				Expect(fakeClient.Get(ctx, ctrlclient.ObjectKey{Name: clusterMoID2}, &ct2)).To(Succeed())
				Expect(ct2.Spec.ID.ID).To(Equal(clusterMoID2))
			})
		})

		When("the zone has duplicate cluster MoIDs", func() {
			BeforeEach(func() {
				zoneClusterIDs = []string{clusterMoID1, clusterMoID1, clusterMoID2}
			})

			It("creates one ConfigTarget per unique cluster MoID", func() {
				_, err := reconciler.ReconcileNormal(ctx, newZone())
				Expect(err).ToNot(HaveOccurred())

				var ctList vimv1.ConfigTargetList
				Expect(fakeClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).To(HaveLen(2))
			})
		})
	})
