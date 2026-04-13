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
