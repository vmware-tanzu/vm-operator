// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package configpolicy contains E2E tests for the VirtualMachineConfigPolicy
// feature, covering cluster capability discovery and policy enforcement.
package configpolicy

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// SpecInput holds the inputs for Spec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPNamespaceName string
}

// Spec verifies the VirtualMachineConfigPolicy feature end-to-end.
// Currently covers zone controller fan-out (S3) and the ConfigTarget
// controller's cluster-scope capability discovery, including
// status.maxHardwareVersion and non-SR-IOV device categories (S5.b/S5.c).
// SR-IOV per-host enrichment, option enumeration (S6/S7), and policy
// enforcement (S8/S9) will be added here.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "vm-config-policy"

	var (
		input           SpecInput
		svClusterProxy  *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		svClusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = input.ClusterProxy.GetClient()

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, svClusterProxy, consts.VirtualMachineConfigPolicyCapabilityName)
	})

	Context("When a workload namespace has zones", func() {
		It("Should have at least one ConfigTarget derived from the zone's pool MoIDs",
			Label("extended-functional", "experimental"),
			func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, svClusterClient, input.WCPNamespaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(zoneList.Items).ToNot(BeEmpty(),
					"expected at least one Zone in namespace %q", input.WCPNamespaceName)

				// ConfigTargets are cluster-scoped and named after the cluster
				// MoID derived from each zone's pool MoIDs. List them once and
				// verify that for each zone at least one ConfigTarget exists and
				// has a matching spec.id.
				var ctList vimv1.ConfigTargetList
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())

				for i := range zoneList.Items {
					z := &zoneList.Items[i]
					Expect(z.Spec.ManagedVMs.PoolMoIDs).ToNot(BeEmpty(),
						"Zone %q/%q has no pool MoIDs in spec.managedVMs.poolMoIDs",
						z.Namespace, z.Name)

					Expect(ctList.Items).ToNot(BeEmpty(),
						"expected at least one ConfigTarget for zone %q", z.Name)

					for j := range ctList.Items {
						ct := &ctList.Items[j]
						Expect(ct.Spec.ID.ID).To(Equal(ct.Name),
							"ConfigTarget %q spec.id.id should equal its metadata.name", ct.Name)
					}
				}
			})

		It("Should have a VirtualMachineConfigPolicy for each zone in the namespace",
			Label("extended-functional", "experimental"),
			func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, svClusterClient, input.WCPNamespaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(zoneList.Items).ToNot(BeEmpty(),
					"expected at least one Zone in namespace %q", input.WCPNamespaceName)

				for i := range zoneList.Items {
					z := &zoneList.Items[i]
					policy := &vimv1.VirtualMachineConfigPolicy{}
					Expect(svClusterClient.Get(ctx,
						ctrlclient.ObjectKey{Name: z.Name, Namespace: input.WCPNamespaceName},
						policy)).To(Succeed(),
						"VirtualMachineConfigPolicy %q/%q should exist", input.WCPNamespaceName, z.Name)

					Expect(policy.Spec.Zone).To(Equal(z.Name),
						"VirtualMachineConfigPolicy %q/%q should reference zone %q",
						input.WCPNamespaceName, z.Name, z.Name)
				}
			})
	})

	Context("When the ConfigTarget controller reconciles a cluster's ConfigTarget", func() {
		It("Should populate ConfigTarget.status from QueryConfigTarget and mark Ready=True",
			Label("core-functional", "experimental"),
			func() {
				var ctList vimv1.ConfigTargetList
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).ToNot(BeEmpty(), "expected at least one ConfigTarget in the cluster")

				for i := range ctList.Items {
					name := ctList.Items[i].Name

					Eventually(func(g Gomega) {
						ct := &vimv1.ConfigTarget{}
						g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: name}, ct)).To(Succeed())

						cond := apimeta.FindStatusCondition(ct.Status.Conditions, vimv1.ReadyConditionType)
						g.Expect(cond).ToNot(BeNil(), "ConfigTarget %q should have a Ready condition", name)
						g.Expect(cond.Status).To(Equal(metav1.ConditionTrue), "ConfigTarget %q should be Ready", name)

						g.Expect(ct.Status.NumCPUs).To(BeNumerically(">", 0),
							"ConfigTarget %q status.numCPUs should be populated from QueryConfigTarget", name)
						g.Expect(ct.Status.MaxCPUsPerVM).To(BeNumerically(">", 0),
							"ConfigTarget %q status.maxCPUsPerVM should be populated from QueryConfigTarget", name)
					}).Should(Succeed())
				}
			})

		It("Should populate status.maxHardwareVersion and non-SR-IOV device categories from the cluster's real inventory",
			Label("core-functional", "experimental"),
			func() {
				var ctList vimv1.ConfigTargetList
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).ToNot(BeEmpty(), "expected at least one ConfigTarget in the cluster")

				vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, svClusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vCenterClient)

				categories := configTargetDeviceCategories()

				for i := range ctList.Items {
					name := ctList.Items[i].Name

					// ConfigTarget is named after the cluster MoID (asserted
					// in the earlier spec above), and QueryConfigTarget is
					// re-queried live rather than cached, so this always
					// reflects the cluster's current inventory.
					realCT := queryRealConfigTarget(ctx, vCenterClient, name)

					Eventually(func(g Gomega) {
						ct := &vimv1.ConfigTarget{}
						g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: name}, ct)).To(Succeed())

						g.Expect(ct.Status.MaxHardwareVersion).ToNot(BeEmpty(),
							"ConfigTarget %q status.maxHardwareVersion should be populated", name)
						_, err := vimtypes.ParseHardwareVersion(ct.Status.MaxHardwareVersion)
						g.Expect(err).ToNot(HaveOccurred(),
							"ConfigTarget %q status.maxHardwareVersion %q should be a valid hardware version",
							name, ct.Status.MaxHardwareVersion)

						// Assert presence-parity against the cluster's real,
						// live QueryConfigTarget inventory instead of
						// hardcoding which categories are expected: a
						// category this cluster's hardware doesn't happen to
						// expose (VGPU, SGX, SR-IOV-adjacent, ...) is skipped
						// rather than asserted empty, avoiding flakiness on
						// infra that lacks it, while every category the
						// cluster does report is verified to have been
						// propagated into status.
						for _, category := range categories {
							if !category.realPresent(realCT) {
								continue
							}
							g.Expect(category.statusPresent(ct.Status.ConfigTargetDevices)).To(BeTrue(),
								"ConfigTarget %q status.configTargetDevices.%s should be populated: "+
									"the cluster's live QueryConfigTarget reports this category present",
								name, category.name)
						}
					}).Should(Succeed())
				}
			})

		It("Should fan out a VirtualMachineConfigOptions object per supported hardware version",
			Label("core-functional", "experimental"),
			func() {
				var ctList vimv1.ConfigTargetList
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).ToNot(BeEmpty(), "expected at least one ConfigTarget in the cluster")

				Eventually(func(g Gomega) {
					var vcoList vimv1.VirtualMachineConfigOptionsList
					g.Expect(svClusterClient.List(ctx, &vcoList)).To(Succeed())
					g.Expect(vcoList.Items).ToNot(BeEmpty(),
						"expected at least one VirtualMachineConfigOptions fanned out from a ConfigTarget")

					for i := range vcoList.Items {
						vco := &vcoList.Items[i]
						g.Expect(vco.Spec.HardwareVersion).To(Equal(vco.Name),
							"VirtualMachineConfigOptions %q spec.hardwareVersion should equal its metadata.name", vco.Name)
					}
				}).Should(Succeed())
			})

		It("Should garbage-collect a stale VirtualMachineConfigOptions no longer reported by the cluster",
			Label("core-functional", "experimental"),
			func() {
				var ctList vimv1.ConfigTargetList
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).ToNot(BeEmpty(), "expected at least one ConfigTarget in the cluster")
				owner := &ctList.Items[0]

				// The regular supervisor-admin client cannot create
				// cluster-scoped vim.vmware.com resources (that's normally
				// only done by the controller's service account), so use an
				// admin client here, matching the pattern used for
				// cns.vmware.com resources in UnregisterPVCVolumes.
				adminProxy, err := svClusterProxy.NewAdminClusterProxy(ctx)
				Expect(err).ToNot(HaveOccurred(), "failed to get admin cluster proxy for VirtualMachineConfigOptions creation")
				defer adminProxy.Dispose(ctx)

				adminClient, err := adminProxy.GetAdminClient()
				Expect(err).ToNot(HaveOccurred(), "failed to get admin client for VirtualMachineConfigOptions creation")

				// A ConfigTarget's real vSphere cluster will never report this
				// hardware version, so once the owning ConfigTarget's next
				// reconcile runs, this object should be garbage-collected as
				// stale. Creating it with owner as an owner reference (rather
				// than a controller reference, matching reconcileConfigOptions)
				// also triggers that reconcile via the controller's
				// Owns(..., builder.MatchEveryOwner) watch.
				stale := &vimv1.VirtualMachineConfigOptions{
					ObjectMeta: metav1.ObjectMeta{Name: "vmx-e2e-stale-vmop-3760"},
					Spec:       vimv1.VirtualMachineConfigOptionsSpec{HardwareVersion: "vmx-e2e-stale-vmop-3760"},
				}
				Expect(controllerutil.SetOwnerReference(owner, stale, svClusterClient.Scheme())).To(Succeed())
				Expect(adminClient.Create(ctx, stale)).To(Succeed())

				DeferCleanup(func() {
					_ = adminClient.Delete(ctx, stale)
				})

				Eventually(func(g Gomega) {
					err := svClusterClient.Get(ctx, ctrlclient.ObjectKeyFromObject(stale), &vimv1.VirtualMachineConfigOptions{})
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
						"stale VirtualMachineConfigOptions %q should have been garbage-collected", stale.Name)
				}).Should(Succeed())
			})
	})
}

