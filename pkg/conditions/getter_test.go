// +build !integration

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

// nolint:scopelint
package conditions

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

var (
	nil1          *vmopv1alpha1.Condition
	true1         = TrueCondition("true1")
	unknown1      = UnknownCondition("unknown1", "reason unknown1", "message unknown1")
	falseInfo1    = FalseCondition("falseInfo1", "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1")
	falseWarning1 = FalseCondition("falseWarning1", "reason falseWarning1", vmopv1alpha1.ConditionSeverityWarning, "message falseWarning1")
	falseError1   = FalseCondition("falseError1", "reason falseError1", vmopv1alpha1.ConditionSeverityError, "message falseError1")
)

func TestGetAndHas(t *testing.T) {
	g := NewWithT(t)

	vm := &vmopv1alpha1.VirtualMachine{}

	g.Expect(Has(vm, "conditionBaz")).To(BeFalse())
	g.Expect(Get(vm, "conditionBaz")).To(BeNil())

	vm.SetConditions(conditionList(TrueCondition("conditionBaz")))

	g.Expect(Has(vm, "conditionBaz")).To(BeTrue())
	g.Expect(Get(vm, "conditionBaz")).To(haveSameStateOf(TrueCondition("conditionBaz")))
}

func TestIsMethods(t *testing.T) {
	g := NewWithT(t)

	obj := getterWithConditions(nil1, true1, unknown1, falseInfo1, falseWarning1, falseError1)

	// test isTrue
	g.Expect(IsTrue(obj, "nil1")).To(BeFalse())
	g.Expect(IsTrue(obj, "true1")).To(BeTrue())
	g.Expect(IsTrue(obj, "falseInfo1")).To(BeFalse())
	g.Expect(IsTrue(obj, "unknown1")).To(BeFalse())

	// test isFalse
	g.Expect(IsFalse(obj, "nil1")).To(BeFalse())
	g.Expect(IsFalse(obj, "true1")).To(BeFalse())
	g.Expect(IsFalse(obj, "falseInfo1")).To(BeTrue())
	g.Expect(IsFalse(obj, "unknown1")).To(BeFalse())

	// test isUnknown
	g.Expect(IsUnknown(obj, "nil1")).To(BeTrue())
	g.Expect(IsUnknown(obj, "true1")).To(BeFalse())
	g.Expect(IsUnknown(obj, "falseInfo1")).To(BeFalse())
	g.Expect(IsUnknown(obj, "unknown1")).To(BeTrue())

	// test GetReason
	g.Expect(GetReason(obj, "nil1")).To(Equal(""))
	g.Expect(GetReason(obj, "falseInfo1")).To(Equal("reason falseInfo1"))

	// test GetMessage
	g.Expect(GetMessage(obj, "nil1")).To(Equal(""))
	g.Expect(GetMessage(obj, "falseInfo1")).To(Equal("message falseInfo1"))

	// test GetSeverity
	g.Expect(GetSeverity(obj, "nil1")).To(BeNil())
	severity := GetSeverity(obj, "falseInfo1")
	expectedSeverity := vmopv1alpha1.ConditionSeverityInfo
	g.Expect(severity).To(Equal(&expectedSeverity))

	// test GetMessage
	g.Expect(GetLastTransitionTime(obj, "nil1")).To(BeNil())
	g.Expect(GetLastTransitionTime(obj, "falseInfo1")).ToNot(BeNil())
}

func TestMirror(t *testing.T) {
	foo := FalseCondition("foo", "reason foo", vmopv1alpha1.ConditionSeverityInfo, "message foo")
	ready := TrueCondition(vmopv1alpha1.ReadyCondition)
	readyBar := ready.DeepCopy()
	readyBar.Type = "bar"

	tests := []struct {
		name string
		from Getter
		t    vmopv1alpha1.ConditionType
		want *vmopv1alpha1.Condition
	}{
		{
			name: "Returns nil when the ready condition does not exists",
			from: getterWithConditions(foo),
			want: nil,
		},
		{
			name: "Returns ready condition from source",
			from: getterWithConditions(ready, foo),
			t:    "bar",
			want: readyBar,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := mirror(tt.from, tt.t)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).To(haveSameStateOf(tt.want))
		})
	}
}

