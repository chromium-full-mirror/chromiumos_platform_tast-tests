// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningEnableFromFilesBannerLowSpace,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the bulk pinning feature can't be enabled from Files app if too low space",
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
			"cbx_stable",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-953ca1fc-5d60-49f2-aa76-283dc47ccde7",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningEnableFromFilesBannerLowSpace(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	driveFsClient := fixt.DriveFs

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	// Provide enough time to remove the fill file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	diskCleanup, err := bulkpinning.FillDiskUntilNotEnoughSpace(ctx)
	if err != nil {
		s.Fatal("Failed to fill the users cryptohome to minimal free space: ", err)
	}
	defer diskCleanup()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	bulkPinningBannerFinder := files.WithTimeout(2 * time.Second).WaitUntilExists(nodewith.Role(role.Banner).ClassName("tast-bulk-pinning-banner"))
	dismissButton := nodewith.Role(role.Button).Name("Dismiss")
	getStartedButton := nodewith.Role(role.Button).Name("Get started")
	turnOnButton := nodewith.Role(role.Button).Name("Turn on")
	notEnoughStorageFooter := nodewith.Role(role.StaticText).NameStartingWith("Not enough storage")
	if err := uiauto.Combine("open drive and enable bulk pinning",
		files.OpenDrive(),
		uiauto.IfFailThen(
			bulkPinningBannerFinder,
			files.LeftClickUntil(dismissButton, bulkPinningBannerFinder),
		),
		files.LeftClickUntil(getStartedButton, files.WaitUntilExists(turnOnButton)),
		files.WaitUntilExists(notEnoughStorageFooter),
	)(ctx); err != nil {
		s.Fatal("Failed to enable bulk pinning from Files: ", err)
	}

	if info, err := files.Info(ctx, turnOnButton); err != nil {
		s.Fatal("Failed to get node info on turn on button: ", err)
	} else if info.Restriction != restriction.Disabled {
		s.Fatal(`The "Turn on" button was not disabled`)
	}
}
