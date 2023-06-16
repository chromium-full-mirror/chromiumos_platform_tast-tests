// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture contains Telemetry Extension fixture.
package fixture

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// Fixture names.
const (
	TelemetryExtension                       = "telemetryExtension"
	TelemetryExtensionLacros                 = "telemetryExtensionLacros"
	TelemetryExtensionSkipOEMNameCheck       = "telemetryExtensionSkipOEMNameCheck"
	TelemetryExtensionSkipOEMNameCheckLacros = "telemetryExtensionSkipOEMNameCheckLacros"
)

const (
	cleanupTimeout = chrome.ResetTimeout + 20*time.Second

	crosHealthdJobName = "cros_healthd"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: TelemetryExtension,
		Desc: "Telemetry Extension fixture with running PWA and companion Telemetry Extension",
		Contacts: []string{
			"chromeos-oem-services@google.com", // Use team email for tickets.
			"bkersting@google.com",
			"lamzin@google.com",
		},
		Impl:            newTelemetryExtensionFixture(),
		SetUpTimeout:    chrome.LoginTimeout + 30*time.Second + cleanupTimeout,
		TearDownTimeout: cleanupTimeout,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
		Data:            extFiles(),
	})
	testing.AddFixture(&testing.Fixture{
		Name: TelemetryExtensionLacros,
		Desc: "Telemetry Extension fixture with running PWA and companion Telemetry Extension in Lacros browser",
		Contacts: []string{
			"chromeos-oem-services@google.com", // Use team email for tickets.
			"bkersting@google.com",
			"lamzin@google.com",
		},
		Impl:            newTelemetryExtensionFixture(lacros()),
		SetUpTimeout:    chrome.LoginTimeout + 30*time.Second + cleanupTimeout,
		TearDownTimeout: cleanupTimeout,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
		Data:            extFiles(),
	})
	testing.AddFixture(&testing.Fixture{
		Name: TelemetryExtensionSkipOEMNameCheck,
		Desc: "Telemetry Extension fixture with running PWA and companion Telemetry Extension on devices that are not officially supported yet",
		Contacts: []string{
			"chromeos-oem-services@google.com", // Use team email for tickets.
			"bkersting@google.com",
			"lamzin@google.com",
		},
		Impl:            newTelemetryExtensionFixture(skipOEMNameCheck()),
		SetUpTimeout:    chrome.LoginTimeout + 30*time.Second + cleanupTimeout,
		TearDownTimeout: cleanupTimeout,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
		Data:            extFiles(),
	})
	testing.AddFixture(&testing.Fixture{
		Name: TelemetryExtensionSkipOEMNameCheckLacros,
		Desc: "Telemetry Extension fixture with running PWA and companion Telemetry Extension in Lacros browser on devices that are not officially supported yet",
		Contacts: []string{
			"chromeos-oem-services@google.com", // Use team email for tickets.
			"bkersting@google.com",
			"lamzin@google.com",
		},
		Impl:            newTelemetryExtensionFixture(lacros(), skipOEMNameCheck()),
		SetUpTimeout:    chrome.LoginTimeout + 30*time.Second + cleanupTimeout,
		TearDownTimeout: cleanupTimeout,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
		Data:            extFiles(),
	})
}

func extFiles() []string {
	return []string{"manifest.json", "sw.js"}
}

type option func(*telemetryExtensionFixture)

func lacros() func(*telemetryExtensionFixture) {
	return func(f *telemetryExtensionFixture) {
		f.bt = browser.TypeLacros
	}
}

func skipOEMNameCheck() func(*telemetryExtensionFixture) {
	return func(f *telemetryExtensionFixture) {
		f.skipOEMNameCheck = true
	}
}

func newTelemetryExtensionFixture(opts ...option) *telemetryExtensionFixture {
	f := &telemetryExtensionFixture{}
	f.bt = browser.TypeAsh
	f.v.ExtID = "gogonhoemckpdpadfnjnpgbjpbjnodgc"

	for _, opt := range opts {
		opt(f)
	}
	return f
}

// telemetryExtensionFixture implements testing.FixtureImpl.
type telemetryExtensionFixture struct {
	bt               browser.Type
	skipOEMNameCheck bool

	dir     string
	cr      *chrome.Chrome
	br      *browser.Browser
	closeBr uiauto.Action

	healthdPID int

	v Value
}

// Value is a value exposed by fixture to tests.
type Value struct {
	ExtID string

	PwaConn *chrome.Conn
	ExtConn *chrome.Conn

	TConn *chrome.TestConn
}

func (f *telemetryExtensionFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()

	defer func(ctx context.Context) {
		if s.HasError() {
			f.TearDown(ctx, s)
		}
	}(cleanupCtx)

	if err := f.setupChromeForConsumers(ctx, s.DataPath); err != nil {
		s.Fatal("Failed to setup Chrome for consumers: ", err)
	}

	// GoBigSleepLint: TODO(b/287416180): Find fix for the
	// actual error instead of this workaround.
	// Add a timeout to wait until the owner is retrieved.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Unable to pause after Ash launch")
	}

	br, closeBr, err := browserfixt.SetUp(ctx, f.cr, f.bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	f.br = br
	f.closeBr = closeBr

	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connections: ", err)
	}
	f.v.TConn = tconn

	if err := f.setupConnectionToPWA(ctx); err != nil {
		s.Fatal("Failed to setup connection to PWA: ", err)
	}

	if err := f.setupConnectionToExtension(ctx); err != nil {
		s.Fatal("Failed to setup connection to Telemetry Extension: ", err)
	}

	if err := upstart.EnsureJobRunning(ctx, crosHealthdJobName); err != nil {
		s.Fatalf("Failed to start %s daemon", crosHealthdJobName)
	}

	return &f.v
}

