// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/data"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/apps"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InputModality describes the available input modalities.
type InputModality string

// Valid values for InputModality.
const (
	InputWithVK          InputModality = "Virtual Keyboard"
	InputWithVoice       InputModality = "Voice"
	InputWithHandWriting InputModality = "Handwriting"
	InputWithPK          InputModality = "Physical Keyboard"
)

// PKCandidatesFinder is the finder for candidates in the IME candidates window.
var PKCandidatesFinder = nodewith.Role(role.ImeCandidate).Onscreen()

// InputEval is a data structure to define common input function and expected out.
type InputEval struct {
	TestName     string
	InputFunc    uiauto.Action
	ExpectedText string
}

// AppCompatTestCase is a data structure to define test case for app compat test.
type AppCompatTestCase struct {
	TestName    string
	Description string
	Steps       uiauto.Action
}

// UIDetectionOptions is a data structure to define options for UIDetection.
type UIDetectionOptions struct {
	ApproxMatch bool
	UseRegex    bool
}

// WaitForFieldTextToBe returns an action checking whether the input field value equals given text.
// The text is case sensitive.
func WaitForFieldTextToBe(tconn *chrome.TestConn, finder *nodewith.Finder, expectedText string) uiauto.Action {
	return WaitForFieldTextToSatisfy(tconn, finder, expectedText, func(actualText string) bool {
		return expectedText == actualText
	})
}

// WaitForFieldTextToBeIgnoringCase returns an action checking whether the input field value equals given text.
// The text is case insensitive.
func WaitForFieldTextToBeIgnoringCase(tconn *chrome.TestConn, finder *nodewith.Finder, expectedText string) uiauto.Action {
	return WaitForFieldTextToSatisfy(tconn, finder, fmt.Sprintf("%s (ignoring case)", expectedText), func(actualText string) bool {
		return strings.ToLower(expectedText) == strings.ToLower(actualText)
	})
}

// WaitForFieldTextStartWithIgnoringCase returns an action checking whether the input field value start with the given text.
// The text is case insensitive.
func WaitForFieldTextStartWithIgnoringCase(tconn *chrome.TestConn, finder *nodewith.Finder, expectedText string) uiauto.Action {
	return WaitForFieldTextToSatisfy(tconn, finder, fmt.Sprintf("%s (ignoring case)", expectedText), func(actualText string) bool {
		return strings.HasPrefix(strings.ToLower(actualText), strings.ToLower(expectedText))
	})
}

// WaitForFieldTextToSatisfy returns an action checking whether the input field value satisfies a predicate.
func WaitForFieldTextToSatisfy(tconn *chrome.TestConn, finder *nodewith.Finder, description string, predicate func(string) bool) uiauto.Action {
	ui := uiauto.New(tconn).WithInterval(time.Second)
	return uiauto.Combine("validate field text",
		// Sleep 200ms before validating text field.
		// Without sleep, it almost never pass the first time check due to the input delay.
		uiauto.Sleep(200*time.Millisecond),
		ui.RetrySilently(8, func(ctx context.Context) error {
			nodeInfo, err := ui.Info(ctx, finder)
			if err != nil {
				return err
			}

			if !predicate(nodeInfo.Value) {
				return errors.Errorf("failed to validate input value: got: %s; want: %s", nodeInfo.Value, description)
			}

			return nil
		}))
}

// WaitForFieldNotEmpty returns an action checking whether the input field value is not empty.
func WaitForFieldNotEmpty(tconn *chrome.TestConn, finder *nodewith.Finder) uiauto.Action {
	return WaitForFieldTextToSatisfy(tconn, finder, "not empty", func(actualText string) bool {
		return actualText != ""
	})
}

// WaitForFieldEmpty returns an action checking whether the input field value is empty.
func WaitForFieldEmpty(tconn *chrome.TestConn, finder *nodewith.Finder) uiauto.Action {
	return WaitForFieldTextToSatisfy(tconn, finder, "empty", func(actualText string) bool {
		return actualText == ""
	})
}

// WaitForFieldSelectionToBe returns an action checking whether the input field text selection equals given range.
func WaitForFieldSelectionToBe(conn *chrome.Conn, tconn *chrome.TestConn, finder *nodewith.Finder, expectedSelectionStart, expectedSelectionEnd int, expectedSelectedText string) uiauto.Action {
	ui := uiauto.New(tconn).WithInterval(time.Second)
	return uiauto.Combine("validate field selection",
		// Sleep 200ms before validating text field.
		// Without sleep, it almost never pass the first time check due to the input delay.
		uiauto.Sleep(200*time.Millisecond),
		ui.RetrySilently(8, func(ctx context.Context) error {
			selectionInfo, err := ui.RetrieveTextSelectionInfo(ctx, conn, finder)
			if err != nil {
				return err
			}

			if selectionInfo.StartIndex == expectedSelectionStart && selectionInfo.EndIndex == expectedSelectionEnd && selectionInfo.Text == expectedSelectedText {
				return nil
			}

			return errors.Errorf("failed to validate selection: got: %d %d %s; want: %d %d %s", selectionInfo.StartIndex, selectionInfo.EndIndex, selectionInfo.Text, expectedSelectionStart, expectedSelectionEnd, expectedSelectedText)
		}))
}

