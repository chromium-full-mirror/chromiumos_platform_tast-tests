// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetUserMediaPolicy,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that admin policy can successfully ban getUserMedia",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational"},
		SoftwareDeps: []string{caps.BuiltinOrVividCamera, "chrome"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.VideoCaptureAllowed{}, pci.VerifiedFunctionalityJS),
		},
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: fixture.ChromePolicyLoggedIn,
				Val:     browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosPolicyLoggedIn,
				ExtraSoftwareDeps: []string{"lacros"},
				Val:               browser.TypeLacros,
			},
		},
	})
}

// GetUserMediaPolicy tests whether admin policy can successfully blocks getUserMedia().
func GetUserMediaPolicy(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{&policy.VideoCaptureAllowed{Val: false}}); err != nil {
		s.Fatal("Failed to serve policy to ban video capture: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Run actual test.
	conn, err := br.NewConn(ctx, "chrome://newtab")
	if err != nil {
		s.Fatal("Failed to connect to the browser: ", err)
	}
	defer conn.Close()

	if err := conn.Call(ctx, nil, `async () => {
		return navigator.mediaDevices.getUserMedia({video: true});
	}`); err == nil { // It is doesn't fail, it is unexpected.
		s.Fatal("Failed to ban getUserMedia() by the policy")
	} else if err.Error() != "DOMException: Permission denied" {
		s.Fatal("Unexpected error when calling getUserMedia(): ", err)
	}
}
