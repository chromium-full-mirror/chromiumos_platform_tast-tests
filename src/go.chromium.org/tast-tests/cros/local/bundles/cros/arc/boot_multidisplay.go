// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	virtualmultidisplay "go.chromium.org/tast-tests/cros/local/virtualmultidisplay"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootMultidisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Android boots when VKMS multidisplay is configured",
		Contacts:     []string{"arc-framework+tast@google.com", "brpol@chromium.org"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"chrome", "virtual_multidisplay", "android_vm"},
		Fixture:      virtualmultidisplay.VirtualMultiDisplay,
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational", "group:hw_agnostic"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
	})
}

func BootMultidisplay(ctx context.Context, s *testing.State) {
	if err := arc.WriteArcvmDevConf(ctx, ""); err != nil {
		s.Fatal("Failed to set arcvm_dev.conf: ", err)
	}
	defer arc.RestoreArcvmDevConf(ctx)

	reader, err := syslog.NewReader(ctx)
	if err != nil {
		s.Fatal("Failed to open syslog reader: ", err)
	}
	defer reader.Close()

	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if err := cr.Close(ctx); err != nil {
			s.Fatal("Failed to close Chrome while booting ARC: ", err)
		}
	}()

	a, err := arc.NewWithSyslogReader(ctx, s.OutDir(), reader, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	// Ensures package manager service is running by checking the existence of the "android" package.
	pkgs, err := a.InstalledPackages(ctx)
	if err != nil {
		s.Fatal("Getting installed packages failed: ", err)
	}

	if _, ok := pkgs["android"]; !ok {
		s.Fatal("android package not found: ", pkgs)
	}

}
