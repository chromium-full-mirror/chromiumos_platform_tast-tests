// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mtbf implements a library used for MTBF testing.
package mtbf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// AccountPool is runtime variable name for credentials to log into a Chrome user session.
	AccountPool = "mtbf.accountPool"

	// chromeLoggedInReuseFixture is a fixture name that will be registered to tast.
	chromeLoggedInReuseFixture = "mtbfChromeLogInReuse"

	// LoginReuseFixture is a fixture name that will be registered to tast.
	LoginReuseFixture = "mtbfLoginReuseCleanTabs"
)

// LoginReuseOptions returns the login option for MTBF tests.
func LoginReuseOptions(accountPool string) []chrome.Option {
	return []chrome.Option{
		chrome.KeepState(),
		chrome.ARCSupported(),
		chrome.GAIALoginPool(accountPool),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		chrome.TryReuseSession(),
	}
}

func loginReuseOptionsCallBack(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
	return LoginReuseOptions(s.RequiredVar(AccountPool)), nil
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            chromeLoggedInReuseFixture,
		Desc:            "Reuse the existing user login session and boot ARC",
		Contacts:        []string{"xliu@cienet.com", "alfredyu@cienet.com", "abergman@google.com"},
		BugComponent:    "b:1025042",
		Impl:            arc.NewMtbfArcBootedFixture(loginReuseOptionsCallBack),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		PreTestTimeout:  arc.PreTestTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{AccountPool},
	})

	testing.AddFixture(&testing.Fixture{
		Name:           LoginReuseFixture,
		Desc:           "Reuse the existing user login session and clean chrome tabs",
		Contacts:       []string{"xliu@cienet.com", "alfredyu@cienet.com", "abergman@google.com"},
		BugComponent:   "b:1025042",
		Parent:         chromeLoggedInReuseFixture,
		Impl:           &mtbfCleanTabsFixture{},
		PreTestTimeout: 4 * clearTabsTimeout,
	})
}

const clearTabsTimeout = 10 * time.Second

// FixtValue holds information made available to tests that specify this Fixture.
type FixtValue struct {
	cr *chrome.Chrome
	// ARC enables interaction with an already-started ARC environment.
	// It cannot be closed by tests.
	ARC *arc.ARC
}

// Chrome gets the CrOS-chrome instance.
// Implements the chrome.HasChrome interface.
func (f FixtValue) Chrome() *chrome.Chrome { return f.cr }

type mtbfCleanTabsFixture struct {
	fixtValue *FixtValue
}

func (f *mtbfCleanTabsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	parentValue := s.ParentValue()

	f.fixtValue = &FixtValue{
		cr:  parentValue.(*arc.PreData).Chrome,
		ARC: parentValue.(*arc.PreData).ARC,
	}

	return f.fixtValue
}

func (f *mtbfCleanTabsFixture) TearDown(ctx context.Context, s *testing.FixtState) {}

func (f *mtbfCleanTabsFixture) Reset(ctx context.Context) error { return nil }

func (f *mtbfCleanTabsFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	br := f.fixtValue.cr.Browser()

	if err := closeExistingAndLeftOffTabs(ctx, br); err != nil {
		s.Fatal("Failed to close existing and left-off tab(s): ", err)
	}
}

func (f *mtbfCleanTabsFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

// closeExistingAndLeftOffTabs closes the existing and left-off tabs.
func closeExistingAndLeftOffTabs(ctx context.Context, br *browser.Browser) error {
	btconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	for {
		tabsCnt, err := countExistingTabs(ctx, btconn)
		if err != nil {
			return err
		}
		if tabsCnt > 0 {
			// The last session did not clanup properly, remove existing tabs can ensure tabs are cleaned.
			testing.ContextLogf(ctx, "Removing %d existing page(s)", tabsCnt)
			return removeExistingTabs(ctx, btconn)
		}

		// Depending on the settings, Chrome might open all left-off pages automatically from last session,
		// which the left-off pages might casues test case fail.
		// Launch Chrome browser by open a blank page to bring up all left-off pages to further remove them.
		testing.ContextLog(ctx, "Opening empty Chrome tab to bring up left-off page(s)")
		conn, err := br.NewConn(ctx, "", browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to launch Chrome browser")
		}
		defer conn.Close()

		// After the first iteration, an empty Chrome tab will be opened,
		// i.e. at least one target will be found, which guarantees this isn't an infinity loop.
	}
}

func countExistingTabs(ctx context.Context, tconn *chrome.TestConn) (int, error) {
	execCtx, cancel := context.WithTimeout(ctx, clearTabsTimeout)
	defer cancel()

	tabs, err := browser.AllTabs(execCtx, tconn)
	if err != nil {
		return 0, err
	}

	for _, tab := range tabs {
		testing.ContextLogf(ctx, "Found an existing tab: %+v", tab)
	}

	return len(tabs), nil
}

func removeExistingTabs(ctx context.Context, tconn *chrome.TestConn) error {
	execCtx, cancel := context.WithTimeout(ctx, clearTabsTimeout)
	defer cancel()

	// If there exist unsave changes on web page, e.g. media content is playing or online document is editing,
	// "leave site" prompt will prevent the tab from closing and block the process,
	// therefore, a short context is required.
	if err := browser.CloseAllTabs(execCtx, tconn); err != nil {
		return errors.Wrap(err, "failed to remove tabs")
	}

	return nil
}
