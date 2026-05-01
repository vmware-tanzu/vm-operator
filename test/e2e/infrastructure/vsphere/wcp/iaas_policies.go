// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

type ComputePolicyCapability string

const (
	ComputePolicyCapabilityVMHostAffinity     ComputePolicyCapability = "com.vmware.vcenter.compute.policies.capabilities.vm_host_affinity"
	ComputePolicyCapabilityVMHostAntiAffinity ComputePolicyCapability = "com.vmware.vcenter.compute.policies.capabilities.vm_host_anti_affinity"
)

type ComputePolicySpec struct {
	Name        string
	Description string
	HostTagID   string
	VMTagID     string
	Capability  ComputePolicyCapability
}

type InfraPolicyEnforcementMode string

const (
	InfraPolicyEnforcementModeMandatory InfraPolicyEnforcementMode = "MANDATORY"
	InfraPolicyEnforcementModeOptional  InfraPolicyEnforcementMode = "OPTIONAL"
)

type InfraPolicySpec struct {
	Name               string
	Description        string
	ComputePolicyID    string
	MatchGuestIDValue  string
	MatchWorkloadLabel map[string]string
	EnforcementMode    InfraPolicyEnforcementMode
}

// TagUsageEntry is a single entry from the compute policies tag-usage list.
type TagUsageEntry struct {
	Capability        string `json:"capability"`
	CategoryName      string `json:"category_name"`
	TagType           string `json:"tag_type"`
	TagName           string `json:"tag_name"`
	PolicyDescription string `json:"policy_description"`
	PolicyName        string `json:"policy_name"`
	Tag               string `json:"tag"`
	Policy            string `json:"policy"`
}

// VMPolicyComplianceStatus is the compliance result for a VM against a compute policy.
type VMPolicyComplianceStatus struct {
	Status string `json:"status"`
}
