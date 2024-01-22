// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package encode provides common code to run Chrome binary tests for video encoding.
package encode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/videovars"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/gtest"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/media/encoding"
	"go.chromium.org/tast-tests/cros/local/media/logging"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// Duration of the interval during which CPU usage will be measured in the performance test.
	measureInterval = 20 * time.Second
	// The number of frames to be used in video_encode_accelerator_tests and
	// --speed tests in video_encode_accelerator_perf_tests.
	numEncodeFrames = 60
)

// The md5 hash values of the first 60 frames in the video file.
var md5OfYUV60Frames = map[string]string{
	"encode/desktop2-240x135_850frames.vp9.webm":   "3ac3550a9082255ebb423fdf991ff245",
	"encode/desktop2-320x180_850frames.vp9.webm":   "63c5e91eef6e0c99431f4e3ff17ed8f9",
	"encode/desktop2-480x270_850frames.vp9.webm":   "e1ebf7d5b2abf9ee3d7a60493c1153db",
	"encode/desktop2-640x360_850frames.vp9.webm":   "c6a49dd9d27335e044db714e8fe2e635",
	"encode/desktop2-960x540_850frames.vp9.webm":   "e1f7c804515040bce6e80c8d63f518e2",
	"encode/desktop2-1280x720_850frames.vp9.webm":  "44148a6abc24bd136588564a5f0d6562",
	"encode/desktop2-1920x1080_490frames.vp9.webm": "3009e864f85fff8ebe41d73971792311",
	"encode/desktop2-3840x2160_170frames.vp9.webm": "650a6b03a33db4822541b4f652764f39",

	"encode/crowd_run-320x180_60frames.vp9.webm":   "7abb7e1db24d92ce733bf29f45ba6e54",
	"encode/crowd_run-480x270_60frames.vp9.webm":   "aa7d027201634fe2015009e48e3e352c",
	"encode/crowd_run-640x360_60frames.vp9.webm":   "f65cb8d48140296490d35e492aef8f71",
	"encode/crowd_run-960x540_60frames.vp9.webm":   "adc1be70223745c547edf5385333bf6e",
	"encode/crowd_run-1280x720_60frames.vp9.webm":  "3ac4bda5f031923bcb9aa3d91d705691",
	"encode/crowd_run-1920x1080_60frames.vp9.webm": "1d8ded3ff5d8ebca7593ebf4f74f73cc",
	"encode/crowd_run-3840x2160_60frames.vp9.webm": "efd01049a3869ef64ac0e60a195aaf52",

	"encode/fallout4-1280x720_600frames.vp9.webm":  "9bad73fd85cc928632473b4f2ffb19f3",
	"encode/fallout4-1920x1080_290frames.vp9.webm": "f30d4ca840541ad8ed6a190b7518224d",
	"encode/fallout4-3840x2160_100frames.vp9.webm": "ef7a8783fed0915679f0a12f261943e7",

	"encode/gipsrecmotion-320x180_850frames.vp9.webm":   "e7c22a24cf97c8e504df27a042f94919",
	"encode/gipsrecmotion-480x270_850frames.vp9.webm":   "7c2d22379256f2f1d99096e94269626b",
	"encode/gipsrecmotion-640x360_850frames.vp9.webm":   "ad828894670ef9c42a42f79276962c64",
	"encode/gipsrecmotion-960x540_850frames.vp9.webm":   "883a251a180041d8657f96a6d1fad694",
	"encode/gipsrecmotion-1280x720_850frames.vp9.webm":  "a8b5749855522d837765728effcfa5ce",
	"encode/gipsrecmotion-1920x1080_430frames.vp9.webm": "e804086216bb777b789f9214ac9c9a9d",
	"encode/gipsrecmotion-3840x2160_140frames.vp9.webm": "965a2c31fe3cc96a6b142e080f22d03a",

	"encode/gipsrestat-320x180_846frames.vp9.webm":   "8d8963d58e4806733e37969159d7bdd4",
	"encode/gipsrestat-480x270_846frames.vp9.webm":   "0c425071c180d8a9552e330d684b0e6e",
	"encode/gipsrestat-640x360_846frames.vp9.webm":   "559638803b898bd1190301af6fa99806",
	"encode/gipsrestat-960x540_846frames.vp9.webm":   "24b33fc0725aebc608aba60b1bed68ee",
	"encode/gipsrestat-1280x720_846frames.vp9.webm":  "7de17229c45c5d4936beaafe4f0d81d8",
	"encode/gipsrestat-1920x1080_450frames.vp9.webm": "f79541aee3d2af780a20ca12cafb9846",
	"encode/gipsrestat-3840x2160_150frames.vp9.webm": "c4a42ff9ae6c8205900ecf5f1b84fea2",

	"encode/life_of_pixel-1920x1080_600frames.vp9.webm": "60a797f607ee18622e61e1090494c428",
	"encode/life_of_pixel-3840x2160_600frames.vp9.webm": "d78016bf96a0ee143d76e48e646249bd",

	"encode/life_of_process-1920x1080_600frames.vp9.webm": "12575c805342bdf5dc0460ef4ba11e8b",
	"encode/sc_web_browsing-1920x1080_300frames.vp9.webm": "ad2aa79912bd777cefd394948f3b5e2d",
}

