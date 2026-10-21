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

package patch

// Option is some configuration that modifies options for a patch request.
type Option interface {
	// ApplyToHelper applies this configuration to the given Helper options.
	ApplyToHelper(*HelperOptions)
}

// HelperOptions contains options for patch options.
type HelperOptions struct {
	// StatusOptimisticLock includes the original resourceVersion in the status
	// patch so concurrent writes cause a conflict instead of a lost update.
	StatusOptimisticLock bool

	// IncludeStatusObservedGeneration sets the status.observedGeneration field
	// on the incoming object to match metadata.generation, only if there is a change.
	IncludeStatusObservedGeneration bool

	// ForceOverwriteConditions allows the patch helper to overwrite conditions in case of conflicts.
	// This option should only ever be set in controller managing the object being patched.
	ForceOverwriteConditions bool

	// OwnedConditions defines condition types owned by the controller.
	// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
	OwnedConditions []string
}

// WithForceOverwriteConditions allows the patch helper to overwrite conditions in case of conflicts.
// This option should only ever be set in controller managing the object being patched.
type WithForceOverwriteConditions struct{}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithForceOverwriteConditions) ApplyToHelper(in *HelperOptions) {
	in.ForceOverwriteConditions = true
}

// WithStatusObservedGeneration sets the status.observedGeneration field
// on the incoming object to match metadata.generation, only if there is a change.
type WithStatusObservedGeneration struct{}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithStatusObservedGeneration) ApplyToHelper(in *HelperOptions) {
	in.IncludeStatusObservedGeneration = true
}

// WithOwnedConditions allows to define condition types owned by the controller.
// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
type WithOwnedConditions struct {
	Conditions []string
}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithOwnedConditions) ApplyToHelper(in *HelperOptions) {
	in.OwnedConditions = w.Conditions
}

// WithStatusOptimisticLock rejects status patches based on a stale resourceVersion.
// Use it at every writer of a shared status list. Conflicts require a fresh
// read and reconcile; they must not be retried with the same computed list.
// The precondition covers the whole object, including spec and metadata writes.
type WithStatusOptimisticLock struct{}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithStatusOptimisticLock) ApplyToHelper(in *HelperOptions) {
	in.StatusOptimisticLock = true
}
