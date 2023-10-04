// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Preopt,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that ARC++ is fully pre-optimized and there is no pre-opt happening during the boot",
		Contacts:     []string{"arc-performance@google.com", "khmel@chromium.org"},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 4 * time.Minute,
	})
}

func Preopt(ctx context.Context, s *testing.State) {
	if err := performBootAndWaitForDexOpt(ctx, s.OutDir()); err != nil {
		s.Fatal("Failed to boot ARC and wait for dexopt: ", err)
	}

	if err := arc.CheckNoDex2Oat(s.OutDir()); err != nil {
		s.Fatal("Failed to verify dex2oat was not running: ", err)
	}
}

func performBootAndWaitForDexOpt(ctx context.Context, outDir string) error {
	args := append(arc.DisableSyncFlags(), "--arc-force-post-boot-dex-opt")
	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(args...))
	if err != nil {
		return errors.Wrap(err, "failed to connect to Chrome browser process")
	}
	defer cr.Close(ctx)

	a, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to connect to ARC")
	}
	defer a.Close(ctx)

	if err := arc.WaitForDexOptOnBoot(ctx, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for dexopt on boot to finish")
	}

	return nil
}
