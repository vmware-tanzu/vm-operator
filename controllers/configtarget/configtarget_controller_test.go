// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configtarget_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/controllers/configtarget"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const clusterMoID = "domain-c9"

var _ = Describe("ConfigTarget Controller", Label(testlabels.Controller, testlabels.API), unitTests)

func unitTests() {
	var (
		ctx        context.Context
		client     ctrlclient.Client
		reconciler *configtarget.Reconciler
		provider   *providerfake.VMProvider
		obj        *vimv1.ConfigTarget
		objReq     ctrl.Request

		withObjs []ctrlclient.Object

		configTargetResult *vimtypes.ConfigTarget
		descriptorsResult  []vimtypes.VirtualMachineConfigOptionDescriptor
		queryErr           error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		provider = providerfake.NewVMProvider()

		withObjs = nil

		obj = &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterMoID,
			},
			Spec: vimv1.ConfigTargetSpec{
				ID: vimv1.ManagedObjectID{ID: clusterMoID},
			},
		}
		objReq = ctrl.Request{
			NamespacedName: types.NamespacedName{Name: obj.Name},
		}

		sevTrue := true
		configTargetResult = &vimtypes.ConfigTarget{
			NumCpus:                8,
			NumCpuCores:            4,
			NumNumaNodes:           2,
			MaxCpusPerHost:         64,
			MaxSimultaneousThreads: 2,
			SmcPresent:             false,
			MaxMemMBOptimalPerf:    4096,
			SupportedMaxMemMB:      8192,
			SevSupported:           &sevTrue,
		}
		descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
			{Key: "vmx-19"},
			{Key: "vmx-20"},
		}
		queryErr = nil
	})

	JustBeforeEach(func() {
		provider.GetVirtualMachineConfigTargetFn = func(
			_ context.Context, gotClusterMoID string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
			Expect(gotClusterMoID).To(Equal(clusterMoID))
			return configTargetResult, descriptorsResult, queryErr
		}

		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vimv1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&vimv1.ConfigTarget{}).
			WithObjects(withObjs...).
			Build()

		reconciler = configtarget.NewReconciler(
			ctx,
			client,
			log.Log.WithName("configtarget"),
			record.New(events.NewFakeRecorder(100)),
			provider)
	})

	When("the ConfigTarget does not exist", func() {
		It("returns without error", func() {
			result, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	When("the ConfigTarget exists", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, obj)
		})

		When("the vSphere queries succeed", func() {
			It("populates status, sets Ready=True, and fans out VirtualMachineConfigOptions", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())

				Expect(got.Status.NumCPUs).To(Equal(int32(8)))
				Expect(got.Status.NumCPUCores).To(Equal(int32(4)))
				Expect(got.Status.NumNumaNodes).To(Equal(int32(2)))
				Expect(got.Status.MaxCPUsPerVM).To(Equal(int32(64)))
				Expect(got.Status.MaxSimultaneousThreads).To(Equal(int32(2)))
				Expect(got.Status.SMCPresent).To(BeFalse())
				Expect(got.Status.SEVSupported).To(BeTrue())
				Expect(got.Status.SEVSNPSupported).To(BeFalse())
				Expect(got.Status.TDXSupported).To(BeFalse())
				Expect(got.Status.MaxMemOptimalPerf).ToNot(BeNil())
				Expect(got.Status.MaxMemOptimalPerf.Value()).To(Equal(int64(4096) * 1024 * 1024))
				Expect(got.Status.SupportedMaxMem).ToNot(BeNil())
				Expect(got.Status.SupportedMaxMem.Value()).To(Equal(int64(8192) * 1024 * 1024))

				// TODO(vmop-3759): devices (including SR-IOV) are left unset
				// until per-host discovery lands.
				Expect(got.Status.ConfigTargetDevices.SRIOV).To(BeEmpty())
				Expect(got.Status.ConfigTargetDevices.CDROM).To(BeEmpty())

				Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
				Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())

				var vmcoList vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &vmcoList)).To(Succeed())
				Expect(vmcoList.Items).To(HaveLen(2))

				names := make([]string, len(vmcoList.Items))
				for i, item := range vmcoList.Items {
					names[i] = item.Name
					Expect(item.Spec.HardwareVersion).To(Equal(item.Name))
					Expect(item.OwnerReferences).To(HaveLen(1))
					Expect(item.OwnerReferences[0].UID).To(Equal(got.UID))
				}

				Expect(names).To(ConsistOf("vmx-19", "vmx-20"))
			})

			It("is idempotent on a second reconcile with unchanged vSphere state", func() {
				_, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())

				var before vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &before)).To(Succeed())

				resourceVersions := map[string]string{}
				for _, item := range before.Items {
					resourceVersions[item.Name] = item.ResourceVersion
				}

				_, err = reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())

				var after vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &after)).To(Succeed())
				Expect(after.Items).To(HaveLen(len(before.Items)))

				for _, item := range after.Items {
					Expect(item.ResourceVersion).To(Equal(resourceVersions[item.Name]),
						"expected no spurious patch for %s", item.Name)
				}
			})

			When("a hardware version is dropped from the descriptor list", func() {
				It("garbage-collects the corresponding VirtualMachineConfigOptions", func() {
					_, err := reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var before vimv1.VirtualMachineConfigOptionsList
					Expect(client.List(ctx, &before)).To(Succeed())
					Expect(before.Items).To(HaveLen(2))

					descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
						{Key: "vmx-20"},
					}
					provider.GetVirtualMachineConfigTargetFn = func(
						_ context.Context, _ string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
						return configTargetResult, descriptorsResult, nil
					}

					_, err = reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var after vimv1.VirtualMachineConfigOptionsList
					Expect(client.List(ctx, &after)).To(Succeed())
					Expect(after.Items).To(HaveLen(1))
					Expect(after.Items[0].Name).To(Equal("vmx-20"))
				})
			})

			When("a VirtualMachineConfigOptions is co-owned by another ConfigTarget", func() {
				var otherObj *vimv1.ConfigTarget

				BeforeEach(func() {
					otherObj = &vimv1.ConfigTarget{
						ObjectMeta: metav1.ObjectMeta{Name: "domain-c-other"},
						Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: "domain-c-other"}},
					}
					withObjs = append(withObjs, otherObj)
				})

				It("only removes its own owner reference and leaves the object when another owner remains", func() {
					// otherObj always reports both vmx-19 and vmx-20; obj's
					// live set is driven by descriptorsResult, mutated below.
					provider.GetVirtualMachineConfigTargetFn = func(
						_ context.Context, gotClusterMoID string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
						if gotClusterMoID == otherObj.Name {
							return configTargetResult, []vimtypes.VirtualMachineConfigOptionDescriptor{
								{Key: "vmx-19"}, {Key: "vmx-20"},
							}, nil
						}
						return configTargetResult, descriptorsResult, queryErr
					}

					// Reconcile the "other" cluster first so it takes an
					// owner reference on the shared vmx-19/vmx-20 objects.
					otherReconciler := configtarget.NewReconciler(
						ctx,
						client,
						log.Log.WithName("configtarget"),
						record.New(events.NewFakeRecorder(100)),
						provider)
					_, err := otherReconciler.Reconcile(ctx, ctrl.Request{
						NamespacedName: types.NamespacedName{Name: otherObj.Name},
					})
					Expect(err).ToNot(HaveOccurred())

					_, err = reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					var vmco vimv1.VirtualMachineConfigOptions
					Expect(client.Get(ctx, types.NamespacedName{Name: "vmx-19"}, &vmco)).To(Succeed())
					Expect(vmco.OwnerReferences).To(HaveLen(2))

					// Now this ConfigTarget drops vmx-19 from its live set.
					descriptorsResult = []vimtypes.VirtualMachineConfigOptionDescriptor{
						{Key: "vmx-20"},
					}

					_, err = reconciler.Reconcile(ctx, objReq)
					Expect(err).ToNot(HaveOccurred())

					// vmx-19 must still exist: otherObj still owns it.
					Expect(client.Get(ctx, types.NamespacedName{Name: "vmx-19"}, &vmco)).To(Succeed())
					Expect(vmco.OwnerReferences).To(HaveLen(1))
					Expect(vmco.OwnerReferences[0].UID).To(Equal(otherObj.UID))
				})
			})
		})

		When("the cluster cannot be resolved", func() {
			BeforeEach(func() {
				notFound := &vimtypes.ManagedObjectNotFound{
					Obj: vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: clusterMoID},
				}
				queryErr = fmt.Errorf("cluster %q not found: %w", clusterMoID, soap.WrapVimFault(notFound))
			})

			It("marks Ready=False with a ClusterNotFound-style reason and does not create VirtualMachineConfigOptions", func() {
				_, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).To(HaveOccurred())

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
				Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
				Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).To(Equal(configtarget.ClusterNotFoundReason))

				var vmcoList vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &vmcoList)).To(Succeed())
				Expect(vmcoList.Items).To(BeEmpty())
			})
		})

		When("a transient error occurs querying vSphere", func() {
			BeforeEach(func() {
				queryErr = errors.New("connection reset by peer")
			})

			It("marks Ready=False with a distinct reason, runs no GC, and signals a retry", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection reset by peer"))
				Expect(result).To(Equal(ctrl.Result{}))

				var got vimv1.ConfigTarget
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
				Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
				Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).To(Equal(configtarget.QueryConfigTargetFailedReason))
				Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).ToNot(Equal(configtarget.ClusterNotFoundReason))

				var vmcoList vimv1.VirtualMachineConfigOptionsList
				Expect(client.List(ctx, &vmcoList)).To(Succeed())
				Expect(vmcoList.Items).To(BeEmpty())
			})
		})
	})
}

