// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestClearHistogramTransferFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestClearHistogramTransferFile")
	if err != nil {
		t.Fatalf("ioutil.TempDir: %v", err)
	}
	defer os.RemoveAll(dir)

	fileName := dir + "/metrics"
	const contents = "ABC123"
	if err = ioutil.WriteFile(fileName, []byte(contents), 0666); err != nil {
		t.Fatalf("ioutil.WriteFile: %v", err)
	}

	if err = clearHistogramTransferFileByName(fileName); err != nil {
		t.Fatalf("clearHistogramTransferFileByName: %v", err)
	}

	if info, err := os.Stat(fileName); err != nil {
		t.Fatalf("os.Stat: %v", err)
	} else if info.Size() != 0 {
		t.Error("file was not truncated")
	}
}

func TestClearHistogramTransferFileWhenFileDoesntExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestClearHistogramTransferFileWhenFileDoesntExist")
	if err != nil {
		t.Fatalf("ioutil.TempDir: %v", err)
	}
	defer os.RemoveAll(dir)

	fileName := dir + "/metrics"
	// Don't create the file.
	if err = clearHistogramTransferFileByName(fileName); err != nil {
		t.Fatalf("clearHistogramTransferFileByName: %v", err)
	}

	_, err = os.Stat(fileName)

	if err == nil {
		t.Error("file was created")
	} else if !os.IsNotExist(err) {
		t.Errorf("os.Stat: %v", err)
	}
}
