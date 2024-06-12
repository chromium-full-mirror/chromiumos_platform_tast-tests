// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

const upsamplerTestInputImage = "super_res_input_640x480.nv12"
const upsamplerTestGoldenImage = "super_res_golden_2560x1920.nv12"

func init() {
	testing.AddTest(&testing.Test{
		Func:         SingleFrameUpsampler,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run single_frame_upsampler_test to verify libupsampler.so library works",
		Contacts:     []string{"chromeos-camera-eng@google.com", "julianachang@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"camera_feature_super_res"},
		Data: []string{
			upsamplerTestInputImage,
			upsamplerTestInputImage + ".json",
			upsamplerTestGoldenImage,
			upsamplerTestGoldenImage + ".json",
		},
		Timeout: 2 * time.Minute,
	})
}

func SingleFrameUpsampler(ctx context.Context, s *testing.State) {
	const gtestExecutable = "single_frame_upsampler_test"
	extraArgs := []string{
		"--input_image_path=" + s.DataPath(upsamplerTestInputImage),
		"--golden_image_path=" + s.DataPath(upsamplerTestGoldenImage),
	}
	t := gtest.New(gtestExecutable,
		gtest.ExtraArgs(extraArgs...),
		gtest.Logfile(filepath.Join(s.OutDir(), gtestExecutable+".log")))
	args, err := t.Args()
	if err != nil {
		s.Fatal("Failed to get GTest execution args: ", err)
	}
	s.Log("Running ", shutil.EscapeSlice(args))
	if _, err := t.Run(ctx); err != nil {
		s.Fatalf("Failed to run %v: %v", gtestExecutable, err)
	}
}
