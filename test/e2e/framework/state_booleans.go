// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package framework

import "strings"

// formal is test's requirement
// actual is the actual of system running the test

// StateIs is a general concept of comparing an actual parameter with formal parameters.
func StateIs(sysActual string, testFormals ...string) bool {
	for _, testFormal := range testFormals {
		if strings.EqualFold(sysActual, testFormal) {
			return true
		}
	}

	return false
}

func InfraIs(configInfra string, testReqInfra ...string) bool {
	return StateIs(configInfra, testReqInfra...)
}

func CNIIs(configCNI string, testReqCNI ...string) bool {
	return StateIs(configCNI, testReqCNI...)
}

func NetworkTopologyIs(configNetworking string, testReqNetworking ...string) bool {
	return StateIs(configNetworking, testReqNetworking...)
}
