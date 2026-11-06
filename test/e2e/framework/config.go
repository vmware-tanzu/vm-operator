// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
// https://github.com/kubernetes-sigs/cluster-api/blob/a260cb4c3b2cf2f32379f6ae5d8c6312780e9ca5/test/framework/clusterctl/e2e_config.go

package framework

import (
	"fmt"

	. "github.com/onsi/gomega"
)

type ConfigInterface interface {
	GetIntervals(spec, key string) []any
	GetVariable(varName string) string
}

type Config struct {
	Variables map[string]string `json:"variables,omitempty"`
	// Intervals to be used for long operations during tests
	Intervals map[string][]string `json:"intervals,omitempty"`
}

// GetIntervals returns the value in the format: "default/key: ["10m", "5s"]".
func (c *Config) GetIntervals(spec, key string) []any {
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
func (c *Config) GetVariable(varName string) string {
	version, ok := c.Variables[varName]
	Expect(ok).NotTo(BeFalse(), "failed to get variable %q", varName)

	return version
}

// https://github.com/kubernetes-sigs/cluster-api/blob/defb99c408b54dd8b58fc586eb424425b4198484/test/framework/clusterctl/e2e_config.go#L174
func ErrInvalidArg(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("invalid argument: %s", msg)
}

func ErrEmptyArg(argName string) error {
	return ErrInvalidArg("%s is empty", argName)
}

// SetVariable sets a variable in the loaded e2e config.
// Use it with caution as it will not be persisted to the config file.
func (c *Config) SetVariable(varName string, value string) {
	c.Variables[varName] = value
}
