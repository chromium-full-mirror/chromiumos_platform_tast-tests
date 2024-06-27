// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	arcaudio "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/audio"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var audioOboetesterGlitchStrictModels = []string{
	// brya
	"redrix",

	// hatch
	"jinlon",
}

type audioOboetesterGlitchStressLoad int

const (
	// audioOboetesterGlitchStressLoadNone run the test without any stress test
	audioOboetesterGlitchStressLoadNone audioOboetesterGlitchStressLoad = iota

	// audioOboetesterGlitchStressLoadSpeedometer run the test with a single tab of speedometer in Chrome
	audioOboetesterGlitchStressLoadSpeedometer
)

type audioOboetesterGlitchParam struct {
	stressMode audioOboetesterGlitchStressLoad
	options    []arc.ActivityStartOption
	threshold  int // [Optional] Mark the test as failed if the number of glitch exceeds threshold. Ignore if zero.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioOboetesterGlitch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs Oboetester glitch test for 60 seconds and stores the result",
		Contacts: []string{
			"chromeos-audio-bugs@google.com", // Media team
			"pteerapong@chromium.org",        // Author
		},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Audio
		BugComponent: "b:879188",
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{"oboetester_debug.apk"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:audio"},
		Timeout:      7 * time.Minute,
		Params: []testing.Param{
			{
				Name: "aaudio_noload",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadNone,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "aaudio"),
						arc.WithExtraString("out_api", "aaudio"),
					},
					threshold: 30,
				},
				Fixture: "arcBooted",
			},
			{
				Name:         "aaudio_noload_pvsched",
				BugComponent: "b:167279",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadNone,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "aaudio"),
						arc.WithExtraString("out_api", "aaudio"),
					},
					threshold: 30,
				},
				Fixture:           "arcBootedWithPvSchedEnabled",
				ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
			},
			{
				Name: "aaudio_speedometer",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadSpeedometer,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "aaudio"),
						arc.WithExtraString("out_api", "aaudio"),
					},
				},
				Fixture: "arcBooted",
			},
			{
				Name:         "aaudio_speedometer_pvsched",
				BugComponent: "b:167279",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadSpeedometer,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "aaudio"),
						arc.WithExtraString("out_api", "aaudio"),
					},
				},
				Fixture:           "arcBootedWithPvSchedEnabled",
				ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
			},
			{
				Name: "opensles_noload",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadNone,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "opensles"),
						arc.WithExtraString("out_api", "opensles"),
					},
					threshold: 30,
				},
				Fixture: "arcBooted",
			},
			{
				Name:         "opensles_noload_pvsched",
				BugComponent: "b:167279",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadNone,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "opensles"),
						arc.WithExtraString("out_api", "opensles"),
					},
					threshold: 30,
				},
				Fixture:           "arcBootedWithPvSchedEnabled",
				ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
			},
			{
				Name: "opensles_speedometer",
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadSpeedometer,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "opensles"),
						arc.WithExtraString("out_api", "opensles"),
					},
				},
				Fixture: "arcBooted",
			},

			// Run the test with a stricter threshold for the selected models.
			{
				Name:              "aaudio_noload_strict",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(audioOboetesterGlitchStrictModels...)),
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadNone,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "aaudio"),
						arc.WithExtraString("out_api", "aaudio"),
					},
					threshold: 5,
				},
				Fixture: "arcBooted",
			},
			{
				Name:              "opensles_noload_strict",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(audioOboetesterGlitchStrictModels...)),
				Val: audioOboetesterGlitchParam{
					stressMode: audioOboetesterGlitchStressLoadNone,
					options: []arc.ActivityStartOption{
						arc.WithExtraString("in_api", "opensles"),
						arc.WithExtraString("out_api", "opensles"),
					},
					threshold: 5,
				},
				Fixture: "arcBooted",
			},
		},
	})
}

// gzipAndDeleteFile gzip the source file to the destination and delete the source file if the gzip succeeded.
func gzipAndDeleteFile(srcFilePath, destFilePath string) error {
	deleteSrc := false
	src, err := os.Open(srcFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open %v", srcFilePath)
	}
	defer func() {
		_ = src.Close()
		if deleteSrc {
			_ = os.Remove(srcFilePath)
		}
	}()

	var dst io.WriteCloser
	if dst, err = os.Create(destFilePath); err != nil {
		return errors.Wrapf(err, "failed to create %v", destFilePath)
	}
	defer dst.Close()

	dstGzip := gzip.NewWriter(dst)
	if _, err := io.Copy(dstGzip, src); err != nil {
		_ = dstGzip.Close()
		return errors.Wrap(err, "failed to gzip")
	}
	if err := dstGzip.Close(); err != nil {
		return errors.Wrap(err, "failed to close gzip")
	}

	deleteSrc = true
	return nil
}

