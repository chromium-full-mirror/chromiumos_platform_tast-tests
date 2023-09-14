// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featured

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	activeTrialFileDirectory               = "/run/featured/active"
	activeTrialBeforeChromeStartupFilename = "test_trial1,test_group1"
	activeTrialAfterChromeStartupFilename  = "test_trial2,test_group2"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FieldTrialEarlyBoot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that active field trials in early boot are recorded as active in UMA",
		Contacts: []string{
			"cros-telemetry@google.com",
			"kendraketsui@google.com",
			"mutexlox@google.com",
		},
		BugComponent: "b:1096648", // ChromeOS > Data > Engineering > Experimentation
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.MinLoginTimeout + 60*time.Second,
		Params: []testing.Param{{
			Name: "active_trial",
			Val:  true,
		}, {
			Name: "inactive_trial",
			Val:  false,
		}},
	})
}

// isFieldTrialActive returns whether the passed in field trial name exists and is active via chrome.autotestPrivate.isFieldTrialActive API.
func isFieldTrialActive(ctx context.Context, tconn *chrome.TestConn, trialName, groupName string) (bool, error) {
	var trialActive bool
	err := tconn.Call(ctx, &trialActive, `tast.promisify(chrome.autotestPrivate.isFieldTrialActive)`, trialName, groupName)
	return trialActive, err
}

func FieldTrialEarlyBoot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Setup for expected files before Chrome startup.
	activeTrialFileBeforeChromeStartupPath := filepath.Join(activeTrialFileDirectory, activeTrialBeforeChromeStartupFilename)
	activeOnPlatform := s.Param().(bool)
	if activeOnPlatform {
		// Create file.
		file, err := os.Create(activeTrialFileBeforeChromeStartupPath)
		if err != nil {
			s.Fatalf("Failed to create file %v: %s", file, err)
		}
	} else {
		// Delete file in case it already exists from a previous test.
		if err := os.Remove(activeTrialFileBeforeChromeStartupPath); err != nil && !os.IsNotExist(err) {
			s.Fatalf("Failed to delete file %s: %s", activeTrialFileBeforeChromeStartupPath, err)
		}
	}
	// Delete file after test finishes.
	defer func() {
		if err := os.Remove(activeTrialFileBeforeChromeStartupPath); err != nil && !os.IsNotExist(err) {
			s.Errorf("Failed to delete file %s: %s", activeTrialFileBeforeChromeStartupPath, err)
		}
	}()

	// Setup for expected files after Chrome startup.
	activeTrialFileAfterChromeStartupPath := filepath.Join(activeTrialFileDirectory, activeTrialAfterChromeStartupFilename)
	// Delete file in case it exists so it doesn't affect the current test.
	if err := os.Remove(activeTrialFileAfterChromeStartupPath); err != nil && !os.IsNotExist(err) {
		s.Fatalf("Failed to delete file %s: %s", activeTrialFileAfterChromeStartupPath, err)
	}
	// Delete file after test finishes.
	defer func() {
		if err := os.Remove(activeTrialFileAfterChromeStartupPath); err != nil && !os.IsNotExist(err) {
			s.Errorf("Failed to delete file %s: %s", activeTrialFileAfterChromeStartupPath, err)
		}
	}()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Checking for field trial files created before Chrome starts up.
	activeInChromeTrial1, err := isFieldTrialActive(ctx, tconn, "test_trial1", "test_group1")
	if err != nil {
		s.Fatal("Failed to query if field trial created before chrome startup is active in chrome: ", err)
	}
	if activeOnPlatform != activeInChromeTrial1 {
		s.Fatalf("Field trial state for %s does not match between platform and Chrome. Expected %t but got %t", activeTrialBeforeChromeStartupFilename, activeOnPlatform, activeInChromeTrial1)
	}

	// Checking for field trial files created after Chrome starts up.
	if activeOnPlatform {
		// Create file.
		file, err := os.Create(activeTrialFileAfterChromeStartupPath)
		if err != nil {
			s.Fatalf("Failed to create file %v: %s", file, err)
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		activeInChromeTrial2, err := isFieldTrialActive(ctx, tconn, "test_trial2", "test_group2")
		if err != nil {
			return errors.Wrap(err, "failed to query if field trial created after chrome startup is active in chrome")
		}
		if activeOnPlatform != activeInChromeTrial2 {
			return errors.Errorf("field trial state for %s does not match between platform and Chrome. Expected %t but got %t", activeTrialAfterChromeStartupFilename, activeOnPlatform, activeInChromeTrial2)
		}
		return nil
	}, &testing.PollOptions{}); err != nil {
		s.Fatal("Failed to verify field trial state after chrome startup: ", err)
	}
}
