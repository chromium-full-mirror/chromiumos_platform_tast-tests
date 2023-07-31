// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/localstate"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/updateengine"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Update,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that non critical update applied during oobe for consumer users and ignored for commercial users",
		Contacts: []string{
			"cros-oobe@google.com",
			"bchikhaoui@google.com",
			"bohdanty@google.com",
			"chromeos-sw-engprod@google.com",
			"cros-oac@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.UpdateEngineCleanOwnership,
		Params: []testing.Param{
			{
				Name: "consumer",
				Val:  true,
			},
			{
				Name: "commercial",
				Val:  false,
			},
		},
	})
}

// triggerUpdate requests an update check at the specified Omaha URL.
func triggerUpdate(ctx context.Context, url string) error {
	// Make sure update_engine_client does not hang.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return testexec.CommandContext(ctx, "update_engine_client", "--check_for_update", fmt.Sprintf("--omaha_url=%s", url)).Run(testexec.DumpLogOnError)
}

func Update(ctx context.Context, s *testing.State) {
	isConsumerSegment := s.Param().(bool)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	const (
		localUpdateEngineLog = "/var/log/update_engine.log"
		// This log is displayed if rollback-happened is set or if it's a non-critical update in the response.
		ignoreNonCriticalUpdateLogEntry = "Ignoring a non-critical Omaha update before OOBE completion."
	)

	if err := localstate.MarshalPref(browser.TypeAsh, "IsConsumerSegment", isConsumerSegment); err != nil {
		s.Fatal("Failed to set local state: ", err)
	}

	updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
	if err != nil {
		s.Fatal("Failed to start nebraska: ", err)
	}
	defer updateServer.Close(cleanupCtx)

	if err := updateServer.SetFakedMetadata(ctx); err != nil {
		s.Fatal("Failed to configure Nebraska with faked update metadata: ", err)
	}

	if err := updateServer.SetCriticalUpdate(ctx, false); err != nil {
		s.Fatal("Failed to configure Nebraska with non-critical update: ", err)
	}

	if err := updateengine.RestartDaemon(ctx); err != nil {
		s.Fatal("Failed to restart daemon: ", err)
	}

	if err := triggerUpdate(ctx, nebraska.UpdateURL(updateServer.Port, false)); err != nil {
		s.Fatal("Failed to trigger update request: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		linkToLog, err := os.Readlink(localUpdateEngineLog)
		if err != nil {
			return errors.Wrap(err, "failed to find update_engine.log on device")
		}

		realLog, err := os.ReadFile(linkToLog)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to open update_engine.log on device"))
		}
		ignoreNonCriticalUpdateLogEntryInLog := strings.Contains(string(realLog), ignoreNonCriticalUpdateLogEntry)
		if ignoreNonCriticalUpdateLogEntryInLog == isConsumerSegment {
			return errors.New("failed to find proper logs in update_engine.log")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 15 * time.Second,
	}); err != nil {
		s.Fatal("Could not find expected values: ", err)
	}

}
