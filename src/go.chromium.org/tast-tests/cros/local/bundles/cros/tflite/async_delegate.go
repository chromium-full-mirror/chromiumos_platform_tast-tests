// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tflite

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/tflite"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AsyncDelegate,
		Desc:         "Runs the TFLite async delegate test",
		Contacts:     []string{"cros-odml-foundations-eng@google.com", "ototot@chromium.org", "shik@chromium.org"},
		BugComponent: "b:1445284", // ChromeOS > Platform > Technologies > Machine Learning > On-Device ML
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"ml_service"},
		Params: []testing.Param{{
			Name: "sample",
			Val: tflite.StableDelegateSettings{
				StableDelegateLoaderSettings: tflite.SampleDelegateLoaderSettings,
			},
		}, {
			Name:              "neuron",
			ExtraSoftwareDeps: []string{"tflite_mtk_neuron"},
			Val: tflite.StableDelegateSettings{
				StableDelegateLoaderSettings: tflite.MtkNeuronDelegateLoaderSettings,
				MtkNeuronSettings: &tflite.MtkNeuronSettings{
					AllowFp16PrecisionForFp32: true,
				},
			},
		}, {
			Name:              "openvino",
			ExtraSoftwareDeps: []string{"tflite_intel_openvino"},
			Val: tflite.StableDelegateSettings{
				StableDelegateLoaderSettings: tflite.IntelOpenVINODelegateLoaderSettings,
			},
		}},
	})
}

// AsyncDelegate runs the async_delegate_test binary, which exercises the
// TFLite async kernel API on the vendor stable delegate.
func AsyncDelegate(ctx context.Context, s *testing.State) {
	settingsPath := filepath.Join(s.OutDir(), "settings.json")
	gtestLogPath := filepath.Join(s.OutDir(), "gtest.log")

	s.Param().(tflite.StableDelegateSettings).WriteTo(settingsPath)

	if report, err := gtest.New(
		"async_delegate_test",
		gtest.Logfile(gtestLogPath),
		gtest.ExtraArgs("--stable_delegate_settings_file="+settingsPath),
	).Run(ctx); err != nil {
		if report != nil {
			failed := report.FailedTestNames()
			for _, name := range failed {
				s.Log("Failed test: ", name)
			}

			numFailed := len(failed)
			if numFailed == 1 {
				s.Errorf("%s failed", failed[0])
			} else if numFailed > 1 {
				s.Errorf("%s and %d more tests failed", failed[0], numFailed-1)
			}
		}
		s.Error("Failed to pass async_delegate_test: ", err)
	}
}
