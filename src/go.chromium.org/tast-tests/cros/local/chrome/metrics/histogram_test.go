// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"os"
	"testing"
)

func TestClearHistogramTransferFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestClearHistogramTransferFile")
	if err != nil {
		t.Fatalf("os.MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	fileName := dir + "/metrics"
	const contents = "ABC123"
	if err = os.WriteFile(fileName, []byte(contents), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	if err = clearHistogramTransferFileByName(fileName); err != nil {
		t.Fatalf("clearHistogramTransferFileByName: %v", err)
	}

	if info, err := os.Stat(fileName); err != nil {
		t.Fatalf("os.Stat: %v", err)
	} else if info.Size() != 0 {
		t.Error("file was not truncated")
	} else if info.Mode().Perm() != 0666 {
		t.Errorf("file mode was %v, want 0666", info.Mode().Perm())
	}
}

func TestClearHistogramTransferFileWhenFileDoesntExist(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestClearHistogramTransferFileWhenFileDoesntExist")
	if err != nil {
		t.Fatalf("os.MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	fileName := dir + "/metrics"
	// Don't create the file beforehand.
	if err = clearHistogramTransferFileByName(fileName); err != nil {
		t.Fatalf("clearHistogramTransferFileByName: %v", err)
	}

	info, err := os.Stat(fileName)
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	} else if info.Size() != 0 {
		t.Error("file was not empty")
	} else if info.Mode().Perm() != 0666 {
		t.Errorf("file mode was %v, want 0666", info.Mode().Perm())
	}
}
