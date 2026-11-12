// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Affinity options", Label(testlabels.Controller), func() {

	const ns = "team-a"

	var (
		ctx context.Context
		vm  *vmopv1.VirtualMachine
		c   ctrlclient.Client
		// declared are the Tags of a VM that declares AAF against app=db.
		declared []vspherepolv1.Tag
	)

	selector := func(value string) []vmopv1.VMAffinityTerm {
		return []vmopv1.VMAffinityTerm{{
			TopologyKey: corev1.LabelHostname,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": value},
			},
		}}
	}
	aaf := func(value string) *vmopv1.AffinitySpec {
		return &vmopv1.AffinitySpec{VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution: selector(value),
		}}
	}
	af := func(value string) *vmopv1.AffinitySpec {
		return &vmopv1.AffinitySpec{VMAffinity: &vmopv1.VMAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution: selector(value),
		}}
	}

	attached := func(spec vimtypes.VirtualMachineConfigSpec) []string {
		var names []string
		for _, ts := range spec.TagSpecs {
			if ts.Operation == vimtypes.ArrayUpdateOperationAdd && ts.Id.NameId != nil {
				names = append(names, ts.Id.NameId.Tag)
			}
		}
		return names
	}
	groupTargets := func(spec vimtypes.VirtualMachineConfigSpec) []string {
		var names []string
		for _, p := range spec.VmPlacementPolicies {
			if g, ok := p.(*vimtypes.VmToVmGroupsAntiAffinity); ok {
				for _, t := range g.AntiAffinedVmGroupTags {
					names = append(names, t.NameId.Tag)
				}
			}
		}
		return names
	}
	receipt := func(spec vimtypes.VirtualMachineConfigSpec) []string {
		v, _ := object.OptionValueList(spec.ExtraConfig).GetString(virtualmachine.ExtraConfigVMTagsKey)
		if v == "" {
			return nil
		}
		return strings.Split(v, ",")
	}
	derivedRe := `^vmoperator:aaf-rds-pde-host-[0-9a-f]{16}-app:db$`

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		pkgcfg.SetContext(ctx, func(cfg *pkgcfg.Config) {
			cfg.Features.TaggingAPI = true
			cfg.Features.VMAffinityDuringExecution = true
			cfg.Features.VMPlacementPolicies = true
			cfg.Features.VMHardAffinityDuringExecution = true
		})
		vm = builder.DummyVirtualMachine()
		vm.Namespace = ns
		vm.Labels = nil

		declarer := builder.DummyVirtualMachine()
		declarer.Namespace = ns
		declarer.Spec.Affinity = aaf("db")
		var err error
		declared, err = virtualmachine.DesiredAffinityTags(ctx, declarer)
		Expect(err).ToNot(HaveOccurred())
		var objs []ctrlclient.Object
		for i := range declared {
			objs = append(objs, declared[i].DeepCopy())
		}
		c = builder.NewFakeClient(objs...)
	})

	Context("without options", func() {
		It("targets original names and attaches only local tags", func() {
			vm.Spec.Affinity = aaf("db")
			vm.Labels = map[string]string{"app": "db"}
			for _, placement := range []bool{false, true} {
				var spec vimtypes.VirtualMachineConfigSpec
				Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(ctx, vm, &spec, placement, nil)).To(Succeed())
				Expect(groupTargets(spec)).To(Equal([]string{"vmoperator:app:db"}))
				Expect(attached(spec)).ToNot(ContainElement(MatchRegexp(derivedRe)))
			}
		})

		It("does not add label-derived tags for a label-only VM", func() {
			vm.Labels = map[string]string{"app": "db"}
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(ctx, vm, &spec, false, nil)).To(Succeed())
			Expect(attached(spec)).To(BeEmpty())
		})
	})

	Context("with label-derived tags", func() {
		It("targets derived names only at create", func() {
			vm.Spec.Affinity = aaf("db")
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &spec, false, &virtualmachine.LabelDerivedTags{Client: c})).To(Succeed())
			Expect(groupTargets(spec)).To(ConsistOf(MatchRegexp(derivedRe)))
		})

		It("targets derived and original names at placement", func() {
			vm.Spec.Affinity = aaf("db")
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &spec, true, &virtualmachine.LabelDerivedTags{Client: c})).To(Succeed())
			Expect(groupTargets(spec)).To(ConsistOf(
				MatchRegexp(derivedRe), "vmoperator:app:db"))
		})

		It("sees a Tag created in this reconcile while the cache lags", func() {
			vm.Labels = map[string]string{"app": "db"}
			lagging := builder.NewFakeClient()
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &spec, false, &virtualmachine.LabelDerivedTags{Client: lagging, OwnedTags: declared})).To(Succeed())
			Expect(attached(spec)).To(HaveLen(2))
		})

		It("records a create receipt equal to the attached tags plus implicit names", func() {
			vm.Spec.Affinity = af("db")
			vm.Labels = map[string]string{"app": "db"}
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &spec, false, &virtualmachine.LabelDerivedTags{Client: c})).To(Succeed())

			// B's own AF hash is both attached and implicit and stays attached.
			Expect(attached(spec)).To(ContainElement(MatchRegexp(`^vmoperator:af-rds-pde-host-[0-9a-f]{16}-app:db$`)))
			Expect(receipt(spec)).To(ConsistOf(
				append([]interface{}{}, toIfaces(attached(spec))...)...))
		})

		It("adds existing AF hashes to a placement VM that carries the label", func() {
			declarer := builder.DummyVirtualMachine()
			declarer.Namespace = ns
			declarer.Spec.Affinity = af("db")
			afTags, err := virtualmachine.DesiredAffinityTags(ctx, declarer)
			Expect(err).ToNot(HaveOccurred())
			var objs []ctrlclient.Object
			for i := range afTags {
				objs = append(objs, afTags[i].DeepCopy())
			}
			vm.Labels = map[string]string{"app": "db"}
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &spec, true, &virtualmachine.LabelDerivedTags{Client: builder.NewFakeClient(objs...)})).To(Succeed())
			Expect(attached(spec)).To(ConsistOf(
				"vmoperator:app:db", MatchRegexp(`^vmoperator:af-rds-pde-host-[0-9a-f]{16}-app:db$`)))
		})

		It("leaves the legacy branch untouched", func() {
			pkgcfg.SetContext(ctx, func(cfg *pkgcfg.Config) {
				cfg.Features.VMHardAffinityDuringExecution = false
			})
			vm.Spec.Affinity = aaf("db")
			vm.Labels = map[string]string{"app": "db"}
			var plain, withOpt vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(ctx, vm, &plain, true, nil)).To(Succeed())
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &withOpt, true, &virtualmachine.LabelDerivedTags{Client: c})).To(Succeed())
			Expect(withOpt).To(Equal(plain))
		})

		Context("update", func() {
			It("attaches label-derived tags for a VM with no affinity", func() {
				vm.Labels = map[string]string{"app": "db"}
				var spec vimtypes.VirtualMachineConfigSpec
				Expect(virtualmachine.UpdateConfigSpecAffinity(ctx, vm,
					&vimtypes.VirtualMachineConfigInfo{}, &spec,
					&virtualmachine.LabelDerivedTags{Client: c})).To(Succeed())
				Expect(attached(spec)).To(ConsistOf(
					"vmoperator:app:db", MatchRegexp(derivedRe)))
			})

			It("writes nothing when the lookup fails", func() {
				vm.Labels = map[string]string{"app": "db"}
				vm.Spec.Affinity = aaf("db")
				failing := builder.NewFakeClientWithInterceptors(interceptor.Funcs{
					List: func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectList, ...ctrlclient.ListOption) error {
						return fmt.Errorf("injected list failure")
					},
				})
				var spec vimtypes.VirtualMachineConfigSpec
				err := virtualmachine.UpdateConfigSpecAffinity(ctx, vm,
					&vimtypes.VirtualMachineConfigInfo{}, &spec,
					&virtualmachine.LabelDerivedTags{Client: failing})
				Expect(err).To(MatchError(ContainSubstring("list affinity Tags")))
				Expect(spec.ExtraConfig).To(BeEmpty())
				Expect(spec.TagSpecs).To(BeEmpty())
				Expect(spec.VmPlacementPolicies).To(BeEmpty())
			})
		})
	})
})

func toIfaces(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}
