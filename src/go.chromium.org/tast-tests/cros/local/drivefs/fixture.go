// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

const (
	driveFsSetupAndTearDownTimeout = time.Minute
)

var (
	driveAPIScopes = []string{"https://www.googleapis.com/auth/drive"}
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "driveFsStarted",
		Desc:            "Ensures DriveFS is mounted and provides an authenticated Drive API Client",
		Contacts:        []string{"benreich@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl:            &fixture{bt: browser.TypeAsh},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: time.Hour,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "driveFsStartedLacros",
		Desc:            "Lacros variant of driveFsStarted",
		Contacts:        []string{"chromeos-files-syd@chromium.org"},
		Impl:            &fixture{bt: browser.TypeLacros},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: time.Hour,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "driveFsStartedTrashEnabled",
		Desc:     "Ensures DriveFS is mounted and provides an authenticated Drive API Client",
		Contacts: []string{"benreich@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl: &fixture{
			chromeOptions: []chrome.Option{chrome.EnableFeatures("FilesTrash")},
			bt:            browser.TypeAsh,
		},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: chrome.ResetTimeout + driveFsSetupAndTearDownTimeout,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "driveFsStartedBulkPinningEnabled",
		Desc:     "Ensures DriveFS is mounted and provides an authenticated Drive API Client",
		Contacts: []string{"benreich@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl: &fixture{
			enableBulkPinning: true,
			bt:                browser.TypeAsh,
		},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: chrome.ResetTimeout + driveFsSetupAndTearDownTimeout,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "driveFsStartedWithNativeMessaging",
		Desc:     "Ensures DriveFS is mounted and the bidirectional messaging functionality is enabled",
		Contacts: []string{"austinct@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl: &fixture{drivefsOptions: map[string]string{
			"switchblade_dss": "true",
		}, bt: browser.TypeAsh},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: chrome.ResetTimeout + driveFsSetupAndTearDownTimeout,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "driveFsStartedWithNativeMessagingLacros",
		Desc:     "Lacros variant of driveFsStartedWithNativeMessaging",
		Contacts: []string{"austinct@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl: &fixture{drivefsOptions: map[string]string{
			"switchblade_dss": "true",
		}, bt: browser.TypeLacros},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: chrome.ResetTimeout + driveFsSetupAndTearDownTimeout,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "driveFsStartedWithNativeMessagingAndBulkPinningEnabledLacros",
		Desc:     "Lacros variant of driveFsStartedWithNativeMessagingLacros",
		Contacts: []string{"msalomao@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl: &fixture{
			enableBulkPinning: true,
			drivefsOptions: map[string]string{
				"switchblade_dss": "true",
			}, bt: browser.TypeLacros},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: chrome.ResetTimeout + driveFsSetupAndTearDownTimeout,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name: "driveFsStartedWithOfficeEnabled", Desc: "Ensures DriveFS is mounted with #upload-to-office enabled",
		Contacts: []string{"lucmult@chromium.org", "chromeos-files-syd@chromium.org"},
		Impl: &fixture{
			chromeOptions: []chrome.Option{chrome.EnableFeatures("UploadOfficeToCloud"), chrome.ExtraArgs("--disable-sync")},
			bt:            browser.TypeAsh,
		},
		SetUpTimeout:    chrome.LoginTimeout + driveFsSetupAndTearDownTimeout,
		ResetTimeout:    driveFsSetupAndTearDownTimeout,
		TearDownTimeout: chrome.ResetTimeout + driveFsSetupAndTearDownTimeout,
		Vars: []string{
			"drivefs.accountPool",
			"drivefs.extensionClientID",
		},
	})

}

// FixtureData is the struct available for tests.
type FixtureData struct {
	// Chrome is a connection to an already-started Chrome instance.
	// It cannot be closed by tests.
	Chrome *chrome.Chrome

	// The path that DriveFS has mounted at.
	MountPath string

	// The API connection to the Test extension, reused by tests.
	TestAPIConn *chrome.TestConn

	// The APIClient singleton.
	APIClient *APIClient

	// The DriveFS helper, reused by tests.
	DriveFs *DriveFs
}