// AccelPerfTestType denotes the tests to be run with video_encode_accelerator_perf_tests.
type AccelPerfTestType int

const (
	// None is only used to check if the AccelPerfTestType has the bit mask.
	None AccelPerfTestType = 0
	// Quality runs the quality performance tests.
	Quality AccelPerfTestType = (1 << 0)
	// Speed runs the speed performance tests.
	Speed AccelPerfTestType = (1 << 1)
	// SpeedAndQuality runs the speed and quality performance tests.
	SpeedAndQuality AccelPerfTestType = Quality | Speed
)

// ContentType represents the content type of a video
type ContentType int

const (
	// Camera denotes the content of a video is one produced by camera.
	Camera ContentType = iota
	// Display denotes the content of a video is one produced by display.
	Display ContentType = iota
)

// TestOptions is the options for runAccelVideoTest.
type TestOptions struct {
	// WebName is the file name of vp9 webm used in video_encode_accelerator_tests.
	WebMName string

	// Profile is the codec that the encoder produces.
	Profile videotype.CodecProfile

	// PSNRThreshold is the PSNR threshold used in video_encode_accelerator_tests.
	PSNRThreshold float32

	// TestType specifies the tests to be run.
	TestType AccelPerfTestType

	// The SVC scalability mode See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes*.
	SVCMode string

	// Used bitrate mode. Either cbr or vbr.
	BitrateMode string

	// Encode bitrate.
	Bitrate int

	// Content type
	Content ContentType
}

func webMJSONFileNameFor(webMFileName string) string {
	const webMSuffix = ".vp9.webm"
	if !strings.HasSuffix(webMFileName, webMSuffix) {
		return ""
	}
	return webMFileName + ".json"
}

func yuvJSONFileNameFor(webMFileName string) string {
	const webMSuffix = ".vp9.webm"
	if !strings.HasSuffix(webMFileName, webMSuffix) {
		return ""
	}
	yuvName := strings.TrimSuffix(webMFileName, webMSuffix) + ".yuv"
	return yuvName + ".json"
}