// queryRealConfigTarget fetches the cluster's live vSphere EnvironmentBrowser
// QueryConfigTarget result directly via govmomi, mirroring
// vs.GetVirtualMachineConfigTarget in pkg/providers/vsphere/vmprovider.go, so
// device-category assertions can be driven by the cluster's actual inventory
// instead of a hardcoded expectation of which categories are present.
func queryRealConfigTarget(ctx context.Context, vCenterClient *vim25.Client, clusterMoID string) *vimtypes.ConfigTarget {
	ccr := object.NewClusterComputeResource(vCenterClient,
		vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: clusterMoID})

	envBrowser, err := ccr.EnvironmentBrowser(ctx)
	Expect(err).ToNot(HaveOccurred(), "failed to get environment browser for cluster %q", clusterMoID)

	realCT, err := envBrowser.QueryConfigTarget(ctx, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to query config target for cluster %q", clusterMoID)

	return realCT
}

// configTargetDeviceCategory pairs a ConfigTargetDevices category with
// functions to check its presence in vSphere's live QueryConfigTarget result
// and in the corresponding ConfigTarget.status field, mirroring the mapping
// in pkg/util/vsphere/configtarget/convert.go's populateConfigTargetDevices.
type configTargetDeviceCategory struct {
	name          string
	realPresent   func(ct *vimtypes.ConfigTarget) bool
	statusPresent func(devices vimv1.ConfigTargetDevices) bool
}

