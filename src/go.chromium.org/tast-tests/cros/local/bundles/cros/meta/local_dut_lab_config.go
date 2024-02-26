// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalDUTLabConfig,
		Desc:         "Example to access DUT lab configuration from a local test",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Fixture:      "metaLocalDUTLabConfigFixture",
	})
}

func LocalDUTLabConfig(ctx context.Context, s *testing.State) {
	dutLabConfig, err := s.ChromeOSDUTLabConfig("")
	if err != nil {
		s.Fatal("Failed to get the lab configuration of the DUT: ", err)
	}
	jsonOut := protojson.Format(dutLabConfig)
	const filename = "dut_lab_config.jsonpb"
	if err := os.WriteFile(filepath.Join(s.OutDir(), filename), []byte(jsonOut), 0644); err != nil {
		s.Fatalf("Failed to create %q: %v", filename, err)
	}
}
