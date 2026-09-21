// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package ec2

import (
	"errors"
	"fmt"

	"github.com/aws/smithy-go"
)

// Class is what a caller should do about an error.
type Class int

const (
	// Terminal means retrying cannot help: a typo, an image that does not
	// exist, a deleted subnet. Report it and stop.
	Terminal Class = iota

	// Retryable means the condition may clear on its own, such as a zone
	// temporarily out of capacity.
	Retryable

	// Denied means a permission is missing. A person has to act, so it is
	// reported with the platform's own action name rather than retried.
	Denied

	// Gone means the instance is not there. On delete that is success; on
	// observe it is terminal. The caller decides, because only the caller
	// knows which it was doing.
	Gone
)

// String names the class for a message.
func (c Class) String() string {
	switch c {
	case Terminal:
		return "Terminal"
	case Retryable:
		return "Retryable"
	case Denied:
		return "Denied"
	case Gone:
		return "Gone"
	}
	return "Unknown"
}

// classification maps an AWS error code to what to do about it.
//
// Keyed on the code alone. Where the same code means different things for
// different calls -- today only not-found, which is success on delete and
// terminal on observe -- the caller decides, through IsGone.
var classification = map[string]Class{
	"UnauthorizedOperation":        Denied,
	"AuthFailure":                  Denied,
	"InvalidAMIID.NotFound":        Terminal,
	"InvalidAMIID.Malformed":       Terminal,
	"InvalidAMIID.Unavailable":     Terminal,
	"InvalidParameterValue":        Terminal,
	"InvalidParameterCombination":  Terminal,
	"InvalidSubnetID.NotFound":     Terminal,
	"InvalidGroup.NotFound":        Terminal,
	"VPCIdNotSpecified":            Terminal,
	"InvalidInstanceID.NotFound":   Gone,
	"InvalidInstanceID.Malformed":  Terminal,
	"InsufficientInstanceCapacity": Retryable,
	"RequestLimitExceeded":         Retryable,
	"Unavailable":                  Retryable,
	"InternalError":                Retryable,

	// A launch retried with the same ClientToken but different parameters:
	// the first launch exists and is not yet visible. Retrying finds it.
	"IdempotentParameterMismatch": Retryable,
}

// refusedBeforeActing lists the codes EC2 returns when it rejected a request
// before doing anything: a malformed or unknown parameter, a permission
// failure, no capacity, throttling. Every one of them is a refusal EC2 decided
// on before it created a thing.
//
// An allow-list rather than a deny-list, and that direction is the whole
// point. The caller uses it to decide whether it may forget that a launch was
// attempted, and forgetting a launch that did happen leaves an instance
// running that nothing in the cluster knows about. So a code this map has
// never heard of -- including one AWS adds next year -- counts as "EC2 may
// have acted", which costs one extra lookup by ClientToken on the next pass.
// Classify guesses in the cheap direction for the same reason.
var refusedBeforeActing = map[string]bool{
	"UnauthorizedOperation":        true,
	"AuthFailure":                  true,
	"InvalidAMIID.NotFound":        true,
	"InvalidAMIID.Malformed":       true,
	"InvalidAMIID.Unavailable":     true,
	"InvalidParameterValue":        true,
	"InvalidParameterCombination":  true,
	"InvalidSubnetID.NotFound":     true,
	"InvalidGroup.NotFound":        true,
	"VPCIdNotSpecified":            true,
	"InvalidInstanceID.Malformed":  true,
	"InsufficientInstanceCapacity": true,
	"RequestLimitExceeded":         true,
}

// CreatedNothing reports whether EC2 refused a call before it created
// anything, so the caller may forget the attempt.
func CreatedNothing(err error) bool {
	return refusedBeforeActing[Code(err)]
}

// Classify reports what to do about an error from EC2.
//
// An unrecognised code is Retryable, deliberately. A wrong Terminal strands a
// machine forever with no way back; a wrong Retryable costs a requeue. When
// the cost of the two mistakes is that lopsided, guess in the cheap direction.
func Classify(err error) Class {
	if err == nil {
		return Terminal // a caller asking about nil has a bug
	}

	var api smithy.APIError
	if !errors.As(err, &api) {
		// Not an API error at all -- a timeout, a DNS failure, a closed
		// connection. All transient by nature.
		return Retryable
	}

	if c, ok := classification[api.ErrorCode()]; ok {
		return c
	}
	return Retryable
}

// Code returns the platform's own error code, or "" if there is not one.
//
// Surfaced so a condition message can name it. An operator reading
// "UnauthorizedOperation" knows which IAM action to add; one reading
// "the operation failed" does not.
func Code(err error) string {
	var api smithy.APIError
	if errors.As(err, &api) {
		return api.ErrorCode()
	}
	return ""
}

// IsGone reports whether an error means the instance no longer exists.
//
// Its own helper because the two call sites read it oppositely: deletion
// treats it as success, observation as terminal.
func IsGone(err error) bool {
	return Code(err) == "InvalidInstanceID.NotFound"
}

// Describe renders an error for a condition message, naming the operation and
// the platform's code.
//
// The caller must still pass the result through internal/redact before it
// reaches a condition, an event or a log.
func Describe(operation string, err error) string {
	if code := Code(err); code != "" {
		return fmt.Sprintf("%s failed: %s: %v", operation, code, err)
	}
	return fmt.Sprintf("%s failed: %v", operation, err)
}
