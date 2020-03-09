// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package testutil

/*
import (
	"context"

	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/mocks"
)

// Contains helper methods to set up mocks, useful for various unit tests.

// MockClientGetOnce is a method to set up a mock client to Get() a resoure and return a list of values.
// Return a pointer to a gomock.Call, so it can be further modified if needed, or used in
// an InOrder() block.
func MockClientGetOnce(mockClient *mocks.MockClient, key types.NamespacedName, expectedType interface{}, retVals ...interface{}) *gomock.Call {
	return mockClient.EXPECT().
		Get(gomock.AssignableToTypeOf(context.Background()),
			gomock.Eq(key),
			gomock.AssignableToTypeOf(expectedType)).
		MaxTimes(1).
		MinTimes(1).
		Return(retVals...)
}

// MockClientListOnce is a helper method to set up a mock client to List() a resource and return a list of values.
// Return a pointer to a gomock.Call, so it can be further modified if needed, or used in
// an InOrder() block.
func MockClientListOnce(mockClient *mocks.MockClient, listNamespace string, expectedType interface{}, retVals ...interface{}) *gomock.Call {
	if listNamespace == "" {
		return mockClient.EXPECT().
			List(gomock.AssignableToTypeOf(context.Background()),
				gomock.AssignableToTypeOf(expectedType)).
			MaxTimes(1).
			MinTimes(1).
			Return(retVals...)
	}
	return mockClient.EXPECT().
		List(gomock.AssignableToTypeOf(context.Background()),
			gomock.AssignableToTypeOf(expectedType),
			gomock.Eq([]client.ListOption{client.InNamespace(listNamespace)})).
		MaxTimes(1).
		MinTimes(1).
		Return(retVals...)
}

// Similar to MockClientListOnce, but expects listing resources in all namespaces.
func MockClientListOnceAllNamespaces(mockClient *mocks.MockClient, expectedType interface{}, retVals ...interface{}) *gomock.Call {
	return mockClient.EXPECT().
		List(gomock.AssignableToTypeOf(context.Background()),
			gomock.AssignableToTypeOf(expectedType)).
		MaxTimes(1).
		MinTimes(1).
		Return(retVals...)
}
*/
