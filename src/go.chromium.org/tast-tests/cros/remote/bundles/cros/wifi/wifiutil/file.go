// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.chromium.org/tast/core/errors"
)

const keyValFileName = "keyval"

// KeyValFile holds keyval file.
type KeyValFile struct {
	file *os.File
}

// NewKeyValsFile creates a new KeyValFile.
func NewKeyValsFile(outDir string) (*KeyValFile, error) {
	filename := filepath.Join(outDir, keyValFileName)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file: %s", filename)
	}
	return &KeyValFile{file: f}, nil
}

// WriteKeyVals writes given key value data to an external file in output directory.
func (k *KeyValFile) WriteKeyVals(keyVals map[string]string) error {
	if keyVals == nil {
		return errors.New("invalid data to write to keyval file")
	}

	// Sorting all results by name before writing to file.
	keys := make([]string, 0, len(keyVals))
	for kv := range keyVals {
		keys = append(keys, kv)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, err := fmt.Fprintf(k.file, "%s=%s\n", key, keyVals[key]); err != nil {
			return errors.Wrap(err, "failed to write keyval file")
		}
	}
	return nil
}

// Close closes the keyval file.
func (k *KeyValFile) Close() {
	k.file.Close()
}
