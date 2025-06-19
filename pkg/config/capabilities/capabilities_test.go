// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capabilities_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const trueString = "true"

var _ = Describe("UpdateCapabilities", func() {
	var (
		ctx     context.Context
		client  ctrlclient.Client
		err     error
		changed bool
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		ctx = logr.NewContext(ctx, logf.Log)
		client = builder.NewFakeClient()
	})

	JustBeforeEach(func() {
		changed, err = capabilities.UpdateCapabilities(ctx, client)
	})

	Context("corev1.ConfigMap", func() {
		When("the resource does not exist", func() {
			Specify("an error should occur", func() {
				Expect(err).To(HaveOccurred())
				Expect(changed).To(BeFalse())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})
		When("the resource exists", func() {
			BeforeEach(func() {
				obj := corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: capabilities.ConfigMapNamespace,
						Name:      capabilities.ConfigMapName,
					},
					Data: map[string]string{
						capabilities.CapabilityKeyBringYourOwnKeyProvider:     trueString,
						capabilities.CapabilityKeyTKGMultipleContentLibraries: trueString,
						capabilities.CapabilityKeyWorkloadIsolation:           trueString,
					},
				}
				Expect(client.Create(ctx, &obj)).To(Succeed())
			})

			When("the capabilities are not different", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = true
						config.Features.TKGMultipleCL = true
						config.Features.WorkloadDomainIsolation = true
					})
				})
				Specify("capabilities did not change", func() {
					Expect(changed).To(BeFalse())
				})
				Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
					Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeTrue())
				})
				Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
					Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
				})
				Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
					Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
				})
			})

			When("the capabilities are different", func() {
				Specify("capabilities changed", func() {
					Expect(changed).To(BeTrue())
				})
				Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
					Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeFalse())
				})
				Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
					Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
				})
				Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
					Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
				})
			})
		})
	})

	Context("capv1.Capabilities", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.SVAsyncUpgrade = true
			})
		})
		When("the resource does not exist", func() {
			Specify("an error should occur", func() {
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
				Expect(changed).To(BeFalse())
			})
		})
		When("the resource exists", func() {

			Context("with true capabilities", func() {
				BeforeEach(func() {
					obj := capv1.Capabilities{
						ObjectMeta: metav1.ObjectMeta{
							Name: capabilities.CapabilitiesName,
						},
					}
					Expect(client.Create(ctx, &obj)).To(Succeed())

					objPatch := ctrlclient.MergeFrom(obj.DeepCopy())
					obj.Status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
						capabilities.CapabilityKeyBringYourOwnKeyProvider: {
							Activated: true,
						},
						capabilities.CapabilityKeyTKGMultipleContentLibraries: {
							Activated: true,
						},
						capabilities.CapabilityKeyWorkloadIsolation: {
							Activated: true,
						},
						capabilities.CapabilityKeyMutableNetworks: {
							Activated: true,
						},
						capabilities.CapabilityKeyVMGroups: {
							Activated: true,
						},
						capabilities.CapabilityKeyImmutableClasses: {
							Activated: true,
						},
						capabilities.CapabilityKeyVMSnapshots: {
							Activated: true,
						},
						capabilities.CapabilityKeyInventoryContentLibrary: {
							Activated: true,
						},
					}
					Expect(client.Status().Patch(ctx, &obj, objPatch)).To(Succeed())
				})
				When("the capabilities are not different", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.BringYourOwnEncryptionKey = true
							config.Features.TKGMultipleCL = true
							config.Features.WorkloadDomainIsolation = true
							config.Features.MutableNetworks = true
							config.Features.VMGroups = true
							config.Features.ImmutableClasses = true
							config.Features.VMSnapshots = true
							config.Features.InventoryContentLibrary = true
						})
					})
					Specify("capabilities did not change", func() {
						Expect(changed).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
						Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
						Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
						Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyMutableNetworks, func() {
						Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyVMGroups, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyImmutableClasses, func() {
						Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyVMSnapshots, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyInventoryContentLibrary, func() {
						Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeTrue())
					})
				})

				When("the capabilities are different", func() {
					Specify("capabilities changed", func() {
						Expect(changed).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
						Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
						Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
						Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyMutableNetworks, func() {
						Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyVMGroups, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyImmutableClasses, func() {
						Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyVMSnapshots, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyInventoryContentLibrary, func() {
						Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeTrue())
					})
				})
			})

			Context("with false capabilities", func() {
				BeforeEach(func() {
					obj := capv1.Capabilities{
						ObjectMeta: metav1.ObjectMeta{
							Name: capabilities.CapabilitiesName,
						},
					}
					Expect(client.Create(ctx, &obj)).To(Succeed())

					objPatch := ctrlclient.MergeFrom(obj.DeepCopy())
					obj.Status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
						capabilities.CapabilityKeyBringYourOwnKeyProvider: {
							Activated: false,
						},
						capabilities.CapabilityKeyTKGMultipleContentLibraries: {
							Activated: false,
						},
						capabilities.CapabilityKeyWorkloadIsolation: {
							Activated: false,
						},
						capabilities.CapabilityKeyMutableNetworks: {
							Activated: false,
						},
						capabilities.CapabilityKeyVMGroups: {
							Activated: false,
						},
						capabilities.CapabilityKeyImmutableClasses: {
							Activated: false,
						},
						capabilities.CapabilityKeyVMSnapshots: {
							Activated: false,
						},
						capabilities.CapabilityKeyInventoryContentLibrary: {
							Activated: false,
						},
					}
					Expect(client.Status().Patch(ctx, &obj, objPatch)).To(Succeed())
				})
				When("the capabilities are not different", func() {
					Specify("capabilities did not change", func() {
						Expect(changed).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
						Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
						Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
						Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyMutableNetworks, func() {
						Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyVMGroups, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyImmutableClasses, func() {
						Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyVMSnapshots, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyInventoryContentLibrary, func() {
						Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeFalse())
					})
				})

				When("the capabilities are different", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.BringYourOwnEncryptionKey = true
							config.Features.TKGMultipleCL = true
							config.Features.WorkloadDomainIsolation = true
							config.Features.MutableNetworks = true
							config.Features.VMGroups = true
						})
					})
					Specify("capabilities changed", func() {
						Expect(changed).To(BeTrue())
					})
					Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
						Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
						Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
						Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyMutableNetworks, func() {
						Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyVMGroups, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyImmutableClasses, func() {
						Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyVMSnapshots, func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeFalse())
					})
					Specify(capabilities.CapabilityKeyInventoryContentLibrary, func() {
						Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeFalse())
					})
				})
			})
		})
	})
})

