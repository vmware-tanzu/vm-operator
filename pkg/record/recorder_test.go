// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package record_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apirecord "k8s.io/client-go/tools/record"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

var _ = Describe("Event utils", func() {
	fakeRecorder := apirecord.NewFakeRecorder(100)
	recorder := record.New(fakeRecorder)

	Context("Publish event", func() {
		It("should not publish an event", func() {
			var err error
			recorder.EmitEvent(nil, "Create", err, true)
			Expect(len(fakeRecorder.Events)).Should(Equal(0))
		})

		It("should publish a success event", func() {
			var err error
			recorder.EmitEvent(nil, "Create", err, false)
			Expect(len(fakeRecorder.Events)).Should(Equal(1))
			event := <-fakeRecorder.Events
			Expect(event).Should(Equal("Normal CreateSuccess Create success"))
		})

		It("should publish a failure event", func() {
			err := errors.New("something wrong")
			recorder.EmitEvent(nil, "Create", err, false)
			Expect(len(fakeRecorder.Events)).Should(Equal(1))
			event := <-fakeRecorder.Events
			Expect(event).Should(Equal("Warning CreateFailure something wrong"))
		})
	})
})
