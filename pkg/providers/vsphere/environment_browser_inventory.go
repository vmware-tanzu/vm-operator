// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/version"

	"github.com/vmware-tanzu/vm-operator/pkg/providers"
)

// GetEnvironmentBrowserInventory reads discovery inputs without calling the
// EnvironmentBrowser. Runtime counters and VM membership are deliberately not
// included: a resync or VM power transition does not change hardware options.
func (vs *vSphereVMProvider) GetEnvironmentBrowserInventory(ctx context.Context, clusterMoID string) (providers.EnvironmentBrowserInventory, error) {
	var result providers.EnvironmentBrowserInventory
	c, err := vs.getVcClient(ctx)
	if err != nil {
		return result, err
	}
	pc := property.DefaultCollector(c.VimClient())
	var cluster mo.ClusterComputeResource
	ref := vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: clusterMoID}
	if err := pc.RetrieveOne(ctx, ref, []string{"host", "environmentBrowser", "configurationEx", "network", "datastore"}, &cluster); err != nil {
		return result, fmt.Errorf("retrieve cluster %s inventory: %w", clusterMoID, err)
	}
	// A cluster can inherit its default hardware version from the datacenter.
	var dc mo.Datacenter
	if c.Datacenter() == nil {
		return result, fmt.Errorf("cluster %s has no configured datacenter", clusterMoID)
	}
	if err := pc.RetrieveOne(ctx, c.Datacenter().Reference(), []string{"configuration"}, &dc); err != nil {
		return result, fmt.Errorf("retrieve datacenter hardware-version policy: %w", err)
	}
	sortRefs := func(refs []vimtypes.ManagedObjectReference) {
		sort.Slice(refs, func(i, j int) bool { return refs[i].String() < refs[j].String() })
	}
	sortRefs(cluster.Host)
	sortRefs(cluster.Network)
	sortRefs(cluster.Datastore)
	var hosts []mo.HostSystem
	if len(cluster.Host) > 0 {
		// Config includes product, PCI/GPU inventory, networking and storage.
		// Hardware and capability include the limits and feature support that
		// determine both config targets and per-hardware-version options.
		if err := pc.Retrieve(ctx, cluster.Host, []string{"config", "hardware", "capability", "runtime.connectionState", "runtime.inMaintenanceMode"}, &hosts); err != nil {
			return result, fmt.Errorf("retrieve cluster %s hosts: %w", clusterMoID, err)
		}
	}
	if len(hosts) != len(cluster.Host) {
		return result, fmt.Errorf("incomplete inventory for cluster %s: expected %d hosts, got %d", clusterMoID, len(cluster.Host), len(hosts))
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Reference().Value < hosts[j].Reference().Value })
	var oldest *version.Version
	for _, host := range hosts {
		if host.Config == nil {
			return result, fmt.Errorf("host %s has no configuration", host.Reference().Value)
		}
		v, err := version.ParseGeneric(host.Config.Product.Version)
		if err != nil {
			return result, fmt.Errorf("host %s product version: %w", host.Reference().Value, err)
		}
		build, err := strconv.ParseUint(host.Config.Product.Build, 10, 64)
		if err != nil {
			return result, fmt.Errorf("host %s product build: %w", host.Reference().Value, err)
		}
		if oldest == nil || v.LessThan(oldest) || v.EqualTo(oldest) && build < result.MinHostBuild {
			oldest = v
			result.MinHostVersion = host.Config.Product.Version
			result.MinHostBuild = build
		}
	}
	data, err := json.Marshal(struct {
		Cluster    mo.ClusterComputeResource
		Hosts      []mo.HostSystem
		Server     vimtypes.AboutInfo
		Datacenter vimtypes.DatacenterConfigInfo
	}{cluster, hosts, c.VimClient().ServiceContent.About, dc.Configuration})
	if err != nil {
		return result, fmt.Errorf("encode cluster %s inventory: %w", clusterMoID, err)
	}
	result.Revision = fmt.Sprintf("%x", sha256.Sum256(data))
	return result, nil
}
