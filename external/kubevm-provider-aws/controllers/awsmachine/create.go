// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package awsmachine

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/intent"
)

// The tags naming the Kubernetes object an instance belongs to.
//
// Not decoration. An instance carrying no tag naming its owner is an orphan
// waiting to happen: if the write recording it fails and the object is later
// deleted, nothing in the account says what the instance was for or who may
// remove it.
const (
	tagManagedBy = "kube-vm.io/managed-by"
	tagNamespace = "kube-vm.io/namespace"
	tagName      = "kube-vm.io/name"

	// managedByValue names this provider, not merely Kubernetes: an account
	// may hold instances from several controllers.
	managedByValue = "kubevm-aws"
)

// reconcileInstance creates the machine if it does not exist, then observes
// and powers it.
func (r *Reconciler) reconcileInstance(ctx context.Context,
	machine *awsv1a1.AWSMachine, want *intent.Intent) (ctrl.Result, error) {

	if machine.Status.ProviderID == "" {
		return r.createInstance(ctx, machine, want)
	}
	return r.observeAndPower(ctx, machine, want)
}

// createInstance launches exactly one EC2 instance.
//
// Safe to call twice. ClientToken carries the object's UID, so a repeated call
// returns the first reservation rather than creating a second machine --
// which is the only thing standing between a failed status write and a
// duplicate. Observed in production: two instances three seconds apart, the
// second never recorded anywhere and outliving every object that knew of it.
//
// A tag lookup cannot substitute for this. EC2's tag index is eventually
// consistent, so an instance seconds old is routinely absent from it, and the
// fallback "search before creating" reports nothing exists when something
// does.
func (r *Reconciler) createInstance(ctx context.Context,
	machine *awsv1a1.AWSMachine, want *intent.Intent) (ctrl.Result, error) {

	if machine.Status.LaunchRequested != nil {
		// A launch was asked for and never recorded, so its reply may have
		// been lost after EC2 acted. Find what it made before asking again:
		// the token stops a second instance, but a retry whose parameters
		// have changed since is refused rather than answered, and only a
		// lookup by the token can then find the first.
		found, err := r.findByToken(ctx, machine)
		if err != nil {
			return r.reportAWSFailure(ctx, machine, "DescribeInstances", err)
		}
		if found != nil {
			return r.settle(ctx, machine, found, want, recordOnly)
		}
	} else if err := r.setLaunchRequested(ctx, machine, true); err != nil {
		return ctrl.Result{}, err
	}

	// Every launch parameter is read from THIS OBJECT'S SPEC, never from the
	// parent. persistResolved has already written the parent's values here,
	// so for an ordinary machine the two are identical -- but only one of them
	// is what `kubectl get -o yaml` shows. Reading the other would let the
	// object describe a machine that is not the one running.
	input := &awsec2.RunInstancesInput{
		ImageId:      aws.String(machine.Spec.ImageID),
		InstanceType: ec2types.InstanceType(machine.Spec.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		ClientToken:  aws.String(string(machine.UID)),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeInstance,
			Tags:         ownerTags(machine),
		}},
	}

	// Subnet and the public-address preference go on a network interface
	// rather than at the top level. Not a style choice:
	// AssociatePublicIpAddress is settable only there, and EC2 refuses a
	// request that mixes a top-level SubnetId with a NetworkInterfaces list.
	if iface := networkInterface(machine); iface != nil {
		input.NetworkInterfaces = []ec2types.
			InstanceNetworkInterfaceSpecification{*iface}
	}

	out, err := r.EC2.RunInstances(ctx, input)
	if err != nil {
		// Refused outright, EC2 created nothing, so there is nothing to find
		// later. Forgetting the attempt also frees the spec to follow the
		// portable object again before the next try.
		if ec2.CreatedNothing(err) {
			if cerr := r.setLaunchRequested(ctx, machine, false); cerr != nil {
				return ctrl.Result{}, cerr
			}
		}
		return r.reportAWSFailure(ctx, machine, "RunInstances", err)
	}
	if len(out.Instances) == 0 {
		return ctrl.Result{}, fmt.Errorf("launching %s/%s: RunInstances "+
			"returned no instance and no error",
			machine.Namespace, machine.Name)
	}

	// Record identity immediately, on the same pass that created it. Every
	// reconcile after this one depends on it, and the window between the
	// launch succeeding and this write landing is exactly where the
	// duplicate came from.
	return r.settle(ctx, machine, &out.Instances[0], want, recordOnly)
}

