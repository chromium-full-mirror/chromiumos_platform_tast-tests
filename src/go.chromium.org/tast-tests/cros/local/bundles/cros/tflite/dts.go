// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tflite

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/tflite"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DTS,
		Desc:         "Runs the TFLite stable delegate test suite",
		Contacts:     []string{"cros-odml-foundations-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:1445284", // ChromeOS > Platform > Technologies > Machine Learning > On-Device ML
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"ml_service"},
		Vars:         []string{"settings", "accel_config"},
		Params: []testing.Param{{
			Name:      "sample",
			Val:       sampleParam,
			ExtraData: []string{sampleAccelConfig},
		}, {
			Name:              "neuron",
			Val:               neuronParam,
			Timeout:           5 * time.Minute,
			ExtraSoftwareDeps: []string{"tflite_mtk_neuron"},
			ExtraData:         []string{neuronAccelConfig},
		}, {
			Name:              "openvino",
			Val:               openvinoParam,
			ExtraSoftwareDeps: []string{"tflite_intel_openvino"},
			ExtraData:         []string{openvinoAccelConfig},
		}},
	})
}

type testingParam struct {
	Settings                  tflite.StableDelegateSettings
	AccelConfig               string
	SkipTestPatterns          []string
	AllowFp16PrecisionForFp32 bool
}

var sampleSettings = tflite.StableDelegateSettings{
	StableDelegateLoaderSettings: tflite.SampleDelegateLoaderSettings,
}

const sampleAccelConfig = "sample_accel_test.conf"

var sampleParam = testingParam{
	Settings:    sampleSettings,
	AccelConfig: sampleAccelConfig,
	// Disable MultiDimBroadcast related tests since it takes ~3 minutes, and the
	// operation is not supported by sample stable delegate.
	SkipTestPatterns: []string{"*MultiDimBroadcastSubshard*"},
}

var neuronSettings = tflite.StableDelegateSettings{
	StableDelegateLoaderSettings: tflite.MtkNeuronDelegateLoaderSettings,
	MtkNeuronSettings: &tflite.MtkNeuronSettings{
		OperationCheckMode:        tflite.PreOperationCheck,
		AllowFp16PrecisionForFp32: true,
	},
}

// TODO(b/338910179): MediaTek to provide the proper config.
// TODO(b/338910179): Use separate AccelConf for neuron pilot in different versions.
const neuronAccelConfig = "neuron_accel_test.conf"

var neuronParam = testingParam{
	Settings:    neuronSettings,
	AccelConfig: neuronAccelConfig,
	SkipTestPatterns: []string{
		// TODO(b/338938802): Neuron delegate is ~30x slower than CPU on these test
		// cases and need ~1hr to finish them. This is a superset of the
		// unsupported data types below as expected.
		"*MultiDimBroadcastSubshard*",

		// Disable the slow MultiDimBroadcast tests with unsupported data types to
		// save test execution time.
		"*IntegerMultiDimBroadcastSubshard*",
		"*Float32MultiDimBroadcastSubshard*",

		// TODO(b/338959718): Neuron delegate failed with all -128 output.
		"*QuantizeOpTest.Int16ZeroPointInt8*",

		// TODO(b/338963077): Neuron delegate should return kTfLiteError when seeing
		// non-positive values.
		"*RsqrtNegativeInt8*",
		"*RsqrtNegativeInt16*",

		// TODO(b/364804438): Neuron delegate failed with incorrect outputs after
		// turning on --allow_fp16_precision_for_fp32.
		"SoftmaxOpTest/SoftmaxOpTest.Softmax1DMax/0",
		"SoftmaxOpTest/SoftmaxOpTest.Softmax1DMax/1",
		"SoftmaxOpTest/SoftmaxOpTest.Softmax1DInf/0",
		"SoftmaxOpTest/SoftmaxOpTest.Softmax1DInf/1",
	},
	AllowFp16PrecisionForFp32: true,
}

var openvinoSettings = tflite.StableDelegateSettings{
	StableDelegateLoaderSettings: tflite.IntelOpenVINODelegateLoaderSettings,
}

// TODO(b/332423167): Intel to provide the proper config.
const openvinoAccelConfig = "openvino_accel_test.conf"

