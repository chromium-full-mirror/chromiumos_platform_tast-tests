// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testserver contains methods to create a local web server for input tests and functions to set / get values of input fields.
package testserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/input/voice"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	"go.chromium.org/tast-tests/cros/local/inputs/util"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InputField is the type of input field.
type InputField string

// ButtonName is the name of the button component.
type ButtonName string

// Different type of input fields.
const (
	TextAreaInputField InputField = "textAreaInputField"
	TextInputField     InputField = "textInputField"
	SearchInputField   InputField = "searchInputField"
	PasswordInputField InputField = "passwordInputField"
	// PasswordTextField is not an editable input.
	// It is used for sync password value for visual testing.
	PasswordTextField              InputField = "passwordTextField"
	NumberInputField               InputField = "numberInputField"
	EmailInputField                InputField = "emailInputField"
	URLInputField                  InputField = "urlInputField"
	TelInputField                  InputField = "telInputField"
	DateInputField                 InputField = "dateInputField"
	MonthInputField                InputField = "monthInputField"
	WeekInputField                 InputField = "weekInputField"
	TimeInputField                 InputField = "timeInputField"
	DateTimeInputField             InputField = "dateTimeInputField"
	TextInputNumericField          InputField = "textInputNumericField"
	TextAreaNoCorrectionInputField InputField = "textArea disabled autocomplete, autocorrect, autocapitalize"
	// These fields are used to test auto-shift (aka autocapitalize).
	TextAreaAutoShiftInSentence InputField = "autocapitalize in sentence mode"
	TextAreaAutoShiftInWord     InputField = "autocapitalize in words mode"
	TextAreaAutoShiftInChar     InputField = "autocapitalize in characters mode"
	TextAreaAutoShiftOff        InputField = "autocapitalize off"
	OffscreenTextField          InputField = "offscreen"

	// This field is used to test GIF insertion from Emoji Picker.
	ContentEditableInputField InputField = "contentEditableInputField"

	// MakeTextButton is used to show password field.
	MakeTextButton ButtonName = "makeTextButton"

	// pageTitle is also the rootWebArea name in A11y to identify the scope of the page.
	pageTitle = "E14s test page"
)

// Inputs test page content.
// TODO(b/196311371) Sync page content with https://sites.google.com/corp/view/e14s-test.
const html = `<!DOCTYPE html>
<meta charset="utf-8">
<title>E14s test page</title>
<pre>No autocomplete</pre>
<textarea aria-label="textArea disabled autocomplete, autocorrect, autocapitalize" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" style="width: 100%"></textarea>
<br /><br />
<pre>&lt;<b>textarea</b> rows="7"&gt;&lt;/textarea&gt;</pre>
<textarea rows="7" aria-label="textAreaInputField" style="width: 100%"></textarea>
<br /><br />
<pre>&lt;input type="<b>text</b>"/&gt;</pre>
<input type="text" aria-label="textInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>search</b>"/&gt;</pre>
<input type="search" aria-label="searchInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>password</b>"/&gt;</pre>
<button onclick="document.getElementById('password-field').type='text'" aria-label="makeTextButton">Make text</button>
<button onclick="document.getElementById('password-field').type='password'">Make password</button>
<input id="password-field" aria-label="passwordInputField" type="password" style="width: 100%" oninput="document.getElementById('e14s-test-password-mirror').value = this.value;">
<br />
<input id="e14s-test-password-mirror" aria-label="passwordTextField" type="text" readonly style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>number</b>"/&gt;</pre>
<input type="number" id="numberInput" aria-label="numberInputField" style="width: 100%" />
<br /><br />
<pre>No spellcheck (should have no autocorrect)</pre>
<textarea spellcheck="false" style="width:100%"></textarea>
<br /><br />
<pre><b>Dark Mode</b></pre>
<textarea rows="7" style="width: 100%;background-color:black;color:#fff"></textarea>
<br /><br />
<pre>&lt;input type="<b>email</b>"/&gt;</pre>
<input type="email" aria-label="emailInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>url</b>"/&gt;</pre>
<input type="url" aria-label="urlInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>tel</b>"/&gt;</pre>
<input type="tel" aria-label="telInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>date</b>"/&gt;</pre>
<input type="date" aria-label="dateInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>month</b>"/&gt;</pre>
<input type="month" aria-label="monthInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>week</b>"/&gt;</pre>
<input type="week" aria-label="weekInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>time</b>"/&gt;</pre>
<input type="time" aria-label="timeInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type="<b>datetime-local</b>"/&gt;</pre>
<input type="datetime-local" aria-label="dateTimeInputField" style="width: 100%" />
<br /><br />
<pre>&lt;input type=”text” inputmode=”numeric” pattern="[0-9]*"/&gt; (UK gov suggested numeric input for A11y)</pre>
<input type="text" inputmode="numeric" aria-label="textInputNumericField"/>
<br /><br />
<pre>&lt;autocapitalize: sentences"/&gt;</pre>
<textarea rows="3" aria-label="autocapitalize in sentence mode" autocapitalize="sentences" style="width: 100%"></textarea>
<br /><br />
<pre>&lt;autocapitalize: words"/&gt;</pre>
<textarea rows="3" aria-label="autocapitalize in words mode" autocapitalize="words" style="width: 100%"></textarea>
<br /><br />
<pre>&lt;autocapitalize: characters"/&gt;</pre>
<textarea rows="3" aria-label="autocapitalize in characters mode" autocapitalize="characters" style="width: 100%"></textarea>
<br /><br />
<pre>&lt;autocapitalize: off"/&gt;</pre>
<textarea rows="3" aria-label="autocapitalize off" autocapitalize="none" style="width: 100%"></textarea>
<br /><br />
<textarea style="position: absolute; top: 6000px; height: 30px; width: 100%;" aria-label="offscreen"></textarea>

<pre>&lt;div contentEditable="true" /&gt;</pre>
<div id="content-editable-editing-field" aria-label="contentEditableInputField"
     contenteditable="true" style="border: 1px solid black; min-height: 30px"></div>
<script type="text/javascript">
  window.addEventListener("load", () => {
    const editingField = document.querySelector("#content-editable-editing-field");
    editingField.addEventListener("paste", (evt) => {
      if (evt.clipboardData.items) {
        for (const item of evt.clipboardData.items) {
          if (item.kind === "file" && item.type.startsWith("image/")) {
            const imageElement = document.createElement("img");
            imageElement.src = URL.createObjectURL(item.getAsFile());
            editingField.appendChild(imageElement);
            return evt.preventDefault();
          }
        }
      }
    });
  });
</script>
`

