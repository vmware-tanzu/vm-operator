// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package awsclient is the only place in this repository permitted to build a
// real AWS client.
//
// That restriction is enforced, not requested: internal/enforce scans the
// syntax tree of every non-test file for SDK constructors and fails on any
// occurrence outside this one. A rule a reviewer has to remember is a rule
// that eventually gets forgotten.
//
// Credentials are never a parameter here, and never an API field anywhere.
// They arrive through the SDK's own default chain: a Secret projected as
// environment variables when running off AWS, an IAM Roles for Service
// Accounts token on EKS. The code cannot tell which, which is exactly what
// lets one Deployment manifest serve both.
package awsclient

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
)

// New returns an EC2 client for one region, authenticated by whatever the
// SDK's default credential chain finds.
//
// The SDK's own retryer is left exactly as it comes. It already handles
// RequestLimitExceeded and the other throttling codes with the backoff AWS
// recommends; replacing it would mean reimplementing that worse, and wrapping
// it would hide which layer gave up.
func New(ctx context.Context, region string) (ec2.Client, error) {
	if region == "" {
		return nil, fmt.Errorf(
			"no AWS region configured: set AWS_REGION on the manager, " +
				"since one deployment serves one account and one region")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region))
	if err != nil {
		// Deliberately not wrapped with any credential detail. The SDK's
		// message names which step of the chain failed, which is what an
		// operator needs, and nothing more.
		return nil, fmt.Errorf("resolving AWS credentials: %w", err)
	}

	return awsec2.NewFromConfig(cfg), nil
}
