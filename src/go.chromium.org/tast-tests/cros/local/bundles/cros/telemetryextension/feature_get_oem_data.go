// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	oemDataFilePath = "/usr/share/cros/oemdata.sh"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeatureGetOEMData,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests chrome.os.telemetry.getOemData Chrome Extension API function exposed to Telemetry Extension",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.TelemetryExtensionSkipOEMNameCheck,
	})
}

// FeatureGetOEMData tests chrome.os.telemetry.getOemData Chrome Extension API functionality.
func FeatureGetOEMData(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	type response struct {
		OemData string `json:"oemData"`
	}

	var resp response
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.telemetry.getOemData)",
	); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}
	testing.ContextLog(ctx, "getOemData response: ", resp.OemData)

	hasOEMData, err := oemDataFileExist(ctx)
	if err != nil {
		s.Fatal("Failed to check the existence of oemdata.sh: ", err)
	}

	if hasOEMData {
		want, err := oemData(ctx)
		if err != nil {
			s.Fatal("Failed to get OEM data: ", err)
		}
		if got := resp.OemData; got != want {
			s.Errorf("Unexpected OEM data: got %q; want %q", got, want)
		}
	} else {
		// Example response:
		// /bin/sh: 1: /usr/share/cros/oemdata.sh: not found
		if !strings.Contains(resp.OemData, "/usr/share/cros/oemdata.sh: not found") {
			s.Errorf("Unexpected OEM data: got %q; should report file not found", resp.OemData)
		}
	}
}

func oemDataFileExist(ctx context.Context) (bool, error) {
	if _, err := os.Stat(oemDataFilePath); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}

func oemData(ctx context.Context) (string, error) {
	b, err := testexec.CommandContext(ctx, oemDataFilePath).Output(testexec.DumpLogOnError)
	if err != nil {
		testing.ContextLog(ctx, "oemdata.sh output: ", string(b))
		return "", err
	}
	return string(b), nil
}