// InputsTestServer is an unified server instance being used to manage web server and connection.
type InputsTestServer struct {
	server *httptest.Server
	cr     *chrome.Chrome
	tconn  *chrome.TestConn
	// Page connection. It is connected when loading the test page.
	// It is used for evaluate javascript.
	pc *chrome.Conn
	ui *uiauto.Context
}

// FieldInputEval encapsulates a function to input text into an input field, and its expected output.
type FieldInputEval struct {
	InputField   InputField
	InputFunc    uiauto.Action
	ExpectedText string
}

// pageRootFinder is the finder of root Node of the test page.
// All sub node should be located on the page.
var pageRootFinder = nodewith.Name(pageTitle).Role(role.RootWebArea)

// Finder returns the finder of the field by Name attribute.
func (inputField InputField) Finder() *nodewith.Finder {
	return nodewith.Ancestor(pageRootFinder).Name(string(inputField))
}

// LaunchServer launches the server that serves e14s-test page.
func LaunchServer(ctx context.Context) (server *httptest.Server) {
	testing.ContextLog(ctx, "Start a local server to test inputs")

	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, html)
	}))

	return newServer
}

// LaunchBrowserInMode launches a local web server to serve inputs testing on
// different type of input fields.
// It can be either normal user mode or incognito mode.
func LaunchBrowserInMode(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, incognitoMode bool) (its *InputsTestServer, err error) {
	return LaunchBrowserWithHTML(ctx, incognitoMode, cr, tconn, html)
}

// LaunchBrowser launches a local web server with the default html to serve
// inputs testing on different type of input fields.
func LaunchBrowser(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*InputsTestServer, error) {
	return LaunchBrowserWithHTML(ctx, false, cr, tconn, html)
}

