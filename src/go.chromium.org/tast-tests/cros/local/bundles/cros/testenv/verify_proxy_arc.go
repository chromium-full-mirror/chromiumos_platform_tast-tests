// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VerifyProxyArc,
		Desc: "A functional test that verifies proxy is supported in ARC",
		Contacts: []string{
			"cros-ufo-testing@google.com",
			"yanghenry@google.com",
		},
		// ChromeOS > EngProd > Software > Trust & Safety > UFO Testing
		BugComponent: "b:1034522",
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
		},
		Params: []testing.Param{{
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

// VerifyProxyArc tests pause and resume of optin flow.
func VerifyProxyArc(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bootParams := []string{
		"--params=androidboot.pause_provisioning=1", // to pause/resume provisioning
		"--params=androidboot.arc.allow_adb_root=1",
	}

	if err := arc.WriteArcvmDevConf(ctx, strings.Join(bootParams, "\n")); err != nil {
		s.Fatal("Failed to set arcvm_dev.conf: ", err)
	}
	defer arc.RestoreArcvmDevConf(cleanupCtx)

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	mp, err := proxy.NewMitmProxy(ctx,
		proxy.CustomCA(true), // Use the test system CA cert trusted by Chrome.
	)
	if err != nil {
		s.Fatal("Failed to start proxy: ", err)
	}
	defer mp.Close(cleanupCtx)
	if err := mp.Connect(ctx, cr); err != nil {
		s.Fatal("Failed to configure chrome for proxy: ", err)
	}

	s.Log("Opting in")
	if err := optin.PerformNoWait(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin: ", err)
	}

	s.Log("Waiting for ARC to boot")
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	s.Log("Inserting test certificate")
	path, _, err := mp.RootCertificate(ctx)
	if err != nil {
		s.Fatal("Failed to find root certificate: ", err)
	}
	if err := a.AddCaCert(ctx, path, proxy.CaHashcodeVar.Value()); err != nil {
		s.Fatal("Failed to insert test certificate: ", err)
	}

	s.Log("Resuming provisioning")
	if err := a.ResumeProvisioning(ctx); err != nil {
		s.Fatal("Failed to resume provisioning: ", err)
	}

	s.Log("Waiting for Play Store to show")
	if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to wait for Play Store: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to create UIAutomator: ", err)
	}
	defer d.Close(cleanupCtx)

	s.Log("Installing app")
	if err := playstore.InstallApp(ctx, a, d, "com.google.android.keep", &playstore.Options{TryLimit: -1, InstallationTimeout: 10 * time.Minute}); err != nil {
		s.Fatal("install the app: ", err)
	}

}
