// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Optin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test that verifies OptIn flow",
		Contacts: []string{
			"arc-core@google.com",
			"mhasank@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:cq-medium"},
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
		},
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"group:cq-minimal"},
				ExtraSoftwareDeps: []string{"android_container"},
			},
			{
				Name:              "vm",
				ExtraAttr:         []string{"group:cq-minimal"},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t"},
			},
			{
				Name:              "x",
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"android_vm_t"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

// Optin tests optin flow.
func Optin(ctx context.Context, s *testing.State) {
	const (
		// If a single variant is flaky, please promote this to test params and increase the
		// attempts only for that specific variant instead of updating the constant for all.
		// See crrev.com/c/2979454 for an example.
		maxAttempts = 1
	)

	gaiaLogin := chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))

	cr, err := setupChrome(ctx, gaiaLogin)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	s.Log("Performing optin")

	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		s.Fatal("Failed to optin: ", err)
	}
}

// setupChrome starts chrome with pooled GAIA account and ARC enabled.
func setupChrome(ctx context.Context, gaiaLogin chrome.Option) (*chrome.Chrome, error) {
	cr, err := chrome.New(ctx,
		gaiaLogin,
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		return nil, err
	}
	return cr, nil
}
