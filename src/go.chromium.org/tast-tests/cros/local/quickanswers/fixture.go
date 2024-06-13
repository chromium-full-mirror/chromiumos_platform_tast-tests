// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// EnabledWithBrowserFixture is a fixture with a browser opened with a
	// query.
	EnabledWithBrowserFixture = "enabledWithBrowserFixture"
	// EnabledWithBrowserLacrosFixture is a lacros fixture with a browser
	// opened with a query.
	EnabledWithBrowserLacrosFixture = "enabledWithBrowserLacrosFixture"
	// NotEnabledWithBrowserFixture is a fixture with a browser opened with
	// a query but quick answers is not enabled. Note that not-enabled is not
	// disabled, i.e., quick answers show a consent UI.
	NotEnabledWithBrowserFixture = "notEnabledWithBrowserFixture"
	// NotEnabledWithBrowserLacrosFixture is a lacros fixture with a browser
	// opened with a query but quick answers is not enabled.
	NotEnabledWithBrowserLacrosFixture = "notEnabledWithBrowserLacrosFixture"

	// VariantSingleWord is a name of WithBrowserFixture variant with a single
	// English word.
	VariantSingleWord = "singleWord"
	// VariantSimpleWord is a name of WithBrowserFixture variant with a simple
	// English word.
	VariantSimpleWord = "simpleWord"
	// VariantUnitConversion is a name of WithBrowserFixture variant with a
	// query for unit conversion intent.
	VariantUnitConversion = "unitConversion"
	// VariantTranslation is a name of WithBrowserFixture variant with a
	// query for translation intent.
	VariantTranslation = "translation"
	// VariantTranslationSentence is a name of WithBrowserFixture variant
	// with a sentence for translation intent.
	VariantTranslationSentence = "translationSentence"
	// VariantSingleWordEs is a name of WithBrowserFixture variant with a
	// single Spanish word.
	VariantSingleWordEs = "singleWordEs"
	// VariantSingleWordIt is a name of WithBrowserFixture variant with a
	// single Italian word.
	VariantSingleWordIt = "singleWordIt"
	// VariantSingleWordFr is a name of WithBrowserFixture variant with a
	// single French word.
	VariantSingleWordFr = "singleWordFr"
	// VariantSingleWordPt is a name of WithBrowserFixture variant with a
	// single Portuguese word.
	VariantSingleWordPt = "singleWordPt"
	// VariantSingleWordDe is a name of WithBrowserFixture variant with a
	// single German word.
	VariantSingleWordDe = "singleWordDe"

	// BaseFixture is a fixture with specified quick answers pref state.
	// TODO(b/339097439): Make this a private. All tests should use
	// WithBrowserFixture.
	BaseFixture = "quickAnswersFixture"
	// BaseLacrosFixture is a lacros fixture with specified quick answers
	// pref state.
	// TODO(b/339097439): Make this a private. All tests should use
	// WithBrowserFixture.
	BaseLacrosFixture = "quickAnswersLacrosFixture"

	variantNotEnabled = "notEnabled"

	lacrosFixtureInternal = "quickAnswersLoggedInFixtureLacros"

	networkConnectionTimeout = time.Minute
	setUpTimeout             = 10 * time.Second
	preTestTimeout           = 30 * time.Second
	postTestTimeout          = 15 * time.Second
)

// Parameterize builds a parameterized fixture name.
func Parameterize(fixtureName, variantName string) string {
	return fixtureName + "." + variantName
}

func withBrowserFixtureParams() []testing.FixtureParam {
	return []testing.FixtureParam{
		{
			Name: VariantSingleWord,
			Val: withBrowserFixtureParam{
				queryWord: "icosahedron",
			},
		},
		{
			Name: VariantSimpleWord,
			Val: withBrowserFixtureParam{
				queryWord: "dog",
			},
		},
		{
			Name: VariantUnitConversion,
			Val: withBrowserFixtureParam{
				queryWord: "50 kg",
			},
		},
		{
			Name: VariantTranslation,
			Val: withBrowserFixtureParam{
				// En-translation: mission
				queryWord: "使命",
			},
		},
		{
			Name: VariantTranslationSentence,
			Val: withBrowserFixtureParam{
				// From https://about.google/intl/zh_cn/
				queryWord: "我们的使命是整合全球信息，供大众使用，让人人受益。",
			},
		},
		{
			Name: VariantSingleWordEs,
			Val: withBrowserFixtureParam{
				queryWord:     "pentágono",
				languageCodes: []string{"es"},
			},
		},
		{
			Name: VariantSingleWordIt,
			Val: withBrowserFixtureParam{
				queryWord:     "settimana",
				languageCodes: []string{"it"},
			},
		},
		{
			Name: VariantSingleWordFr,
			Val: withBrowserFixtureParam{
				queryWord:     "semaine",
				languageCodes: []string{"fr"},
			},
		},
		{
			Name: VariantSingleWordPt,
			Val: withBrowserFixtureParam{
				queryWord:     "futebol",
				languageCodes: []string{"pt"},
			},
		},
		{
			Name: VariantSingleWordDe,
			Val: withBrowserFixtureParam{
				queryWord:     "verdreifachen",
				languageCodes: []string{"de"},
			},
		},
	}
}

