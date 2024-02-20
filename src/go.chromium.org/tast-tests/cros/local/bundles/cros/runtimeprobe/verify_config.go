// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runtimeprobe contains local tast tests that verifies runtime_probe.
package runtimeprobe

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/runtimeprobe/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/runtimeprobe/fixture"
	"go.chromium.org/tast-tests/cros/local/debugd"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyConfig,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks runtime_probe probe config",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"chungsheng@google.com",
		},
		BugComponent: "b:606088",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"racc"},
		Params: []testing.Param{
			{
				Name:              "released",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
			}, {
				Name:              "unreleased",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
			},
		},
	})
}

// VerifyConfig verifies runtime_probe probe config can be loaded and execute
// without error.
func VerifyConfig(ctx context.Context, s *testing.State) {
	// Debugd is used by runtime_probe. Check we can connect to it to make the
	// error more specific.
	if _, err := debugd.New(ctx); err != nil {
		s.Fatal("Failed to connect to debugd: ", err)
	}

	cmd := testexec.CommandContext(ctx, "runtime_probe", "--to_stdout")
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to run runtime_probe: ", err)
	}
}
