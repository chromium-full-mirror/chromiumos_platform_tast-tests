// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Reboot,
		Desc:         "Verifies that system comes back after rebooting",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"reboot", "no_qemu"},
		Attr:         []string{"group:mainline", "group:labqual"},
	})
}

func Reboot(ctx context.Context, s *testing.State) {
	d := s.DUT()

	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}
}