var _ = Describe("UpdateCapabilitiesFeatures", func() {
	var (
		ctx  context.Context
		ok   bool
		diff string
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		ctx = logr.NewContext(ctx, logf.Log)

		ok, diff = false, ""
	})

	When("obj is map[string]string", func() {
		var (
			obj map[string]string
		)
		BeforeEach(func() {
			obj = map[string]string{}
		})
		JustBeforeEach(func() {
			diff, ok = capabilities.UpdateCapabilitiesFeatures(ctx, obj)
		})
		Context(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
				obj[capabilities.CapabilityKeyTKGMultipleContentLibraries] = trueString
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("TKGMultipleCL=true"))
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
			})
		})

		Context("SVAsyncUpgrade is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.SVAsyncUpgrade = true
				})
			})
			Context(capabilities.CapabilityKeyWorkloadIsolation, func() {
				BeforeEach(func() {
					Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
					obj[capabilities.CapabilityKeyWorkloadIsolation] = trueString
				})
				Specify("Enabled", func() {
					Expect(ok).To(BeTrue())
					Expect(diff).To(Equal("WorkloadDomainIsolation=true"))
					Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
				})
			})
		})
	})

	When("obj is corev1.ConfigMap", func() {
		var (
			obj corev1.ConfigMap
		)
		BeforeEach(func() {
			obj.Data = map[string]string{}
		})
		JustBeforeEach(func() {
			diff, ok = capabilities.UpdateCapabilitiesFeatures(ctx, obj)
		})
		Context(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
				obj.Data[capabilities.CapabilityKeyTKGMultipleContentLibraries] = trueString
			})
			Specify("Enabled", func() {
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
			})
		})

		Context("SVAsyncUpgrade is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.SVAsyncUpgrade = true
				})
			})
			Context(capabilities.CapabilityKeyWorkloadIsolation, func() {
				BeforeEach(func() {
					Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
					obj.Data[capabilities.CapabilityKeyWorkloadIsolation] = trueString
				})
				Specify("Enabled", func() {
					Expect(ok).To(BeTrue())
					Expect(diff).To(Equal("WorkloadDomainIsolation=true"))
					Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
				})
			})
		})
	})

	When("obj is capv1.Capabilities", func() {
		var (
			obj capv1.Capabilities
		)
		BeforeEach(func() {
			obj.Status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{}
		})
		JustBeforeEach(func() {
			diff, ok = capabilities.UpdateCapabilitiesFeatures(ctx, obj)
		})
		Context(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyBringYourOwnKeyProvider] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("BringYourOwnEncryptionKey=true"))
				Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyTKGMultipleContentLibraries] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("TKGMultipleCL=true"))
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyWorkloadIsolation, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyWorkloadIsolation] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("WorkloadDomainIsolation=true"))
				Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyMutableNetworks, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyMutableNetworks] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("MutableNetworks=true"))
				Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyVMGroups, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyVMGroups] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("VMGroups=true"))
				Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyImmutableClasses, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyImmutableClasses] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("ImmutableClasses=true"))
				Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyVMSnapshots, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyVMSnapshots] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("VMSnapshots=true"))
				Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
			})
		})
		Context(capabilities.CapabilityKeyInventoryContentLibrary, func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeFalse())
				obj.Status.Supervisor[capabilities.CapabilityKeyInventoryContentLibrary] = capv1.CapabilityStatus{
					Activated: true,
				}
			})
			Specify("Enabled", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("InventoryContentLibrary=true"))
				Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeTrue())
			})
		})
	})
})

