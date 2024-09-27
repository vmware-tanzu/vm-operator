// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/paused"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto/internal"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

type ptrCfgSpec = *vimtypes.VirtualMachineConfigSpec
type ptrDevCfgSpec = *vimtypes.VirtualDeviceConfigSpec

type cryptoKey struct {
	id                string
	provider          string
	isDefaultProvider bool
}

type reconcileArgs struct {
	k8sClient      ctrlclient.Client
	vimClient      *vim25.Client
	vm             *vmopv1.VirtualMachine
	moVM           mo.VirtualMachine
	configSpec     ptrCfgSpec
	curKey         cryptoKey
	newKey         cryptoKey
	isEncStorClass bool
}

//nolint:gocyclo // The allowed complexity is 30, this is 31.
func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec ptrCfgSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}

	var (
		err    error
		msgs   []string
		reason Reason
		args   = reconcileArgs{
			k8sClient:  k8sClient,
			vimClient:  vimClient,
			vm:         vm,
			moVM:       moVM,
			configSpec: configSpec,
		}
	)

	// Check to if the VM is currently encrypted and record the current provider
	// ID and key ID.
	args.curKey = getCurCryptoKey(moVM)

	// Check whether or not the storage class is encrypted.
	args.isEncStorClass, err = kubeutil.IsEncryptedStorageClass(
		ctx,
		k8sClient,
		vm.Spec.StorageClass)
	if err != nil {
		return err
	}
	if args.isEncStorClass {
		internal.MarkEncryptedStorageClass(ctx)
	}

	// Do not proceed further if the VM is paused.
	if paused.ByAdmin(moVM) || paused.ByDevOps(vm) {
		return nil
	}

	// Get the new provider ID and key ID.
	if args.newKey, reason, msgs, err = getNewCryptoKey(ctx, args); err != nil {
		return err
	}

	var op string
	switch {
	case args.curKey.provider == "" && args.newKey.provider != "":
		op = "encrypting"
		if reason == 0 && len(msgs) == 0 {
			r, m, err := onEncrypt(ctx, args)
			if err != nil {
				return err
			}
			reason |= r
			msgs = append(msgs, m...)
		}
	case args.curKey.provider != "" && args.newKey.provider != "" &&
		((args.curKey.provider != args.newKey.provider) ||
			(args.curKey.provider == args.newKey.provider && args.curKey.id != args.newKey.id)):

		op = "recrypting"
		if reason == 0 && len(msgs) == 0 {
			r, m, err := onRecrypt(ctx, args)
			if err != nil {
				return err
			}
			reason |= r
			msgs = append(msgs, m...)
		}
	case args.curKey.provider != "":
		op = "updating encrypted"
		if reason == 0 && len(msgs) == 0 {
			r, m, err := validateUpdateEncrypted(args)
			if err != nil {
				return err
			}
			reason |= r
			msgs = append(msgs, m...)
		}

	case args.curKey.provider == "":
		op = "updating unencrypted"
		if reason == 0 && len(msgs) == 0 {
			r, m, err := validateUpdateUnencrypted(args)
			if err != nil {
				return err
			}
			reason |= r
			msgs = append(msgs, m...)
		}
	}

	if reason == 0 && len(msgs) == 0 {
		internal.SetOperation(ctx, op)
	} else {
		markEncryptionStateNotSynced(vm, op, reason, msgs...)
	}

	return nil
}

func getCurCryptoKey(moVM mo.VirtualMachine) cryptoKey {
	var curKey cryptoKey
	if moVM.Config == nil {
		return curKey
	}
	if kid := moVM.Config.KeyId; kid != nil {
		curKey.id = kid.KeyId
		if pid := kid.ProviderId; pid != nil {
			curKey.provider = pid.Id
		}
	}
	return curKey
}