func quickAnswersFixtureParams(browserType browser.Type) []testing.FixtureParam {
	return []testing.FixtureParam{
		{
			Val: quickAnswersFixtureParam{
				state:       StateEnabled,
				browserType: browserType,
			},
		},
		{
			Name: variantNotEnabled,
			Val: quickAnswersFixtureParam{
				state:       StateNotEnabled,
				browserType: browserType,
			},
		},
	}
}

// init registers fixtures of Quick Answers.
//
// Quick Answers provide two fixtures:
//   - withBrowserFixture: opens a browser with a data url page of a specified
//     query word. This fixture also takes a screenshot just before closing a
//     browser if a test has failed. A screenshot provides a quicker way to know
//     a failed state compared to a screen recording.
//   - quickAnswersFixture: configures quick answers pref value. This fixture
//     saves a screen recording if a test has failed.
//
// Fixtures are set up in the following relationship:
// withBrowserFixture <- quickAnswersFixture <- ChromeLoggedInWithGaia
func init() {
	testing.AddFixture(&testing.Fixture{
		Name: EnabledWithBrowserFixture,
		Desc: "A fixture with a test query page",
		Contacts: []string{
			"assitive-eng@google.com",
			"yawano@google.com",
		},
		BugComponent:    "b:905229", // ChromeOS > Software > Assistive
		Parent:          BaseFixture,
		Impl:            &withBrowserFixture{},
		Params:          withBrowserFixtureParams(),
		SetUpTimeout:    setUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: EnabledWithBrowserLacrosFixture,
		Desc: "A lacros fixture with a test query page",
		Contacts: []string{
			"assitive-eng@google.com",
			"yawano@google.com",
		},
		BugComponent:    "b:905229", // ChromeOS > Software > Assistive
		Parent:          BaseLacrosFixture,
		Impl:            &withBrowserFixture{},
		Params:          withBrowserFixtureParams(),
		SetUpTimeout:    setUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: NotEnabledWithBrowserFixture,
		Desc: "A quick answers not enabled fixture with a test query page",
		Contacts: []string{
			"assitive-eng@google.com",
			"yawano@google.com",
		},
		BugComponent:    "b:905229", // ChromeOS > Software > Assistive
		Parent:          Parameterize(BaseFixture, variantNotEnabled),
		Impl:            &withBrowserFixture{},
		Params:          withBrowserFixtureParams(),
		SetUpTimeout:    setUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: NotEnabledWithBrowserLacrosFixture,
		Desc: "A quick answers not enabled lacros fixture with a test query page",
		Contacts: []string{
			"assitive-eng@google.com",
			"yawano@google.com",
		},
		BugComponent:    "b:905229", // ChromeOS > Software > Assistive
		Parent:          Parameterize(BaseLacrosFixture, variantNotEnabled),
		Impl:            &withBrowserFixture{},
		Params:          withBrowserFixtureParams(),
		SetUpTimeout:    setUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: BaseFixture,
		Desc: "A base fixture for Quick Answers test",
		Contacts: []string{
			"assitive-eng@google.com",
			"yawano@google.com",
		},
		BugComponent:    "b:905229", // ChromeOS > Software > Assistive
		Parent:          fixture.ChromeLoggedInWithGaia,
		Impl:            &quickAnswersFixture{},
		SetUpTimeout:    setUpTimeout + networkConnectionTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Params:          quickAnswersFixtureParams(browser.TypeAsh),
	})
	testing.AddFixture(&testing.Fixture{
		Name: BaseLacrosFixture,
		Desc: "A base lacros fixture for a Quick Answers test",
		Contacts: []string{
			"assistive-eng@google.com",
			"yawano@google.com",
		},
		BugComponent:    "b:905229", // ChromeOS > Software > Assistive
		Parent:          lacrosFixtureInternal,
		Impl:            &quickAnswersFixture{},
		SetUpTimeout:    setUpTimeout + networkConnectionTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Params:          quickAnswersFixtureParams(browser.TypeLacros),
	})

	testing.AddFixture(&testing.Fixture{
		Name: lacrosFixtureInternal,
		Desc: "Lacros Chrome session logged in with OTA for Quick answers testing",
		Contacts: []string{
			"assistive-eng@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := []chrome.Option{
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
			}
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

}

type hasBrowserType interface {
	browserType() browser.Type
}

// State is a state of quick answers pref.
type State string

const (
	// StateNotEnabled is a state where quick answers pref is left at default
	// value. This is not equal to disabled.
	StateNotEnabled State = "notEnabled"
	// StateEnabled is a state where quick answers pref is set to enabled.
	StateEnabled State = "enabled"
)

type quickAnswersFixtureParam struct {
	state       State
	browserType browser.Type
}

type quickAnswersFixture struct {
	tconn    *chrome.TestConn
	recorder *uiauto.ScreenRecorder
	cr       *chrome.Chrome
	bt       browser.Type
	state    State
}

func (f *quickAnswersFixture) browserType() browser.Type {
	return f.bt
}

func (f *quickAnswersFixture) Chrome() *chrome.Chrome {
	return f.cr
}

func (f *quickAnswersFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	param := s.Param().(quickAnswersFixtureParam)
	f.state = param.state
	f.bt = param.browserType

	f.cr = s.ParentValue().(chrome.HasChrome).Chrome()
	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a Test API connection: ", err)
	}
	f.tconn = tconn

	// All QuickAnswers tast tests require an internet connection.
	if err := ping.VerifyInternetConnectivity(
		ctx, networkConnectionTimeout); err != nil {
		s.Fatal("Failed to wait an internet connection: ", err)
	}

	return f
}

func (f *quickAnswersFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)

	switch f.state {
	case StateEnabled:
		if err := Enable(ctx, f.tconn); err != nil {
			s.Fatal("Failed to enable Quick Answers: ", err)
		}
	case StateNotEnabled:
		if err := ResetPref(ctx, f.tconn); err != nil {
			s.Fatal("Failed to reset Quick Answers prefs: ", err)
		}
	}
}