// GetNthCandidateText returns the candidate text in the specified position in the candidates window.
func GetNthCandidateText(ctx context.Context, tconn *chrome.TestConn, n int) (string, error) {
	ui := uiauto.New(tconn)
	var lastText, curText string

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		candidate, err := ui.Info(ctx, PKCandidatesFinder.Nth(n))
		if err != nil {
			return err
		}
		curText = candidate.Name
		if curText != lastText {
			lastText = curText
			return errors.New("text is not stable between internval")
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		return "", err
	}

	return curText, nil
}

// WaitForNCandidates waits until there are exactly n candidates in the candidates window.
func WaitForNCandidates(tconn *chrome.TestConn, n int) uiauto.Action {
	ui := uiauto.New(tconn)

	return func(ctx context.Context) error {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			candidate, err := ui.NodesInfo(ctx, PKCandidatesFinder)
			if err != nil {
				return err
			}
			if len(candidate) != n {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 2 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
			return err
		}

		return nil
	}
}

// RunSubtestsPerInputMethodAndMessage runs subtest that uses testName and inputdata on
// every combination of given input methods and messages.
func RunSubtestsPerInputMethodAndMessage(ctx context.Context, uc *useractions.UserContext, s *testing.State,
	inputMethods []ime.InputMethod, messages []data.Message, subtest func(testName string, inputData data.InputData) func(ctx context.Context, s *testing.State)) {
	for _, im := range inputMethods {
		// Setup input method.
		s.Logf("Set current input method to: %q", im)
		if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
			s.Fatalf("Failed to set input method to %q: %v: ", im, err)
		}
		uc.SetAttribute(useractions.AttributeInputMethod, im.Name)

		for _, message := range messages {
			inputData, ok := message.GetInputData(im)
			if !ok {
				s.Fatalf("Test Data for input method %q does not exist", im)
			}
			testName := string(im.Name) + "-" + string(inputData.ExpectedText)

			s.Run(ctx, testName, subtest(testName, inputData))
		}

		// Remove the input method after testing if it is not the default one.
		// Installing too many input methods on low-end devices may cause VK
		// crash.
		if im != ime.DefaultInputMethod {
			s.Logf("Remove current input method: %q", im)
			if err := im.Remove(uc.TestAPIConn())(ctx); err != nil {
				s.Logf("Failed to remove input method %q: %v: ", im, err)
			}
		}
	}
}

// RunSubtestsPerInputMethodAndModalidy runs subtest that uses testName and inputdata on
// every combination of given input methods and messages.
func RunSubtestsPerInputMethodAndModalidy(ctx context.Context, uc *useractions.UserContext, s *testing.State,
	inputMethods []ime.InputMethod, messages map[InputModality]data.Message, subtest func(testName string, modality InputModality, inputData data.InputData) func(ctx context.Context, s *testing.State)) {
	for _, im := range inputMethods {
		// Setup input method.
		s.Logf("Set current input method to: %s", im)
		if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
			s.Fatalf("Failed to set input method to %s: %v: ", im, err)
		}

		for modality, message := range messages {
			inputData, ok := message.GetInputData(im)
			if !ok {
				s.Fatalf("Test Data for input method %s does not exist", im)
			}
			testName := fmt.Sprintf("%s-%s-%s", im.Name, modality, inputData.ExpectedText)

			s.Run(ctx, testName, subtest(testName, modality, inputData))
		}

		// Remove the input method after testing if it is not the default one.
		// Installing too many input methods on low-end devices may cause VK
		// crash.
		if im != ime.DefaultInputMethod {
			s.Logf("Remove current input method: %q", im)
			if err := im.Remove(uc.TestAPIConn())(ctx); err != nil {
				s.Logf("Failed to remove input method %q: %v: ", im, err)
			}
		}
	}
}

// ExtractExternalFilesFromMap returns the file names contained in messages for
// selected input methods.
func ExtractExternalFilesFromMap(messages map[InputModality]data.Message, inputMethods []ime.InputMethod) []string {
	messageList := make([]data.Message, 0, len(messages))
	for _, message := range messages {
		messageList = append(messageList, message)
	}
	return data.ExtractExternalFiles(messageList, inputMethods)
}

