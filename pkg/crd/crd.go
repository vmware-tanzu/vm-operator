// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd

import (
	"context"
	"embed"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/config/crd"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

// UnstructuredBases returns a list of the base CRDs.
func UnstructuredBases() ([]unstructured.Unstructured, error) {
	files, err := crd.Bases.ReadDir("bases")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded crd bases: %w", err)
	}

	// The following command may be used to list the expected CRD files:
	//
	//   /bin/ls ./config/crd/bases | xargs -n1 echo | sed 's~^\(.\{1,\}\)~case "bases/\1":~g'
	crds := make([]unstructured.Unstructured, len(files))
	for i := range files {
		crds[i].Object = map[string]any{}
		if err := decode(
			crd.Bases,
			"bases/"+files[i].Name(),
			&crds[i]); err != nil {

			return nil, err
		}
	}

	return crds, nil
}

// UnstructuredExternal returns a list of the external CRDs.
func UnstructuredExternal() ([]unstructured.Unstructured, error) {
	files, err := crd.External.ReadDir("external-crds")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read embedded crd external bases: %w", err)
	}

	// The following command may be used to list the expected CRD files:
	//
	//   /bin/ls ./config/crd/external-crds | xargs -n1 echo | sed 's~^\(.\{1,\}\)~case "external-crds/\1":~g'
	crds := make([]unstructured.Unstructured, len(files))
	for i := range files {
		crds[i].Object = map[string]any{}
		if err := decode(
			crd.External,
			"external-crds/"+files[i].Name(),
			&crds[i]); err != nil {

			return nil, err
		}
	}

	return crds, nil
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions/status,verbs=get;update;patch

