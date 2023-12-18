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

	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
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
	handwritingLegacyRecognition
	emojiPickerGifSupport
	firstPartyVietnameseInput
	altClickAndSixPackCustomization
	orca
	picker
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
	ClamshellNonVKInGuest                             = "clamshellNonVKInGuest"
	ClamshellNonVKRestart                             = "clamshellNonVKRestart"
	ClamshellNonVKWithMultiwordSuggest                = "clamshellNonVKWithMultiwordSuggest"
	ClamshellNonVKWithOrca                            = "clamshellNonVKWithOrca"
	ClamshellNonVKWithPicker                          = "clamshellNonVKWithPicker"
	ClamshellNonVKInGAIA                              = "clamshellNonVKInGAIA"
	ClamshellVKWithHandWritingLegacyRecognitionOn     = "clamshellVKWithHandWritingLegacyRecognitionOn"
	TabletVK                                          = "tabletVK"
	TabletVKWithHandWritingLegacyRecognitionOn        = "tabletVKWithHandWritingLegacyRecognitionOn"
	TabletVKStereoAloopLoaded                         = "tabletVKStereoAloopLoaded"
	TabletVKRestart                                   = "tabletVKRestart"
	TabletVKInGuest                                   = "tabletVKInGuest"
	// Lacros fixtures.
	LacrosAnyVK                                             = "lacrosAnyVK"
	LacrosAnyVKInGuest                                      = "lacrosAnyVKInGuest"
	LacrosAnyVKInGAIA                                       = "lacrosAnyVKInGaia"
	LacrosClamshellVK                                       = "lacrosClamshellVK"
	LacrosClamshellVKWithHandWritingLegacyRecognitionOn     = "lacrosclamshellVKWithHandWritingLegacyRecognitionOn"
	LacrosClamshellNonVK                                    = "lacrosClamshellNonVK"
	LacrosClamshellNonVKStereoAloopLoaded                   = "lacrosClamshellNonVKStereoAloopLoaded"
	LacrosClamshellNonVKInGuest                             = "lacrosClamshellNonVKInGuest"
	LacrosClamshellNonVKInGAIA                              = "lacrosClamshellNonVKInGaia"
	LacrosClamshellNonVKRestart                             = "lacrosClamshellNonVKRestart"
	LacrosClamshellNonVKWithAltClickAndSixPackCustomization = "lacrosClamshellNonVKWithAltClickAndSixPackCustomization"
	LacrosClamshellNonVKWithMultiwordSuggest                = "lacrosClamshellNonVKWithMultiwordSuggest"
	LacrosClamshellNonVKWithOrca                            = "lacrosClamshellNonVKWithOrca"
	LacrosClamshellNonVKWithDiacriticsOnPKLongpress         = "lacrosClamshellWithDiacriticsOnPKLongpress"
	LacrosClamshellNonVKWithFirstPartyVietnamese            = "lacrosClamshellNonVKWithFirstPartyVietnamese"
	LacrosTabletVK                                          = "lacrosTabletVK"
	LacrosTabletVKWithHandWritingLegacyRecognitionOn        = "lacrostabletVKWithHandWritingLegacyRecognitionOn"
	LacrosTabletVKStereoAloopLoaded                         = "lacrosTabletVKStereoAloopLoaded"
	LacrosTabletVKInGuest                                   = "lacrosTabletVKInGuest"
	LacrosTabletVKRestart                                   = "lacrosTabletVKRestart"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: AnyVK,
		Desc: "Any mode with VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, browser.TypeAsh),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, browser.TypeAsh, guestLogin),
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
		Impl:            inputsFixture(notForced, true, false, browser.TypeAsh, gaiaLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellNonVKInGAIA,
		Desc: "Clamshell mode Gaia login with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, gaiaLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellVK,
		Desc: "Clamshell mode with A11y VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false, browser.TypeAsh),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, true, browser.TypeAsh),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, autocorrectToggle, emojiPickerGifSupport),
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
			"alvinjia@google.com",
		},
		Impl: inputsFixture(clamshellMode, false, false, browser.TypeAsh, autocorrectToggle, emojiPickerGifSupport),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, true, browser.TypeAsh),
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
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, altClickAndSixPackCustomization),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, assistMultiWord),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, orca),
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
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, picker),
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
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, diacriticsOnPhysicalKeyboardLongpress),
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
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, firstPartyVietnameseInput),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeAsh, guestLogin, emojiPickerGifSupport),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, false, browser.TypeAsh),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl: inputsFixture(tabletMode, true, false, browser.TypeAsh),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, true, browser.TypeAsh),
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
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, false, browser.TypeAsh, guestLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellVKWithHandWritingLegacyRecognitionOn,
		Desc: "Clamshell mode with handwriting legacy recognition on",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false, browser.TypeAsh, handwritingLegacyRecognition),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: TabletVKWithHandWritingLegacyRecognitionOn,
		Desc: "Tablelet mode with handwriting legacy recognition on",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false, browser.TypeAsh, handwritingLegacyRecognition),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	//--------------Lacros Fixtures--------------------------------------------
	testing.AddFixture(&testing.Fixture{
		Name: LacrosAnyVK,
		Desc: "Lacros variant: any mode with VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, browser.TypeLacros),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosAnyVKInGuest,
		Desc: "Lacros variant: any mode in guest login with VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, browser.TypeLacros, guestLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellVK,
		Desc: "Lacros variant: clamshell mode with A11y VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false, browser.TypeLacros),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVK,
		Desc: "Lacros variant: clamshell mode with VK disabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, autocorrectToggle, emojiPickerGifSupport),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKStereoAloopLoaded,
		Desc: "Lacros variant: clamshell mode with VK disabled and stereo aloop loaded",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"alvinjia@google.com",
		},
		Impl: inputsFixture(clamshellMode, false, false, browser.TypeLacros, autocorrectToggle, emojiPickerGifSupport),
		// Need aloop for route playback to capture.
		Parent:          fixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKRestart,
		Desc: "Lacros variant: clamshell mode with VK disabled, restarting chrome session for every test",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, true, browser.TypeLacros),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKWithAltClickAndSixPackCustomization,
		Desc: "Lacros variant: Clamshell mode with alt-click and six pack customization",
		Contacts: []string{
			"jhtin@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, altClickAndSixPackCustomization),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKWithMultiwordSuggest,
		Desc: "Lacros variant: clamshell mode with VK disabled and multiword suggest",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, assistMultiWord),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKWithOrca,
		Desc: "Lacros variant: clamshell mode with VK disabled and Orca enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, orca),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKWithDiacriticsOnPKLongpress,
		Desc: "Lacros variant: clamshell mode with VK disabled and diacritics on PK longpress",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, diacriticsOnPhysicalKeyboardLongpress),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKWithFirstPartyVietnamese,
		Desc: "Lacros variant: clamshell mode with VK disabled and first party Vietnamese input enabled",
		Contacts: []string{
			"jhtin@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, firstPartyVietnameseInput),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKInGuest,
		Desc: "Lacros variant: clamshell mode in guest login with VK disabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, guestLogin, emojiPickerGifSupport),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosTabletVK,
		Desc: "Lacros variant: tablet mode with VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, false, browser.TypeLacros),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosTabletVKStereoAloopLoaded,
		Desc: "Lacros variant: tablet mode with VK enabled and stereo aloop loaded",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl: inputsFixture(tabletMode, true, false, browser.TypeLacros),
		// Need aloop for route playback to capture.
		Parent:          fixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosTabletVKInGuest,
		Desc: "Lacros variant: tablet mode in guest login with VK enabled",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, false, browser.TypeLacros, guestLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosTabletVKRestart,
		Desc: "Lacros variant: tablet mode with VK enabled restarting chrome session for every test",
		Contacts: []string{
			"alvinjia@google.com",
			"shengjun@chromium.org",
			"essential-inputs-team@google.com",
		},
		Impl:            inputsFixture(tabletMode, true, true, browser.TypeLacros),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellNonVKInGAIA,
		Desc: "Lacros variant: clamshell mode in gaia login with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, false, false, browser.TypeLacros, gaiaLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosAnyVKInGAIA,
		Desc: "Lacros variant: Any mode with VK in gaia login",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(notForced, true, false, browser.TypeLacros, gaiaLogin),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosClamshellVKWithHandWritingLegacyRecognitionOn,
		Desc: "Lacros variant: Clamshell mode with handwriting legacy recognition on",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false, browser.TypeLacros, handwritingLegacyRecognition),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosTabletVKWithHandWritingLegacyRecognitionOn,
		Desc: "Lacros variant: Tablelet mode with handwriting legacy recognition on",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            inputsFixture(clamshellMode, true, false, browser.TypeLacros, handwritingLegacyRecognition),
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
	BrowserType browser.Type
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
	cr          *chrome.Chrome // Underlying Chrome instance
	dm          deviceMode     // Device ui mode to test
	vkEnabled   bool           // Whether virtual keyboard is force enabled
	restart     bool           // Whether restart the fixture after each test
	browserType browser.Type   // Whether Ash or Lacros is used for test
	fOpts       []chromeOpts   // Options that are passed to chrome.New
	tconn       *chrome.TestConn
	recorder    *uiauto.ScreenRecorder
	uc          *useractions.UserContext
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
			opts = append(opts, chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))
		case autocorrectToggle:
			opts = append(opts, chrome.ExtraArgs("--enable-features=AutocorrectToggle"))
		case assistMultiWord:
			opts = append(opts, chrome.ExtraArgs("--enable-features=AssistMultiWord"))
		case diacriticsOnPhysicalKeyboardLongpress:
			opts = append(opts, chrome.ExtraArgs("--enable-features=DiacriticsOnPhysicalKeyboardLongpress,DiacriticsOnPhysicalKeyboardLongpressDefaultOn"))
		case firstPartyVietnameseInput:
			opts = append(opts, chrome.ExtraArgs("--enable-features=FirstPartyVietnameseInput"))
		case handwritingLegacyRecognition:
			opts = append(opts, chrome.ExtraArgs("--enable-features=HandwritingLegacyRecognition"))
		case emojiPickerGifSupport:
			opts = append(opts, chrome.ExtraArgs(("--enable-features=SystemEmojiPickerGIFSupport")))
		case altClickAndSixPackCustomization:
			opts = append(opts, chrome.ExtraArgs("--enable-features=AltClickAndSixPackCustomization"))
		case orca:
			opts = append(opts, chrome.ExtraArgs("--enable-features=OrcaDogfood,MantaService"))
			opts = append(opts, chrome.LacrosEnableFeatures("OrcaDogfood"))
		case picker:
			opts = append(opts, chrome.ExtraArgs("--enable-features=Picker"))
			opts = append(opts, chrome.ExtraArgs("--picker-feature-key="+s.RequiredVar("inputs.Picker.pickerFeatureTestKey")))
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

	cr, err := browserfixt.NewChrome(ctx, f.browserType, lacrosfixt.NewConfig(), opts...)
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
	return FixtData{f.cr, f.tconn, f.uc, f.browserType}
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

func inputsFixture(dm deviceMode, vkEnabled, restart bool, browserType browser.Type, opts ...chromeOpts) testing.FixtureImpl {
	return &inputsFixtureImpl{
		dm:          dm,
		vkEnabled:   vkEnabled,
		restart:     restart,
		browserType: browserType,
		fOpts:       opts,
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
