// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	lacroscommon "go.chromium.org/tast-tests/cros/common/cros/lacros"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfaillog"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosinfo"
	"go.chromium.org/tast-tests/cros/local/chrome/versionutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShelfLaunchOmaha,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests launching and interacting with stateful-lacros across the supported channels served in Omaha",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "lacros-tast@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros", "lacros_stable"},
		// Only run on a subset of devices since it downloads from omaha and it will not use our lab's caching mechanisms. We don't want to overload our lab.
		HardwareDeps: hwdep.D(hwdep.Model("kasumi", "vilboz" /* amd64 */, "krane" /* arm */, "lazor" /* arm64 */)),
		Timeout:      4 * time.Minute,
	})
}

// chromeOSVersion returns a string representation of the current OS version. eg. "12345.0.0"
func chromeOSVersion() (string, error) {
	lsb, err := lsbrelease.Load()
	if err != nil {
		return "", errors.Wrap(err, "failed to read lsbrelease")
	}
	version, ok := lsb[lsbrelease.Version]
	if !ok {
		return "", errors.Errorf("failed to find %s in lsbrelease", lsbrelease.Version)
	}
	return version, nil
}

// supportedLacrosPlatform returns a string representation of the lacros platform supported on the architecture the test runs on.
// It internally reads the userspace architecture (possibly different from kernel) to find the matching lacros platform.
// The result is useful for looking up lacros version info using VersionHistory API.
func supportedLacrosPlatform() (string, error) {
	switch arch := runtime.GOARCH; arch {
	case "amd64":
		return "lacros", nil
	case "arm64":
		return "lacros_arm64", nil
	case "arm":
		return "lacros_arm32", nil
	default:
		return "", errors.Errorf("unsupported userspace arch for this test: %v", arch)
	}
}

func waitForLacrosPath(ctx context.Context, tconn *chrome.TestConn) (execPath string, err error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		info, err := lacrosinfo.Snapshot(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get lacros info"))
		}
		if len(info.LacrosPath) == 0 {
			return errors.Wrap(err, "lacros is not yet running (received empty LacrosPath)")
		}
		execPath = filepath.Join(info.LacrosPath, "chrome")
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: time.Second}); err != nil {
		return "", errors.Wrap(err, "lacros is not running")
	}
	return execPath, nil
}

func clearStatefulLacros(ctx context.Context) error {
	// Mark the stateful partition corrupt, so the provision can restore it.
	// Remove it only if the clean up is successful.
	if err := ioutil.WriteFile(lacroscommon.CorruptStatefulFilePath, nil, 0644); err != nil {
		return errors.Wrap(err, "failed to mark the stateful corrupt")
	}

	// Try to unmount provisioned stateful-lacros, then remove mount points.
	matches, _ := filepath.Glob("/run/imageloader/lacros*/*")
	for _, match := range matches {
		if err := testexec.CommandContext(ctx, "umount", "-f", match).Run(); err != nil {
			testing.ContextLog(ctx, "Failed to unmount ", match)
		}
		if err := os.RemoveAll(match); err != nil {
			testing.ContextLog(ctx, "Failed to remove ", match)
		}
	}

	// Remove provisioned files. Note that 'sh' is used to handle the glob.
	lacrosComponentPathGlob := filepath.Join(lacroscommon.LacrosRootComponentPath, "*")
	if err := testexec.CommandContext(ctx, "sh", "-c",
		strings.Join([]string{"rm", "-rf", lacrosComponentPathGlob}, " ")).Run(); err != nil {
		testing.ContextLog(ctx, "Failed to remove provisioned components at ", lacrosComponentPathGlob)
	}

	// If succeeded to clear, we no longer need to mark the stateful partition corrupt.
	matches, _ = filepath.Glob(lacrosComponentPathGlob)
	if len(matches) == 0 {
		if err := os.Remove(lacroscommon.CorruptStatefulFilePath); err != nil {
			testing.ContextLogf(ctx, "Failed to remove the marker file: %v, but the provision will reset the stateful", lacroscommon.CorruptStatefulFilePath)
		}
	}
	return nil
}