var _ = Describe("ConfigTarget Controller against vcsim",
	Label(testlabels.Controller, testlabels.API, testlabels.EnvTest, testlabels.VCSim),
	vcsimTests)

func vcsimTests() {
	var (
		vcsimCtx   *builder.TestContextForVCSim
		reconciler *configtarget.Reconciler
		obj        *vimv1.ConfigTarget
		objReq     ctrl.Request
	)

	BeforeEach(func() {
		vcsimCtx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		provider := vsphere.NewVSphereVMProviderFromClient(vcsimCtx, vcsimCtx.Client, vcsimCtx.Recorder)

		reconciler = configtarget.NewReconciler(
			vcsimCtx,
			vcsimCtx.Client,
			log.Log.WithName("configtarget"),
			vcsimCtx.Recorder,
			provider)
	})

	AfterEach(func() {
		vcsimCtx.AfterEach()
	})

	When("the ConfigTarget names a real vcsim cluster", func() {
		BeforeEach(func() {
			ccr := vcsimCtx.GetFirstClusterFromFirstZone()
			Expect(ccr).ToNot(BeNil())

			obj = &vimv1.ConfigTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name: ccr.Reference().Value,
				},
				Spec: vimv1.ConfigTargetSpec{
					ID: vimv1.ManagedObjectID{ID: ccr.Reference().Value},
				},
			}
			Expect(vcsimCtx.Client.Create(vcsimCtx, obj)).To(Succeed())

			objReq = ctrl.Request{
				NamespacedName: types.NamespacedName{Name: obj.Name},
			}
		})

		It("populates status and creates VirtualMachineConfigOptions from the real EnvironmentBrowser result", func() {
			_, err := reconciler.Reconcile(vcsimCtx, objReq)
			Expect(err).ToNot(HaveOccurred())

			var got vimv1.ConfigTarget
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
			Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(got.Status.NumCPUs).To(BeNumerically(">", 0))

			var vmcoList vimv1.VirtualMachineConfigOptionsList
			Expect(vcsimCtx.Client.List(vcsimCtx, &vmcoList)).To(Succeed())
			Expect(vmcoList.Items).ToNot(BeEmpty())

			// SKIP: a GC-via-vcsim-mutation scenario (dropping a previously
			// reported hardware version key at runtime) is not exercised here.
			// vcsim's EnvironmentBrowser reports a fixed, version-derived set
			// of ConfigOptionDescriptor keys that cannot be shrunk without
			// restarting the simulator, so this behavior is only covered by
			// the fake-provider-backed unit test above.
		})
	})

	When("the ConfigTarget names a cluster that does not exist", func() {
		BeforeEach(func() {
			obj = &vimv1.ConfigTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name: "domain-c-does-not-exist",
				},
				Spec: vimv1.ConfigTargetSpec{
					ID: vimv1.ManagedObjectID{ID: "domain-c-does-not-exist"},
				},
			}
			Expect(vcsimCtx.Client.Create(vcsimCtx, obj)).To(Succeed())

			objReq = ctrl.Request{
				NamespacedName: types.NamespacedName{Name: obj.Name},
			}
		})

		It("marks Ready=False and does not panic", func() {
			Expect(func() {
				_, _ = reconciler.Reconcile(vcsimCtx, objReq)
			}).ToNot(Panic())

			var got vimv1.ConfigTarget
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())
			Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
		})
	})
}
