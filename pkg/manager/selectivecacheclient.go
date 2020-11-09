// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewSelectiveCacheClient returns a new client.Client that bypasses the cache for ConfigMaps and Secrets.
func NewSelectiveCacheClient(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
	noCacheClient, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	selectiveCacheReader, err := newSelectiveCacheReader(cache, noCacheClient, options)
	if err != nil {
		return nil, err
	}

	delegatingClient := &client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  selectiveCacheReader,
			ClientReader: noCacheClient,
		},
		Writer:       noCacheClient,
		StatusClient: noCacheClient,
	}

	return delegatingClient, nil
}

type selectiveCacheReader struct {
	cacheReader   client.Reader
	noCacheReader client.Reader
	scheme        *runtime.Scheme
	bypassCacheGK map[schema.GroupKind]bool
}

var _ client.Reader = &selectiveCacheReader{}

func newSelectiveCacheReader(cacheReader, noCacheReader client.Reader, options client.Options) (client.Reader, error) {
	scheme := options.Scheme
	if scheme == nil {
		// Same default as client.New()
		scheme = kscheme.Scheme
	}

	s := &selectiveCacheReader{
		cacheReader:   cacheReader,
		noCacheReader: noCacheReader,
		scheme:        scheme,
		bypassCacheGK: map[schema.GroupKind]bool{},
	}

	objs := []runtime.Object{&corev1.ConfigMap{}, &corev1.Secret{}}
	for _, obj := range objs {
		gvk, err := apiutil.GVKForObject(obj, scheme)
		if err != nil {
			return nil, err
		}

		s.bypassCacheGK[gvk.GroupKind()] = true
	}

	return s, nil
}

func (s *selectiveCacheReader) shouldBypassCache(obj runtime.Object) (bool, error) {
	gvk, err := apiutil.GVKForObject(obj, s.scheme)
	if err != nil {
		return false, err
	}

	if strings.HasSuffix(gvk.Kind, "List") && meta.IsListType(obj) {
		// If this was a list, treat it as a request for the item's resource.
		gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	}

	return s.bypassCacheGK[gvk.GroupKind()], nil
}

func (s *selectiveCacheReader) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	bypass, err := s.shouldBypassCache(obj)
	if err != nil {
		return err
	}

	if bypass {
		return s.noCacheReader.Get(ctx, key, obj)
	}

	return s.cacheReader.Get(ctx, key, obj)
}

func (s *selectiveCacheReader) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	bypass, err := s.shouldBypassCache(list)
	if err != nil {
		return err
	}

	if bypass {
		return s.noCacheReader.List(ctx, list, opts...)
	}

	return s.cacheReader.List(ctx, list, opts...)
}
