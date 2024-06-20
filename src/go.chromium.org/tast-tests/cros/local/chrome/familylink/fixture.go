// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink provides Family Link user login functions.
package familylink

import (
	"context"
	"fmt"
	"time"

	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// resetTimeout is the timeout duration of trying to reset the current fixture.
const resetTimeout = 30 * time.Second

// NewFamilyLinkFixture creates a new implementation of the Family Link fixture.
func NewFamilyLinkFixture(parentUser, parentPassword, childUser, childPassword string, isOwner bool, opts ...chrome.Option) testing.FixtureImpl {
	return &familyLinkFixture{
		opts:           opts,
		parentUser:     parentUser,
		parentPassword: parentPassword,
		childUser:      childUser,
		childPassword:  childPassword,
		isOwner:        isOwner,
		isLacros:       false,
	}
}

// NewFamilyLinkFixtureLacros creates a new implementation of the Family Link fixture for Lacros.
func NewFamilyLinkFixtureLacros(parentUser, parentPassword, childUser, childPassword string, isOwner bool, opts ...chrome.Option) testing.FixtureImpl {
	return &familyLinkFixture{
		opts:           opts,
		parentUser:     parentUser,
		parentPassword: parentPassword,
		childUser:      childUser,
		childPassword:  childPassword,
		isOwner:        isOwner,
		isLacros:       true,
	}
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornLogin",
		Desc: "Supervised Family Link user login with Unicorn account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.unicornEmail", "family.unicornPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornEmail",
			"family.unicornPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornLoginWithLacros",
		Desc: "Supervised Family Link user login with Unicorn account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixtureLacros("family.parentEmail", "family.parentPassword", "family.unicornEmail", "family.unicornPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornEmail",
			"family.unicornPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornWebAllowlistLogin",
		Desc: "This fixture logs in Unicorn account with 'Only allow approved sites' website filtering setting",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.unicornAllowlistEmail", "family.unicornAllowlistPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornAllowlistEmail",
			"family.unicornAllowlistPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornWebAllowlistLoginWithLacros",
		Desc: "This fixture enables LaCrOS and logs in Unicorn account with 'Only allow approved sites' website filtering setting",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixtureLacros("family.parentEmail", "family.parentPassword", "family.unicornAllowlistEmail", "family.unicornAllowlistPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornAllowlistEmail",
			"family.unicornAllowlistPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornLoginNonOwner",
		Desc: "Supervised Family Link user login with Unicorn account as second user on device",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.unicornEmail", "family.unicornPassword", false),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornEmail",
			"family.unicornPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornLoginNonOwnerWithLacros",
		Desc: "Supervised Family Link user login with Unicorn account as second user on device",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"hyungtaekim@chromium.org",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixtureLacros("family.parentEmail", "family.parentPassword", "family.unicornEmail", "family.unicornPassword", false),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornEmail",
			"family.unicornPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkGellerLogin",
		Desc: "Supervised Family Link user login with Geller account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.gellerEmail", "family.gellerPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.gellerEmail",
			"family.gellerPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkGellerLoginWithLacros",
		Desc: "Supervised Family Link user login with Geller account on Lacros",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"hyungtaekim@chromium.org",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixtureLacros("family.parentEmail", "family.parentPassword", "family.gellerEmail", "family.gellerPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.gellerEmail",
			"family.gellerPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornArcLogin",
		Desc: "Supervised Family Link user login with Unicorn account and ARC support",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent:    "b:1079167", // ChromeOS > Software > Family
		Impl:            NewFamilyLinkFixture(arcCommon.ParentAccountVarName, "", arcCommon.ChildAccountVarName, "", true, chrome.ARCSupported()),
		SetUpTimeout:    chrome.GAIALoginChildTimeout + arc.BootTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkGriffinLogin",
		Desc: "Supervised Family Link user login with Griffin account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.griffinEmail", "family.griffinPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.griffinEmail",
			"family.griffinPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkGriffinLoginWithLacros",
		Desc: "Supervised Family Link user login with Griffin account on Lacros",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixtureLacros("family.parentEmail", "family.parentPassword", "family.griffinEmail", "family.griffinPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.griffinEmail",
			"family.griffinPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkParentArcLogin",
		Desc: "Non-supervised Family Link user login with regular parent account and ARC support",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent:    "b:1079167", // ChromeOS > Software > Family
		Impl:            NewFamilyLinkFixture(arcCommon.ParentAccountVarName, "", "", "", true, chrome.ARCSupported(), chrome.ExtraArgs(arc.DisableSyncFlags()...)),
		SetUpTimeout:    chrome.GAIALoginTimeout + arc.BootTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornPolicyLogin",
		Desc: "Supervised Family Link user login with Unicorn account and policy setup",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.unicornEmail", "family.unicornPassword", true),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornEmail",
			"family.unicornPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Parent:          fixture.PersistentFamilyLink,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkUnicornArcPolicyLogin",
		Desc: "Supervised Family Link user login with Unicorn account and ARC support with fakeDMS setup",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent:    "b:1079167", // ChromeOS > Software > Family
		Impl:            NewFamilyLinkFixture(arcCommon.ParentAccountVarName, "", arcCommon.ChildAccountVarName, "", true, chrome.ARCSupported(), chrome.ExtraArgs(arc.DisableSyncFlags()...)),
		SetUpTimeout:    chrome.GAIALoginChildTimeout + arc.BootTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Parent:          fixture.PersistentFamilyLinkARC,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "familyLinkGellerArcPolicyLogin",
		Desc: "Supervised Family Link user login with Geller account and ARC support with fakeDMS setup",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"agawronska@chromium.org",
		},
		BugComponent: "b:1079167", // ChromeOS > Software > Family
		Impl:         NewFamilyLinkFixture("family.parentEmail", "family.parentPassword", "family.gellerEmail", "family.gellerPassword", true, chrome.ARCSupported(), chrome.ExtraArgs(arc.DisableSyncFlags()...)),
		Vars: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.gellerEmail",
			"family.gellerPassword",
		},
		SetUpTimeout:    chrome.GAIALoginChildTimeout + arc.BootTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Parent:          fixture.PersistentGellerARC,
	})
}

