// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for inputs tests.
package fixture

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

// chromeOpts describes the extra chrome options needed.
type chromeOpts int

const (
	guestLogin chromeOpts = iota
	gaiaLogin
	assistMultiWord
	autocorrectToggle
	diacriticsOnPhysicalKeyboardLongpress
	emojiPickerGifSupport
	firstPartyVietnameseInput
	altClickAndSixPackCustomization
	orca
	picker
	withoutAssistMultiword
)

// List of fixture names for inputs.
const (
	AnyVK                                             = "anyVK"
	AnyVKInGuest                                      = "anyVKInGuest"
	AnyVKInGAIA                                       = "anyVKInGaia"
	ClamshellVK                                       = "clamshellVK"
	ClamshellVKRestart                                = "clamshellVKRestart"
	ClamshellNonVKWithAltClickAndSixPackCustomization = "clamshellNonVKWithAltClickAndSixPackCustomization"
	ClamshellNonVKWithDiacriticsOnPKLongpress         = "clamshellWithDiacriticsOnPKLongpress"
	ClamshellNonVKWithFirstPartyVietnamese            = "clamshellNonVKWithFirstPartyVietnamese"
	ClamshellNonVK                                    = "clamshellNonVK"
	ClamshellNonVKStereoAloopLoaded                   = "clamshellNonVKStereoAloopLoaded"
	ClamshellNonVKInGAIA                              = "clamshellNonVKInGAIA"
	ClamshellNonVKInGuest                             = "clamshellNonVKInGuest"
	ClamshellNonVKInGuestWithoutMultiwordSuggest      = "clamshellNonVKInGuestWithoutMultiwordSuggest"
	ClamshellNonVKRestart                             = "clamshellNonVKRestart"
	ClamshellNonVKWithMultiwordSuggest                = "clamshellNonVKWithMultiwordSuggest"
	ClamshellNonVKWithOrca                            = "clamshellNonVKWithOrca"
	ClamshellNonVKWithPicker                          = "clamshellNonVKWithPicker"
	ClamshellNonVKWithoutMultiwordSuggest             = "clamshellNonVKWithoutMultiwordSuggest"
	TabletVK                                          = "tabletVK"
	TabletVKStereoAloopLoaded                         = "tabletVKStereoAloopLoaded"
	TabletVKRestart                                   = "tabletVKRestart"
	TabletVKInGuest                                   = "tabletVKInGuest"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: AnyVK,
		Desc: "Any mode with VK enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: AnyVKInGuest,
		Desc: "Any mode in guest login with VK enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, guestLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: AnyVKInGAIA,
		Desc: "Gaia login with VK enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, gaiaLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKInGAIA,
		Desc: "Clamshell mode Gaia login with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, gaiaLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellVK,
		Desc: "Clamshell mode with A11y VK enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellVKRestart,
		Desc: "Clamshell mode with A11y VK enabled, restarting chrome session for every test",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, true),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVK,
		Desc: "Clamshell mode with VK disabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, autocorrectToggle, emojiPickerGifSupport),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKStereoAloopLoaded,
		Desc: "Clamshell mode with VK disabled and stereo aloop loaded",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl: inputsFixture(clamshellMode, false, false, autocorrectToggle, emojiPickerGifSupport),
		// Need aloop for route playback to capture.
		Parent:          fixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKRestart,
		Desc: "Clamshell mode with VK disabled, restarting chrome session for every test",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, true),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithAltClickAndSixPackCustomization,
		Desc: "Clamshell mode with alt-click and six pack customization",
		Contacts: []string{
			"jhtin@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, altClickAndSixPackCustomization),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithMultiwordSuggest,
		Desc: "Clamshell mode with VK disabled and multiword suggest",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, assistMultiWord),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithoutMultiwordSuggest,
		Desc: "Clamshell mode with VK disabled and multiword suggest disabled",
		Contacts: []string{
			"hdchuong@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, withoutAssistMultiword),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithOrca,
		Desc: "Clamshell mode with VK disabled and Orca enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, true, orca),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithPicker,
		Desc: "Clamshell mode with VK disabled and Picker enabled",
		Contacts: []string{
			"shend@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, picker, gaiaLogin, orca),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"inputs.Picker.pickerFeatureTestKey"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithDiacriticsOnPKLongpress,
		Desc: "Clamshell mode with diacritics",
		Contacts: []string{
			"jhtin@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, diacriticsOnPhysicalKeyboardLongpress),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKWithFirstPartyVietnamese,
		Desc: "Clamshell mode with First Party Vietnamese Input",
		Contacts: []string{
			"jhtin@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, firstPartyVietnameseInput),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKInGuest,
		Desc: "Clamshell mode in guest login with VK disabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, guestLogin, emojiPickerGifSupport),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKInGuestWithoutMultiwordSuggest,
		Desc: "Clamshell mode in guest login with VK disabled and multiword suggest disabled",
		Contacts: []string{
			"hdchuong@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, guestLogin, withoutAssistMultiword),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: TabletVK,
		Desc: "Tablet mode with VK enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, false),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: TabletVKStereoAloopLoaded,
		Desc: "Tablet mode with VK enabled and stereo aloop loaded",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl: inputsFixture(tabletMode, true, false),
		// Need aloop for route playback to capture.
		Parent:          fixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: TabletVKRestart,
		Desc: "Tablet mode with VK enabled, restarting chrome session for every test",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, true),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: TabletVKInGuest,
		Desc: "Tablet mode in guest login with VK enabled",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, false, guestLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// FixtData is the data returned by SetUp and passed to tests.
type FixtData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	UserContext *useractions.UserContext
}

// deviceMode describes the device UI mode it boots in.
type deviceMode int

const (
	notForced deviceMode = iota
	tabletMode
	clamshellMode
)

// inputsFixtureImpl implements testing.FixtureImpl.
type inputsFixtureImpl struct {
	cr        *chrome.Chrome // Underlying Chrome instance
	dm        deviceMode     // Device ui mode to test
	vkEnabled bool           // Whether virtual keyboard is force enabled
	restart   bool           // Whether restart the fixture after each test
	fOpts     []chromeOpts   // Options that are passed to chrome.New
	tconn     *chrome.TestConn
	recorder  *uiauto.ScreenRecorder
	uc        *useractions.UserContext
}

func (f *inputsFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var opts []chrome.Option
	// If there's a parent fixture and the fixture supplies extra options, use them.
	if extraOpts, ok := s.ParentValue().([]chrome.Option); ok {
		opts = append(opts, extraOpts...)
	}

	for _, opt := range f.fOpts {
		switch opt {
		case guestLogin:
			opts = append(opts, chrome.GuestLogin())
		case gaiaLogin:
			opts = append(opts, chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))
		case autocorrectToggle:
			opts = append(opts, chrome.ExtraArgs("--enable-features=AutocorrectToggle"))
		case assistMultiWord:
			opts = append(opts, chrome.ExtraArgs("--enable-features=AssistMultiWord"))
		case diacriticsOnPhysicalKeyboardLongpress:
			opts = append(opts, chrome.ExtraArgs("--enable-features=DiacriticsOnPhysicalKeyboardLongpress,DiacriticsOnPhysicalKeyboardLongpressDefaultOn"))
		case firstPartyVietnameseInput:
			opts = append(opts, chrome.ExtraArgs("--enable-features=FirstPartyVietnameseInput"))
		case emojiPickerGifSupport:
			opts = append(opts, chrome.ExtraArgs(("--enable-features=SystemEmojiPickerGIFSupport")))
		case altClickAndSixPackCustomization:
			opts = append(opts, chrome.ExtraArgs("--enable-features=AltClickAndSixPackCustomization"))
		case orca:
			opts = append(opts, chrome.ExtraArgs("--enable-features=OrcaDogfood,MantaService"))
		case picker:
			opts = append(opts, chrome.ExtraArgs("--enable-features=Picker,PickerGrid"))
			opts = append(opts, chrome.ExtraArgs("--picker-feature-key="+s.RequiredVar("inputs.Picker.pickerFeatureTestKey")))
			opts = append(opts, chrome.ExtraArgs("--disable-sync"))
		case withoutAssistMultiword:
			opts = append(opts, chrome.ExtraArgs("--disable-features=AssistMultiWord"))
		}
	}

	switch f.dm {
	case tabletMode:
		opts = append(opts, chrome.ExtraArgs("--force-tablet-mode=touch_view"))
	case clamshellMode:
		opts = append(opts, chrome.ExtraArgs("--force-tablet-mode=clamshell"))
	}

	if f.vkEnabled && f.dm != clamshellMode {
		// Force enable tablet VK by default. Even the device is actually in clamshell mode but not explicitly mentioned.
		opts = append(opts, chrome.VKEnabled())
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if s.HasError() {
			cr.Close(ctx)
		}
	}()
	f.cr = cr

	f.tconn, err = f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	if f.vkEnabled && f.dm == clamshellMode {
		// Enable a11y virtual keyboard.
		if err := vkb.NewContext(f.cr, f.tconn).EnableA11yVirtualKeyboard(true)(ctx); err != nil {
			s.Fatal("Failed to enable a11y virtual keyboard: ", err)
		}
	}

	uc, err := inputactions.NewInputsUserContextWithoutState(ctx, "", s.OutDir(), f.cr, f.tconn, nil)
	if err != nil {
		s.Fatal("Failed to create new inputs user context: ", err)
	}
	f.uc = uc

	chrome.Lock()
	return FixtData{f.cr, f.tconn, f.uc}
}

func (f *inputsFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.uc.SetTestName(s.TestName())

	recorder, err := uiauto.NewScreenRecorder(ctx, f.tconn)
	if err != nil {
		s.Log("Failed to create screen recorder: ", err)
		return
	}
	if err := recorder.Start(ctx, f.tconn); err != nil {
		s.Log("Failed to start screen recorder: ", err)
		return
	}
	f.recorder = recorder
}

func (f *inputsFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Hide virtual keyboard in case it is still on screen.
	if f.vkEnabled {
		if err := vkb.NewContext(f.cr, f.tconn).HideVirtualKeyboard()(ctx); err != nil {
			s.Log("Failed to hide virtual keyboard: ", err)
		}
	}

	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *inputsFixtureImpl) Reset(ctx context.Context) error {
	if f.restart {
		return errors.New("Intended error to trigger fixture restart")
	}
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := resetIMEStatus(ctx, f.tconn); err != nil {
		return errors.Wrap(err, "failed resetting ime")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *inputsFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
	f.tconn = nil
}

func inputsFixture(dm deviceMode, vkEnabled, restart bool, opts ...chromeOpts) testing.FixtureImpl {
	return &inputsFixtureImpl{
		dm:        dm,
		vkEnabled: vkEnabled,
		restart:   restart,
		fOpts:     opts,
	}
}

// resetIMEStatus resets IME input method and settings.
func resetIMEStatus(ctx context.Context, tconn *chrome.TestConn) error {
	if err := ime.DefaultInputMethod.Install(tconn)(ctx); err != nil {
		return errors.Wrapf(err, "failed to install default ime %q", ime.DefaultInputMethod)
	}
	// Uninstall all input methods except the default one.
	installedIMEs, err := ime.InstalledInputMethods(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get installed ime list")
	}
	prefix, err := ime.Prefix(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get ime prefix")
	}
	for _, installedIME := range installedIMEs {
		if strings.TrimPrefix(installedIME.ID, prefix) == ime.DefaultInputMethod.ID {
			continue
		}
		if err := ime.RemoveInputMethod(ctx, tconn, installedIME.ID); err != nil {
			return errors.Wrapf(err, "failed to remove %s", installedIME.ID)
		}
	}
	// Reset input to default input method.
	if err := ime.DefaultInputMethod.Activate(tconn)(ctx); err != nil {
		return errors.Wrapf(err, "failed to set ime to %q", ime.DefaultInputMethod)
	}
	if err := ime.DefaultInputMethod.ResetSettings(tconn)(ctx); err != nil {
		return errors.Wrapf(err, "failed to reset ime settings of the default ime %s", ime.DefaultInputMethod)
	}
	return nil
}
