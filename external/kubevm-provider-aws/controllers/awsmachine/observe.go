// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package awsmachine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/intent"
)

// applyMode says whether a pass may change the machine, or only record what
// it already is. The create path records the instance it has just launched
// without acting on it again; every later pass acts.
type applyMode bool

const (
	recordOnly   applyMode = false
	applyChanges applyMode = true
)

// stepResult is what one step did to the instance on this pass.
type stepResult struct {
	acted     bool
	operation string
	err       error
}

// observeAndPower reads the instance, applies its public address and power,
// and records what it found.
func (r *Reconciler) observeAndPower(ctx context.Context,
	machine *awsv1a1.AWSMachine, want *intent.Intent) (ctrl.Result, error) {

	instance, err := r.findInstance(ctx, machine)
	if err != nil {
		return r.reportAWSFailure(ctx, machine, "DescribeInstances", err)
	}
	if instance == nil {
		return r.vanished(ctx, machine)
	}
	return r.settle(ctx, machine, instance, want, applyChanges)
}

// vanished reports an instance EC2 cannot find, after allowing for one too
// new to be visible yet.
func (r *Reconciler) vanished(ctx context.Context,
	machine *awsv1a1.AWSMachine) (ctrl.Result, error) {

	base := machine.DeepCopy()

	// Absence means three things, not two: terminated long ago, never
	// existed, or created moments ago and not yet visible. Only the third is
	// recoverable by waiting, so it is separated out. RunInstances returns
	// before the new id has reached every DescribeInstances endpoint, and the
	// gap answers InvalidInstanceID.NotFound; observed on a live run, where a
	// machine that was running was reported to the cluster as destroyed.
	if r.withinGrace(machine) {
		setCond(machine, notReady(awsv1a1.ReasonProvisioning,
			"waiting for the new instance id to become visible to "+
				"DescribeInstances"))
		return ctrl.Result{RequeueAfter: bootRequeue},
			r.patchStatusIfChanged(ctx, machine, base)
	}

	// Past the window, absence is real, and terminal: EC2 forgets terminated
	// instances after roughly an hour, so absence cannot be distinguished
	// from never-existed, and a machine that has gone away is not one to
	// quietly rebuild under the same name. What was observed of it goes too,
	// so the object does not go on describing a machine that is not there.
	// The providerID stays: it names the machine that was lost.
	machine.Status.PowerState = ""
	machine.Status.Addresses = nil
	machine.Status.ProviderMetadata = nil
	setCond(machine, notReady(awsv1a1.ReasonInstanceGone,
		fmt.Sprintf("instance %s no longer exists",
			instanceIDFrom(machine.Status.ProviderID))))
	return ctrl.Result{}, r.patchStatusIfChanged(ctx, machine, base)
}

// withinGrace reports whether the launch is recent enough that EC2 may not
// show the new instance yet.
func (r *Reconciler) withinGrace(machine *awsv1a1.AWSMachine) bool {
	// Timed from the launch, not the object: an AWSMachine may wait a long
	// time for its VirtualMachine before anything launches.
	since := machine.CreationTimestamp.Time
	if machine.Status.LaunchRequested != nil {
		since = machine.Status.LaunchRequested.Time
	}
	return r.now().Sub(since) < r.grace()
}

// settle records what the instance is, acts on it if asked to, and writes the
// status once.
func (r *Reconciler) settle(ctx context.Context,
	machine *awsv1a1.AWSMachine, instance *ec2types.Instance,
	want *intent.Intent, mode applyMode) (ctrl.Result, error) {

	base := machine.DeepCopy()
	state := ec2types.InstanceStateNamePending
	if instance.State != nil {
		state = instance.State.Name
	}
	id := aws.ToString(instance.InstanceId)
	recordObserved(machine, instance, id, state)

	// Both steps run on every pass that acts, so one that keeps failing
	// cannot hold the other up. A step that called EC2 is read back on the
	// next pass rather than assumed.
	var res ctrl.Result
	var failed stepResult
	if mode == applyChanges {
		for _, a := range []stepResult{
			r.applyPublicIP(ctx, want, instance, state),
			r.applyPower(ctx, id, state, want),
		} {
			if a.acted && a.err == nil {
				res = soonest(res, bootRequeue)
			}
			if a.err != nil && failed.err == nil {
				failed = a
			}
		}
	}

	cond, retry := readiness(state, want.PowerState, failed)
	if retry || ec2.IsTransitional(state) {
		res = soonest(res, bootRequeue)
	}
	setCond(machine, cond)
	setCond(machine, upToDate(want.Unsupported))

	// Written once per pass, and only if something changed. Every status
	// write wakes this controller again, so two steps that each wrote a
	// condition -- one saying ready, the next saying a start failed -- would
	// rewrite it forever, and every round is another set of EC2 calls.
	//
	// The result is dropped when the write fails: controller-runtime ignores
	// a Result that arrives with an error, and returning both only produces
	// a warning nobody can act on.
	if err := r.patchStatusIfChanged(ctx, machine, base); err != nil {
		return ctrl.Result{}, err
	}
	return res, nil
}

