// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Alternate the flag value crasFeaturesIterations times to test flag propagation.
	crasFeaturesIterations          = 4
	crasFeaturesTimeoutPerIteration = 20 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasFeatures,
		Desc:         "CRAS can talk to feature_library correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline"},
		Timeout:      crasFeaturesIterations * (crasFeaturesTimeoutPerIteration + chrome.ResetTimeout),
		SoftwareDeps: []string{"chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// CrasFeatures checks that CRAS can talk to feature_library correctly.
func CrasFeatures(ctx context.Context, s *testing.State) {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}

	for i := 0; i < crasFeaturesIterations; i++ {
		enabled := i%2 == 0
		s.Logf("Test iteration %d, enabled = %v", i, enabled)

		shortCtx, cancel := context.WithTimeout(ctx, crasFeaturesTimeoutPerIteration+chrome.ResetTimeout)
		defer cancel()

		if err := crasFeaturesOneIteration(shortCtx, cras, enabled); err != nil {
			s.Fatalf("Iteration %d failed: %v", i, err)
		}
	}
}

// crasFeaturesOneIteration runs one iteration of the CrasFeatures test.
func crasFeaturesOneIteration(ctx context.Context, cras *audio.Cras, featureEnabled bool) error {
	const testFeatureFlag = "CrOSLateBootDisabledByDefault"

	var flagOption chrome.Option
	if featureEnabled {
		flagOption = chrome.EnableFeatures(testFeatureFlag)
	} else {
		flagOption = chrome.DisableFeatures(testFeatureFlag)
	}

	cleanupCtx, cancel := ctxutil.Shorten(ctx, chrome.ResetTimeout)
	defer cancel()

	cr, err := chrome.New(
		ctx,
		// org.chromium.ChromeFeaturesService does not need login to work.
		// Don't login to speed up the test.
		chrome.NoLogin(),
		flagOption,
	)
	if err != nil {
		return errors.Wrap(err, "cannot start Chrome")
	}
	defer cr.Close(cleanupCtx)

	return cras.WaitUntilFeatureFlagHasValue(ctx, testFeatureFlag, featureEnabled)
}