// findByToken returns the instance this machine's ClientToken launched, or
// nil if EC2 shows none.
func (r *Reconciler) findByToken(ctx context.Context,
	machine *awsv1a1.AWSMachine) (*ec2types.Instance, error) {

	out, err := r.EC2.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			Name:   aws.String("client-token"),
			Values: []string{string(machine.UID)},
		}},
	})
	if err != nil {
		return nil, err
	}
	for _, res := range out.Reservations {
		if len(res.Instances) > 0 {
			return &res.Instances[0], nil
		}
	}
	return nil, nil
}

// setLaunchRequested records, or forgets, that a launch has been asked for.
func (r *Reconciler) setLaunchRequested(ctx context.Context,
	machine *awsv1a1.AWSMachine, requested bool) error {

	// Recorded before RunInstances is called, so a reply lost after EC2
	// acted still leaves a trace: the next pass and the delete path then
	// look for the instance by its token instead of assuming there is none.
	base := machine.DeepCopy()
	machine.Status.LaunchRequested = nil
	if requested {
		t := metav1.NewTime(r.now())
		machine.Status.LaunchRequested = &t
	}
	return r.patchStatusIfChanged(ctx, machine, base)
}

// networkInterface builds the interface specification, or nil when the
// portable object expressed no preference at all.
//
// Returning nil matters: omitting the whole structure lets EC2 select a
// subnet from the account's default VPC, which is its documented behaviour and
// what a caller who said nothing should get.
func networkInterface(machine *awsv1a1.AWSMachine) *ec2types.
	InstanceNetworkInterfaceSpecification {

	stated := machine.Spec.PublicIP != nil
	if machine.Spec.Subnet == "" && !stated {
		return nil
	}

	spec := &ec2types.InstanceNetworkInterfaceSpecification{
		DeviceIndex: aws.Int32(0),
	}
	if machine.Spec.Subnet != "" {
		spec.SubnetId = aws.String(machine.Spec.Subnet)
	}
	if stated {
		public := *machine.Spec.PublicIP
		// Overrides the subnet's own MapPublicIpOnLaunch, which is how a
		// caller keeps a machine off the public internet even in a default
		// VPC where every subnet auto-assigns one.
		spec.AssociatePublicIpAddress = aws.Bool(public)
	}
	return spec
}

// ownerTags name the Kubernetes object an instance belongs to.
//
// managedBy carries the provider's own name, so an instance can be traced
// back to this controller and not merely to some Kubernetes cluster.
func ownerTags(machine *awsv1a1.AWSMachine) []ec2types.Tag {
	return []ec2types.Tag{
		{Key: aws.String(tagManagedBy), Value: aws.String(managedByValue)},
		{Key: aws.String(tagNamespace),
			Value: aws.String(machine.Namespace)},
		{Key: aws.String(tagName), Value: aws.String(machine.Name)},
	}
}

// reportAWSFailure records an EC2 error as a condition and decides whether the
// call is worth retrying.
//
// A retryable failure is returned as an error rather than a fixed requeue.
// controller-runtime forgets an item's backoff whenever a reconcile asks for
// RequeueAfter, so a flat delay would retry a throttled or capacity-starved
// launch every few seconds forever, in lockstep across every machine. Handing
// the error back puts the workqueue's exponential backoff in charge, which is
// what it is for.
func (r *Reconciler) reportAWSFailure(ctx context.Context,
	machine *awsv1a1.AWSMachine, operation string, err error) (
	ctrl.Result, error) {

	log.FromContext(ctx).Error(err, "EC2 call failed",
		"operation", operation, "class", ec2.Classify(err),
		"code", ec2.Code(err))

	c, retry := failureCondition(operation, err)
	if perr := r.setCondition(ctx, machine, c); perr != nil {
		return ctrl.Result{}, perr
	}
	if retry {
		return ctrl.Result{}, fmt.Errorf("%s: %w", operation, err)
	}
	return ctrl.Result{}, nil
}
