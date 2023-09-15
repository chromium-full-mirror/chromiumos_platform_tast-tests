// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/testing"
)

type timezoneInfo struct {
	Posix  string `json:"posix"`
	Region string `json:"region"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeTimezoneInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for timezone info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"yycheng@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

func ProbeTimezoneInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryTimezone}
	var timezone timezoneInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &timezone); err != nil {
		s.Fatal("Failed to get timezone telemetry info: ", err)
	}

	if timezone.Posix == "" {
		s.Error("Missing posix info")
	}

	if timezone.Region == "" {
		s.Error("Missing region info")
	}
}
