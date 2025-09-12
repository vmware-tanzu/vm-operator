// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"k8s.io/apimachinery/pkg/util/sets"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	unexpectedTypeErrMsg = "expected value's datatype string (got %T)"
)

var (
	extraConfigDenyKeyPrefixes = [...]string{
		"guestinfo.",
		"vmservice.",
	}
	extraConfigAllowKeys = [...]string{
		"vmservice.virtualmachine.pvc.disk.data",
		"vmservice.vmi.labels",
	}
)

// GetExtraConfigFromObject assumes VirtualMachine object is created properly
// with a vim25 client and a managed reference.
func GetExtraConfigFromObject(
	ctx context.Context,
	vm *object.VirtualMachine) (pkgutil.OptionValues, error) {
	var o mo.VirtualMachine
	if err := vm.Properties(
		ctx,
		vm.Reference(),
		[]string{"config.extraConfig"},
		&o); err != nil {
		return nil, err
	}

	return o.Config.ExtraConfig, nil
}

func GetFilteredExtraConfigFromObject(
	ctx context.Context,
	vm *object.VirtualMachine,
	reset bool) (pkgutil.OptionValues, error) {
	var extraConfig pkgutil.OptionValues
	var err error
	if extraConfig, err = GetExtraConfigFromObject(ctx, vm); err != nil {
		return nil, err
	}

	return FilteredExtraConfig(extraConfig, reset)
}

// FilteredExtraConfig removes or resets those key entries which have prefix
// that is in the deny key prefix list, but not in the allow key list.
// It returns a new filtered OptionValues object if the operation is successful.
func FilteredExtraConfig(
	in pkgutil.OptionValues,
	reset bool) (pkgutil.OptionValues, error) {
	allowKeys := sets.New(extraConfigAllowKeys[:]...)
	filtered := in.Map()

	for k, v := range filtered {
		lowerK := strings.ToLower(k)
		if allowKeys.Has(lowerK) {
			continue
		}
		for _, denyKeyPrefix := range extraConfigDenyKeyPrefixes {
			if strings.HasPrefix(lowerK, denyKeyPrefix) {
				if reset {
					if _, ok := v.(string); !ok {
						return nil, fmt.Errorf("failed to filter extraConfig: "+
							unexpectedTypeErrMsg, v)
					}
					filtered[k] = ""
				} else {
					delete(filtered, k)
				}
				break
			}
		}
	}

	return pkgutil.OptionValuesFromMap(filtered), nil
}
