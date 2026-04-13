// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
)

const (
	clusterMoIDKey              = "vc_cluster"
	fullClusterID               = "cluster_id_full"
	supervisorID                = "supervisor_id"
	vCenterPNIDKey              = "vc_pnid"
	vCenterIPKey                = "VCSA_IP"
	clusterConfigMapName        = "wcp-cluster-config"
	kubeSystemNamespace         = "kube-system"
	clusterConfigJSONPathFilter = "jsonpath={.data.wcp-cluster-config\\.yaml}"
	VCSSHPort                   = 22
)

// GetClusterMoIDFromKubeconfig uses the framework's current kubeconfig context to determine the MoID of the WCP enabled cluster.
// This relies on the present of the wcp-cluster-config ConfigMap in kube-system
// Only users in the Administrators@<domain> group can access this.
func GetClusterMoIDFromKubeconfig(ctx context.Context, path string) string {
	yamlData := getWCPClusterConfig(ctx, path)
	return yamlData[clusterMoIDKey]
}

// GetSupervisorIDFromKubeconfig uses the framework's current kubeconfig context to determine the supervisor ID.
// This relies on the presence of the wcp-cluster-config ConfigMap in kube-system
// Only users in the Administrators@<domain> group can access this.
func GetSupervisorIDFromKubeconfig(ctx context.Context, path string) string {
	yamlData := getWCPClusterConfig(ctx, path)
	return yamlData[supervisorID]
}

// GetFullClusterIDFromKubeConfig uses the framework's current kubeconfig context to determine the full ID of the WCP enabled cluster.
// This relies on the presence of the wcp-cluster-config ConfigMap in kube-system
// Only users in the Administrators@<domain> group can access this.
func GetFullClusterIDFromKubeConfig(ctx context.Context, path string) string {
	yamlData := getWCPClusterConfig(ctx, path)
	return yamlData[fullClusterID]
}

// GetVCPNIDFromKubeconfig uses the framework's current kubeconfig context to determine the PNID of the VC that the WCP enabled cluster was deployed by.
// This primarily relies of the VCSA_IP env variable that is setup during the gce2e runs
// As a fallback strategy it relies on the vc_pnid key present in the wcp-cluster-config ConfigMap in kube-system
// Only users in the Administrators@<domain> group can access this.
func GetVCPNIDFromKubeconfig(ctx context.Context, path string) string {
	val, ok := os.LookupEnv(vCenterIPKey)
	if !ok {
		// Useful when the current context does not point to a kubeconfig, eg. during GC app tests.
		yamlData := getWCPClusterConfig(ctx, path)
		return yamlData[vCenterPNIDKey]
	}

	return val
}

// GetVCPNIDFromKubeconfigFile (See [GetVCPNIDFromKubeconfig]).
func GetVCPNIDFromKubeconfigFile(ctx context.Context, path string) string {
	return GetVCPNIDFromKubeconfig(ctx, path)
}

// GetClusterMoIDFromKubeconfigFile returns the clusterMoID from a kubeconfig file currently pointing to a supervisor cluster.
func GetClusterMoIDFromKubeconfigFile(ctx context.Context, path string) string {
	// Useful when the current context does not point to a kubeconfig, eg. during GC app tests.
	yamlData := getWCPClusterConfig(ctx, path)
	return yamlData[clusterMoIDKey]
}

// CreateUserAndAssignToGrp creates user with given username and password, and assigns it to the given group.
func CreateUserAndAssignToGrp(ctx context.Context, vimClient *vim25.Client, sshCommandRunner e2essh.SSHCommandRunner, userName, password, group string) (*User, error) {
	// Create the non-admin VC user via admin
	user := NewUser(userName, password).
		WithAdminCreds(dcli.VCenterUserCredentials{
			Username: testbed.AdminUsername,
			Password: testbed.AdminPassword,
		}).
		WithSSHCommandRunner(sshCommandRunner)

	CreateUserOrFail(user)

	// Add user to SupervisorProviderAdministrators via admin vimClient
	err := AddToGroup(ctx, vimClient, user.Credentials.Username, group)
	if err != nil {
		return nil, err
	}

	return user, err
}

