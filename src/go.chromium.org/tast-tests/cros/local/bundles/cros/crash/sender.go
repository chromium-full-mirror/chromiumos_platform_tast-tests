// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Sender,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Basic test to check that minidump crashes are uploaded",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"iby@chromium.org",
			"nya@chromium.org", // ported to Tast
		},
		BugComponent: "b:1032705",
		// We only care about crash_sender on internal builds.
		// Chrome is needed either for real consent or for metrics_consent.
		SoftwareDeps: []string{"cros_internal", "chrome"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			Name:              "real_consent",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"metrics_consent"},
			Fixture:           crash.LoggedInRealConsent,
		}, {
			Name:    "mock_consent_fieldtrial_testing_config_off",
			Fixture: crash.MockConsentFieldTrialConfigDisable,
		}, {
			Name:    "mock_consent_fieldtrial_testing_config_on",
			Fixture: crash.MockConsentFieldTrialConfigEnable,
		}},
	})
}

func Sender(ctx context.Context, s *testing.State) {
	if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes(crash.FilterInIgnoreAllCrashes)); err != nil {
		s.Fatal("Setup failed: ", err)
	}
	defer crash.TearDownCrashTest(ctx)

	const basename = "some_program.1.2.3"
	exp, err := crash.AddFakeMinidumpCrash(ctx, basename)
	if err != nil {
		s.Fatal("Failed to add a fake minidump crash: ", err)
	}

	got, err := crash.RunSender(ctx)
	if err != nil {
		s.Fatal("Failed to run crash_sender: ", err)
	}
	want := []*crash.SendResult{{
		Success: true,
		Data:    *exp,
	}}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(crash.SendResult{}, "Schedule")); diff != "" {
		s.Log("Results mismatch (-got +want): ", diff)
		s.Errorf("crash_sender sent unexpected %d results; see logs for diff", len(got))
	}

	// Below we do extra checks that might not be covered by variants of this test.

	// Check that the scheduled upload time is reasonable.
	if len(got) == 1 {
		r := got[0]
		d := r.Schedule.Sub(time.Now())
		const limit = time.Hour
		if d >= limit {
			s.Errorf("Scheduled time was too late: got %v, want <%v", d, limit)
		}
	}

	// Check that the metadata was removed.
	if _, err := os.Stat(filepath.Join(crash.SystemCrashDir, basename+".meta")); err == nil {
		s.Errorf("%s.meta was not removed by crash_sender", basename)
	} else if !os.IsNotExist(err) {
		s.Errorf("Failed to stat %s.meta: %v", basename, err)
	}

	// Check that a send record file is created for rate limiting.
	if rs, err := crash.ListSendRecords(); err != nil {
		s.Error("Failed to list send records: ", err)
	} else if len(rs) != 1 {
		s.Errorf("Found %d send record(s); want 1", len(rs))
	}
}