func TestSummary(t *testing.T) {
	foo := TrueCondition("foo")
	bar := FalseCondition("bar", "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1")
	baz := FalseCondition("baz", "reason falseInfo2", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo2")
	existingReady := FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseError1", vmopv1alpha1.ConditionSeverityError, "message falseError1") //NB. existing ready has higher priority than other conditions

	tests := []struct {
		name    string
		from    Getter
		options []MergeOption
		want    *vmopv1alpha1.Condition
	}{
		{
			name: "Returns nil when there are no conditions to summarize",
			from: getterWithConditions(),
			want: nil,
		},
		{
			name: "Returns ready condition with the summary of existing conditions (with default options)",
			from: getterWithConditions(foo, bar),
			want: FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounter options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounter()},
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounterIf(false)},
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounterIf(true)},
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithStepCounter(), WithStepCounterIfOnly("bar")},
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "0 of 1 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf and WithStepCounterIfOnly options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounter(), WithStepCounterIfOnly("foo")},
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithConditions("foo")}, // bar should be ignored
			want:    TrueCondition(vmopv1alpha1.ReadyCondition),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounter options)",
			from:    getterWithConditions(foo, bar, baz),
			options: []MergeOption{WithConditions("foo", "bar"), WithStepCounter()}, // baz should be ignored, total steps should be 2
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithConditions("bar", "baz"), WithStepCounter(), WithStepCounterIfOnly("bar")}, // there is only bar, the step counter should be set and counts only a subset of conditions
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "0 of 1 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options - with inconsistent order between the two)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithConditions("baz", "bar"), WithStepCounter(), WithStepCounterIfOnly("bar", "baz")}, // conditions in WithStepCounterIfOnly could be in different order than in WithConditions
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "0 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar, baz),
			options: []MergeOption{WithConditions("bar", "baz"), WithStepCounter(), WithStepCounterIfOnly("bar")}, // there is also baz, so the step counter should not be set
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Ready condition respects merge order",
			from:    getterWithConditions(bar, baz),
			options: []MergeOption{WithConditions("baz", "bar")}, // baz should take precedence on bar
			want:    FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo2", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo2"),
		},
		{
			name: "Ignores existing Ready condition when computing the summary",
			from: getterWithConditions(existingReady, foo, bar),
			want: FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := summary(tt.from, tt.options...)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).To(haveSameStateOf(tt.want))
		})
	}
}

func TestAggregate(t *testing.T) {
	ready1 := TrueCondition(vmopv1alpha1.ReadyCondition)
	ready2 := FalseCondition(vmopv1alpha1.ReadyCondition, "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "message falseInfo1")
	bar := FalseCondition("bar", "reason falseError1", vmopv1alpha1.ConditionSeverityError, "message falseError1") //NB. bar has higher priority than other conditions

	tests := []struct {
		name string
		from []Getter
		t    vmopv1alpha1.ConditionType
		want *vmopv1alpha1.Condition
	}{
		{
			name: "Returns nil when there are no conditions to aggregate",
			from: []Getter{},
			want: nil,
		},
		{
			name: "Returns foo condition with the aggregation of object's ready conditions",
			from: []Getter{
				getterWithConditions(ready1),
				getterWithConditions(ready1),
				getterWithConditions(ready2, bar),
				getterWithConditions(),
				getterWithConditions(bar),
			},
			t:    "foo",
			want: FalseCondition("foo", "reason falseInfo1", vmopv1alpha1.ConditionSeverityInfo, "2 of 5 completed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := aggregate(tt.from, tt.t)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).To(haveSameStateOf(tt.want))
		})
	}
}

func getterWithConditions(conditions ...*vmopv1alpha1.Condition) Getter {
	obj := &vmopv1alpha1.VirtualMachine{}
	obj.SetConditions(conditionList(conditions...))
	return obj
}

func conditionList(conditions ...*vmopv1alpha1.Condition) vmopv1alpha1.Conditions {
	cs := vmopv1alpha1.Conditions{}
	for _, x := range conditions {
		if x != nil {
			cs = append(cs, *x)
		}
	}
	return cs
}

func haveSameStateOf(expected *vmopv1alpha1.Condition) types.GomegaMatcher {
	return &ConditionMatcher{
		Expected: expected,
	}
}

type ConditionMatcher struct {
	Expected *vmopv1alpha1.Condition
}

func (matcher *ConditionMatcher) Match(actual interface{}) (success bool, err error) {
	actualCondition, ok := actual.(*vmopv1alpha1.Condition)
	if !ok {
		return false, errors.New("Value should be a condition")
	}

	return hasSameState(actualCondition, matcher.Expected), nil
}

func (matcher *ConditionMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to have the same state of", matcher.Expected)
}
func (matcher *ConditionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to have the same state of", matcher.Expected)
}
