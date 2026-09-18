// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package record

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
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
func New(eventRecorder events.EventRecorder) Recorder {
	return recorder{EventRecorder: eventRecorder}
}

type recorder struct {
	events.EventRecorder
}

// titleCase returns a title-cased string. A new Caser is created on each call
// because cases.Caser is not safe for concurrent use.
func titleCase(s string) string {
	return cases.Title(language.English, cases.NoLower).String(s)
}

func (r recorder) Event(object runtime.Object, reason, message string) {
	reason = titleCase(reason)
	r.EventRecorder.Eventf(object, nil, corev1.EventTypeNormal,
		reason, reason, "%s", message)
}

// Eventf is just like Event, but with Sprintf for the message field.
func (r recorder) Eventf(object runtime.Object, reason, message string, args ...interface{}) {
	reason = titleCase(reason)
	r.EventRecorder.Eventf(object, nil, corev1.EventTypeNormal,
		reason, reason, message, args...)
}

// Warn constructs a warning event from the given information and puts it in the queue for sending.
func (r recorder) Warn(object runtime.Object, reason, message string) {
	reason = titleCase(reason)
	r.EventRecorder.Eventf(object, nil, corev1.EventTypeWarning,
		reason, reason, "%s", message)
}

// Warnf is just like Event, but with Sprintf for the message field.
func (r recorder) Warnf(object runtime.Object, reason, message string, args ...interface{}) {
	r.EventRecorder.Eventf(object, nil, corev1.EventTypeWarning,
		titleCase(reason), titleCase(reason), message, args...)
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
