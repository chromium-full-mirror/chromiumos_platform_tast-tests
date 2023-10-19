// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/libvda"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LibvdaGpuTests,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the non-decoding tests targetting libvda's GPU implementation",
		Contacts:     []string{"chromeos-video-eng@google.com", "mwojtas@google.com"},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Video
		BugComponent: "b:632502",
		Attr:         []string{"group:mainline", "informational"},
		// "no_qemu" disables the test on betty. b/168566159#comment3
		SoftwareDeps: []string{"android_vm", "chrome", "no_qemu"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
	})
}

func LibvdaGpuTests(ctx context.Context, s *testing.State) {
	libvda.RunGPUNonDecodeTests(ctx, s)
}
