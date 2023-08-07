// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/ureadahead"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UreadaheadValidation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validates that ARC ureadahead packs in the host/guest OS exist and are valid",
		Contacts: []string{
			"arc-performance@google.com",
			"alanding@google.com",
			"khmel@google.com",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		// NOTE: This test has build dependency and it will always fail in PFQ/Uprev
		//       since we don't have ureadahead pack generated at PFQ test time.
		Attr: []string{"group:arc", "arc_core", "group:arc-functional"},
		// Skip userdebug boards which we don't generate ureadahead packs for.
		SoftwareDeps: []string{"chrome", "no_arc_userdebug"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm_r",
			ExtraSoftwareDeps: []string{"android_vm_r"},
		}, {
			Name:              "vm_t",
			ExtraSoftwareDeps: []string{"android_vm_t"},
		}},
		// Minimum acceptable.
		Timeout: 5 * time.Minute,
	})
}

func UreadaheadValidation(ctx context.Context, s *testing.State) {
	const (
		// Names of ureadahead dump logs.
		ureadaheadLogName      = "ureadahead.log"
		ureadaheadGuestLogName = "guest_ureadahead.log"

		// Normally generated host ureadahead pack covers >300MB of data.
		minAcceptableUreadaheadPackSizeKB = 300 * 1024
		// Guest ureadahead pack could be smaller than host from lab data.
		minAcceptableGuestUreadaheadPackSizeKB = 100 * 1024
	)

	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		s.Fatal("Failed to get whether ARCVM is enabled: ", err)
	}

	// If VM, only verify guest OS ureadahead dump.
	if vmEnabled {
		vmLogPath := filepath.Join(s.OutDir(), ureadaheadGuestLogName)

		cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(ctx)

		outDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			s.Fatal("Failed to get name of the output directory")
		}

		// Connect to the ARCVM instance.
		a, err := arc.New(ctx, outDir, cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to connect to ARCVM: ", err)
		}
		defer a.Close(ctx)

		if err = ureadahead.DumpGuestPack(ctx, a, vmLogPath); err != nil {
			s.Fatal("Failed to dump guest ureadahead pack: ", err)
		}

		// Verify the guest pack file dump.
		if err = ureadahead.CheckPackFileDump(ctx, vmLogPath, minAcceptableGuestUreadaheadPackSizeKB); err != nil {
			s.Fatalf("Failed to verify guest ureadahead pack file dump, please check %q: %v", ureadaheadGuestLogName, err)
		}
		return
	}

	logPath := filepath.Join(s.OutDir(), ureadaheadLogName)
	packPath := arc.ARCPath + "/ureadahead.pack"
	if err = ureadahead.DumpHostPack(ctx, packPath, logPath); err != nil {
		s.Fatal("Failed to dump host ureadahead pack: ", err)
	}

	if err = ureadahead.CheckPackFileDump(ctx, logPath, minAcceptableUreadaheadPackSizeKB); err != nil {
		s.Fatalf("Failed to verify ureadahead pack file dump, please check %q: %v", ureadaheadLogName, err)
	}
}
