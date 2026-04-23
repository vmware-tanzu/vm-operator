// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"

	"time"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
)

// E2EConfig extends the shared framework config with infrastructure settings.
// For configuration, it should be a "bottom up" manner in terms of the layering architecture.
type E2EConfig struct {
	framework.Config

	InfraConfig *InfraConfig `json:"infraConfig,omitempty"`
}

type InfraConfig struct {
	// kind, vcenter
	InfraName          string `json:"infraName,omitempty"`
	KubeconfigPath     string `json:"kubeconfigPath,omitempty"`
	NetworkingTopology string `json:"networkingTopology,omitempty"`
	KeepVCSIM          bool   `json:"keepVCSIM,omitempty"`
	// Assume only one namespace for mgmt cluster on the infra provider
	ManagementClusterConfig *ManagementClusterConfig `json:"managementClusterConfig,omitempty"`
}

type ManagementClusterConfig struct {
	ManagementClusterName string     `json:"managementClusterName,omitempty"`
	Resources             *Resources `json:"resources,omitempty"`
}

type Resources struct {
	PhotonImageDisplayName        string `json:"photonImageDisplayName,omitempty"`
	UbuntuImageDisplayName        string `json:"ubuntuImageDisplayName,omitempty"`
	WindowsImageDisplayName       string `json:"windowsImageDisplayName,omitempty"`
	StorageClassName              string `json:"storageClassName,omitempty"`
	WorkerStorageClassName        string `json:"workerStorageClassName,omitempty"`
	VMClassName                   string `json:"vmClassName,omitempty"`
	VMResourcePolicyName          string `json:"vmResourcePolicyName,omitempty"`
	ContentLibrarySubscriptionURL string `json:"contentLibrarySubscriptionURL,omitempty"`
}

func (c *E2EConfig) GetInfraConfig() *InfraConfig {
	return c.InfraConfig
}

// GetIntervals returns the value in the format: "default/key: ["10m", "5s"]".
func (c *E2EConfig) GetIntervals(spec, key string) []any {
	intervals, ok := c.Intervals[fmt.Sprintf("%s/%s", spec, key)]
	if !ok {
		if intervals, ok = c.Intervals[fmt.Sprintf("default/%s", key)]; !ok {
			return nil
		}
	}

	intervalsInterfaces := make([]any, len(intervals))
	for i := range intervals {
		intervalsInterfaces[i] = intervals[i]
	}

	return intervalsInterfaces
}

// GetVariable returns a variable from the e2e config file.
func (c *E2EConfig) GetVariable(varName string) string {
	version, ok := c.Variables[varName]
	Expect(ok).NotTo(BeFalse(), "failed to get variable %q", varName)

	return version
}

// LoadE2EConfig loads the configuration for the e2e test environment.
func LoadE2EConfig(configPath string) *E2EConfig {
	configData, err := os.ReadFile(configPath)

	Expect(err).ToNot(HaveOccurred(), "failed to read the e2e test config file: %s", configPath)
	Expect(configData).ToNot(BeEmpty(), "the e2e test config file should not be empty: %s", configPath)

	configData = []byte(os.Expand(string(configData), func(v string) string {
		parts := strings.SplitN(v, ":-", 2)
		if val, ok := os.LookupEnv(parts[0]); ok && val != "" {
			return val
		}

		if len(parts) == 2 {
			return parts[1]
		}

		return ""
	}))
	config := &E2EConfig{}
	Expect(yaml.Unmarshal(configData, config)).To(Succeed(), "failed to convert the e2e test config file to yaml")

	Expect(config.Validate()).To(Succeed(), "The e2e test config file is not valid")

	return config
}

func (c *E2EConfig) Validate() error {
	// kind or wcp must be provided
	if c.InfraConfig.InfraName == "" {
		return framework.ErrEmptyArg("InfraConfig.InfraName")
	}

	// Intervals should be valid ginkgo intervals.
	for k, intervals := range c.Intervals {
		switch len(intervals) {
		case 0:
			return framework.ErrInvalidArg("Intervals[%s]=%q", k, intervals)
		case 1, 2:
		default:
			return framework.ErrInvalidArg("Intervals[%s]=%q", k, intervals)
		}

		for _, i := range intervals {
			if _, err := time.ParseDuration(i); err != nil {
				return framework.ErrInvalidArg("Intervals[%s]=%q", k, intervals)
			}
		}
	}

	return nil
}
