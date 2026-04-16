/*
Copyright 2019 The Kubernetes Authors.
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

package fuzztests

import (
	"math/rand"
	"time"

	//nolint:depguard
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/randfill"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
)

// GetFuzzer returns a new fuzzer to be used for testing.
func GetFuzzer(scheme *runtime.Scheme, funcs ...fuzzer.FuzzerFuncs) *randfill.Filler {
	funcs = append([]fuzzer.FuzzerFuncs{
		metafuzzer.Funcs,
		func(_ runtimeserializer.CodecFactory) []interface{} {
			return []interface{}{
				// Custom fuzzer for metav1.Time pointers which weren't
				// fuzzed and always resulted in `nil` values.
				// This implementation is somewhat similar to the one provided
				// in the metafuzzer.Funcs.
				func(input **metav1.Time, c randfill.Continue) {
					if c.Bool() {
						// Leave the Time sometimes nil to also get coverage for this case.
						return
					}
					now := metav1.NewTime(time.Now().Add(time.Duration(-c.Intn(3600)) * time.Second)).Rfc3339Copy()
					*input = &now
				},
				// Custom fuzzer for intstr.IntOrString which does not get fuzzed otherwise.
				func(in **intstr.IntOrString, c randfill.Continue) {
					if c.Bool() {
						// Leave the IntOrString sometimes nil to also get coverage for this case.
						return
					}
					if c.Bool() {
						// Set the IntOrString sometimes empty to also get coverage for this case.
						*in = &intstr.IntOrString{}
						return
					}
					v := intstr.FromInt32(c.Int31n(50))
					*in = &v
				},
			}
		},
	}, funcs...)
	return fuzzer.FuzzerFor(
		fuzzer.MergeFuzzerFuncs(funcs...),
		rand.NewSource(rand.Int63()), //nolint:gosec
		runtimeserializer.NewCodecFactory(scheme),
	)
}

// FuzzTestFuncInput contains input parameters
// for the FuzzTestFunc function.
type FuzzTestFuncInput struct {
	Scheme *runtime.Scheme

	Hub              ctrlconversion.Hub
	HubAfterMutation func(ctrlconversion.Hub)

	Spoke                      ctrlconversion.Convertible
	SpokeAfterMutation         func(convertible ctrlconversion.Convertible)
	SkipSpokeAnnotationCleanup bool

	FuzzerFuncs []fuzzer.FuzzerFuncs
}

func SpokeHubSpoke(input FuzzTestFuncInput) {
	fuzzer := GetFuzzer(input.Scheme, input.FuzzerFuncs...)

	for i := 0; i < 10000; i++ {
		// Create the spoke and fuzz it
		spokeBefore := input.Spoke.DeepCopyObject().(ctrlconversion.Convertible)
		fuzzer.Fill(spokeBefore)

		// First convert spoke to hub
		hubCopy := input.Hub.DeepCopyObject().(ctrlconversion.Hub)
		gomega.ExpectWithOffset(1, spokeBefore.ConvertTo(hubCopy)).To(gomega.Succeed())

		// Convert hub back to spoke and check if the resulting spoke is equal to the spoke before the round trip
		spokeAfter := input.Spoke.DeepCopyObject().(ctrlconversion.Convertible)
		gomega.ExpectWithOffset(1, spokeAfter.ConvertFrom(hubCopy)).To(gomega.Succeed())

		// Remove data annotation eventually added by ConvertFrom for avoiding data loss in hub-spoke-hub round trips
		// NOTE: There are use case when we want to skip this operation, e.g. if the spoke object does not have ObjectMeta (e.g. kubeadm types).
		if !input.SkipSpokeAnnotationCleanup {
			metaAfter := spokeAfter.(metav1.Object)
			delete(metaAfter.GetAnnotations(), utilconversion.AnnotationKey)
		}

		if input.SpokeAfterMutation != nil {
			input.SpokeAfterMutation(spokeAfter)
		}

		if !apiequality.Semantic.DeepEqual(spokeBefore, spokeAfter) {
			diff := cmp.Diff(spokeBefore, spokeAfter)
			gomega.Expect(false).To(gomega.BeTrue(), diff)
		}
	}
}

func HubSpokeHub(input FuzzTestFuncInput) {
	fuzzer := GetFuzzer(input.Scheme, input.FuzzerFuncs...)

	for i := 0; i < 10000; i++ {
		// Create the hub and fuzz it
		hubBefore := input.Hub.DeepCopyObject().(ctrlconversion.Hub)
		fuzzer.Fill(hubBefore)

		// First convert hub to spoke
		dstCopy := input.Spoke.DeepCopyObject().(ctrlconversion.Convertible)
		gomega.ExpectWithOffset(1, dstCopy.ConvertFrom(hubBefore)).To(gomega.Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		hubAfter := input.Hub.DeepCopyObject().(ctrlconversion.Hub)
		gomega.ExpectWithOffset(1, dstCopy.ConvertTo(hubAfter)).To(gomega.Succeed())

		if input.HubAfterMutation != nil {
			input.HubAfterMutation(hubAfter)
		}

		if !apiequality.Semantic.DeepEqual(hubBefore, hubAfter) {
			diff := cmp.Diff(hubBefore, hubAfter)
			gomega.Expect(false).To(gomega.BeTrue(), diff)
		}
	}
}
