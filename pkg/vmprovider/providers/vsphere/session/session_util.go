// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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

func EncryptWebMKS(pubkey string, plaintext string) (string, error) {
	block, _ := pem.Decode([]byte(pubkey))
	if block == nil || block.Type != "PUBLIC KEY" {
		return "", errors.New("failed to decode PEM block containing public key")
	}
	pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return "", err
	}
	cipherbytes, err := rsa.EncryptOAEP(sha512.New(), rand.Reader, pub, []byte(plaintext), nil)
	if err != nil {
		return "", err
	}
	ciphertext := base64.StdEncoding.EncodeToString(cipherbytes)
	return ciphertext, nil
}

func DecryptWebMKS(privkey *rsa.PrivateKey, ciphertext string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	decrypted, err := rsa.DecryptOAEP(sha512.New(), rand.Reader, privkey, decoded, nil)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}
