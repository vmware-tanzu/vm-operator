// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package awsmachine

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/intent"
)

// applyPublicIP adds or removes the instance's public address to match what
// the portable object asks for, and reports what it did.
//
// Read from the parent's intent, not from this object's spec, for the reason
// applyPower gives: the spec is a copy, and a copy is whatever was written
// last rather than what is being asked for now.
func (r *Reconciler) applyPublicIP(ctx context.Context,
	want *intent.Intent, instance *ec2types.Instance,
	state ec2types.InstanceStateName) stepResult {

	// Nil means the portable object states no preference, so whatever the
	// subnet's default gave the instance is left alone.
	wanted, stated := want.WantsPublicIP()
	if !stated {
		return stepResult{}
	}

	// Only a running instance has a public address to add or remove. A
	// stopped one is corrected when it next runs, and a moving one on the
	// pass after it settles.
	if state != ec2types.InstanceStateNameRunning {
		return stepResult{}
	}

	has := aws.ToString(instance.PublicIpAddress) != ""
	if has == wanted {
		return stepResult{}
	}

	// An Elastic IP is not ours to take away. AssociatePublicIpAddress
	// governs only the address EC2 assigns automatically, so asking it to
	// remove an EIP changes nothing -- and the mismatch would still be there
	// on the next pass, and the one after that, forever. Report it instead.
	if !wanted && isElasticIP(instance) {
		want.Unsupported = append(want.Unsupported,
			"spec.network.interfaces[0].publicIP is false and the instance "+
				"has an Elastic IP, which this provider did not allocate "+
				"and does not release")
		return stepResult{}
	}

	// EC2 always reports a primary interface for a running instance; with
	// none there is nothing to name, and the next pass will look again.
	eni := primaryInterface(instance)
	if eni == "" {
		return stepResult{}
	}

	// EC2 applies the change shortly after the call returns, so the result
	// is read back on the next pass rather than trusted.
	_, err := r.EC2.ModifyNetworkInterfaceAttribute(ctx,
		&awsec2.ModifyNetworkInterfaceAttributeInput{
			NetworkInterfaceId:       aws.String(eni),
			AssociatePublicIpAddress: aws.Bool(wanted),
		})
	return stepResult{acted: true, operation: "ModifyNetworkInterfaceAttribute",
		err: err}
}

// isElasticIP reports whether the address on the primary interface was
// allocated by someone rather than assigned by EC2.
//
// EC2 owns an auto-assigned address, and says so: the association's owner is
// "amazon". Anything else is an Elastic IP somebody allocated deliberately.
func isElasticIP(instance *ec2types.Instance) bool {
	for _, ni := range instance.NetworkInterfaces {
		if ni.Attachment == nil || aws.ToInt32(ni.Attachment.DeviceIndex) != 0 {
			continue
		}
		if ni.Association == nil {
			return false
		}
		return aws.ToString(ni.Association.IpOwnerId) != "amazon"
	}
	return false
}

// primaryInterface returns the id of an instance's primary network
// interface, or empty if it reports none.
func primaryInterface(instance *ec2types.Instance) string {
	for _, ni := range instance.NetworkInterfaces {
		if ni.Attachment != nil &&
			aws.ToInt32(ni.Attachment.DeviceIndex) == 0 {
			return aws.ToString(ni.NetworkInterfaceId)
		}
	}
	return ""
}
