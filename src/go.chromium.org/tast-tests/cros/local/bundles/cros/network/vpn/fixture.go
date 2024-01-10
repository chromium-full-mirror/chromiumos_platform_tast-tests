// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/shill"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const certOpTimeout = 30 * time.Second

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnv",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    5 * time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
		Impl:            &vpnFixture{useCert: false, useCr: false},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCerts",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states and installing certs",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    certOpTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: certOpTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, useCr: false},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCertsAndChromeLoggedIn",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states, installing certs, and starting Chrome session",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, useCr: true},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithArcBooted",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states, starting Chrome session, and also booting ARC",
		Contacts: []string{
			"cassiewang@google.com",      // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    chrome.LoginTimeout + 5*time.Second + arc.BootTimeout,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: 5 * time.Second,
		Impl:            &vpnFixture{useCert: false, useCr: true, useARC: true},
	})
}

// resetShillVPNState resets the VPN-related states in shill in a best-effort
// way. Note that resetting all the shill profiles and then restarting shill
// should be the most ideal way, but in practice we found that restarting shill
// may cause tests flaky due to ssh connection lost (b/228272750).
func resetShillVPNState(ctx context.Context) {
	logErr := func(err error) {
		testing.ContextLog(ctx, "Failed to reset VPN state: ", err)
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		logErr(errors.Wrap(err, "failed to connect to shill Manager"))
		return
	}

	// Remove all the existing VPN services in shill.
	rmVPNSvcs := func() error {
		svcs, _, err := m.ServicesByTechnology(ctx, shill.TechnologyVPN)
		if err != nil {
			return errors.Wrap(err, "failed to get VPN services")
		}
		for _, svc := range svcs {
			testing.ContextLog(ctx, "Removing VPN service: ", svc)
			if err := svc.Remove(ctx); err != nil {
				return errors.Wrapf(err, "failed to remove VPN service %s", svc.ObjectPath())
			}
		}
		return nil
	}
	if err := rmVPNSvcs(); err != nil {
		logErr(err)
	}

	if err := virtualnet.ResetEthernetEphemeralPriority(ctx, m); err != nil {
		logErr(err)
	}
}

// vpnFixture is a fixture to prepare environment that can be used to test VPN
// connections. Particularly, this fixture does the followings:
//   - Reset shill in SetUp and TearDown, to make sure we have a clean shill profile.
//   - Prepare the cert store and install user certificate (and server CA certificate
//     if Chrome is required).
//   - Start a new Chrome session if required.
//   - Boot ARC if ARC is required.
//
// When a test failed, to ensure we have a clean setup, shill will be reset if
// there is no Chrome, and a full restart of this fixture will happen if there is Chrome.
type vpnFixture struct {
	hasError  bool // if the previous test has error
	useCert   bool // if we need to install certs
	useCr     bool // if Chrome is needed
	useARC    bool // if ARC is needed
	cr        *chrome.Chrome
	certStore *netcertstore.Store
	logMarker *logsaver.Marker // to store fixture and per-test log
	a         *arc.ARC
}

// FixtureEnv wraps the variables created by the fixture and used in the tests.
type FixtureEnv struct {
	Cr       *chrome.Chrome
	CertVals CertVals
	ARC      *arc.ARC
}

// CertVals contains the required values to setup a cert-based VPN service.
type CertVals struct {
	id   string
	slot string
	pin  string
}

func installUserCert(ctx context.Context, certStore *netcertstore.Store) (CertVals, error) {
	slot := fmt.Sprintf("%d", certStore.UserToken.Slot)
	pin := certStore.UserToken.Pin
	clientCred := certificate.TestCert1().ClientCred
	id, err := certStore.InstallCertKeyPair(ctx, clientCred.PrivateKey, clientCred.Cert)
	return CertVals{id, slot, pin}, err
}

