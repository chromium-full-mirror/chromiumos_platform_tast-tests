// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/deskscujv2"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksCUJV2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the performance of critical user journey for virtual desks",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"cienet-development@googlegroups.com",
			"vivian.chen@cienet.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Data: []string{
			cujrecorder.SystemTraceConfigFile,
			deskscujv2.AnimationFile,
			"animation.js",
			deskscujv2.MapPDFFile,
		},
		Vars: []string{
			// Parsable test duration, like 10m or 60s, to run the test. This
			// duration is split into 2 sections, where each of the 2 sections
			// of the test gets a second of the total run time. The overall test
			// timeout is still 50 minutes, so command-line test durations must
			// still run in less than that total time. Test time defaults to
			// 10 minutes.
			"ui.DesksCUJV2.duration",
		},
		Fixture: "loggedInToCUJUser",
		Timeout: 30 * time.Minute,
	})
}

func DesksCUJV2(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	var testParam deskscujv2.TestParam

	deskCUJTestDuration := 10 * time.Minute
	if testDuration, ok := s.Var("ui.DesksCUJV2.duration"); ok {
		var err error
		deskCUJTestDuration, err = time.ParseDuration(testDuration)
		if err != nil {
			s.Fatalf("Failed to parse command-line arg ui.DesksCUJV2.duration=%q: %v", testDuration, err)
		}
	}
	testParam.TestDuration = deskCUJTestDuration

	// Run an http server to serve the test contents for accessing from the chrome browsers.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	testParam.AnimationURL = filepath.Join(server.URL, deskscujv2.AnimationFile)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	pdfFileLocation := filepath.Join(downloadsPath, deskscujv2.MapPDFFile)
	if err := fsutil.CopyFile(s.DataPath(deskscujv2.MapPDFFile), pdfFileLocation); err != nil {
		s.Fatalf("Failed to copy the test file %q to Downloads: %v", deskscujv2.MapPDFFile, err)
	}
	defer func(ctx context.Context) {
		if err := os.Remove(pdfFileLocation); err != nil {
			testing.ContextLogf(ctx, "Failed to remove %q", pdfFileLocation)
		}
	}(cleanupCtx)

	pv, err := deskscujv2.Run(ctx, cr, testParam, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile))
	if err != nil {
		s.Fatal("Failed to run DesksCUJV2: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}
