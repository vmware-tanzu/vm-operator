// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// GetNamespaceCacheConfigs returns a map of cache configurations for the
// provided namespaces. A nil value is returned if the provided list is
// empty or has a single, empty element.
func GetNamespaceCacheConfigs(namespaces ...string) map[string]cache.Config {
	if len(namespaces) == 0 {
		return nil
	}
	if len(namespaces) == 1 && namespaces[0] == "" {
		return nil
	}
	nsc := make(map[string]cache.Config, len(namespaces))
	for i := range namespaces {
		if v := namespaces[i]; v != "" {
			nsc[v] = cache.Config{}
		}
	}
	return nsc
}