// fillYUVJSON fills a proper JSON values to yuvJSONFilePath for the YUV file
// generated by encoding.DecodeInI420WithNumFrames() with numEncodedFrames=60.
func fillYUVJSON(yuvJSONFilePath, webMJSONFilePath string) error {
	const webMJSONSuffix = ".vp9.webm.json"
	webMJSONFileName := filepath.Base(webMJSONFilePath)
	if !strings.HasSuffix(webMJSONFileName, webMJSONSuffix) {
		return errors.Errorf("webm json file doesn't end with vp9.webm.json: %v", webMJSONFileName)
	}
	webMJSONData, err := os.ReadFile(webMJSONFilePath)
	if err != nil {
		return errors.Wrapf(err, "read the webm json file: %s", webMJSONFilePath)
	}

	type webMJSON struct {
		PixelFormat string `json:"pixel_format"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		NumFrames   int    `json:"num_frames"`
		FrameRate   int    `json:"frame_rate"`
	}
	var webMInfo webMJSON
	if err = json.Unmarshal(webMJSONData, &webMInfo); err != nil {
		return errors.Wrap(err, "failed unmarshaling json file")
	}

	type yuvJSON struct {
		PixelFormat string `json:"pixel_format"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		NumFrames   int    `json:"num_frames"`
		FrameRate   int    `json:"frame_rate"`
	}
	yuvInfo := yuvJSON{
		PixelFormat: "I420",
		Width:       webMInfo.Width,
		Height:      webMInfo.Height,
		NumFrames:   numEncodeFrames,
		FrameRate:   webMInfo.FrameRate,
	}
	if webMInfo.NumFrames < numEncodeFrames {
		yuvInfo.NumFrames = webMInfo.NumFrames
	}

	yuvJSONData, err := json.MarshalIndent(yuvInfo, "", "\t")
	if err != nil {
		return errors.Wrap(err, "failed marshaling json")
	}

	return os.WriteFile(yuvJSONFilePath, yuvJSONData, 0644)
}

func layersOfSVCMode(svcMode string) (int, int, error) {
	spatialLayers := 0
	temporalLayers := 0
	if strings.HasSuffix(svcMode, "_KEY") {
		if _, err := fmt.Sscanf(svcMode, "L%dT%d_KEY", &spatialLayers, &temporalLayers); err != nil {
			return -1, -1, errors.Wrap(err, "unknown svc mode")
		}
	} else if strings.HasPrefix(svcMode, "S") {
		if _, err := fmt.Sscanf(svcMode, "S%dT%d", &spatialLayers, &temporalLayers); err != nil {
			return -1, -1, errors.Wrap(err, "unknown svc mode")
		}
	} else {
		if _, err := fmt.Sscanf(svcMode, "L%dT%d", &spatialLayers, &temporalLayers); err != nil {
			return -1, -1, errors.Wrap(err, "unknown svc mode")
		}
	}

	if spatialLayers <= 0 || temporalLayers <= 0 {
		return -1, -1, errors.Errorf("failed to parse svc mode: spatialLayers=%d, temporalLayers=%d", spatialLayers, temporalLayers)
	}
	return spatialLayers, temporalLayers, nil
}

func codecProfileToEncodeCodecOption(profile videotype.CodecProfile) (string, error) {
	switch profile {
	case videotype.H264BaselineProf:
		return "h264baseline", nil
	case videotype.H264MainProf:
		return "h264main", nil
	case videotype.H264HighProf:
		return "h264high", nil
	case videotype.VP8Prof:
		return "vp8", nil
	case videotype.VP9Prof:
		return "vp9", nil
	case videotype.AV1MainProf:
		return "av1", nil
	default:
		return "", errors.Errorf("unknown codec profile: %v", profile)
	}
}

