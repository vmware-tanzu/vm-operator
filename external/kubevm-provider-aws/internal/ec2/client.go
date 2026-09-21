// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package ec2 is the whole of EC2 this provider uses.
//
// The interface is narrow on purpose, and the narrowness is load-bearing
// twice. It makes "no test reaches AWS" provable by construction, because a
// fake implementing six methods is trivial to write and impossible to
// accidentally route to the network. And it makes the required IAM policy
// readable off the type: one method, one action, so the policy cannot silently
// grow beyond what the code calls.
//
// Adding a method here means adding an IAM permission. That cost should be
// visible at the point the method is added.
package ec2

import (
	"context"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Client is every EC2 operation this provider performs.
//
// Signatures match the SDK's generated client exactly, so *awsec2.Client
// satisfies this without an adapter and the fake is the only implementation
// anyone has to write.
//
// Deliberately absent, each because a decision removed the need:
// DescribeImages (the image id is read verbatim from the portable object),
// DescribeSubnets (placement is passed through or omitted),
// DescribeInstanceTypes (nothing checks architecture), and CreateTags (tags
// are applied at launch instead). Restoring any of them restores an IAM
// action. Tagging at launch still needs the ec2:CreateTags permission,
// though never the call.
type Client interface {
	// RunInstances creates an instance. Always exactly one, and always with
	// a ClientToken, or a retried call creates a second machine.
	RunInstances(ctx context.Context, in *awsec2.RunInstancesInput,
		opts ...func(*awsec2.Options)) (*awsec2.RunInstancesOutput, error)

	// DescribeInstances reads instance state for status reporting.
	DescribeInstances(ctx context.Context, in *awsec2.DescribeInstancesInput,
		opts ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error)

	// TerminateInstances destroys an instance.
	TerminateInstances(ctx context.Context,
		in *awsec2.TerminateInstancesInput,
		opts ...func(*awsec2.Options),
	) (*awsec2.TerminateInstancesOutput, error)

	// StartInstances powers a stopped instance on.
	StartInstances(ctx context.Context, in *awsec2.StartInstancesInput,
		opts ...func(*awsec2.Options)) (*awsec2.StartInstancesOutput, error)

	// StopInstances powers a running instance off.
	StopInstances(ctx context.Context, in *awsec2.StopInstancesInput,
		opts ...func(*awsec2.Options)) (*awsec2.StopInstancesOutput, error)

	// ModifyNetworkInterfaceAttribute adds or removes the public IPv4
	// address on an instance's primary network interface.
	ModifyNetworkInterfaceAttribute(ctx context.Context,
		in *awsec2.ModifyNetworkInterfaceAttributeInput,
		opts ...func(*awsec2.Options),
	) (*awsec2.ModifyNetworkInterfaceAttributeOutput, error)
}

// Compile-time proof that the real SDK client satisfies this interface.
//
// Without it, a signature drifting from the SDK's would only be discovered
// wherever the real client is finally constructed, which is one file and one
// call site away from anything a test exercises.
var _ Client = (*awsec2.Client)(nil)