func ShelfLaunchOmaha(ctx context.Context, s *testing.State) {
	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Get the current OS, Ash and stateful-lacros versions.
	osVersion, err := chromeOSVersion()
	if err != nil {
		s.Fatal("Failed to get OS version: ", err)
	}
	ashVersion, err := versionutil.AshVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get Ash version: ", err)
	}
	lacrosPlatform, err := supportedLacrosPlatform()
	if err != nil {
		s.Fatal("Failed to get lacros platform: ", err)
	}
	statefulLacrosVersions, err := versionutil.OmahaLacrosVersions(ctx, ashVersion, lacrosPlatform)
	if err != nil {
		s.Fatal("Failed to get Lacros versions compatible with Ash: ", err)
	}

	s.Logf("ShelfLaunch with OS: %v, Ash: %v, stateful-lacros: %v channel(s) %v", osVersion, ashVersion, len(statefulLacrosVersions), statefulLacrosVersions)
	cfg := lacrosfixt.NewConfig(lacrosfixt.Selection(lacros.Omaha))

	// Run sub-tests to check if stateful-lacros is installable and launchable on the channels compatible with Ash
	// from the older milestone to the newer.
	for _, lacrosChannel := range []string{"stable", "beta", "dev", "canary"} {
		lacrosVersion, ok := statefulLacrosVersions[lacrosChannel]
		if !ok {
			continue
		}
		s.Run(ctx, fmt.Sprintf("OS: %v, Ash: %v, stateful-lacros: %v (%v)", osVersion, ashVersion, lacrosVersion, lacrosChannel), func(ctx context.Context, s *testing.State) {
			cr, err := browserfixt.NewChrome(ctx, browser.TypeLacros, cfg,
				chrome.ExtraArgs("--lacros-stability="+lacrosChannel))
			if err != nil {
				s.Fatal("Failed to start Chrome: ", err)
			}
			defer cr.Close(cleanupCtx)

			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to connect to test API: ", err)
			}

			// Ensure Lacros is installed.
			execPath, err := waitForLacrosPath(ctx, tconn)
			if err != nil {
				s.Fatalf("Lacros is not installed from stateful-lacros: %v (%v), Ash: %v, OS: %v, err: %v", lacrosVersion, lacrosChannel, ashVersion, osVersion, err)
			}
			s.Log("Lacros is installed at ", execPath)

			s.Log("Checking that Lacros is included in installed apps")
			appItems, err := ash.ChromeApps(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get installed apps: ", err)
			}
			browser, err := apps.PrimaryBrowser(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get browser app: ", err)
			}
			found := false
			for _, appItem := range appItems {
				if appItem.Type == ash.StandaloneBrowser && appItem.AppID == browser.ID && appItem.Name == browser.Name {
					found = true
					break
				}
			}
			if !found {
				s.Logf("AppID: %v, Name: %v, Type: %v, was expected, but got", browser.ID, browser.Name, ash.StandaloneBrowser)
				for _, appItem := range appItems {
					s.Logf("AppID: %v, Name: %v, Type: %v", appItem.AppID, appItem.Name, appItem.Type)
				}
				s.Fatal("Lacros was not included in the list of installed applications: ", err)
			}

			s.Log("Checking that Lacros is a pinned app in the shelf")
			shelfItems, err := ash.ShelfItems(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get shelf items: ", err)
			}
			found = false
			for _, shelfItem := range shelfItems {
				if shelfItem.AppID == browser.ID && shelfItem.Title == browser.Name && shelfItem.Type == ash.ShelfItemTypePinnedApp {
					found = true
					break
				}
			}
			if !found {
				s.Fatal("Lacros was not found in the list of shelf items: ", err)
			}

			// Reset Lacros to purge user data and close open windows for a clean start.
			if err := lacros.ResetState(ctx, tconn); err != nil {
				s.Fatal("Failed resetting Lacros state: ", err)
			}
			if err = ash.LaunchAppFromShelf(ctx, tconn, browser.Name, browser.ID); err != nil {
				s.Fatal("Failed to launch Lacros: ", err)
			}

			s.Log("Checking that Lacros window is visible")
			if err := lacros.WaitForLacrosWindow(ctx, tconn, "New Tab"); err != nil {
				// Grab Lacros logs to assist debugging before exiting.
				lacrosfaillog.Save(ctx, tconn)
				s.Fatal("Failed waiting for Lacros window to be visible: ", err)
			}

			s.Log("Connecting to the lacros-chrome browser")
			l, err := lacros.Connect(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to connect to lacros-chrome: ", err)
			}
			defer func() {
				s.Log("Closing lacros-chrome browser")
				if err := l.Close(ctx); err != nil {
					s.Fatal("Failed to close lacros-chrome: ", err)
				}
				if err := ash.WaitForAppClosed(ctx, tconn, browser.ID); err != nil {
					s.Fatalf("%s did not close successfully: %s", browser.Name, err)
				}
			}()

			s.Log("Opening a new blank page")
			conn, err := l.NewConn(ctx, chrome.BlankURL)
			if err != nil {
				s.Fatal("Failed to open new tab: ", err)
			}
			defer conn.Close()
			defer conn.CloseTarget(cleanupCtx)
			if err := lacros.WaitForLacrosWindow(ctx, tconn, chrome.BlankURL); err != nil {
				s.Fatal("Failed waiting for Lacros to open new tab page: ", err)
			}
		})
	}

	s.Log("Cleaning up stateful partition for subsequent tests")
	if err := clearStatefulLacros(cleanupCtx); err != nil {
		s.Fatal("Failed cleaning up stateful partition: ", err)
	}
}
