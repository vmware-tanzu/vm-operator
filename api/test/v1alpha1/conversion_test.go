// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"encoding/json"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/randfill"

	"github.com/vmware-tanzu/vm-operator/api/test/utilconversion/fuzztests"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
)

var _ = Describe("FuzzyConversion", Label("api", "fuzz"), func() {

	var (
		scheme *runtime.Scheme
		input  fuzztests.FuzzTestFuncInput
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(vmopv1a1.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
	})

	AfterEach(func() {
		input = fuzztests.FuzzTestFuncInput{}
	})

	Context("VirtualMachine", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachine{},
				Spoke:  &vmopv1a1.VirtualMachine{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineClass", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineClass{},
				Spoke:  &vmopv1a1.VirtualMachineClass{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineClassFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineImage", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineImage{},
				Spoke:  &vmopv1a1.VirtualMachineImage{},
				SpokeAfterMutation: func(convertible ctrlconversion.Convertible) {
					vmImage := convertible.(*vmopv1a1.VirtualMachineImage)
					if vmImage.Spec.ProviderRef.Name != "" {
						vmImage.Spec.ProviderRef.Namespace = vmImage.Namespace
					}
				},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineImageFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})

	Context("ClusterVirtualMachineImage", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.ClusterVirtualMachineImage{},
				Spoke:  &vmopv1a1.ClusterVirtualMachineImage{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachineImageFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachinePublishRequest", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachinePublishRequest{},
				Spoke:  &vmopv1a1.VirtualMachinePublishRequest{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{
					overrideVirtualMachinePublishRequestFieldsFuncs,
				},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineService", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineService{},
				Spoke:  &vmopv1a1.VirtualMachineService{},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})

	Context("VirtualMachineSetResourcePolicy", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineSetResourcePolicy{},
				Spoke:  &vmopv1a1.VirtualMachineSetResourcePolicy{},
			}
		})
		Context("Spoke-Hub-Spoke", func() {
			It("should get fuzzy with it", func() {
				fuzztests.SpokeHubSpoke(input)
			})
		})
		Context("Hub-Spoke-Hub", func() {
			It("should get fuzzy with it", func() {
				fuzztests.HubSpokeHub(input)
			})
		})
	})
})

func overrideVirtualMachineFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(vmSpec *vmopv1a1.VirtualMachineSpec, c randfill.Continue) {
			c.FillNoCustom(vmSpec)

			if md := vmSpec.VmMetadata; md != nil {
				t := []vmopv1a1.VirtualMachineMetadataTransport{
					vmopv1a1.VirtualMachineMetadataCloudInitTransport,
					vmopv1a1.VirtualMachineMetadataOvfEnvTransport,
					vmopv1a1.VirtualMachineMetadataSysprepTransport,
					vmopv1a1.VirtualMachineMetadataVAppConfigTransport,
				}
				md.Transport = t[c.Intn(len(t))]

				if md.SecretName != "" && md.ConfigMapName != "" {
					if c.Bool() {
						md.SecretName = ""
					} else {
						md.ConfigMapName = ""
					}
				}
			}

			for i := range vmSpec.NetworkInterfaces {
				t := []string{"nsx-t", "nsx-t-subnet", "nsx-t-subnetset", "vsphere-distributed"}
				vmSpec.NetworkInterfaces[i].NetworkType = t[c.Intn(len(t))]
				vmSpec.NetworkInterfaces[i].ProviderRef = nil
				vmSpec.NetworkInterfaces[i].EthernetCardType = ""
			}

			var volumes []vmopv1a1.VirtualMachineVolume
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

				// TODO: Conversion
				vmSpec.AdvancedOptions = nil
			}

			// This is effectively deprecated.
			vmSpec.Ports = nil
		},
		func(vmSpec *vmopv1.VirtualMachineSpec, c randfill.Continue) {
			c.FillNoCustom(vmSpec)

			if vmSpec.Image != nil {
				vmSpec.Image.Name = vmSpec.ImageName
			}

			// TODO: Conversion
			if vmSpec.Class != nil {
				vmSpec.Class = nil
			}

			vmSpec.Volumes = nil

			if rs := vmSpec.Reserved; rs != nil {
				if rs.ResourcePolicyName == "" {
					vmSpec.Reserved = nil
				}
			}

			if bs := vmSpec.Bootstrap; bs != nil {
				// v1a1 has just single for the bootstrap type field (transport) so
				// adjust the filled to valid bootstrap combinations.
				switch c.Rand.Intn(4) {
				case 0: // CloudInit
					bs.LinuxPrep = nil
					bs.Sysprep = nil
					bs.VAppConfig = nil
				case 1: // LinuxPrep
					bs.CloudInit = nil
					bs.Sysprep = nil
					if c.Bool() {
						bs.VAppConfig = nil
					}
				case 2: // Sysprep
					bs.CloudInit = nil
					bs.LinuxPrep = nil
					if c.Bool() {
						bs.VAppConfig = nil
					}
				case 3: // vAppConfig
					bs.CloudInit = nil
					bs.LinuxPrep = nil
					bs.Sysprep = nil
				}

				if reflect.DeepEqual(bs, &vmopv1.VirtualMachineBootstrapSpec{}) {
					vmSpec.Bootstrap = nil
				}
			}
		},
		func(vmStatus *vmopv1a1.VirtualMachineStatus, c randfill.Continue) {
			c.FillNoCustom(vmStatus)
			overrideConditions(vmStatus.Conditions)

			for i := range vmStatus.NetworkInterfaces {
				// Connected does not exist in nextver so assume it is true.
				vmStatus.NetworkInterfaces[i].Connected = true
			}

			// Does not exist in nextver.
			vmStatus.Phase = vmopv1a1.Unknown
		},
		func(vmStatus *vmopv1.VirtualMachineStatus, c randfill.Continue) {
			c.FillNoCustom(vmStatus)
			overrideConditionsObservedGeneration(vmStatus.Conditions)

			vmStatus.Class = nil
			vmStatus.Network = nil
		},
		func(sel *vmopv1common.SecretKeySelector, c randfill.Continue) {
			sel.Name = "foo"
			sel.Key = c.String(0)
		},
		func(msg *json.RawMessage, c randfill.Continue) {
			*msg = []byte(`{"foo":"bar"}`)
		},
	}
}

