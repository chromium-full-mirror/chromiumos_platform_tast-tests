// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package docscuj

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
)

// paragraph includes all the information needed to type a paragraph
// in the open Google Doc.
type paragraph struct {
	// description is a short description of what we are typing.
	description string

	// body is the actual string we are typing. This value will be
	// ignored if |customTypeAction| is non-nil in this struct.
	body string

	// language is the language that should be active at the time of
	// typing this paragraph.
	language ime.InputMethod

	// customTypeAction is an action that types the paragraph. If this
	// is nil, then |body| must be non-nil, and the paragraph is
	// typed using |typeParagraphAction|, which simply types the
	// paragraph with the keyboard at a given WPM.
	customTypeAction action.Action

	// setUp performs any setup actions that are needed before writing
	// the paragraph. Activation of the paragraph language does not
	// need to be done in setUp, as the language is activated before
	// calling customTypeAction or the default typeParagraphAction.
	setUp action.Action

	// cleanUp performs any cleanup actions that are needed to limit
	// anything that could affect typing further paragraphs.
	cleanUp action.Action

	// tracingCfg indicates which perfetto tracing config to use to collect a
	// trace of this paragraph. No trace is collected if empty.
	tracingCfg string
}

// These values for typing speed were chosen to be equidistant, and to
// reflect generally higher WPM counts than the average population,
// because typing faster would hit the device harder and might give
// better performance data.
const (
	fastTypingSpeed   = 120
	mediumTypingSpeed = 90
	slowTypingSpeed   = 60
)

// englishString is a custom English paragraph with accelerators
// scattered throughout in the format described by typeParagraphAction.
const englishString = "The recorder is a [Ctrl+I]collection utility[Ctrl+I] - it does not generate the metrics itself. " +
	"The recorder comes with a set of predefined metrics spanning multiple areas of performance to help " +
	"with development. Usage of the recorder can be broken up into [Ctrl+B]3 sections[Ctrl+B]: [Ctrl+I]" +
	"Setup, Test Running, and Cleanup[Ctrl+I].[Enter][Ctrl+B]Step 1:[Ctrl+B] Create a new recorder with " +
	"cujrecorder.NewRecorder. [Enter][Ctrl+B]Step 2:[Ctrl+B] Configure additional options for the recorder. " +
	"[Enter][Ctrl+B]Step 3:[Ctrl+B] Add desired metrics to the recorder."

// Create a separate virtual keyboard string that contains only
// keys visible on the simplified keyboard layout. The exact keyboard
// layout that appears is sometimes inconsistent, so keeping the string
// relatively simple can help prevent problems finding certain keys.
const virtualKeyboardString = "This is my virtual keyboard paragraph written entirely in English. Im not totally " +
	"sure what Im supposed to be writing here, just that the string should be about 300 characters long. Im about " +
	"200 characters in, and if I keep mentioning how many characters Ive written, Ill hit 300 in no time."

