// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/randfill"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	mb "github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
)

var _ = Describe("VirtualMachineA5", func() {

	// VirtualMachineYaml's Affinity, Hardware, and Policies are v1alpha6
	// types that VirtualMachineA5 copies into their v1alpha5 equivalents.
	// Filling every field with a non-nil value catches a field that exists
	// in both versions but was not copied.
	It("copies every v1alpha6 Affinity, Hardware, and Policies field", func() {
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
})

func toJSON(v any) string {
	GinkgoHelper()
	b, err := json.Marshal(v)
	Expect(err).ToNot(HaveOccurred())
	return string(b)
}
