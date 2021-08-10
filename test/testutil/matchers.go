// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package testutil

// Contains custom matcher definitions to use with Gomega.
// ref: https://onsi.github.io/gomega/#adding-your-own-matchers

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/onsi/gomega/types"
)

func WrapError(expected interface{}) types.GomegaMatcher {
	return &wrapsErrorMatcher{
		expected: expected,
	}
}

type wrapsErrorMatcher struct {
	expected interface{}
}

// Match verifies that the actual error wraps the expected one.
func (matcher *wrapsErrorMatcher) Match(actual interface{}) (success bool, err error) {
	var expectedErrorString string
	// Allow the user to pass in either an error or a string.
	// Fail early if neither was passed in.
	switch val := matcher.expected.(type) {
	case string:
		expectedErrorString = val
	case error:
		expectedErrorString = val.Error()
	default:
		return false, fmt.Errorf("WrapError expects an error to match against. Got: %+v", reflect.TypeOf(matcher.expected))
	}
	actualErr, ok := actual.(error)
	if !ok {
		return false, fmt.Errorf("wrapsErrorMatcher expects an error. Got: %+v", reflect.TypeOf(actual))
	}
	// errors.Is() does not seem to work for some reason, possibly a Go versioning issue?
	// In that case, a simple strings.Contains seems like a reasonable substitution.
	return strings.Contains(actualErr.Error(), expectedErrorString), nil
}

func (matcher *wrapsErrorMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected\n\t%+v\n\n to wrap\n\t%+v", actual, matcher.expected)
}

func (matcher *wrapsErrorMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected\n\t%+v not to wrap\n\t%+v", actual, matcher.expected)
}
