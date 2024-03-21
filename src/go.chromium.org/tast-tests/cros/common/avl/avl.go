// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package avl contains utility functions for AVL testing.
package avl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"go.chromium.org/tast/core/errors"
)

const infoFileName = "avl_info.json"

// Info contains fields shared by different AVL component types.
type Info struct {
	PartModel     string `json:"avl_part_model"`
	PartFirmware  string `json:"avl_part_firmware"`
	ComponentType string `json:"avl_component_type"`
}

// SaveInfo saves AVL info to the provided output directory.
// If the file already exists, SaveInfo returns an error.
// In practice, outDir should always be (*testing.State).OutDir().
func SaveInfo(ctx context.Context, outDir string, info Info) error {
	path := filepath.Join(outDir, infoFileName)
	_, err := os.Stat(path)
	if err == nil {
		return errors.Errorf("file %q already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	jsonData, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return os.WriteFile(path, jsonData, 0666)
}
