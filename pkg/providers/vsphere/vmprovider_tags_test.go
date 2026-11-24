// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/tags"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("ResolveTagIDs", Label(testlabels.VCSim), func() {
	var (
		ctx        *builder.TestContextForVCSim
		vmProvider providers.VirtualMachineProviderInterface
		mgr        *tags.Manager
	)

	const (
		catName  = "resolve-ns"
		otherCat = "resolve-other-ns"
	)

	createCategory := func(name string) string {
		id, err := mgr.CreateCategory(ctx, &tags.Category{
			Name:            name,
			Cardinality:     "MULTIPLE",
			AssociableTypes: []string{"VirtualMachine"},
		})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		return id
	}

	createTag := func(categoryID, name string) string {
		id, err := mgr.CreateTag(ctx, &tags.Tag{
			Name:       name,
			CategoryID: categoryID,
		})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		return id
	}

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		mgr = tags.NewManager(ctx.RestClient)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vmProvider = nil
	})

	It("returns the category and tag IDs", func() {
		cID := createCategory(catName)
		t1 := createTag(cID, "vmoperator:a")
		t2 := createTag(cID, "vmoperator:b")

		gotCat, ids, err := vmProvider.ResolveTagIDs(
			ctx, catName, []string{"vmoperator:a", "vmoperator:b"})
		Expect(err).ToNot(HaveOccurred())
		Expect(gotCat).To(Equal(cID))
		Expect(ids).To(Equal(map[string]string{
			"vmoperator:a": t1,
			"vmoperator:b": t2,
		}))
	})

	It("returns no IDs when the category is missing", func() {
		gotCat, ids, err := vmProvider.ResolveTagIDs(
			ctx, "no-such-category", []string{"vmoperator:a"})
		Expect(err).ToNot(HaveOccurred())
		Expect(gotCat).To(BeEmpty())
		Expect(ids).To(BeEmpty())
	})

	It("omits tags that do not exist", func() {
		cID := createCategory(catName)
		t1 := createTag(cID, "vmoperator:a")

		gotCat, ids, err := vmProvider.ResolveTagIDs(
			ctx, catName, []string{"vmoperator:a", "vmoperator:missing"})
		Expect(err).ToNot(HaveOccurred())
		Expect(gotCat).To(Equal(cID))
		Expect(ids).To(Equal(map[string]string{"vmoperator:a": t1}))
	})

	It("ignores a tag with the same name in another category", func() {
		cID := createCategory(catName)
		oID := createCategory(otherCat)
		createTag(oID, "vmoperator:a")

		gotCat, ids, err := vmProvider.ResolveTagIDs(
			ctx, catName, []string{"vmoperator:a"})
		Expect(err).ToNot(HaveOccurred())
		Expect(gotCat).To(Equal(cID))
		Expect(ids).To(BeEmpty())

		t1 := createTag(cID, "vmoperator:a")
		_, ids, err = vmProvider.ResolveTagIDs(
			ctx, catName, []string{"vmoperator:a"})
		Expect(err).ToNot(HaveOccurred())
		Expect(ids).To(Equal(map[string]string{"vmoperator:a": t1}))
	})

	It("requires a category name", func() {
		_, _, err := vmProvider.ResolveTagIDs(
			ctx, "", []string{"vmoperator:a"})
		Expect(err).To(MatchError(ContainSubstring("category name is required")))
	})

	It("returns early when there are no names", func() {
		gotCat, ids, err := vmProvider.ResolveTagIDs(ctx, catName, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotCat).To(BeEmpty())
		Expect(ids).To(BeEmpty())
	})

	When("the category ID is cached", func() {
		It("serves from the cache and refreshes it after the category is recreated", func() {
			cID := createCategory(catName)
			t1 := createTag(cID, "vmoperator:a")

			// Prime the cache.
			_, ids, err := vmProvider.ResolveTagIDs(
				ctx, catName, []string{"vmoperator:a"})
			Expect(err).ToNot(HaveOccurred())
			Expect(ids).To(HaveKeyWithValue("vmoperator:a", t1))

			// A cache hit resolves a tag created later.
			t2 := createTag(cID, "vmoperator:b")
			_, ids, err = vmProvider.ResolveTagIDs(
				ctx, catName, []string{"vmoperator:b"})
			Expect(err).ToNot(HaveOccurred())
			Expect(ids).To(HaveKeyWithValue("vmoperator:b", t2))

			// Recreate the category: the cached ID is stale and a miss must
			// invalidate it.
			cat, err := mgr.GetCategory(ctx, cID)
			Expect(err).ToNot(HaveOccurred())
			Expect(mgr.DeleteCategory(ctx, cat)).To(Succeed())
			newID := createCategory(catName)
			t3 := createTag(newID, "vmoperator:a")

			gotCat, ids, err := vmProvider.ResolveTagIDs(
				ctx, catName, []string{"vmoperator:a"})
			Expect(err).ToNot(HaveOccurred())
			Expect(gotCat).To(Equal(newID))
			Expect(ids).To(Equal(map[string]string{"vmoperator:a": t3}))
		})
	})
})