// LaunchBrowserWithHTML launches a local web server with the specified html to
// serve inputs testing on different type of input fields.
func LaunchBrowserWithHTML(ctx context.Context, incognitoMode bool, cr *chrome.Chrome, tconn *chrome.TestConn, rawHTML string) (*InputsTestServer, error) {
	// URL path needs to be in the allowlist to enable some features.
	// https://source.chromium.org/chromium/chromium/src/+/main:chrome/browser/ash/input_method/assistive_suggester.cc.
	const urlPath = "e14s-test"
	testing.ContextLog(ctx, "Start a local server to test inputs")
	hasError := true

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, rawHTML)
	}))
	defer func() {
		if hasError {
			server.Close()
		}
	}()

	browserConn, err := setUpBrowser(ctx, incognitoMode, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to browser")
	}
	defer func() {
		if hasError {
			if err := browserConn.Close(); err != nil {
				testing.ContextLog(ctx, "Failed to close browser connection: ", err)
			}
		}
	}()

	if err = browserConn.Navigate(ctx, server.URL+"/"+urlPath); err != nil {
		return nil, errors.Wrapf(err, "failed to navigate to %q", server.URL)
	}

	activeWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get active window")
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, activeWindow.ID, ash.WindowStateMaximized); err != nil {
		return nil, errors.Wrap(err, "failed to maximize Chrome window")
	}

	if err := webutil.WaitForQuiescence(ctx, browserConn, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to load test page")
	}

	ui := uiauto.New(tconn)
	// Even document is ready, target is not yet in a11y tree.
	if err := ui.WaitUntilExists(pageRootFinder)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to render test page")
	}

	hasError = false
	return &InputsTestServer{
		server: server,
		cr:     cr,
		tconn:  tconn,
		pc:     browserConn,
		ui:     ui,
	}, nil
}

func setUpBrowser(ctx context.Context, incognitoMode bool, cr *chrome.Chrome) (*chrome.Conn, error) {
	switch incognitoMode {
	case true:
		// Launch an incognito browser using keyboard shortcut `Ctrl+Shift+N`.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to a keyboard")
		}
		defer kb.Close(ctx)
		if err := kb.Accel(ctx, "Ctrl+Shift+N"); err != nil {
			return nil, errors.Wrap(err, "failed to launch incognito Chrome browser")
		}
		return cr.NewConnForTarget(ctx, chrome.MatchTargetURL(chrome.NewTabURL))
	default:
		return cr.NewConn(ctx, chrome.NewTabURL)
	}
}

// CloseAll releasees the connection, stops the local web server, and closees
// the browser.
// TODO(b/230416109) Merge CloseAll and Close after adding the lacros variants.
func (its *InputsTestServer) CloseAll(ctx context.Context) {
	if err := its.pc.Close(); err != nil {
		testing.ContextLog(ctx, "Failed to close browser connection: ", err)
	}
	its.server.Close()
}

// Close release the connection and stop the local web server.
func (its *InputsTestServer) Close() {
	its.pc.Close()
	its.server.Close()
}

// Clear returns an action clearing given input field by setting value to empty string via javascript.
func (its *InputsTestServer) Clear(inputField InputField) uiauto.Action {
	if inputField == ContentEditableInputField {
		return func(ctx context.Context) error {
			return its.pc.Eval(ctx, fmt.Sprintf(`document.querySelector("*[aria-label='%s']").innerHTML=''`, inputField), nil)
		}
	}
	return func(ctx context.Context) error {
		return its.pc.Eval(ctx, fmt.Sprintf(`document.querySelector("*[aria-label='%s']").value=''`, inputField), nil)
	}
}

// ScrollTo returns an action that scrolls to a given coordinate in the page
// via javascript.
func (its *InputsTestServer) ScrollTo(x, y int32) uiauto.Action {
	return func(ctx context.Context) error {
		return its.pc.Eval(ctx, fmt.Sprintf(`window.scrollTo(%d, %d);`, x, y), nil)
	}
}

// ScrollIntoView returns an action that scrolls into a given input field
// and aligns it with the view edges via javascript.
func (its *InputsTestServer) ScrollIntoView(inputField InputField, alignToTop bool) uiauto.Action {
	return func(ctx context.Context) error {
		return its.pc.Eval(ctx, fmt.Sprintf(`document.querySelector("*[aria-label='%s']").scrollIntoView(%s);`, inputField, strconv.FormatBool(alignToTop)), nil)
	}
}

// WaitForFieldToBeActive returns an action waiting for certain input field to be the active element.
func (its *InputsTestServer) WaitForFieldToBeActive(inputField InputField) uiauto.Action {
	waitForFieldAction := func(ctx context.Context) error {
		return its.pc.WaitForExprFailOnErrWithTimeout(ctx,
			fmt.Sprintf(`!!document.activeElement && document.querySelector("*[aria-label='%s']")===document.activeElement`,
				inputField), 3*time.Second)
	}

	return uiauto.Combine("... wait for input field to be active",
		waitForFieldAction,
		// The following sleep is required to avoid a race condition
		// between OnFocus events and the first keypress event (or other
		// input event) in the focused input. There have been cases of
		// flakey tests resulting from a race condition like this
		// recently, where the first keypress is entered before the
		// input method has been fully initialized via the respective
		// OnFocus event. The sleep has been added to this method as
		// input actions are likely to follow an invocation of this
		// method. See b/235417796 for more.
		uiauto.Sleep(500*time.Millisecond),
	)
}

