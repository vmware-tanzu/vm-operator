// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func GetWebConsoleTicket(
	vmCtx context.VirtualMachineContext,
	vm *object.VirtualMachine,
	pubKey string) (string, error) {

	vmCtx.Logger.V(5).Info("GetWebMKSTicket")

	ticket, err := vm.AcquireTicket(vmCtx, string(types.VirtualMachineTicketTypeWebmks))
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("wss://%s:%d/ticket/%s", ticket.Host, ticket.Port, ticket.Ticket)
	return EncryptWebMKS(pubKey, url)
}

func EncryptWebMKS(pubKey string, plaintext string) (string, error) {
	block, _ := pem.Decode([]byte(pubKey))
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

func DecryptWebMKS(privKey *rsa.PrivateKey, ciphertext string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	decrypted, err := rsa.DecryptOAEP(sha512.New(), rand.Reader, privKey, decoded, nil)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}
