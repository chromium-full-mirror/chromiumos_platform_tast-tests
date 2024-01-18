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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	certManager "go.chromium.org/tast-tests/cros/local/networkui/certificate"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const certOpTimeout = 2 * time.Minute

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnv",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent:    "b:1493959",
		SetUpTimeout:    5 * time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
		Impl:            &vpnFixture{useCert: false, crMode: notUsed},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCerts",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states and installing certs (via `netcertstore`)",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent:    "b:1493959",
		SetUpTimeout:    certOpTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: certOpTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, crMode: notUsed},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCertsAndChromeLoggedIn",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states, installing certs (via UI), and starting Chrome session",
		Contacts: []string{
			"jiejiang@google.com",        // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent:    "b:1493959",
		SetUpTimeout:    certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, crMode: loggedInDeviceOwner},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithArcBooted",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states, starting Chrome session, and also booting ARC",
		Contacts: []string{
			"cassiewang@google.com",      // fixture maintainer
			"cros-networking@google.com", // platform networking team
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent:    "b:1493959",
		SetUpTimeout:    chrome.LoginTimeout + 5*time.Second + arc.BootTimeout,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: 5 * time.Second,
		Impl:            &vpnFixture{useCert: false, crMode: loggedInDeviceOwner, useARC: true},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCertsAndNonDeviceOwnerLoggedIn",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states, installing certs (via UI), and starting Chrome session for a non-device owner",
		Contacts: []string{
			"alfredyu@cienet.com",                              // primary fixture maintainer
			"chromeos-connectivity-cienet-external@google.com", // external automation team
			"jiejiang@google.com",                              // secondary fixture maintainer
			"cros-networking@google.com",                       // platform networking team
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent:    "b:1493959",
		SetUpTimeout:    certOpTimeout + 2*chrome.LoginTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: certOpTimeout + 2*chrome.LoginTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, crMode: loggedInNonDeviceOwner},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "vpnEnvWithCertsAndGuestLoggedIn",
		Desc: "A fixture that sets up the environment for VPN connections, including resetting shill states, installing certs (via UI), and starting Chrome session for a guest user",
		Contacts: []string{
			"alfredyu@cienet.com",                              // primary fixture maintainer
			"chromeos-connectivity-cienet-external@google.com", // external automation team
			"jiejiang@google.com",                              // secondary fixture maintainer
			"cros-networking@google.com",                       // platform networking team
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent:    "b:1493959",
		SetUpTimeout:    certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		PostTestTimeout: charonExitTimeout + 5*time.Second,
		ResetTimeout:    chrome.ResetTimeout + 5*time.Second,
		TearDownTimeout: certOpTimeout + chrome.LoginTimeout + 5*time.Second,
		Impl:            &vpnFixture{useCert: true, crMode: loggedInGuest},
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

	if err := virtualnet.ResetEthernetProperties(ctx, m); err != nil {
		logErr(err)
	}
}

type crMode int

const (
	notUsed crMode = iota
	loggedInDeviceOwner
	loggedInNonDeviceOwner
	loggedInGuest
)

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
	hasError     bool // if the previous test has error
	useCert      bool // if we need to install certs
	useARC       bool // if ARC is needed
	crMode       crMode
	cr           *chrome.Chrome
	tconn        *chrome.TestConn
	logMarker    *logsaver.Marker // to store fixture and per-test log
	a            *arc.ARC
	certsManager certsManager
}

// FixtureEnv wraps the variables created by the fixture and used in the tests.
type FixtureEnv struct {
	Cr       *chrome.Chrome
	CertVals *CertVals
	ARC      *arc.ARC
}

// Chrome implements the HasChrome interface.
func (f FixtureEnv) Chrome() *chrome.Chrome {
	if f.Cr == nil {
		panic("Chrome is called with nil chrome instance")
	}
	return f.Cr
}

// CertVals contains the required values to setup a cert-based VPN service.
type CertVals struct {
	// This is the credentials of the set of certificates.
	Credentials certificate.CertStore
	// This is the information of where the the certificates are installed.
	// Note that it's only available when the `netcertstore` is used.
	Store *tpmStore
}

type tpmStore struct {
	id, slot, pin string
}

// NewCertVals returns a new CertVals initialized.
// id is the ID to the object when certificates inserted into the user token.
// userToken is the PKCS#11 data related to the user token.
func NewCertVals(cert certificate.CertStore, userToken netcertstore.Token, id string) *CertVals {
	return &CertVals{
		Credentials: cert,
		Store: &tpmStore{
			id:   id,
			slot: fmt.Sprintf("%d", userToken.Slot),
			pin:  userToken.Pin,
		},
	}
}

func installUserCert(ctx context.Context, certCreds certificate.CertStore, certStore *netcertstore.Store) (*CertVals, error) {
	clientCred := certCreds.ClientCred
	id, err := certStore.InstallCertKeyPair(ctx, clientCred.PrivateKey, clientCred.Cert)
	return NewCertVals(certCreds, certStore.UserToken, id), err
}