// RunAccelVideoTest runs all tests in video_encode_accelerator_tests.
func RunAccelVideoTest(ctx context.Context, s *testing.State, opts TestOptions) {
	md5Hash, found := md5OfYUV60Frames[opts.WebMName]
	if !found {
		s.Fatal("Unknown webm file: ", opts.WebMName)
	}

	vl, err := logging.NewVideoLogger()
	if err != nil {
		s.Fatal("Failed to set values for verbose logging")
	}
	defer vl.Close()

	yuvPath, err := encoding.DecodeInI420WithNumFrames(ctx, s.DataPath(opts.WebMName),
		numEncodeFrames, md5Hash)
	if err != nil {
		s.Fatal("Failed to create a yuv file: ", err)
	} else if videovars.ShouldRemoveArtifacts(ctx) {
		defer os.Remove(yuvPath)
	}

	yuvJSONPath := yuvPath + ".json"
	if err := fillYUVJSON(yuvJSONPath, s.DataPath(webMJSONFileNameFor(opts.WebMName))); err != nil {
		s.Fatal("Failed to create a yuv json file: ", err)
	} else if videovars.ShouldRemoveArtifacts(ctx) {
		defer os.Remove(yuvJSONPath)
	}

	codec, err := codecProfileToEncodeCodecOption(opts.Profile)
	if err != nil {
		s.Fatal("Failed to get codec option: ", err)
	}
	testArgs := []string{logging.ChromeVmoduleFlag(),
		fmt.Sprintf("--codec=%s", codec),
		"--reverse",
		yuvPath,
		yuvJSONPath,
	}

	if opts.SVCMode != "" {
		testArgs = append(testArgs, fmt.Sprintf("--svc_mode=%s", opts.SVCMode))
	}
	if opts.BitrateMode != "" {
		testArgs = append(testArgs, fmt.Sprintf("--bitrate_mode=%s", opts.BitrateMode))
	}
	if opts.PSNRThreshold != 0 {
		testArgs = append(testArgs, fmt.Sprintf("--psnr_threshold=%.2f", opts.PSNRThreshold))
	}

	exec := filepath.Join(chrome.BinTestDir, "video_encode_accelerator_tests")
	logfile := filepath.Join(s.OutDir(), fmt.Sprintf("output_%s_%d.txt", filepath.Base(exec), time.Now().Unix()))
	t := gtest.New(exec, gtest.Logfile(logfile),
		gtest.ExtraArgs(testArgs...),
		gtest.UID(int(sysutil.ChronosUID)))

	command, _ := t.Args()
	if command != nil {
		testing.ContextLogf(ctx, "Running %s", shutil.EscapeSlice(command))
	}

	if report, err := t.Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		s.Fatalf("Failed to run %v: %v", exec, err)
	}
}

// RunAccelVideoPerfTest runs video_encode_accelerator_perf_tests with the specified video file.
// if opts.TestType has Speed mask, then uncapped performance, capped performance, multiple concurrent performance runs.
// The first 60 frames are decoded from the file to the I420 file. The test runs with the I420 file.
// Each test encodes 300 frames using the 60 frames.
// - Uncapped performance: encodes the frames by the hardware encoder as fast as possible.
// This provides an estimate of the encoders's max performance (e.g. the maximum FPS).
// - Capped performance: encodes by the hardware encoder with inputting frames at 30fps.
// This is used to measure cpu usage and power consumption in the practical case.
// - Multiple concurrent performance: the specified test video is encoded with multiple concurrent encoders as fast as possible.
// If opts.TestType has Quality mask, then quality performance runs.
// The vp9 webm file is input to the video_encode_accelerator_perf_tests. It encodes all the frames with decoding frames on demand.
// So that the input vp9 webm needs to have frequent key frames. The example command is below.
// `ffmpeg -i input.y4m -vcodec libvpx-vp9 -lossless 1 -force_key_frames "expr:gte(n,n_forced*10)" output.vp9.webm`
// - Quality performance: computes the quality metrics of the encoded stream.
func RunAccelVideoPerfTest(ctx context.Context, s *testing.State, opts TestOptions) error {
	codec, err := codecProfileToEncodeCodecOption(opts.Profile)
	if err != nil {
		return errors.Wrap(err, "failed to get codec option")
	}

	testArgs := []string{
		fmt.Sprintf("--codec=%s", codec),
		fmt.Sprintf("--output_folder=%s", s.OutDir()),
		"--reverse",
	}
	spatialLayers := 1
	temporalLayers := 1
	if opts.SVCMode != "" {
		spatialLayers, temporalLayers, err = layersOfSVCMode(opts.SVCMode)
		if err != nil {
			return errors.Wrap(err, "failed to get the number of layers from svc mode")
		}
		testArgs = append(testArgs, fmt.Sprintf("--svc_mode=%s", opts.SVCMode))
	}
	if opts.BitrateMode != "" {
		testArgs = append(testArgs, fmt.Sprintf("--bitrate_mode=%s", opts.BitrateMode))
	}
	if opts.Bitrate > 0 {
		testArgs = append(testArgs, fmt.Sprintf("--bitrate=%d", opts.Bitrate))
	}
	switch opts.Content {
	case Camera:
		testArgs = append(testArgs, fmt.Sprintf("--content_type=camera"))
	case Display:
		testArgs = append(testArgs, fmt.Sprintf("--content_type=display"))
	}

	p := perf.NewValues()
	if opts.TestType&Speed != None {
		if err := runAccelVideoSpeedPerfTest(ctx, s, testArgs, opts.WebMName, p); err != nil {
			return err
		}
	}
	if opts.TestType&Quality != None {
		if err := runAccelVideoQualityPerfTest(ctx, s, testArgs, opts.WebMName, opts.SVCMode, spatialLayers, temporalLayers, p); err != nil {
			return err
		}
	}

	if err := p.Save(s.OutDir()); err != nil {
		return errors.Wrap(err, "failed to save performance metrics")
	}

	return nil
}

