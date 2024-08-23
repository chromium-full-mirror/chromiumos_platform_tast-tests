// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package extension

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

// ComputeExtensionID computes the 32-character ID that Chrome will use for an unpacked
// extension in dir. The extension's manifest file must contain the "key" field.
// Use the following command to generate a new key:
//
//	openssl genrsa 2048 | openssl rsa -pubout -outform der | openssl base64 -A
func ComputeExtensionID(dir string) (string, error) {
	key, err := readKeyFromExtensionManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return "", err
	}

	// Chrome computes an extension's ID by creating a SHA-256 digest of the extension's public key
	// and converting its first 16 bytes to 32 hex characters, with the added twist that the
	// characters 'a'-'p' are used rather than '0'-'f'.
	sum := sha256.Sum256(key)
	id := make([]byte, 32)
	for i, b := range sum[:len(id)/2] {
		id[i*2] = b/16 + 'a'
		id[i*2+1] = b%16 + 'a'
	}
	return string(id), nil
}

// readKeyFromExtensionManifest returns the decoded public key from an
// extension manifest located at path. An error is returned if the manifest
// is missing or malformed. A nil key is returned if the manifest is
// parsable but doesn't contain a key.
func readKeyFromExtensionManifest(path string) ([]byte, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	j := make(map[string]interface{})
	if err = json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	if enc, ok := j["key"].(string); ok {
		return base64.StdEncoding.DecodeString(enc)
	}
	return nil, nil
}
