// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"os"
	"path/filepath"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SenderDryRun,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Basic test to check that dry run mode prints uploads.log entries",
		Contacts: []string{
			"cros-telemetry@google.com",
			"mutexlox@chromium.org",
			"iby@chromium.org",
			"xuhong@chromium.org",
		},
		// ChromeOS > Data > Engineering > Crash Reporting
		BugComponent: "b:1032705",
		// We only care about crash_sender on internal builds.
		SoftwareDeps: []string{"cros_internal"},
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Name:              "real_consent",
			ExtraSoftwareDeps: []string{"metrics_consent"},
			Fixture:           crash.LoggedInRealConsent,
		}, {
			Name:    "mock_consent",
			Fixture: crash.MockConsentFixture,
		}},
	})
}

func SenderDryRun(ctx context.Context, s *testing.State) {
	if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes(crash.FilterInIgnoreAllCrashes)); err != nil {
		s.Fatal("Setup failed: ", err)
	}
	defer crash.TearDownCrashTest(ctx)

	const basename = "some_program.6.7.8"
	exp, err := crash.AddFakeMinidumpCrash(ctx, basename)
	if err != nil {
		s.Fatal("Failed to add a fake minidump crash: ", err)
	}

	got, uploadsLogEntries, err := crash.RunSenderInDryRun(ctx)
	if err != nil {
		s.Fatal("Failed to run crash_sender: ", err)
	}
	want := []*crash.SendResult{{
		Success: false,
		Data:    *exp,
	}}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(crash.SendResult{}, "Schedule")); diff != "" {
		s.Log("Results mismatch (-got +want): ", diff)
		s.Errorf("crash_sender sent unexpected %d results; see logs for diff", len(got))
	}

	// Below we do extra checks that might not be covered by variants of this test.

	// Check that printed uploads.log entries are reasonable.
	if len(uploadsLogEntries) != 1 {
		s.Errorf("Found %d uploads.log entries in stdout; want 1", len(uploadsLogEntries))
	}
	uploadsLogEntry := uploadsLogEntries[0]
	s.Logf("entry of uploads.log: %+v", uploadsLogEntry)
	uploadsLogEntryWant := crash.UploadsLogEntry{
		FatalCrashType: "",
		LocalID:        "ChromeOS",
		Source:         "some_exec",
		// md5sum of "/var/spool/crash/some_program.6.7.8.meta"
		PathHash: "09219fc3eb027acf7e76f039407ba3c4",
		State:    3,
		UploadID: "",
		// This field depends on the time at which the test runs
		UploadTime: uploadsLogEntry.UploadTime,
	}
	if *uploadsLogEntry != uploadsLogEntryWant {
		s.Errorf("Found uploads.log entry has source %+v; want %+v", uploadsLogEntry, uploadsLogEntryWant)
	}

	// Check that the metadata file stays.
	if _, err := os.Stat(filepath.Join(crash.SystemCrashDir, basename+".meta")); err != nil {
		s.Errorf("%s.meta was removed by crash_sender", basename)
	}

	// Check that no send record file is created for rate limiting.
	if rs, err := crash.ListSendRecords(); err != nil {
		s.Error("Failed to list send records: ", err)
	} else if len(rs) != 0 {
		s.Errorf("Found %d send record(s); want 0", len(rs))
	}
}