// Install installs the CRDs into the provided Kubernetes environment based on
// the current set of feature/capability flags. This will also remove any APIs
// from the provided environment if their flags have been disabled.
func Install( //nolint:gocyclo
	ctx context.Context,
	k8sClient ctrlclient.Client,
	mutateFn func(kind string, obj *unstructured.Unstructured) error) error {

	baseCRDs, err := UnstructuredBases()
	if err != nil {
		return fmt.Errorf("failed to read embedded crd bases: %w", err)
	}

	externalCRDs, err := UnstructuredExternal()
	if err != nil {
		return fmt.Errorf("failed to read embedded crd external bases: %w", err)
	}

	crds := slices.Concat(baseCRDs, externalCRDs)
	logger := pkglog.FromContextOrDefault(ctx)
	features := pkgcfg.FromContext(ctx).Features

	for i := range crds {
		c := &crds[i]
		k, _, err := unstructured.NestedString(c.Object, "spec", "names", "kind")
		if err != nil {
			return fmt.Errorf("failed to get crd name: %w", err)
		}

		logger.Info("Processing CRD", "kind", k)

		if mutateFn != nil {
			if err := mutateFn(k, c); err != nil {
				return fmt.Errorf("failed to mutate crd %s: %w", k, err)
			}
		}

		// Use the following command from the root of this project to print a
		// full and current list of CRD names:
		//
		//   /bin/ls ./config/crd/bases | xargs -I% -n1 grep '    kind: ' config/crd/bases/% | sed 's~^.\{1,\}: \(.\{1,\}\)~case "\1":~g'
		//
		// The output from the above command may be pasted directly into the
		// following "switch" statement in order to keep it up-to-date.
		switch k {
		case "ComputePolicy",
			"PolicyEvaluation",
			"TagPolicy":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				features.VSpherePolicies,
				c,
				k,
				nil); err != nil {

				return err
			}
		case "EncryptionClass":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				features.BringYourOwnEncryptionKey,
				c,
				k,
				nil); err != nil {

				return err
			}
		// case "ClusterVirtualMachineImage":
		// case "ContentLibraryProvider":
		// case "ContentSourceBinding":
		// case "ContentSource":
		// case "VirtualMachineClassBinding":
		// case "VirtualMachineClass":
		case "VirtualMachineClassInstance":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				features.ImmutableClasses,
				c,
				k,
				nil); err != nil {

				return err
			}
		case "VirtualMachineGroupPublishRequest",
			"VirtualMachineGroup":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				features.VMGroups,
				c,
				k,
				nil); err != nil {

				return err
			}
		case "VirtualMachineImageCache":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				features.FastDeploy,
				c,
				k,
				nil); err != nil {

				return err
			}
		// case "VirtualMachineImage":
		// case "VirtualMachinePublishRequest":
		// case "VirtualMachineReplicaSet":
		case "VirtualMachine":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				true,
				c,
				k,
				func(
					kind string,
					obj *unstructured.Unstructured,
					shouldRemoveFields bool) error {

					if !features.ImmutableClasses {
						if err := removeFields(
							ctx,
							k,
							obj,
							shouldRemoveFields,
							specFieldPath("class")); err != nil {

							return err
						}
					}

					if !features.VMGroups {
						if err := removeFields(
							ctx,
							k,
							obj,
							shouldRemoveFields,
							specFieldPath("bootOptions"),
							specFieldPath("groupName")); err != nil {

							return err
						}
					}

					if !features.VMSnapshots {
						if err := removeFields(
							ctx,
							k,
							obj,
							shouldRemoveFields,
							specFieldPath("currentSnapshotName"),
							statusFieldPath("currentSnapshot"),
							statusFieldPath("rootSnapshots")); err != nil {

							return err
						}
					}

					if !features.VSpherePolicies {
						if err := removeFields(
							ctx,
							k,
							obj,
							shouldRemoveFields,
							specFieldPath("policies"),
							statusFieldPath("policies")); err != nil {

							return err
						}
					}

					if !features.GuestCustomizationVCDParity {
						if err := removeFields(
							ctx,
							k,
							obj,
							shouldRemoveFields,
							specFieldPath("bootstrap", "linuxPrep", "password"),
							specFieldPath("bootstrap", "linuxPrep", "scriptText"),
							specFieldPath("bootstrap", "linuxPrep", "expirePasswordAfterNextLogin"),
							specFieldPath("bootstrap", "sysprep", "sysprep", "scriptText"),
							specFieldPath("bootstrap", "sysprep", "sysprep", "expirePasswordAfterNextLogin"),
						); err != nil {

							return err
						}
					}

					if !features.VMSharedDisks {
						if err := removeFields(
							ctx,
							k,
							obj,
							shouldRemoveFields,
							specFieldPath("volumes", "[]", "persistentVolumeClaim", "applicationType"),
							specFieldPath("volumes", "[]", "persistentVolumeClaim", "controllerBusNumber"),
							specFieldPath("volumes", "[]", "persistentVolumeClaim", "controllerType"),
							specFieldPath("volumes", "[]", "persistentVolumeClaim", "diskMode"),
							specFieldPath("volumes", "[]", "persistentVolumeClaim", "sharingMode"),
							specFieldPath("volumes", "[]", "persistentVolumeClaim", "unitNumber"),

							specFieldPath("hardware", "ideControllers"),
							specFieldPath("hardware", "nvmeControllers"),
							specFieldPath("hardware", "sataControllers"),
							specFieldPath("hardware", "scsiControllers"),
							// Only remove part of RAC related fields, keeping
							// the remaining cdrom configs.
							specFieldPath("hardware", "cdrom", "[]", "controllerBusNumber"),
							specFieldPath("hardware", "cdrom", "[]", "controllerType"),
							specFieldPath("hardware", "cdrom", "[]", "unitNumber"),

							statusFieldPath("volumes", "[]", "controllerBusNumber"),
							statusFieldPath("volumes", "[]", "controllerType"),
							statusFieldPath("volumes", "[]", "diskMode"),
							statusFieldPath("volumes", "[]", "sharingMode"),

							statusFieldPath("hardware", "controllers"),
						); err != nil {

							return err
						}
					}

					return nil
				}); err != nil {

				return err
			}

		// case "VirtualMachineService":
		// case "VirtualMachineSetResourcePolicy":
		case "VirtualMachineSnapshot":
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				features.VMSnapshots,
				c,
				k,
				nil); err != nil {

				return err
			}
		// case "VirtualMachineWebConsoleRequest":
		// case "WebConsoleRequest":
		default:
			if err := updateOrDeleteUnstructured(
				ctx,
				k8sClient,
				true,
				c,
				k,
				nil); err != nil {

				return err
			}
		}
	}

	return nil
}

func removeFields(
	ctx context.Context,
	k string,
	c *unstructured.Unstructured,
	shouldRemoveFields bool,
	fields ...[]string) error {

	logger := pkglog.FromContextOrDefault(ctx)

	for _, f := range fields {
		if !shouldRemoveFields {
			logger.Info(
				"Skipping CRD field removal",
				"kind", k,
				"field", strings.Join(f, "."))
		} else {
			logger.Info(
				"Removing CRD field",
				"kind", k,
				"field", strings.Join(f, "."))

			versions, _, err := unstructured.NestedSlice(
				c.Object, "spec", "versions")
			if err != nil {
				return fmt.Errorf(
					"failed to get crd %s versions: %w", k, err)
			}

			for k := range versions {
				v := versions[k].(map[string]any)
				unstructured.RemoveNestedField(v, f...)
			}

			if err := unstructured.SetNestedSlice(
				c.Object,
				versions,
				"spec",
				"versions"); err != nil {
				return fmt.Errorf(
					"failed to remove fields for crd %q: %w", k, err)
			}
		}
	}
	return nil
}

