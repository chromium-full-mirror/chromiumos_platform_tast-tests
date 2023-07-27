// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeatureFile,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Chekcs if Arc feature json file exists",
		Contacts:     []string{"arc-core@google.com", "lgcheng@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"arc", "chrome"},
		Timeout:      4 * time.Minute,
	})
}

func FeatureFile(ctx context.Context, s *testing.State) {
	var arcFeatureFilePath = "/etc/arc/features.json"
	if isVMEnabled, err := arc.VMEnabled(); err != nil {
		s.Fatal("Failed to determine guest type: ", err)
	} else if isVMEnabled {
		arcFeatureFilePath = "/etc/arcvm/features.json"
	}

	if _, err := os.Stat(arcFeatureFilePath); err == nil {
		return
	} else if errors.Is(err, os.ErrNotExist) {
		s.Fatal("Fail to find Arc feature json file")
	} else {
		s.Fatal("Unknow error: ", err)
	}
}