func getNewCryptoKey(
	ctx context.Context,
	args reconcileArgs) (cryptoKey, Reason, []string, error) {

	key, reason, msgs, err := getNewCryptoKeyFromEncryptionClass(ctx, args)
	if err != nil || reason > 0 || len(msgs) > 0 {
		// If there was an error getting the key from the encryption class or
		// any reason/messages were set, return that information early.
		return cryptoKey{}, reason, msgs, err
	}

	// The field spec.crypto.UseDefaultProvider defaults to true, so let's
	// assume it is true if nil.
	useDefaultProvider := true
	if crypto := args.vm.Spec.Crypto; crypto != nil &&
		crypto.UseDefaultKeyProvider != nil {

		useDefaultProvider = *crypto.UseDefaultKeyProvider
	}

	// The provider was not found via the encryption class and the user has
	// specified not to use the default provider, so go ahead and return.
	if key.provider == "" && !useDefaultProvider {
		return cryptoKey{}, 0, nil, nil
	}

	// Create a new VIM crypto manager that points to the CryptoManagerKmip
	// singleton on vSphere.
	m := crypto.NewManagerKmip(args.vimClient)

	if key.provider != "" {

		// The encryption class specified a provider, so we need to verify it is
		// valid. Then, if a key was specified, we need to verify that also.

		ok, err := m.IsValidProvider(ctx, key.provider)
		if err != nil {
			return cryptoKey{}, 0, nil, err
		}
		if !ok {
			reason |= ReasonEncryptionClassInvalid
			msgs = append(msgs, "specify encryption class with a valid provider")
		}
		if key.id != "" {
			ok, err := m.IsValidKey(ctx, key.id)
			if err != nil {
				return cryptoKey{}, 0, nil, err
			}
			if !ok {
				reason |= ReasonEncryptionClassInvalid
				msgs = append(msgs, "specify encryption class with a valid key")
			}
		}
		if reason > 0 || len(msgs) > 0 {
			return cryptoKey{}, reason, msgs, nil
		}
		return key, 0, nil, nil
	}

	//
	// At this point we know the provider was not discovered via the encryption
	// class and the VM wants to try and use the default provider, so we need to
	// figure out if one exists.
	//

	if key.provider, _ = m.GetDefaultKmsClusterID(
		ctx, nil, true); key.provider != "" {

		// Ensure key.id="" so vSphere generates a key using the default
		// provider.
		key.id = ""

		// Indicate this is the default provider for use later.
		key.isDefaultProvider = true

		return key, 0, nil, nil
	}

	// There is no default provider. This is not an error, so just return an
	// empty provider ID indicating no provider is available.
	return cryptoKey{}, 0, nil, nil
}

func getNewCryptoKeyFromEncryptionClass(
	ctx context.Context,
	args reconcileArgs) (cryptoKey, Reason, []string, error) {

	var objName string
	if crypto := args.vm.Spec.Crypto; crypto != nil {
		objName = crypto.EncryptionClassName
	}
	if objName == "" {
		// When no encryption class is specified, there is nothing else to do.
		return cryptoKey{}, 0, nil, nil
	}

	// When an encryption class is specified, get the provider ID and key ID
	// from the encryption class.

	var (
		key    cryptoKey
		reason Reason
		msgs   []string
		obj    byokv1.EncryptionClass
		objKey = ctrlclient.ObjectKey{
			Namespace: args.vm.Namespace,
			Name:      objName,
		}
	)

	if err := args.k8sClient.Get(ctx, objKey, &obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return cryptoKey{}, 0, nil, err
		}

		reason |= ReasonEncryptionClassNotFound
		msgs = append(msgs, "specify encryption class that exists")

		// Allow the other reconciliation logic to continue.
		// The VM's encryption state can be synced once the encryption
		// class is ready.
		return cryptoKey{}, reason, msgs, nil

	}

	key.provider, key.id = obj.Spec.KeyProvider, obj.Spec.KeyID
	if key.provider != "" {
		// Return the key from the encryption class.
		return key, 0, nil, nil
	}

	// Allow the other reconciliation logic to continue.
	// The VM's encryption state can be synced once the encryption
	// class is ready.
	reason |= ReasonEncryptionClassInvalid
	msgs = append(msgs, "specify encryption class with a non-empty provider")
	return cryptoKey{}, reason, msgs, nil

}

