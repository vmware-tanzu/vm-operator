// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

func uniTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		unitTestsMutating,
	)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	vm *vmopv1.VirtualMachine
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vm := builder.DummyVirtualMachine()
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vm:                                vm,
	}
}

func unitTestsMutating() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
	})

	Describe("VirtualMachineMutator should admit updates when object is under deletion", func() {
		Context("when update request comes in while deletion in progress ", func() {
			It("should admit update operation", func() {
				t := metav1.Now()
				ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Describe("AddDefaultNetworkInterface", func() {

		Context("When VM Network is nil", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Network = nil
			})

			// Just any network is OK here - just checking that we don't NPE.
			When("VDS network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
					})
				})

				It("Should add default network interface with type vsphere-distributed", func() {
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("Network"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("netoperator.vmware.com/v1alpha1"))
				})
			})
		})

		Context("When VM Network.Interfaces is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{}
			})

			When("VDS network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
					})
				})

				It("Should add default network interface with type vsphere-distributed", func() {
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("Network"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("netoperator.vmware.com/v1alpha1"))
				})
			})

			When("NSX-T network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeNSXT
					})
				})

				It("Should add default network interface with type NSX-T", func() {
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("VirtualNetwork"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("vmware.com/v1alpha1"))
				})
			})

			When("VPC network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVPC
					})
				})

				It("Should add default network interface with type SubnetSet", func() {
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("SubnetSet"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("crd.nsx.vmware.com/v1alpha1"))
				})
			})

			When("Named network", func() {
				const networkName = "VM Network"

				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeNamed
					})
				})

				It("Should add default network interface with name set in the configMap Network", func() {
					configMap := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: ctx.Namespace,
							Name:      config.ProviderConfigMapName,
						},
						Data: map[string]string{"Network": networkName},
					}
					Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())

					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).To(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).To(BeEmpty())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).To(BeEmpty())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Name).To(Equal(networkName))
				})
			})

			When("NoNetwork annotation is set", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
					})
				})

				BeforeEach(func() {
					ctx.vm.Annotations[v1alpha1.NoDefaultNicAnnotation] = "true"
				})

				It("Should not add default network interface", func() {
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeFalse())
					Expect(ctx.vm.Spec.Network.Interfaces).To(BeEmpty())
				})

				Context("Interfaces is not empty", func() {
					BeforeEach(func() {
						ctx.vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name: "eth0",
							},
						}
					})

					It("back fills Network ref", func() {
						Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
						Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
						Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("Network"))
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("netoperator.vmware.com/v1alpha1"))
					})
				})
			})
		})

		Context("VM Network.Interfaces is not empty", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
				})
			})

			It("Should set default network for interfaces that don't have it set", func() {
				ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
						},
						{
							Name:    "eth1",
							Network: &common.PartialObjectRef{},
						},
						{
							Name: "eth2",
							Network: &common.PartialObjectRef{
								Name: "do-not-change-this-network-name",
							},
						},
					},
				}

				Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
				Expect(ctx.vm.Spec.Network.Interfaces).To(HaveLen(3))

				iface0 := &ctx.vm.Spec.Network.Interfaces[0]
				Expect(iface0.Name).To(Equal("eth0"))
				Expect(iface0.Network).ToNot(BeNil())
				Expect(iface0.Network.Name).To(BeEmpty())
				Expect(iface0.Network.Kind).To(Equal("Network"))
				Expect(iface0.Network.APIVersion).To(Equal("netoperator.vmware.com/v1alpha1"))

				iface1 := &ctx.vm.Spec.Network.Interfaces[1]
				Expect(iface1.Name).To(Equal("eth1"))
				Expect(iface1.Network).ToNot(BeNil())
				Expect(iface1.Network.Name).To(BeEmpty())
				Expect(iface1.Network.Kind).To(Equal("Network"))
				Expect(iface1.Network.APIVersion).To(Equal("netoperator.vmware.com/v1alpha1"))

				iface2 := &ctx.vm.Spec.Network.Interfaces[2]
				Expect(iface2.Name).To(Equal("eth2"))
				Expect(iface2.Network).ToNot(BeNil())
				Expect(iface2.Network.Name).To(Equal("do-not-change-this-network-name"))
				Expect(iface2.Network.Kind).To(Equal("Network"))
				Expect(iface2.Network.APIVersion).To(Equal("netoperator.vmware.com/v1alpha1"))
			})
		})
	})

	Describe("SetDefaultNetworkOnUpdate", func() {

		Context("When VM Network is nil", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Network = nil
			})

			// Just any network is OK here - just checking that we don't NPE.
			When("VDS network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
					})
				})

				It("Should not update VM", func() {
					Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeFalse())
					Expect(ctx.vm.Spec.Network).To(BeNil())
				})
			})
		})

		Context("When interface is added without network set", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
					},
				}
			})

			When("VDS network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
					})
				})

				It("Should set network ref with type vsphere-distributed", func() {
					Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("Network"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("netoperator.vmware.com/v1alpha1"))
				})
			})

			When("NSX-T network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeNSXT
					})
				})

				It("Should set network ref with type NSX-T", func() {
					Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("VirtualNetwork"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("vmware.com/v1alpha1"))
				})
			})

			When("VPC network", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeVPC
					})
				})

				It("Should set network ref with type SubnetSet", func() {
					Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Name).Should(Equal("eth0"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).Should(Equal("SubnetSet"))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).Should(Equal("crd.nsx.vmware.com/v1alpha1"))
				})
			})

			When("Named network", func() {
				const networkName = "VM Network"

				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.NetworkProviderType = pkgcfg.NetworkProviderTypeNamed
					})

					configMap := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: ctx.Namespace,
							Name:      config.ProviderConfigMapName,
						},
						Data: map[string]string{"Network": networkName},
					}
					Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())
				})

				It("Should set network ref with name set in the configMap Network", func() {
					Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.Network.Interfaces).To(HaveLen(1))
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).To(BeEmpty())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).To(BeEmpty())
					Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Name).To(Equal(networkName))
				})

				Context("Network ref is partial set", func() {
					BeforeEach(func() {
						ctx.vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name:    "eth0",
								Network: &common.PartialObjectRef{},
							},
							{
								Name: "eth1",
								Network: &common.PartialObjectRef{
									Name: "my-network",
								},
							},
						}

					})

					It("Should update network ref", func() {
						Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
						Expect(ctx.vm.Spec.Network.Interfaces).To(HaveLen(2))

						Expect(ctx.vm.Spec.Network.Interfaces[0].Name).To(Equal("eth0"))
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network).ShouldNot(BeNil())
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Kind).To(BeEmpty())
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network.APIVersion).To(BeEmpty())
						Expect(ctx.vm.Spec.Network.Interfaces[0].Network.Name).To(Equal(networkName))

						Expect(ctx.vm.Spec.Network.Interfaces[1].Name).To(Equal("eth1"))
						Expect(ctx.vm.Spec.Network.Interfaces[1].Network).ShouldNot(BeNil())
						Expect(ctx.vm.Spec.Network.Interfaces[1].Network.Kind).To(BeEmpty())
						Expect(ctx.vm.Spec.Network.Interfaces[1].Network.APIVersion).To(BeEmpty())
						Expect(ctx.vm.Spec.Network.Interfaces[1].Network.Name).To(Equal("my-network"))
					})
				})
			})
		})

		Context("VM Network.Interfaces is not empty", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
				})
			})

			It("Should set default network for interfaces that don't have it set", func() {
				ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
						},
						{
							Name:    "eth1",
							Network: &common.PartialObjectRef{},
						},
						{
							Name: "eth2",
							Network: &common.PartialObjectRef{
								Name: "do-not-change-this-network-name",
							},
						},
					},
				}

				Expect(mutation.SetDefaultNetworkOnUpdate(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
				Expect(ctx.vm.Spec.Network.Interfaces).To(HaveLen(3))

				iface0 := &ctx.vm.Spec.Network.Interfaces[0]
				Expect(iface0.Name).To(Equal("eth0"))
				Expect(iface0.Network).ToNot(BeNil())
				Expect(iface0.Network.Name).To(BeEmpty())
				Expect(iface0.Network.Kind).To(Equal("Network"))
				Expect(iface0.Network.APIVersion).To(Equal("netoperator.vmware.com/v1alpha1"))

				iface1 := &ctx.vm.Spec.Network.Interfaces[1]
				Expect(iface1.Name).To(Equal("eth1"))
				Expect(iface1.Network).ToNot(BeNil())
				Expect(iface1.Network.Name).To(BeEmpty())
				Expect(iface1.Network.Kind).To(Equal("Network"))
				Expect(iface1.Network.APIVersion).To(Equal("netoperator.vmware.com/v1alpha1"))

				iface2 := &ctx.vm.Spec.Network.Interfaces[2]
				Expect(iface2.Name).To(Equal("eth2"))
				Expect(iface2.Network).ToNot(BeNil())
				Expect(iface2.Network.Name).To(Equal("do-not-change-this-network-name"))
				Expect(iface2.Network.Kind).To(Equal("Network"))
				Expect(iface2.Network.APIVersion).To(Equal("netoperator.vmware.com/v1alpha1"))
			})
		})
	})

	Describe("SetDefaultPowerState", func() {

		Context("When VM PowerState is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = ""
			})

			It("Should set PowerState to PoweredOn", func() {
				Expect(mutation.SetDefaultPowerState(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
				Expect(ctx.vm.Spec.PowerState).Should(Equal(vmopv1.VirtualMachinePowerStateOn))
			})

		})

		Context("When VM PowerState is not empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})

			It("Should not mutate PowerState", func() {
				oldVM := ctx.vm.DeepCopy()
				Expect(mutation.SetDefaultPowerState(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeFalse())
				Expect(ctx.vm.Spec.PowerState).Should(Equal(oldVM.Spec.PowerState))
			})
		})
	})

	Describe("SetDefaultInstanceUUID", func() {

		var (
			err         error
			wasMutated  bool
			inUUID      = uuid.NewString()
			expectedErr = field.Forbidden(
				field.NewPath("spec", "instanceUUID"),
				"only privileged users may set this field")
		)

		JustBeforeEach(func() {
			wasMutated, err = mutation.SetDefaultInstanceUUID(
				&ctx.WebhookRequestContext,
				ctx.Client,
				ctx.vm)
		})

		When("spec.instanceUUID is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.InstanceUUID = ""
			})

			Context("privileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = true
				})

				It("Should set InstanceUUID", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.InstanceUUID).ToNot(BeEmpty())
				})
			})

			Context("unprivileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = false
				})

				It("Should set InstanceUUID", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.InstanceUUID).ToNot(BeEmpty())
				})
			})
		})

		When("spec.instanceUUID is not empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.InstanceUUID = inUUID
			})

			Context("privileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = true
				})

				It("Should allow InstanceUUID", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.InstanceUUID).To(Equal(inUUID))
				})
			})

			Context("unprivileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = false
				})

				It("Should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(expectedErr))
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.InstanceUUID).To(Equal(inUUID))
				})
			})
		})
	})

	Describe("SetDefaultBiosUUID", func() {

		var (
			err         error
			wasMutated  bool
			inUUID      = uuid.NewString()
			expectedErr = field.Forbidden(
				field.NewPath("spec", "biosUUID"),
				"only privileged users may set this field")
		)

		JustBeforeEach(func() {
			wasMutated, err = mutation.SetDefaultBiosUUID(
				&ctx.WebhookRequestContext,
				ctx.Client,
				ctx.vm)
		})

		When("spec.biosUUID is empty", func() {

			BeforeEach(func() {
				ctx.vm.Spec.BiosUUID = ""
			})

			Context("privileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = true
				})

				It("Should set BiosUUID", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.BiosUUID).ToNot(BeEmpty())
				})
			})

			Context("unprivileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = false
				})

				It("Should set BiosUUID", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.BiosUUID).ToNot(BeEmpty())
				})
			})

			When("spec.bootstrap.cloudInit.instanceID is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
					}
				})

				Context("privileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = true
					})

					It("Should set BiosUUID and set CloudInit InstanceID", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(wasMutated).To(BeTrue())
						Expect(ctx.vm.Spec.BiosUUID).ToNot(BeEmpty())
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(ctx.vm.Spec.BiosUUID))
					})
				})

				Context("unprivileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = false
					})

					It("Should set BiosUUID and set CloudInit InstanceID", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(wasMutated).To(BeTrue())
						Expect(ctx.vm.Spec.BiosUUID).ToNot(BeEmpty())
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(ctx.vm.Spec.BiosUUID))
					})
				})
			})

			When("spec.bootstrap.cloudInit.instanceID is not empty", func() {
				var (
					inInstanceID = uuid.NewString()
				)

				BeforeEach(func() {
					ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
							InstanceID: inInstanceID,
						},
					}
				})

				Context("privileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = true
					})

					It("Should set BiosUUID and ignore CloudInit InstanceID", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(wasMutated).To(BeTrue())
						Expect(ctx.vm.Spec.BiosUUID).ToNot(BeEmpty())
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(inInstanceID))
					})
				})

				Context("unprivileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = false
					})

					It("Should set BiosUUID and ignore CloudInit InstanceID", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(wasMutated).To(BeTrue())
						Expect(ctx.vm.Spec.BiosUUID).ToNot(BeEmpty())
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(inInstanceID))
					})
				})
			})
		})

		When("spec.biosUUID is not empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.BiosUUID = inUUID
			})

			Context("privileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = true
				})

				It("Should allow BiosUUID", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.BiosUUID).To(Equal(inUUID))
				})
			})

			Context("unprivileged user", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = false
				})

				It("Should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(expectedErr))
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.BiosUUID).To(Equal(inUUID))
				})
			})

			When("spec.bootstrap.cloudInit.instanceID is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
					}
				})

				Context("privileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = true
					})

					It("Should allow BiosUUID and set CloudInit InstanceID", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(wasMutated).To(BeTrue())
						Expect(ctx.vm.Spec.BiosUUID).To(Equal(inUUID))
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(inUUID))
					})
				})

				Context("unprivileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = false
					})

					It("Should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(expectedErr))
						Expect(wasMutated).To(BeFalse())
						Expect(ctx.vm.Spec.BiosUUID).To(Equal(inUUID))
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(BeEmpty())
					})
				})
			})

			When("spec.bootstrap.cloudInit.instanceID is not empty", func() {
				var (
					inInstanceID = uuid.NewString()
				)

				BeforeEach(func() {
					ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
							InstanceID: inInstanceID,
						},
					}
				})

				Context("privileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = true
					})

					It("Should allow BiosUUID and ignore CloudInit InstanceID", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(wasMutated).To(BeFalse())
						Expect(ctx.vm.Spec.BiosUUID).To(Equal(inUUID))
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(inInstanceID))
					})
				})

				Context("unprivileged user", func() {
					BeforeEach(func() {
						ctx.IsPrivilegedAccount = false
					})

					It("Should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(expectedErr))
						Expect(wasMutated).To(BeFalse())
						Expect(ctx.vm.Spec.BiosUUID).To(Equal(inUUID))
						Expect(ctx.vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal(inInstanceID))
					})
				})
			})
		})
	})

	Describe("ResolveImageName", func() {
		const (
			vmiKind  = "VirtualMachineImage"
			cvmiKind = "Cluster" + vmiKind

			nsImg1ID   = "vmi-1"
			nsImg1Name = "image-a"

			nsImg2ID   = "vmi-2"
			nsImg2Name = "image-b"

			nsImg3ID   = "vmi-3"
			nsImg3Name = "image-b"

			nsImg4ID   = "vmi-4"
			nsImg4Name = "image-c"

			clImg1ID   = "vmi-5"
			clImg1Name = "image-d"

			clImg2ID   = "vmi-6"
			clImg2Name = "image-e"

			clImg3ID   = "vmi-7"
			clImg3Name = "image-e"

			clImg4ID   = "vmi-8"
			clImg4Name = "image-c"
		)

		var (
			mutatedErr  error
			wasMutated  bool
			initObjects []client.Object
		)

		newNsImgFn := func(id, name string) *vmopv1.VirtualMachineImage {
			img := builder.DummyVirtualMachineImage(id)
			img.Namespace = ctx.vm.Namespace
			img.Status.Name = name
			return img
		}

		newClImgFn := func(id, name string) *vmopv1.ClusterVirtualMachineImage {
			img := builder.DummyClusterVirtualMachineImage(id)
			img.Status.Name = name
			return img
		}

		BeforeEach(func() {
			initObjects = []client.Object{
				newNsImgFn(nsImg1ID, nsImg1Name),
				newNsImgFn(nsImg2ID, nsImg2Name),
				newNsImgFn(nsImg3ID, nsImg3Name),
				newNsImgFn(nsImg4ID, nsImg4Name),
				newClImgFn(clImg1ID, clImg1Name),
				newClImgFn(clImg2ID, clImg2Name),
				newClImgFn(clImg3ID, clImg3Name),
				newClImgFn(clImg4ID, clImg4Name),
			}
		})

		JustBeforeEach(func() {
			// Replace the client with a fake client that has the index of VM images.
			ctx.Client = fake.NewClientBuilder().WithScheme(builder.NewScheme()).
				WithIndex(
					&vmopv1.VirtualMachineImage{},
					"status.name",
					func(rawObj client.Object) []string {
						image := rawObj.(*vmopv1.VirtualMachineImage)
						return []string{image.Status.Name}
					}).
				WithIndex(&vmopv1.ClusterVirtualMachineImage{},
					"status.name",
					func(rawObj client.Object) []string {
						image := rawObj.(*vmopv1.ClusterVirtualMachineImage)
						return []string{image.Status.Name}
					}).
				WithObjects(initObjects...).
				Build()
			wasMutated, mutatedErr = mutation.ResolveImageNameOnCreate(
				&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
		})

		When("spec.image is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Image = nil
			})
			When("spec.imageName is vmi", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg1ID
				})
				It("Should mutate Image but not ImageName", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(vmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(nsImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(Equal(nsImg1ID))
				})
				When("no image exists", func() {
					const missingVmi = "vmi-9999999"
					BeforeEach(func() {
						ctx.vm.Spec.ImageName = missingVmi
					})
					It("Should return an error", func() {
						Expect(mutatedErr).To(HaveOccurred())
						Expect(mutatedErr.Error()).To(Equal(fmt.Sprintf("no VM image exists for %q in namespace or cluster scope", missingVmi)))
						Expect(wasMutated).To(BeFalse())
						Expect(ctx.vm.Spec.Image).To(BeNil())
						Expect(ctx.vm.Spec.ImageName).To(Equal(missingVmi))
					})
				})
			})
			When("spec.imageName is display name", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg1Name
				})
				It("Should mutate Image but not ImageName", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(vmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(nsImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(Equal(nsImg1Name))
				})
			})
			When("spec.imageName is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = ""
				})
				It("Should not mutate Image or ImageName", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).To(BeNil())
					Expect(ctx.vm.Spec.ImageName).To(BeEmpty())
				})
			})

			When("spec.imageName matches multiple, namespaced-scoped images", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg2Name
				})
				It("Should return an error", func() {
					Expect(mutatedErr).To(HaveOccurred())
					Expect(mutatedErr.Error()).To(Equal(fmt.Sprintf("multiple VM images exist for %q in namespace scope", nsImg2Name)))
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).To(BeNil())
					Expect(ctx.vm.Spec.ImageName).To(Equal(nsImg2Name))
				})
			})

			When("spec.imageName matches multiple, cluster-scoped images", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = clImg2Name
				})
				It("Should return an error", func() {
					Expect(mutatedErr).To(HaveOccurred())
					Expect(mutatedErr.Error()).To(Equal(fmt.Sprintf("multiple VM images exist for %q in cluster scope", clImg2Name)))
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).To(BeNil())
					Expect(ctx.vm.Spec.ImageName).To(Equal(clImg2Name))
				})
			})

			When("spec.imageName matches both namespace and cluster-scoped images", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = clImg4Name
				})
				It("Should return an error", func() {
					Expect(mutatedErr).To(HaveOccurred())
					Expect(mutatedErr.Error()).To(Equal(fmt.Sprintf("multiple VM images exist for %q in namespace and cluster scope", clImg4Name)))
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).To(BeNil())
					Expect(ctx.vm.Spec.ImageName).To(Equal(clImg4Name))
				})
			})

			When("spec.imageName does not match any namespace or cluster-scoped images", func() {
				const invalidImageID = "invalid"
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = invalidImageID
				})
				It("Should return an error", func() {
					Expect(mutatedErr).To(HaveOccurred())
					Expect(mutatedErr.Error()).To(Equal(fmt.Sprintf("no VM image exists for %q in namespace or cluster scope", invalidImageID)))
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).To(BeNil())
					Expect(ctx.vm.Spec.ImageName).To(Equal(invalidImageID))
				})
			})

			When("spec.imageName matches a single namespace-scoped image", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg1Name
				})
				It("Should mutate Image but not ImageName", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(vmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(nsImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(Equal(nsImg1Name))
				})
			})

			When("spec.imageName matches a single cluster-scoped image", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = clImg1Name
				})
				It("Should mutate Image but not ImageName", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(cvmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(clImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(Equal(clImg1Name))
				})
			})
		})

		When("spec.image is non-empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
					Kind: vmiKind,
					Name: nsImg1ID,
				}
			})
			When("spec.imageName is vmi", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg1ID
				})
				It("Should not mutate anything", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(vmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(nsImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(Equal(nsImg1ID))
				})
			})
			When("spec.imageName is display name", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg1Name
				})
				It("Should not mutate anything", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(vmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(nsImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(Equal(nsImg1Name))
				})
			})
			When("spec.imageName is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = ""
				})
				It("Should not mutate anything", func() {
					Expect(mutatedErr).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.Image).ToNot(BeNil())
					Expect(ctx.vm.Spec.Image.Kind).To(Equal(vmiKind))
					Expect(ctx.vm.Spec.Image.Name).To(Equal(nsImg1ID))
					Expect(ctx.vm.Spec.ImageName).To(BeEmpty())
				})
			})
			When("spec.imageName points to a different image", func() {
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = clImg1ID
				})
				It("Should not mutate anything", func() {
					Expect(mutatedErr).To(HaveOccurred())
					Expect(mutatedErr.Error()).To(Equal(field.Invalid(
						field.NewPath("spec", "imageName"),
						clImg1ID,
						"must refer to the same resource as spec.image").Error()))
					Expect(wasMutated).To(BeFalse())
				})
			})
			When("spec.imageName points to the same image ID but different scope", func() {
				const (
					clImg0ID   = "vmi-1"
					clImg0Name = "image-a"
				)
				BeforeEach(func() {
					ctx.vm.Spec.ImageName = nsImg1ID
					ctx.vm.Spec.Image.Name = clImg0ID
					ctx.vm.Spec.Image.Kind = cvmiKind
					initObjects = append(initObjects, newClImgFn(clImg0ID, clImg0Name))
				})
				It("Should not mutate anything", func() {
					Expect(mutatedErr).To(HaveOccurred())
					Expect(mutatedErr.Error()).To(Equal(field.Invalid(
						field.NewPath("spec", "imageName"),
						nsImg1ID,
						"must refer to the same resource as spec.image").Error()))
					Expect(wasMutated).To(BeFalse())
				})
			})
		})
	})

	Describe("SetNextRestartTime", func() {

		var (
			oldVM *vmopv1.VirtualMachine
		)

		BeforeEach(func() {
			oldVM = ctx.vm.DeepCopy()
		})

		When("oldVM has empty spec.nextRestartTime", func() {
			BeforeEach(func() {
				oldVM.Spec.NextRestartTime = ""
			})
			Context("newVM has spec.nextRestartTime set to an empty value", func() {
				It("should not mutate anything", func() {
					ctx.vm.Spec.NextRestartTime = ""
					ok, err := mutation.SetNextRestartTime(
						&ctx.WebhookRequestContext,
						ctx.vm,
						oldVM)
					Expect(ok).To(BeFalse())
					Expect(err).ToNot(HaveOccurred())
					Expect(ctx.vm.Spec.NextRestartTime).To(BeEmpty())
				})
			})
			Context("newVM has spec.nextRestartTime set to 'now' (case-insensitive)", func() {
				Context("vm is powered on", func() {
					BeforeEach(func() {
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})
					It("should mutate the field to a valid UTC timestamp", func() {
						for _, s := range []string{"now", "Now", "NOW"} {
							ctx.vm.Spec.NextRestartTime = s
							ok, err := mutation.SetNextRestartTime(
								&ctx.WebhookRequestContext,
								ctx.vm,
								oldVM)
							Expect(ok).To(BeTrue())
							Expect(err).ToNot(HaveOccurred())
							Expect(ctx.vm.Spec.NextRestartTime).ToNot(BeEmpty())
							_, err = time.Parse(time.RFC3339Nano, ctx.vm.Spec.NextRestartTime)
							Expect(err).ShouldNot(HaveOccurred())
						}
					})
				})
				Context("vm is powered off", func() {
					BeforeEach(func() {
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})
					It("should return an error", func() {
						ctx.vm.Spec.NextRestartTime = "now"
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							"now",
							"can only restart powered on vm").Error()))
					})
				})
				Context("vm is suspended", func() {
					BeforeEach(func() {
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})
					It("should return an error", func() {
						ctx.vm.Spec.NextRestartTime = "now"
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							"now",
							"can only restart powered on vm").Error()))
					})
				})
			})
			DescribeTable(
				`newVM has spec.nextRestartTime set a non-empty value that is not "now"`,
				append([]any{
					func(nextRestartTime string) {
						ctx.vm.Spec.NextRestartTime = nextRestartTime
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							nextRestartTime,
							`may only be set to "now"`).Error()))
						Expect(ctx.vm.Spec.NextRestartTime).To(Equal(nextRestartTime))
					}},
					newInvalidNextRestartTimeTableEntries("should return an invalid field error"))...,
			)
		})

		When("oldVM has non-empty spec.nextRestartTime", func() {
			var (
				lastRestartTime time.Time
			)
			BeforeEach(func() {
				lastRestartTime = time.Now().UTC()
				oldVM.Spec.NextRestartTime = lastRestartTime.Format(time.RFC3339Nano)
			})
			Context("newVM has spec.nextRestartTime set to an empty value", func() {
				It("should mutate to match oldVM", func() {
					ctx.vm.Spec.NextRestartTime = ""
					ok, err := mutation.SetNextRestartTime(
						&ctx.WebhookRequestContext,
						ctx.vm,
						oldVM)
					Expect(ok).To(BeTrue())
					Expect(err).ToNot(HaveOccurred())
					Expect(ctx.vm.Spec.NextRestartTime).To(Equal(oldVM.Spec.NextRestartTime))
				})
			})
			Context("newVM has spec.nextRestartTime set to 'now' (case-insensitive)", func() {
				It("should mutate the field to a valid UTC timestamp", func() {
					for _, s := range []string{"now", "Now", "NOW"} {
						ctx.vm.Spec.NextRestartTime = s
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeTrue())
						Expect(err).ToNot(HaveOccurred())
						Expect(ctx.vm.Spec.NextRestartTime).ToNot(BeEmpty())
						nextRestartTime, err := time.Parse(time.RFC3339Nano, ctx.vm.Spec.NextRestartTime)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(lastRestartTime.Before(nextRestartTime)).To(BeTrue())
					}
				})
			})
			DescribeTable(
				`newVM has spec.nextRestartTime set a non-empty value that is not "now"`,
				append([]any{
					func(nextRestartTime string) {
						ctx.vm.Spec.NextRestartTime = nextRestartTime
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							nextRestartTime,
							`may only be set to "now"`).Error()))
						Expect(ctx.vm.Spec.NextRestartTime).To(Equal(nextRestartTime))
					}},
					newInvalidNextRestartTimeTableEntries("should return an invalid field error"))...,
			)
		})
	})

	Describe("SetCreatedAtAnnotations", func() {
		var (
			vm *vmopv1.VirtualMachine
		)

		BeforeEach(func() {
			vm = ctx.vm.DeepCopy()
		})

		When("vm does not have any annotations", func() {
			It("should add the created-at annotations", func() {
				Expect(vm.Annotations).ToNot(HaveKey(constants.CreatedAtBuildVersionAnnotationKey))
				Expect(vm.Annotations).ToNot(HaveKey(constants.CreatedAtSchemaVersionAnnotationKey))
				mutation.SetCreatedAtAnnotations(
					pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = "v1"
					}), vm)
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtBuildVersionAnnotationKey, "v1"))
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtSchemaVersionAnnotationKey, vmopv1.GroupVersion.Version))
			})
		})

		When("vm does have some existing annotations", func() {
			It("should add the created-at annotations", func() {
				vm.Annotations = map[string]string{"k1": "v1", "k2": "v2"}
				mutation.SetCreatedAtAnnotations(
					pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = "v1"
					}), vm)
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtBuildVersionAnnotationKey, "v1"))
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtSchemaVersionAnnotationKey, vmopv1.GroupVersion.Version))
			})
		})

		When("vm has the created-at annotations", func() {
			It("should overwrite the created-at annotations", func() {
				vm.Annotations = map[string]string{
					constants.CreatedAtBuildVersionAnnotationKey:  "fake-build-version",
					constants.CreatedAtSchemaVersionAnnotationKey: "fake-schema-version",
				}
				mutation.SetCreatedAtAnnotations(
					pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = "v1"
					}), vm)
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtBuildVersionAnnotationKey, "v1"))
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtSchemaVersionAnnotationKey, vmopv1.GroupVersion.Version))
			})
		})
	})

	Describe("SetLastResizeAnnotation", func() {
		const newClassName = "my-new-class"

		DescribeTableSubtree("Resize Tests",
			func(fullResize bool) {
				var (
					oldVM *vmopv1.VirtualMachine
				)

				BeforeEach(func() {
					pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
						if fullResize {
							config.Features.VMResize = true
						} else {
							config.Features.VMResizeCPUMemory = true
						}
					})
					oldVM = ctx.vm.DeepCopy()
				})

				When("vm ClassName does not change", func() {
					It("does not set last-resize annotation", func() {
						updated, err := mutation.SetLastResizeAnnotations(&ctx.WebhookRequestContext, ctx.vm, oldVM)
						Expect(err).ToNot(HaveOccurred())
						Expect(updated).To(BeFalse())

						_, _, _, exists := vmopv1util.GetLastResizedAnnotation(*ctx.vm)
						Expect(exists).To(BeFalse())
					})
				})

				When("existing vm ClassName changes", func() {
					It("set last-resize annotation", func() {
						ctx.vm.Spec.ClassName = newClassName

						updated, err := mutation.SetLastResizeAnnotations(&ctx.WebhookRequestContext, ctx.vm, oldVM)
						Expect(err).ToNot(HaveOccurred())
						Expect(updated).To(BeTrue())

						className, _, _, exists := vmopv1util.GetLastResizedAnnotation(*ctx.vm)
						Expect(exists).To(BeTrue())
						Expect(className).To(Equal(oldVM.Spec.ClassName))
					})
				})

				When("existing vm is classless", func() {
					It("set last-resize annotation", func() {
						oldVM.Spec.ClassName = ""
						ctx.vm.Spec.ClassName = newClassName

						updated, err := mutation.SetLastResizeAnnotations(&ctx.WebhookRequestContext, ctx.vm, oldVM)
						Expect(err).ToNot(HaveOccurred())
						Expect(updated).To(BeTrue())

						className, uid, gen, exists := vmopv1util.GetLastResizedAnnotation(*ctx.vm)
						Expect(exists).To(BeTrue())
						Expect(className).To(Equal(oldVM.Spec.ClassName))
						Expect(uid).To(BeEmpty())
						Expect(gen).To(BeZero())
					})
				})

				When("vm already has last-resize annotation", func() {
					It("annotation is not changed", func() {
						vmClass := builder.DummyVirtualMachineClass("my-class")
						vmopv1util.MustSetLastResizedAnnotation(ctx.vm, *vmClass)

						updated, err := mutation.SetLastResizeAnnotations(&ctx.WebhookRequestContext, ctx.vm, oldVM)
						Expect(err).ToNot(HaveOccurred())
						Expect(updated).To(BeFalse())

						className, _, _, exists := vmopv1util.GetLastResizedAnnotation(*ctx.vm)
						Expect(exists).To(BeTrue())
						Expect(className).To(Equal(vmClass.Name))
					})
				})
			},

			Entry("Full", true),
			Entry("CPU & Memory", false),
		)
	})

	Describe("SetDefaultCdromImgKindOnCreate", func() {

		BeforeEach(func() {
			ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
				Cdrom: []vmopv1.VirtualMachineCdromSpec{
					{
						Name: "cdrom1",
						Image: vmopv1.VirtualMachineImageRef{
							Name: "vmi-1",
						},
					},
					{
						Name: "cdrom2",
						Image: vmopv1.VirtualMachineImageRef{
							Name: "vmi-2",
						},
					},
				},
			}
		})

		It("should set the default image kind as VirtualMachineImage", func() {
			mutation.SetDefaultCdromImgKindOnCreate(&ctx.WebhookRequestContext, ctx.vm)
			Expect(ctx.vm.Spec.Hardware.Cdrom[0].Image.Kind).To(Equal("VirtualMachineImage"))
			Expect(ctx.vm.Spec.Hardware.Cdrom[1].Image.Kind).To(Equal("VirtualMachineImage"))
		})
	})

	Describe("SetDefaultCdromImgKindOnUpdate", func() {

		var oldVM *vmopv1.VirtualMachine

		BeforeEach(func() {
			ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
				Cdrom: []vmopv1.VirtualMachineCdromSpec{
					{
						Name: "cdrom1",
						Image: vmopv1.VirtualMachineImageRef{
							Name: "vmi-1",
							Kind: "VirtualMachineImage",
						},
					},
					{
						Name: "cdrom2",
						Image: vmopv1.VirtualMachineImageRef{
							Name: "vmi-2",
							Kind: "ClusterVirtualMachineImage",
						},
					},
				},
			}
			oldVM = ctx.vm.DeepCopy()
			ctx.vm.Spec.Hardware.Cdrom[0].Image.Kind = ""
			ctx.vm.Spec.Hardware.Cdrom[1].Image.Kind = ""
		})

		It("should set the default image kind if previously set to default", func() {
			mutation.SetDefaultCdromImgKindOnUpdate(&ctx.WebhookRequestContext, ctx.vm, oldVM)
			Expect(ctx.vm.Spec.Hardware.Cdrom[0].Image.Kind).To(Equal("VirtualMachineImage"))
			Expect(ctx.vm.Spec.Hardware.Cdrom[1].Image.Kind).To(BeEmpty())
		})
	})

	Describe("SetImageNameFromCdrom", func() {

		BeforeEach(func() {
			ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
				Cdrom: []vmopv1.VirtualMachineCdromSpec{
					{
						Image: vmopv1.VirtualMachineImageRef{
							Name: "vmi-cdrom",
						},
					},
				},
			}
		})

		It("should not set the image name from CD-ROM if already set", func() {
			ctx.vm.Spec.ImageName = "vmi-original"
			mutation.SetImageNameFromCdrom(&ctx.WebhookRequestContext, ctx.vm)
			Expect(ctx.vm.Spec.ImageName).To(Equal("vmi-original"))
		})

		It("should set the image name from CD-ROM if not set", func() {
			ctx.vm.Spec.ImageName = ""
			mutation.SetImageNameFromCdrom(&ctx.WebhookRequestContext, ctx.vm)
			Expect(ctx.vm.Spec.ImageName).To(Equal("vmi-cdrom"))
		})

		It("should set to the first connected CD-ROM if multiple exist", func() {
			// Update the existing CD-ROM to be disconnected to ensure the first
			// connected CD-ROM's image name is used.
			for i := range ctx.vm.Spec.Hardware.Cdrom {
				ctx.vm.Spec.Hardware.Cdrom[i].Connected = ptr.To(false)
			}
			ctx.vm.Spec.Hardware.Cdrom = append(
				ctx.vm.Spec.Hardware.Cdrom, vmopv1.VirtualMachineCdromSpec{
					Image: vmopv1.VirtualMachineImageRef{
						Name: "vmi-new",
					},
					Connected: ptr.To(true),
				})
			ctx.vm.Spec.ImageName = ""
			mutation.SetImageNameFromCdrom(&ctx.WebhookRequestContext, ctx.vm)
			Expect(ctx.vm.Spec.ImageName).To(Equal("vmi-new"))
		})
	})

	Describe("CleanupApplyPowerStateChangeTimeAnno", func() {
		var oldVM *vmopv1.VirtualMachine

		BeforeEach(func() {
			oldVM = ctx.vm.DeepCopy()
		})

		When("VM's apply power state change time annotation is changed", func() {
			BeforeEach(func() {
				now := time.Now().UTC()
				oldVM.Annotations[constants.ApplyPowerStateTimeAnnotation] = now.Format(time.RFC3339Nano)
				ctx.vm.Annotations[constants.ApplyPowerStateTimeAnnotation] = now.Add(time.Minute).Format(time.RFC3339Nano)
			})

			It("should be a no-op", func() {
				mutated := mutation.CleanupApplyPowerStateChangeTimeAnno(&ctx.WebhookRequestContext, ctx.vm, oldVM)
				Expect(mutated).To(BeFalse())
				Expect(ctx.vm.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
			})
		})

		When("VM's power state is not changed with a pre-existing apply power state change time annotation", func() {
			BeforeEach(func() {
				now := time.Now().UTC()
				oldVM.Annotations[constants.ApplyPowerStateTimeAnnotation] = now.Format(time.RFC3339Nano)
				ctx.vm.Annotations[constants.ApplyPowerStateTimeAnnotation] = now.Format(time.RFC3339Nano)
				oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})

			It("should be a no-op", func() {
				mutated := mutation.CleanupApplyPowerStateChangeTimeAnno(&ctx.WebhookRequestContext, ctx.vm, oldVM)
				Expect(mutated).To(BeFalse())
				Expect(ctx.vm.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
			})
		})

		When("VM's power state is changed with a pre-existing apply power state change time annotation", func() {
			BeforeEach(func() {
				now := time.Now().UTC()
				oldVM.Annotations[constants.ApplyPowerStateTimeAnnotation] = now.Format(time.RFC3339Nano)
				ctx.vm.Annotations[constants.ApplyPowerStateTimeAnnotation] = now.Format(time.RFC3339Nano)
				oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			It("should remove the apply power state change time annotation", func() {
				mutated := mutation.CleanupApplyPowerStateChangeTimeAnno(&ctx.WebhookRequestContext, ctx.vm, oldVM)
				Expect(mutated).To(BeTrue())
				Expect(ctx.vm.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
			})
		})
	})

	Describe("ResolveClassAndClassName", func() {
		const (
			testClassName        = "test-class"
			testInstanceName     = "test-instance"
			activeInstanceName   = "active-instance"
			inactiveInstanceName = "inactive-instance"
		)

		var (
			wasMutated bool
			err        error
		)

		JustBeforeEach(func() {
			wasMutated, err = mutation.ResolveClassAndClassName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
		})

		Context("when both spec.className and spec.class are unset", func() {
			BeforeEach(func() {
				ctx.vm.Spec.ClassName = ""
				ctx.vm.Spec.Class = nil
			})

			It("should return early without mutation", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				Expect(ctx.vm.Spec.ClassName).To(BeEmpty())
				Expect(ctx.vm.Spec.Class).To(BeNil())
			})
		})

		Context("when spec.class has empty name", func() {
			BeforeEach(func() {
				ctx.vm.Spec.ClassName = ""
				ctx.vm.Spec.Class = &common.LocalObjectRef{
					Name: "",
				}
			})

			It("should return error for empty class instance name", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("empty class instance name"))
				Expect(wasMutated).To(BeFalse())
			})
		})

		Context("when both spec.className and spec.class are set", func() {
			BeforeEach(func() {
				ctx.vm.Spec.ClassName = testClassName
				ctx.vm.Spec.Class = &common.LocalObjectRef{
					Name: testInstanceName,
				}
			})

			It("should return early without mutation", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				Expect(ctx.vm.Spec.ClassName).To(Equal(testClassName))
				Expect(ctx.vm.Spec.Class.Name).To(Equal(testInstanceName))
			})
		})

		Context("when spec.className is set but spec.class is nil", func() {
			var activeInstance *vmopv1.VirtualMachineClassInstance
			BeforeEach(func() {
				ctx.vm.Spec.ClassName = testClassName
				ctx.vm.Spec.Class = nil
			})

			Context("when no instances exist for the class", func() {
				It("should not mutate when no instances found", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.ClassName).To(Equal(testClassName))
					Expect(ctx.vm.Spec.Class).To(BeNil())
				})
			})

			Context("when an instance exists, but is not active", func() {
				BeforeEach(func() {
					inactiveInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      inactiveInstanceName,
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: testClassName,
									Kind: "VirtualMachineClass",
								},
							},
						},
						Spec: vmopv1.VirtualMachineClassInstanceSpec{},
					}
					Expect(ctx.Client.Create(ctx, inactiveInstance)).To(Succeed())
				})

				It("should not mutate when no active instance found", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.ClassName).To(Equal(testClassName))
					Expect(ctx.vm.Spec.Class).To(BeNil())
				})
			})

			Context("when an active vmclassinstance exists, but it is not owned by VM class", func() {
				BeforeEach(func() {
					instance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testInstanceName,
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: "some-other-resource",
									Kind: "SomeOtherKind",
								},
							},
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
						Spec: vmopv1.VirtualMachineClassInstanceSpec{},
					}
					Expect(ctx.Client.Create(ctx, instance)).To(Succeed())
				})

				It("should not mutate when no VirtualMachineClass owner found", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.ClassName).To(Equal(testClassName))
					Expect(ctx.vm.Spec.Class).To(BeNil())
				})
			})

			Context("when a valid and active instance exists", func() {
				BeforeEach(func() {
					activeInstance = &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      activeInstanceName,
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: testClassName,
									Kind: "VirtualMachineClass",
								},
							},
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
						Spec: vmopv1.VirtualMachineClassInstanceSpec{},
					}
					Expect(ctx.Client.Create(ctx, activeInstance)).To(Succeed())
				})

				It("should populate spec.class with active instance", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.ClassName).To(Equal(testClassName))
					Expect(ctx.vm.Spec.Class.Name).To(Equal(activeInstance.Name))
					Expect(ctx.vm.Spec.Class.Kind).To(Equal(activeInstance.Kind))
				})
			})
		})

		Context("when spec.class is set but spec.className is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.ClassName = ""
				ctx.vm.Spec.Class = &common.LocalObjectRef{
					Name: testInstanceName,
				}
			})

			Context("when the instance does not exist", func() {
				It("should return error when instance not found", func() {
					Expect(err).To(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
				})
			})

			Context("when the instance exists but is inactive", func() {
				BeforeEach(func() {
					instance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testInstanceName,
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: testClassName,
									Kind: "VirtualMachineClass",
								},
							},
						},
						Spec: vmopv1.VirtualMachineClassInstanceSpec{},
					}
					Expect(ctx.Client.Create(ctx, instance)).To(Succeed())
				})

				It("should return error for inactive instance", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("must specify a VirtualMachineClassInstance that is active"))
					Expect(wasMutated).To(BeFalse())
				})
			})

			Context("when an active vmclassinstance exists, but it is not owned by VM class", func() {
				BeforeEach(func() {
					instance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testInstanceName,
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: "some-other-resource",
									Kind: "SomeOtherKind",
								},
							},
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
						Spec: vmopv1.VirtualMachineClassInstanceSpec{},
					}
					Expect(ctx.Client.Create(ctx, instance)).To(Succeed())
				})

				It("should not mutate when no VirtualMachineClass owner found", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(ctx.vm.Spec.ClassName).To(BeEmpty())
					Expect(ctx.vm.Spec.Class.Name).To(Equal(testInstanceName))
				})
			})

			Context("when the instance exists and is active", func() {
				BeforeEach(func() {
					instance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testInstanceName,
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: testClassName,
									Kind: "VirtualMachineClass",
								},
							},
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
						Spec: vmopv1.VirtualMachineClassInstanceSpec{},
					}
					Expect(ctx.Client.Create(ctx, instance)).To(Succeed())
				})

				It("should populate spec.className from instance owner", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					Expect(ctx.vm.Spec.ClassName).To(Equal(testClassName))
					Expect(ctx.vm.Spec.Class.Name).To(Equal(testInstanceName))
				})
			})
		})
	})

	Describe("SetPVCVolumeDefaultsOnCreate", func() {
		var (
			vm         *vmopv1.VirtualMachine
			wasMutated bool
			err        error
		)

		BeforeEach(func() {
			vm = ctx.vm.DeepCopy()
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "test-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
						},
					},
				},
			}
		})

		JustBeforeEach(func() {
			wasMutated, err = mutation.SetPVCVolumeDefaultsOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
		})

		When("vm has pvc and it doesn't have application type", func() {
			It("should not set any defaults", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.ApplicationType).To(BeEmpty())
			})
		})

		When("vm has pvc and it has application type OracleRAC", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
			})

			It("should set the default PVC volume application type", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.DiskMode).To(Equal(vmopv1.VolumeDiskModeIndependentPersistent))
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.SharingMode).To(Equal(vmopv1.VolumeSharingModeMultiWriter))
			})
		})

		When("vm has pvc and it has application type MicrosoftWSFC", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ApplicationType = vmopv1.VolumeApplicationTypeMicrosoftWSFC
			})

			It("should set the default PVC volume application type", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.DiskMode).To(Equal(vmopv1.VolumeDiskModeIndependentPersistent))
			})
		})

		When("vm has pvc and it has application type other than OracleRAC or MicrosoftWSFC", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "test-volume-2",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc-2",
							},
							ApplicationType: "invalid",
						},
					},
				})
			})

			It("should return error with unsupported application type", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(
					"spec[1].persistentVolumeClaim.applicationType: " +
						"Unsupported value: \"invalid\":" +
						" supported values: \"OracleRAC\", \"MicrosoftWSFC\""))
				Expect(wasMutated).To(BeFalse())
			})
		})

		When("vm has multiple pvcs and different application types", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
				vm.Spec.Volumes = append(vm.Spec.Volumes,
					vmopv1.VirtualMachineVolume{
						Name: "test-volume-2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc-2",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeMicrosoftWSFC,
							},
						},
					},
					vmopv1.VirtualMachineVolume{
						Name: "test-volume-3",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc-3",
								},
							},
						},
					},
				)
			})

			It("should set the PVC volume with correct disk mode and sharing mode", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.DiskMode).To(Equal(vmopv1.VolumeDiskModeIndependentPersistent))
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.SharingMode).To(Equal(vmopv1.VolumeSharingModeMultiWriter))
				Expect(vm.Spec.Volumes[1].PersistentVolumeClaim.DiskMode).To(Equal(vmopv1.VolumeDiskModeIndependentPersistent))
				Expect(vm.Spec.Volumes[1].PersistentVolumeClaim.SharingMode).To(BeEmpty())
				Expect(vm.Spec.Volumes[2].PersistentVolumeClaim.DiskMode).To(BeEmpty())
				Expect(vm.Spec.Volumes[2].PersistentVolumeClaim.SharingMode).To(BeEmpty())
			})
		})

		When("vm has no pvc", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}
			})

			It("should not set any defaults disk mode and sharing mode", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
			})
		})
	})

}
