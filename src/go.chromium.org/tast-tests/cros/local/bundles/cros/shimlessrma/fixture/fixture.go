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
	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shimlessrmaapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// Fixture names.
const (
	Install3pDiag = "install3pDiag"
)

const (
	cleanupTimeout   = chrome.ResetTimeout + 20*time.Second
	iwaFile          = "iwa/dist/diagnostics_app.swbn"
	extensionFile    = "extension/diagnostics_app.crx"
	tmpUsbMountPoint = "/tmp/virtual_usb"
	testFile         = "/var/lib/rmad/.test"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            Install3pDiag,
		Desc:            "Trigger shimless flow and install the diagnostic IWA",
		Contacts:        []string{"chromeos-shimless-eng@google.com"},
		Impl:            newInstall3pDiagFixture(),
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

func newInstall3pDiagFixture() *install3pDiagFixture {
	f := &install3pDiagFixture{}
	return f
}

// install3pDiagFixture implements testing.FixtureImpl.
type install3pDiagFixture struct {
	cr                                    *chrome.Chrome
	v                                     Value
	signinProfileTestExtensionManifestKey string
}

// Value is a value exposed by fixture to tests.
type Value struct {
	Tconn           *chrome.TestConn
	RestartChrome   func(ctx context.Context) error
	Launch3pDiagApp func(ctx context.Context, withUSB bool) error
}

func (f *install3pDiagFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()
	defer func(ctx context.Context) {
		if s.HasError() {
			f.TearDown(ctx, s)
		}
	}(cleanupCtx)
	f.v.RestartChrome = f.restartChrome
	f.v.Launch3pDiagApp = f.launch3pDiagApp

	// Make sure rmad is not currently running.
	if err := upstart.StopJob(ctx, "rmad"); err != nil {
		s.Fatal("Failed to stop rmad, err: ", err)
	}
	// Create a valid empty rmad state file and enter the test mode.
	if err := shimlessrmaapp.CreateEmptyStateFile(); err != nil {
		s.Fatal("Failed to create empty state file for rmad, err: ", err)
	}
	if _, err := os.Create(testFile); err != nil {
		s.Fatal("Failed to create .test file, err: ", err)
	}

	// Open Chrome with Shimless RMA enabled.
	f.signinProfileTestExtensionManifestKey = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	if err := f.restartChrome(ctx); err != nil {
		s.Fatal("Failed to new chrome, err: ", err)
	}

	// Start to create a virtual USB mass storage device.
	usbMassStorage := usbdevice.NewUSBMassStorage()
	if err := usbMassStorage.Init(ctx, 100); err != nil {
		s.Fatal("Failed to set up virtual USB backing file, err: ", err)
	}
	if err := usbMassStorage.PlugIn(ctx, false /* readOnly= */); err != nil {
		s.Fatal("Failed to plug in the virtual USB as RW, err: ", err)
	}
	if err := usbMassStorage.FormatFileSystem(ctx, "mkfs.fat"); err != nil {
		s.Fatal("Failed to format file system, err: ", err)
	}

	// Mount the USB.
	if err := f.mountVirtualUSB(ctx, usbMassStorage.DevicePath()); err != nil {
		s.Fatal("Failed to mount the USB device, err: ", err)
	}

	// Move extension and IWA to target directory.
	for _, file := range extFiles() {
		target := filepath.Join(tmpUsbMountPoint, filepath.Base(file))
		if err := fsutil.CopyFile(s.DataPath(file), target); err != nil {
			s.Fatal("Failed to copy file to /tmp, err: ", err)
		}
		if err := os.Chmod(target, 0777); err != nil {
			s.Fatal("Failed to chmod, err: ", err)
		}
	}

	// There is a strange thing, we can mount the device as RW but fail to mount it as RO.
	// However, re-create the device as RO can solve this problem.
	// Set the virtual USB as read-only so rmad can mount it successfully.
	// TODO(b/299851049) Investigate the root cause.
	if err := testexec.CommandContext(ctx, "umount", tmpUsbMountPoint).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to umount USB, err: ", err)
	}
	if err := usbMassStorage.PlugOut(ctx); err != nil {
		s.Fatal("Failed to plug out the virtual USB, err: ", err)
	}
	if err := usbMassStorage.PlugIn(ctx, true /* readOnly= */); err != nil {
		s.Fatal("Failed to plug in the virtual USB as RO, err: ", err)
	}

	if err := f.launch3pDiagApp(ctx, true /* withUSB= */); err != nil {
		s.Fatal("Failed to enter install flow")
	}

	ui := uiauto.New(f.v.Tconn)
	installButton := nodewith.NameContaining("Install").Role(role.Button)
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

	// Installation should complete, we don't need the USB anymore.
	if err := usbMassStorage.PlugOut(ctx); err != nil {
		s.Fatal("Failed to plug out the virtual USB, err: ", err)
	}
	usbMassStorage.CleanUp(ctx)

	return &f.v
}

func (f *install3pDiagFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.cr != nil {
		if err := f.cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome: ", err)
		}
		f.cr = nil
	}
	shimlessrmaapp.RemoveStateFile()
	upstart.StopJob(ctx, "rmad")
	os.Remove(testFile)
	os.RemoveAll(tmpUsbMountPoint)
}

func (f *install3pDiagFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *install3pDiagFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *install3pDiagFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *install3pDiagFixture) mountVirtualUSB(ctx context.Context, usbPath string) error {
	os.RemoveAll(tmpUsbMountPoint)
	if err := os.MkdirAll(tmpUsbMountPoint, 0777); err != nil {
		return errors.Wrap(err, "failed to create the temporary USB mount point")
	}
	if err := testexec.CommandContext(ctx, "mount", usbPath, tmpUsbMountPoint).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to mount USB")
	}

	return nil
}

func (f *install3pDiagFixture) restartChrome(ctx context.Context) error {
	if f.cr != nil {
		if err := f.cr.Close(ctx); err != nil {
			return errors.Wrap(err, "failed to close chrome")
		}
		f.cr = nil
	}

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ShimlessRMAFlow"),
		chrome.EnableFeatures("IsolatedWebApps"),
		chrome.EnableFeatures("IsolatedWebAppDevMode"),
		chrome.EnableFeatures("ShimlessRMA3pDiagnostics"),
		chrome.EnableFeatures("ShimlessRMA3pDiagnosticsDevMode"),
		chrome.EnableFeatures("IWAForTelemetryExtensionAPI"),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(f.signinProfileTestExtensionManifestKey),
		chrome.ExtraArgs("--launch-rma"))
	if err != nil {
		return errors.Wrap(err, "failed to new chrome")
	}
	f.cr = cr

	tconn, err := f.cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}
	f.v.Tconn = tconn

	return nil
}

func (f *install3pDiagFixture) launch3pDiagApp(ctx context.Context, withUSB bool) error {
	// Trigger the installation flow.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open Keyboard device")
	}
	defer kb.Close(ctx)

	ui := uiauto.New(f.v.Tconn)
	var button *nodewith.Finder
	if withUSB {
		// When there is a USB, UI should enter the install flow.
		button = nodewith.NameContaining("Install").Role(role.Button)
	} else {
		// When there is no USB and the extension and IWA are already installed, it should open the IWA directly.
		button = nodewith.NameContaining("Check extension").Role(role.Button)
	}
	// When rmad is in a busy state, the shortcut is disabled. So we retry
	// for 30 seconds.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		kb.Accel(ctx, "alt+shift+d")
		return ui.Exists(button)(ctx)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to enter the IWA installation flow")
	}
	return nil
}
