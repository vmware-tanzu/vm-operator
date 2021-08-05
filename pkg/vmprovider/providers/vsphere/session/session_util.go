// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"math"
	"strings"

	"github.com/vmware/govmomi/find"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func MemoryQuantityToMb(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func CPUQuantityToMhz(q resource.Quantity, cpuFreqMhz uint64) int64 {
	return int64(math.Ceil(float64(q.MilliValue()) * float64(cpuFreqMhz) / float64(1000)))
}

// Transform Govmomi error to Kubernetes error
// TODO: Fill out with VIM fault types.
func transformError(resourceType string, resource string, err error) error {
	switch err.(type) {
	case *find.NotFoundError, *find.DefaultNotFoundError:
		return k8serrors.NewNotFound(schema.GroupResource{Group: "vmoperator.vmware.com", Resource: strings.ToLower(resourceType)}, resource)
	case *find.MultipleFoundError, *find.DefaultMultipleFoundError:
		// Transform?
		return err
	default:
		return err
	}
}

func transformVMError(resource string, err error) error {
	return transformError("VirtualMachine", resource, err)
}

func ExtraConfigToMap(input []vimTypes.BaseOptionValue) (output map[string]string) {
	output = make(map[string]string)
	for _, opt := range input {
		if optValue := opt.GetOptionValue(); optValue != nil {
			// Only set string type values
			if val, ok := optValue.Value.(string); ok {
				output[optValue.Key] = val
			}
		}
	}
	return
}

// MergeExtraConfig adds the key/value to the ExtraConfig if the key is not present, to let to the value be
// changed by the VM. The existing usage of ExtraConfig is hard to fit in the reconciliation model.
func MergeExtraConfig(extraConfig []vimTypes.BaseOptionValue, newMap map[string]string) []vimTypes.BaseOptionValue {
	merged := make([]vimTypes.BaseOptionValue, 0)
	ecMap := ExtraConfigToMap(extraConfig)
	for k, v := range newMap {
		if _, exists := ecMap[k]; !exists {
			merged = append(merged, &vimTypes.OptionValue{Key: k, Value: v})
		}
	}
	return merged
}

// GetMergedvAppConfigSpec prepares a vApp VmConfigSpec which will set the vmMetadata supplied key/value fields. Only
// fields marked userConfigurable and pre-existing on the VM (ie. originated from the OVF Image)
// will be set, and all others will be ignored.
func GetMergedvAppConfigSpec(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo) *vimTypes.VmConfigSpec {
	outProps := make([]vimTypes.VAppPropertySpec, 0)

	for _, vmProp := range vmProps {
		if vmProp.UserConfigurable == nil || !*vmProp.UserConfigurable {
			continue
		}

		inPropValue, found := inProps[vmProp.Id]
		if !found || vmProp.Value == inPropValue {
			continue
		}

		vmPropCopy := vmProp
		vmPropCopy.Value = inPropValue
		outProp := vimTypes.VAppPropertySpec{
			ArrayUpdateSpec: vimTypes.ArrayUpdateSpec{
				Operation: vimTypes.ArrayUpdateOperationEdit,
			},
			Info: &vmPropCopy,
		}
		outProps = append(outProps, outProp)
	}

	if len(outProps) == 0 {
		return nil
	}

	return &vimTypes.VmConfigSpec{Property: outProps}
}

func EncodeGzipBase64(s string) (string, error) {
	var zbuf bytes.Buffer
	zw := gzip.NewWriter(&zbuf)
	if _, err := zw.Write([]byte(s)); err != nil {
		return "", err
	}
	if err := zw.Flush(); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	b64 := base64.StdEncoding.EncodeToString(zbuf.Bytes())
	return b64, nil
}
