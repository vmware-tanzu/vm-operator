/*
Copyright 2020-2024 The Kubernetes Authors.

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

//nolint:scopelint
package conditions

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	nil1          *metav1.Condition
	true1         = TrueCondition("true1")
	unknown1      = UnknownCondition("unknown1", "reason unknown1", "message unknown1")
	falseInfo1    = FalseCondition("falseInfo1", "reason falseInfo1", "message falseInfo1")
	falseWarning1 = FalseCondition("falseWarning1", "reason falseWarning1", "message falseWarning1")
	falseError1   = FalseCondition("falseError1", "reason falseError1", "message falseError1")
)

func TestGetAndHas(t *testing.T) {
	g := NewWithT(t)

	var obj nonKubeObj

	g.Expect(Has(&obj, "conditionBaz")).To(BeFalse())
	g.Expect(Get(&obj, "conditionBaz")).To(BeNil())

	obj.SetConditions(conditionList(TrueCondition("conditionBaz")))

	g.Expect(Has(&obj, "conditionBaz")).To(BeTrue())
	g.Expect(Get(&obj, "conditionBaz")).To(haveSameStateOf(TrueCondition("conditionBaz")))
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

	// test GetMessage
	g.Expect(GetLastTransitionTime(obj, "nil1")).To(BeNil())
	g.Expect(GetLastTransitionTime(obj, "falseInfo1")).ToNot(BeNil())
}

func TestIsReady(t *testing.T) {
	ready := func(
		status metav1.ConditionStatus,
		observedGeneration int64) metav1.Condition {

		return metav1.Condition{
			Type:               ReadyConditionType,
			Status:             status,
			Reason:             "reason",
			ObservedGeneration: observedGeneration,
		}
	}

	testCases := []struct {
		name    string
		in      Conditions
		options []ReadyOption
		out     bool
	}{
		{
			name: "no conditions",
			in:   nil,
			out:  false,
		},
		{
			name: "no Ready condition",
			in:   Conditions{*true1},
			out:  false,
		},
		{
			name: "Ready is False",
			in:   Conditions{ready(metav1.ConditionFalse, 1)},
			out:  false,
		},
		{
			name: "Ready is Unknown",
			in:   Conditions{ready(metav1.ConditionUnknown, 1)},
			out:  false,
		},
		{
			name: "Ready is True",
			in:   Conditions{ready(metav1.ConditionTrue, 0)},
			out:  true,
		},
		{
			name: "Ready is True with other conditions",
			in:   Conditions{*falseInfo1, ready(metav1.ConditionTrue, 0), *unknown1},
			out:  true,
		},
		{
			name:    "without a generation option the observed generation is ignored",
			in:      Conditions{ready(metav1.ConditionTrue, 1)},
			options: nil,
			out:     true,
		},
		{
			name:    "generation: no conditions",
			in:      nil,
			options: []ReadyOption{WithGeneration(1)},
			out:     false,
		},
		{
			name:    "generation: Ready is False for the generation",
			in:      Conditions{ready(metav1.ConditionFalse, 2)},
			options: []ReadyOption{WithGeneration(2)},
			out:     false,
		},
		{
			name:    "generation: Ready is Unknown for the generation",
			in:      Conditions{ready(metav1.ConditionUnknown, 2)},
			options: []ReadyOption{WithGeneration(2)},
			out:     false,
		},
		{
			name:    "generation: Ready is True for an older generation",
			in:      Conditions{ready(metav1.ConditionTrue, 1)},
			options: []ReadyOption{WithGeneration(2)},
			out:     false,
		},
		{
			name:    "generation: Ready is True with an observed generation of zero",
			in:      Conditions{ready(metav1.ConditionTrue, 0)},
			options: []ReadyOption{WithGeneration(1)},
			out:     false,
		},
		{
			name:    "generation: Ready is True for the generation",
			in:      Conditions{ready(metav1.ConditionTrue, 2)},
			options: []ReadyOption{WithGeneration(2)},
			out:     true,
		},
		{
			name:    "generation: Ready is True for a newer generation",
			in:      Conditions{ready(metav1.ConditionTrue, 3)},
			options: []ReadyOption{WithGeneration(2)},
			out:     true,
		},
		{
			name:    "generation: zero is always satisfied",
			in:      Conditions{ready(metav1.ConditionTrue, 0)},
			options: []ReadyOption{WithGeneration(0)},
			out:     true,
		},
		{
			name: "generation: the last option wins",
			in:   Conditions{ready(metav1.ConditionTrue, 2)},
			options: []ReadyOption{
				WithGeneration(5),
				WithGeneration(2),
			},
			out: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			// Exercise both the function and the method.
			g.Expect(IsReady(tc.in, tc.options...)).To(Equal(tc.out))
			g.Expect(tc.in.IsReady(tc.options...)).To(Equal(tc.out))
		})
	}
}

func TestGetObservedAt(t *testing.T) {
	cond := func(
		conditionType string,
		observedGeneration int64) metav1.Condition {

		return metav1.Condition{
			Type:               conditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "reason",
			ObservedGeneration: observedGeneration,
		}
	}

	testCases := []struct {
		name       string
		in         Conditions
		generation int64
		out        Conditions
	}{
		{
			name:       "no conditions",
			in:         nil,
			generation: 1,
			out:        Conditions{},
		},
		{
			name:       "all conditions describe the generation",
			in:         Conditions{cond("a", 2), cond("b", 2)},
			generation: 2,
			out:        Conditions{cond("a", 2), cond("b", 2)},
		},
		{
			name:       "no condition describes the generation",
			in:         Conditions{cond("a", 1), cond("b", 1)},
			generation: 2,
			out:        Conditions{},
		},
		{
			name:       "each condition is judged on its own",
			in:         Conditions{cond("a", 1), cond("b", 2), cond("c", 1)},
			generation: 2,
			out:        Conditions{cond("b", 2)},
		},
		{
			name:       "a newer observed generation describes the generation",
			in:         Conditions{cond("a", 3), cond("b", 1)},
			generation: 2,
			out:        Conditions{cond("a", 3)},
		},
		{
			name:       "no observed generation is behind a non-zero generation",
			in:         Conditions{cond("a", 0), cond("b", 1)},
			generation: 1,
			out:        Conditions{cond("b", 1)},
		},
		{
			name:       "a zero generation is always satisfied",
			in:         Conditions{cond("a", 0), cond("b", 1)},
			generation: 0,
			out:        Conditions{cond("a", 0), cond("b", 1)},
		},
		{
			name:       "the order is kept",
			in:         Conditions{cond("c", 2), cond("a", 2), cond("b", 2)},
			generation: 2,
			out:        Conditions{cond("c", 2), cond("a", 2), cond("b", 2)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			// Exercise both the function and the method.
			g.Expect(GetObservedAt(tc.in, tc.generation)).To(Equal(tc.out))
			g.Expect(tc.in.GetObservedAt(tc.generation)).To(Equal(tc.out))
		})
	}

	t.Run("the result does not alias the input", func(t *testing.T) {
		g := NewWithT(t)

		in := Conditions{cond("a", 2)}
		out := GetObservedAt(in, 2)
		out[0].Reason = "changed"

		g.Expect(in[0].Reason).To(Equal("reason"))
	})
}

func TestMirror(t *testing.T) {
	foo := FalseCondition("foo", "reason foo", "message foo")
	ready := TrueCondition(ReadyConditionType)
	readyBar := ready.DeepCopy()
	readyBar.Type = "bar"

	tests := []struct {
		name string
		from Getter
		t    string
		want *metav1.Condition
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
	bar := FalseCondition("bar", "reason falseInfo1", "message falseInfo1")
	baz := FalseCondition("baz", "reason falseInfo2", "message falseInfo2")
	existingReady := FalseCondition(ReadyConditionType, "reason falseError1", "message falseError1") // NB. existing ready has higher priority than other conditions

	tests := []struct {
		name    string
		from    Getter
		options []MergeOption
		want    *metav1.Condition
	}{
		{
			name: "Returns nil when there are no conditions to summarize",
			from: getterWithConditions(),
			want: nil,
		},
		{
			name: "Returns ready condition with the summary of existing conditions (with default options)",
			from: getterWithConditions(foo, bar),
			want: FalseCondition(ReadyConditionType, "reason falseInfo1", "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounter options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounter()},
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounterIf(false)},
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounterIf(true)},
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithStepCounter(), WithStepCounterIfOnly("bar")},
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "0 of 1 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf and WithStepCounterIfOnly options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounter(), WithStepCounterIfOnly("foo")},
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithConditions("foo")}, // bar should be ignored
			want:    TrueCondition(ReadyConditionType),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounter options)",
			from:    getterWithConditions(foo, bar, baz),
			options: []MergeOption{WithConditions("foo", "bar"), WithStepCounter()}, // baz should be ignored, total steps should be 2
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithConditions("bar", "baz"), WithStepCounter(), WithStepCounterIfOnly("bar")}, // there is only bar, the step counter should be set and counts only a subset of conditions
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "0 of 1 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options - with inconsistent order between the two)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithConditions("baz", "bar"), WithStepCounter(), WithStepCounterIfOnly("bar", "baz")}, // conditions in WithStepCounterIfOnly could be in different order than in WithConditions
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "0 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar, baz),
			options: []MergeOption{WithConditions("bar", "baz"), WithStepCounter(), WithStepCounterIfOnly("bar")}, // there is also baz, so the step counter should not be set
			want:    FalseCondition(ReadyConditionType, "reason falseInfo1", "message falseInfo1"),
		},
		{
			name:    "Ready condition respects merge order",
			from:    getterWithConditions(bar, baz),
			options: []MergeOption{WithConditions("baz", "bar")}, // baz should take precedence on bar
			want:    FalseCondition(ReadyConditionType, "reason falseInfo2", "message falseInfo2"),
		},
		{
			name: "Ignores existing Ready condition when computing the summary",
			from: getterWithConditions(existingReady, foo, bar),
			want: FalseCondition(ReadyConditionType, "reason falseInfo1", "message falseInfo1"),
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
	ready1 := TrueCondition(ReadyConditionType)
	ready2 := FalseCondition(ReadyConditionType, "reason falseInfo1", "message falseInfo1")
	bar := FalseCondition("bar", "reason falseError1", "message falseError1") // NB. bar has higher priority than other conditions

	tests := []struct {
		name string
		from []Getter
		t    string
		want *metav1.Condition
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
			want: FalseCondition("foo", "reason falseInfo1", "2 of 5 completed"),
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

func getterWithConditions(conditions ...*metav1.Condition) Getter {
	var obj nonKubeObj
	obj.SetConditions(conditionList(conditions...))
	return obj
}

func conditionList(conditions ...*metav1.Condition) []metav1.Condition {
	var cs []metav1.Condition
	for _, x := range conditions {
		if x != nil {
			cs = append(cs, *x)
		}
	}
	return cs
}

func haveSameStateOf(expected *metav1.Condition) types.GomegaMatcher {
	return &ConditionMatcher{
		Expected: expected,
	}
}

type ConditionMatcher struct {
	Expected *metav1.Condition
}

func (matcher *ConditionMatcher) Match(actual interface{}) (success bool, err error) {
	actualCondition, ok := actual.(*metav1.Condition)
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
