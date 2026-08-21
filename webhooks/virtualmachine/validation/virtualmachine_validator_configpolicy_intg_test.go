// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const intgConfigPolicyZoneName = "config-policy-zone"

var _ = Describe(
	"VirtualMachineConfigPolicy webhook enforcement",
	Label(
		testlabels.Update,
		testlabels.EnvTest,
		testlabels.API,
		testlabels.Validation,
		testlabels.Webhook,
	),
	func() {
		var (
			ctx    *intgValidatingWebhookContext
			zone   *topologyv1.Zone
			policy *vimv1.VirtualMachineConfigPolicy
		)

		BeforeEach(func() {
			ctx = newIntgValidatingWebhookContext()

			ctx.vm.Labels[corev1.LabelTopologyZone] = intgConfigPolicyZoneName

			// validateAvailabilityZone (a separate, pre-existing check)
			// requires the zone label to reference a real Zone; this
			// policy test suite has no interest in its contents.
			zone = &topologyv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      intgConfigPolicyZoneName,
					Namespace: ctx.Namespace,
				},
			}
			Expect(ctx.Client.Create(ctx, zone)).To(Succeed())

			policy = &vimv1.VirtualMachineConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      intgConfigPolicyZoneName,
					Namespace: ctx.Namespace,
				},
				Spec: vimv1.VirtualMachineConfigPolicySpec{
					Zone:        intgConfigPolicyZoneName,
					CreateMode:  vimv1.VirtualMachineConfigPolicyModeDeny,
					UpdateMode:  vimv1.VirtualMachineConfigPolicyModeDeny,
					PowerOnMode: vimv1.VirtualMachineConfigPolicyModeDeny,
					VMClassMode: vimv1.VirtualMachineConfigPolicyVMClassModeAsConfig,
				},
			}
			Expect(ctx.Client.Create(ctx, policy)).To(Succeed())
		})

		AfterEach(func() {
			err := ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, policy))
			Expect(err).To(Succeed())
			Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, zone))).To(Succeed())

			ctx.AfterEach()
			ctx = nil
		})

		setExtraConfigDenied := func(key string) {
			policyKey := ctrlclient.ObjectKeyFromObject(policy)
			Expect(ctx.Client.Get(ctx, policyKey, policy)).To(Succeed())
			policy.Spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
				Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
					{Type: vimv1.MatchTypeFixed, Key: key},
				},
			}
			Expect(ctx.Client.Update(ctx, policy)).To(Succeed())
		}

		setMaxHardwareVersion := func(maxHardwareVersion string) {
			policyKey := ctrlclient.ObjectKeyFromObject(policy)
			Expect(ctx.Client.Get(ctx, policyKey, policy)).To(Succeed())
			policy.Spec.HardwareVersions = &vimv1.HardwareVersionRange{
				Max: vimv1.MustParseHardwareVersion(maxHardwareVersion),
			}
			Expect(ctx.Client.Update(ctx, policy)).To(Succeed())
		}

		Context("create", func() {
			It("allows a create request that does not violate the policy", func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("rejects a create request whose extraConfig key is denied", func() {
				setExtraConfigDenied("guestinfo.foo")
				ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.foo", Value: "bar"},
					},
				}

				err := ctx.Client.Create(ctx, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(
					ContainSubstring("denied by the namespace's VirtualMachineConfigPolicy"))
			})

			It("allows a create request denied by the policy when createMode=Allow",
				func() {
					setExtraConfigDenied("guestinfo.foo")
					policyKey := ctrlclient.ObjectKeyFromObject(policy)
					Expect(ctx.Client.Get(ctx, policyKey, policy)).To(Succeed())
					policy.Spec.CreateMode = vimv1.VirtualMachineConfigPolicyModeAllow
					policy.Spec.PowerOnMode = vimv1.VirtualMachineConfigPolicyModeAllow
					Expect(ctx.Client.Update(ctx, policy)).To(Succeed())

					ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						ExtraConfig: []vmopv1common.KeyValuePair{
							{Key: "guestinfo.foo", Value: "bar"},
						},
					}

					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
				})

			It("rejects a create request whose minHardwareVersion exceeds "+
				"hardwareVersions.max", func() {
				setMaxHardwareVersion("vmx-19")
				ctx.vm.Spec.MinHardwareVersion = 21

				err := ctx.Client.Create(ctx, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(
					ContainSubstring("exceeds the maximum hardware version vmx-19"))
			})

			It("allows a create request within hardwareVersions.max", func() {
				setMaxHardwareVersion("vmx-21")
				ctx.vm.Spec.MinHardwareVersion = 21

				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("bypasses the policy for VM Class config, vmClassMode=AsPolicy", func() {
				setExtraConfigDenied("guestinfo.foo")
				policyKey := ctrlclient.ObjectKeyFromObject(policy)
				Expect(ctx.Client.Get(ctx, policyKey, policy)).To(Succeed())
				policy.Spec.VMClassMode =
					vimv1.VirtualMachineConfigPolicyVMClassModeAsPolicy
				Expect(ctx.Client.Update(ctx, policy)).To(Succeed())

				ctx.vm.Spec.ClassName = builder.DummyClassName
				ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.foo", Value: "bar"},
					},
				}

				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("allows a create request when no policy governs the VM's zone", func() {
				Expect(ctx.Client.Delete(ctx, policy)).To(Succeed())

				ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.foo", Value: "bar"},
					},
				}

				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})
		})

		Context("update", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("rejects an update whose extraConfig key is denied", func() {
				setExtraConfigDenied("guestinfo.foo")
				ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.foo", Value: "bar"},
					},
				}

				err := ctx.Client.Update(ctx, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(
					ContainSubstring("denied by the namespace's VirtualMachineConfigPolicy"))
			})

			It("allows an update denied by the policy when updateMode=Allow", func() {
				setExtraConfigDenied("guestinfo.foo")
				policyKey := ctrlclient.ObjectKeyFromObject(policy)
				Expect(ctx.Client.Get(ctx, policyKey, policy)).To(Succeed())
				policy.Spec.UpdateMode = vimv1.VirtualMachineConfigPolicyModeAllow
				Expect(ctx.Client.Update(ctx, policy)).To(Succeed())

				ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.foo", Value: "bar"},
					},
				}

				Expect(ctx.Client.Update(ctx, ctx.vm)).To(Succeed())
			})

			It("rejects power-on via powerOnMode even when updateMode=Allow", func() {
				vmKey := ctrlclient.ObjectKeyFromObject(ctx.vm)
				Expect(ctx.Client.Get(ctx, vmKey, ctx.vm)).To(Succeed())
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(ctx.Client.Update(ctx, ctx.vm)).To(Succeed())

				setExtraConfigDenied("guestinfo.foo")
				policyKey := ctrlclient.ObjectKeyFromObject(policy)
				Expect(ctx.Client.Get(ctx, policyKey, policy)).To(Succeed())
				policy.Spec.UpdateMode = vimv1.VirtualMachineConfigPolicyModeAllow
				Expect(ctx.Client.Update(ctx, policy)).To(Succeed())

				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.foo", Value: "bar"},
					},
				}

				err := ctx.Client.Update(ctx, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(
					ContainSubstring("denied by the namespace's VirtualMachineConfigPolicy"))
			})
		})
	},
)
