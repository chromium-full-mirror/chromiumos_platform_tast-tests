// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MountPerfWithArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Performance for cryptohome mount operation with an android app installed",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@google.com", // Test author
		},
		BugComponent: "b:1188704",
		Attr:         []string{"hwsec_destructive_crosbolt_perbuild", "group:hwsec_destructive_crosbolt"},
		SoftwareDeps: []string{"tpm_clear_allowed", "chrome", "selinux"},
		Vars: []string{
			"cryptohome.MountPerfWithArc.mountOperations",
		},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name: "vm",
			ExtraSoftwareDeps: []string{
				"android_vm",
				// Skip the test when ARCVM virtio-blk /data is enabled.
				// ARC's /data directory is encapsulated in a disk image when it is enabled.
				"no_arcvm_virtio_blk_data",
			},
		}},
		VarDeps: []string{"ui.gaiaPoolDefault"},
		Timeout: 12 * time.Minute,
	})
}

func MountPerfWithArc(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	var creds credconfig.Creds

	{
		// Set up Chrome instance.
		cr, err := chrome.New(ctx,
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			chrome.ARCSupported(),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)
		// Get the creds to use the same one next in subsequent logins.
		creds = cr.Creds()

		if err := performArcOptin(ctx, cr); err != nil {
			s.Fatal("Failed to perform arc optin: ", err)
		}

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		if a == nil {
			s.Fatal("Faled to start ARC")
		}
		defer a.Close(cleanupCtx)

		if err := installAndroidApp(ctx, a); err != nil {
			s.Fatal("Failed to install Android app from Play Store: ", err)
		}
	}

	defer func() {
		// This cleans up the user state.
		cr, _ := chrome.New(ctx,
			chrome.DeferLogin())
		cr.Close(cleanupCtx)
	}()

	valueOrDefaultForVar := func(s *testing.State, key string, defaultValue int) int {
		if val, ok := s.Var(key); ok {
			converted, err := strconv.Atoi(val)
			if err != nil {
				s.Fatalf("Unable to parse %v variable: %v", key, err)
			}
			return converted
		}
		return defaultValue
	}

	// Get iterations count from the variable or default it.
	mountOperations := valueOrDefaultForVar(s, "cryptohome.MountPerfWithArc.mountOperations", 10)
	value := perf.NewValues()

	// Run mountOperations times MountVault.
	for i := 0; i < mountOperations; i++ {
		startTs := time.Now()
		// Fake Login is good enough here. We know that account already exists.
		cr, err := chrome.New(ctx,
			chrome.FakeLogin(creds),
			chrome.ARCEnabled(),
			chrome.KeepState(),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		cr.Close(ctx)
		// Needed so defer does not cause issue above.
		cr = nil
		duration := time.Now().Sub(startTs)

		value.Append(perf.Metric{
			Name:      "normal_login_duration",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(duration.Milliseconds()))
	}

	if err := value.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf-results: ", err)
	}
}

// performArcOptin performs ARC optin flow and ensure Play Store window is shown.
func performArcOptin(ctx context.Context, cr *chrome.Chrome) error {
	const maxAttempts = 2
	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		return errors.Wrap(err, "failed to optin")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test API connection")
	}

	return optin.WaitForPlayStoreShown(ctx, tconn, time.Minute)
}

// installAndroidApp checks that an Android app can be installed from
// Play Store and installs it.
func installAndroidApp(ctx context.Context, a *arc.ARC) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize UI Automator")
	}
	defer d.Close(cleanupCtx)

	const pkgName = "com.google.android.calculator"
	return playstore.InstallApp(ctx, a, d, pkgName, &playstore.Options{TryLimit: -1})
}
