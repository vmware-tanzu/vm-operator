// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// TRANSITIONAL: this file compares the typed builders against the legacy
// text/template fixtures they replace. It is deleted along with the
// fixtures.

package manifestbuilders_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"text/template"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	mb "github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
)

const fixturesDir = "../fixtures/yaml/vmoperator"

func renderTemplate(file string, data any) []byte {
	GinkgoHelper()
	in, err := os.ReadFile(filepath.Join(fixturesDir, file))
	Expect(err).ToNot(HaveOccurred())
	tmpl := template.Must(template.New(file).Parse(string(in)))
	var out bytes.Buffer
	Expect(tmpl.Execute(&out, data)).To(Succeed())
	return out.Bytes()
}

// parseDocs splits a multi-document manifest into maps with null values
// removed, so that "key: null" and an absent key compare equal.
func parseDocs(manifest []byte) []map[string]any {
	GinkgoHelper()
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(manifest)))
	var docs []map[string]any
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		Expect(err).ToNot(HaveOccurred())
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		var m map[string]any
		Expect(yaml.Unmarshal(doc, &m)).To(Succeed(), string(doc))
		if m == nil {
			continue
		}
		docs = append(docs, dropNulls(m).(map[string]any))
	}
	return docs
}

// normalize re-marshals docs so that key order and formatting do not
// register as differences.
func normalize(docs []map[string]any) []string {
	GinkgoHelper()
	out := make([]string, len(docs))
	for i := range docs {
		b, err := yaml.Marshal(docs[i])
		Expect(err).ToNot(HaveOccurred())
		out[i] = string(b)
	}
	return out
}

func dropNulls(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if vv == nil {
				delete(t, k)
				continue
			}
			t[k] = dropNulls(vv)
		}
	case []any:
		for i := range t {
			t[i] = dropNulls(t[i])
		}
	}
	return v
}

type equivCase struct {
	template string
	data     any
	typed    func() []byte

	// allow, when set, edits the template's documents to account for a
	// known, accepted difference from the typed builder.
	allow func(docs []map[string]any)
}

func expectEquivalent(c equivCase) {
	GinkgoHelper()
	want := parseDocs(renderTemplate(c.template, c.data))
	if c.allow != nil {
		c.allow(want)
	}
	Expect(normalize(parseDocs(c.typed()))).To(Equal(normalize(want)))
}

func spec(doc map[string]any) map[string]any {
	return doc["spec"].(map[string]any)
}

var (
	labels      = map[string]string{"app": "foo", "tier": "backend"}
	annotations = map[string]string{"example.com/note": "hello world"}

	inlineSysprep = `
        guiUnattended:
          autoLogon: true
          autoLogonCount: 1
          password:
            name: my-secret
            key: vmsvc-pwd
          timeZone: 004
        identification:
          joinWorkgroup: vmware
        guiRunOnce:
          commands:
          - "dir C:"
          - "echo Hello"
          - 'C:\sysprep\guestcustutil.exe restoreMountedDevices'
          - 'C:\sysprep\guestcustutil.exe flagComplete'
          - 'C:\sysprep\guestcustutil.exe deleteContainingFolder'
        userData:
          fullName: "First User"
          orgName: "Broadcom"`

	inlineCloudConfig = `
        defaultUserEnabled: true
        ssh_pwauth: true
        users:
        - name: vmware
          lock_passwd: false
          passwd:
            name: my-secret
            key: vmsvc-pwd
        runcmd:
        - [ "ls", "-a", "-l", "/" ]
        - - echo
          - "hello, world."
        write_files:
        - path: /etc/my-plaintext
          permissions: '0644'
          owner: root:root
          content:
            name: my-secret
            key: hello`
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

func a5PVCs() []mb.PVC {
	return []mb.PVC{
		{
			VolumeName:       "vol-1",
			ClaimName:        "claim-1",
			StorageClassName: "wcpglobal-storage-profile",
			RequestSize:      "1Gi",
			Namespace:        "my-ns",
		},
		{
			VolumeName:          "vol-2",
			ClaimName:           "claim-2",
			StorageClassName:    "wcpglobal-storage-profile",
			RequestSize:         "2Gi",
			Namespace:           "my-ns",
			ControllerBusNumber: ptr.To[int32](0),
			UnitNumber:          ptr.To[int32](3),
			SharingMode:         ptr.To("MultiWriter"),
			DiskMode:            ptr.To("IndependentPersistent"),
			VolumeMode:          ptr.To(corev1.PersistentVolumeBlock),
			AccessModes:         []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			ControllerType:      ptr.To(vmopv1.VirtualControllerTypeSCSI),
			ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
		},
	}
}

func affinityTerms(preferred bool) *vmopv1.AffinitySpec {
	term := vmopv1.VMAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "db"},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"a", "b"}},
				{Key: "zone", Operator: metav1.LabelSelectorOpExists},
			},
		},
		TopologyKey: "topology.kubernetes.io/zone",
	}
	a := &vmopv1.AffinitySpec{
		VMAffinity: &vmopv1.VMAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term},
		},
		VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term},
		},
	}
	if preferred {
		a.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{term}
		a.VMAntiAffinity.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{term}
	}
	return a
}