// WaitUntilFieldContainsImages returns an action waiting for images inserted in certain input field.
func (its *InputsTestServer) WaitUntilFieldContainsImages(inputField InputField) uiauto.Action {
	return func(ctx context.Context) error {
		return its.pc.WaitForExprWithTimeout(ctx,
			fmt.Sprintf(`document.querySelectorAll("*[aria-label='%s'] img").length > 0`, inputField), 3*time.Second)
	}
}

// ClickFieldAndWaitForActive returns an action clicking the input field and waiting for it to be active.
func (its *InputsTestServer) ClickFieldAndWaitForActive(inputField InputField) uiauto.Action {
	return uiauto.RetrySilently(3,
		uiauto.Combine(
			"click input field and wait for it to be active",
			its.ClickField(inputField),
			its.WaitForFieldToBeActive(inputField),
		))
}

// ClearThenClickFieldAndWaitForActive returns an action clearing the input field, clicking it and waiting for it to be active.
func (its *InputsTestServer) ClearThenClickFieldAndWaitForActive(inputField InputField) uiauto.Action {
	return uiauto.Combine(
		"clear input field, click it, and wait for it to be active",
		its.Clear(inputField),
		its.ClickFieldAndWaitForActive(inputField),
	)
}

// ClickField returns an action clicking the input field.
func (its *InputsTestServer) ClickField(inputField InputField) uiauto.Action {
	fieldFinder := inputField.Finder()
	return uiauto.Combine(
		"make input field visible on the screen and click it",
		its.ui.WaitUntilExists(fieldFinder),
		its.ui.MakeVisible(fieldFinder),
		its.ui.LeftClick(fieldFinder),
	)
}

// ClickFieldByDoDefault returns an action invoking the default action on the input field.
// This is equivalent to ClickField but uses DoDefault (accessibility action) instead of a physical LeftClick.
func (its *InputsTestServer) ClickFieldByDoDefault(inputField InputField) uiauto.Action {
	fieldFinder := inputField.Finder()
	return uiauto.Combine(
		"make input field visible on the screen and invoke default action on it",
		its.ui.WaitUntilExists(fieldFinder),
		its.ui.MakeVisible(fieldFinder),
		its.ui.DoDefault(fieldFinder),
	)
}

// RightClickFieldAndWaitForActive returns an action right clicking the input field.
func (its *InputsTestServer) RightClickFieldAndWaitForActive(inputField InputField) uiauto.Action {
	fieldFinder := inputField.Finder()
	return uiauto.RetrySilently(3, uiauto.Combine(
		"right click input field and wait for it to be active",
		its.ui.WaitUntilExists(fieldFinder),
		its.ui.MakeVisible(fieldFinder),
		its.ui.RightClick(fieldFinder),
		its.WaitForFieldToBeActive(inputField),
	))
}

// ClickFieldUntilVKShown returns an action clicking the input field and waits for the virtual keyboard to show up.
func (its *InputsTestServer) ClickFieldUntilVKShown(inputField InputField) uiauto.Action {
	fieldFinder := inputField.Finder()
	return uiauto.Combine(
		"make input field visible on the screen and click it until virtual keyboard is shown",
		its.ui.WaitUntilExists(fieldFinder),
		its.ui.MakeVisible(fieldFinder),
		// Use vkb.ClickUntilVKShown because it has retry internally.
		vkb.NewContext(its.cr, its.tconn).ClickUntilVKShown(fieldFinder),
	)
}

// ValidateInputOnField returns an action to test an input action on given input field.
// It clears field first and click to activate input.
// After input action, it checks whether the outcome equals to expected value.
func (its *InputsTestServer) ValidateInputOnField(inputField InputField, inputFunc uiauto.Action, expectedValue string) uiauto.Action {
	return uiauto.Combine("validate input function on field "+string(inputField),
		its.Clear(inputField),
		its.ClickFieldAndWaitForActive(inputField),
		inputFunc,
		util.WaitForFieldTextToBeIgnoringCase(its.tconn, inputField.Finder(), expectedValue),
	)
}

