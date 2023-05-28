// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    NNAPIDriver,
		Desc:    "Validates that the HAL driver conforms to the NNAPI specification",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"jmpollock@google.com",
			"slangley@google.com",
		},
		BugComponent: "b:1140118",
		Attr: []string{
			"group:mainline", "informational",
		},
		SoftwareDeps: []string{"nnapi"},
		Params: []testing.Param{
			{
				Name:              "cts",
				Val:               []string{"cros_nnapi_cts"},
				ExtraSoftwareDeps: []string{"nnapi_vendor_driver"},
			},
			{
				Name:              "vts_1_3",
				Val:               []string{"cros_nnapi_vts_1_3"},
				ExtraSoftwareDeps: []string{"nnapi_vendor_driver"},
			},
			{
				Name: "vts_1_0",
				Val:  []string{"cros_nnapi_vts_1_0"},
			}, {
				Name: "vts_1_1",
				Val:  []string{"cros_nnapi_vts_1_1"},
			}, {
				Name:              "vts_1_2",
				Val:               []string{"cros_nnapi_vts_1_2"},
				ExtraSoftwareDeps: []string{"nnapi_vendor_driver"},
			},
		},
	})
}

func NNAPIDriver(ctx context.Context, s *testing.State) {
	cmdArgs := s.Param().([]string)

	cmd := testexec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), "ANDROID_LOG_TAGS=*:f")

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Error("Failed to run test suite: ", err)
	}
}
