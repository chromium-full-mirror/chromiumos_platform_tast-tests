// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/chrome/version"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/versionutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CompareVersionWithAsh,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that Lacros should not be older than Ash based on the version skew policy",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "lacros-team@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "group:criticalstaging", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
	})
}

func CompareVersionWithAsh(ctx context.Context, s *testing.State) {
	// This test compares Lacros and Ash versions to check if they are supported version skews.
	// If the lacros.DeployedBinary var is specified, the deployed Lacros will be used for comparison to Ash.
	// Otherwise, Rootfs Lacros is used by default.
	lacrosBinary := "rootfs"
	lacrosVersion, err := versionutil.RootfsLacrosVersion()
	if err != nil {
		s.Fatal("Failed to get the Rootfs Lacros version: ", err)
	}

	if deployedPath := lacros.DeployedBinary.Value(); len(deployedPath) != 0 {
		lacrosBinary = "deployed"
		if _, err := os.Stat(deployedPath); err != nil {
			s.Fatal("Failed to locate the path specified with lacros.DeployedBinary: ", err)
		}
		lacrosVersion, err = versionutil.VersionFromExecPath(ctx, filepath.Join(deployedPath, "chrome"))
		if err != nil {
			s.Fatal("Failed to get the deployed Lacros version: ", err)
		}

		// TODO(b/285225974): Move to a common place if it is useful for other tests.
		// Safeguard: Throw an error if --lacros-chrome-path is set in /etc/chrome_dev.conf for override and doesn't match the deployedPath.
		config, err := ioutil.ReadFile("/etc/chrome_dev.conf")
		if err == nil {
			for _, line := range strings.Split(string(config), "\n") {
				tokens := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(tokens) == 2 && tokens[0] == "--lacros-chrome-path" {
					overridePath := tokens[1]
					if deployedPath != overridePath {
						s.Fatal("Found --lacros-chrome-path in /etc/chrome_def.conf, but it doesn't match the lacros.DeployedBinary var")
					}
					break
				}
			}
		}
	}

	ashVersion, err := versionutil.AshVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get the Ash version: ", err)
	}

	s.Logf("Comparing versions of Lacros(%v): %v and Ash: %v", lacrosBinary, lacrosVersion, ashVersion)
	if err := version.IsSkewValid(lacrosVersion, ashVersion); err != nil {
		s.Fatal("Found Lacros is incompatible with Ash: ", err) // err will print both versions.
	}
}
