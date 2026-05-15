// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	testCaFile = "managed_device_policy_ca_cert.pem"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: OptinPause,
		Desc: "A functional test that verifies OptIn flow can be paused/resumed",
		Contacts: []string{
			"arc-core@google.com",
			"jhorwich@google.com",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
		},
		Data: []string{testCaFile},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

// OptinPause tests pause and resume of optin flow.
func OptinPause(ctx context.Context, s *testing.State) {
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
	if err := a.AddCaCert(ctx, s.DataPath(testCaFile), "49e258f8"); err != nil {
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
}