func hardware() *vmopv1.VirtualMachineHardwareSpec {
	return &vmopv1.VirtualMachineHardwareSpec{
		SCSIControllers: []vmopv1.SCSIControllerSpec{
			{BusNumber: 1, Type: vmopv1.SCSIControllerTypeParaVirtualSCSI, SharingMode: vmopv1.VirtualControllerSharingModeNone},
		},
		NVMEControllers: []vmopv1.NVMEControllerSpec{
			{BusNumber: 0, SharingMode: vmopv1.VirtualControllerSharingModeNone},
		},
		SATAControllers: []vmopv1.SATAControllerSpec{
			{BusNumber: 2},
		},
		Cdrom: []vmopv1.VirtualMachineCdromSpec{
			{
				Name:                "cdrom1",
				Image:               vmopv1.VirtualMachineImageRef{Name: "vmi-iso", Kind: "VirtualMachineImage"},
				ControllerBusNumber: ptr.To[int32](0),
				ControllerType:      vmopv1.VirtualControllerTypeIDE,
				UnitNumber:          ptr.To[int32](1),
				Connected:           ptr.To(true),
				AllowGuestControl:   ptr.To(true),
			},
		},
	}
}

func fullA5(preferredAffinity bool) mb.VirtualMachineYaml {
	p := minimalVM()
	p.Labels = labels
	p.Annotations = annotations
	p.GroupName = "my-group"
	p.GuestID = "vmwarePhoton64Guest"
	p.Bootstrap = mb.Bootstrap{
		CloudInit: &mb.CloudInit{
			RawCloudConfig: &mb.KeySelector{Name: "my-secret", Key: "user-data"},
		},
		LinuxPrep: &mb.LinuxPrep{
			HardwareClockIsUTC:     true,
			TimeZone:               "US/Pacific",
			CustomizeAtNextPowerOn: ptr.To(true),
		},
		VAppConfig: &mb.VAppConfig{
			Properties: &[]mb.KeyValueOrSecretKeySelectorPair{
				{Key: "prop-1", Value: mb.ValueOrSecretKeySelector{Value: "my-val-1"}},
			},
		},
	}
	p.PVCs = a5PVCs()
	p.Affinity = affinityTerms(preferredAffinity)
	p.Hardware = hardware()
	p.Policies = []vmopv1.PolicySpec{
		{APIVersion: "vsphere.policy.vmware.com/v1alpha1", Kind: "ComputePolicy", Name: "my-policy"},
	}
	return p
}

