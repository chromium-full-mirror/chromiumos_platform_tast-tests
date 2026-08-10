// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testrunner provides helper functions to run input tast tests.
package testrunner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/data"

	"go.chromium.org/tast/core/ctxutil"
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

// RunSubtestsPerInputMethodAndMessage runs subtest that uses testName and inputdata on
// every combination of given input methods and messages.
func RunSubtestsPerInputMethodAndMessage(ctx context.Context, uc *useractions.UserContext, s *testing.State,
	inputMethods []ime.InputMethod, messages []data.Message, subtest func(testName string, inputData data.InputData, im ime.InputMethod) func(ctx context.Context, s *testing.State)) {
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

			s.Run(ctx, testName, subtest(testName, inputData, im))
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
