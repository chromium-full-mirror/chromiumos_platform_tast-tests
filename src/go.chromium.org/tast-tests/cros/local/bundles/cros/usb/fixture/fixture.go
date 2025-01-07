// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture contains fixtures which can be used while running Type C tests.
package fixture

import (
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	localTcpdump "go.chromium.org/tast-tests/cros/local/network/tcpdump"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "usbmonFixture",
		Desc:            "Capture the usbmon traffic for the duration of the test.",
		Contacts:        []string{"danielgeorgem@google.com", "chromeos-usb-champs@google.com"},
		BugComponent:    "b:958036", // ChromeOS > Platform > Connectivity > USB
		Impl:            &implUsbMon{},
		SetUpTimeout:    10 * time.Second,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
	})
}

const (
	tcpdumpStderrFile = "usbmon_fixture.stderr"
	tcpdumpStdoutFile = "usbmon_fixture.stdout"
	tcpdumpFile       = "usbmon_fixture.pcap"
	intf              = "usbmon0"
	mountCmd          = "mount -t debugfs none_debugs /sys/kernel/debug"
	tcpdumpListCmd    = "tcpdump --list-interfaces | grep -i usbmon"
	debugfsUsbmon     = "/sys/kernel/debug/usb/usbmon/"
)

type implUsbMon struct {
	usbmonCapable bool
	runner        *localTcpdump.Runner
}

// To run usbmon we need the usbmon driver and debugfs
func (i *implUsbMon) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	i.usbmonCapable = false

	//Try to mount debugfs if it's not mounted. If that fails, disable the fixture.
	if _, err := os.Stat(debugfsUsbmon); errors.Is(err, os.ErrNotExist) {
		if err := testexec.CommandContext(ctx, "sh", "-c", mountCmd).Run(); err != nil {
			s.Log("Can't mount debugfs, skipping fixture:", err)
			return i
		}
	}

	if err := testexec.CommandContext(ctx, "modprobe", "usbmon").Run(); err != nil {
		s.Log("Can't load usbmon driver, skipping fixture: ", err)
		return i
	}

	//We need to make sure that tcpdump can see the usbmon interfaces.
	if err := testexec.CommandContext(ctx, "sh", "-c", tcpdumpListCmd).Run(); err != nil {
		s.Log("No usbmon mon interfaces recognized by tcpdump, skipping fixture:", err)
		return i
	}

	i.usbmonCapable = true

	return i
}

func (i *implUsbMon) TearDown(ctx context.Context, s *testing.FixtState) {
	//Debugfs is mounted by default on test images.
	//Usbmon driver can be safely be left loaded.
}

// The reset will do nothing.
func (i *implUsbMon) Reset(ctx context.Context) error {
	return nil
}

// Start a usbmon capture if we managed to mount debugfs and load usbmon.
// The capture will be stopped in PostTest.
func (i *implUsbMon) PreTest(ctx context.Context, s *testing.FixtTestState) {

	//Make sure we have all the required things before attempting
	//to start the capture.
	if !i.usbmonCapable {
		return
	}

	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	//Create test output artifacts files.
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Log("Output directory not found", outDir)
		return
	}

	stdoutFile, err := prepareDirFile(ctx, path.Join(outDir, tcpdumpStdoutFile))
	if err != nil {
		s.Log("Failed to open stdout log of tcpdump: ", err)
		return
	}

	stderrFile, err := prepareDirFile(ctx, path.Join(outDir, tcpdumpStderrFile))
	if err != nil {
		s.Log("Failed to open stderr log of tcpdump: ", err)
		return
	}

	runner := localTcpdump.NewLocalRunner()
	if err := runner.StartTcpdump(ctx, intf, path.Join(outDir, tcpdumpFile), stdoutFile, stderrFile); err != nil {
		return
	}

	//Save the capture object as it's needed for canceling.
	i.runner = runner
}

// This will stop the usbmon capture started in PreTest. If no capture was started
// due to various errors, this function will do nothing.
func (i *implUsbMon) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if !i.usbmonCapable || i.runner == nil {
		return
	}

	//Stop the usbmon capture.
	cleanupCtx := ctx
	ctx, cancel := i.runner.ReserveForClose(ctx)
	defer cancel()
	defer func(cleanupCtx context.Context) {
		if err := i.runner.Close(cleanupCtx); err != nil {
			s.Log("Failed to stop tcpdump:", err)
		}
	}(cleanupCtx)
}

// Helper function to create the artifact files on the the disk.
func prepareDirFile(ctx context.Context, filename string) (*os.File, error) {
	if err := os.MkdirAll(path.Dir(filename), 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create basedir for %q", filename)
	}

	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)

	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file %q", filename)
	}

	return f, nil
}