func runAccelVideoSpeedPerfTest(ctx context.Context, s *testing.State, testArgs []string, webMName string, p *perf.Values) error {
	const (
		// Name of the uncapped performance test.
		uncappedTestname = "MeasureUncappedPerformance"
		// Name of the capped performance test.
		cappedTestname = "MeasureCappedPerformance"
		// Name of the multiple concurrent encoders test.
		multipleConcurrentTestname = "MeasureUncappedPerformance_MultipleConcurrentEncoders"
		// The binary performance test.
		exec = "video_encode_accelerator_perf_tests"
	)

	md5Hash, found := md5OfYUV60Frames[webMName]
	if !found {
		s.Fatal("Unknown webm file: ", webMName)
	}

	yuvPath, err := encoding.DecodeInI420WithNumFrames(ctx, s.DataPath(webMName),
		numEncodeFrames, md5Hash)
	if err != nil {
		s.Fatal("Failed to create a yuv file: ", err)
	} else if videovars.ShouldRemoveArtifacts(ctx) {
		defer os.Remove(yuvPath)
	}
	yuvJSONPath := yuvPath + ".json"
	if err := fillYUVJSON(yuvJSONPath, s.DataPath(webMJSONFileNameFor(webMName))); err != nil {
		s.Fatal("Failed to create a yuv json file: ", err)
	} else if videovars.ShouldRemoveArtifacts(ctx) {
		defer os.Remove(yuvJSONPath)
	}

	// Setup benchmark mode.
	cleanUpBenchmark, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to set up benchmark mode")
	}
	defer cleanUpBenchmark(ctx)

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for CPU to become idle")
	}

	testArgs = append(testArgs, []string{
		"--speed",
		yuvPath,
		yuvJSONPath,
	}...)

	// Test 1: Measure maximum FPS.
	if report, err := gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".uncap.log")),
		gtest.Filter(fmt.Sprintf("*%s:*%s", uncappedTestname, multipleConcurrentTestname)),
		gtest.ExtraArgs(testArgs...),
		gtest.UID(int(sysutil.ChronosUID)),
	).Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		return errors.Wrapf(err, "failed to run %v", exec)
	}
	uncappedJSON := filepath.Join(s.OutDir(), "VideoEncoderTest", uncappedTestname+".json")
	if _, err := os.Stat(uncappedJSON); os.IsNotExist(err) {
		return errors.Wrap(err, "failed to find uncapped performance metrics file")
	}
	if err := encoding.ParseUncappedPerfMetrics(uncappedJSON, p, "single_encoder"); err != nil {
		return errors.Wrap(err, "failed to parse uncapped performance metrics")
	}

	multipleConcurrentJSON := filepath.Join(s.OutDir(), "VideoEncoderTest", multipleConcurrentTestname+".json")
	if _, err := os.Stat(multipleConcurrentJSON); os.IsNotExist(err) {
		return errors.Wrap(err, "failed to find multiple concurrent encoders performance metrics file")
	}
	// Use the uncapped parser since only one performance evaluator is current being used in the test.
	// TODO(b/211783279) Replace this parser with one that can handle multiple captures.
	if err := encoding.ParseUncappedPerfMetrics(multipleConcurrentJSON, p, "multiple_concurrent_encoders"); err != nil {
		return errors.Wrap(err, "failed to parse multiple concurrent encoders performance metrics")
	}

	// Test 2: Measure CPU usage and power consumption while running capped
	// performance test only.
	measurements, err := mediacpu.MeasureProcessUsage(ctx, measureInterval, mediacpu.KillProcess, gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".cap.log")),
		gtest.Filter("*"+cappedTestname),
		// Repeat enough times to run for full measurement duration. We don't
		// use -1 here as this can result in huge log files (b/138822793).
		gtest.Repeat(1000),
		gtest.AlsoRunDisabledTests(),
		gtest.ExtraArgs(testArgs...),
		gtest.UID(int(sysutil.ChronosUID)),
	))
	if err != nil {
		return errors.Wrapf(err, "failed to measure CPU usage %v", exec)
	}
	p.Set(perf.Metric{
		Name:      "cpu_usage",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, measurements["cpu"])

	// Power measurements are not supported on all platforms.
	if power, ok := measurements["power"]; ok {
		p.Set(perf.Metric{
			Name:      "power_consumption",
			Unit:      "watt",
			Direction: perf.SmallerIsBetter,
		}, power)
	}
	return nil
}

