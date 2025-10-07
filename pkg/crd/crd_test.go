// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd_test

import (
	"context"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgcrd "github.com/vmware-tanzu/vm-operator/pkg/crd"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var (
	basesNonGated = []string{
		"clustervirtualmachineimages.vmoperator.vmware.com",
		"contentlibraryproviders.vmoperator.vmware.com",
		"contentsourcebindings.vmoperator.vmware.com",
		"contentsources.vmoperator.vmware.com",
		"virtualmachineclassbindings.vmoperator.vmware.com",
		"virtualmachineclasses.vmoperator.vmware.com",
		"virtualmachineimages.vmoperator.vmware.com",
		"virtualmachinepublishrequests.vmoperator.vmware.com",
		"virtualmachinereplicasets.vmoperator.vmware.com",
		"virtualmachines.vmoperator.vmware.com",
		"virtualmachineservices.vmoperator.vmware.com",
		"virtualmachinesetresourcepolicies.vmoperator.vmware.com",
		"virtualmachinewebconsolerequests.vmoperator.vmware.com",
		"webconsolerequests.vmoperator.vmware.com",
	}

	basesVMGroups = []string{
		"virtualmachinegrouppublishrequests.vmoperator.vmware.com",
		"virtualmachinegroups.vmoperator.vmware.com",
	}

	basesSnapshots = []string{
		"virtualmachinesnapshots.vmoperator.vmware.com",
	}

	basesFastDeploy = []string{
		"virtualmachineimagecaches.vmoperator.vmware.com",
	}

	basesImmutableClasses = []string{
		"virtualmachineclassinstances.vmoperator.vmware.com",
	}

	basesAll = slices.Concat(
		basesNonGated,
		basesFastDeploy,
		basesImmutableClasses,
		basesSnapshots,
		basesVMGroups,
	)

	externalBYOK = []string{
		"encryptionclasses.encryption.vmware.com",
	}

	externalVSpherePolicy = []string{
		"computepolicies.vsphere.policy.vmware.com",
		"policyevaluations.vsphere.policy.vmware.com",
		"tagpolicies.vsphere.policy.vmware.com",
	}

	externalAll = slices.Concat(
		externalBYOK,
		externalVSpherePolicy,
	)
)

func assertCRDsConsistOf[T any](
	crds []T,
	expectedNames ...string) {

	ExpectWithOffset(1, expectedNames).To(HaveLen(len(crds)))

	actualNames := make([]string, len(crds))
	for i := range crds {
		switch tCRD := (any)(crds[i]).(type) {
		case unstructured.Unstructured:
			actualNames[i] = tCRD.GetName()
		case apiextensionsv1.CustomResourceDefinition:
			actualNames[i] = tCRD.GetName()
		case *unstructured.Unstructured:
			actualNames[i] = tCRD.GetName()
		case *apiextensionsv1.CustomResourceDefinition:
			actualNames[i] = tCRD.GetName()
		}

	}

	ExpectWithOffset(1, actualNames).To(ConsistOf(expectedNames))

}

var _ = Describe("UnstructuredBases", func() {
	It("should get the expected crds", func() {
		crds, err := pkgcrd.UnstructuredBases()
		Expect(err).ToNot(HaveOccurred())
		assertCRDsConsistOf(crds, basesAll...)
	})
})

var _ = Describe("UnstructuredExternal", func() {
	It("should get the expected crds", func() {
		crds, err := pkgcrd.UnstructuredExternal()
		Expect(err).ToNot(HaveOccurred())
		assertCRDsConsistOf(crds, externalAll...)
	})
})

var _ = Describe("Install", func() {
	var (
		ctx    context.Context
		client ctrlclient.Client
	)

	BeforeEach(func() {
		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			CRDCleanupEnabled: false,
			Features: pkgcfg.FeatureStates{
				FastDeploy:       false,
				ImmutableClasses: false,
				VMGroups:         false,
				VMSnapshots:      false,
			},
		})

		scheme := runtime.NewScheme()
		Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		client = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()
	})

	JustBeforeEach(func() {
		Expect(pkgcrd.Install(ctx, client, nil)).To(Succeed())
	})

	assertField := func(expected bool, fields ...string) {
		obj := unstructured.Unstructured{
			Object: map[string]any{},
		}
		obj.SetAPIVersion("apiextensions.k8s.io/v1")
		obj.SetKind("CustomResourceDefinition")
		obj.SetName("virtualmachines.vmoperator.vmware.com")

		ExpectWithOffset(1, client.Get(
			ctx,
			ctrlclient.ObjectKeyFromObject(&obj),
			&obj)).To(Succeed())

		versions, _, err := unstructured.NestedSlice(
			obj.Object, "spec", "versions")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		hasField := false
		for j := range versions {
			v := versions[j].(map[string]any)
			_, okay, err := unstructured.NestedFieldNoCopy(
				v,
				fields...)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			if okay {
				hasField = okay
				break
			}
		}
		ExpectWithOffset(1, hasField).To(Equal(expected))
	}

	When("no crds are installed", func() {
		When("no capabilities are enabled", func() {
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})

			DescribeTable("vm api should not have spec fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"spec",
						"properties",
						field,
					}
					assertField(false, fields...)
				},
				Entry("bootOptions", "bootOptions"),
				Entry("class", "class"),
				Entry("currentSnapshotName", "currentSnapshotName"),
				Entry("groupName", "groupName"),
				Entry("policies", "policies"),
			)

			DescribeTable("vm api should not have status fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"status",
						"properties",
						field,
					}
					assertField(false, fields...)
				},
				Entry("currentSnapshot", "currentSnapshot"),
				Entry("rootSnapshots", "rootSnapshots"),
				Entry("policies", "policies"),
			)
		})

		When("byok is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.BringYourOwnEncryptionKey = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalBYOK)...)
			})
		})

		When("vSphere policies are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalVSpherePolicy)...)
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"spec",
						"properties",
						field,
					}
					assertField(true, fields...)
				},
				Entry("policies", "policies"),
			)

			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"status",
						"properties",
						field,
					}
					assertField(true, fields...)
				},
				Entry("policies", "policies"),
			)
		})

		When("groups are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMGroups = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesVMGroups)...)
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"spec",
						"properties",
						field,
					}
					assertField(true, fields...)
				},
				Entry("bootOptions", "bootOptions"),
				Entry("groupName", "groupName"),
			)
		})

		When("snapshots are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMSnapshots = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesSnapshots)...)
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"spec",
						"properties",
						field,
					}
					assertField(true, fields...)
				},
				Entry("currentSnapshotName", "currentSnapshotName"),
			)

			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"status",
						"properties",
						field,
					}
					assertField(true, fields...)
				},
				Entry("currentSnapshot", "currentSnapshot"),
				Entry("rootSnapshots", "rootSnapshots"),
			)
		})

		When("immutable classes are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.ImmutableClasses = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesImmutableClasses)...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := []string{
						"schema",
						"openAPIV3Schema",
						"properties",
						"spec",
						"properties",
						field,
					}
					assertField(true, fields...)
				},
				Entry("class", "class"),
			)
		})

		When("fast deploy is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesFastDeploy)...)
			})
		})

		When("all features are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
					config.Features.ImmutableClasses = true
					config.Features.VMGroups = true
					config.Features.VMSnapshots = true
					config.Features.VSpherePolicies = true
					config.Features.BringYourOwnEncryptionKey = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesAll, externalAll)...)
			})
		})
	})

	When("crds were already installed with caps enabled and with conversion info", func() {
		var (
			crc apiextensionsv1.CustomResourceConversion
		)

		BeforeEach(func() {

			crc = apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ConversionReviewVersions: []string{"v1"},
					ClientConfig: &apiextensionsv1.WebhookClientConfig{
						URL: ptr.To("http://127.0.0.1"),
						Service: &apiextensionsv1.ServiceReference{
							Namespace: "default",
							Name:      "webhook",
							Path:      ptr.To("/convert"),
							Port:      ptr.To(int32(443)),
						},
					},
				},
			}

			Expect(pkgcrd.Install(
				pkgcfg.WithConfig(pkgcfg.Config{
					Features: pkgcfg.FeatureStates{
						FastDeploy:                true,
						ImmutableClasses:          true,
						VMGroups:                  true,
						VMSnapshots:               true,
						VSpherePolicies:           true,
						BringYourOwnEncryptionKey: true,
					},
				}),
				client,
				func(kind string, obj *unstructured.Unstructured) error {
					if err := unstructured.SetNestedMap(
						obj.Object,
						map[string]any{
							"strategy": string(crc.Strategy),
							"webhook": map[string]any{
								"clientConfig": map[string]any{
									"url": *crc.Webhook.ClientConfig.URL,
									"service": map[string]any{
										"namespace": crc.Webhook.ClientConfig.Service.Namespace,
										"name":      crc.Webhook.ClientConfig.Service.Name,
										"path":      *crc.Webhook.ClientConfig.Service.Path,
									},
								},
							},
						},
						"spec",
						"conversion"); err != nil {
						return err
					}

					if err := unstructured.SetNestedStringSlice(
						obj.Object,
						crc.Webhook.ConversionReviewVersions,
						"spec",
						"conversion",
						"webhook",
						"conversionReviewVersions"); err != nil {
						return err
					}

					if err := unstructured.SetNestedField(
						obj.Object,
						int64(*crc.Webhook.ClientConfig.Service.Port),
						"spec",
						"conversion",
						"webhook",
						"clientConfig",
						"service",
						"port"); err != nil {
						return err
					}

					return nil

				})).To(Succeed())

			// Verify the CRDs were installed.
			var obj apiextensionsv1.CustomResourceDefinitionList
			Expect(client.List(ctx, &obj)).To(Succeed())
			assertCRDsConsistOf(obj.Items, slices.Concat(basesAll, externalAll)...)
			for i := range obj.Items {
				ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
				ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
			}
		})

		When("CRD cleanup is disabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.CRDCleanupEnabled = false
				})
			})

			When("no capabilities are enabled", func() {
				It("should get the expected crds", func() {
					var obj apiextensionsv1.CustomResourceDefinitionList
					Expect(client.List(ctx, &obj)).To(Succeed())
					assertCRDsConsistOf(obj.Items, slices.Concat(basesAll, externalAll)...)
					for i := range obj.Items {
						ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
						ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
					}
				})

				DescribeTable("vm api should have removed spec fields",
					func(field string) {
						fields := []string{
							"schema",
							"openAPIV3Schema",
							"properties",
							"spec",
							"properties",
							field,
						}
						assertField(true, fields...)
					},
					Entry("bootOptions", "bootOptions"),
					Entry("class", "class"),
					Entry("currentSnapshotName", "currentSnapshotName"),
					Entry("groupName", "groupName"),
					Entry("policies", "policies"),
				)

				DescribeTable("vm api should have removed status fields",
					func(field string) {
						fields := []string{
							"schema",
							"openAPIV3Schema",
							"properties",
							"status",
							"properties",
							field,
						}
						assertField(true, fields...)
					},
					Entry("currentSnapshot", "currentSnapshot"),
					Entry("rootSnapshots", "rootSnapshots"),
					Entry("policies", "policies"),
				)
			})
		})

		When("CRD cleanup is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.CRDCleanupEnabled = true
				})
			})
			When("no capabilities are enabled", func() {
				It("should get the expected crds", func() {
					var obj apiextensionsv1.CustomResourceDefinitionList
					Expect(client.List(ctx, &obj)).To(Succeed())
					assertCRDsConsistOf(obj.Items, basesNonGated...)
					for i := range obj.Items {
						ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
						ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
					}
				})

				DescribeTable("vm api should have removed spec fields",
					func(field string) {
						fields := []string{
							"schema",
							"openAPIV3Schema",
							"properties",
							"spec",
							"properties",
							field,
						}
						assertField(false, fields...)
					},
					Entry("bootOptions", "bootOptions"),
					Entry("class", "class"),
					Entry("currentSnapshot", "currentSnapshot"),
					Entry("groupName", "groupName"),
					Entry("policies", "policies"),
				)

				DescribeTable("vm api should have removed status fields",
					func(field string) {
						fields := []string{
							"schema",
							"openAPIV3Schema",
							"properties",
							"status",
							"properties",
							field,
						}
						assertField(false, fields...)
					},
					Entry("currentSnapshot", "currentSnapshot"),
					Entry("rootSnapshots", "rootSnapshots"),
					Entry("policies", "policies"),
				)

				When("one of the crds is already deleted", func() {
					BeforeEach(func() {
						obj := apiextensionsv1.CustomResourceDefinition{
							ObjectMeta: metav1.ObjectMeta{
								Name: "virtualmachinegroups.vmoperator.vmware.com",
							},
						}
						Expect(client.Delete(ctx, &obj)).To(Succeed())
					})

					It("should get the expected crds", func() {
						var obj apiextensionsv1.CustomResourceDefinitionList
						Expect(client.List(ctx, &obj)).To(Succeed())
						assertCRDsConsistOf(obj.Items, basesNonGated...)
						for i := range obj.Items {
							ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
							ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
						}
					})

					DescribeTable("vm api should have removed spec fields",
						func(field string) {
							fields := []string{
								"schema",
								"openAPIV3Schema",
								"properties",
								"spec",
								"properties",
								field,
							}
							assertField(false, fields...)
						},
						Entry("bootOptions", "bootOptions"),
						Entry("class", "class"),
						Entry("currentSnapshotName", "currentSnapshotName"),
						Entry("groupName", "groupName"),
						Entry("policies", "policies"),
					)

					DescribeTable("vm api should have removed status fields",
						func(field string) {
							fields := []string{
								"schema",
								"openAPIV3Schema",
								"properties",
								"status",
								"properties",
								field,
							}
							assertField(false, fields...)
						},
						Entry("currentSnapshot", "currentSnapshot"),
						Entry("rootSnapshots", "rootSnapshots"),
						Entry("policies", "policies"),
					)
				})
			})
		})
	})
})
