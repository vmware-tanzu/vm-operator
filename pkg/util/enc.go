// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
)

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
		decoded, err := base64.StdEncoding.DecodeString(string(data))
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
