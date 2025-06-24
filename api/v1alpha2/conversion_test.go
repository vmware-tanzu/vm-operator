// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"encoding/json"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion/fuzztests"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha4/sysprep"
)

var _ = Describe("FuzzyConversion", Label("api", "fuzz"), func() {

	var (
		scheme *runtime.Scheme
		input  fuzztests.FuzzTestFuncInput
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(vmopv1a2.AddToScheme(scheme)).To(Succeed())
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
				Spoke:  &vmopv1a2.VirtualMachine{},
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
				Spoke:  &vmopv1a2.VirtualMachineClass{},
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
				Spoke:  &vmopv1a2.VirtualMachineImage{},
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
				Spoke:  &vmopv1a2.ClusterVirtualMachineImage{},
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
				Spoke:  &vmopv1a2.VirtualMachinePublishRequest{},
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
				Spoke:  &vmopv1a2.VirtualMachineService{},
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
				Spoke:  &vmopv1a2.VirtualMachineSetResourcePolicy{},
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

	Context("VirtualMachineWebConsoleRequest", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineWebConsoleRequest{},
				Spoke:  &vmopv1a2.VirtualMachineWebConsoleRequest{},
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
		func(vmSpec *vmopv1a2.VirtualMachineSpec, c randfill.Continue) {
			c.Fill(vmSpec)

			if bs := vmSpec.Bootstrap; bs != nil {
				if bs.Sysprep != nil && bs.Sysprep.Sysprep != nil {
					sysPrep := vmSpec.Bootstrap.Sysprep.Sysprep

					// In v1a3, GUIRunOnce was changed to a pointer field. Change the empty slice into a nil
					// field so reflect.DeepEqual() determines correctly if GUIRunOnce is unset.
					if len(sysPrep.GUIRunOnce.Commands) == 0 {
						sysPrep.GUIRunOnce.Commands = nil
					}
				}
			}
		},
		func(vmSpec *vmopv1.VirtualMachineSpec, c randfill.Continue) {
			c.Fill(vmSpec)

			if bs := vmSpec.Bootstrap; bs != nil {
				if bs.Sysprep != nil && bs.Sysprep.Sysprep != nil {
					sysPrep := vmSpec.Bootstrap.Sysprep

					// Match the check done in sysprep conversion.
					if reflect.DeepEqual(sysPrep.Sysprep, &vmopv1sysprep.Sysprep{}) {
						sysPrep.Sysprep = nil
					}
				}
			}
		},
		func(vmStatus *vmopv1a2.VirtualMachineStatus, c randfill.Continue) {
			c.Fill(vmStatus)

			// This field was removed in v1a3.
			vmStatus.Image = nil
		},
		func(msg *json.RawMessage, c randfill.Continue) {
			*msg = []byte(`{"foo": "bar"}`)
		},
	}
}

func overrideVirtualMachineImageFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(vmiStatus *vmopv1.VirtualMachineImageStatus, c randfill.Continue) {
			c.Fill(vmiStatus)

			// Since only VMOP updates the CVMI/VMI's we didn't bother with conversion
			// when adding this field.
			vmiStatus.Disks = nil
		},
	}
}

func ptrOf[T any](v T) *T {
	return &v
}
