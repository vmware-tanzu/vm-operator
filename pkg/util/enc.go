// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
)

// Base64Decode returns a byte slice decoded from a base64 byte slice.
func Base64Decode(src []byte) ([]byte, error) {
	b64 := base64.StdEncoding
	dst := make([]byte, b64.DecodedLen(len(src)))
	n, err := b64.Decode(dst, src)
	if err != nil {
		return nil, err
	}
	return dst[:n], nil
}

// TryToDecodeBase64Gzip base64-decodes the provided data until the
// DecodeString function fails. If the result is gzipped, then it is
// decompressed and returned. Otherwise the decoded data is returned.
//
// This function will also return the original data as a string if it was
// neither base64 encoded or gzipped.
func TryToDecodeBase64Gzip(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	gzipOrPlainText := data
	for {
		decoded, err := Base64Decode(data)
		if err != nil {
			break
		}
		data = decoded
		gzipOrPlainText = data
	}

	r, err := gzip.NewReader(bytes.NewReader(gzipOrPlainText))
	if err != nil {
		//nolint:nilerr
		return string(gzipOrPlainText), nil
	}

	plainText, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

// EncodeGzipBase64 compresses the input string using gzip and then encodes it
// using base64.
func EncodeGzipBase64(s string) (string, error) {
	var zbuf bytes.Buffer
	zw := gzip.NewWriter(&zbuf)
	if _, err := zw.Write([]byte(s)); err != nil {
		return "", err
	}
	// Flush before closing to ensure all data is written.
	if err := zw.Flush(); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	b64 := base64.StdEncoding.EncodeToString(zbuf.Bytes())
	return b64, nil
}
