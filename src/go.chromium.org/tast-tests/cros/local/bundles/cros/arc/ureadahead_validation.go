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

	"go.chromium.org/tast/core/errors"
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
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
	})
}

func UreadaheadValidation(ctx context.Context, s *testing.State) {
	const (
		// Names of ureadahead dump logs.
		ureadaheadLogName                 = "ureadahead.log"
		ureadaheadGuestInitialLogName     = "guest_ureadahead_initial.log"
		ureadaheadGuestProvisionedLogName = "guest_ureadahead_provisioned.log"
	)

	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		s.Fatal("Failed to get whether ARCVM is enabled: ", err)
	}

	// If VM, only verify guest OS ureadahead dump.
	if vmEnabled {
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			s.Fatal("Failed to get name of the output directory: ", err)
		}
		if err := verifyGuestUreadaheadDump(ctx, outDir, ureadaheadGuestInitialLogName, true /*isInitialBoot*/); err != nil {
			s.Fatal("Failed to verify guest ureadahead initial pack: ", err)
		}
		if err := verifyGuestUreadaheadDump(ctx, outDir, ureadaheadGuestProvisionedLogName, false /*isInitialBoot*/); err != nil {
			s.Fatal("Failed to verify guest ureadahead provisioned pack: ", err)
		}
		return
	}

	logPath := filepath.Join(s.OutDir(), ureadaheadLogName)
	packPath := arc.ARCPath + "/ureadahead.pack"
	if err = ureadahead.DumpHostPack(ctx, packPath, logPath); err != nil {
		s.Fatal("Failed to dump host ureadahead pack: ", err)
	}
	if err = ureadahead.CheckPackFileDump(ctx, logPath, ureadahead.MinHostPackSizeKB); err != nil {
		s.Fatalf("Failed to verify ureadahead pack file dump, please check %q: %v", ureadaheadLogName, err)
	}
}

// verifyGuestUreadaheadDump boots ARCVM and verifies guest ureadahead pack file depending on boot type.
func verifyGuestUreadaheadDump(ctx context.Context, outDir, logName string, isInitialBoot bool) error {
	vmLogPath := filepath.Join(outDir, logName)

	extraArg := chrome.UnRestrictARCCPU()
	if !isInitialBoot {
		extraArg = chrome.KeepState()
	}
	cr, err := chrome.New(ctx, chrome.ARCEnabled(), extraArg)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(ctx)

	// Connect to the ARCVM instance.
	a, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to connect to ARCVM")
	}
	defer a.Close(ctx)

	if err = ureadahead.DumpGuestPack(ctx, a, vmLogPath); err != nil {
		return errors.Wrap(err, "failed to dump guest ureadahead pack")
	}

	// Verify the guest pack file dump.
	if err = ureadahead.CheckPackFileDump(ctx, vmLogPath, ureadahead.MinGuestPackSizeKB); err != nil {
		return errors.Wrapf(err, "failed to verify guest ureadahead pack file dump, please check %q", logName)
	}
	return nil
}
