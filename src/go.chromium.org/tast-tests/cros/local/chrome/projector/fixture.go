// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package projector provides Projector login functions.
package projector

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// resetTimeout is the timeout duration of trying to reset the current fixture.
const resetTimeout = 30 * time.Second

// chromeFlags are the feature flags that need to be enabled for the Projector app.
var chromeFlags = chrome.EnableFeatures("Projector, ProjectorAppDebug, ProjectorAnnotator, ProjectorTutorialVideoView, ProjectorLocalPlayback")

// NewProjectorFixture creates a new implementation of the Projector fixture.
func NewProjectorFixture(fOpts chrome.OptionsCallback) testing.FixtureImpl {
	return &projectorFixture{fOpts: fOpts}
}

// GetProjectorApp gets the installed Projector app.
func GetProjectorApp(ctx context.Context, tconn *chrome.TestConn) (*apps.App, error) {
	var appID *apps.App = nil
	err := testing.Poll(ctx, func(ctx context.Context) error {
		if installed, err := ash.ChromeAppInstalled(ctx, tconn, apps.Projector.ID); err != nil {
			return testing.PollBreak(err)
		} else if installed {
			appID = &apps.Projector
			return nil
		}

		if installed, err := ash.ChromeAppInstalled(ctx, tconn, apps.ProjectorV2.ID); err != nil {
			return testing.PollBreak(err)
		} else if installed {
			appID = &apps.ProjectorV2
			return nil
		}

		return errors.New("failed to wait for Projector installed")
	}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 300 * time.Millisecond})

	return appID, err
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         "projectorLogin",
		Desc:         "Regular user login with Projector feature flag enabled",
		Contacts:     []string{"cros-projector+tast@google.com", "xiqiruan@chromium.org"},
		BugComponent: "b:1080013", // ChromeOS Server Projects > Enterprise Management > Edu Features > Projector
		Impl: NewProjectorFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chromeFlags,
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			}, nil
		}),
		Vars: []string{
			"ui.gaiaPoolDefault",
		},
		SetUpTimeout:    chrome.GAIALoginTimeout + time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosProjectorLogin",
		Desc:         "Regular user login to lacros with Projector feature flag enabled",
		Contacts:     []string{"hyungtaekim@chromium.org", "cros-projector@google.com"},
		BugComponent: "b:1088267",
		Impl: NewProjectorFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chromeFlags,
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			)).Opts()
		}),
		Vars: []string{
			"ui.gaiaPoolDefault",
		},
		SetUpTimeout:    chrome.GAIALoginTimeout + time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	// Similar to the familyLinkUnicornLogin fixture, but uses a
	// different test account and isolates sessions for Projector
	// tests.
	testing.AddFixture(&testing.Fixture{
		Name:         "projectorUnicornLogin",
		Desc:         "Supervised Family Link user login with Unicorn account and fakeDMS policy setup for Projector tests",
		Contacts:     []string{"cros-projector+tast@google.com", "xiqiruan@chromium.org"},
		BugComponent: "b:1080013", // ChromeOS Server Projects > Enterprise Management > Edu Features > Projector
		Impl: NewProjectorFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chromeFlags,
				chrome.GAIALogin(chrome.Creds{
					User:       s.RequiredVar("projector.childEmail"),
					Pass:       s.RequiredVar("projector.childPassword"),
					ParentUser: s.RequiredVar("projector.parentEmail"),
					ParentPass: s.RequiredVar("projector.parentPassword"),
				}),
			}, nil
		}),
		Vars: []string{
			"projector.childEmail",
			"projector.childPassword",
			"projector.parentEmail",
			"projector.parentPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Parent:          fixture.PersistentProjectorChild,
	})

	// Managed user login requires fakeDMS to work.
	testing.AddFixture(&testing.Fixture{
		Name:         "projectorEduLogin",
		Desc:         "Managed user login with fakeDMS setup for Projector tests",
		Contacts:     []string{"cros-projector+tast@google.com", "xiqiruan@chromium.org"},
		BugComponent: "b:1080013", // ChromeOS Server Projects > Enterprise Management > Edu Features > Projector
		Impl: NewProjectorFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chromeFlags,
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			}, nil
		}),
		Vars: []string{
			"ui.gaiaPoolDefault",
		},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Parent:          fixture.FakeDMS,
	})
}

type projectorFixture struct {
	cr    *chrome.Chrome
	fOpts chrome.OptionsCallback
	fdms  *fakedms.FakeDMS
}

// HasApp interface provides access to the installed projector app.
type HasApp interface {
	App() *apps.App
}

// FixtData holds information made available to tests that specify this Fixture.
type FixtData struct {
	// Chrome is the running chrome instance.
	chrome *chrome.Chrome
	// TestConn is a connection to the test extension.
	testConn *chrome.TestConn
	// FakeDMS is the running DMS server if any.
	fakeDMS *fakedms.FakeDMS
	// The installed Projector app information.
	appRef *apps.App
}

// Chrome implements the HasChrome interface.
func (f FixtData) Chrome() *chrome.Chrome {
	if f.chrome == nil {
		panic("Chrome is called with nil chrome instance")
	}
	return f.chrome
}

// TestConn implements the HasTestConn interface.
func (f FixtData) TestConn() *chrome.TestConn {
	if f.testConn == nil {
		panic("TestConn is called with nil testConn instance")
	}
	return f.testConn
}

// FakeDMS implements the HasFakeDMS interface.
func (f FixtData) FakeDMS() *fakedms.FakeDMS {
	if f.fakeDMS == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return f.fakeDMS
}

// App implements the HasApp interface.
func (f FixtData) App() *apps.App {
	if f.appRef == nil {
		panic("Projector App has not been set.")
	}
	return f.appRef
}

// Check at compile-time that FixtData implements the appropriate interfaces.
var _ chrome.HasChrome = FixtData{}
var _ familylink.HasTestConn = FixtData{}
var _ fakedms.HasFakeDMS = FixtData{}
var _ HasApp = FixtData{}

func (f *projectorFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	opts, err := f.fOpts(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain Chrome options: ", err)
	}

	// Checks whether the current fixture has a FakeDMS parent fixture.
	fdms, isPolicyTest := s.ParentValue().(*fakedms.FakeDMS)
	if isPolicyTest {
		if err := fdms.Ping(ctx); err != nil {
			s.Fatal("Failed to ping FakeDMS: ", err)
		}

		opts = append(opts, chrome.DMSPolicy(fdms.URL))
		opts = append(opts, chrome.DisablePolicyKeyVerification())
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	if isPolicyTest {
		if err := policyutil.RefreshChromePolicies(ctx, cr); err != nil {
			s.Fatal("Failed to serve policies: ", err)
		}
	}

	f.cr = cr
	f.fdms = fdms

	// SWA installation is not guaranteed during startup.
	// Wait for installation finished before starting test.
	s.Log("Wait for Screencast app to be installed")
	app, err := GetProjectorApp(ctx, tconn)
	if err != nil {
		s.Fatal("Something failed: ", err)
	}

	// Lock chrome after all Setup is complete so we don't block other fixtures.
	chrome.Lock()
	return &FixtData{
		chrome:   cr,
		testConn: tconn,
		fakeDMS:  fdms,
		appRef:   app,
	}
}

func (f *projectorFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}

func (f *projectorFixture) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}

	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}

	return nil
}

func (f *projectorFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *projectorFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
