// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalCloudStorageSpace,
		Desc:         "Access cloud storage file with a space in the path",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
	})
}

func LocalCloudStorageSpace(ctx context.Context, s *testing.State) {
	_, err := s.CloudStorage().Open(ctx, "gs://chromiumos-test-assets-public/tast/cros/arc/space error/SpaceError.txt")
	if err != nil {
		s.Fatal("Failed to download file: ", err)
	}
}
