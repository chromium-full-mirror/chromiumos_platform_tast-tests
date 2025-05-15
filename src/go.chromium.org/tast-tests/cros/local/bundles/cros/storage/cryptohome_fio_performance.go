// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/storage/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CryptohomeFIOPerformance,
		Desc: "Measure Cryptohome performance",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
		Data:         util.Configs,
		Timeout:      30 * time.Minute,
		Fixture:      "chromeLoggedIn",
		Attr:         []string{"group:crosbolt"},
		Params: []testing.Param{{
			Name: "unencrypted",
			Val:  "/mnt/stateful_partition/unencrypted/",
		}, {
			Name: "sys_encrypted",
			Val:  "/var/",
		}, {
			Name: "user_encrypted",
			Val:  "/home/chronos/user/",
		}},
	})
}

func CryptohomeFIOPerformance(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Run tests to collect metrics.
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	path := filepath.Join(s.Param().(string), "testfile")
	testConfig := &util.TestConfig{
		ResultWriter: resultWriter,
		Path:         path,
	}
	// Run fio workloads
	for _, job := range []string{"4k_read", "4k_write", "4k_seq_read", "4k_seq_write"} {
		if err := util.RunFioStress(ctx,
			testConfig.WithJob(job).
				WithDuration(1*time.Minute).
				WithJobFile(s.DataPath(job))); err != nil {
			s.Error("FIO stress failed: ", err)
		}
	}

}
