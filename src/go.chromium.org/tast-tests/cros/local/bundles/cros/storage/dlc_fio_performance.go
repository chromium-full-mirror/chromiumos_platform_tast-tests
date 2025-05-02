// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/storage/util"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const dlcID = "termina-dlc"

func init() {
	testing.AddTest(&testing.Test{
		Func: DLCFIOPerformance,
		Desc: "Measure performance of DLC",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
		Attr:         []string{"group:crosbolt"},
		Data:         util.Configs,
		Timeout:      20 * time.Minute,
	},
	)
}

func DLCFIOPerformance(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Run tests to collect metrics.
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), false)

	// Install termina-dlc.
	if err := dlc.Install(ctx, dlcID, ""); err != nil {
		s.Fatal("Failed to install termina-dlc: ", err)
	}

	// Ensure termina-dlc is uninstalled after the test completes.
	defer func() {
		if err := dlc.Uninstall(cleanupCtx, dlcID); err != nil {
			s.Fatal("Failed to uninstall termina-dlc: ", err)
		}
	}()

	state, err := dlc.GetDlcState(ctx, dlcID)
	if err != nil {
		s.Fatal("Failed to get DLC state: ", err)
	}

	// Get the device path backing the termina-dlc root filesystem.
	path, err := testexec.CommandContext(ctx, "rootdev", state.RootPath).Output()
	if err != nil {
		s.Fatal("Failed to get backing device path for DLC: ", err)
	}

	mountPath := strings.TrimSpace(string(path))

	testConfig := &util.TestConfig{
		ResultWriter: resultWriter,
		Path:         mountPath,
	}

	// Run fio workloads.
	for _, job := range []string{"4k_read", "4k_seq_read"} {
		if err := util.RunFioStress(ctx,
			testConfig.WithJob(job).
				WithJobFile(s.DataPath(job))); err != nil {
			s.Error("FIO stress failed: ", err)
		}
	}
}