func runAccelVideoQualityPerfTest(ctx context.Context, s *testing.State, testArgs []string, webMName, svcMode string, spatialLayers, temporalLayers int, p *perf.Values) error {
	const (
		// Name of the bitstream quality test.
		qualityTestname = "MeasureProducedBitstreamQuality"
		// The binary performance test.
		exec = "video_encode_accelerator_perf_tests"
	)
	testArgs = append(testArgs, []string{
		"--quality",
		s.DataPath(webMName),
		s.DataPath(webMJSONFileNameFor(webMName)),
	}...)

	// Test: Measure quality of the encoded bitstream.
	if report, err := gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".quality.log")),
		gtest.Filter(fmt.Sprintf("*%s", qualityTestname)),
		gtest.ExtraArgs(append(testArgs, "--quality")...),
		gtest.UID(int(sysutil.ChronosUID)),
	).Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		return errors.Wrapf(err, "failed to run %v", exec)
	}

	qualityJSONPath := filepath.Join(s.OutDir(), "VideoEncoderTest", qualityTestname)
	if temporalLayers > 1 || spatialLayers > 1 {
		for sID := 1; sID <= spatialLayers; sID++ {
			for tID := 1; tID <= temporalLayers; tID++ {
				var scalabilityMode string
				if strings.HasPrefix(svcMode, "S") {
					scalabilityMode = fmt.Sprintf("S%dT%d", sID, tID)
				} else {
					scalabilityMode = fmt.Sprintf("L%dT%d", sID, tID)
				}
				if err := addQualityMetrics(qualityJSONPath, scalabilityMode, p); err != nil {
					return errors.Wrapf(err, "failed to parse quality performance metrics for %v", scalabilityMode)
				}
			}
		}
	} else {
		if err := addQualityMetrics(qualityJSONPath, "", p); err != nil {
			return errors.Wrap(err, "failed to parse quality performance metrics")
		}
	}
	return nil
}

// addQualityMetrics reads quality metrics from JSON file for scalabilityMode and add them to p.
// Empty scalabilityMode represents the JSON contains the metrics of non SVC encoding.
func addQualityMetrics(qualityJSONPath, scalabilityMode string, p *perf.Values) error {
	qualityJSON := qualityJSONPath
	if scalabilityMode != "" {
		qualityJSON += "." + scalabilityMode
	}
	qualityJSON += ".json"
	if _, err := os.Stat(qualityJSON); os.IsNotExist(err) {
		return errors.Wrap(err, "failed to find quality performance metrics file")
	}
	if err := encoding.ParseQualityPerfMetrics(qualityJSON, scalabilityMode, p); err != nil {
		return errors.Wrap(err, "failed to parse quality performance metrics file")
	}
	return nil
}
