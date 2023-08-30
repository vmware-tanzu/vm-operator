// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	vimTypes "github.com/vmware/govmomi/vim25/types"
)

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
