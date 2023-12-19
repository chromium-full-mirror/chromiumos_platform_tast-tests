// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenRecorder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the screen recorder Tast API works",
		Contacts:     []string{"chromeos-engprod-syd@google.com", "alvinjia@google.com", "mattlui@google.com"},
		BugComponent: "b:1103568", // ChromeOS -> EngProd -> Apps
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      chrome.LoginTimeout + 2 * time.Minute,
	})
}

// ScreenRecorder Verifies that that the screen recorder Tast API works.
// Steps:
// 1, Initialize the screen recorder.
// 2, Start the screen recorder.
// 3, Wait for 10 seconds.
// 4, Stop the screen recorder.
// 5, Save the recording file.
// 6, Verify that the recording file exists.
func ScreenRecorder(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	recorder, err := uiauto.NewScreenRecorder(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create screen recorder: ", err)
	}

	if err := recorder.Start(ctx, tconn); err != nil {
		s.Fatal("Failed to start screen recorder: ", err)
	}

	// GoBigSleepLint: Allow the screen recording to run for a short period.
	testing.Sleep(ctx, 10*time.Second)

	if err := recorder.Stop(ctx); err != nil {
		s.Fatal("Failed to stop screen recorder: ", err)
	}

	filename := filepath.Join(s.OutDir(), "recording.webm")
	if err := recorder.SaveInBytes(ctx, filename); err != nil {
		s.Fatal("Failed to save screen recorder: ", err)
	}
	defer os.Remove(filename)

	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		s.Fatal("Failed to file the recording file: ", err)
	}
}