type fixture struct {
	mountPath         string // The path where Drivefs is mounted
	cr                *chrome.Chrome
	tconn             *chrome.TestConn
	APIClient         *APIClient
	driveFs           *DriveFs
	chromeOptions     []chrome.Option
	drivefsOptions    map[string]string
	bt                browser.Type
	enableBulkPinning bool
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	ctx, st := timing.Start(ctx, "prepare_drivefs_fixture")
	defer st.End()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// If mountPath exists and API client is not nil, check if Drive has stabilized and return early if it has.
	if f.mountPath != "" && f.APIClient != nil {
		dfs, err := NewDriveFs(ctx, f.cr.NormalizedUser())
		if err != nil {
			s.Log("Failed waiting for DriveFS to stabilize: ", err)
			chrome.Unlock()
			f.cleanUp(ctx, s)
		} else {
			f.driveFs = dfs
			f.mountPath = f.driveFs.MountPath()
			return &FixtureData{
				Chrome:      f.cr,
				MountPath:   f.mountPath,
				TestAPIConn: f.tconn,
				APIClient:   f.APIClient,
				DriveFs:     f.driveFs,
			}
		}
	}

	// If initialization fails, this defer is used to clean-up the partially-initialized pre.
	// Stolen verbatim from arc/pre.go
	shouldClose := true
	defer func() {
		if shouldClose {
			f.cleanUp(cleanupCtx, s)
		}
	}()

	func() {
		opts := append(f.chromeOptions,
			chrome.GAIALoginPool(s.RequiredVar("drivefs.accountPool")),
			chrome.ExtraArgs("--get-access-token-for-test"),
			chrome.ARCDisabled(),
		)
		if f.bt == browser.TypeLacros {
			var err error
			opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
			if err != nil {
				s.Fatal("Failed to get lacros options: ", err)
			}
		}
		if f.enableBulkPinning {
			opts = append(opts, chrome.EnableFeatures("FeatureManagementDriveFsBulkPinning"))
		}

		ctx, cancel := context.WithTimeout(ctx, chrome.LoginTimeout)
		defer cancel()

		var err error
		f.cr, err = chrome.New(ctx, opts...)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
	}()

	dfs, err := NewDriveFs(ctx, f.cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed waiting for DriveFS to start: ", err)
	}
	s.Log("drivefs fully started")
	f.driveFs = dfs
	f.mountPath = f.driveFs.MountPath()

	if len(f.drivefsOptions) > 0 {
		var options []string
		for flag, value := range f.drivefsOptions {
			options = append(options, fmt.Sprintf("%s:%s", flag, value))
		}
		cliArgs := fmt.Sprintf("--features=%s", strings.Join(options, ","))
		if err := f.driveFs.WriteCommandLineFlags(cliArgs); err != nil {
			s.Fatal("Failed to write command line args: ", err)
		}
		if err := f.driveFs.Restart(ctx); err != nil {
			s.Fatal("Failed waiting for DriveFS to restart: ", err)
		}
	}

	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed creating test API connection: ", err)
	}
	f.tconn = tconn

	// Perform Drive API authentication.
	ts := NewChromeOSTokenSourceForAccount(
		s.FixtContext(),
		tconn, driveAPIScopes, f.cr.Creds().User)
	rts := RetryTokenSource(ts, WithContext(s.FixtContext()), WithDelay(time.Second*5))
	apiClient, err := CreateAPIClient(ctx, rts)
	if err != nil {
		s.Fatal("Failed to create Drive API client: ", err)
	}
	f.APIClient = apiClient

	// Lock Chrome and make sure deferred function does not run cleanup.
	chrome.Lock()
	shouldClose = false

	return &FixtureData{
		Chrome:      f.cr,
		MountPath:   f.mountPath,
		TestAPIConn: f.tconn,
		APIClient:   f.APIClient,
		DriveFs:     f.driveFs,
	}
}