func (f *vpnFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := f.startLogSaver(ctx); err != nil {
		s.Error("Failed to start log saver: ", err)
	}

	resetShillVPNState(ctx)

	defaultCred := chrome.Creds{User: "testuser@gmail.com", Pass: "testpass"}
	defaultCerts := certificate.TestCert1()

	var chromeOpts []chrome.Option
	switch f.crMode {
	case notUsed:
		if f.useCert {
			f.certsManager = newCertsManagerNonUI(defaultCerts)
		}
	case loggedInNonDeviceOwner:
		// Creating a user profile of the device owner so that
		// the next user will be the non-device owner.
		deviceOwnerCred := chrome.Creds{User: "deviceOwner_" + defaultCred.User, Pass: defaultCred.Pass}
		if err := userutil.CreateDeviceOwner(ctx, deviceOwnerCred.User, deviceOwnerCred.Pass); err != nil {
			s.Fatal("Failed to create device owner: ", err)
		}
		chromeOpts = append(chromeOpts, chrome.KeepState())
		fallthrough
	case loggedInDeviceOwner:
		chromeOpts = append(chromeOpts, chrome.FakeLogin(defaultCred))
		if f.useARC {
			chromeOpts = append(chromeOpts, chrome.ARCEnabled())
		}
		if f.useCert {
			f.certsManager = newCertsManagerUI(defaultCerts, certManager.TypeImportAndBind)
		}
	case loggedInGuest:
		chromeOpts = append(chromeOpts, chrome.GuestLogin())
		if f.useCert {
			f.certsManager = newCertsManagerUI(defaultCerts, certManager.TypeImport) // Guests are not supposed to bind the certificates.
		}
	}

	if chromeOpts != nil {
		cr, err := chrome.New(ctx, chromeOpts...)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		f.cr = cr

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}
		f.tconn = tconn
	}

	if f.useARC {
		a, err := arc.NewWithTimeout(ctx, s.OutDir(), arc.BootTimeout, f.cr.NormalizedUser())
		if err != nil {
			s.Error("Failed to start ARC: ", err)
		}
		f.a = a
	}

	var certVals *CertVals
	if f.certsManager != nil {
		vals, err := f.certsManager.install(ctx, f.cr, f.tconn)
		if err != nil {
			s.Fatal("Failed to import certificates: ", err)
		}
		certVals = vals
	}

	if err := f.stopLogSaver(ctx, "net.setup.log"); err != nil {
		s.Error("Failed to stop log saver: ", err)
	}

	return FixtureEnv{f.cr, certVals, f.a}
}

func (f *vpnFixture) Reset(ctx context.Context) error {
	resetShillVPNState(ctx)
	if f.cr == nil {
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

	if f.useARC {
		if err := f.a.ResetOutDir(ctx, s.OutDir()); err != nil {
			s.Error("Failed to to reset outDir field of ARC object: ", err)
		}
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

	if f.useARC {
		if err := f.a.SaveLogFiles(ctx); err != nil {
			s.Error("Failed to to save ARC-related log files: ", err)
		}
	}
}

func (f *vpnFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.startLogSaver(ctx); err != nil {
		s.Error("Failed to start log saver: ", err)
	}

	if f.certsManager != nil {
		if err := f.certsManager.delete(ctx); err != nil {
			s.Error("Failed to delete certificates: ", err)
		}
		f.certsManager = nil
	}

	if f.cr != nil {
		if err := f.cr.Close(ctx); err != nil {
			s.Log("Failed to close Chrome connection: ", err)
		}
		f.cr = nil
	}

	if f.a != nil {
		if err := f.a.Close(ctx); err != nil {
			s.Error("Failed to close ARC: ", err)
		}
		f.a = nil
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

type certsManager interface {
	install(context.Context, *chrome.Chrome, *chrome.TestConn) (*CertVals, error)
	delete(context.Context) error
}

type certsManagerNonUI struct {
	certStore *netcertstore.Store
	certs     certificate.CertStore
}

func newCertsManagerNonUI(certs certificate.CertStore) certsManager {
	return &certsManagerNonUI{certs: certs}
}

func (c *certsManagerNonUI) install(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*CertVals, error) {
	runner := hwsec.NewCmdRunner()
	certStore, err := netcertstore.CreateStore(ctx, runner)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cert store")
	}
	c.certStore = certStore

	certVals, err := installUserCert(ctx, c.certs, c.certStore)
	if err != nil {
		return nil, errors.Wrap(err, "failed to install cert")
	}

	return certVals, nil
}

func (c *certsManagerNonUI) delete(ctx context.Context) error {
	if c.certStore != nil {
		if err := c.certStore.Cleanup(ctx); err != nil {
			return errors.Wrap(err, "failed to clean up cert store")
		}
		c.certStore = nil
	}
	return nil
}

type certsManagerUI struct {
	certs      certificate.CertStore
	importType certManager.ImportType

	cr    *chrome.Chrome
	tconn *chrome.TestConn
}

func newCertsManagerUI(certs certificate.CertStore, importType certManager.ImportType) certsManager {
	return &certsManagerUI{
		certs:      certs,
		importType: importType,
	}
}

func (c *certsManagerUI) install(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*CertVals, error) {
	c.cr = cr
	c.tconn = tconn
	if c.cr == nil || c.tconn == nil {
		return nil, errors.New("failed to import certificates by the certificate manager: Chrome not yet started")
	}
	// TODO(crbug/1366609): Support Lacros once the issue has been resolved.
	browserType := browser.TypeAsh
	return &CertVals{Credentials: c.certs}, certManager.CreateCertAndImport(
		ctx,
		c.cr,
		c.tconn,
		browserType,
		c.certs,
		c.importType,
		"", /* password */
		0,  /* trust settings for the CA certificate */
	)
}

func (c *certsManagerUI) delete(ctx context.Context) error {
	if c.cr == nil || c.tconn == nil {
		return errors.New("failed to delete certificates by the certificate manager: Chrome not yet started")
	}
	return certManager.DeleteCert(
		c.tconn,
		c.cr.Browser(),
		certManager.NewCertData(c.certs, certManager.TypeClient),
		certManager.NewCertData(c.certs, certManager.TypeCA),
	)(ctx)
}
