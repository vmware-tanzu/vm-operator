// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func cacheTests() {
	var (
		ctx    *builder.IntegrationTestContext
		client ctrlclient.Client
		cache  *cacheProxy
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		client = suite.GetManager().GetClient()
		Expect(suite.GetManager().GetCache()).To(BeAssignableToTypeOf(&cacheProxy{}))
		cache = suite.GetManager().GetCache().(*cacheProxy)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Getting objects", func() {
		var (
			obj    ctrlclient.Object
			objKey ctrlclient.ObjectKey
		)
		JustBeforeEach(func() {
			obj.SetGenerateName("my-object-")
			obj.SetNamespace(ctx.Namespace)
			Expect(client.Create(ctx, obj)).To(Succeed())
			objKey = ctrlclient.ObjectKeyFromObject(obj)
		})

		Context("ConfigMap", func() {
			BeforeEach(func() {
				obj = &corev1.ConfigMap{}
			})
			It("should return the object with a live read", func() {
				Expect(client.Get(ctx, objKey, obj)).To(Succeed())
				Expect(cache.getCallCountFor(obj)).To(Equal(0))
			})
		})
		Context("Node", func() {
			BeforeEach(func() {
				obj = &corev1.Node{}
			})
			It("should return the object with a live read", func() {
				Expect(client.Get(ctx, objKey, obj)).To(Succeed())
				Expect(cache.getCallCountFor(obj)).To(Equal(1))
			})
		})
		Context("Secret", func() {
			BeforeEach(func() {
				obj = &corev1.Secret{}
			})
			It("should return the object with a live read", func() {
				Expect(client.Get(ctx, objKey, obj)).To(Succeed())
				Expect(cache.getCallCountFor(obj)).To(Equal(0))
			})
		})
	})
}

type cacheProxy struct {
	sync.Mutex
	ctrlcache.Cache
	getCalls map[ctrlclient.Object]int
}

func newCacheProxy(
	config *rest.Config,
	opts ctrlcache.Options) (ctrlcache.Cache, error) {

	cache, err := ctrlcache.New(config, opts)
	if err != nil {
		return nil, err
	}

	return &cacheProxy{
		Cache:    cache,
		getCalls: map[ctrlclient.Object]int{},
	}, nil
}

func (c *cacheProxy) getCallCountFor(obj ctrlclient.Object) int {
	c.Lock()
	defer c.Unlock()
	return c.getCalls[obj]
}

func (c *cacheProxy) Get(
	ctx context.Context,
	key ctrlclient.ObjectKey,
	obj ctrlclient.Object,
	opts ...ctrlclient.GetOption) error {

	c.Lock()
	defer c.Unlock()

	if i, ok := c.getCalls[obj]; ok {
		c.getCalls[obj] = i + 1
	} else {
		c.getCalls[obj] = 1
	}

	return c.Cache.Get(ctx, key, obj, opts...)
}
