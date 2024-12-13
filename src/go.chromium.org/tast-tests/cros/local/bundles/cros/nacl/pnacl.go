// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nacl

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/chrome/extension"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var extensionFiles = []string{
	"chrome_nacl_app/background.js",
	"chrome_nacl_app/manifest.json",
	"chrome_nacl_app/nacl_module.nmf",
	"chrome_nacl_app/nacl_module.pexe",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Pnacl,
		Desc:         "Tests running a PNaCl module",
		Contacts:     []string{"nacl-eng@google.com", "emaxx@chromium.org"},
		Data:         extensionFiles,
		SoftwareDeps: []string{"chrome", "nacl"},
		BugComponent: "b:1258585", // ChromeOS Public Tracker > Enterprise & Edu > NaCl
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(0)),
				ExtraTestBedDeps:  []string{tbdep.Cbx(false)},
			}, {
				// TODO(b/352753237): The test is failing on all cbx models now. Have a
				// separate informational subtest until we have a fix.
				Name:              "flaky",
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraTestBedDeps:  []string{tbdep.Cbx(true)},
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func Pnacl(ctx context.Context, s *testing.State) {
	extDir, err := os.MkdirTemp("", "tast.nacl.PnaclApp.")
	if err != nil {
		s.Fatal("Failed to create temp dir: ", err)
	}
	defer os.RemoveAll(extDir)

	for _, file := range extensionFiles {
		dst := filepath.Join(extDir, filepath.Base(file))
		if err := fsutil.CopyFile(s.DataPath(file), dst); err != nil {
			s.Fatalf("Failed to copy %q file to %q: %v", file, extDir, err)
		}
	}

	extID, err := extension.ComputeExtensionID(extDir)
	if err != nil {
		s.Fatalf("Failed to compute extension ID for %v: %v", extDir, err)
	}

	var opts []chrome.Option
	opts = append(opts, chrome.UnpackedExtension(extDir))
	opts = append(opts, chrome.EnableFeatures("NaclAllow"))

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	s.Log("Connecting to background page")
	bgURL := chrome.ExtensionBackgroundPageURL(extID)
	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		s.Fatalf("Failed to connect to background page at %v: %v", bgURL, err)
	}
	defer conn.Close()

	s.Log("Waiting for JS test function to become available")
	if err := conn.WaitForExpr(ctx, "runTest"); err != nil {
		s.Fatal("JS test function unavailable: ", err)
	}

	s.Log("Executing JS test function")
	if err := conn.Eval(ctx, "runTest()", nil); err != nil {
		s.Fatal("Failed to call JS test function: ", err)
	}
}