func (f *vpnFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := f.startLogSaver(ctx); err != nil {
		s.Error("Failed to start log saver: ", err)
	}

	resetShillVPNState(ctx)

	var certVals CertVals
	if f.useCert {
		runner := hwsec.NewCmdRunner()
		certStore, err := netcertstore.CreateStore(ctx, runner)
		if err != nil {
			s.Fatal("Failed to create cert store: ", err)
		}
		f.certStore = certStore

		certVals, err = installUserCert(ctx, f.certStore)
		if err != nil {
			s.Fatal("Failed to install cert: ", err)
		}
	}

	if f.useCr {
		// To avoid resetting TPM.
		chromeOpts := []chrome.Option{chrome.KeepState()}

		if f.useCert {
			// Install CA cert to TPM. Since CA certs are stored as raw strings in
			// shill's profile, this is only required when Chrome is involved.
			if _, err := f.certStore.InstallCertKeyPair(ctx, "", certificate.TestCert1().CACred.Cert); err != nil {
				s.Fatal("Failed to install CA cert: ", err)
			}

			cred := chrome.Creds{User: netcertstore.TestUsername, Pass: netcertstore.TestPassword}

			// To use the same user as certs are installed for.
			chromeOpts = append(chromeOpts, chrome.FakeLogin(cred))
		}
		if f.useARC {
			chromeOpts = append(chromeOpts, chrome.ARCEnabled())
		}

		cr, err := chrome.New(ctx, chromeOpts...)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		f.cr = cr
	}

	if f.useARC {
		a, err := arc.NewWithTimeout(ctx, s.OutDir(), arc.BootTimeout, f.cr.NormalizedUser())
		if err != nil {
			s.Error("Failed to start ARC: ", err)
		}
		f.a = a
	}

	if err := f.stopLogSaver(ctx, "net.setup.log"); err != nil {
		s.Error("Failed to stop log saver: ", err)
	}

	return FixtureEnv{f.cr, certVals, f.a}
}

func (f *vpnFixture) Reset(ctx context.Context) error {
	resetShillVPNState(ctx)
	if !f.useCr {
		return nil
	}
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed to reset existing Chrome session")
	}
	return nil
}

func (f *vpnFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.startLogSaver(ctx); err != nil {
		s.Error("Failed to start log saver: ", err)
	}
}

func (f *vpnFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := waitForCharonExitOrKill(ctx); err != nil {
		s.Error("Failed to wait for charon to stop: ", err)
	}
	f.hasError = s.HasError()
	if err := f.stopLogSaver(ctx, "net.log"); err != nil {
		s.Error("Failed to stop log saver: ", err)
	}
}

func (f *vpnFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.startLogSaver(ctx); err != nil {
		s.Error("Failed to start log saver: ", err)
	}

	if f.useCr {
		if err := f.cr.Close(ctx); err != nil {
			s.Log("Failed to close Chrome connection: ", err)
		}
		f.cr = nil
	}

	if f.useCert {
		if err := f.certStore.Cleanup(ctx); err != nil {
			s.Error("Failed to clean up cert store: ", err)
		}
	}

	if err := f.stopLogSaver(ctx, "net.teardown.log"); err != nil {
		s.Error("Failed to stop log saver: ", err)
	}
}

func (f *vpnFixture) startLogSaver(ctx context.Context) error {
	if f.logMarker != nil {
		testing.ContextLog(ctx, "A log marker is already created but not cleaned up")
		f.logMarker = nil
	}

	logMarker, err := logsaver.NewMarker("/var/log/net.log")
	if err != nil {
		return errors.Wrap(err, "failed to create log saver for net.log")
	}
	f.logMarker = logMarker
	return nil
}

func (f *vpnFixture) stopLogSaver(ctx context.Context, name string) error {
	if f.logMarker == nil {
		testing.ContextLog(ctx, "No available log saver")
		return nil
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get ContextOutDir")
	}
	if err := f.logMarker.Save(filepath.Join(outDir, name)); err != nil {
		return errors.Wrapf(err, "failed to store log to %s", name)
	}
	f.logMarker = nil
	return nil
}