func (f *quickAnswersFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *quickAnswersFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(
			ctx, filepath.Join(s.OutDir(), "recording.webm"), s.HasError)
	}
}

func (f *quickAnswersFixture) TearDown(ctx context.Context, s *testing.FixtState) {}

// HasQueryWord is an interface for getting a query word tied to a fixture.
type HasQueryWord interface {
	QueryWord() string
}

type withBrowserFixtureParam struct {
	queryWord     string
	languageCodes []string
}

type withBrowserFixture struct {
	queryWord     string
	bt            browser.Type
	cr            *chrome.Chrome
	conn          *chrome.Conn
	tconn         *chrome.TestConn
	languageCodes []string
	closeBrowser  func(context.Context) error
}

func maybeSetPreferredLanguages(ctx context.Context,
	tconn *chrome.TestConn, languageCodes []string) error {
	if languageCodes == nil {
		return nil
	}

	return SetPreferredLanguages(ctx, tconn, languageCodes)
}

func (f *withBrowserFixture) Chrome() *chrome.Chrome {
	return f.cr
}

func (f *withBrowserFixture) QueryWord() string {
	return f.queryWord
}

func (f *withBrowserFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	param := s.Param().(withBrowserFixtureParam)
	f.queryWord = param.queryWord
	f.languageCodes = param.languageCodes
	f.bt = s.ParentValue().(hasBrowserType).browserType()
	f.cr = s.ParentValue().(chrome.HasChrome).Chrome()

	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a Test API connection")
	}
	f.tconn = tconn

	if err := maybeSetPreferredLanguages(ctx, tconn, f.languageCodes); err != nil {
		s.Fatal("Failed to set preferred languages: ", err)
	}

	return f
}

func (f *withBrowserFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(
		ctx, f.cr, f.bt, BuildDataURL(f.queryWord))
	if err != nil {
		s.Fatal("Failed to open a browser: ", err)
	}
	f.conn = conn
	f.closeBrowser = closeBrowser
}

func (f *withBrowserFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, f.cr, "browser_ui")

	f.closeBrowser(ctx)
	f.conn.Close()
}

func (f *withBrowserFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := SetPreferredLanguages(ctx, f.tconn, []string{"en"}); err != nil {
		s.Fatal("Failed to set preferred languages: ", err)
	}
}

func (f *withBrowserFixture) Reset(ctx context.Context) error {
	return maybeSetPreferredLanguages(ctx, f.tconn, f.languageCodes)
}