var _ = Describe("Client-side conversion", func() {
	It("should convert VirtualMachine from current API version to latest API version", func() {
		scheme := runtime.NewScheme()
		Expect(vmopv1a1.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		vm1 := &vmopv1a1.VirtualMachine{}
		vm2 := &vmopv1.VirtualMachine{}
		Expect(scheme.Convert(vm1, vm2, nil)).To(Succeed())
	})
})

func overrideVirtualMachineClassFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(classSpec *vmopv1.VirtualMachineClassSpec, c randfill.Continue) {
			c.FillNoCustom(classSpec)

			// Since all random byte arrays are not valid JSON
			// Passing an empty string as a valid input
			classSpec.ConfigSpec = []byte("")
		},
		func(classSpec *vmopv1a1.VirtualMachineClassSpec, c randfill.Continue) {
			c.FillNoCustom(classSpec)

			// Since all random byte arrays are not valid JSON
			// Passing an empty string as a valid input
			classSpec.ConfigSpec = []byte("")
		},
	}
}

func overrideVirtualMachineImageFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(imageSpec *vmopv1a1.VirtualMachineImageSpec, c randfill.Continue) {
			c.FillNoCustom(imageSpec)

			// This same type is used in both the namespace and cluster
			// scoped types so we don't determine the NS backfill here.
			imageSpec.ProviderRef = vmopv1a1.ContentProviderReference{}

			if imageSpec.OVFEnv != nil {
				m := make(map[string]vmopv1a1.OvfProperty, len(imageSpec.OVFEnv))
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
		func(imageStatus *vmopv1a1.VirtualMachineImageStatus, c randfill.Continue) {
			c.FillNoCustom(imageStatus)

			overrideConditions(imageStatus.Conditions)

			// TODO: Figure out the default ready conditions backfill.
			imageStatus.Conditions = nil

			// Do not exist in nextver.
			imageStatus.ImageSupported = nil

			// These are deprecated.
			imageStatus.Uuid = ""
			imageStatus.InternalId = ""
			imageStatus.PowerState = ""

			// This is backed from annotation.
			imageStatus.ContentLibraryRef = nil //nolint:staticcheck
		},
		func(osInfo *vmopv1.VirtualMachineImageOSInfo, c randfill.Continue) {
			c.FillNoCustom(osInfo)
			// TODO: Need to save serialized object to support lossless conversions.
			osInfo.ID = ""
		},
		func(imageStatus *vmopv1.VirtualMachineImageStatus, c randfill.Continue) {
			c.FillNoCustom(imageStatus)

			for i := range imageStatus.VMwareSystemProperties {
				k := imageStatus.VMwareSystemProperties[i].Key
				imageStatus.VMwareSystemProperties[i].Key = "vmware-system-" + k
			}

			overrideConditionsObservedGeneration(imageStatus.Conditions)
			imageStatus.Conditions = nil

			// TODO: Need to save serialized object to support lossless conversions.
			imageStatus.Type = ""
			imageStatus.Disks = nil
			imageStatus.Capabilities = nil
		},
		func(imageSpec *vmopv1.VirtualMachineImageSpec, c randfill.Continue) {
			c.FillNoCustom(imageSpec)
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
		func(publishStatus *vmopv1a1.VirtualMachinePublishRequestStatus, c randfill.Continue) {
			c.Fill(publishStatus)
			overrideConditions(publishStatus.Conditions)
		},
		func(publishStatus *vmopv1.VirtualMachinePublishRequestStatus, c randfill.Continue) {
			c.Fill(publishStatus)
			overrideConditionsObservedGeneration(publishStatus.Conditions)
		},
	}
}

func overrideConditions(conditions []vmopv1a1.Condition) {
	for i := range conditions {
		// metav1.Conditions do not have this field, so on down conversions it
		// will always be empty.
		conditions[i].Severity = ""

		// metav1.Conditions required this field so we'll backfill it so set it
		// to some value.
		if conditions[i].Reason == "" {
			conditions[i].Reason = "reason is now required"
		}
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