func updateOrDeleteUnstructured(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	enabled bool,
	c *unstructured.Unstructured,
	k string,
	mutateFn func(
		kind string,
		obj *unstructured.Unstructured,
		shouldRemoveFields bool) error,
) error {

	logger := pkglog.FromContextOrDefault(ctx)

	obj := unstructured.Unstructured{
		Object: map[string]any{},
	}
	obj.SetGroupVersionKind(c.GroupVersionKind())
	obj.SetName(c.GetName())
	obj.SetNamespace(c.GetNamespace())

	exists := true
	if err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKeyFromObject(&obj),
		&obj); err != nil {

		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get crd %q: %w", k, err)
		}

		exists = false
	}

	if !exists {
		//
		// Create the CRD
		//

		if enabled {
			if mutateFn != nil {
				logger.Info("Mutating CRD on create", "kind", k)
				if err := mutateFn(k, c, true); err != nil {
					return fmt.Errorf(
						"failed to mutate crd %q on create: %w", k, err)
				}
			}

			logger.Info("Creating CRD", "kind", k)
			if err := k8sClient.Create(ctx, c); err != nil {
				return fmt.Errorf("failed to create crd %q: %w", k, err)
			}
			logger.V(4).Info("Created CRD", "kind", k)
		} else {
			logger.Info("Skipped creation of CRD", "kind", k)
		}

		return nil
	}

	if !enabled {
		//
		// Delete the CRD
		//

		if pkgcfg.FromContext(ctx).CRDCleanupEnabled {
			logger.Info("Deleting CRD", "kind", k)
			if err := k8sClient.Delete(ctx, c); err != nil {
				if !apierrors.IsNotFound(err) {
					return fmt.Errorf("failed to delete crd %s: %w", k, err)
				}
			} else {
				logger.V(4).Info("Deleted CRD", "kind", k)
			}
		} else {
			logger.Info("Skipped CRD deletion", "kind", k)
		}
		return nil
	}

	//
	// Update the CRD
	//

	// Create the patch to update the object.
	objPatch := ctrlclient.MergeFrom(
		obj.DeepCopyObject().(ctrlclient.Object))

	// Get a possible CustomResourceConversion field value.
	crc, hasCRC, err := unstructured.NestedMap(
		obj.Object,
		"spec", "conversion")
	if err != nil {
		return fmt.Errorf("failed to get crd %q conversion: %w", k, err)
	}

	// Copy the new spec into position.
	spec, _, err := unstructured.NestedMap(c.Object, "spec")
	if err != nil {
		return fmt.Errorf("failed to get crd %q spec: %w", k, err)
	}
	if err := unstructured.SetNestedMap(
		obj.Object,
		spec,
		"spec"); err != nil {
		return fmt.Errorf("failed to set crd %q spec: %w", k, err)
	}

	// Reapply the CustomResourceConversion if one existed.
	if hasCRC {
		if err := unstructured.SetNestedMap(
			obj.Object,
			crc,
			"spec", "conversion"); err != nil {
			return fmt.Errorf("failed to set crd %q conversion: %w", k, err)
		}
	}

	// Do possible mutations.
	if mutateFn != nil {
		logger.Info("Mutating CRD on update", "kind", k)
		if err := mutateFn(
			k,
			&obj,
			pkgcfg.FromContext(ctx).CRDCleanupEnabled); err != nil {

			return fmt.Errorf(
				"failed to mutate crd %q on update: %w", k, err)
		}
	}

	// Patch the CRD with the change.
	if err := k8sClient.Patch(ctx, &obj, objPatch); err != nil {
		return fmt.Errorf("failed to patch crd %q: %w", k, err)
	}

	return nil
}

func decode(fs embed.FS, fileName string, dst ctrlclient.Object) error {
	data, err := fs.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("failed to read embedded crd: %w", err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("failed to decode embedded crd: %w", err)
	}
	return nil
}

func specFieldPath(fieldNames ...string) []string {
	result := []string{"schema", "openAPIV3Schema", "properties", "spec", "properties"}
	result = append(result, fieldNames[0])
	for _, name := range fieldNames[1:] {
		// Use "[]" to indicate previous element is an array type.
		if name == "[]" {
			result = append(result, "items")
			continue
		}
		result = append(result, "properties", name)
	}
	return result
}

func statusFieldPath(fieldNames ...string) []string {
	result := []string{"schema", "openAPIV3Schema", "properties", "status", "properties"}
	result = append(result, fieldNames[0])
	for _, name := range fieldNames[1:] {
		// Use "[]" to indicate previous element is an array type.
		if name == "[]" {
			result = append(result, "items")
			continue
		}
		result = append(result, "properties", name)
	}
	return result
}
