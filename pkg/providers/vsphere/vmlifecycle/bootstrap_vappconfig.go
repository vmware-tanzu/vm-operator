// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

func BootstrapVAppConfig(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	bsArgs *BootstrapArgs) (*vimtypes.VirtualMachineConfigSpec, *vimtypes.CustomizationSpec, error) {

	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.V(4).Info("Reconciling vApp bootstrap state")

	var (
		err        error
		configSpec vimtypes.VirtualMachineConfigSpec
	)

	configSpec.VAppConfig, err = GetOVFVAppConfigForConfigSpec(
		config,
		vAppConfigSpec,
		bsArgs.BootstrapData.VAppData,
		bsArgs.BootstrapData.VAppExData,
		bsArgs.TemplateRenderFn)

	return &configSpec, nil, err
}

func GetOVFVAppConfigForConfigSpec(
	config *vimtypes.VirtualMachineConfigInfo,
	vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec,
	vAppData map[string]string,
	vAppExData map[string]map[string]string,
	templateRenderFn TemplateRenderFunc) (vimtypes.BaseVmConfigSpec, error) {

	var vAppConfigInfo *vimtypes.VmConfigInfo
	if config.VAppConfig != nil {
		vAppConfigInfo = config.VAppConfig.GetVmConfigInfo()
	}

	if vAppConfigInfo == nil {
		return nil, errors.New("vAppConfig is not yet available")
	}

	if len(vAppConfigSpec.Properties) > 0 {
		vAppData = map[string]string{}

		for _, p := range vAppConfigSpec.Properties {
			if p.Value.Value != nil {
				vAppData[p.Key] = *p.Value.Value
			} else if p.Value.From != nil {
				from := p.Value.From
				vAppData[p.Key] = vAppExData[from.Name][from.Key]
			}
		}
	}

	if templateRenderFn != nil {
		// If we have a templating func, apply it to whatever data we have, regardless of the source.
		for k, v := range vAppData {
			vAppData[k] = templateRenderFn(k, v)
		}
	}

	return GetMergedvAppConfigSpec(vAppData, vAppConfigInfo.Property)
}

const maxVAppPropStringLen = 65535

var (
	strWithMinLenRx      = regexp.MustCompile(`^(?:string|password)\(\.\.(\d+)\)$`)
	strWithMaxLenRx      = regexp.MustCompile(`^(?:string|password)\((\d+)\.\.\)$`)
	strWithMinMaxLenRx   = regexp.MustCompile(`^(?:string|password)\((\d+)\.\.(\d+)\)$`)
	intWithMinMaxSizeRx  = regexp.MustCompile(`^int\(([+-]?\d+)\.\.([+-]?\d+)\)$`)
	realWithMinMaxSizeRx = regexp.MustCompile(`^real\(([+-]?(?:\d+(?:\.\d*)?|\.\d+))\.\.([+-]?(?:\d+(?:\.\d*)?|\.\d+))\)$`)
)

