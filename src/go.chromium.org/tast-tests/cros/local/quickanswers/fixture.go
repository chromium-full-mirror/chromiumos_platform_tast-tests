// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/testing"
)

const (
	// BasicFixture is a fixture with a screen recording.
	BasicFixture = "quickAnswersFixture"
	// lacrosFixtureInternal is a fixture of a Lacros Chrome session with a GAIA.
	lacrosFixtureInternal = "quickAnswersLoggedInFixtureLacros"
	// LacrosFixture is a lacros fixture with a screen recording.
	LacrosFixture = "quickAnswersLacrosFixture"

	setUpTimeout    = 10 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: BasicFixture,
		Desc: "A fixture with screen recording for QuickAnswers test",
		Contacts: []string{
			"assitive-eng@google.com",
			"yawano@google.com",
		},
		Parent:          fixture.ChromeLoggedInWithGaia,
		Impl:            &quickAnswersFixture{},
		SetUpTimeout:    setUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: lacrosFixtureInternal,
		Desc: "Lacros Chrome session logged in with OTA for Quick answers testing",
		Contacts: []string{
			"assistive-eng@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			}
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosFixture,
		Desc: "A fixture with screen recording for a Lacros QuickAnswers test",
		Contacts: []string{
			"assistive-eng@google.com",
			"yawano@google.com",
		},
		Parent:          lacrosFixtureInternal,
		Impl:            &quickAnswersFixture{},
		SetUpTimeout:    setUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
}

type quickAnswersFixture struct {
	tconn    *chrome.TestConn
	recorder *uiauto.ScreenRecorder
	cr       *chrome.Chrome
}

func (f *quickAnswersFixture) Chrome() *chrome.Chrome {
	return f.cr
}

func (f *quickAnswersFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.cr = s.ParentValue().(chrome.HasChrome).Chrome()
	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a Test API connection: ", err)
	}
	f.tconn = tconn
	return f
}
func (f *quickAnswersFixture) TearDown(ctx context.Context, s *testing.FixtState) {}
func (f *quickAnswersFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *quickAnswersFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)
}

func (f *quickAnswersFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(
			ctx, filepath.Join(s.OutDir(), "recording.webm"), s.HasError)
	}
}
