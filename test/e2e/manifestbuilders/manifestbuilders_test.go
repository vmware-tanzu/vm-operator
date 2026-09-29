// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/randfill"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	mb "github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
)

func minimalVM() mb.VirtualMachineYaml {
	return mb.VirtualMachineYaml{
		Namespace:        "my-ns",
		Name:             "my-vm",
		VMClassName:      "best-effort-small",
		StorageClassName: "wcpglobal-storage-profile",
		ImageName:        "vmi-0123456789",
		PowerState:       "PoweredOn",
	}
}

// parseDocs splits a multi-document manifest into its documents.
func parseDocs(manifest []byte) []*unstructured.Unstructured {
	GinkgoHelper()
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(manifest)))
	var docs []*unstructured.Unstructured
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return docs
		}
		Expect(err).ToNot(HaveOccurred())
		u := &unstructured.Unstructured{}
		Expect(yaml.Unmarshal(doc, &u.Object)).To(Succeed(), string(doc))
		docs = append(docs, u)
	}
}

func toJSON(v any) string {
	GinkgoHelper()
	b, err := json.Marshal(v)
	Expect(err).ToNot(HaveOccurred())
	return string(b)
}

var _ = Describe("Manifest builders", func() {

	DescribeTable("render a single document of the expected API version and kind",
		func(manifest func() []byte, apiVersion, kind string) {
			docs := parseDocs(manifest())
			Expect(docs).To(HaveLen(1))
			Expect(docs[0].GetAPIVersion()).To(Equal(apiVersion))
			Expect(docs[0].GetKind()).To(Equal(kind))
			Expect(docs[0].Object).ToNot(HaveKey("status"))
			Expect(docs[0].Object["metadata"]).ToNot(HaveKey("creationTimestamp"))
		},
		Entry("VirtualMachine v1alpha1",
			func() []byte { return mb.GetVirtualMachineYaml(minimalVM()) },
			"vmoperator.vmware.com/v1alpha1", "VirtualMachine"),
		Entry("VirtualMachine v1alpha2",
			func() []byte { return mb.GetVirtualMachineYamlA2(minimalVM()) },
			"vmoperator.vmware.com/v1alpha2", "VirtualMachine"),
		Entry("VirtualMachine v1alpha3",
			func() []byte { return mb.GetVirtualMachineYamlA3(minimalVM()) },
			"vmoperator.vmware.com/v1alpha3", "VirtualMachine"),
		Entry("VirtualMachine v1alpha5",
			func() []byte { return mb.GetVirtualMachineYamlA5(minimalVM()) },
			"vmoperator.vmware.com/v1alpha5", "VirtualMachine"),
		Entry("VirtualMachine v1alpha6",
			func() []byte { return mb.GetVirtualMachineYamlA6(minimalVM()) },
			"vmoperator.vmware.com/v1alpha6", "VirtualMachine"),
		Entry("PersistentVolumeClaim",
			func() []byte {
				return mb.GetPersistentVolumeClaimYaml(mb.PVC{ClaimName: "claim", Namespace: "my-ns", RequestSize: "1Gi"})
			},
			"v1", "PersistentVolumeClaim"),
		Entry("VirtualMachineGroup",
			func() []byte { return mb.GetVirtualMachineGroupYaml(mb.VirtualMachineGroupYaml{Name: "g"}) },
			"vmoperator.vmware.com/v1alpha5", "VirtualMachineGroup"),
		Entry("VirtualMachineGroup v1alpha5 with boot order",
			func() []byte {
				return mb.GetVirtualMachineGroupWithBootOrderYaml(mb.VirtualMachineGroupYaml{Name: "g"})
			},
			"vmoperator.vmware.com/v1alpha5", "VirtualMachineGroup"),
		Entry("VirtualMachineGroup v1alpha6 with boot order",
			func() []byte {
				return mb.GetVirtualMachineGroupWithBootOrderYamlV1Alpha6(mb.VirtualMachineGroupYaml{Name: "g"})
			},
			"vmoperator.vmware.com/v1alpha6", "VirtualMachineGroup"),
		Entry("VirtualMachineGroupPublishRequest",
			func() []byte {
				return mb.GetVirtualMachineGroupPublishRequestYaml(mb.VirtualMachineGroupPublishRequestYaml{Name: "p"})
			},
			"vmoperator.vmware.com/v1alpha5", "VirtualMachineGroupPublishRequest"),
		Entry("VirtualMachinePublishRequest",
			func() []byte {
				return mb.GetVirtualMachinePublishRequestYaml(mb.VirtualMachinePublishRequestYaml{Name: "p"})
			},
			"vmoperator.vmware.com/v1alpha2", "VirtualMachinePublishRequest"),
		Entry("VirtualMachineSnapshot",
			func() []byte { return mb.GetVirtualMachineSnapshotYaml(mb.VirtualMachineSnapshotYaml{Name: "s"}) },
			"vmoperator.vmware.com/v1alpha5", "VirtualMachineSnapshot"),
		Entry("WebConsoleRequest",
			func() []byte {
				return mb.GetV1A1WebConsoleRequestYaml(mb.VirtualMachineWebConsoleRequestYaml{Name: "w"})
			},
			"vmoperator.vmware.com/v1alpha1", "WebConsoleRequest"),
		Entry("VirtualMachineWebConsoleRequest",
			func() []byte {
				return mb.GetVirtualMachineWebConsoleRequestYaml(mb.VirtualMachineWebConsoleRequestYaml{Name: "w"})
			},
			"vmoperator.vmware.com/v1alpha2", "VirtualMachineWebConsoleRequest"),
		Entry("VirtualMachineClass",
			func() []byte { return mb.GetVirtualMachineClassYaml("my-ns", "c") },
			"vmoperator.vmware.com/v1alpha2", "VirtualMachineClass"),
	)

	DescribeTable("render the VirtualMachine followed by its PersistentVolumeClaims",
		func(manifest func(mb.VirtualMachineYaml) []byte, apiVersion string) {
			p := minimalVM()
			p.PVCs = []mb.PVC{
				{VolumeName: "vol-1", ClaimName: "claim-1", Namespace: "my-ns", RequestSize: "1Gi"},
				{VolumeName: "vol-2", ClaimName: "claim-2", Namespace: "my-ns", RequestSize: "2Gi"},
			}

			docs := parseDocs(manifest(p))
			Expect(docs).To(HaveLen(3))
			Expect(docs[0].GetAPIVersion()).To(Equal(apiVersion))
			Expect(docs[0].GetKind()).To(Equal("VirtualMachine"))
			for i, name := range []string{"claim-1", "claim-2"} {
				Expect(docs[i+1].GetKind()).To(Equal("PersistentVolumeClaim"))
				Expect(docs[i+1].GetName()).To(Equal(name))
			}
		},
		Entry("v1alpha5", mb.GetVirtualMachineYamlA5, "vmoperator.vmware.com/v1alpha5"),
		Entry("v1alpha6", mb.GetVirtualMachineYamlA6, "vmoperator.vmware.com/v1alpha6"),
	)

	It("renders VirtualMachineGroupPublishRequest virtualMachines as a list", func() {
		docs := parseDocs(mb.GetVirtualMachineGroupPublishRequestYaml(mb.VirtualMachineGroupPublishRequestYaml{
			Name:            "p",
			VirtualMachines: []string{"vm-1", "vm-2"},
		}))
		Expect(docs).To(HaveLen(1))
		vms, _, err := unstructured.NestedStringSlice(docs[0].Object, "spec", "virtualMachines")
		Expect(err).ToNot(HaveOccurred())
		Expect(vms).To(Equal([]string{"vm-1", "vm-2"}))
	})

	It("renders VirtualMachineClass without a spec", func() {
		docs := parseDocs(mb.GetVirtualMachineClassYaml("my-ns", "c"))
		Expect(docs).To(HaveLen(1))
		Expect(docs[0].Object).ToNot(HaveKey("spec"))
	})

	// VirtualMachineYaml's Affinity, Hardware, and Policies are v1alpha6
	// types that VirtualMachineA5 copies into their v1alpha5 equivalents.
	// Filling every field with a non-nil value catches a field that exists
	// in both versions but was not copied.
	It("VirtualMachineA5 copies every v1alpha6 Affinity, Hardware, and Policies field", func() {
		filler := randfill.New().NilChance(0).NumElements(1, 3)

		for range 50 {
			p := minimalVM()
			p.Affinity = &vmopv1.AffinitySpec{}
			p.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
			filler.Fill(p.Affinity)
			filler.Fill(p.Hardware)
			filler.Fill(&p.Policies)

			vm, err := mb.VirtualMachineA5(p)
			Expect(err).ToNot(HaveOccurred())

			Expect(toJSON(vm.Spec.Affinity)).To(MatchJSON(toJSON(p.Affinity)))
			Expect(toJSON(vm.Spec.Hardware)).To(MatchJSON(toJSON(p.Hardware)))
			Expect(toJSON(vm.Spec.Policies)).To(MatchJSON(toJSON(p.Policies)))
		}
	})

	It("returns an error for an invalid inline sysprep", func() {
		p := minimalVM()
		p.Bootstrap.Sysprep = &mb.Sysprep{Sysprep: new("notAField: true")}
		_, err := mb.VirtualMachineA2(p)
		Expect(err).To(MatchError(ContainSubstring("notAField")))
	})
})
