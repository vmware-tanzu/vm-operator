/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package conditions2

import (
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Setter interface defines methods that a Cluster API object should implement in order to
// use the conditions package for setting conditions.
type Setter interface {
	Getter
	SetConditions([]metav1.Condition)
}

// Set sets the given condition.
//
// NOTE: If a condition already exists, the LastTransitionTime is updated only if a change is detected
// in any of the following fields: Status, Reason, Severity and Message.
func Set(to Setter, condition *metav1.Condition) {
	if to == nil || condition == nil {
		return
	}

	// Check if the new conditions already exists, and change it only if there is a status
	// transition (otherwise we should preserve the current last transition time)-
	conditions := to.GetConditions()
	exists := false
	for i := range conditions {
		existingCondition := conditions[i]
		if existingCondition.Type == condition.Type {
			exists = true
			if !hasSameState(&existingCondition, condition) {
				condition.LastTransitionTime = metav1.NewTime(time.Now().UTC().Truncate(time.Second))
				conditions[i] = *condition
				break
			}
			condition.LastTransitionTime = existingCondition.LastTransitionTime
			break
		}
	}

	// If the condition does not exist, add it, setting the transition time only if not already set
	if !exists {
		if condition.LastTransitionTime.IsZero() {
			condition.LastTransitionTime = metav1.NewTime(time.Now().UTC().Truncate(time.Second))
		}
		conditions = append(conditions, *condition)
	}

	// Sorts conditions for convenience of the consumer, i.e. kubectl.
	sort.Slice(conditions, func(i, j int) bool {
		return lexicographicLess(&conditions[i], &conditions[j])
	})

	to.SetConditions(conditions)
}

// TrueCondition returns a condition with Status=True and the given type.
func TrueCondition(t string) *metav1.Condition {
	return &metav1.Condition{
		Type:   t,
		Reason: t, // BMV: This is required field in metav1.Conditions. Fixup API later.
		Status: metav1.ConditionTrue,
	}
}

// FalseCondition returns a condition with Status=False and the given type.
func FalseCondition(t string, reason string, messageFormat string, messageArgs ...interface{}) *metav1.Condition {
	return &metav1.Condition{
		Type:    t,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: fmt.Sprintf(messageFormat, messageArgs...),
	}
}

// UnknownCondition returns a condition with Status=Unknown and the given type.
func UnknownCondition(t string, reason string, messageFormat string, messageArgs ...interface{}) *metav1.Condition {
	return &metav1.Condition{
		Type:    t,
		Status:  metav1.ConditionUnknown,
		Reason:  reason,
		Message: fmt.Sprintf(messageFormat, messageArgs...),
	}
}

// MarkTrue sets Status=True for the condition with the given type.
func MarkTrue(to Setter, t string) {
	Set(to, TrueCondition(t))
}

// MarkUnknown sets Status=Unknown for the condition with the given type.
func MarkUnknown(to Setter, t string, reason, messageFormat string, messageArgs ...interface{}) {
	Set(to, UnknownCondition(t, reason, messageFormat, messageArgs...))
}

// MarkFalse sets Status=False for the condition with the given type.
func MarkFalse(to Setter, t string, reason string, messageFormat string, messageArgs ...interface{}) {
	Set(to, FalseCondition(t, reason, messageFormat, messageArgs...))
}

// SetSummary sets a Ready condition with the summary of all the conditions existing
// on an object. If the object does not have other conditions, no summary condition is generated.
func SetSummary(to Setter, options ...MergeOption) {
	Set(to, summary(to, options...))
}

// SetMirror creates a new condition by mirroring the the Ready condition from a dependent object;
// if the Ready condition does not exists in the source object, no target conditions is generated.
func SetMirror(to Setter, targetCondition string, from Getter, options ...MirrorOptions) {
	Set(to, mirror(from, targetCondition, options...))
}

// SetAggregate creates a new condition with the aggregation of all the the Ready condition
// from a list of dependent objects; if the Ready condition does not exists in one of the source object,
// the object is excluded from the aggregation; if none of the source object have ready condition,
// no target conditions is generated.
func SetAggregate(to Setter, targetCondition string, from []Getter, options ...MergeOption) {
	Set(to, aggregate(from, targetCondition, options...))
}

// Delete deletes the condition with the given type.
func Delete(to Setter, t string) {
	if to == nil {
		return
	}

	conditions := to.GetConditions()
	newConditions := make([]metav1.Condition, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Type != t {
			newConditions = append(newConditions, condition)
		}
	}
	to.SetConditions(newConditions)
}

// lexicographicLess returns true if a condition is less than another with regards to the
// to order of conditions designed for convenience of the consumer, i.e. kubectl.
// According to this order the Ready condition always goes first, followed by all the other
// conditions sorted by Type.
func lexicographicLess(i, j *metav1.Condition) bool {
	return (i.Type == ReadyConditionType || i.Type < j.Type) && j.Type != ReadyConditionType
}

// hasSameState returns true if a condition has the same state of another; state is defined
// by the union of following fields: Type, Status, Reason, Severity and Message (it excludes LastTransitionTime).
func hasSameState(i, j *metav1.Condition) bool {
	return i.Type == j.Type &&
		i.Status == j.Status &&
		i.Reason == j.Reason &&
		i.Message == j.Message
}