// getParagraphs returns a list of paragraphs that will be typed,
// including their corresponding input language. The current list of
// paragraphs include basic English, Lorem Ipsum, Chinese, Korean,
// Japanese, and a paragraph in pageless mode.
func getParagraphs(pc pointer.Context, cr *chrome.Chrome, tconn *chrome.TestConn, docsConn *chrome.Conn, ac *uiauto.Context, kw *input.KeyboardEventWriter) []paragraph {
	vkbCtx := vkb.NewContext(cr, tconn)
	return []paragraph{
		{
			description: fmt.Sprintf("English WPM %d", slowTypingSpeed),
			body:        englishString,
			language:    ime.DefaultInputMethod,
		},
		{
			description: fmt.Sprintf("English WPM %d", mediumTypingSpeed),
			body:        englishString,
			language:    ime.DefaultInputMethod,
		},
		{
			description: fmt.Sprintf("English WPM %d", fastTypingSpeed),
			body:        englishString,
			language:    ime.DefaultInputMethod,
			tracingCfg:  cujrecorder.SystemTraceConfigFile,
		},
		{
			description: "Lorem Ipsum",
			body: "Lorem ipsum [Ctrl+I]dolor[Ctrl+I] sit amet, consectetur adipiscing elit. [Ctrl+U]Vivamus " +
				"condimentum rhoncus est volutpat venenatis.[Ctrl+U] Fusce semper, sapien ut venenatis " +
				"pellentesque, lorem dui aliquam sapien, non pharetra diam neque id mi. [Ctrl+B]Suspendisse " +
				"sollicitudin[Ctrl+B], metusut gravida semper, nunc ipsum ullamcorper nunc, ut maximus nulla " +
				"nunc dignissim justo. Duis nec nisi leo.",
			language: ime.DefaultInputMethod,
		},
		{
			description: "Chinese Simplified",
			body: "si nian ling qi nian qian , women de fubei zai zhege dalu shang danshengle yige xin guojia, " +
				"ta yunyu yu ziyou , zhili yu ren ren sheng er pingdeng de zhuzhang . xianzai women zhengzai " +
				"jinxing yi chang weida de neizhan , kaoyanzhe zhege guojia , huozhe renhe yige ruci gouxiang he " +
				"ruci xianshen de guojia , shifou nenggou changqi cunzai . women zai na chang zhanzheng de da " +
				"zhanchang shang xiang yu . ",
			language: ime.ChinesePinyin,
		},
		{
			description: "Korean",
			body: "nyeon jeon uliui josangdeul-eun i daelyug-e jayu an-eseo ingtaedoego modeun salam-i " +
				"pyeongdeunghage changjodoeeossdaneun myeongjee jeonnyeomhaneun saeloun guggaleul " +
				"tansaengsikyeossseubnida . jigeum ulineun widaehan naejeon-eul beol-igo iss-eumyeo , geu nala " +
				"ttoneun geuleohge saeng-gagdoego heonsindoen eotteon nalaga olae jisogdoel su issneunji " +
				"siheomhago issseubnida . ulineun geu jeonjaeng-ui keun jeonjaengteoeseo mannassseubnida .",
			language: ime.Korean,
		},
		{
			description: "Japanese",
			body: "40 to 7-nen mae, watashitachi no senzo wa kono tairiku ni atarashi kokka o umidashimashita. " +
				"kono atarashi kokka wa , jiyu ni umare , subete no hito ga byodo ni tsukura rete iru to iu meidai " +
				"ni sen'nen shite imasu . genzai , watashitachi wa okina naisen ni makikoma rete ori , sono kuni , " +
				"matawa sonoyoni kangae rare , kenshin-tekina kuni ga nagaku taeru koto ga dekiru ka do ka o tesuto " +
				"shite imasu .[Enter]",
			language: ime.Japanese,
		},
		{
			description: "Pageless English",
			body:        englishString,
			language:    ime.DefaultInputMethod,
			setUp:       googledocs.UpdatePagelessModeAction(pc, docsConn, ac, kw, true),
			cleanUp:     googledocs.UpdatePagelessModeAction(pc, docsConn, ac, kw, false),
		},
		{
			description: "Virtual Keyboard English",
			language:    ime.DefaultInputMethod,
			customTypeAction: action.Combine(
				"type with virtual keyboard and press Enter",
				vkbCtx.TypeIgnoreCaseAction(pc, tconn, virtualKeyboardString),
				kw.AccelAction("Enter"),
			),
			setUp: action.Combine(
				"enable and activate the virtual keyboard",
				vkbCtx.EnableA11yVirtualKeyboard(true),
				vkbCtx.ShowVirtualKeyboard(),
				// Wait for a specific key on the keyboard to exist, to
				// ensure the keyboard has properly loaded. Sometimes,
				// ShowVirtualKeyboard completes before the keyboard
				// has completed loading.
				ac.WaitUntilExists(vkb.KeyFinder.Name("space")),
			),
			cleanUp: action.Combine(
				"disable virtual keyboard and wait until it is gone",
				vkbCtx.EnableA11yVirtualKeyboard(false),
				vkbCtx.WaitUntilHidden(),
			),
		},
	}
}

// typeParagraphAction types |p| using keyboard |kw| at a given |wpm|.
// |p| as a string has a special format that allows for inputting
// keyboard shortcuts in the middle of the text. Any series of
// characters within open and closed square brackets are assumed to
// be an accelerator, and are pressed together. For example, if in
// the middle of the string there is "[Ctrl+B]", typeParagraphAction
// will press Ctrl+B on the keyboard, essentially bolding the text.
// If the text is "Hello [Ctrl+I]World[Ctrl+I][Enter]", then
// "Hello World" will be typed, with "World" italicized and with the
// cursor ending on a new line.
func typeParagraphAction(ctx context.Context, kw *input.KeyboardEventWriter, p string, wpm int) action.Action {
	// phrase is the current portion of the string |p| to type.
	var phrase []string

	// actions is a list of actions that make up the overall action of
	// typing |p|.
	var actions []action.Action

	// appendPhraseAction is an action that types |phrase|, and resets
	// |phrase| to be empty after typing.
	appendPhraseAction := func(ctx context.Context) {
		if len(phrase) > 0 {
			actions = append(actions, inputsimulations.TypeSequenceWPMAction(ctx, kw, wpm, phrase))
			phrase = nil
		}
	}

	// Run through each character. Accumulate characters within
	// |phrase|, and use the |phrase| differently depending on if we
	// are outside of square brackets, or inside them.
	for _, c := range strings.Split(p, "") {
		switch c {
		case "[":
			// If we reached a square bracket, create an action to type
			// everything before that bracket.
			appendPhraseAction(ctx)
		case "]":
			// If we are at the end of the accelerator, create an action to
			// press that exact accelerator.
			actions = append(actions, action.Combine(
				fmt.Sprintf("type %q and sleep", phrase),
				kw.AccelAction(strings.Join(phrase, "")),
				action.Sleep(20*time.Millisecond),
			))
			phrase = nil
		default:
			// Otherwise, add the current character to the active phrase.
			phrase = append(phrase, c)
		}
	}

	// Ensure to type any remaining characters.
	appendPhraseAction(ctx)

	return action.Combine(
		"type paragraph",
		actions...,
	)
}
