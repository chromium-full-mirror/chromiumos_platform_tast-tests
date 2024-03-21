// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package avl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveInfo(t *testing.T) {
	dir := t.TempDir()
	info := Info{
		PartModel:     "test-model",
		PartFirmware:  "test-firmware",
		ComponentType: "test-component-type",
	}

	if err := SaveInfo(context.Background(), dir, info); err != nil {
		t.Fatal("SaveInfo: ", err)
	}

	outBytes, err := os.ReadFile(filepath.Join(dir, infoFileName))
	if err != nil {
		t.Fatal("ReadFile: ", err)
	}

	var outInfo Info
	err = json.Unmarshal(outBytes, &outInfo)
	if err != nil {
		t.Fatal("Unmarshal: ", err)
	}

	if outInfo != info {
		t.Errorf("Unmarshal(): expected %v, got %v", info, outInfo)
	}

	err = SaveInfo(context.Background(), dir, info)
	if err == nil {
		t.Errorf("SaveInfo(): expected error, got no error")
	}
}
