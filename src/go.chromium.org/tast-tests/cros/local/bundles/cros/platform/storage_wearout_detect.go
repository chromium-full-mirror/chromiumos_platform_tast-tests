// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/storage"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StorageWearoutDetect,
		Desc: "Fails if storage device information indicates impending failure",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com",
		},
		BugComponent: "b:974567",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"storage_wearout_detect"},
	})
}

func StorageWearoutDetect(ctx context.Context, s *testing.State) {
	info, err := storage.Get(ctx)
	if err != nil {
		s.Fatal("Failed to get storage info: ", err)
	}

	if info.Status == storage.Failing {
		s.Error("Storage device is failing, consider removing from DUT farm")
	}
}
