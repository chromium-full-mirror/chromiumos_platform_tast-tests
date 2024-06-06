// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/filesystem"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         QuotaProjectID,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that Android's quota project ID setting logic works",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "android_vm", "arc_android_data_cros_access"},
		Fixture:      "arcBooted",
	})
}

func QuotaProjectID(ctx context.Context, s *testing.State) {
	// This number comes from Android's android_filesystem_config.h.
	const aidAppStart = 10000
	// This number comes from Android's android_projectid_config.h.
	const projectIDExtDataStart = 20000

	const (
		apkName      = "ArcQuotaProjectIdTest.apk"
		pkgName      = "org.chromium.arc.testapp.quotaprojectid"
		activityName = "org.chromium.arc.testapp.quotaprojectid.MainActivity"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC
	virtioBlkDataEnabled, err := arc.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is enabled: ", err)
	}

	var androidUIDOffset uint32
	if virtioBlkDataEnabled {
		// When virtio-blk /data is enabled, we mount the disk image of Android's /data on
		// the host's /home/root/<hash>/android-data/data, so there is no UID offset.
		androidUIDOffset = 0
	} else {
		androidUIDOffset = 655360
	}

	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	s.Log("Installing " + apkName)
	if err := a.Install(ctx, arc.APKPath(apkName)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	act, err := arc.NewActivity(a, pkgName, activityName)
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}
	defer act.Close(ctx)

	s.Log("Starting MainActivity")
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start MainActivity: ", err)
	}
	cleanupFunc, err := arc.MountVirtioBlkDataDiskImageReadOnlyIfUsed(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to make Android /data directory available on host: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	// Check the project ID of the package data directory.
	pkgDataDir, err := arc.PkgDataDir(ctx, cr.NormalizedUser(), pkgName)
	if err != nil {
		s.Fatal("Failed to get package data dir: ", err)
	}
	fileInfo, err := os.Stat(pkgDataDir)
	if err != nil {
		s.Fatal("Failed to stat the package data dir: ", err)
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		s.Fatal("Failed to get the stat of the package data dir")
	}
	pkgProjectID := int64(stat.Uid - androidUIDOffset - aidAppStart + projectIDExtDataStart)

	androidDataDir, err := arc.AndroidDataDir(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get Android data dir: ", err)
	}

	downloadsDir, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get Downloads dir: ", err)
	}

	for _, testcase := range []struct {
		path       string
		expectedID int64
	}{
		{pkgDataDir, pkgProjectID},
		// Check the project ID of the file in the external files dir.
		{filepath.Join(pkgDataDir, "files/Pictures/test.png"), pkgProjectID},
		// Check the project ID of the file in the primary external volume.
		{filepath.Join(androidDataDir, "data/media/0/Pictures/test.png"), arc.ProjectIDExtMediaImage},
		// Check the project ID of the file in the Downloads directory.
		{filepath.Join(downloadsDir, "test.png"), arc.ProjectIDExtMediaImage},
	} {
		testing.ContextLog(ctx, "Checking project ID for "+testcase.path)

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			actualID, err := filesystem.QuotaProjectID(ctx, testcase.path)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get project ID"))
			}
			if actualID != testcase.expectedID {
				return errors.Errorf("unexpected project ID: %d, expected %d", actualID, testcase.expectedID)
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
			s.Errorf("Failed to validate project ID for %q: %s", testcase.path, err)
		}
	}
}