// RunSubTest is designed to run an action as a subtest.
// It reserves 5s for general cleanup, dumping ui tree and screenshot on error.
func RunSubTest(ctx context.Context, s *testing.State, cr *chrome.Chrome, testName string, action uiauto.Action) {
	s.Run(ctx, testName, func(ctx context.Context, s *testing.State) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, filepath.Join(s.OutDir(), testName), s.HasError, cr, "ui_tree_"+testName)

		if err := action(ctx); err != nil {
			s.Fatalf("Subtest %q failed: %v", testName, err)
		}
	})
}

// GetNthCandidateTextAndThen returns an action that performs two steps in sequence:
// 1) Get the specified candidate.
// 2) Pass the specified candidate into provided function and runs the returned action.
// This is used when an action depends on the text of a candidate.
func GetNthCandidateTextAndThen(tconn *chrome.TestConn, n int, fn func(text string) uiauto.Action) uiauto.Action {
	return func(ctx context.Context) error {
		text, err := GetNthCandidateText(ctx, tconn, n)
		if err != nil {
			return err
		}

		if err := fn(text)(ctx); err != nil {
			return err
		}

		return nil
	}
}

// IMESearchFlags generates searchFlags based on the list of input methods.
func IMESearchFlags(imes []ime.InputMethod) []*testing.StringPair {
	var searchFlags = []*testing.StringPair{}
	for _, ime := range imes {
		searchFlags = append(
			searchFlags,
			&testing.StringPair{
				Key:   "ime",
				Value: ime.Name,
			},
		)
	}
	return searchFlags
}

// VerifyTextWithUIDetection returns an action checking the given text is shown on screen using ACUITI.
func VerifyTextWithUIDetection(tconn *chrome.TestConn, finder *nodewith.Finder, expectedText string, opts UIDetectionOptions) uiauto.Action {
	ud := uidetection.NewDefault(tconn).WithTimeout(time.Minute).WithScreenshotStrategy(uidetection.ImmediateScreenshot).WithScreenshotResizing()
	text := uidetection.Word(expectedText, uidetection.DisableApproxMatch(opts.ApproxMatch), uidetection.RegexMode(opts.UseRegex))
	if finder != nil {
		return ud.WaitUntilExists(text.WithinA11yNode(finder))
	}
	return ud.WaitUntilExists(text.First())
}

// OpenRemoteApplicationInCitirx opens a remote application in citirx workspace.
func OpenRemoteApplicationInCitirx(ctx context.Context, s *testing.FixtTestState, tconn *chrome.TestConn, vdi apps.VDIInt, uidetector *uidetection.Context, appName, textToLookForWhenLaunched string) {
	isOpened := func(ctx context.Context) error {
		// Wait for actual application window to open.
		if _, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, appName); err != nil {
			return errors.Wrap(err, "failed to find remote app window")
		}

		return uidetector.WaitUntilExists(uidetection.TextBlock(strings.Split(textToLookForWhenLaunched, " ")))(ctx)
	}

	if err := vdi.SearchAndOpenApplication(ctx, appName, isOpened)(ctx); err != nil {
		s.Fatal("Failed open remote app: ", err)
	}

	var textBlocks = strings.Split(textToLookForWhenLaunched, " ")
	if err := uidetector.LeftClick(uidetection.TextBlock(textBlocks))(ctx); err != nil {
		s.Fatal("Failed to click on launched window: ", err)
	}
}

// SearchFlagsWithIMEAndScreenPlay generates searchFlags based on the list of input methods with screenplay id.
func SearchFlagsWithIMEAndScreenPlay(imes []ime.InputMethod, screenPlayIDs []string) []*testing.StringPair {
	var searchFlags = []*testing.StringPair{}
	for _, ime := range imes {
		searchFlags = append(
			searchFlags,
			&testing.StringPair{
				Key:   "ime",
				Value: ime.Name,
			},
		)
	}

	for _, screenPlayID := range screenPlayIDs {
		searchFlags = append(
			searchFlags,
			&testing.StringPair{
				Key:   "feature_id",
				Value: screenPlayID,
			},
		)
	}

	return searchFlags
}

// ClearTextFieldViaClickingBackspace returns an action to clear text field by clicking backspace.
// This function is for the scenario that need to clean up the text field and hot-key may not working because the current IME is not english.
func ClearTextFieldViaClickingBackspace(kb *input.KeyboardEventWriter, times int) action.Action {
	var keySequence = []string{}
	for i := 1; i <= times; i++ {
		keySequence = append(keySequence, "Backspace")
	}
	return kb.TypeSequenceAction(keySequence)
}