// AudioOboetesterGlitch runs Oboetester glitch test for 60 seconds and stores the result.
func AudioOboetesterGlitch(ctx context.Context, s *testing.State) {
	const (
		cleanupTime  = 30 * time.Second
		testDuration = 60 // test duration in seconds.

		apkName      = "oboetester_debug.apk"
		pkg          = "com.mobileer.oboetester"
		activityName = ".MainActivity"
	)

	param := s.Param().(audioOboetesterGlitchParam)
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	// Restart CRAS to reset state that might affect this test (e.g. system mute)
	if _, err := audio.RestartCras(ctx); err != nil {
		s.Fatal("Failed to restart cras: ", err)
	}

	// Reserve time to remove input file and unload ALSA loopback at the end of the test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	cleanup, err := arcaudio.SetupLoopbackDevice(ctx, cr, s.OutDir(), s.HasError)
	if err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}
	defer cleanup(cleanupCtx)

	testing.ContextLog(ctx, "Install app")
	if err := a.Install(ctx, s.DataPath(apkName)); err != nil {
		s.Fatal("Failed to install app: ", err)
	}
	defer a.Uninstall(cleanupCtx, pkg)

	// Grant permissions and create activity
	if err := a.GrantPermission(ctx, pkg, "android.permission.RECORD_AUDIO"); err != nil {
		s.Fatal("Failed to grant RECORD_AUDIO permission: ", err)
	}
	if err := a.GrantPermission(ctx, pkg, "android.permission.WRITE_EXTERNAL_STORAGE"); err != nil {
		s.Fatal("Failed to grant WRITE_EXTERNAL_STORAGE permission: ", err)
	}
	activity, err := arc.NewActivity(a, pkg, activityName)
	if err != nil {
		s.Fatalf("Failed to create activity %q in package %q: %v", activityName, pkg, err)
	}
	defer activity.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Start stress test
	switch param.stressMode {
	case audioOboetesterGlitchStressLoadSpeedometer:
		// Open and start Speedometer test. Note that we don't wait for it to finish.
		// It will be closed once the glitch test is finished.
		conn, err := cr.NewConn(ctx, "https://browserbench.org/Speedometer2.0/")
		if err != nil {
			s.Fatal("Failed to open Speedometer website: ", err)
		}
		defer func() {
			conn.CloseTarget(cleanupCtx)
			conn.Close()
		}()

		uia := uiauto.New(tconn)
		startButton := nodewith.Name("Start Test").Role(role.Button).Onscreen()
		if err := uiauto.Combine("Click Start Test",
			uia.WaitUntilExists(startButton),
			uia.LeftClick(startButton),
		)(ctx); err != nil {
			s.Fatal("Failed to start speedometer: ", err)
		}
	}

	// Dump audio diagnostics once the test is finished.
	defer func(ctx context.Context) {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
	}(cleanupCtx)

	// Capture audio with crastestclient and gzip the result.
	rawCaptureFilePath := path.Join(s.OutDir(), "capture.raw")
	gzipCaptureFilePath := path.Join(s.OutDir(), "capture.raw.gz")
	cmd := crastestclient.CaptureFileCommand(
		ctx, rawCaptureFilePath,
		testDuration+5, // Add 5 seconds buffer for the audio starting delay.
		8,
		48000)
	if err := cmd.Start(); err != nil {
		s.Fatal("Start crastestclient capture error: ", err)
	}
	defer func(ctx context.Context) {
		if err := cmd.Wait(testexec.DumpLogOnError); err != nil {
			testing.ContextLog(ctx, "Wait for crastestclient capture error: ", err)
			return
		}

		// Raw recording file greatly benefits from compression. (From 50MB to <1MB)
		testing.ContextLog(ctx, "Gzipping capture.raw file")
		if err := gzipAndDeleteFile(rawCaptureFilePath, gzipCaptureFilePath); err != nil {
			testing.ContextLog(ctx, "Gzip capture.raw file error: ", err)
		}
	}(cleanupCtx)

	// Launch app
	launchParams := param.options
	launchParams = append(launchParams, arc.WithExtraString("test", "glitch"))
	launchParams = append(launchParams, arc.WithExtraInt("duration", testDuration))
	if err := activity.Start(ctx, tconn, launchParams...); err != nil {
		s.Fatalf("Failed to start activity %q in package %q: %v", activityName, pkg, err)
	}
	defer func(ctx context.Context) error {
		// Check that app is still running
		if _, err := ash.GetARCAppWindowInfo(ctx, tconn, activity.PackageName()); err != nil {
			return err
		}
		testing.ContextLogf(ctx, "Stopping activities in package %s", pkg)
		return activity.Stop(ctx, tconn)
	}(cleanupCtx)

	testing.ContextLogf(ctx, "Sleeping for %v seconds to wait for the test to finish", testDuration)
	// GoBigSleepLint: Run the test for testDuration seconds as a part of measurement.
	testing.Sleep(ctx, testDuration*time.Second)

	// Polling until the time total in `time.total = xx.xx seconds` is more than testDuration.
	testing.ContextLog(ctx, "Polling for the test to finish")
	var resultText string
	timeTotalRegex := regexp.MustCompile(`time.total = (\d+\.\d+) seconds`)
	if err := testing.Poll(ctx, func(ctx context.Context) (err error) {
		resultText, err = d.Object(ui.ID("com.mobileer.oboetester:id/text_status")).GetText(ctx)
		if err != nil {
			return err
		}
		match := timeTotalRegex.FindStringSubmatch(resultText)
		if match == nil {
			s.Fatalf("Failed to find time total in result text. Result text = %q", resultText)
		}
		timeTotal, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			s.Fatalf("Failed to parse time total %q to float: %v", match[1], err)
		}
		if timeTotal < testDuration {
			return errors.Errorf("time total %.2f is less than test duration", timeTotal)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  1 * time.Minute,
		Interval: 1 * time.Second,
	}); err != nil {
		s.Fatal("Failed to wait for result OK text: ", err)
	}

	// Parse `glitch.count = <number>`
	glitchCountRegex := regexp.MustCompile(`glitch.count = (\d+)`)
	match := glitchCountRegex.FindStringSubmatch(resultText)
	if match == nil {
		s.Fatalf("Failed to find glitch count in result text. Result text = %q", resultText)
	}
	glitchCount, err := strconv.Atoi(match[1])
	if err != nil {
		s.Fatalf("Failed to parse glitch count %q to int: %v", match[1], err)
	}

	// Parse `glitch.frames = <number>`
	glitchFramesRegex := regexp.MustCompile(`glitch.frames = (\d+)`)
	match = glitchFramesRegex.FindStringSubmatch(resultText)
	if match == nil {
		s.Fatalf("Failed to find glitch frames in result text. Result text = %q", resultText)
	}
	glitchFrames, err := strconv.Atoi(match[1])
	if err != nil {
		s.Fatalf("Failed to parse glitch frames %q to int: %v", match[1], err)
	}

	// Parse xrun# from the input and output stream configuration.
	xrunRegex := regexp.MustCompile(`xRun# = (\d+)`)
	getXrunOfStreamConfig := func(streamConfigID string) int {
		statusView := d.Object(ui.TextContains("xRun# = "))
		if err := d.Object(ui.ID(streamConfigID)).GetChild(ctx, statusView); err != nil {
			s.Fatal("Failed to get statusView: ", err)
		}
		statusText, err := statusView.GetText(ctx)
		if err != nil {
			s.Fatal("Failed to get statusText: ", err)
		}
		match = xrunRegex.FindStringSubmatch(statusText)
		if match == nil {
			s.Fatalf("Failed to find xRun in text. statusText = %q", statusText)
		}
		xrun, err := strconv.Atoi(match[1])
		if err != nil {
			s.Fatalf("Failed to parse xrun %q to int: %v", match[1], err)
		}
		return xrun
	}
	inputXrun := getXrunOfStreamConfig("com.mobileer.oboetester:id/inputStreamConfiguration")
	outputXrun := getXrunOfStreamConfig("com.mobileer.oboetester:id/outputStreamConfiguration")

	// Stores test result
	perfValues := perf.NewValues()
	defer func() {
		if err := perfValues.Save(s.OutDir()); err != nil {
			s.Error("Cannot save perf data: ", err)
		}
	}()

	perfValues.Set(perf.Metric{
		Name:      "glitch_count",
		Unit:      "times",
		Direction: perf.SmallerIsBetter,
	}, float64(glitchCount))

	perfValues.Set(perf.Metric{
		Name:      "glitch_frames",
		Unit:      "frames",
		Direction: perf.SmallerIsBetter,
	}, float64(glitchFrames))

	perfValues.Set(perf.Metric{
		Name:      "input_xrun",
		Unit:      "times",
		Direction: perf.SmallerIsBetter,
	}, float64(inputXrun))

	perfValues.Set(perf.Metric{
		Name:      "output_xrun",
		Unit:      "times",
		Direction: perf.SmallerIsBetter,
	}, float64(outputXrun))

	if param.threshold != 0 && glitchCount > param.threshold {
		s.Errorf("glitch count exceeds threshold: %d > %d", glitchCount, param.threshold)
	}
}
