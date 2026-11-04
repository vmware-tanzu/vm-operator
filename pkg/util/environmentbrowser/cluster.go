// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package environmentbrowser

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/version"

	"github.com/vmware-tanzu/vm-operator/pkg/providers"
)

// Cluster identifies an owning cluster and its current discovery inventory.
type Cluster struct {
	ID        string
	UID       string
	Inventory providers.EnvironmentBrowserInventory
}

// SelectCluster chooses the oldest host software version/build, retaining one
// internally consistent cluster result. It also returns an owner-set revision;
// unknown revisions disable caching. Cluster ID breaks equal-version ties.
func SelectCluster(clusters []Cluster) (string, string, error) {
	type candidate struct {
		cluster Cluster
		version *version.Version
	}
	candidates := make([]candidate, 0, len(clusters))
	revisions := make([]string, 0, len(clusters))
	cacheable := true
	for _, cluster := range clusters {
		var v *version.Version
		if cluster.Inventory.MinHostVersion != "" {
			var err error
			v, err = version.ParseGeneric(cluster.Inventory.MinHostVersion)
			if err != nil {
				return "", "", fmt.Errorf("invalid host version for cluster %s: %w", cluster.ID, err)
			}
		}
		if cluster.Inventory.Revision == "" {
			cacheable = false
		}
		candidates = append(candidates, candidate{cluster, v})
		revisions = append(revisions, cluster.ID+":"+cluster.UID+":"+cluster.Inventory.Revision)
	}
	if len(candidates) == 0 {
		return "", "", nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		// Unknown software cannot be ranked above known software.
		if (a.version == nil) != (b.version == nil) {
			return a.version == nil
		}
		if a.version != nil && !a.version.EqualTo(b.version) {
			return a.version.LessThan(b.version)
		}
		if a.cluster.Inventory.MinHostBuild != b.cluster.Inventory.MinHostBuild {
			return a.cluster.Inventory.MinHostBuild < b.cluster.Inventory.MinHostBuild
		}
		return a.cluster.ID < b.cluster.ID
	})
	sort.Strings(revisions)
	revision := ""
	if cacheable {
		revision = strings.Join(revisions, ",")
	}
	return candidates[0].cluster.ID, revision, nil
}
