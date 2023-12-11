// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/shutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCACLI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies CCA command line tool works",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
	})
}

func ccaRun(ctx context.Context, args ...string) error {
	cmd := testexec.CommandContext(ctx, "cca", args...)
	testing.ContextLog(ctx, "Running command: ", shutil.EscapeSlice(cmd.Args))
	return cmd.Run(testexec.DumpLogOnError)
}

func ccaTestOpenClose(ctx context.Context, dir string) error {
	if err := ccaRun(ctx, "open"); err != nil {
		return err
	}

	if err := ccaRun(ctx, "close"); err != nil {
		return err
	}

	return nil
}

func ccaTestTakePhoto(ctx context.Context, dir string) error {
	output := filepath.Join(dir, "photo.jpg")

	if err := ccaRun(ctx, "take-photo", "--output", output); err != nil {
		return errors.Wrap(err, "failed to take a photo")
	}

	return nil
}

func ccaTestRecordVideo(ctx context.Context, dir string) error {
	output := filepath.Join(dir, "video.mp4")

	if err := ccaRun(ctx, "record-video", "--duration=3", "--output", output); err != nil {
		return errors.Wrap(err, "failed to record a video")
	}

	return nil
}

func ccaTestScreenshot(ctx context.Context, dir string) error {
	output := filepath.Join(dir, "screenshot.png")

	if err := ccaRun(ctx, "screenshot", "--output", output); err != nil {
		return errors.Wrap(err, "failed to capture a screenshot")
	}

	return nil
}

func CCACLI(ctx context.Context, s *testing.State) {
	subTestTimeout := 20 * time.Second

	for _, tc := range []struct {
		name     string
		testFunc func(ctx context.Context, dir string) error
	}{
		{"testOpenClose", ccaTestOpenClose},
		{"testTakePhoto", ccaTestTakePhoto},
		{"testRecordVideo", ccaTestRecordVideo},
		{"testScreenshot", ccaTestScreenshot},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
			defer cancel()

			dir, err := os.MkdirTemp("", "cca-cli-*")
			if err != nil {
				s.Fatal("Failed to create temporary directory for output: ", err)
			}
			defer os.RemoveAll(dir)

			if err = tc.testFunc(subTestCtx, dir); err != nil {
				// Preserve the output directory for investigation if it's failed.
				fsutil.CopyDir(dir, filepath.Join(s.OutDir(), tc.name))

				s.Error("Subtest failed: ", err)
			}
		})

	}
}
