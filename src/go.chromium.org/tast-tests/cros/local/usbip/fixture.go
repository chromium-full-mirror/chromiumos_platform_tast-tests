// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/drivefs"
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
		Contacts:        []string{
			"chromeos-engprod-syd@google.com",
			"mattlui@google.com",
			"alvinjia@google.com",
		},
		Impl:            &LoadModuleFixture{},
		SetUpTimeout:    UsbipModulesLoadedTimeout,
		TearDownTimeout: UsbipModulesLoadedTimeout,
		PreTestTimeout:  UsbipModulesLoadedTimeout,
		PostTestTimeout: UsbipModulesLoadedTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "usbipServer",
		Desc:            "A running USBIP server for device emulation",
		Contacts:        []string{
			"chromeos-engprod-syd@google.com",
			"mattlui@google.com",
			"alvinjia@google.com",
		},
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
	modulesCleanup cleanupFn
	tcpdumpCleanup cleanupFn
}

// SetUp loads the required kernel modules.
func (l *LoadModuleFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if modulesCleanup, err := AddUSBIPModules(ctx); err != nil {
		s.Error("Failed to install usbip kernel modules: ", err)
	} else {
		l.modulesCleanup = modulesCleanup
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
	if tcpdumpCleanup, err := StartLocalTCPDump(ctx, s.OutDir()); err != nil {
		s.Error("Failed to start tcp tracing: ", err)
	} else {
		l.tcpdumpCleanup = tcpdumpCleanup
	}
}

// PostTest stops traffic capture.
func (l *LoadModuleFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := l.tcpdumpCleanup(ctx); err != nil {
		s.Error("Failed to stop tcp tracing: ", err)
	}
}

// TearDown unloads the required kernel modules.
func (l *LoadModuleFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := l.modulesCleanup(ctx); err != nil {
		s.Error("Failed to remove usbip kernel modules: ", err)
	}
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
		s.Fatal("Error while serving requests: ", err)
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
