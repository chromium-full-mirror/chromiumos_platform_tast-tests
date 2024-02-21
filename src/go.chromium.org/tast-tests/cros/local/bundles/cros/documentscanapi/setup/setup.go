// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package setup contains functions and constants needed for setting up documentScan tests.
package setup

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// EsclCapabilities gives basic eSCL scanning capabilities to the virtual-usb-printer.
const EsclCapabilities = "/usr/local/etc/virtual-usb-printer/escl_capabilities.json"

// CreateExtension moves the extension files into the extension directory and returns extension ID.
func CreateExtension(ctx context.Context, s *testing.State, extDir string) (string, error) {
	for _, name := range []string{"manifest.json", "background.js", "scan.html", "scan.js", "scan.css"} {
		if err := fsutil.CopyFile(s.DataPath(name), filepath.Join(extDir, name)); err != nil {
			return "", errors.Wrapf(err, "failed to copy file %q", name)
		}
	}

	extID, err := chrome.ComputeExtensionID(extDir)
	if err != nil {
		s.Fatalf("Failed to compute extension ID for %q: %v", extDir, err)
	}

	return extID, nil
}