func onEncrypt(
	ctx context.Context,
	args reconcileArgs) (Reason, []string, error) {

	logger := logr.FromContextOrDiscard(ctx)

	reason, msgs, err := validateEncrypt(args)
	if reason > 0 || len(msgs) > 0 || err != nil {
		return reason, msgs, err
	}

	args.configSpec.Crypto = &vimtypes.CryptoSpecEncrypt{
		CryptoKeyId: vimtypes.CryptoKeyId{
			ProviderId: &vimtypes.KeyProviderId{
				Id: args.newKey.provider,
			},
			KeyId: args.newKey.id,
		},
	}

	logger.Info(
		"Encrypt VM",
		"newKeyID", args.newKey.id,
		"newProviderID", args.newKey.provider,
		"newProviderIsDefault", args.newKey.isDefaultProvider)

	return 0, nil, nil
}

func onRecrypt(
	ctx context.Context,
	args reconcileArgs) (Reason, []string, error) {

	logger := logr.FromContextOrDiscard(ctx)

	reason, msgs, err := validateRecrypt(args)
	if reason > 0 || len(msgs) > 0 || err != nil {
		return reason, msgs, err
	}

	args.configSpec.Crypto = &vimtypes.CryptoSpecShallowRecrypt{
		NewKeyId: vimtypes.CryptoKeyId{
			ProviderId: &vimtypes.KeyProviderId{
				Id: args.newKey.provider,
			},
			KeyId: args.newKey.id,
		},
	}

	logger.Info(
		"Recrypt VM",
		"currentKeyID", args.curKey.id,
		"currentProviderID", args.curKey.provider,
		"newKeyID", args.newKey.id,
		"newProviderID", args.newKey.provider,
		"newProviderIsDefault", args.newKey.isDefaultProvider)

	return 0, nil, nil
}

func validateEncrypt(args reconcileArgs) (Reason, []string, error) {
	var (
		msgs   []string
		reason Reason
	)
	if has, add := hasVTPM(args.moVM, args.configSpec); !has && !add && !args.isEncStorClass {
		if args.newKey.isDefaultProvider {
			return 0, nil, errors.New(
				"encrypting vm requires compatible storage class or vTPM")
		}
		reason |= ReasonInvalidState
		msgs = append(msgs, "use encryption storage class or have vTPM")
	}
	if r, m := validatePoweredOffNoSnapshots(args.moVM); len(m) > 0 {
		reason |= r
		msgs = append(msgs, m...)
	}
	if r, m := validateDeviceChanges(args.configSpec); len(m) > 0 {
		reason |= r
		msgs = append(msgs, m...)
	}
	return reason, msgs, nil
}

func validateRecrypt(args reconcileArgs) (Reason, []string, error) {
	var (
		msgs   []string
		reason Reason
	)
	if has, add := hasVTPM(args.moVM, args.configSpec); !has && !add && !args.isEncStorClass {
		if args.newKey.isDefaultProvider {
			return 0, nil, errors.New(
				"recrypting vm requires compatible storage class or vTPM")
		}
		reason |= ReasonInvalidState
		msgs = append(msgs, "use encryption storage class or have vTPM")
	}
	if hasSnapshotTree(args.moVM) {
		msgs = append(msgs, "not have snapshot tree")
		reason |= ReasonInvalidState
	}
	if r, m := validateDeviceChanges(args.configSpec); len(m) > 0 {
		reason |= r
		msgs = append(msgs, m...)
	}
	return reason, msgs, nil
}

func validateUpdateEncrypted(args reconcileArgs) (Reason, []string, error) {
	var (
		msgs   []string
		reason Reason
	)
	if has, add := hasVTPM(args.moVM, args.configSpec); !has && !add && !args.isEncStorClass {
		if args.newKey.isDefaultProvider {
			return 0, nil, errors.New(
				"updating encrypted vm requires compatible storage class or vTPM")
		}
		reason |= ReasonInvalidState
		msgs = append(msgs, "use encryption storage class or have vTPM")
	}
	if isChangingSecretKey(args.configSpec) {
		msgs = append(msgs, "not add/remove/modify secret key")
		reason |= ReasonInvalidChanges
	}
	if r, m := validateDeviceChanges(args.configSpec); len(m) > 0 {
		reason |= r
		msgs = append(msgs, m...)
	}
	return reason, msgs, nil
}