// GetMergedvAppConfigSpec prepares a vApp VmConfigSpec which will set the
// provided key/value fields. Only fields marked userConfigurable and
// pre-existing on the VM (ie. originated from the OVF Image) will be set, and
// all others will be ignored.
//
//nolint:gocyclo
func GetMergedvAppConfigSpec(
	keyVals map[string]string,
	inProps []vimtypes.VAppPropertyInfo) (vimtypes.BaseVmConfigSpec, error) {

	var outProps []vimtypes.VAppPropertySpec //nolint:prealloc

	for i := range inProps {
		p := inProps[i]

		if p.UserConfigurable == nil || !*p.UserConfigurable {
			continue
		}

		val, found := keyVals[p.Id]
		if !found || p.Value == val {
			continue
		}

		switch p.Type {
		case "string", "password":
			//
			// A generic string. Max length 65535 (64k).
			//
			if l := len(val); l > maxVAppPropStringLen {
				return nil, newParseLenErr(p, l, 0, maxVAppPropStringLen)
			}
			p.Value = val

		case "boolean":
			//
			// A boolean. The value can be "True" or "False".
			//
			if ok, _ := strconv.ParseBool(val); ok {
				p.Value = "True"
			} else {
				p.Value = "False"
			}

		case "int":
			//
			// An integer value. Is semantically equivalent to
			// int(-2147483648..2147483647) e.g. signed int32.
			//
			if _, err := strconv.ParseInt(val, 10, 32); err != nil {
				return nil, newParseErr(p, val)
			}
			p.Value = val

		case "real":
			//
			// An IEEE 8-byte floating-point value, i.e. a float64.
			//
			if _, err := strconv.ParseFloat(val, 64); err != nil {
				return nil, newParseErr(p, val)
			}
			p.Value = val

		case "ip":
			//
			// An IPv4 address in dot-decimal notation or an IPv6 address in
			// colon-hexadecimal notation.
			//
			if v, _, err := pkgutil.ParseIP(val); v == nil || err != nil {
				return nil, newParseErr(p, val)
			}
			p.Value = val

		case "ip:network":
			//
			// An IP address in dot-notation (IPv4) and colon-hexadecimal (IPv6)
			// on a particular network. The behavior of this type depends on the
			// ipAllocationPolicy.
			//

			// TODO(akutz) Figure out the correct parsing strategy.
			p.Value = val

		case "expression":
			//
			// The default value specifies an expression that is calculated
			// by the system.
			//

			// TODO(akutz) Figure out the correct parsing strategy.
			p.Value = val

		default:
			if m := strWithMinLenRx.FindStringSubmatch(p.Type); len(m) > 0 {
				//
				// A string with minimum character length x.
				//
				minLen, _ := strconv.Atoi(m[1])
				if l := len(val); l < minLen {
					return nil, newParseLenErr(p, l, minLen, maxVAppPropStringLen)
				}
				p.Value = val

			} else if m := strWithMaxLenRx.FindStringSubmatch(p.Type); len(m) > 0 {
				//
				// A string with maximum character length x.
				//
				maxLen, _ := strconv.Atoi(m[1])
				if l := len(val); l > maxLen {
					return nil, newParseLenErr(p, l, 0, maxLen)
				}
				p.Value = val

			} else if m := strWithMinMaxLenRx.FindStringSubmatch(p.Type); len(m) > 0 {
				//
				// A string with minimum character length x and maximum
				// character length y.
				//
				minLen, _ := strconv.Atoi(m[1])
				maxLen, _ := strconv.Atoi(m[2])

				if minLen > maxLen {
					return nil, newParseMinMaxErr(p, int64(minLen), int64(maxLen))
				}

				if l := len(val); l < minLen || l > maxLen {
					return nil, newParseLenErr(p, l, minLen, maxLen)
				}
				p.Value = val

			} else if m := intWithMinMaxSizeRx.FindStringSubmatch(p.Type); len(m) > 0 {
				//
				// An integer value with a minimum size x and a maximum size y.
				// For example int(0..255) is a number between 0 and 255 both
				// included. This is also a way to specify that the number must
				// be a uint8. There is always a lower and lower bound. Max
				// number of digits is 100 including any sign. If exported to
				// OVF the value will be truncated to max of uint64 or int64.
				//
				minSize, _ := strconv.ParseInt(m[1], 10, 64)
				maxSize, _ := strconv.ParseInt(m[2], 10, 64)

				if minSize > maxSize {
					return nil, newParseMinMaxErr(p, minSize, maxSize)
				}

				if minSize >= 0 {
					v, err := strconv.ParseUint(val, 10, 64)
					if err != nil {
						return nil, newParseErr(p, val)
					}
					umin, umax := uint64(minSize), uint64(maxSize) //nolint:gosec
					if v < umin || v > umax {
						return nil, newParseUintSizeErr(p, v, umin, umax)
					}
				} else {
					v, err := strconv.ParseInt(val, 10, 64)
					if err != nil {
						return nil, newParseErr(p, val)
					}
					if v < minSize || v > maxSize {
						return nil, newParseIntSizeErr(p, v, minSize, maxSize)
					}
				}

				p.Value = val

			} else if m := realWithMinMaxSizeRx.FindStringSubmatch(p.Type); len(m) > 0 {
				//
				// An IEEE 8-byte floating-point value with a minimum size x and
				// a maximum size y. For example real(-1.5..1.5) must be a
				// number between -1.5 and 1.5. Because of the nature of float
				// some conversions can truncate the value. Real must be encoded
				// according to CIM.
				//
				minSize, _ := strconv.ParseFloat(m[1], 64)
				maxSize, _ := strconv.ParseFloat(m[2], 64)

				if minSize > maxSize {
					return nil, newParseRealMinMaxErr(p, minSize, maxSize)
				}

				v, err := strconv.ParseFloat(val, 64)
				if err != nil {
					return nil, newParseErr(p, val)
				}
				if v < minSize || v > maxSize {
					return nil, newParseRealSizeErr(p, v, minSize, maxSize)
				}

				p.Value = val

			} else {
				p.Value = val
			}
		}

		outProp := vimtypes.VAppPropertySpec{
			ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{
				Operation: vimtypes.ArrayUpdateOperationEdit,
			},
			Info: &p,
		}
		outProps = append(outProps, outProp)
	}

	if len(outProps) == 0 {
		return nil, nil
	}

	return &vimtypes.VmConfigSpec{
		Property: outProps,
		// Ensure the transport is guestInfo in case the VM does not have
		// a CD-ROM device required to use the ISO transport.
		OvfEnvironmentTransport: []string{OvfEnvironmentTransportGuestInfo},
	}, nil
}

func newParseErr(p vimtypes.VAppPropertyInfo, val string) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s, value=%v",
		p.Id,
		p.Type,
		val)
}

func newParseLenErr(p vimtypes.VAppPropertyInfo, actLen, minLen, maxLen int) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s due to length: "+
			"len=%d, min=%d, max=%d",
		p.Id,
		p.Type,
		actLen,
		minLen,
		maxLen)
}

func newParseIntSizeErr(p vimtypes.VAppPropertyInfo, val, minSize, maxSize int64) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s due to size: "+
			"val=%d, min=%d, max=%d",
		p.Id,
		p.Type,
		val,
		minSize,
		maxSize)
}

func newParseUintSizeErr(p vimtypes.VAppPropertyInfo, val, minSize, maxSize uint64) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s due to size: "+
			"val=%d, min=%d, max=%d",
		p.Id,
		p.Type,
		val,
		minSize,
		maxSize)
}

func newParseRealSizeErr(p vimtypes.VAppPropertyInfo, val, minSize, maxSize float64) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s due to size: "+
			"val=%f, min=%f, max=%f",
		p.Id,
		p.Type,
		val,
		minSize,
		maxSize)
}

func newParseMinMaxErr(p vimtypes.VAppPropertyInfo, minSize, maxSize int64) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s due to min=%d > max=%d",
		p.Id,
		p.Type,
		minSize,
		maxSize)
}

func newParseRealMinMaxErr(p vimtypes.VAppPropertyInfo, minSize, maxSize float64) error {
	return fmt.Errorf(
		"failed to parse prop=%q, type=%s due to min=%f > max=%f",
		p.Id,
		p.Type,
		minSize,
		maxSize)
}
