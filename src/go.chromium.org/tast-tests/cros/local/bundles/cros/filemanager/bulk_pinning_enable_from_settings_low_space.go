// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"io"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/syslog"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningEnableFromSettingsLowSpace,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the bulk pinning can't be enabled from settings with low disk space",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
			"fdegros@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_unstable",
		},
		Data: []string{
			"test_1KB.txt",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-008c8610-3a45-4adc-a744-aaa57a3683de",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningEnableFromSettingsLowSpace(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	driveFsClient := fixt.DriveFs

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	// Reserve enough time for the fill file to be cleaned up.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	logLineChan, err := waitForNotEnoughSpaceLog(ctx)

	// Fill the disk until there is minimal free space available. Bulk pinning
	// should not be able to toggle on at this point.
	fillFile, err := disk.FillUntil(cleanup.UserHome, cleanup.MinimalFreeSpace)
	if err != nil {
		s.Fatal("Failed to fill disk space: ", err)
	}
	defer func() {
		if err := os.Remove(fillFile); err != nil {
			s.Errorf("Failed to remove fill file %s: %v", fillFile, err)
		}
	}()
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, fixt.Chrome, bulkpinning.GoogleDriveSettingsPageURL, ui.Exists(bulkpinning.SettingsToggleFinder)); err != nil {
		s.Fatal("Failed to launch settings page: ", err)
	}

	// Ensure that the NotEnoughSpace log line has been seen before continuing
	// to enable bulk pinning in OS Settings. A race condition can happen here
	// where the fill file isn't picked up immediately by the pinning manager.
	select {
	case err := <-logLineChan:
		if err != nil {
			s.Fatal("Failed waiting for 'NotEnoughSpace' log line: ", err)
		}
	case <-ctx.Done():
		s.Fatal("Timed out waiting for matching log line")
	}

	notEnoughSpaceDialog := nodewith.Role(role.Dialog).Name("Not enough storage space")
	if err := uiauto.Combine("toggle bulk pinning",
		// Ensure the toggle is disabled to begin with.
		ui.Exists(bulkpinning.SettingsToggleFinder.Attribute("checked", "false")),
		// Toggle the bulk pinning toggle in OS Settings and wait until the
		// "Not enough storage space" dialog appears.
		ui.LeftClickUntil(
			bulkpinning.SettingsToggleFinder,
			ui.WithTimeout(2*time.Second).WaitUntilExists(notEnoughSpaceDialog),
		),
		// Keep clicking the OK button until the dialog disappears.
		ui.LeftClickUntil(
			nodewith.Role(role.Button).Name("OK"),
			ui.WithTimeout(2*time.Second).WaitUntilGone(notEnoughSpaceDialog),
		),
		// Assert the toggle is still in the unchecked state.
		ui.Exists(bulkpinning.SettingsToggleFinder.Attribute("checked", "false")),
	)(ctx); err != nil {
		s.Fatal("Failed to toggle bulk pinning: ", err)
	}
}

// waitForNotEnoughSpaceLog reads the /var/log/chrome/chrome logs to identify
// the log line:
// [drivefs_pinning_manager.cc(1123)] Finished with error: NotEnoughSpace
// This indicates that the pinning manager knows that there is not enough space
// on the device and future operations should properly behave.
func waitForNotEnoughSpaceLog(ctx context.Context) (chan error, error) {
	logReader, err := syslog.NewLineReader(ctx, syslog.ChromeLogFile, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not get Chrome log reader")
	}

	// Channel will be used to return an error or nil if the log line appears.
	channel := make(chan error, 1)

	go func() {
		defer logReader.Close()
		for {
			select {
			case <-ctx.Done():
				close(channel)
				return
			default:
			}

			line, err := logReader.ReadLine()
			if err == io.EOF {
				// GoBigSleepLint: sleep to avoid spending unnecessary cycles polling for new log lines.
				testing.Sleep(ctx, 200*time.Millisecond)
				continue
			}
			if err != nil {
				channel <- errors.Wrap(err, "failed to read Chrome log line")
				logReader.Close()
				return
			}
			if regexp.MustCompile(`drivefs_pinning_manager\.cc.*?NotEnoughSpace`).MatchString(line) {
				channel <- nil
				logReader.Close()
				return
			}
		}
	}()

	return channel, nil
}
