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
		Entry("VirtualMachine v1alpha5",
			func() []byte { return mb.GetVirtualMachineYamlA5(minimalVM()) },
			"vmoperator.vmware.com/v1alpha5", "VirtualMachine"),
		Entry("PersistentVolumeClaim",
			func() []byte {
				return mb.GetPersistentVolumeClaimYaml(mb.PVC{ClaimName: "claim", Namespace: "my-ns", RequestSize: "1Gi"})
			},
			"v1", "PersistentVolumeClaim"),
		Entry("VirtualMachineGroupPublishRequest",
			func() []byte {
				return mb.GetVirtualMachineGroupPublishRequestYaml(mb.VirtualMachineGroupPublishRequestYaml{Name: "p"})
			},
			"vmoperator.vmware.com/v1alpha5", "VirtualMachineGroupPublishRequest"),
		Entry("VirtualMachineSnapshot",
			func() []byte { return mb.GetVirtualMachineSnapshotYaml(mb.VirtualMachineSnapshotYaml{Name: "s"}) },
			"vmoperator.vmware.com/v1alpha5", "VirtualMachineSnapshot"),
		Entry("Secret",
			func() []byte { return mb.ToYAML(mb.CloudConfigSecret(mb.Secret{Name: "s", Namespace: "my-ns"})) },
			"v1", "Secret"),
		Entry("SecurityPolicy",
			func() []byte { return mb.GetSecurityPolicyYaml(mb.SecurityPolicy{Name: "sp", Namespace: "my-ns"}) },
			"crd.nsx.vmware.com/v1alpha1", "SecurityPolicy"),
	)

	It("renders SecurityPolicy spec fields", func() {
		docs := parseDocs(mb.GetSecurityPolicyYaml(mb.SecurityPolicy{Name: "sp", Namespace: "my-ns"}))
		Expect(docs).To(HaveLen(1))
		Expect(toJSON(docs[0].Object["spec"])).To(MatchJSON(`{
			"priority": 10,
			"appliedTo": [{"vmSelector": {"matchLabels": {"role": "allow-ingress"}}}],
			"rules": [{"direction": "in", "action": "allow"}]
		}`))
	})

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

	DescribeTable("render Secret data",
		func(manifest func() []byte, key, value string) {
			docs := parseDocs(manifest())
			Expect(docs).To(HaveLen(1))
			for _, field := range []string{"data", "stringData"} {
				if v, found, _ := unstructured.NestedString(docs[0].Object, field, key); found {
					Expect(v).To(Equal(value))
					return
				}
			}
			Fail("key " + key + " not found in data or stringData")
		},
		Entry("Secret CloudConfig user-data", func() []byte {
			return mb.ToYAML(mb.CloudConfigSecret(mb.Secret{Name: "s", Namespace: "my-ns"}))
		}, "user-data", "#cloud-config\nssh_pwauth: true\nusers:\n  - name: vmware\n"+
			"    sudo: ALL=(ALL) NOPASSWD:ALL\n    lock_passwd: false\n"+
			"    # Password set to Admin!23\n    passwd: '$1$salt$SOC33fVbA/ZxeIwD5yw1u1'\n"+
			"    shell: /bin/bash\nwrite_files:\n  - content: |\n      VMSVC Says Hello World\n"+
			"    path: /helloworld\n"),
		Entry("Secret InlineSysprepData vmsvc-pwd", func() []byte {
			return mb.ToYAML(mb.InlineSysprepSecret(mb.Secret{Name: "s", Namespace: "my-ns"}))
		}, "vmsvc-pwd", "vmware"),
	)

	DescribeTable("render a Subnet or SubnetSet with only the fields that are set",
		func(manifest func(mb.SubnetOrSubnetSet, bool) []byte, kind string, private bool, expectedSpec string) {
			docs := parseDocs(manifest(mb.SubnetOrSubnetSet{Kind: kind, Namespace: "my-ns", Name: "s"}, private))
			Expect(docs).To(HaveLen(1))
			Expect(docs[0].GetAPIVersion()).To(Equal("crd.nsx.vmware.com/v1alpha1"))
			Expect(docs[0].GetKind()).To(Equal(kind))
			Expect(docs[0].GetNamespace()).To(Equal("my-ns"))
			Expect(docs[0].GetName()).To(Equal("s"))
			Expect(docs[0].Object).ToNot(HaveKey("status"))
			Expect(toJSON(docs[0].Object["spec"])).To(MatchJSON(expectedSpec))
		},
		Entry("DHCP private Subnet", mb.GetDHCPSubnetOrSubnetSetYaml, "Subnet", true,
			`{"accessMode": "Private", "subnetDHCPConfig": {"mode": "DHCPServer"}}`),
		Entry("DHCP public SubnetSet", mb.GetDHCPSubnetOrSubnetSetYaml, "SubnetSet", false,
			`{"accessMode": "Public", "subnetDHCPConfig": {"mode": "DHCPServer"}}`),
		Entry("CIDR private Subnet", mb.GetCIDRSubnetOrSubnetSetYaml, "Subnet", true,
			`{"accessMode": "Private", "ipv4SubnetSize": 16}`),
		Entry("CIDR public SubnetSet", mb.GetCIDRSubnetOrSubnetSetYaml, "SubnetSet", false,
			`{"accessMode": "Public", "ipv4SubnetSize": 16}`),
	)

	It("returns an error for a Subnet kind that is neither Subnet nor SubnetSet", func() {
		_, err := mb.DHCPSubnetOrSubnetSet(mb.SubnetOrSubnetSet{Kind: "Network", Name: "s"}, true)
		Expect(err).To(MatchError(ContainSubstring(`invalid kind "Network"`)))
	})

	// VirtualMachineYaml's Hardware is a v1alpha6 type that VirtualMachineA5
	// copies into its v1alpha5 equivalent. Filling every field with a non-nil
	// value catches a field that exists in both versions but was not copied.
	It("VirtualMachineA5 copies every v1alpha6 Hardware field", func() {
		filler := randfill.New().NilChance(0).NumElements(1, 3)

		for range 50 {
			p := minimalVM()
			p.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
			filler.Fill(p.Hardware)

			vm := mb.VirtualMachineA5(p)

			Expect(toJSON(vm.Spec.Hardware)).To(MatchJSON(toJSON(p.Hardware)))
		}
	})
})