//nolint:unparam
func validateUpdateUnencrypted(args reconcileArgs) (Reason, []string, error) {
	var (
		msgs   []string
		reason Reason
	)

	if has, add := hasVTPM(args.moVM, args.configSpec); has || add || args.isEncStorClass {
		if args.isEncStorClass {
			reason |= ReasonInvalidState
			msgs = append(msgs, "not use encryption storage class")
		}
		if has {
			reason |= ReasonInvalidState
			msgs = append(msgs, "not have vTPM")
		}
		if add {
			reason |= ReasonInvalidChanges
			msgs = append(msgs, "not add vTPM")
		}
	}
	return reason, msgs, nil
}

func hasVTPM(moVM mo.VirtualMachine, configSpec ptrCfgSpec) (bool, bool) {
	var (
		has bool
		add bool
	)
	if c := moVM.Config; c != nil {
		for i := range c.Hardware.Device {
			if _, ok := c.Hardware.Device[i].(*vimtypes.VirtualTPM); ok {
				has = true
				break
			}
		}
	}
	if configSpec == nil {
		return has, add
	}
	for i := range configSpec.DeviceChange {
		if devChange := configSpec.DeviceChange[i]; devChange != nil {
			if devSpec := devChange.GetVirtualDeviceConfigSpec(); devSpec != nil {
				if _, ok := devSpec.Device.(*vimtypes.VirtualTPM); ok {
					if devSpec.Operation == vimtypes.VirtualDeviceConfigSpecOperationAdd {
						add = true
					} else if devSpec.Operation == vimtypes.VirtualDeviceConfigSpecOperationRemove {
						has = false
					}
					break
				}
			}
		}
	}
	return has, add
}

func validatePoweredOffNoSnapshots(moVM mo.VirtualMachine) (Reason, []string) {
	var (
		msgs   []string
		reason Reason
	)
	if moVM.Summary.Runtime.PowerState != "" &&
		moVM.Summary.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOff {

		msgs = append(msgs, "be powered off")
		reason |= ReasonInvalidState
	}
	if moVM.Snapshot != nil && moVM.Snapshot.CurrentSnapshot != nil {
		msgs = append(msgs, "not have snapshots")
		reason |= ReasonInvalidState
	}
	return reason, msgs
}

func validateDeviceChanges(configSpec ptrCfgSpec) (Reason, []string) {

	var (
		msgs   []string
		reason Reason
	)

	for i := range configSpec.DeviceChange {
		if devChange := configSpec.DeviceChange[i]; devChange != nil {
			devSpec := devChange.GetVirtualDeviceConfigSpec()
			if isAddEditDeviceSpecEncryptedSansPolicy(devSpec) {
				msgs = append(msgs, "specify policy when encrypting devices")
				reason |= ReasonInvalidChanges
			}
			if isEncryptedRawDiskMapping(devSpec) {
				msgs = append(msgs, "not encrypt raw disks")
				reason |= ReasonInvalidChanges
			}
			if isEncryptedDeviceNonDisk(devSpec) {
				msgs = append(msgs, "not encrypt non-disk devices")
				reason |= ReasonInvalidChanges
			}
			if isEncryptedDeviceWithMultipleBackings(devSpec) {
				msgs = append(msgs, "not encrypt devices with multiple backings")
				reason |= ReasonInvalidChanges
			}
		}
	}

	return reason, msgs
}

var secretKeys = map[string]struct{}{
	"ancestordatafilekeys":           {},
	"cryptostate":                    {},
	"datafilekey":                    {},
	"encryption.required":            {},
	"encryption.required.vtpm":       {},
	"encryption.unspecified.default": {},
}