// getWCPClusterConfig is helper method to get the WCP cluster config.
func getWCPClusterConfig(ctx context.Context, path string) map[string]string {
	applyCmd := framework.NewCommand(
		framework.WithCommand("cat"),
		framework.WithArgs(path),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Printf("stderr getting kubeconfig file: %s", string(stderr))
		fmt.Printf("err getting kubeconfig file: %v", err)
	}
	// TODO: kubectl GET returns "Unable to connect to the server: Service Unavailable" sometimes. Add retry to further debugging.
	var configYAML []byte

	Eventually(func() error {
		configYAML, err = framework.KubectlGet(ctx, path, "configmap", clusterConfigMapName, "-n", kubeSystemNamespace, "-o", clusterConfigJSONPathFilter)
		return err
	}, 5*time.Minute, 10*time.Second).ShouldNot(HaveOccurred(), "failed to get configmap %q/%q with jsonpath=%q. Kubeconfig file: %s", kubeSystemNamespace, clusterConfigMapName, clusterConfigJSONPathFilter, string(stdout))

	yamlData := map[string]string{}
	err = yaml.Unmarshal(configYAML, yamlData)
	Expect(err).NotTo(HaveOccurred(), "should be able to parse the WCP cluster config as YAML: %s", string(configYAML))

	return yamlData
}

// NewVcsimFinder returns a vcsim finder.
func NewVcsimFinder(ctx context.Context, vimClient *vim25.Client) *find.Finder {
	finder := find.NewFinder(vimClient, false)
	dc, err := finder.DefaultDatacenter(ctx)
	Expect(err).NotTo(HaveOccurred(), "Should be able to get the default DC.")
	finder.SetDatacenter(dc)

	return finder
}

// GetClusterRefs returns available clusters in the vcsim.
func GetClusterRefs(ctx context.Context, finder *find.Finder) []*object.ClusterComputeResource {
	clusters, err := finder.ClusterComputeResourceList(ctx, "*")
	Expect(err).NotTo(HaveOccurred(), "Should be able to find clusters.")
	Expect(clusters).ShouldNot(BeEmpty(), "Should be able to find at least one cluster.")

	return clusters
}

func GetClusterResourcePool(ctx context.Context, finder *find.Finder) types.ManagedObjectReference {
	clusters := GetClusterRefs(ctx, finder)
	Expect(len(clusters)).To(BeNumerically(">", 0))
	rp, err := clusters[0].ResourcePool(ctx)
	Expect(err).ToNot(HaveOccurred())

	rpRef := rp.Reference()

	return rpRef
}

func GetClusterIDToRefMapping(clusterRefs []*object.ClusterComputeResource) map[string]*object.ClusterComputeResource {
	refMapping := make(map[string]*object.ClusterComputeResource)
	for _, ref := range clusterRefs {
		refMapping[ref.Reference().Value] = ref
	}

	return refMapping
}

// SimulateFindClusterByMoid returns a cluster reference in the vcsim by the given cluster moid.
func SimulateFindClusterByMoid(ctx context.Context, vcip, moid string) *object.ClusterComputeResource {
	vimClient := NewVcSimClient(ctx, vcip)
	finder := NewVcsimFinder(ctx, vimClient)

	clusterRefs := GetClusterRefs(ctx, finder)
	refMapping := GetClusterIDToRefMapping(clusterRefs)
	Expect(refMapping).Should(HaveKey(moid))

	return refMapping[moid]
}

// GetResourcePoolAndFolder returns associated resource pool and folder for a cluster.
// Note: the folder here is the root folder of datacenter DC0 in the vcsim.
func GetResourcePoolAndFolder(ctx context.Context, finder *find.Finder,
	cluster *object.ClusterComputeResource) (rp, folder string) {
	rpRef, err := cluster.ResourcePool(ctx)
	Expect(err).NotTo(HaveOccurred(), "Should return the cluster's resource pool.")

	rp = rpRef.Reference().Value

	folderRef, err := finder.DefaultFolder(ctx)
	Expect(err).NotTo(HaveOccurred(), "Should return default folder.")

	folder = folderRef.Reference().Value

	return rp, folder
}

// DeleteFolder deletes a folder and all its contents (VMs/templates/folders).
func DeleteFolder(ctx context.Context, folder *object.Folder) {
	task, err := folder.Destroy(ctx)
	Expect(err).ToNot(HaveOccurred())

	// Wait for task completion
	err = task.Wait(ctx)
	Expect(err).ToNot(HaveOccurred())
}