// TearDown ensures Chrome is unlocked and closed.
func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.Reset(ctx)
	chrome.Unlock()
	f.cleanUp(ctx, s)
}

func (f *fixture) Reset(ctx context.Context) error {
	if f.enableBulkPinning {
		conn, err := f.cr.Browser().NewConn(ctx, "chrome://drive-internals")
		if err != nil {
			return errors.Wrap(err, "failed to open drive-internals to ensure Bulk Pinning is disabled")
		}

		// Wait for the "bulk pinning" toggle to render in "drive-internals".
		if err = conn.WaitForExpr(ctx, "document.querySelector('#bulk-pinning-toggle')"); err != nil {
			return errors.Wrap(err, "bulk pinning toggle did not render in drive-internals")
		}

		// Flip the toggle OFF if it's ON.
		if err = conn.Eval(ctx, "const t = document.querySelector('#bulk-pinning-toggle'); t && t.checked && t.click();", nil); err != nil {
			return errors.Wrap(err, "failed to disable Bulk Pinning")
		}

		// Wait until the end of the event loop cycle to make sure changes are saved before closing all windows.
		if err = conn.Eval(ctx, "new Promise(r => setTimeout(r))", nil); err != nil {
			return errors.Wrap(err, "failed to wait for Bulk Pinning to be disabled")
		}
	}
	if err := ash.CloseAllWindows(ctx, f.tconn); err != nil {
		testing.ContextLog(ctx, "Failed trying to close all windows: ", err)
	}
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

// cleanUp makes a best effort attempt to restore the state to where it was pretest.
func (f *fixture) cleanUp(ctx context.Context, s *testing.FixtState) {
	if f.tconn != nil {
		if err := ash.CloseAllWindows(ctx, f.tconn); err != nil {
			s.Error("Failed trying to close all windows: ", err)
		}
	}

	f.tconn = nil

	if len(f.drivefsOptions) > 0 && f.driveFs != nil {
		if err := f.driveFs.ClearCommandLineFlags(); err != nil {
			s.Fatal("Failed to remove command line args file: ", err)
		}
	}
	f.driveFs = nil
	f.mountPath = ""

	// Clean up files in this account that are older than 1 hour, files past this
	// date are assumed no longer required and were not successfully cleaned up.
	// Note this removal can take a while ~1s per file and may end up exceeding
	// the timeout, this is not a failure as the next run will try to remove the
	// files that weren't deleted in time.
	if f.APIClient != nil {
		fileList, err := f.APIClient.ListAllFilesOlderThan(ctx, time.Hour)
		if err != nil {
			s.Error("Failed to list all my drive files: ", err)
		} else {
			s.Logf("Attempting to remove %d files older than 1 hour", len(fileList.Files))
			for _, i := range fileList.Files {
				if err := f.APIClient.RemoveFileByID(ctx, i.Id); err != nil {
					s.Logf("Failed to remove file %q (%s): %v", i.Name, i.Id, err)
				} else {
					s.Logf("Successfully removed file %q (%s, %s)", i.Name, i.Id, i.ModifiedTime)
				}
			}
		}
	}
	f.APIClient = nil
	if f.cr != nil {
		if err := f.cr.Close(ctx); err != nil {
			s.Log("Failed closing chrome: ", err)
		}
		f.cr = nil
	}
}

// getRefreshTokenForAccount returns the matching refresh token for the
// supplied account. The tokens are stored in a multi line strings as key value
// pairs separated by a ':' character.
func getRefreshTokenForAccount(account, refreshTokens string) (string, error) {
	for i, line := range strings.Split(refreshTokens, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		ps := strings.SplitN(line, ":", 2)
		if len(ps) != 2 {
			return "", errors.Errorf("failed to parse refresh token list: line %d: does not contain a colon", i+1)
		}
		if ps[0] == account {
			return ps[1], nil
		}
	}
	return "", errors.Errorf("failed to retrieve account token for %q", account)
}