func (f *telemetryExtensionFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.v.ExtConn != nil {
		if err := f.v.ExtConn.Close(); err != nil {
			s.Error("Failed to close connection to Telemetry Extension: ", err)
		}
		f.v.ExtConn = nil
	}

	if f.v.PwaConn != nil {
		if err := f.v.PwaConn.Close(); err != nil {
			s.Error("Failed to close connection to google.com: ", err)
		}
		f.v.PwaConn = nil
	}

	if f.br != nil {
		f.closeBr(ctx)

		f.br = nil
		f.closeBr = nil
	}

	if f.cr != nil {
		if err := f.cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome: ", err)
		}
		f.cr = nil
	}

	if f.dir != "" {
		if err := os.RemoveAll(f.dir); err != nil {
			s.Error("Failed to remove directory with Telemetry Extension: ", err)
		}
		f.dir = ""
	}
}

func (f *telemetryExtensionFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	_, _, pid, err := upstart.JobStatus(ctx, crosHealthdJobName)
	if err != nil {
		s.Fatalf("Unable to get %s PID: %s", crosHealthdJobName, err)
	}

	f.healthdPID = pid
}

func (f *telemetryExtensionFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	_, _, pid, err := upstart.JobStatus(ctx, crosHealthdJobName)
	if err != nil {
		s.Fatalf("Unable to get %s PID: %s", crosHealthdJobName, err)
	}

	if pid != f.healthdPID {
		s.Fatalf("%s PID changed: got %d, want %d", crosHealthdJobName, pid, f.healthdPID)
	}
}

func (f *telemetryExtensionFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *telemetryExtensionFixture) setupChromeForConsumers(ctx context.Context, dataPathFunc func(string) string) error {
	dir, err := ioutil.TempDir("", "telemetry_extension")
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory for TelemetryExtension")
	}
	f.dir = dir

	if err := os.Chown(dir, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		return errors.Wrap(err, "failed to chown TelemetryExtension dir")
	}

	for _, file := range extFiles() {
		if err := fsutil.CopyFile(dataPathFunc(file), filepath.Join(dir, file)); err != nil {
			return errors.Wrapf(err, "failed to copy %q file to %q", file, dir)
		}

		if err := os.Chown(filepath.Join(dir, file), int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
			return errors.Wrapf(err, "failed to chown %q", file)
		}
	}

	opts := []chrome.Option{
		chrome.EnableFeatures("TelemetryExtensionPendingApprovalApi"),
	}
	f.addSkipOEMNameCheckChromeArg(ctx, &opts)

	if f.bt == browser.TypeAsh {
		opts = append(opts, chrome.UnpackedExtension(dir))
	}
	if f.bt == browser.TypeLacros {
		extraOpts, err := lacrosfixt.NewConfig().Opts()
		if err != nil {
			return errors.Wrap(err, "failed to get lacros options")
		}
		opts = append(opts, extraOpts...)
		opts = append(opts, chrome.LacrosUnpackedExtension(dir))
		opts = append(opts, chrome.LacrosEnableFeatures("TelemetryExtensionPendingApprovalApi"))
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	f.cr = cr

	return nil
}

func (f *telemetryExtensionFixture) setupConnectionToPWA(ctx context.Context) error {
	pwaConn, err := f.br.NewConn(ctx, "https://googlechromelabs.github.io/cros-sample-telemetry-extension")
	if err != nil {
		return errors.Wrap(err, "failed to create connection to googlechromelabs.github.io")
	}
	f.v.PwaConn = pwaConn

	if err := chrome.AddTastLibrary(ctx, pwaConn); err != nil {
		return errors.Wrap(err, "failed to add Tast library to googlechromelabs.github.io")
	}
	return nil
}

func (f *telemetryExtensionFixture) setupConnectionToExtension(ctx context.Context) error {
	conn, err := f.br.NewConn(ctx, fmt.Sprintf("chrome-extension://%s/sw.js", f.v.ExtID))
	if err != nil {
		return errors.Wrap(err, "failed to create connection to Telemetry Extension")
	}
	f.v.ExtConn = conn

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		const js = `
			(function() {
				return document.querySelector("body > pre") !== null;
			})()
		`
		isExtensionInstalled := false
		if err := conn.Eval(ctx, js, &isExtensionInstalled); err != nil {
			return errors.Wrap(err, "failed to verify whether chrome.os.telemetry is defined")
		}

		if isExtensionInstalled {
			return nil
		}

		reloadButton := nodewith.Role(role.Button).Name("Reload").ClassName("ReloadButton")
		if err := uiauto.New(f.v.TConn).LeftClick(reloadButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click reload button")
		}

		return errors.New("chrome.os.telemetry is undefined")
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify that extension is installed")
	}

	if err := chrome.AddTastLibrary(ctx, conn); err != nil {
		return errors.Wrap(err, "failed to add Tast library to Telemetry Extension")
	}

	return nil
}

func (f *telemetryExtensionFixture) addSkipOEMNameCheckChromeArg(ctx context.Context, opts *[]chrome.Option) {
	if f.skipOEMNameCheck {
		*opts = append(*opts, chrome.ExtraArgs("--telemetry-extension-skip-manufacturer-check-for-testing"))
	}
}
