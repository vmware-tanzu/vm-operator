// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = DescribeTable("IsVirtualMachineSchemaUpgraded",
	func(
		buildVersion string,
		buildAnnotation,
		schemaAnnotation,
		featureAnnotation *string,
		annotationsNil,
		vmSharedDisks,
		allDisksArePVCs,
		expected bool,
	) {
		ctx := pkgcfg.WithConfig(pkgcfg.Config{
			BuildVersion: buildVersion,
			Features: pkgcfg.FeatureStates{
				VMSharedDisks:   vmSharedDisks,
				AllDisksArePVCs: allDisksArePVCs,
			},
		})

		vm := vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "test-namespace",
			},
		}

		if annotationsNil {
			vm.Annotations = nil
		} else {
			vm.Annotations = map[string]string{}
			if buildAnnotation != nil {
				vm.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey] = *buildAnnotation
			}
			if schemaAnnotation != nil {
				vm.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey] = *schemaAnnotation
			}
			if featureAnnotation != nil {
				vm.Annotations[pkgconst.UpgradedToFeatureVersionAnnotationKey] = *featureAnnotation
			}
		}

		Ω(vmopv1util.IsVirtualMachineSchemaUpgraded(ctx, vm)).Should(Equal(expected))
	},
	Entry(
		"all annotations are set correctly with no features",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("1"),
		false,
		false,
		false,
		true,
	),
	Entry(
		"all annotations are set correctly with VMSharedDisks",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("3"),
		false,
		true,
		false,
		true,
	),
	Entry(
		"all annotations are set correctly with AllDisksArePVCs",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("5"),
		false,
		false,
		true,
		true,
	),
	Entry(
		"all annotations are set correctly with all features",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("7"),
		false,
		true,
		true,
		true,
	),
	Entry(
		"build version annotation is missing",
		"1.2.3-test",
		nil,
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("1"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"schema version annotation is missing",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		nil,
		ptr.To("1"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"feature version annotation is missing",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		nil,
		false,
		false,
		false,
		false,
	),
	Entry(
		"all annotations are missing",
		"1.2.3-test",
		nil,
		nil,
		nil,
		false,
		false,
		false,
		false,
	),
	Entry(
		"build version annotation is empty",
		"1.2.3-test",
		ptr.To(""),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("1"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"schema version annotation is empty",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(""),
		ptr.To("1"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"feature version annotation is empty",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To(""),
		false,
		false,
		false,
		false,
	),
	Entry(
		"build version annotation does not match context build version",
		"1.2.3-test",
		ptr.To("0.0.0-wrong"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("1"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"schema version annotation does not match current schema version",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To("v1alpha4"),
		ptr.To("1"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"feature version annotation does not match activated features",
		"1.2.3-test",
		ptr.To("1.2.3-test"),
		ptr.To(vmopv1.GroupVersion.Version),
		ptr.To("1"),
		false,
		true,
		false,
		false,
	),
	Entry(
		"all annotations do not match",
		"1.2.3-test",
		ptr.To("0.0.0-wrong"),
		ptr.To("v1alpha4"),
		ptr.To("0"),
		false,
		false,
		false,
		false,
	),
	Entry(
		"annotations map is nil",
		"1.2.3-test",
		nil,
		nil,
		nil,
		true,
		false,
		false,
		false,
	),
)

var _ = Describe("FeatureVersion", func() {
	Describe("IsValid", func() {
		DescribeTable("validates feature versions",
			func(fv vmopv1util.FeatureVersion, expected bool) {
				Ω(fv.IsValid()).Should(Equal(expected))
			},
			Entry("empty is invalid", vmopv1util.FeatureVersionEmpty, false),
			Entry("base is valid", vmopv1util.FeatureVersionBase, true),
			Entry("VMSharedDisks is valid", vmopv1util.FeatureVersionVMSharedDisks, true),
			Entry("AllDisksArePVCs is valid", vmopv1util.FeatureVersionAllDisksArePVCs, true),
			Entry("Base + VMSharedDisks is valid", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks, true),
			Entry("Base + AllDisksArePVCs is valid", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionAllDisksArePVCs, true),
			Entry("All is valid", vmopv1util.FeatureVersionAll, true),
			Entry("invalid bit 8 is invalid", vmopv1util.FeatureVersion(8), false),
			Entry("invalid bit 16 is invalid", vmopv1util.FeatureVersion(16), false),
			Entry("invalid bit 255 is invalid", vmopv1util.FeatureVersion(255), false),
		)
	})

	Describe("String", func() {
		DescribeTable("converts to string",
			func(fv vmopv1util.FeatureVersion, expected string) {
				Ω(fv.String()).Should(Equal(expected))
			},
			Entry("empty", vmopv1util.FeatureVersionEmpty, "0"),
			Entry("base", vmopv1util.FeatureVersionBase, "1"),
			Entry("VMSharedDisks", vmopv1util.FeatureVersionVMSharedDisks, "2"),
			Entry("AllDisksArePVCs", vmopv1util.FeatureVersionAllDisksArePVCs, "4"),
			Entry("Base + VMSharedDisks", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks, "3"),
			Entry("Base + AllDisksArePVCs", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionAllDisksArePVCs, "5"),
			Entry("VMSharedDisks + AllDisksArePVCs", vmopv1util.FeatureVersionVMSharedDisks|vmopv1util.FeatureVersionAllDisksArePVCs, "6"),
			Entry("All", vmopv1util.FeatureVersionAll, "7"),
		)
	})

	Describe("Has", func() {
		DescribeTable("checks if feature is present",
			func(fv vmopv1util.FeatureVersion, check vmopv1util.FeatureVersion, expected bool) {
				Ω(fv.Has(check)).Should(Equal(expected))
			},
			Entry("empty has base", vmopv1util.FeatureVersionEmpty, vmopv1util.FeatureVersionBase, false),
			Entry("base has base", vmopv1util.FeatureVersionBase, vmopv1util.FeatureVersionBase, true),
			Entry("base has VMSharedDisks", vmopv1util.FeatureVersionBase, vmopv1util.FeatureVersionVMSharedDisks, false),
			Entry("VMSharedDisks has VMSharedDisks", vmopv1util.FeatureVersionVMSharedDisks, vmopv1util.FeatureVersionVMSharedDisks, true),
			Entry("Base + VMSharedDisks has base", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks, vmopv1util.FeatureVersionBase, true),
			Entry("Base + VMSharedDisks has VMSharedDisks", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks, vmopv1util.FeatureVersionVMSharedDisks, true),
			Entry("Base + VMSharedDisks has AllDisksArePVCs", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks, vmopv1util.FeatureVersionAllDisksArePVCs, false),
			Entry("All has base", vmopv1util.FeatureVersionAll, vmopv1util.FeatureVersionBase, true),
			Entry("All has VMSharedDisks", vmopv1util.FeatureVersionAll, vmopv1util.FeatureVersionVMSharedDisks, true),
			Entry("All has AllDisksArePVCs", vmopv1util.FeatureVersionAll, vmopv1util.FeatureVersionAllDisksArePVCs, true),
		)
	})

	Describe("Set", func() {
		It("should set a single feature", func() {
			fv := vmopv1util.FeatureVersionEmpty
			fv.Set(vmopv1util.FeatureVersionBase)
			Ω(fv).Should(Equal(vmopv1util.FeatureVersionBase))
		})

		It("should set multiple features", func() {
			fv := vmopv1util.FeatureVersionEmpty
			fv.Set(vmopv1util.FeatureVersionBase)
			fv.Set(vmopv1util.FeatureVersionVMSharedDisks)
			Ω(fv).Should(Equal(vmopv1util.FeatureVersionBase | vmopv1util.FeatureVersionVMSharedDisks))
			Ω(fv.Has(vmopv1util.FeatureVersionBase)).Should(BeTrue())
			Ω(fv.Has(vmopv1util.FeatureVersionVMSharedDisks)).Should(BeTrue())
		})

		It("should set all features", func() {
			fv := vmopv1util.FeatureVersionEmpty
			fv.Set(vmopv1util.FeatureVersionBase)
			fv.Set(vmopv1util.FeatureVersionVMSharedDisks)
			fv.Set(vmopv1util.FeatureVersionAllDisksArePVCs)
			Ω(fv).Should(Equal(vmopv1util.FeatureVersionAll))
		})

		It("should be idempotent", func() {
			fv := vmopv1util.FeatureVersionEmpty
			fv.Set(vmopv1util.FeatureVersionBase)
			fv.Set(vmopv1util.FeatureVersionBase)
			Ω(fv).Should(Equal(vmopv1util.FeatureVersionBase))
		})
	})
})

var _ = Describe("ParseFeatureVersion", func() {
	DescribeTable("parses feature version strings",
		func(s string, expected vmopv1util.FeatureVersion) {
			Ω(vmopv1util.ParseFeatureVersion(s)).Should(Equal(expected))
		},
		Entry("empty string", "", vmopv1util.FeatureVersionEmpty),
		Entry("zero", "0", vmopv1util.FeatureVersionEmpty),
		Entry("base", "1", vmopv1util.FeatureVersionBase),
		Entry("VMSharedDisks", "2", vmopv1util.FeatureVersionVMSharedDisks),
		Entry("Base + VMSharedDisks", "3", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks),
		Entry("AllDisksArePVCs", "4", vmopv1util.FeatureVersionAllDisksArePVCs),
		Entry("Base + AllDisksArePVCs", "5", vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionAllDisksArePVCs),
		Entry("VMSharedDisks + AllDisksArePVCs", "6", vmopv1util.FeatureVersionVMSharedDisks|vmopv1util.FeatureVersionAllDisksArePVCs),
		Entry("All", "7", vmopv1util.FeatureVersionAll),
		Entry("invalid negative", "-1", vmopv1util.FeatureVersionEmpty),
		Entry("invalid text", "abc", vmopv1util.FeatureVersionEmpty),
		Entry("invalid with invalid bits", "8", vmopv1util.FeatureVersionEmpty),
		Entry("invalid large number", "65536", vmopv1util.FeatureVersionEmpty),
		Entry("invalid with valid and invalid bits", "255", vmopv1util.FeatureVersionEmpty),
	)
})

var _ = Describe("ActivatedFeatureVersion", func() {
	DescribeTable("returns activated feature version",
		func(vmSharedDisks, allDisksArePVCs bool, expected vmopv1util.FeatureVersion) {
			ctx := pkgcfg.WithConfig(pkgcfg.Config{
				Features: pkgcfg.FeatureStates{
					VMSharedDisks:   vmSharedDisks,
					AllDisksArePVCs: allDisksArePVCs,
				},
			})
			Ω(vmopv1util.ActivatedFeatureVersion(ctx)).Should(Equal(expected))
		},
		Entry("no features", false, false, vmopv1util.FeatureVersionBase),
		Entry("VMSharedDisks only", true, false, vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionVMSharedDisks),
		Entry("AllDisksArePVCs only", false, true, vmopv1util.FeatureVersionBase|vmopv1util.FeatureVersionAllDisksArePVCs),
		Entry("both features", true, true, vmopv1util.FeatureVersionAll),
	)
})