// recordObserved writes every contract status path from one instance.
//
// The field names here are not ours to choose. The core reads this object as
// unstructured, by path, knowing no AWS field name -- so a value this provider
// can see but the core cannot is worthless.
func recordObserved(machine *awsv1a1.AWSMachine, instance *ec2types.Instance,
	id string, state ec2types.InstanceStateName) {

	zone := ""
	if instance.Placement != nil {
		zone = aws.ToString(instance.Placement.AvailabilityZone)
	}
	machine.Status.ProviderID = fmt.Sprintf("aws:///%s/%s", zone, id)
	machine.Status.Addresses = addressesOf(instance)
	machine.Status.ProviderMetadata = metadataOf(zone)

	// ABSENT while transitional, not empty-string and not a guess. An absent
	// path is explicitly not an error under the contract; a guessed one
	// produces a UI flickering between values the machine was never in.
	machine.Status.PowerState = ""
	if ps, ok := ec2.PowerStateFor(state); ok {
		machine.Status.PowerState = string(ps)
	}
}

// readiness decides InfrastructureReady from the instance's state, the power
// state asked for, and any step that failed, and says whether to retry soon.
func readiness(state ec2types.InstanceStateName,
	want kubevmv1a1.PowerState, failed stepResult) (metav1.Condition, bool) {

	if failed.err != nil {
		return failureCondition(failed.operation, failed.err)
	}

	// Ready means settled in the state asked for, as on vSphere, where a
	// machine is not ready until its power state is synced.
	running := state == ec2types.InstanceStateNameRunning
	stopped := state == ec2types.InstanceStateNameStopped
	switch {
	case ec2.IsTerminated(state):
		return notReady(awsv1a1.ReasonInstanceGone,
			"the instance has been terminated"), false
	case !running && !stopped:
		return notReady(awsv1a1.ReasonProvisioning,
			fmt.Sprintf("the instance is %s", state)), false
	case running && want == kubevmv1a1.PowerStateOff:
		return notReady(awsv1a1.ReasonPowerChanging,
			"the instance is running; stopping it, as asked"), false
	case stopped && want == kubevmv1a1.PowerStateOn:
		return notReady(awsv1a1.ReasonPowerChanging,
			"the instance is stopped; starting it, as asked"), false
	case running:
		return ready(awsv1a1.ReasonRunning, "the instance is running"), false
	}
	return ready(awsv1a1.ReasonStopped, "the instance is stopped"), false
}

// soonest returns whichever requeue comes first.
func soonest(res ctrl.Result, after time.Duration) ctrl.Result {
	if res.RequeueAfter == 0 || after < res.RequeueAfter {
		return ctrl.Result{RequeueAfter: after}
	}
	return res
}

// findInstance reads the instance this machine created, by id.
//
// By id, deliberately, and never by tag. EC2's tag index is eventually
// consistent, so a tag query can report nothing for an instance that plainly
// exists. Tags are for a human searching the console, not for a controller
// deciding whether to create something.
//
// An id lookup is far better but is NOT strongly consistent, which this
// comment used to claim. A new id takes a moment to reach every
// DescribeInstances endpoint, and until it does the answer is
// InvalidInstanceID.NotFound. The caller handles that window; see above.
func (r *Reconciler) findInstance(ctx context.Context,
	machine *awsv1a1.AWSMachine) (*ec2types.Instance, error) {

	id := instanceIDFrom(machine.Status.ProviderID)
	if id == "" {
		return nil, nil
	}

	out, err := r.EC2.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	})
	if err != nil {
		if ec2.IsGone(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, res := range out.Reservations {
		for i := range res.Instances {
			if aws.ToString(res.Instances[i].InstanceId) == id {
				return &res.Instances[i], nil
			}
		}
	}
	return nil, nil
}

// addressesOf reads the instance's addresses into the contract's shape.
func addressesOf(instance *ec2types.Instance) []awsv1a1.AWSMachineAddress {
	var out []awsv1a1.AWSMachineAddress
	add := func(kind, value string) {
		if value == "" {
			return
		}
		out = append(out, awsv1a1.AWSMachineAddress{
			Type: kind, Address: value,
		})
	}

	add(awsv1a1.AddressInternalIP, aws.ToString(instance.PrivateIpAddress))
	add(awsv1a1.AddressExternalIP, aws.ToString(instance.PublicIpAddress))
	add(awsv1a1.AddressInternalDNS, aws.ToString(instance.PrivateDnsName))
	add(awsv1a1.AddressExternalDNS, aws.ToString(instance.PublicDnsName))
	return out
}

// metadataOf reports the zone EC2 placed the instance in.
//
// This map is the only route a platform fact has to the portable object: the
// core copies it wholesale, knowing no AWS field name, and never reads a value
// back into a decision. Being untyped, it is also the easiest place for an
// accidental API to grow, so a key has to earn its way in twice: the portable
// object must have no other way to say it, and a consumer that knows nothing
// about AWS must be able to act on it.
//
// The zone passes. The portable spec asks for a failureDomain and the portable
// status cannot answer, and "are these two machines in the same zone" is a
// question anyone can ask of any cloud. Nothing else here passes: the image
// and the instance type are already stated by the portable spec, the instance
// id is the second half of providerID, and a subnet or VPC id is an AWS name
// a portable consumer can only print. Those belong on this object, where the
// reader has already accepted AWS -- see the follow-up to persist a subnet
// EC2 chose into this object's own spec.
func metadataOf(zone string) map[string]string {
	m := map[string]string{}
	if zone != "" {
		m["availabilityZone"] = zone
	}
	return m
}

// instanceIDFrom pulls the instance id out of a providerID.
//
// The format is aws:///<availabilityZone>/<instanceID>, so the id is whatever
// follows the last separator.
func instanceIDFrom(providerID string) string {
	if providerID == "" {
		return ""
	}
	idx := strings.LastIndex(providerID, "/")
	if idx < 0 || idx == len(providerID)-1 {
		return ""
	}
	return providerID[idx+1:]
}