type familyLinkFixture struct {
	cr             *chrome.Chrome
	opts           []chrome.Option
	fdms           *fakedms.FakeDMS
	policyUser     string
	parentUser     string
	parentPassword string
	childUser      string
	childPassword  string
	isOwner        bool
	isLacros       bool
}

// FixtData holds information made available to tests that specify this Fixture.
type FixtData struct {
	// Chrome is the running chrome instance.
	chrome *chrome.Chrome

	// FakeDMS is the running DMS server if any.
	fakeDMS *fakedms.FakeDMS

	// TestConn is a connection to the test extension.
	testConn *chrome.TestConn

	// PolicyUser is the user account used in the policy blob.
	policyUser string
}

// Chrome implements the HasChrome interface.
func (f FixtData) Chrome() *chrome.Chrome {
	if f.chrome == nil {
		panic("Chrome is called with nil chrome instance")
	}
	return f.chrome
}

// HasTestConn is an interface for fixture values that contain a TestConn instance. It allows
// retrieval of the underlying TestConn object.
type HasTestConn interface {
	TestConn() *chrome.TestConn
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

// HasPolicyUser is an interface for fixture values that contain a policy user. It allows
// retrieval of the underlying policy user string.
type HasPolicyUser interface {
	PolicyUser() string
}

// PolicyUser implements the HasPolicyUser interface.
func (f FixtData) PolicyUser() string {
	if f.policyUser == "" {
		panic("PolicyUser is called with empty policyUser")
	}
	return f.policyUser
}

// Check at compile-time that FixtData implements the appropriate interfaces.
var _ chrome.HasChrome = FixtData{}
var _ HasTestConn = FixtData{}
var _ fakedms.HasFakeDMS = FixtData{}
var _ HasPolicyUser = FixtData{}

func (f *familyLinkFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var parentUser, parentPass string
	if f.parentUser == arcCommon.ParentAccountVarName {
		user, pass, err := dma.UserPassFromPool(f.parentUser)
		if err != nil {
			panic(fmt.Sprintf("Failed to get parent account: %v", err))
		}
		parentUser = user
		parentPass = pass
	} else {
		parentUser = s.RequiredVar(f.parentUser)
		parentPass = s.RequiredVar(f.parentPassword)
	}

	// Dev tools are necessary for the test instrumentation to work, but by default
	// disabled for supervised users. Always force enable them in supervised users tests.
	f.opts = append(f.opts, chrome.ExtraArgs("--force-devtools-available"))

	var childUser, childPass string
	dmaChildLogin := f.childUser == arcCommon.ChildAccountVarName
	isChildLogin := len(f.childUser) > 0 && len(f.childPassword) > 0
	if isChildLogin || dmaChildLogin {
		if dmaChildLogin {
			user, pass, err := dma.UserPassFromPool(f.childUser)
			if err != nil {
				panic(fmt.Sprintf("Failed to get child account: %v", err))
			}
			childUser = user
			childPass = pass
		} else {
			childUser = s.RequiredVar(f.childUser)
			childPass = s.RequiredVar(f.childPassword)
		}

		f.opts = append(f.opts, chrome.GAIALogin(chrome.Creds{
			User:       childUser,
			Pass:       childPass,
			ParentUser: parentUser,
			ParentPass: parentPass,
		}))
	} else {
		f.opts = append(f.opts, chrome.GAIALogin(chrome.Creds{
			User: parentUser,
			Pass: parentPass,
		}))
	}

	// Checks whether the current fixture has a FakeDMS parent fixture.
	fdms, isPolicyTest := s.ParentValue().(*fakedms.FakeDMS)
	if isPolicyTest {
		if err := fdms.Ping(ctx); err != nil {
			s.Fatal("Failed to ping FakeDMS: ", err)
		}

		if isChildLogin {
			f.policyUser = childUser
		} else {
			f.policyUser = parentUser
		}

		f.opts = append(f.opts, chrome.DMSPolicy(fdms.URL))
		// Family Link users look like consumer users with
		// @gmail.com emails but require policy. Since policy
		// key verification doesn't work for gmail users,
		// disable it.
		f.opts = append(f.opts, chrome.DisablePolicyKeyVerification())
	}

	if f.isLacros {
		var err error
		f.opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(f.opts...)).Opts()
		if err != nil {
			s.Fatal("Failed to get lacros options: ", err)
		}
		f.opts = append(f.opts, chrome.EnableFeatures("LacrosForSupervisedUsers"))
	}

	if !f.isOwner {
		func() {
			// Log in and log out to create a user pod on the login screen.
			cr, err := chrome.New(ctx, chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))
			if err != nil {
				s.Fatal("Chrome login failed: ", err)
			}
			defer cr.Close(ctx)

			if err := upstart.RestartJob(ctx, "ui"); err != nil {
				s.Fatal("Failed to restart ui: ", err)
			}
		}()

		// chrome.KeepState() is needed to show the login screen with a user pod (instead of the OOBE login screen).
		f.opts = append(f.opts, chrome.KeepState())
	}

	cr, err := chrome.New(ctx, f.opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
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
	fixtData := &FixtData{
		chrome:     cr,
		fakeDMS:    fdms,
		testConn:   tconn,
		policyUser: f.policyUser,
	}

	// Lock chrome after all Setup is complete so we don't block other fixtures.
	chrome.Lock()

	return fixtData
}

func (f *familyLinkFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}

func (f *familyLinkFixture) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}

	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}

	return nil
}

func (f *familyLinkFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *familyLinkFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
