// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/updateengine"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	preservedFolderName       = "/mnt/stateful_partition/unencrypted/preserve/update_engine/prefs"
	prefsFileRollbackHappened = "rollback-happened"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FauBlockedAfterRollback,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that no FAU happens if rollback-happened flag is set",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"igorcov@google.com",
		},
		BugComponent: "b:261430340",
		Attr:         []string{"group:hw_agnostic", "group:mainline", "informational"},
		Fixture:      fixture.UpdateEngineCleanOwnership,
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy to a version older than latest FAU and ensure that
				// supported devices correctly roll back and stay on that
				// version.
				// COM_FOUND_CUJ13_TASK7_WF1
				Value: "screenplay-71c595e4-c8e4-41b2-ab1d-1c4e3459e7b5",
			},
		},
	})
}

// triggerUpdate requests an update check at the specified Omaha URL.
func triggerUpdate(ctx context.Context, url string) error {
	// Make sure update_engine_client does not hang.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := testexec.CommandContext(ctx, "update_engine_client", "--check_for_update", fmt.Sprintf("--omaha_url=%s", url)).Run(testexec.DumpLogOnError); err != nil {
		return err
	}

	return nil
}

// createRollbackHappenedFile sets the rollback-happened flag. This should block any forced update.
func createRollbackHappenedFile() error {
	if err := os.MkdirAll(preservedFolderName, 0777); err != nil && !os.IsExist(err) {
		return err
	}
	if err := os.WriteFile(filepath.Join(preservedFolderName, prefsFileRollbackHappened), []byte("true"), 0644); err != nil {
		return err
	}

	return nil
}

// FauBlockedAfterRollback tests that no FAU happens if rollback-happened flag is set.
func FauBlockedAfterRollback(ctx context.Context, s *testing.State) {
	const (
		localUpdateEngineLog = "/var/log/update_engine.log"
		// This log is displayed if rollback-happened is set or if it's a non-critical update in the response.
		blockedByRollbackHappenedLogEntry = "Ignoring a non-critical Omaha update before OOBE completion."
	)

	updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
	if err != nil {
		s.Fatal("Failed to start nebraska: ", err)
	}
	defer updateServer.Close(ctx)

	if err := updateServer.SetFakedMetadata(ctx); err != nil {
		s.Fatal("Failed to configure Nebraska with faked update metadata: ", err)
	}

	if err := createRollbackHappenedFile(); err != nil {
		s.Fatal("Failed to set the rollback-happened pref file: ", err)
	}

	if err := updateengine.RestartDaemon(ctx); err != nil {
		s.Fatal("Failed to trigger update request: ", err)
	}

	if err := triggerUpdate(ctx, nebraska.UpdateURL(updateServer.Port, true)); err != nil {
		s.Fatal("Failed to trigger update request: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		linkToLog, err := os.Readlink(localUpdateEngineLog)
		if err != nil {
			return errors.Wrap(err, "failed to find update_engine.log on device")
		}

		realLog, err := os.ReadFile(linkToLog)
		if err != nil {
			return errors.Wrap(err, "failed to find on device")
		}

		// Look up for corresponding strings in the update_engine.log
		if !strings.Contains(string(realLog), blockedByRollbackHappenedLogEntry) {
			return errors.New("failed to find proper logs in update_engine.log")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 15 * time.Second,
	}); err != nil {
		s.Error("Could not find expected values: ", err)
	}
}