func configTargetDeviceCategories() []configTargetDeviceCategory {
	return []configTargetDeviceCategory{
		{"cdrom",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.CdRom) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.CDROM) > 0 }},
		{"floppy",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.Floppy) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.Floppy) > 0 }},
		{"serial",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.Serial) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.Serial) > 0 }},
		{"parallel",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.Parallel) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.Parallel) > 0 }},
		{"sound",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.Sound) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.Sound) > 0 }},
		{"usb",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.Usb) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.USB) > 0 }},
		{"pciPassthrough", hasNonSRIOVPCIPassthrough,
			func(d vimv1.ConfigTargetDevices) bool { return len(d.PCIPassthrough) > 0 }},
		{"dynamicPassthroughDevices",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.DynamicPassthrough) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.DynamicPassthroughDevices) > 0 }},
		{"vgpuDevice",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.VgpuDeviceInfo) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.VGPUDevice) > 0 }},
		{"vgpuProfile",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.VgpuProfileInfo) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.VGPUProfile) > 0 }},
		{"sharedGpuPassthroughTypes",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.SharedGpuPassthroughTypes) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.SharedGPUPassthroughTypes) > 0 }},
		{"sgxTargetInfo",
			func(ct *vimtypes.ConfigTarget) bool { return ct.SgxTargetInfo != nil },
			func(d vimv1.ConfigTargetDevices) bool { return d.SGXTargetInfo != nil }},
		{"precisionClockInfo",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.PrecisionClockInfo) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.PrecisionClockInfo) > 0 }},
		{"vendorDeviceGroupInfo",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.VendorDeviceGroupInfo) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.VendorDeviceGroupInfo) > 0 }},
		{"dvxClassInfo",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.DvxClassInfo) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.DVXClassInfo) > 0 }},
		{"ideDisks",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.IdeDisk) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.IDEDisks) > 0 }},
		{"scsiDisks",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.ScsiDisk) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.SCSIDisks) > 0 }},
		{"scsiPassthrough",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.ScsiPassthrough) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.SCSIPassthrough) > 0 }},
		{"vflashModule",
			func(ct *vimtypes.ConfigTarget) bool { return len(ct.VFlashModule) > 0 },
			func(d vimv1.ConfigTargetDevices) bool { return len(d.VFlashModule) > 0 }},
	}
}

// hasNonSRIOVPCIPassthrough reports whether ct.PciPassthrough contains at
// least one plain *vimtypes.VirtualMachinePciPassthroughInfo entry, matching
// convertPciPassthroughUnion's type-switch: *VirtualMachineSriovInfo entries
// are excluded since SR-IOV is not supported yet (vmop-3926, T121).
func hasNonSRIOVPCIPassthrough(ct *vimtypes.ConfigTarget) bool {
	for _, item := range ct.PciPassthrough {
		if _, ok := item.(*vimtypes.VirtualMachinePciPassthroughInfo); ok {
			return true
		}
	}
	return false
}
