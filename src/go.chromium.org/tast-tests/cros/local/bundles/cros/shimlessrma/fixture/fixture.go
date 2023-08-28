// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shimlessrmaapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// Fixture names.
const (
	InstallIWA = "installIWA"
)

const (
	cleanupTimeout = chrome.ResetTimeout + 20*time.Second
	testFile       = "/var/lib/rmad/.test"
	iwaFile        = "iwa/dist/diag.swbn"
	extensionFile  = "extension/diag.crx"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            InstallIWA,
		Desc:            "Trigger shimless flow and install the diagnostic IWA",
		Contacts:        []string{"chromeos-shimless-eng@google.com"},
		Impl:            newInstallIWAFixture(),
		SetUpTimeout:    chrome.LoginTimeout + 30*time.Second + cleanupTimeout,
		TearDownTimeout: cleanupTimeout,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
		Data:            extFiles(),
	})
}

func extFiles() []string {
	return []string{iwaFile, extensionFile}
}

func newInstallIWAFixture() *installIWAFixture {
	f := &installIWAFixture{}
	return f
}

// installIWAFixture implements testing.FixtureImpl.
type installIWAFixture struct {
	cr *chrome.Chrome
	v  Value
}

// Value is a value exposed by fixture to tests.
type Value struct {
	Tconn *chrome.TestConn
}

func (f *installIWAFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()
	defer func(ctx context.Context) {
		if s.HasError() {
			f.TearDown(ctx, s)
		}
	}(cleanupCtx)

	// Make sure rmad is not currently running.
	if err := upstart.StopJob(ctx, "rmad"); err != nil {
		s.Fatal("Failed to stop rmad, err: ", err)
	}
	// Create a valid empty rmad state file.
	if err := shimlessrmaapp.CreateEmptyStateFile(); err != nil {
		s.Fatal("Failed to create empty state file for rmad, err: ", err)
	}
	// Enable rmad test mode.
	if _, err := os.Create(testFile); err != nil {
		s.Fatal("Failed to create .test file, err: ", err)
	}

	// Open Chrome with Shimless RMA enabled.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("ShimlessRMAFlow"),
		chrome.EnableFeatures("IsolatedWebApps"),
		chrome.EnableFeatures("IsolatedWebAppDevMode"),
		chrome.EnableFeatures("ShimlessRMA3pDiagnostics"),
		chrome.EnableFeatures("ShimlessRMA3pDiagnosticsDevMode"),
		chrome.EnableFeatures("IWAForTelemetryExtensionAPI"),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.ExtraArgs("--launch-rma"))
	if err != nil {
		s.Fatal("Failed to new chrome, err: ", err)
	}
	f.cr = cr

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection, err: ", err)
	}
	f.v.Tconn = tconn

	// Move extension and IWA to target directory.
	for _, file := range extFiles() {
		target := filepath.Join("/tmp", filepath.Base(file))
		if err := fsutil.CopyFile(s.DataPath(file), target); err != nil {
			s.Fatal("Failed to copy file to /tmp, err: ", err)
		}
		if err := os.Chown(target, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
			s.Fatal("Failed to chown, err: ", err)
		}
	}

	// Trigger the installation flow.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open Keyboard device: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)
	installButton := nodewith.NameContaining("Install").Role(role.Button)
	// When rmad is in a busy state, the shortcut is disabled. So we retry
	// for 30 seconds.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		kb.Accel(ctx, "alt+shift+d")
		return ui.Exists(installButton)(ctx)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to enter the IWA installation flow, err: ", err)
	}

	// Install the IWA.
	if err := uiauto.Combine("click the install button",
		ui.WaitUntilExists(installButton),
		ui.LeftClick(installButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click the install button: ", err)
	}

	acceptButton := nodewith.NameContaining("Accept").Role(role.Button)
	if err := uiauto.Combine("click the accept button",
		ui.WaitUntilExists(acceptButton),
		ui.LeftClick(acceptButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click the accept button: ", err)
	}

	return &f.v
}

func (f *installIWAFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.cr != nil {
		if err := f.cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome: ", err)
		}
		f.cr = nil
	}
	shimlessrmaapp.RemoveStateFile()
	testexec.CommandContext(ctx, "stop", "rmad").Run()
	os.Remove(testFile)
}

func (f *installIWAFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *installIWAFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *installIWAFixture) Reset(ctx context.Context) error {
	return nil
}