var openvinoParam = testingParam{
	Settings:    openvinoSettings,
	AccelConfig: openvinoAccelConfig,
	SkipTestPatterns: []string{
		// TODO(b/357734672): Openvino delegate is not fully delegated on
		// MultiDimBroadcast tests and is ~2000x slower than CPU on some of them.
		// It would need >5hr to finish them and some tests fails because of FP16
		// precision loss.
		"*MultiDimBroadcastSubshard*",

		// TODO(b/357498049): Openvino delegate didn't properly handle resize
		// bilinear operators, which results in weird result.
		"ResizeBilinearOpTest/ResizeBilinearOpTest.VerticalResize/0",
		"ResizeBilinearOpTest/ResizeBilinearOpTest.TwoDimensionalResizeWithTwoBatches/0",
		"ResizeBilinearOpTest/ResizeBilinearOpTest.TwoDimensionalResizeWithTwoBatches_HalfPixelCenters/0",

		// TODO(b/364772332): Openvino delegate output incorrect results.
		"FloatPoolingOpTest.MaxPoolActivationRelu6",

		// TODO(b/381967953): Openvino delegate crashed on these tests after updated to v1.10.0.
		"DepthwiseConvolutionOpTest/DepthwiseConvolutionOpTest.BatchPaddingSameTest/0",
		"DepthwiseConvolutionOpTest/DepthwiseConvolutionOpTest.BatchPaddingSameTest/1",
		"DepthwiseConvolutionOpTest/DepthwiseConvolutionOpTest.BatchPaddingSameTest/2",
		"DepthwiseConvolutionOpTest/DepthwiseConvolutionOpTest.MultithreadBatchPaddingSameTest/0",
		"DepthwiseConvolutionOpTest/DepthwiseConvolutionOpTest.MultithreadBatchPaddingSameTest/1",
		"DepthwiseConvolutionOpTest/DepthwiseConvolutionOpTest.MultithreadBatchPaddingSameTest/2",

		// TODO(b/381968609): Openvino cannot broadcast eltwise inputs.
		"ConstantInputs/MulOpTest.FloatMixedBroadcast/0",
	},
	AllowFp16PrecisionForFp32: true,
}

// DTS runs the Tensorflow Lite Stable Delegate Test Suite (DTS).
func DTS(ctx context.Context, s *testing.State) {
	param := s.Param().(testingParam)
	settingsPath := filepath.Join(s.OutDir(), "settings.json")
	accelConfigPath := filepath.Join(s.OutDir(), "accel.conf")
	gtestLogPath := filepath.Join(s.OutDir(), "gtest.log")

	settingsVar, ok := s.Var("settings")
	if ok {
		s.Log("Use provided settings file at ", settingsVar)
		if err := fsutil.CopyFile(settingsVar, settingsPath); err != nil {
			s.Fatal("Failed to copy settings file: ", err)
		}
	} else {
		if err := param.Settings.WriteTo(settingsPath); err != nil {
			s.Fatal("Failed to write settings.json: ", err)
		}
	}

	accelConfigVar, ok := s.Var("accel_config")
	if ok {
		s.Log("Use provided accel_config file at ", accelConfigVar)
		if err := fsutil.CopyFile(accelConfigVar, accelConfigPath); err != nil {
			s.Fatal("Failed to copy accel_config file: ", err)
		}
	} else {
		if err := fsutil.CopyFile(s.DataPath(param.AccelConfig), accelConfigPath); err != nil {
			s.Fatal("Failed to copy default accel_config file: ", err)
		}
	}

	gtestFilter := ""
	if len(param.SkipTestPatterns) > 0 {
		gtestFilter = "-" + strings.Join(param.SkipTestPatterns, ":")
	}

	test := gtest.New(
		"/usr/local/bin/stable_delegate_test_suite",
		gtest.Logfile(gtestLogPath),
		gtest.Filter(gtestFilter),
		gtest.ExtraArgs(
			"--stable_delegate_settings_file="+settingsPath,
			"--acceleration_test_config_path="+accelConfigPath,
			"--allow_fp16_precision_for_fp32="+strconv.FormatBool(param.AllowFp16PrecisionForFp32),
		),
	)
	args, err := test.Args()
	if err != nil {
		s.Fatal("Failed to get gtest args: ", err)
	}
	s.Log("Running ", shutil.EscapeSlice(args))

	if report, err := test.Run(ctx); err != nil {
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
		s.Error("Failed to pass DTS: ", err)
	}
}
