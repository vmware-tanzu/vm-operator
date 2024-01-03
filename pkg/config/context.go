// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"sync"
)

type configContextKey uint8

const configContextKeyValue configContextKey = 0

type threadSafeConfig struct {
	sync.RWMutex
	Config
}

// NewContext returns a new context with an empty Config.
func NewContext() context.Context {
	return WithConfig(Config{})
}

// NewContextWithDefaultConfig returns a new context with a default Config.
func NewContextWithDefaultConfig() context.Context {
	return WithConfig(Default())
}

// JoinContext returns a new context that contains a reference to the Config
// from the specified context.
// This function panics if the provided context does not contain a Config.
// This function is thread-safe.
func JoinContext(parent context.Context, contextWithConfig context.Context) context.Context {
	if parent == nil {
		panic("parent context is nil")
	}
	if contextWithConfig == nil {
		panic("contextWithConfig is nil")
	}
	obj := contextWithConfig.Value(configContextKeyValue)
	if obj == nil {
		panic("config is missing from context")
	}
	return context.WithValue(parent, configContextKeyValue, obj.(*threadSafeConfig))
}

// WithConfig returns a new context with the provided Config.
func WithConfig(config Config) context.Context {
	return WithContext(context.Background(), config)
}

// WithContext sets the provided Config in the provided context.
// If ctx is nil, one is created with context.Background.
func WithContext(parent context.Context, config Config) context.Context {
	if parent == nil {
		panic("parent context is nil")
	}
	return context.WithValue(
		parent,
		configContextKeyValue,
		&threadSafeConfig{Config: config})
}

// SetContext allows the Config in the provided context to be updated.
// This function panics if the provided context is nil, does not contain a
// Config, or setFn is nil.
// This function is thread-safe.
func SetContext(ctx context.Context, setFn func(config *Config)) {
	_ = UpdateContext(ctx, setFn)
}

// UpdateContext allows the Config in the provided context to be updated.
// This function returns the same context passed into the ctx parameter.
// This function panics if the provided context is nil, does not contain a
// Config, or setFn is nil.
// This function is thread-safe.
func UpdateContext(ctx context.Context, setFn func(config *Config)) context.Context {
	if ctx == nil {
		panic("context is nil")
	}
	if setFn == nil {
		panic("setFn is nil")
	}
	obj := ctx.Value(configContextKeyValue)
	if obj == nil {
		panic("config is missing from context")
	}
	config := obj.(*threadSafeConfig)
	config.Lock()
	defer config.Unlock()
	copyOfConfig := config.Config
	setFn(&copyOfConfig)
	config.Config = copyOfConfig
	return ctx
}

// FromContext returns the Config from the provided context.
// This function panics if the provided context is nil or does not contain a
// Config.
// This function is thread-safe.
func FromContext(ctx context.Context) Config {
	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(configContextKeyValue)
	if obj == nil {
		panic("config is missing from context")
	}
	config := obj.(*threadSafeConfig)
	config.RLock()
	defer config.RUnlock()
	return config.Config // return by value is a copy
}
