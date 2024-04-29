// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosca

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/crosca/croscalinux"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoPlaying,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run cuj.VideoPlaying python test",
		Contacts:     []string{"cienet-development@googlegroups.com", "alston.huang@cienet.com"},
		BugComponent: "b:1485133", // ChromeOS > Platform > baseOS > Performance > Competitive Analysis
		SoftwareDeps: []string{"chrome", "arc"},
		Timeout:      15 * time.Minute,
		Vars: []string{
			"crosca.cujAccountPool", // Required. GAIA login credentials. Format: username:password,username:password
			"crosca.projectDir",     // Optional. The "cros_ca_linux" directory path on this host.
			"duration",              // Optional. Format: var=val,var=val
		},
	})
}

// VideoPlaying runs cuj.VideoPlaying python test.
func VideoPlaying(ctx context.Context, s *testing.State) {
	// TODO: Download CrOS CA package from GS Bucket.
	projectDir := croscalinux.DefaultDirPath
	if p, ok := s.Var("crosca.projectDir"); ok {
		projectDir = p
	}

	dut := s.DUT().HostName()
	args, err := croscalinux.GenerateCommand(dut, "cuj.VideoPlaying", s.OutDir(), s.RequiredVar("crosca.cujAccountPool"))
	if err != nil {
		s.Fatal("Failed to generate test command: ", err)
	}

	if duration, ok := s.Var("duration"); ok {
		args = append(args, fmt.Sprintf("--var=duration=%s", duration))
	}

	cmd := testexec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run the test: ", err)
	}
	s.Log("Output: ", string(out))

	// Check if the python program is ended.
	// TODO: |timeout| should be configurable.
	resultDir, err := croscalinux.GetResultDirPath(ctx, projectDir, 10*time.Minute)
	if err != nil {
		s.Fatal("Failed to finish the test: ", err)
	}
	testing.ContextLog(ctx, "Result directory: ", resultDir)

	results, err := croscalinux.ParseResultsJSON(ctx, resultDir)
	if err != nil {
		s.Fatal("Failed to parse test results: ", err)
	}
	for _, result := range results {
		if result.Result != "PASS" {
			s.Fatalf("Failed to run the test %q failed: %s", result.Name, result.Error)
		}
	}
}