var _ = Describe("WouldUpdateCapabilitiesFeatures", func() {
	var (
		ctx context.Context
		obj capv1.Capabilities

		ok   bool
		diff string
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		ctx = logr.NewContext(ctx, logf.Log)
		obj.Status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
			capabilities.CapabilityKeyBringYourOwnKeyProvider: {
				Activated: true,
			},
			capabilities.CapabilityKeyTKGMultipleContentLibraries: {
				Activated: true,
			},
			capabilities.CapabilityKeyWorkloadIsolation: {
				Activated: true,
			},
			capabilities.CapabilityKeyMutableNetworks: {
				Activated: true,
			},
			capabilities.CapabilityKeyVMGroups: {
				Activated: true,
			},
			capabilities.CapabilityKeyImmutableClasses: {
				Activated: true,
			},
			capabilities.CapabilityKeyVMSnapshots: {
				Activated: true,
			},
			capabilities.CapabilityKeyInventoryContentLibrary: {
				Activated: true,
			},
		}

		ok, diff = false, ""
	})

	JustBeforeEach(func() {
		diff, ok = capabilities.WouldUpdateCapabilitiesFeatures(ctx, obj)
	})

	When("the resource exists", func() {
		When("the capabilities are not different", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.BringYourOwnEncryptionKey = true
					config.Features.TKGMultipleCL = true
					config.Features.WorkloadDomainIsolation = true
					config.Features.MutableNetworks = true
					config.Features.VMGroups = true
					config.Features.ImmutableClasses = true
					config.Features.VMSnapshots = true
					config.Features.InventoryContentLibrary = true
				})
			})
			Specify("capabilities did not change", func() {
				Expect(ok).To(BeFalse())
				Expect(diff).To(BeEmpty())
			})
			Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
				Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
				Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyMutableNetworks, func() {
				Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyVMGroups, func() {
				Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyImmutableClasses, func() {
				Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyVMSnapshots, func() {
				Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
			})
			Specify(capabilities.CapabilityKeyInventoryContentLibrary, func() {
				Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeTrue())
			})
		})

		When("the capabilities are different", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.BringYourOwnEncryptionKey = false
					config.Features.TKGMultipleCL = false
					config.Features.WorkloadDomainIsolation = false
					config.Features.MutableNetworks = false
					config.Features.VMGroups = false
					config.Features.ImmutableClasses = false
					config.Features.InventoryContentLibrary = false
				})
			})
			Specify("capabilities changed", func() {
				Expect(ok).To(BeTrue())
				Expect(diff).To(Equal("BringYourOwnEncryptionKey=true,ImmutableClasses=true,InventoryContentLibrary=true,MutableNetworks=true,TKGMultipleCL=true,VMGroups=true,VMSnapshots=true,WorkloadDomainIsolation=true"))
				Expect(diff).To(Equal("BringYourOwnEncryptionKey=true,ImmutableClasses=true,InventoryContentLibrary=true,MutableNetworks=true,TKGMultipleCL=true,VMGroups=true,WorkloadDomainIsolation=true"))
			})
			Specify(capabilities.CapabilityKeyBringYourOwnKeyProvider, func() {
				Expect(pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyTKGMultipleContentLibraries, func() {
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyWorkloadIsolation, func() {
				Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyMutableNetworks, func() {
				Expect(pkgcfg.FromContext(ctx).Features.MutableNetworks).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyVMGroups, func() {
				Expect(pkgcfg.FromContext(ctx).Features.VMGroups).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyImmutableClasses, func() {
				Expect(pkgcfg.FromContext(ctx).Features.ImmutableClasses).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyVMSnapshots, func() {
				Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeFalse())
			})
			Specify(capabilities.CapabilityKeyInventoryContentLibrary, func() {
				Expect(pkgcfg.FromContext(ctx).Features.InventoryContentLibrary).To(BeFalse())
			})
		})
	})
})
