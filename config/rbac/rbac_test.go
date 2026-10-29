// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package rbac_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	rbacv1 "k8s.io/api/rbac/v1"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"slices"
	"strings"
)

var _ = Describe("Environment browser access", Label(testlabels.API), func() {
	It("grants discovery only to the operator role, without tenant-role aggregation", func() {
		files, err := filepath.Glob("*.yaml")
		Expect(err).NotTo(HaveOccurred())
		resources := []string{"configtargets", "virtualmachineconfigoptions", "virtualmachineguestoptions"}
		granted := map[string]bool{}
		for _, file := range files {
			data, err := os.ReadFile(file)
			Expect(err).NotTo(HaveOccurred())
			var role rbacv1.ClusterRole
			Expect(yaml.Unmarshal(data, &role)).To(Succeed())
			if role.Kind != "ClusterRole" && role.Kind != "Role" {
				continue
			}
			for _, rule := range role.Rules {
				if !slices.Contains(rule.APIGroups, "vim.vmware.com") && !slices.Contains(rule.APIGroups, "*") {
					continue
				}
				for _, resource := range resources {
					if !slices.Contains(rule.Resources, resource) && !slices.Contains(rule.Resources, "*") {
						continue
					}
					Expect(role.Name).To(Equal("manager-role"), "unexpected discovery grant in %s", file)
					for key := range role.Labels {
						Expect(strings.HasPrefix(key, "rbac.authorization.k8s.io/aggregate-to-")).To(BeFalse())
					}
					Expect(rule.Verbs).To(ContainElements("get", "list", "watch", "create", "update", "patch", "delete"))
					granted[resource] = true
				}
			}
		}
		Expect(granted).To(HaveLen(len(resources)))
		data, err := os.ReadFile("role_binding.yaml")
		Expect(err).NotTo(HaveOccurred())
		var binding rbacv1.ClusterRoleBinding
		Expect(yaml.Unmarshal(data, &binding)).To(Succeed())
		Expect(binding.RoleRef.Name).To(Equal("manager-role"))
		Expect(binding.Subjects).To(Equal([]rbacv1.Subject{{Kind: "ServiceAccount", Name: "vmoperator-service-account", Namespace: "system"}}))
	})
})
