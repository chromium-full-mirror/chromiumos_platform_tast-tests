// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixtures

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	tapeFixturetotalRunTime = 30 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.ChromeTAPELoggedIn,
		Desc:            "Logged into a real managed user session",
		Contacts:        []string{"vsavu@google.com", "chromeos-commercial-remote-management@google.com"},
		Impl:            &tapeChromeFixture{},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeTAPEEnrolledLoggedIn,
		Desc:     "Logged into a real managed user session on an erolled device",
		Contacts: []string{"vsavu@google.com", "chromeos-commercial-remote-management@google.com"},
		Impl: &tapeChromeFixture{
			importUser: true,
		},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.TAPEEnrolled,
	})
}

// TAPEChromeFixtData is returned by the fixtures and used in tests
// by using interfaces HasChrome to get chrome.
type TAPEChromeFixtData struct {
	// chrome is a connection to an already-started Chrome instance that loads policies from DMServer.
	chrome *chrome.Chrome

	// Username, Password and RequestID belonged to the leased TAPE user.
	Username  string
	Password  string
	RequestID string
}

// Chrome implements the HasChrome interface.
func (t TAPEChromeFixtData) Chrome() *chrome.Chrome {
	if t.chrome == nil {
		panic("Chrome is called with nil chrome instance")
	}
	return t.chrome
}

type tapeChromeFixture struct {
	// importUser indicates the fixture should import the user from the parent fixture.
	importUser bool

	// chrome is a connection to an already-started Chrome instance that loads policies from DMServer.
	chrome *chrome.Chrome

	// username, password and requestID belonged to the leased TAPE user.
	username  string
	password  string
	requestID string

	// Marker for per-test log.
	logMarker *logsaver.Marker

	// clean stores if Chrome is clean after PostTest.
	// It is considered clean if it does not interfere with the next test, e.g. with a locked screen.
	clean bool

	// accountManager is used to clean up when leasing a user.
	accountManager *tape.OwnedTestAccountManager
}

func (t *tapeChromeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	ok := false

	if t.importUser {
		fixtData := fixture.TAPEEnrolledFixtData{}
		if err := s.ParentFillValue(&fixtData); err != nil {
			s.Fatal("Failed to deserialize remote fixture data: ", err)
		}

		t.username = fixtData.Username
		t.password = fixtData.Password
		t.requestID = fixtData.RequestID
	} else {
		tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
		if err != nil {
			s.Fatal("Failed to create tape client: ", err)
		}

		timeout := int32(tapeFixturetotalRunTime.Seconds())
		accountManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
		if err != nil {
			s.Fatal("Failed to create an account manager and lease an account: ", err)
		}
		defer func(ctx context.Context) {
			if !ok {
				accountManager.CleanUp(cleanupCtx)
			}
		}(cleanupCtx)

		t.accountManager = accountManager

		t.username = account.Username
		t.password = account.Password
		t.requestID = account.RequestID
	}

	cr, err := chrome.New(ctx,
		chrome.GAIALogin(chrome.Creds{User: t.username, Pass: t.password}),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}

	ok = true

	t.chrome = cr

	return &TAPEChromeFixtData{
		chrome:    t.chrome,
		Username:  t.username,
		Password:  t.password,
		RequestID: t.requestID,
	}
}

func (t *tapeChromeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if t.accountManager != nil {
		if err := t.accountManager.CleanUp(ctx); err != nil {
			s.Error("Failed to cleanup TAPE: ", err)
		}
	}

	chrome.Unlock()

	if t.chrome == nil {
		s.Fatal("Chrome not yet started")
	}

	if err := t.chrome.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}

	t.chrome = nil
}

func (t *tapeChromeFixture) Reset(ctx context.Context) error {
	// If Chrome not in a clean state, a failure here would invoke a TearDown and SetUp of
	// the fixture, ensuring a clean state for the next test.
	if !t.clean {
		return errors.New("failed to clean up Chrome after the last test")
	}

	// Check the connection to Chrome.
	if err := t.chrome.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}

	return nil
}

func (t *tapeChromeFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if t.logMarker != nil {
		s.Error("A log marker is already created but not cleaned up")
	}
	logMarker, err := logsaver.NewMarker(t.chrome.LogFilename())
	if err != nil {
		s.Error("Failed to start the log saver: ", err)
	} else {
		t.logMarker = logMarker
	}
}
func (t *tapeChromeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	t.clean = false

	if t.logMarker != nil {
		if err := t.logMarker.Save(filepath.Join(s.OutDir(), "chrome.log")); err != nil {
			s.Error("Failed to store per-test log data: ", err)
		}
		t.logMarker = nil
	}

	tconn, err := t.chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create TestAPI connection: ", err)
	}

	policies, err := policyutil.PoliciesFromDUT(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain policies from Chrome: ", err)
	}

	b, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		s.Fatal("Failed to marshal policies: ", err)
	}

	// The following checks are here in PostTest to associate any failures with the previous test.
	// A failure here generally means that something went wrong in the previous test,
	// or the test has insufficient cleanup.

	// Dump all policies as seen by Chrome to the tests OutDir.
	if err := os.WriteFile(filepath.Join(s.OutDir(), PolicyFileDump), b, 0644); err != nil {
		s.Error("Failed to dump policies to file: ", err)
	}

	// Reset Chrome state.
	if err := t.chrome.ResetState(ctx); err != nil {
		s.Fatal("Failed resetting existing Chrome session: ", err)
	}

	// Check that Chrome is not left with a locked screen.
	if st, err := lockscreen.GetState(ctx, tconn); err != nil {
		s.Fatal("Failed getting the lockscreen state: ", err)
	} else if st.Locked {
		s.Fatal("Unexpected lockscreen state after the test, the screen is locked")
	}

	// Check that no windows remain.
	if windows, err := ash.GetAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to get the windows: ", err)
	} else if len(windows) != 0 {
		s.Fatalf("Unexpected number of windows after the test; got %d, want 0", len(windows))
	}

	// Chrome should be in a good state to execute the next test.
	t.clean = true
}