var _ = Describe("Typed builders match the legacy templates", func() {

	DescribeTable("VirtualMachine v1alpha1",
		func(p mb.VirtualMachineYaml) {
			expectEquivalent(equivCase{
				template: "virtualmachines/singlevm.yaml.in",
				data:     p,
				typed:    func() []byte { return mb.GetVirtualMachineYaml(p) },
				// ACCEPTED: spec.resourcePolicyName is not omitempty, so an
				// unset value is rendered as "". The server decodes "" and
				// an absent value to the same Go value.
				allow: func(docs []map[string]any) {
					if _, ok := spec(docs[0])["resourcePolicyName"]; !ok {
						spec(docs[0])["resourcePolicyName"] = ""
					}
				},
			})
		},
		Entry("minimal", minimalVM()),
		Entry("full", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Labels = labels
			p.Annotations = annotations
			p.Network = mb.Network{Name: "primary", Type: "vsphere-distributed"}
			p.ResourcePolicy = "my-rp"
			p.PowerOffMode = "hard"
			p.ConfigMapName = "my-cm"
			p.SecretName = "my-secret"
			p.Transport = "CloudInit"
			p.PVCNames = []string{"pvc-a", "pvc-b"}
			return p
		}()),
	)

	DescribeTable("VirtualMachine v1alpha2",
		func(p mb.VirtualMachineYaml) {
			expectEquivalent(equivCase{"virtualmachines/v1a2singlevm.yaml.in", p, func() []byte { return mb.GetVirtualMachineYamlA2(p) }, nil})
		},
		Entry("minimal", minimalVM()),
		Entry("network, reserved, power off mode, volumes", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Labels = labels
			p.Annotations = annotations
			p.NetworkA2 = mb.NetworkA2{Interfaces: []mb.InterfaceSpec{
				{Name: "subnet-1", APIVersion: "crd.nsx.vmware.com/v1alpha1", Kind: "Subnet"},
			}}
			p.ResourcePolicy = "my-rp"
			p.PowerOffMode = "Hard"
			p.PVCNames = []string{"pvc-a"}
			return p
		}()),
		Entry("raw cloud config", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Bootstrap.CloudInit = &mb.CloudInit{RawCloudConfig: &mb.KeySelector{Name: "my-secret", Key: "user-data"}}
			return p
		}()),
		Entry("inline cloud config", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Bootstrap.CloudInit = &mb.CloudInit{CloudConfig: ptr.To(inlineCloudConfig)}
			return p
		}()),
		Entry("raw sysprep", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Bootstrap.Sysprep = &mb.Sysprep{RawSysprep: &mb.KeySelector{Name: "my-secret", Key: "unattend"}}
			return p
		}()),
		Entry("inline sysprep", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.PowerOffMode = "Hard"
			p.Bootstrap.Sysprep = &mb.Sysprep{Sysprep: ptr.To(inlineSysprep)}
			return p
		}()),
		Entry("vApp properties with empty linuxPrep", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Bootstrap.LinuxPrep = &mb.LinuxPrep{}
			p.Bootstrap.VAppConfig = &mb.VAppConfig{Properties: &[]mb.KeyValueOrSecretKeySelectorPair{
				{Key: "prop-1", Value: mb.ValueOrSecretKeySelector{Value: "my-val-1"}},
				{Key: "bool-false", Value: mb.ValueOrSecretKeySelector{Value: "false"}},
			}}
			return p
		}()),
		Entry("raw vApp properties", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Bootstrap.VAppConfig = &mb.VAppConfig{RawProperties: ptr.To("my-secret")}
			return p
		}()),
		Entry("linuxPrep", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Bootstrap.LinuxPrep = &mb.LinuxPrep{HardwareClockIsUTC: true, TimeZone: "US/Pacific"}
			return p
		}()),
	)

	// Mirrors the parameters used by vm_vpcnetworking.go, the only caller.
	// The multi-network template ignored PowerOffMode, PVCNames, and all but
	// the first vApp property; that caller sets none of them.
	It("VirtualMachine v1alpha2 with multiple networks", func() {
		p := minimalVM()
		p.ResourcePolicy = "my-rp"
		p.NetworkA2 = mb.NetworkA2{Interfaces: []mb.InterfaceSpec{
			{Name: "subnet-1", APIVersion: "crd.nsx.vmware.com/v1alpha1", Kind: "Subnet"},
			{Name: "subnet-2", APIVersion: "crd.nsx.vmware.com/v1alpha1", Kind: "Subnet"},
		}}
		p.Bootstrap.CloudInit = &mb.CloudInit{RawCloudConfig: &mb.KeySelector{Name: "my-secret", Key: "user-data"}}
		expectEquivalent(equivCase{"virtualmachines/v1a2vm-multi-network.yaml.in", p, func() []byte { return mb.GetVirtualMachineWithMultiNetworkYamlA2(p) }, nil})
	})

	DescribeTable("VirtualMachine v1alpha3",
		func(p mb.VirtualMachineYaml) {
			expectEquivalent(equivCase{
				template: "virtualmachines/v1a3singlevm.yaml.in",
				data:     p,
				typed:    func() []byte { return mb.GetVirtualMachineYamlA3(p) },
				// ACCEPTED: spec.cdrom[].image.kind is not omitempty, so an
				// unset ImageKind is rendered as "". The field is required
				// by the CRD, so neither form is valid, and every caller
				// sets it.
				allow: func(docs []map[string]any) {
					cdroms, _ := spec(docs[0])["cdrom"].([]any)
					for _, c := range cdroms {
						image := c.(map[string]any)["image"].(map[string]any)
						if _, ok := image["kind"]; !ok {
							image["kind"] = ""
						}
					}
				},
			})
		},
		Entry("minimal", minimalVM()),
		Entry("full", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.Labels = labels
			p.Annotations = annotations
			p.GuestID = "vmwarePhoton64Guest"
			p.Crypto = &mb.Crypto{EncryptionClassName: "my-ec"}
			p.PVCNames = []string{"pvc-a"}
			p.Cdrom = []mb.Cdrom{
				{Name: "cdrom1", ImageName: "vmi-iso", ImageKind: "VirtualMachineImage", Connected: true, AllowGuestControl: true},
				{Name: "cdrom2", ImageName: "vmi-iso2"},
			}
			return p
		}()),
		Entry("default key provider", func() mb.VirtualMachineYaml {
			p := minimalVM()
			p.ImageName = ""
			p.PowerState = ""
			p.Crypto = &mb.Crypto{UseDefaultKeyProvider: true}
			return p
		}()),
	)

	DescribeTable("VirtualMachine v1alpha5",
		func(p mb.VirtualMachineYaml) {
			expectEquivalent(equivCase{"virtualmachines/v1a5singlevm.yaml.in", p, func() []byte { return mb.GetVirtualMachineYamlA5(p) }, nil})
		},
		Entry("minimal", minimalVM()),
		Entry("full", fullA5(true)),
	)

	DescribeTable("VirtualMachine v1alpha6",
		func(p mb.VirtualMachineYaml) {
			expectEquivalent(equivCase{"virtualmachines/v1a6singlevm.yaml.in", p, func() []byte { return mb.GetVirtualMachineYamlA6(p) }, nil})
		},
		Entry("minimal", minimalVM()),
		Entry("full", fullA5(false)),
	)

	DescribeTable("PersistentVolumeClaim",
		func(p mb.PVC) {
			expectEquivalent(equivCase{"virtualmachines/pvc.yaml.in", p, func() []byte { return mb.GetPersistentVolumeClaimYaml(p) }, nil})
		},
		Entry("defaults", a5PVCs()[0]),
		Entry("full", a5PVCs()[1]),
	)

	It("VirtualMachineGroup", func() {
		p := mb.VirtualMachineGroupYaml{
			Namespace: "my-ns",
			Name:      "my-group",
			GroupName: "parent-group",
			Members: []vmopv1.GroupMember{
				{Name: "vm-1", Kind: "VirtualMachine"},
				{Name: "vm-2", Kind: "VirtualMachine"},
			},
		}
		expectEquivalent(equivCase{"virtualmachinegroups/vm-group.yaml.in", p, func() []byte { return mb.GetVirtualMachineGroupYaml(p) }, nil})
	})

	bootOrderGroup := mb.VirtualMachineGroupYaml{
		Namespace:                   "my-ns",
		Name:                        "my-group",
		GroupName:                   "parent-group",
		PowerState:                  "PoweredOn",
		PowerOffMode:                "Hard",
		NextForcePowerStateSyncTime: "now",
		BootOrder: []mb.BootOrder{
			{Members: []vmopv1.GroupMember{{Name: "vm-1", Kind: "VirtualMachine"}}, PowerOnDelay: "30s", PowerOffDelay: "1m0s"},
			{Members: []vmopv1.GroupMember{{Name: "vm-2", Kind: "VirtualMachine"}, {Name: "vm-3", Kind: "VirtualMachine"}}},
		},
	}

	It("VirtualMachineGroup v1alpha5 with boot order", func() {
		expectEquivalent(equivCase{"virtualmachinegroups/vm-group-with-boot-order.yaml.in", bootOrderGroup, func() []byte { return mb.GetVirtualMachineGroupWithBootOrderYaml(bootOrderGroup) }, nil})
	})

	It("VirtualMachineGroup v1alpha6 with boot order", func() {
		expectEquivalent(equivCase{"virtualmachinegroups/vm-group-with-boot-order-v1alpha6.yaml.in", bootOrderGroup, func() []byte { return mb.GetVirtualMachineGroupWithBootOrderYamlV1Alpha6(bootOrderGroup) }, nil})
	})

	DescribeTable("VirtualMachineGroupPublishRequest",
		func(p mb.VirtualMachineGroupPublishRequestYaml) {
			expectEquivalent(equivCase{
				template: "virtualmachinegrouppublishrequests/vm-group-publish.yaml.in",
				data:     p,
				typed:    func() []byte { return mb.GetVirtualMachineGroupPublishRequestYaml(p) },
				// FIXED: the template rendered the list as "[vm-1 vm-2]",
				// which YAML parses as the single element "vm-1 vm-2".
				allow: func(docs []map[string]any) {
					if len(p.VirtualMachines) > 0 {
						vms := make([]any, len(p.VirtualMachines))
						for i := range p.VirtualMachines {
							vms[i] = p.VirtualMachines[i]
						}
						spec(docs[0])["virtualMachines"] = vms
					}
				},
			})
		},
		Entry("minimal", mb.VirtualMachineGroupPublishRequestYaml{Namespace: "my-ns", Name: "my-pub", Source: "my-group", Target: "my-cl"}),
		Entry("single VM and TTL", mb.VirtualMachineGroupPublishRequestYaml{Namespace: "my-ns", Name: "my-pub", Source: "my-group", Target: "my-cl", VirtualMachines: []string{"vm-1"}, TTLSecondsAfterFinished: 60}),
		Entry("multiple VMs", mb.VirtualMachineGroupPublishRequestYaml{Namespace: "my-ns", Name: "my-pub", Source: "my-group", Target: "my-cl", VirtualMachines: []string{"vm-1", "vm-2"}}),
	)

	It("VirtualMachinePublishRequest", func() {
		p := mb.VirtualMachinePublishRequestYaml{
			Namespace:   "my-ns",
			Name:        "my-pub",
			Labels:      labels,
			Annotations: annotations,
			Source:      mb.VirtualMachinePublishRequestSource{Name: "my-vm"},
			Target: mb.VirtualMachinePublishRequestTarget{
				Item:     mb.VirtualMachinePublishRequestTargetItem{Name: "my-item", Description: "my description"},
				Location: mb.VirtualMachinePublishRequestTargetLocation{Name: "my-cl"},
			},
		}
		expectEquivalent(equivCase{"virtualmachinepublishrequests/singlevirtualmachinepublishrequest.yaml.in", p, func() []byte { return mb.GetVirtualMachinePublishRequestYaml(p) }, nil})
	})

	DescribeTable("VirtualMachineSnapshot",
		func(p mb.VirtualMachineSnapshotYaml) {
			expectEquivalent(equivCase{"virtualmachinesnapshot/v1alpha5-vmsnapshot.yaml.in", p, func() []byte { return mb.GetVirtualMachineSnapshotYaml(p) }, nil})
		},
		Entry("minimal", mb.VirtualMachineSnapshotYaml{Namespace: "my-ns", Name: "my-snap", VMName: "my-vm"}),
		Entry("full", mb.VirtualMachineSnapshotYaml{Namespace: "my-ns", Name: "my-snap", VMName: "my-vm", Memory: true, Quiesce: "10m0s", Description: "my description", ImportedSnapshot: true}),
	)

	It("WebConsoleRequest v1alpha1", func() {
		p := mb.VirtualMachineWebConsoleRequestYaml{Namespace: "my-ns", Name: "my-wcr", VMName: "my-vm"}
		expectEquivalent(equivCase{"virtualmachinewebconsolerequests/webconsolerequests.yaml.in", p, func() []byte { return mb.GetV1A1WebConsoleRequestYaml(p) }, nil})
	})

	It("VirtualMachineWebConsoleRequest v1alpha2", func() {
		p := mb.VirtualMachineWebConsoleRequestYaml{Namespace: "my-ns", Name: "my-wcr", VMName: "my-vm"}
		expectEquivalent(equivCase{"virtualmachinewebconsolerequests/virtualmachinewebconsolerequests.yaml.in", p, func() []byte { return mb.GetVirtualMachineWebConsoleRequestYaml(p) }, nil})
	})

	nameNS := struct{ Namespace, Name string }{"my-ns", "my-name"}

	It("VirtualMachineClass", func() {
		expectEquivalent(equivCase{"virtualmachineclasses/vmclass.yaml.in", nameNS, func() []byte { return mb.GetVirtualMachineClassYaml("my-ns", "my-name") }, nil})
	})

	It("VirtualMachineClassBinding", func() {
		expectEquivalent(equivCase{"virtualmachineclasses/vmclassbindings.yaml.in", nameNS, func() []byte { return mb.GetVirtualMachineClassBindingYaml("my-ns", "my-name") }, nil})
	})

	It("ContentSourceBinding", func() {
		expectEquivalent(equivCase{"contentsources/contentsourcebindings.yaml.in", nameNS, func() []byte { return mb.GetContentSourceBindingYaml("my-ns", "my-name") }, nil})
	})
})
