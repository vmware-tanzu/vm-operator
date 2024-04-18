// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fuzz "github.com/google/gofuzz"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	nextver "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

var _ = Describe("FuzzyConversion", Label(testlabels.API, testlabels.Fuzz), func() {

	var (
		scheme *runtime.Scheme
		input  utilconversion.FuzzTestFuncInput
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(nextver.AddToScheme(scheme)).To(Succeed())
	})

	AfterEach(func() {
		input = utilconversion.FuzzTestFuncInput{}
	})

	Context("VirtualMachine", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.VirtualMachine{},
				Spoke:  &v1alpha1.VirtualMachine{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineClass", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.VirtualMachineClass{},
				Spoke:  &v1alpha1.VirtualMachineClass{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineClassFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineImage", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.VirtualMachineImage{},
				Spoke:  &v1alpha1.VirtualMachineImage{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineImageFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})

	Context("ClusterVirtualMachineImage", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.ClusterVirtualMachineImage{},
				Spoke:  &v1alpha1.ClusterVirtualMachineImage{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineImageFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachinePublishRequest", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.VirtualMachinePublishRequest{},
				Spoke:  &v1alpha1.VirtualMachinePublishRequest{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachinePublishRequestFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineService", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.VirtualMachineService{},
				Spoke:  &v1alpha1.VirtualMachineService{},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})
	Context("VirtualMachineSetResourcePolicy", func() {
		BeforeEach(func() {
			input = utilconversion.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &nextver.VirtualMachineSetResourcePolicy{},
				Spoke:  &v1alpha1.VirtualMachineSetResourcePolicy{},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				utilconversion.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				utilconversion.HubSpokeHub(input)
			})
		})
	})
})

func overrideVirtualMachineFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(vmSpec *v1alpha1.VirtualMachineSpec, c fuzz.Continue) {
			c.Fuzz(vmSpec)

			var volumes []v1alpha1.VirtualMachineVolume
			for _, vol := range vmSpec.Volumes {
				// vSphere volumes are gone in nextver so skip those.
				if vol.VsphereVolume == nil {
					volumes = append(volumes, vol)
				}
			}
			vmSpec.Volumes = volumes

			if opts := vmSpec.AdvancedOptions; opts != nil {
				if provOpts := opts.DefaultVolumeProvisioningOptions; provOpts != nil {
					if provOpts.ThinProvisioned != nil {
						// Both cannot be set.
						provOpts.EagerZeroed = nil
					}
				}

				if opts.ChangeBlockTracking != nil && !*opts.ChangeBlockTracking {
					opts.ChangeBlockTracking = nil
				}
			}

			// This is effectively deprecated.
			vmSpec.Ports = nil
		},
		func(vmSpec *nextver.VirtualMachineSpec, c fuzz.Continue) {
			c.Fuzz(vmSpec)
		},
		func(vmStatus *v1alpha1.VirtualMachineStatus, c fuzz.Continue) {
			c.Fuzz(vmStatus)
			overrideConditionsSeverity(vmStatus.Conditions)

			// Do not exist in nextver.
			vmStatus.Phase = v1alpha1.Unknown
		},
		func(vmStatus *nextver.VirtualMachineStatus, c fuzz.Continue) {
			c.Fuzz(vmStatus)
			overrideConditionsObservedGeneration(vmStatus.Conditions)

			vmStatus.Class = nil
			vmStatus.Network = nil
		},
	}
}

func overrideVirtualMachineClassFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(classSpec *nextver.VirtualMachineClassSpec, c fuzz.Continue) {
			c.Fuzz(classSpec)

			// Since all random byte arrays are not valid JSON
			// Passing an empty string as a valid input
			classSpec.ConfigSpec = []byte("")
		},
		func(classSpec *v1alpha1.VirtualMachineClassSpec, c fuzz.Continue) {
			c.Fuzz(classSpec)

			// Since all random byte arrays are not valid JSON
			// Passing an empty string as a valid input
			classSpec.ConfigSpec = []byte("")
		},
	}
}

func overrideVirtualMachineImageFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(imageSpec *v1alpha1.VirtualMachineImageSpec, c fuzz.Continue) {
			c.Fuzz(imageSpec)

			if imageSpec.OVFEnv != nil {
				m := make(map[string]v1alpha1.OvfProperty, len(imageSpec.OVFEnv))
				for k, v := range imageSpec.OVFEnv {
					// In practice, the value key always will be the map key.
					v.Key = k
					// Do not exist in nextver.
					v.Description = ""
					v.Label = ""

					m[k] = v
				}
				imageSpec.OVFEnv = m
			}

			// Do not exist in nextver.
			imageSpec.Type = ""
			imageSpec.ImageSourceType = ""
			imageSpec.ProviderRef.Namespace = ""
		},
		func(imageStatus *v1alpha1.VirtualMachineImageStatus, c fuzz.Continue) {
			c.Fuzz(imageStatus)
			overrideConditionsSeverity(imageStatus.Conditions)

			// Do not exist in nextver.
			//imageStatus.ContentLibraryRef = nil
			imageStatus.ImageSupported = nil

			// These are deprecated.
			imageStatus.Uuid = ""
			imageStatus.InternalId = ""
			imageStatus.PowerState = ""
		},
		func(osInfo *nextver.VirtualMachineImageOSInfo, c fuzz.Continue) {
			c.Fuzz(osInfo)
			// TODO: Need to save serialized object to support lossless conversions.
			osInfo.ID = ""
		},
		func(imageStatus *nextver.VirtualMachineImageStatus, c fuzz.Continue) {
			c.Fuzz(imageStatus)
			overrideConditionsObservedGeneration(imageStatus.Conditions)
			// TODO: Need to save serialized object to support lossless conversions.
			imageStatus.Capabilities = nil
		},
		func(imageSpec *nextver.VirtualMachineImageSpec, c fuzz.Continue) {
			c.Fuzz(imageSpec)
			if pr := imageSpec.ProviderRef; pr != nil {
				if pr.APIVersion == "" && pr.Kind == "" && pr.Name == "" {
					imageSpec.ProviderRef = nil
				}
			}
		},
	}
}

func overrideVirtualMachinePublishRequestFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(publishStatus *v1alpha1.VirtualMachinePublishRequestStatus, c fuzz.Continue) {
			c.Fuzz(publishStatus)
			overrideConditionsSeverity(publishStatus.Conditions)
		},
		func(publishStatus *nextver.VirtualMachinePublishRequestStatus, c fuzz.Continue) {
			c.Fuzz(publishStatus)
			overrideConditionsObservedGeneration(publishStatus.Conditions)
		},
	}
}

func overrideConditionsSeverity(conditions []v1alpha1.Condition) {
	// metav1.Conditions do not have this field, so on down conversions it will always be empty.
	for i := range conditions {
		conditions[i].Severity = ""
	}
}

func overrideConditionsObservedGeneration(conditions []metav1.Condition) {
	// We'd need to add this field to our v1a1 Condition to support down conversions.
	for i := range conditions {
		conditions[i].ObservedGeneration = 0
	}
}

func ptrOf[T any](v T) *T {
	return &v
}
