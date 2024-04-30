// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Diagnostics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Camera Diagnostics tests",
		Contacts:     []string{"chromeos-camera-eng@google.com", "imranziad@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      10 * time.Minute,
	})
}

func Diagnostics(ctx context.Context, s *testing.State) {
	const gtestExecutable = "camera_diag_tests"
	if report, err := gtest.New(
		gtestExecutable,
		gtest.Logfile(filepath.Join(s.OutDir(), gtestExecutable+".log")),
	).Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		s.Errorf("Failed to run %v: %v", gtestExecutable, err)
	}
}
