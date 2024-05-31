// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Drivefs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that drivefs mounts on sign in",
		Contacts:     []string{"chromeos-files-syd@google.com", "austinct@chromium.org"},
		BugComponent: "b:167289",
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
			"gaia",
		},
		Attr:    []string{"group:drivefs-cq", "group:mainline"},
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
		Timeout: chrome.GAIALoginTimeout + time.Minute,
	})
}

func Drivefs(ctx context.Context, s *testing.State) {
	// Sign in a real user.
	cr, err := chrome.New(
		ctx,
		chrome.ARCDisabled(),
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	mountPath, err := drivefs.WaitForDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed waiting for DriveFS to start: ", err)
	}
	s.Log("drivefs fully started")

	// We expect to find at least this folder in the mount point.
	drivefsRoot := path.Join(mountPath, "root")
	dir, err := os.Stat(drivefsRoot)
	if err != nil {
		s.Fatal("Could not stat ", drivefsRoot, ": ", err)
	}
	if !dir.IsDir() {
		s.Fatal("Could not find root folder inside ", mountPath, ": ", err)
	}

	// Check for team_drives too.
	drivefsTeamDrives := path.Join(mountPath, "team_drives")
	dir, err = os.Stat(drivefsTeamDrives)
	if err != nil {
		s.Fatal("Could not stat ", drivefsTeamDrives, ": ", err)
	}
	if !dir.IsDir() {
		s.Fatal("Could not find team_drives folder inside ", mountPath, ": ", err)
	}
}