func (its *InputsTestServer) validatePKTypingInField(uc *useractions.UserContext, inputField InputField, inputData data.InputData) uiauto.Action {
	action := func(ctx context.Context) error {
		// This is either an actual PK device, or a PK simulator for injecting
		// key codes.
		keyboard, err := input.Keyboard(ctx)
		if err != nil {
			return err
		}
		defer keyboard.Close(ctx)

		return uiauto.Combine("validate pk input function on field "+string(inputField),
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			keyboard.TypeSequenceAction(inputData.LocationKeySeq),
			func(ctx context.Context) error {
				if inputData.SubmitFromSuggestion {
					return keyboard.Accel(ctx, "space")
				}
				return nil
			},
		)(ctx)
	}

	return uiauto.UserAction(
		"PK typing input",
		action,
		uc,
		&useractions.UserActionCfg{
			ValidateResult: its.ValidateResult(inputField, inputData.ExpectedText),
			Attributes: map[string]string{
				useractions.AttributeFeature:    useractions.FeaturePKTyping,
				useractions.AttributeInputField: string(inputField),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

func (its *InputsTestServer) validateVKTypingInField(uc *useractions.UserContext, inputField InputField, inputData data.InputData) uiauto.Action {
	vkbCtx := vkb.NewContext(its.cr, its.tconn)
	action := uiauto.Combine("validate vk input function on field "+string(inputField),
		its.CleanFieldAndTriggerVK(inputField),
		vkbCtx.TapKeysIgnoringCase(inputData.CharacterKeySeq),
		func(ctx context.Context) error {
			if inputData.SubmitFromSuggestion {
				return vkbCtx.SelectFromSuggestion(inputData.ExpectedText)(ctx)
			}
			return nil
		},
	)
	return uiauto.UserAction(
		"VK typing input",
		action,
		uc,
		&useractions.UserActionCfg{
			ValidateResult: its.ValidateResult(inputField, inputData.ExpectedText),
			Attributes: map[string]string{
				useractions.AttributeFeature:    useractions.FeatureVKTyping,
				useractions.AttributeInputField: string(inputField),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

func (its *InputsTestServer) validateVoiceInField(uc *useractions.UserContext, inputField InputField, inputData data.InputData, dataPath func(string) string) uiauto.Action {
	action := func(ctx context.Context) error {
		// Setup CRAS Aloop for audio test.
		if err := voice.ActivateAloopNodes(ctx, its.tconn, voice.LoopbackPlayBack, voice.LoopbackCapture); err != nil {
			return err
		}

		vkbCtx := vkb.NewContext(its.cr, its.tconn)
		return uiauto.Combine("validate vk voice input function on field "+string(inputField),
			its.CleanFieldAndTriggerVK(inputField),
			vkbCtx.SwitchToVoiceInput(),
			func(ctx context.Context) error {
				return voice.AudioFromFile(ctx, dataPath(inputData.VoiceFile), 0 /*duration*/)
			},
		)(ctx)
	}
	return uiauto.UserAction(
		"Voice input",
		action,
		uc,
		&useractions.UserActionCfg{
			ValidateResult: its.ValidateResult(inputField, inputData.ExpectedText),
			Attributes: map[string]string{
				useractions.AttributeFeature:    useractions.FeatureVoiceInput,
				useractions.AttributeInputField: string(inputField),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

func (its *InputsTestServer) validateHandwritingInField(uc *useractions.UserContext, inputField InputField, inputData data.InputData, dataPath func(string) string, expectLongform bool) uiauto.Action {
	action := func(ctx context.Context) error {
		vkbCtx := vkb.NewContext(its.cr, its.tconn)
		if err := its.CleanFieldAndTriggerVK(inputField)(ctx); err != nil {
			return err
		}

		hwCtx, err := vkbCtx.SwitchToHandwriting(ctx, expectLongform)
		if err != nil {
			return err
		}

		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
		defer cancel()
		defer hwCtx.SwitchToKeyboard()(cleanupCtx)

		return uiauto.Combine("handwriting input on virtual keyboard",
			its.WaitForHandwritingEngineReadyOnField(hwCtx, inputField, dataPath(inputData.HandwritingFile)),
			hwCtx.DrawStrokesFromFile(dataPath(inputData.HandwritingFile)),
			// Wait for and validate handwriting result. Note that this needs to occur before cleanup
			// switches back to the main keyboard, since that can interrupt handwriting recogition.
			its.ValidateResult(inputField, inputData.ExpectedText),
		)(ctx)
	}
	return uiauto.UserAction(
		"Handwriting",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:    useractions.FeatureHandWriting,
				useractions.AttributeInputField: string(inputField),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// WaitForHandwritingEngineReadyOnField tries handwriting until the field is not empty.
func (its *InputsTestServer) WaitForHandwritingEngineReadyOnField(hwCtx *vkb.HandwritingContext, inputField InputField, dataPathStr string) uiauto.Action {
	// Warm-up steps to check handwriting engine ready.
	checkEngineReady := uiauto.Combine("wait for handwriting engine to be ready",
		hwCtx.DrawFirstStrokeFromFile(dataPathStr),
		util.WaitForFieldNotEmpty(its.tconn, inputField.Finder()),
		hwCtx.ClearHandwritingCanvas(),
		its.Clear(inputField),
	)
	return hwCtx.WaitForHandwritingEngineReady(checkEngineReady)
}

// ValidateInputFieldForMode tests input in the given field.
// After input action, it checks whether the outcome equals to expected value.
func (its *InputsTestServer) ValidateInputFieldForMode(uc *useractions.UserContext, inputField InputField, inputModality util.InputModality, inputData data.InputData, dataPath func(string) string) uiauto.Action {
	if !inputField.isSupported(inputModality) {
		return func(ctx context.Context) error {
			return errors.Errorf("%s is not supported for %s", inputModality, inputField)
		}
	}

	// TODO(b/195083581): Enable ValidateInputFieldForMode for physical keyboard and emoji.
	switch inputModality {
	case util.InputWithVK:
		return its.validateVKTypingInField(uc, inputField, inputData)
	case util.InputWithVoice:
		return its.validateVoiceInField(uc, inputField, inputData, dataPath)
	case util.InputWithHandWriting:
		return its.validateHandwritingInField(uc, inputField, inputData, dataPath, false)
	case util.InputWithHandWritingLongForm:
		return its.validateHandwritingInField(uc, inputField, inputData, dataPath, true)
	case util.InputWithPK:
		return its.validatePKTypingInField(uc, inputField, inputData)
	}

	return func(ctx context.Context) error {
		return errors.Errorf("input modality not supported: %q", inputModality)
	}
}

func (inputField InputField) isSupported(inputModality util.InputModality) bool {
	if inputField == PasswordInputField {
		if inputModality == util.InputWithHandWriting || inputModality == util.InputWithHandWritingLongForm || inputModality == util.InputWithVoice {
			return false
		}
	}
	return true
}

// CleanFieldAndTriggerVK returns an action to clear a given input field and trigger the vk.
func (its *InputsTestServer) CleanFieldAndTriggerVK(inputField InputField) uiauto.Action {
	vkbCtx := vkb.NewContext(its.cr, its.tconn)
	return uiauto.Combine("clean and trigger VK on field "+string(inputField),
		vkbCtx.HideVirtualKeyboard(),
		its.Clear(inputField),
		its.ClickFieldUntilVKShown(inputField),
	)
}

// ValidateResult returns an action to validate input field text on test server.
// It deals with Password field especially to validate both displayed placebolder and actual text.
func (its *InputsTestServer) ValidateResult(inputField InputField, expectedText string) uiauto.Action {
	validateField := util.WaitForFieldTextToBeIgnoringCase(its.tconn, inputField.Finder(), expectedText)
	if inputField == PasswordInputField {
		// Password input is a special case. The value is presented with placeholder "•".
		// Using PasswordTextField field to verify the outcome.
		validateField = uiauto.Combine("validate passward field",
			util.WaitForFieldTextToBe(its.tconn, inputField.Finder(), strings.Repeat("•", len(expectedText))),
			util.WaitForFieldTextToBeIgnoringCase(its.tconn, PasswordTextField.Finder(), expectedText),
		)
	}
	return validateField
}

// Conn returns the browser driver used in the test page.
func (its *InputsTestServer) Conn() *chrome.Conn {
	return its.pc
}

// RetrieveTextSelectionInfo is a wrapper of uiauto.RetrieveTextSelectionInfo
// to simplify the use for inputs tests.
func (its *InputsTestServer) RetrieveTextSelectionInfo(ctx context.Context, conn *chrome.Conn, inputField InputField) (*uiauto.TextSelectionInfo, error) {
	return its.ui.RetrieveTextSelectionInfo(ctx, its.pc, inputField.Finder())
}

// ClickButton returns an action to click the given button.
func (its *InputsTestServer) ClickButton(buttonName ButtonName) uiauto.Action {
	return its.ui.LeftClick(nodewith.Ancestor(pageRootFinder).Name(string(buttonName)))
}
