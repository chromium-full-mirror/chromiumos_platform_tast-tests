// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"chromiumos/tast/common/crypto/certificate"
	"chromiumos/tast/common/pkcs11/netcertstore"
	"chromiumos/tast/local/bundles/cros/network/shill"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/hwsec"
	"chromiumos/tast/local/logsaver"
	"chromiumos/tast/local/network"
	"chromiumos/tast/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const certOpTimeout = 30 * time.Second

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnv",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    shill.ResetShillTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    shill.ResetShillTimeout + 5*time.Second,
		TearDownTimeout: shill.ResetShillTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: false, useCr: false},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCerts",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill and installing certs",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    shill.ResetShillTimeout + certOpTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    shill.ResetShillTimeout + 5*time.Second,
		TearDownTimeout: shill.ResetShillTimeout + certOpTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, useCr: false},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCertsAndChromeLoggedIn",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill, installing certs, and starting Chrome session",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		SetUpTimeout:    shill.ResetShillTimeout + certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    shill.ResetShillTimeout + chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: shill.ResetShillTimeout + certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, useCr: true},
	})
}

func resetShillWithLockingHook(ctx context.Context) error {
	// We lose connectivity along the way here, and if that races with the
	// recover_duts network-recovery hooks, it may interrupt us. Lock the hook
	// before shill restarted.
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lock check network hook")
	}
	defer unlock()

	if errs := shill.ResetShill(ctx); len(errs) != 0 {
		for _, err := range errs {
			testing.ContextLog(ctx, "ResetShill error: ", err)
		}
		return errors.Wrap(errs[0], "failed to reset shill")
	}

	return nil
}

// vpnFixture is a fixture to prepare environment that can be used to test VPN
// connections. Particularly, this fixture does the followings:
//   - Reset shill in SetUp and TearDown, to make sure we have a clean shill profile.
//   - Prepare the cert store and install user certificate (and server CA certificate
//     if Chrome is required).
//   - Start a new Chrome session if required.
//
// When a test failed, to ensure we have a clean setup, shill will be reset if
// there is no Chrome, and a full restart of this fixture will happen if there is Chrome.
type vpnFixture struct {
	hasError  bool // if the previous test has error
	useCert   bool // if we need to install certs
	useCr     bool // if Chrome is needed
	cr        *chrome.Chrome
	certStore *netcertstore.Store
	logMarker *logsaver.Marker // to store fixture and per-test log
}

// FixtureEnv wraps the variables created by the fixture and used in the tests.
type FixtureEnv struct {
	Cr       *chrome.Chrome
	CertVals CertVals
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

	if err := resetShillWithLockingHook(ctx); err != nil {
		s.Fatal("Failed to reset shill: ", err)
	}

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
		if !f.useCert {
			s.Fatal("Cert and Chrome should be enabled together")
		}

		// Install CA cert to TPM. Since CA certs are stored as raw strings in
		// shill's profile, this is only required when Chrome is involved.
		if _, err := f.certStore.InstallCertKeyPair(ctx, "", certificate.TestCert1().CACred.Cert); err != nil {
			s.Fatal("Failed to install CA cert: ", err)
		}

		cred := chrome.Creds{User: netcertstore.TestUsername, Pass: netcertstore.TestPassword}
		cr, err := chrome.New(
			ctx,
			chrome.KeepState(),     // to avoid resetting TPM
			chrome.FakeLogin(cred), // to use the same user as certs are installed for
		)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		f.cr = cr
	}

	if err := f.stopLogSaver(ctx, "net.setup.log"); err != nil {
		s.Error("Failed to stop log saver: ", err)
	}

	return FixtureEnv{f.cr, certVals}
}

func (f *vpnFixture) Reset(ctx context.Context) error {
	// When there is a failure and no Chrome, we only need to reset shill.
	if !f.useCr && f.hasError {
		f.hasError = false
		testing.ContextLog(ctx, "Test failed, resetting shill")
		if err := resetShillWithLockingHook(ctx); err != nil {
			return errors.Wrap(err, "failed to reset shill")
		}
		return nil
	}

	if !f.useCr {
		return nil
	}

	// We need to reset shill when the test failed, and thus it will invalidate
	// the shill profile known to Chrome. Since there seems to be no reliable way
	// to check that Chrome gets new profile, we'll fully restart of this fixture,
	// which is triggered by reporting error here.
	if f.hasError {
		f.hasError = false
		return errors.New("last test failed, triggering a full reset")
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

	// Restart ui so that cryptohome unmounts all user mounts before shill is
	// restarted so that shill does not keep the mounts open perpetually.
	// TODO(b/205726835): Remove once the mount propagation for shill is fixed.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Error("Failed to restart ui: ", err)
	}

	if err := resetShillWithLockingHook(ctx); err != nil {
		s.Error("Failed to reset shill in TearDown: ", err)
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
