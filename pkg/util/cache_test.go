// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("CacheTest", func() {
	type person struct {
		Name string
		Age  int
	}

	var (
		c                   *util.Cache[person]
		expireAfter         time.Duration
		checkExpireInterval time.Duration
		maxItems            int
	)

	BeforeEach(func() {
		expireAfter = 3 * time.Second
		checkExpireInterval = 1 * time.Second
		maxItems = 3
	})

	AfterEach(func() {
		c.Close()
	})

	JustBeforeEach(func() {
		c = util.NewCache[person](expireAfter, checkExpireInterval, maxItems)
	})

	It("item should be created and expire after three seconds", func() {
		Expect(c.Put("1", person{Name: "name"})).To(Equal(util.CachePutResultCreate))
		p, ok := c.Get("1", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name"}))
		Eventually(func() string {
			if p, ok := c.Get("1", nil); ok {
				return p.Name
			}
			return ""
		}, 5*time.Second, 1*time.Second).Should(BeEmpty())
	})

	It("item should be updated and expire after three seconds", func() {
		Expect(c.Put("1", person{Name: "name"})).To(Equal(util.CachePutResultCreate))
		p, ok := c.Get("1", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name"}))

		Expect(c.Put("1", person{Name: "new-name"})).To(Equal(util.CachePutResultUpdate))
		p, ok = c.Get("1", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "new-name"}))

		Eventually(func() string {
			if p, ok := c.Get("1", nil); ok {
				return p.Name
			}
			return ""
		}, 5*time.Second, 1*time.Second).Should(BeEmpty())
	})

	It("should receive eviction notice even if cache is closed", func() {
		Expect(c.Put("1", person{Name: "name"})).To(Equal(util.CachePutResultCreate))
		p, ok := c.Get("1", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name"}))

		Eventually(func() string {
			if p, ok := c.Get("1", nil); ok {
				return p.Name
			}
			return ""
		}, 5*time.Second, 1*time.Second).Should(BeEmpty())

		c.Close()

		Expect(<-c.ExpiredChan()).To(Equal("1"))
		Eventually(c.ExpiredChan()).Should(BeClosed())
	})

	It("should prevent more than max items from being added", func() {
		Expect(c.Put("1", person{Name: "name"})).To(Equal(util.CachePutResultCreate))
		p, ok := c.Get("1", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name"}))

		Expect(c.Put("2", person{Name: "name"})).To(Equal(util.CachePutResultCreate))
		p, ok = c.Get("2", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name"}))

		Expect(c.Put("3", person{Name: "name"})).To(Equal(util.CachePutResultCreate))
		p, ok = c.Get("3", nil)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name"}))

		Expect(c.Put("4", person{Name: "name"})).To(Equal(util.CachePutResultMaxItemsExceeded))
		p, ok = c.Get("4", nil)
		Expect(ok).To(BeFalse())
		Expect(p).To(Equal(person{}))
	})

	It("should match only if isHit", func() {
		Expect(c.Put("1", person{Name: "name", Age: 99})).To(Equal(util.CachePutResultCreate))
		Expect(c.Put("2", person{Name: "name", Age: 150})).To(Equal(util.CachePutResultCreate))

		eqGt100 := func(p person) bool {
			return p.Age >= 100
		}

		p, ok := c.Get("1", eqGt100)
		Expect(ok).To(BeFalse())
		Expect(p).To(Equal(person{}))

		p, ok = c.Get("2", eqGt100)
		Expect(ok).To(BeTrue())
		Expect(p).To(Equal(person{Name: "name", Age: 150}))
	})
})
