// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/audio"
	arcaudio "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/audio"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AudioOboetesterLatency,
		Desc: "Runs oboetester Round Trip Latency test and stores the result",
		Contacts: []string{
			"chromeos-audio-bugs@google.com", // Media team
			"pteerapong@chromium.org",        // Author
		},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Audio
		BugComponent: "b:879188",
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{"oboetester_debug.apk"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:audio"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name: "aaudio",
			Val: []arc.ActivityStartOption{
				arc.WithExtraString("in_api", "aaudio"),
				arc.WithExtraString("out_api", "aaudio"),
			},
			Fixture: "arcBooted",
		}, {
			Name:         "aaudio_pvsched",
			BugComponent: "b:167279",
			Val: []arc.ActivityStartOption{
				arc.WithExtraString("in_api", "aaudio"),
				arc.WithExtraString("out_api", "aaudio"),
			},
			Fixture:           "arcBootedWithPvSchedEnabled",
			ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
		}, {
			Name: "opensles",
			Val: []arc.ActivityStartOption{
				arc.WithExtraString("in_api", "opensles"),
				arc.WithExtraString("out_api", "opensles"),
			},
			Fixture: "arcBooted",
		}, {
			Name:         "opensles_pvsched",
			BugComponent: "b:167279",
			Val: []arc.ActivityStartOption{
				arc.WithExtraString("in_api", "opensles"),
				arc.WithExtraString("out_api", "opensles"),
			},
			Fixture:           "arcBootedWithPvSchedEnabled",
			ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
		}},
	})
}

// AudioOboetesterLatency runs oboetester Round Trip Latency test and stores the result.
func AudioOboetesterLatency(ctx context.Context, s *testing.State) {
	const (
		cleanupTime = 30 * time.Second

		apkName      = "oboetester_debug.apk"
		pkg          = "com.mobileer.oboetester"
		activityName = ".MainActivity"
		// Android download directory points to the same directory as ChromeOS' downloads directory.
		androidDownloadPath = "/storage/emulated/0/Download"
	)

	param := s.Param().([]arc.ActivityStartOption)
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

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

	// Generate an output file name and path.
	// If a static file name is used, the second test and subsequent tests will fail
	// because Oboetester will be unable to write to the file for some reason, even
	// after the file is removed.
	// Work around this by using a dynamic file name.
	outFileName := fmt.Sprintf("latency-result-%v.txt", time.Now().Format("20060102_150405"))
	hostDownloadPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's downloads path: ", err)
	}
	hostFilePath := path.Join(hostDownloadPath, outFileName)
	androidFilePath := path.Join(androidDownloadPath, outFileName)

	// Launch app
	param = append(param, arc.WithExtraString("test", "latency"))
	param = append(param, arc.WithExtraString("file", androidFilePath))
	if err := activity.Start(ctx, tconn, param...); err != nil {
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

	testing.ContextLog(ctx, "Waiting for the test to finish")
	if err := testing.Poll(ctx, func(ctx context.Context) (err error) {
		if _, err := os.Stat(hostFilePath); err != nil {
			return err
		}
		return nil // File exists so stop polling
	}, &testing.PollOptions{
		Timeout:  10 * time.Minute,
		Interval: 1 * time.Second,
	}); err != nil {
		s.Fatal("Failed to wait for result output file: ", err)
	}
	defer func() {
		if err := os.Remove(hostFilePath); err != nil {
			s.Error("Remove output file error: ", err)
		}
	}()

	resultBytes, err := os.ReadFile(hostFilePath)
	if err != nil {
		s.Fatalf("Failed to read result output file from %q: %v", hostFilePath, err)
	}
	resultText := string(resultBytes)

	// Parse `latency.msec = xx.xx`
	latencyRegex := regexp.MustCompile(`latency.msec = (\d+\.\d+)`)
	match := latencyRegex.FindStringSubmatch(resultText)
	if match == nil {
		s.Fatalf("Failed to find latency in result text. Result text = %q", resultText)
	}
	latency, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		s.Fatalf("Failed to parse latency text %q to float: %v", match[1], err)
	}

	perfValues := perf.NewValues()
	defer func() {
		if err := perfValues.Save(s.OutDir()); err != nil {
			s.Error("Cannot save perf data: ", err)
		}
	}()

	perfValues.Set(
		perf.Metric{
			Name:      "latency",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, latency)
}
