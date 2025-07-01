// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package record

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// Recorder knows how to record events on behalf of a source.
type Recorder interface {
	// EmitEvent records a Success or Failure depending on whether or not an error occurred.
	EmitEvent(object runtime.Object, opName string, err error, ignoreSuccess bool)

	// Event constructs an event from the given information and puts it in the queue for sending.
	Event(object runtime.Object, reason, message string)

	// Eventf is just like Event, but with Sprintf for the message field.
	Eventf(object runtime.Object, reason, message string, args ...interface{})

	// Warn constructs a warning event from the given information and puts it in the queue for sending.
	Warn(object runtime.Object, reason, message string)

	// Warnf is just like Event, but with Sprintf for the message field.
	Warnf(object runtime.Object, reason, message string, args ...interface{})
}

// New returns a new instance of a Recorder.
func New(eventRecorder record.EventRecorder) Recorder {
	return recorder{EventRecorder: eventRecorder}
}

type recorder struct {
	record.EventRecorder
}

// Event constructs an event from the given information and puts it in the queue for sending.
func (r recorder) Event(object runtime.Object, reason, message string) {
	r.EventRecorder.Event(object, corev1.EventTypeNormal,
		cases.Title(language.English, cases.NoLower).String(reason), message)
}

// Eventf is just like Event, but with Sprintf for the message field.
func (r recorder) Eventf(object runtime.Object, reason, message string, args ...interface{}) {
	r.EventRecorder.Eventf(object, corev1.EventTypeNormal,
		cases.Title(language.English, cases.NoLower).String(reason), message, args...)
}

// Warn constructs a warning event from the given information and puts it in the queue for sending.
func (r recorder) Warn(object runtime.Object, reason, message string) {
	r.EventRecorder.Event(object, corev1.EventTypeWarning,
		cases.Title(language.English, cases.NoLower).String(reason), message)
}

// Warnf is just like Event, but with Sprintf for the message field.
func (r recorder) Warnf(object runtime.Object, reason, message string, args ...interface{}) {
	r.EventRecorder.Eventf(object, corev1.EventTypeWarning,
		cases.Title(language.English, cases.NoLower).String(reason), message, args...)
}

// EmitEvent records a Success or Failure depending on whether or not an error occurred.
func (r recorder) EmitEvent(object runtime.Object, opName string, err error, ignoreSuccess bool) {
	if err == nil {
		if !ignoreSuccess {
			r.Event(object, opName+"Success", opName+" success")
		}
	} else {
		r.Warn(object, opName+"Failure", err.Error())
	}
}
