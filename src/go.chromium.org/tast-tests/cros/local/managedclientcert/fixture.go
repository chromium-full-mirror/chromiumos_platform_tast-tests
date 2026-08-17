// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package managedclientcert

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
)

// LoggedInFixture is the fixture for the network.ClientCertificate test. It
// requests a GAC-managed account with the client certificate provisioning policy
// enabled from the gcac_cert_provisioning TAPE pool and logs into Chrome via GAIA.
const LoggedInFixture = "managedClientCertLoggedIn"

// NoPolicyLoggedInFixture is the fixture for the network.ClientCertificateNoPolicy
// test. It requests a default GAC-managed account without the client certificate
// provisioning policy enabled from the default_managed TAPE pool and logs into
// Chrome via GAIA.
const NoPolicyLoggedInFixture = "managedClientCertNoPolicyLoggedIn"

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: LoggedInFixture,
		Desc: "Requests a GAC-managed account from TAPE (gcac_cert_provisioning pool) and logs into Chrome via GAIA for managed client certificate tests",
		Contacts: []string{
			"cbe-cep-eng@google.com",         // Team
			"vishwa.kalubowila@codimite.com", // Test author
			"seblalancette@chromium.org",     // Test owner
		},
		BugComponent: "b:1000044",
		Parent:       fixture.TAPERemoteBase,
		// Leasing a TAPE account can take up to tape's request timeout (5 minutes)
		// on top of the GAIA login.
		SetUpTimeout:    chrome.GAIALoginTimeout + 5*time.Minute,
		PostTestTimeout: time.Minute,
		TearDownTimeout: chrome.ResetTimeout + time.Minute,
		Impl:            &fixtureImpl{poolID: tape.GCACCertProvisioning},
	})

	testing.AddFixture(&testing.Fixture{
		Name: NoPolicyLoggedInFixture,
		Desc: "Requests a default GAC-managed account from TAPE (default_managed pool) and logs into Chrome via GAIA for managed client certificate negative tests",
		Contacts: []string{
			"cbe-cep-eng@google.com",         // Team
			"vishwa.kalubowila@codimite.com", // Test author
			"seblalancette@chromium.org",     // Test owner
		},
		BugComponent: "b:1000044",
		Parent:       fixture.TAPERemoteBase,
		// Leasing a TAPE account can take up to tape's request timeout (5 minutes)
		// on top of the GAIA login.
		SetUpTimeout:    chrome.GAIALoginTimeout + 5*time.Minute,
		PostTestTimeout: time.Minute,
		TearDownTimeout: chrome.ResetTimeout + time.Minute,
		Impl:            &fixtureImpl{poolID: tape.DefaultManaged},
	})
}

// FixtValue is the value exposed to tests via s.FixtValue(). It carries the
// GAIA-logged-in Chrome, its test API connection, and the account username that
// the Chaps user-token check needs.
type FixtValue struct {
	cr       *chrome.Chrome
	tconn    *chrome.TestConn
	username string
}

// Chrome returns the GAIA-logged-in Chrome. It implements chrome.HasChrome.
func (v FixtValue) Chrome() *chrome.Chrome { return v.cr }

// TestAPIConn returns the test API connection for the logged-in Chrome.
func (v FixtValue) TestAPIConn() *chrome.TestConn { return v.tconn }

// Username returns the username of the account Chrome logged in with.
func (v FixtValue) Username() string { return v.username }

type fixtureImpl struct {
	poolID         string
	accountManager *tape.OwnedTestAccountManager
	value          FixtValue
}

func (f *fixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	manager, account, err := tape.NewOwnedTestAccountManager(ctx, []byte{}, false /*lock*/, tape.WithTimeout(5*60), tape.WithPoolID(f.poolID))
	if err != nil {
		s.Fatal("Failed to request an account: ", err)
	}
	f.accountManager = manager

	// TearDown does not run when SetUp fails, so release resources here.
	setUpOK := false
	defer func() {
		if setUpOK {
			return
		}
		if err := manager.CleanUp(ctx); err != nil {
			s.Log("Failed to release the account during setup cleanup: ", err)
		}
		f.accountManager = nil
	}()

	cr, err := chrome.New(ctx,
		chrome.GAIALogin(chrome.Creds{User: account.Username, Pass: account.Password}),
		chrome.ProdPolicy(),
		chrome.ExtraArgs("--allow-insecure-localhost"),
	)
	if err != nil {
		s.Fatal("Failed to log in with the provided user: ", err)
	}
	defer func() {
		if setUpOK {
			return
		}
		if err := cr.Close(ctx); err != nil {
			s.Log("Failed to close Chrome during setup cleanup: ", err)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a test API connection: ", err)
	}

	s.Logf("Successfully logged in as %s", account.Username)

	f.value = FixtValue{cr: cr, tconn: tconn, username: account.Username}
	setUpOK = true
	return f.value
}

func (f *fixtureImpl) Reset(ctx context.Context) error { return nil }

func (f *fixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, f.value.tconn)
}

func (f *fixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.value.cr != nil {
		if err := f.value.cr.Close(ctx); err != nil {
			s.Log("Failed to close Chrome: ", err)
		}
		f.value.cr = nil
	}
	if f.accountManager != nil {
		if err := f.accountManager.CleanUp(ctx); err != nil {
			s.Log("Failed to release the account: ", err)
		}
		f.accountManager = nil
	}
}