func isChangingSecretKey(configSpec ptrCfgSpec) bool {
	for i := range configSpec.ExtraConfig {
		if bov := configSpec.ExtraConfig[i]; bov != nil {
			if ov := bov.GetOptionValue(); ov != nil {
				if _, ok := secretKeys[ov.Key]; ok {
					return true
				}
			}
		}
	}
	return false
}

func isAddEditDeviceSpecEncryptedSansPolicy(spec ptrDevCfgSpec) bool {
	if spec != nil {
		switch spec.Operation {
		case vimtypes.VirtualDeviceConfigSpecOperationAdd,
			vimtypes.VirtualDeviceConfigSpecOperationEdit:

			if backing := spec.Backing; backing != nil {
				switch backing.Crypto.(type) {
				case *vimtypes.CryptoSpecEncrypt,
					*vimtypes.CryptoSpecDeepRecrypt,
					*vimtypes.CryptoSpecShallowRecrypt:

					for i := range spec.Profile {
						if doesProfileHaveIOFilters(spec.Profile[i]) {
							return false
						}
					}
					return true // is encrypted/recrypted sans policy
				}
			}
		}
	}
	return false
}

var whiteSpaceRegex = regexp.MustCompile(`[\s\t\n\r]`)

func doesProfileHaveIOFilters(spec vimtypes.BaseVirtualMachineProfileSpec) bool {
	if profile, ok := spec.(*vimtypes.VirtualMachineDefinedProfileSpec); ok {
		if data := profile.ProfileData; data != nil {
			if data.ExtensionKey == "com.vmware.vim.sips" {
				return strings.Contains(
					whiteSpaceRegex.ReplaceAllString(data.ObjectData, ""),
					"<namespace>IOFILTERS</namespace>")
			}
		}
	}
	return false
}

func isEncryptedDeviceNonDisk(spec ptrDevCfgSpec) bool {
	if spec != nil {
		if backing := spec.Backing; backing != nil {
			switch backing.Crypto.(type) {
			case *vimtypes.CryptoSpecEncrypt,
				*vimtypes.CryptoSpecDeepRecrypt,
				*vimtypes.CryptoSpecShallowRecrypt:

				_, isDisk := spec.Device.(*vimtypes.VirtualDisk)
				if !isDisk {
					return true
				}
			}
		}
	}
	return false
}

func isEncryptedDeviceWithMultipleBackings(spec ptrDevCfgSpec) bool {
	if spec != nil {
		if backing := spec.Backing; backing != nil {
			switch backing.Crypto.(type) {
			case *vimtypes.CryptoSpecEncrypt,
				*vimtypes.CryptoSpecDeepRecrypt,
				*vimtypes.CryptoSpecShallowRecrypt:

				return spec.Backing.Parent != nil
			}
		}
	}
	return false
}

func isEncryptedRawDiskMapping(spec ptrDevCfgSpec) bool {
	if spec != nil {
		if backing := spec.Backing; backing != nil {
			switch backing.Crypto.(type) {
			case *vimtypes.CryptoSpecEncrypt,
				*vimtypes.CryptoSpecDeepRecrypt,
				*vimtypes.CryptoSpecShallowRecrypt:

				if disk, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
					//nolint:gocritic
					switch disk.Backing.(type) {
					case *vimtypes.VirtualDiskRawDiskVer2BackingInfo:
						return true
					}
				}
			}
		}
	}
	return false
}

func hasSnapshotTree(moVM mo.VirtualMachine) bool {
	var snapshotTree []vimtypes.VirtualMachineSnapshotTree
	if si := moVM.Snapshot; si != nil {
		snapshotTree = si.RootSnapshotList
	}
	return hasSnapshotTreeInner(snapshotTree)
}

func hasSnapshotTreeInner(nodes []vimtypes.VirtualMachineSnapshotTree) bool {
	switch len(nodes) {
	case 0:
		return false
	case 1:
		return hasSnapshotTreeInner(nodes[0].ChildSnapshotList)
	default:
		return true
	}
}
