// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager_test

import (
	"context"
	"net/http"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
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

	Context("Validating webhook configurations", func() {
		It("should cache only the named configuration and follow its lifecycle", func() {
			testCtx := ctx
			reader := suite.GetManager().GetAPIReader()
			key := ctrlclient.ObjectKey{Name: pkgconst.ValidatingWebhookConfigurationName}
			webhook := &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: key.Name},
			}
			other := &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "unrelated-webhook-"},
			}
			// Populate both before the first cached read starts the informer.
			Expect(ctx.Client.Create(ctx, webhook)).To(Succeed())
			DeferCleanup(func() {
				Expect(ctrlclient.IgnoreNotFound(testCtx.Client.Delete(testCtx, webhook))).To(Succeed())
			})
			Expect(ctx.Client.Create(ctx, other)).To(Succeed())
			DeferCleanup(func() {
				Expect(testCtx.Client.Delete(testCtx, other)).To(Succeed())
			})

			var cached admissionv1.ValidatingWebhookConfiguration
			Expect(client.Get(ctx, key, &cached)).To(Succeed())
			var list admissionv1.ValidatingWebhookConfigurationList
			Expect(client.List(ctx, &list)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].Name).To(Equal(key.Name))
			otherKey := ctrlclient.ObjectKeyFromObject(other)
			Expect(reader.Get(ctx, otherKey, &admissionv1.ValidatingWebhookConfiguration{})).To(Succeed())
			Expect(apierrors.IsNotFound(client.Get(ctx, otherKey, &cached))).To(BeTrue())

			webhook.Annotations = map[string]string{"cache-test": "updated"}
			Expect(ctx.Client.Update(ctx, webhook)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(client.Get(ctx, key, &cached)).To(Succeed())
				g.Expect(cached.Annotations).To(HaveKeyWithValue("cache-test", "updated"))
			}).Should(Succeed())

			Expect(ctx.Client.Delete(ctx, webhook)).To(Succeed())
			Eventually(func() bool {
				return apierrors.IsNotFound(client.Get(ctx, key, &cached))
			}).Should(BeTrue())
			webhook = &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: key.Name},
			}
			Expect(ctx.Client.Create(ctx, webhook)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(client.Get(ctx, key, &cached)).To(Succeed())
				g.Expect(cached.UID).To(Equal(webhook.UID))
			}).Should(Succeed())

			// The API server filters these objects before they enter the cache.
			requests := cache.getWebhookRequests()
			Expect(requests).NotTo(BeEmpty())
			Expect(requests).To(ContainElement(webhookRequest{
				selector: "metadata.name=" + key.Name,
				watch:    true,
			}))
			for _, request := range requests {
				Expect(request.selector).To(Equal("metadata.name=" + key.Name))
			}
		})
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
	getCalls        map[ctrlclient.Object]int
	requestMu       sync.Mutex
	webhookRequests []webhookRequest
}

type webhookRequest struct {
	selector string
	watch    bool
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newCacheProxy(
	config *rest.Config,
	opts ctrlcache.Options) (ctrlcache.Cache, error) {

	proxy := &cacheProxy{getCalls: map[ctrlclient.Object]int{}}
	// The manager supplies an HTTP client, so wrap that client's transport
	// without changing the shared client used by direct API readers.
	httpClient := *opts.HTTPClient
	rt := httpClient.Transport
	httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/validatingwebhookconfigurations") {
			proxy.requestMu.Lock()
			proxy.webhookRequests = append(proxy.webhookRequests, webhookRequest{
				selector: req.URL.Query().Get("fieldSelector"),
				watch:    req.URL.Query().Get("watch") == "true",
			})
			proxy.requestMu.Unlock()
		}
		return rt.RoundTrip(req)
	})
	opts.HTTPClient = &httpClient
	cache, err := ctrlcache.New(config, opts)
	if err != nil {
		return nil, err
	}

	proxy.Cache = cache
	return proxy, nil
}

func (c *cacheProxy) getWebhookRequests() []webhookRequest {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()
	return append([]webhookRequest(nil), c.webhookRequests...)
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
