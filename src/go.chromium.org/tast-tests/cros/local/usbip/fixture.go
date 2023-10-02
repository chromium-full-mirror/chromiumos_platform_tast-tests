// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/network/tcpdump"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// UsbipModulesLoadedTimeout defines the timeouts for loadModuleFixture.
// The timeout is arbitrarily set. We don't have a clear
// idea of how long it takes, but we suspect it is not more than this.
const UsbipModulesLoadedTimeout = 10 * time.Second

// UsbipServerTimeout defines the timeouts for usbipServerFixture.
const UsbipServerTimeout = 10 * time.Second

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "usbipModulesLoaded",
		Desc:            "Kernel modules necessary for `usbip` loaded",
		Contacts:        []string{"chromeos-engprod-syd@google.com", "ashpakov@google.com"},
		Impl:            &LoadModuleFixture{},
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "usbipServer",
		Desc:            "A running USBIP server for device emulation",
		Contacts:        []string{"chromeos-engprod-syd@google.com", "ashpakov@google.com"},
		Impl:            &ServerFixture{},
		Parent:          "usbipModulesLoaded",
		SetUpTimeout:    UsbipServerTimeout,
		TearDownTimeout: UsbipServerTimeout,
		PreTestTimeout:  UsbipServerTimeout,
		PostTestTimeout: UsbipServerTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoaded",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `chromeLoggedIn` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "chromeLoggedIn",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoadedWithDriveFsStarted",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `chromeLoggedInWithGaia` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "driveFsStarted",
		SetUpTimeout:    UsbipModulesLoadedTimeout + drivefs.DriveFsSetupAndTearDownTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout + drivefs.DriveFsSetupAndTearDownTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoadedWithLacros",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `lacros` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "lacros",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoadedWithArcBooted",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `arcBooted` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "arcBooted",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoadedWithChromePolicyLoggedIn",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `chromePolicyLoggedIn` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "chromePolicyLoggedIn",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUsbPrinterModulesLoadedWithLacrosPolicyLoggedIn",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `lacrosPolicyLoggedIn` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "lacrosPolicyLoggedIn",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUSBPrinterModulesLoadedWithChromeEnrolledLoggedIn",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `chromeEnrolledLoggedIn` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "chromeEnrolledLoggedIn",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUSBPrinterModulesLoadedWithLacrosEnrolledLoggedIn",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `lacrosEnrolledLoggedIn` fixture)",
		Contacts:        []string{"project-bolton@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "lacrosEnrolledLoggedIn",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUSBPrinterModulesLoadedWithLacrosPrinterSetupAssistance",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `lacrosPrinterSetupAssistanceEnabled` fixture)",
		Contacts:        []string{"cros-peripherals@google.com", "ashleydp@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "lacrosPrinterSetupAssistanceEnabled",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "virtualUSBPrinterModulesLoadedWithPrinterSetupAssistance",
		Desc:            "Kernel modules necessary for `virtual-usb-printer` loaded (with `chromeLoggedInWithPrinterSetupAssistance` fixture)",
		Contacts:        []string{"cros-peripherals@google.com", "ashleydp@google.com"},
		Impl:            &LoadModuleFixture{},
		Parent:          "chromeLoggedInWithPrinterSetupAssistance",
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
}

// LoadModuleFixture loads kernel modules required for usbip to work
// and starts tcp traffic monitoring.
type LoadModuleFixture struct {
	tcpdump *tcpdump.Runner
}

// SetUp loads the required kernel modules.
func (LoadModuleFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmd := testexec.CommandContext(ctx, "modprobe", "-a", "usbip_core", "vhci-hcd")
	if err := cmd.Run(); err != nil {
		s.Fatal("Failed to install usbip kernel modules: ", err)
	}
	// Provides pass-through for the value yielded by the parent fixture.
	return s.ParentValue()
}

// Reset does nothing.
func (*LoadModuleFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest starts traffic capture.
func (l *LoadModuleFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	l.tcpdump = tcpdump.NewLocalRunner()
	iface := "lo"
	outDir := s.OutDir()
	pcapPath := path.Join(outDir, "tcpdump.pcap")
	stdoutPath := path.Join(outDir, "tcpdump.stdout")
	stderrPath := path.Join(outDir, "tcpdump.stderr")

	stdoutFile, err := prepareDirFile(ctx, stdoutPath)
	if err != nil {
		s.Error("Failed to open stdout log of tcpdump: ", err)
	}
	stderrFile, err := prepareDirFile(ctx, stderrPath)
	if err != nil {
		s.Error("Failed to open stderr log of tcpdump: ", err)
	}
	if err := l.tcpdump.StartTcpdump(ctx, iface, pcapPath, stdoutFile, stderrFile); err != nil {
		s.Error("Failed to start tcpdump: ", err)
	}
}

// PostTest stops traffic capture.
func (l *LoadModuleFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := l.tcpdump.Close(ctx); err != nil {
		s.Error("Failed to close tcpdump: ", err)
	}
}

// TearDown unloads the required kernel modules.
func (*LoadModuleFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	cmd := testexec.CommandContext(ctx, "modprobe", "-r", "-a", "vhci-hcd", "usbip_core")
	if err := cmd.Run(); err != nil {
		s.Error("Failed to remove usbip kernel modules: ", err)
	}
}

// prepareDirFile prepares the base directory for filename and opens the file.
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

// ServerFixture contains a reference to a USBIP server used by the fixture.
type ServerFixture struct {
	server *Server
}

// ServerFixtureData contains a reference to a USBIP server available to the tests.
type ServerFixtureData struct {
	Server *Server
}

// SetUp creates a USBIP server.
func (u *ServerFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	u.server = NewServer()
	return &ServerFixtureData{
		Server: u.server,
	}
}

// Reset is a noop.
func (u *ServerFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest starts the USBIP server.
func (u *ServerFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if err := u.server.Start(ctx, func(err error) {
		s.Fatalf("Error while serving requests: %s", err)
	}); err != nil {
		s.Fatal("Failed to start the server: ", err)
	}
}

// PostTest stops the USBIP server and removes devices.
func (u *ServerFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	u.server.Stop(ctx)
	u.server.RemoveDevices()
}

// TearDown is a noop.
func (u *ServerFixture) TearDown(ctx context.Context, s *testing.FixtState) {}
