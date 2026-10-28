// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"encoding/json"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"

	"github.com/vmware-tanzu/vm-operator/api/test/utilconversion/fuzztests"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

var _ = Describe("FuzzyConversion", Label("api", "fuzz"), func() {

	var (
		scheme *runtime.Scheme
		input  fuzztests.FuzzTestFuncInput
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(vmopv1a5.AddToScheme(scheme)).To(Succeed())
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
				Spoke:  &vmopv1a5.VirtualMachine{},
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
				HubAfterMutation: func(h ctrlconversion.Hub) {
					if m := h.(metav1.Object); len(m.GetAnnotations()) == 0 {
						m.SetAnnotations(nil)
					}
				},
				SpokeAfterMutation: func(h ctrlconversion.Convertible) {
					if m := h.(metav1.Object); len(m.GetAnnotations()) == 0 {
						m.SetAnnotations(nil)
					}
				},
				Hub:         &vmopv1.VirtualMachineClass{},
				Spoke:       &vmopv1a5.VirtualMachineClass{},
				FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzztests.ClassPolicyFuzzerFuncs, overrideVirtualMachineFieldsFuncs},
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
				Spoke:  &vmopv1a5.VirtualMachineGroup{},
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
				Spoke:  &vmopv1a5.VirtualMachineImage{},
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
				Spoke:  &vmopv1a5.ClusterVirtualMachineImage{},
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

	Context("VirtualMachineImageCache", func() {
		BeforeEach(func() {
			input = fuzztests.FuzzTestFuncInput{
				Scheme: scheme,
				Hub:    &vmopv1.VirtualMachineImageCache{},
				Spoke:  &vmopv1a5.VirtualMachineImageCache{},
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
				Spoke:  &vmopv1a5.VirtualMachinePublishRequest{},
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
				Spoke:  &vmopv1a5.VirtualMachineService{},
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
				Spoke:  &vmopv1a5.VirtualMachineSetResourcePolicy{},
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
				Spoke:  &vmopv1a5.VirtualMachineWebConsoleRequest{},
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
		Expect(vmopv1a5.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		vm1 := &vmopv1a5.VirtualMachine{}
		vm2 := &vmopv1.VirtualMachine{}
		Expect(scheme.Convert(vm1, vm2, nil)).To(Succeed())
	})
})

func overrideVirtualMachineFieldsFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(msg *json.RawMessage, c randfill.Continue) {
			*msg = []byte(`{"foo":"bar"}`)
		},
	}
}

var _ = Describe("VMClass schema migration", Label("api"), func() {
	for _, version := range []struct {
		name     string
		newSpoke func() ctrlconversion.Convertible
	}{
		{"v1alpha1", func() ctrlconversion.Convertible { return &vmopv1a1.VirtualMachineClass{} }},
		{"v1alpha2", func() ctrlconversion.Convertible { return &vmopv1a2.VirtualMachineClass{} }},
		{"v1alpha3", func() ctrlconversion.Convertible { return &vmopv1a3.VirtualMachineClass{} }},
		{"v1alpha4", func() ctrlconversion.Convertible { return &vmopv1a4.VirtualMachineClass{} }},
		{"v1alpha5", func() ctrlconversion.Convertible { return &vmopv1a5.VirtualMachineClass{} }},
	} {
		Context(version.name, func() {
			It("moves legacy scalars without changing units, devices, storage or reservation slots", func() {
				spoke := version.newSpoke()
				Expect(json.Unmarshal([]byte(`{
					"metadata":{"name":"legacy","namespace":"tenant"},
					"spec":{
						"hardware":{"cpus":4,"memory":"4Gi","devices":{
							"vgpuDevices":[{"profileName":"gpu-a"}],
							"dynamicDirectPathIODevices":[{"vendorID":1,"deviceID":2,"customLabel":"a"},{"vendorID":1,"deviceID":2,"customLabel":"b"}]},
							"instanceStorage":{"storageClass":"local","volumes":[{"size":"16Gi"}]}},
						"policies":{"resources":{"requests":{"cpu":"2","memory":"1Gi"},"limits":{"cpu":"4","memory":"8Gi"}}},
						"configSpec":{"_typeName":"VirtualMachineConfigSpec","annotation":"keep"},
						"reservedProfileID":"profile-a","reservedSlots":0
					}}`), spoke)).To(Succeed())
				before := spoke.DeepCopyObject()
				var hub vmopv1.VirtualMachineClass
				Expect(spoke.ConvertTo(&hub)).To(Succeed())
				Expect(hub.Spec.Resources.Size.CPU.Default.String()).To(Equal("4"))
				Expect(hub.Spec.Resources.Size.CPU.Min.Cmp(*hub.Spec.Resources.Size.CPU.Default)).To(BeZero())
				Expect(hub.Spec.Resources.Size.CPU.Max.Cmp(*hub.Spec.Resources.Size.CPU.Default)).To(BeZero())
				Expect(hub.Spec.Resources.Size.Memory.Default.String()).To(Equal("4Gi"))
				Expect(hub.Spec.Resources.Requests.CPU.Default.String()).To(Equal("2"))
				Expect(hub.Spec.Resources.Requests.Memory.Default.String()).To(Equal("1Gi"))
				Expect(hub.Spec.Resources.Limits.CPU.Default.String()).To(Equal("4"))
				Expect(hub.Spec.Resources.Limits.Memory.Default.String()).To(Equal("8Gi"))
				Expect(hub.Spec.ReservedProfileID).To(Equal("profile-a"))
				Expect(hub.Spec.ReservedSlots).To(BeZero())
				Expect(hub.Spec.Hardware.Devices.VGPUDevices).To(Equal([]vmopv1.VGPUDevice{{ProfileName: "gpu-a"}}))
				Expect(hub.Spec.Hardware.Devices.DynamicDirectPathIODevices).To(Equal([]vmopv1.DynamicDirectPathIODevice{
					{VendorID: 1, DeviceID: 2, CustomLabel: "a"}, {VendorID: 1, DeviceID: 2, CustomLabel: "b"},
				}))
				Expect(hub.Spec.Hardware.InstanceStorage.StorageClass).To(Equal("local"))
				Expect(hub.Spec.Hardware.InstanceStorage.Volumes[0].Size.String()).To(Equal("16Gi"))
				Expect(hub.Spec.ConfigSpec).To(MatchJSON(`{"_typeName":"VirtualMachineConfigSpec","annotation":"keep"}`))
				Expect(apiequality.Semantic.DeepEqual(spoke, before)).To(BeTrue())
			})

			It("preserves policies through an unchanged older-client write", func() {
				hub := classWithPolicies()
				spoke := version.newSpoke()
				Expect(spoke.ConvertFrom(hub)).To(Succeed())
				before := spoke.DeepCopyObject()
				var restored vmopv1.VirtualMachineClass
				Expect(spoke.ConvertTo(&restored)).To(Succeed())
				Expect(restored.Spec).To(Equal(hub.Spec))
				Expect(restored.Annotations).NotTo(HaveKey(utilconversion.AnnotationKey))
				Expect(apiequality.Semantic.DeepEqual(spoke, before)).To(BeTrue())
				Expect(hub.Annotations).NotTo(HaveKey(utilconversion.AnnotationKey))
			})

			It("copies metadata before preserving class policies", func() {
				hub := classWithPolicies()
				hub.Annotations = map[string]string{"user": "keep"}
				hub.Labels = map[string]string{"user": "keep"}
				before := hub.DeepCopy()
				spoke := version.newSpoke()
				Expect(spoke.ConvertFrom(hub)).To(Succeed())
				Expect(hub).To(Equal(before))
				spoke.(metav1.Object).GetLabels()["user"] = "changed"
				spoke.(metav1.Object).GetAnnotations()["user"] = "changed"
				Expect(hub).To(Equal(before))
			})

			It("honors older-writer edits without discarding other policies", func() {
				hub := classWithPolicies()
				spoke := version.newSpoke()
				Expect(spoke.ConvertFrom(hub)).To(Succeed())
				Expect(json.Unmarshal([]byte(`{"spec":{"hardware":{"cpus":8},"reservedProfileID":"profile-b","reservedSlots":0}}`), spoke)).To(Succeed())
				var restored vmopv1.VirtualMachineClass
				Expect(spoke.ConvertTo(&restored)).To(Succeed())
				Expect(restored.Spec.Resources.Size.CPU.Default.String()).To(Equal("8"))
				Expect(restored.Spec.Resources.Size.CPU.Min.String()).To(Equal("8"))
				Expect(restored.Spec.Resources.Size.CPU.Max.String()).To(Equal("8"))
				Expect(restored.Spec.Resources.Size.Memory).To(Equal(hub.Spec.Resources.Size.Memory))
				Expect(restored.Spec.Hardware.Devices).To(Equal(hub.Spec.Hardware.Devices))
				Expect(restored.Spec.ReservedProfileID).To(Equal("profile-b"))
				Expect(restored.Spec.ReservedSlots).To(BeZero())
			})

			It("preserves nil, explicit zero and empty list policy values", func() {
				zero := resource.MustParse("0")
				empty := []vmopv1.VGPUDevice{}
				hub := &vmopv1.VirtualMachineClass{Spec: vmopv1.VirtualMachineClassSpec{
					Resources: vmopv1.VirtualMachineClassResources{Requests: vmopv1.VirtualMachineClassResourceSpec{CPU: &vmopv1.ResourceQuantityRange{Default: &zero}}},
					Hardware:  vmopv1.VirtualMachineClassHardware{Devices: vmopv1.VirtualDevices{VGPUDevices: empty}},
				}}
				spoke := version.newSpoke()
				Expect(spoke.ConvertFrom(hub)).To(Succeed())
				var restored vmopv1.VirtualMachineClass
				Expect(spoke.ConvertTo(&restored)).To(Succeed())
				Expect(restored.Spec.Resources.Size.CPU).To(BeNil())
				Expect(restored.Spec.Resources.Requests.CPU.Default).NotTo(BeNil())
				Expect(restored.Spec.Resources.Requests.CPU.Default.IsZero()).To(BeTrue())
				Expect(restored.Spec.Resources.Requests.CPU.Min).To(BeNil())
				Expect(restored.Spec.Hardware.Devices.VGPUDevices).NotTo(BeNil())
				Expect(restored.Spec.Hardware.Devices.VGPUDevices).To(BeEmpty())
			})

			It("returns malformed conversion data without modifying the input", func() {
				spoke := version.newSpoke()
				spoke.(metav1.Object).SetAnnotations(map[string]string{utilconversion.AnnotationKey: "{"})
				before := spoke.DeepCopyObject()
				Expect(spoke.ConvertTo(&vmopv1.VirtualMachineClass{})).NotTo(Succeed())
				Expect(apiequality.Semantic.DeepEqual(spoke, before)).To(BeTrue())
			})
		})
	}

	It("preserves policies in client-side scheme and list conversion", func() {
		scheme := runtime.NewScheme()
		Expect(vmopv1a5.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		hub := &vmopv1.VirtualMachineClassList{Items: []vmopv1.VirtualMachineClass{*classWithPolicies()}}
		var spoke vmopv1a5.VirtualMachineClassList
		Expect(scheme.Convert(hub, &spoke, nil)).To(Succeed())
		var restored vmopv1.VirtualMachineClassList
		Expect(scheme.Convert(&spoke, &restored, nil)).To(Succeed())
		Expect(restored.Items[0].Spec).To(Equal(hub.Items[0].Spec))
	})

	for _, version := range []struct {
		name     string
		newSpoke func() ctrlconversion.Convertible
	}{
		{"v1alpha4", func() ctrlconversion.Convertible { return &vmopv1a4.VirtualMachineClassInstance{} }},
		{"v1alpha5", func() ctrlconversion.Convertible { return &vmopv1a5.VirtualMachineClassInstance{} }},
	} {
		Context(version.name+" class instances", func() {
			It("preserves policies and isolates source metadata in both directions", func() {
				hub := &vmopv1.VirtualMachineClassInstance{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"user": "keep"}, Labels: map[string]string{"user": "keep"}},
					Spec:       vmopv1.VirtualMachineClassInstanceSpec{VirtualMachineClassSpec: classWithPolicies().Spec},
				}
				beforeHub := hub.DeepCopy()
				spoke := version.newSpoke()
				Expect(spoke.ConvertFrom(hub)).To(Succeed())
				Expect(hub).To(Equal(beforeHub))
				beforeSpoke := spoke.DeepCopyObject()
				var restored vmopv1.VirtualMachineClassInstance
				Expect(spoke.ConvertTo(&restored)).To(Succeed())
				Expect(restored.Spec).To(Equal(hub.Spec))
				Expect(restored.Annotations).NotTo(HaveKey(utilconversion.AnnotationKey))
				restored.Labels["user"] = "changed"
				restored.Annotations["user"] = "changed"
				Expect(spoke).To(Equal(beforeSpoke))
				spoke.(metav1.Object).GetLabels()["user"] = "changed"
				spoke.(metav1.Object).GetAnnotations()["user"] = "changed"
				Expect(hub).To(Equal(beforeHub))
			})
			It("returns malformed annotations without changing source metadata", func() {
				spoke := version.newSpoke()
				spoke.(metav1.Object).SetAnnotations(map[string]string{utilconversion.AnnotationKey: "{", "user": "keep"})
				before := spoke.DeepCopyObject()
				Expect(spoke.ConvertTo(&vmopv1.VirtualMachineClassInstance{})).To(MatchError(ContainSubstring("restore VMClassInstance conversion data")))
				Expect(spoke).To(Equal(before))
			})
		})
	}

})

func classWithPolicies() *vmopv1.VirtualMachineClass {
	minimum, maximum, def := resource.MustParse("2"), resource.MustParse("16"), resource.MustParse("4")
	memory := resource.MustParse("4Gi")
	devices := []vmopv1.VGPUDevice{{ProfileName: "gpu-a"}}
	return &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: "class", Namespace: "tenant"},
		Spec: vmopv1.VirtualMachineClassSpec{
			Resources: vmopv1.VirtualMachineClassResources{Size: vmopv1.VirtualMachineClassResourceSpec{
				CPU:    &vmopv1.ResourceQuantityRange{Min: &minimum, Max: &maximum, Default: &def},
				Memory: &vmopv1.ResourceQuantityRange{Default: &memory},
			}},
			Hardware:          vmopv1.VirtualMachineClassHardware{Devices: vmopv1.VirtualDevices{VGPUDevices: devices}},
			ReservedProfileID: "profile-a",
			ReservedSlots:     4,
		},
	}
}
