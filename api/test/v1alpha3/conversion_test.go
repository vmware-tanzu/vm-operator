// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3_test

import (
	"encoding/json"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"

	"github.com/vmware-tanzu/vm-operator/api/test/utilconversion/fuzztests"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
)

var _ = Describe("FuzzyConversion", Label("api", "fuzz"), func() {

	var (
		scheme *runtime.Scheme
		input  fuzztests.FuzzTestFuncInput
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(vmopv1a3.AddToScheme(scheme)).To(Succeed())
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
				Spoke:  &vmopv1a3.VirtualMachine{},
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
				Spoke:  &vmopv1a3.VirtualMachineClass{},
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
				Spoke:  &vmopv1a3.VirtualMachineImage{},
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
				Spoke:  &vmopv1a3.ClusterVirtualMachineImage{},
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
				Spoke:  &vmopv1a3.VirtualMachinePublishRequest{},
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
				Spoke:  &vmopv1a3.VirtualMachineService{},
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
				Spoke:  &vmopv1a3.VirtualMachineSetResourcePolicy{},
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
				Spoke:  &vmopv1a3.VirtualMachineWebConsoleRequest{},
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

	Context("VirtualMachineGroup", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineGroup{},
				Spoke:  &vmopv1a3.VirtualMachineGroup{},
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

var _ = Describe("Client-side conversion", func() {
	It("should convert VirtualMachine from current API version to latest API version", func() {
		scheme := runtime.NewScheme()
		Expect(vmopv1a3.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		vm1 := &vmopv1a3.VirtualMachine{}
		vm2 := &vmopv1.VirtualMachine{}
		Expect(scheme.Convert(vm1, vm2, nil)).To(Succeed())
	})
})

func overrideVirtualMachineImageFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(status *vmopv1.VirtualMachineImageStatus, c randfill.Continue) {
			c.Fill(status)
			status.Disks = nil
		},
		func(status *vmopv1a3.VirtualMachineImageStatus, c randfill.Continue) {
			c.Fill(status)
			status.Disks = nil
		},
	}
}

func overrideVirtualMachineFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(vmSpec *vmopv1a3.VirtualMachineSpec, c randfill.Continue) {
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
		func(vmStatus *vmopv1a3.VirtualMachineStatus, c randfill.Continue) {
			c.Fill(vmStatus)
		},
		func(msg *json.RawMessage, c randfill.Continue) {
			*msg = []byte(`{"foo": "bar"}`)
		},
	}
}

func ptrOf[T any](v T) *T {
	return &v
}
