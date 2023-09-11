// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
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
	InstallIWA = "installIWA"
)

const (
	cleanupTimeout   = chrome.ResetTimeout + 20*time.Second
	iwaFile          = "iwa/dist/diagnostics_app.swbn"
	extensionFile    = "extension/diagnostics_app.crx"
	virtualUSBFile   = "/tmp/virtual_usb_file"
	tmpUsbMountPoint = "/tmp/virtual_usb"
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

	// Start to create a virtual USB mass storage device.
	if err := f.setupVirtualUSB(ctx); err != nil {
		s.Fatal("Failed to set up virtual USB, err: ", err)
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
	if err := f.setupVirtualUSBAsRO(ctx); err != nil {
		s.Fatal("Failed to set up virtual USB as RO, err: ", err)
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
	upstart.StopJob(ctx, "rmad")

	// Clean up virtual USB mass storage device.
	// Stop ui to prevent opening files under the mount point. If we fail to stop ui, then we can't umount it as well.
	testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "-r").Run()
	testexec.CommandContext(ctx, "modprobe", "dummy_hcd", "-r").Run()
	os.Remove(virtualUSBFile)
	os.RemoveAll(tmpUsbMountPoint)
}

func (f *installIWAFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *installIWAFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *installIWAFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *installIWAFixture) findUSBDevicePath(ctx context.Context) (string, error) {
	partUUID, err := testexec.CommandContext(ctx, "sfdisk", "--part-uuid", virtualUSBFile, "1").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}

	return "/dev/disk/by-partuuid/" + strings.ToLower(strings.TrimSuffix(string(partUUID), "\n")), nil
}

func (f *installIWAFixture) setupVirtualUSB(ctx context.Context) error {
	// Create the virtual root hub.
	if err := testexec.CommandContext(ctx, "modprobe", "dummy_hcd").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to load dummy_hcd module")
	}
	// Create the backing file for 10MB space.
	os.Remove(virtualUSBFile)
	if err := testexec.CommandContext(ctx, "fallocate", "--length=10M", virtualUSBFile).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create virtual USB file")
	}
	// Set disk label type.
	if err := testexec.CommandContext(ctx, "parted", virtualUSBFile, "mklabel", "gpt").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set disk label type")
	}
	// Create partition. It actually works without this step, but it means there is no /dev/sdX1, rmad will hit a bug here because they assume the final path is /dev/sdXn.
	if err := testexec.CommandContext(ctx, "parted", "-s", virtualUSBFile, "mkpart", "primary", "0%", "4MB").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create partition")
	}

	// Create the virtual USB by using the backing file.
	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "file="+virtualUSBFile, "removable=1").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create virtual USB storage")
	}

	// Find the USB device path.
	usbPath, err := f.findUSBDevicePath(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find the USB device path")
	}

	// Format the file system. We use polling here because there is a delay after modprobe.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return testexec.CommandContext(ctx, "mkfs.fat", usbPath).Run(testexec.DumpLogOnError)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return err
	}

	// Mount the USB.
	os.RemoveAll(tmpUsbMountPoint)
	if err := os.MkdirAll(tmpUsbMountPoint, 0777); err != nil {
		return errors.Wrap(err, "failed to create the temporary USB mount point")
	}
	if err := testexec.CommandContext(ctx, "mount", usbPath, tmpUsbMountPoint).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to mount USB")
	}

	return nil
}

func (f *installIWAFixture) setupVirtualUSBAsRO(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "umount", tmpUsbMountPoint).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to umount USB")
	}
	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "-r").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to remove g_mass_storage module")
	}
	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "file="+virtualUSBFile, "removable=1", "ro=1").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create virtual USB storage as RO device")
	}

	return nil
}
