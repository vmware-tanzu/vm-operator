// +build !integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package record

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/record"
	controllererror "sigs.k8s.io/cluster-api/pkg/controller/error"
)

var _ = Describe("Event utils", func() {
	Context("Publish event", func() {
		It("should not publish an event", func() {
			var err error
			EmitEvent(nil, "Create", &err, true)
			fakeRecorder := defaultRecorder.(*record.FakeRecorder)
			Expect(len(fakeRecorder.Events)).Should(Equal(0))
		})

		It("should publish a success event", func() {
			var err error
			EmitEvent(nil, "Create", &err, false)
			fakeRecorder := defaultRecorder.(*record.FakeRecorder)
			Expect(len(fakeRecorder.Events)).Should(Equal(1))
			event := <-fakeRecorder.Events
			Expect(event).Should(Equal("Normal SuccessfulCreate Create success"))
		})

		It("should publish a failure event", func() {
			err := errors.New("something wrong")
			EmitEvent(nil, "Create", &err, false)
			fakeRecorder := defaultRecorder.(*record.FakeRecorder)
			Expect(len(fakeRecorder.Events)).Should(Equal(1))
			event := <-fakeRecorder.Events
			Expect(event).Should(Equal("Warning FailedCreate something wrong"))
		})

		It("should not publish an event when it is a reque after error", func() {
			var err error = &controllererror.RequeueAfterError{RequeueAfter: time.Minute}
			EmitEvent(nil, "Create", &err, false)
			fakeRecorder := defaultRecorder.(*record.FakeRecorder)
			Expect(len(fakeRecorder.Events)).Should(Equal(0))
		})

		It("getKey", func() {
			regString := "xxx " + Failure + "CreateVM xxx xxx xxx"
			key := getKey(regString)
			Expect(key).Should(Equal(Failure + "CreateVM"))
		})
	})
})
