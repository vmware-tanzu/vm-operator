// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package intent reads what a portable VirtualMachine is asking for.
//
// Every value is read fresh on each reconcile rather than cached at creation,
// because the portable object is where a user expresses a change and a cached
// copy would stop noticing.
//
// The package deliberately does two things that look like one. It resolves the
// fields this provider consumes, and it reports the fields it does NOT.
// Silently ignoring a field somebody set is the failure this exists to
// prevent: a user who asks for something and gets no machine-readable signal
// that it was dropped has been misled, and will find out from the bill or the
// incident rather than from the object.
package intent

import (
	"errors"
	"fmt"
	"regexp"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"
)

// Intent is what a portable VirtualMachine asks for, in EC2's terms.
type Intent struct {
	// ImageID is the AMI, read verbatim from the image reference's name.
	ImageID string

	// InstanceType is the EC2 instance type, read verbatim.
	InstanceType string

	// Subnet is where to place the instance, or empty to let EC2 choose
	// from the account's default VPC.
	Subnet string

	// PublicIP is set only when the portable object expressed a preference.
	// Nil means "say nothing", which leaves the subnet's own
	// MapPublicIpOnLaunch setting in charge.
	PublicIP *bool

	// PowerState is the desired power state.
	PowerState kubevmv1a1.PowerState

	// Unsupported lists everything asked for that this provider cannot do,
	// each phrased so a user can tell what was dropped and why.
	Unsupported []string
}

// ErrNoInstanceType says the portable object named no size.
//
// Terminal rather than a wait: EC2 would silently substitute an x86_64 type of
// unknown identity, so launching anyway would produce a machine at a size
// nobody chose and nobody can read off the object.
var ErrNoInstanceType = fmt.Errorf(
	"no instance type named, and EC2 would silently substitute one")

// ErrNoImage says the portable object named no image.
var ErrNoImage = fmt.Errorf("no boot image named, and EC2 requires one")

// ErrNotAnAWSID says a portable name cannot be the AWS id this provider reads
// it as.
var ErrNotAnAWSID = errors.New("not an AWS id")

// Read extracts everything this provider needs from a portable VirtualMachine.
//
// It returns an error for the things that make a launch impossible: a missing
// instance type or image, and a name that cannot be an AWS id. Everything else
// it cannot honour is reported through Unsupported, so a machine is never
// abandoned over one unappliable field.
//
// Shape is checked HERE and not left to the CRD's pattern. The portable
// object's image and network are object references whose names are ordinary
// Kubernetes names, so "ubuntu-2204" is the natural first thing a user writes.
// Caught here it becomes a condition naming the field; left to the schema it
// becomes a rejected write on a controller's own patch, with nowhere to put
// the reason and nothing on the object to read.
func Read(vm *kubevmv1a1.VirtualMachine) (*Intent, error) {
	in := readAll(vm)
	if in.InstanceType == "" {
		return nil, ErrNoInstanceType
	}
	if in.ImageID == "" {
		return nil, ErrNoImage
	}
	if !amiID.MatchString(in.ImageID) {
		return nil, fmt.Errorf("%w: spec.bootDisk.source.image.name is %q, "+
			"and this provider reads that name as an AMI id, which looks "+
			"like ami-0123456789abcdef0", ErrNotAnAWSID, in.ImageID)
	}
	if in.Subnet != "" && !subnetID.MatchString(in.Subnet) {
		return nil, fmt.Errorf("%w: spec.network.interfaces[0].network.name "+
			"is %q, and this provider reads that name as a subnet id, which "+
			"looks like subnet-0123456789abcdef0", ErrNotAnAWSID, in.Subnet)
	}
	return in, nil
}

// The shapes AWS gives these ids, and the same patterns the CRD enforces on
// the fields they are written into.
var (
	amiID    = regexp.MustCompile(`^ami-[0-9a-f]{8,17}$`)
	subnetID = regexp.MustCompile(`^subnet-[0-9a-f]{8,17}$`)
)

// ReadLaunched reads the intent for a machine that is already launched,
// reporting a missing instance type or image instead of refusing.
func ReadLaunched(vm *kubevmv1a1.VirtualMachine) *Intent {
	// The instance keeps what it was launched with either way, so a field
	// that disappears afterwards is one more thing not applied -- not a
	// reason to stop reconciling power and status.
	in := readAll(vm)
	if in.InstanceType == "" {
		in.note("no instance type is named any more; the instance keeps " +
			"the one it was launched with")
	}
	if in.ImageID == "" {
		in.note("no boot image is named any more; the instance keeps the " +
			"image it booted from")
	}
	return in
}

// readAll reads every field it can, leaving a missing one empty.
func readAll(vm *kubevmv1a1.VirtualMachine) *Intent {
	in := &Intent{PowerState: vm.Spec.PowerState}
	_ = in.readInstanceType(vm)
	_ = in.readImage(vm)
	in.readNetwork(vm)
	in.reportUnsupported(vm)
	return in
}

// readInstanceType takes the size, verbatim, from the portable name.
func (i *Intent) readInstanceType(vm *kubevmv1a1.VirtualMachine) error {
	it := vm.Spec.InstanceType
	if it == nil || it.Name == "" {
		// Inline Resources is a legitimate way to ask and EC2 cannot honour
		// it -- there is no custom sizing, only a fixed catalogue. Reported
		// rather than approximated, because rounding somebody's 6-vCPU
		// request up to the nearest type is a decision this provider has no
		// business making silently.
		return ErrNoInstanceType
	}
	i.InstanceType = it.Name
	return nil
}

// readImage takes the AMI id, verbatim, from the image reference's name.
func (i *Intent) readImage(vm *kubevmv1a1.VirtualMachine) error {
	bd := vm.Spec.BootDisk
	if bd == nil || bd.Source.Image == nil || bd.Source.Image.Name == "" {
		return ErrNoImage
	}
	// apiGroup and kind are required by the schema and resolve to nothing:
	// no such resource exists. The name IS the AMI id.
	i.ImageID = bd.Source.Image.Name
	return nil
}

// readNetwork takes placement and the public-address preference from the
// first interface, when the portable object names one.
func (i *Intent) readNetwork(vm *kubevmv1a1.VirtualMachine) {
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return
	}
	first := vm.Spec.Network.Interfaces[0]
	if first.Network != nil {
		i.Subnet = first.Network.Name
	}
	if first.PublicIP != nil {
		v := *first.PublicIP
		i.PublicIP = &v
	}
}

// WantsPublicIP reports the public-address preference and whether one was
// expressed at all.
//
// The second return exists because "not stated" and "stated false" produce
// different requests: the first leaves the subnet's own setting in charge, and
// the second overrides it.
func (i *Intent) WantsPublicIP() (bool, bool) {
	if i.PublicIP == nil {
		return false, false
	}
	return *i.PublicIP, true
}
