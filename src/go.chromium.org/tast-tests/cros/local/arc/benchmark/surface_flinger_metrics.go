// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmark

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// frame is a struct used to store frame information from a single SurfaceFlinger
// latency entry.
type frame struct {
	// When the app started to draw, in ns.
	draw int64
	// The vsync immediately preceding SF submitting the frame to the h/w, in ns.
	vsync int64
	// Timestamp immediately after SF submitted that frame to the h/w, in ns.
	submit int64
}

// SurfaceFlingerMetrics contains information needed to call SurfaceFlinger and
// data returned by SurfaceFlinger.
type SurfaceFlingerMetrics struct {
	frames     []frame
	framerates []float64
	// Contains the last saved timestamp. Used to filter out duplicates.
	lastTimestamp int64
	appPkgName    string
	surfaceName   string
	arc           *arc.ARC
	collecting    chan bool
	collectingErr chan error
}

// NewSurfaceFlingerMetrics returns a new instance of SurfaceFlinger.
func NewSurfaceFlingerMetrics(appPkgName string, a *arc.ARC) *SurfaceFlingerMetrics {
	f := &SurfaceFlingerMetrics{
		frames:        nil,
		lastTimestamp: -1,
		appPkgName:    appPkgName,
		arc:           a,
	}
	return f
}

// Start starts the SurfaceFlinger dumpsys for reporting latencies.
func (f *SurfaceFlingerMetrics) Start(ctx context.Context) error {
	if f.collecting != nil {
		return errors.New("SurfaceFlinger start failed since it was already started")
	}
	f.collecting = make(chan bool)
	f.collectingErr = make(chan error, 1)
	if err := f.detectAndSetupSurfaceName(ctx); err != nil {
		return errors.Wrap(err, "failed to detect and setup surface name")
	}

	// TODO(b/230396035): Change the interval to be obtained based on the device's screen refresh rate.
	const screenRefreshInterval = 500 * time.Millisecond

	// Start running SurfaceFlinger.
	go func() {
		ticker := time.NewTicker(screenRefreshInterval)
		if _, err := f.arc.Command(ctx, "/system/bin/sh", "-c", "dumpsys SurfaceFlinger --timestats -enable -clear").Output(testexec.DumpLogOnError); err != nil {
			return
		}
		defer ticker.Stop()

		for {
			select {
			case <-f.collecting:
				close(f.collectingErr)
				return
			case <-ticker.C:
				if err := f.recentFrames(ctx); err != nil {
					f.collectingErr <- errors.Wrap(err, "failed to get recent frames")
					return
				}
				if err := f.recentFrameRates(ctx); err != nil {
					f.collectingErr <- errors.Wrap(err, "failed to get recent framerates")
					return
				}
			case <-ctx.Done():
				f.collectingErr <- ctx.Err()
				return
			}
		}
	}()

	return nil
}

// Stop stops the SurfaceFlinger dumpsys, calculates FPS and average latency, and returns them.
func (f *SurfaceFlingerMetrics) Stop(ctx context.Context) (fps, latency float64, err error) {
	if _, err := f.arc.Command(ctx, "/system/bin/sh", "-c", "dumpsys SurfaceFlinger --timestats -disable -clear").Output(testexec.DumpLogOnError); err != nil {
		return 0, 0, err
	}
	if f.collecting == nil {
		return 0, 0, errors.New("SurfaceFlinger stop failed since it was never started")
	}
	// Stop SurfaceFlinger routine.
	close(f.collecting)
	select {
	case err = <-f.collectingErr:
		if err != nil {
			return 0, 0, err
		}
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	}
	fps, latency, err = f.calculateMetrics()
	return fps, latency, err
}

// recentFrameRates saves all SurfaceFlinger timestats average FPS data
// points to the SurfaceFlinger struct.
func (f *SurfaceFlingerMetrics) recentFrameRates(ctx context.Context) error {
	/*
		adb shell dumpsys SurfaceFlinger --timestats -dump prints:
		displayRefreshRate = 60 fps
		renderRate = 60 fps
		uid = 10074
		layerName = com.mojang.minecraftpe/com.mojang.minecraftpe.MainActivity#145
		packageName =
		gameMode = Unsupported
		totalFrames = 28
		droppedFrames = 0
		lateAcquireFrames = 0
		badDesiredPresentFrames = 0
		Jank payload for this layer:
		totalTimelineFrames = 0
		jankyFrames = 0
		sfLongCpuJankyFrames = 0
		sfLongGpuJankyFrames = 0
		sfUnattributedJankyFrames = 0
		appUnattributedJankyFrames = 0
		sfSchedulingJankyFrames = 0
		sfPredictionErrorJankyFrames = 0
		appBufferStuffingJankyFrames = 0
		SetFrateRate vote for this layer:
		frameRate = 0.00
		frameRateCompatibility = Undefined
		seamlessness = Undefined
		averageFPS = 62.500
		present2present histogram is as below:
		0ms=0 1ms=0 2ms=0 (...)
		latch2present histogram is as below:
		0ms=0 1ms=0 2ms=0 (...)
		desired2present histogram is as below:
		0ms=0 1ms=0 2ms=0 (...)
		acquire2present histogram is as below:
		0ms=0 1ms=0 2ms=0 (...)
		post2present histogram is as below:
		0ms=0 1ms=0 2ms=0 (...)
		post2acquire histogram is as below:
		0ms=0 1ms=0 2ms=0 (...)

		Since we are only concerned with the framerates, we parse the output to
		draw out only the averageFPS readings.
	*/
	out, err := f.arc.Command(ctx, "/system/bin/sh", "-c", "dumpsys SurfaceFlinger --timestats -dump").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to execute SurfaceFlinger start command")
	}
	lines := strings.Split(string(out), "\n")
	// Loop through lines to find average FPS readings
	for _, line := range lines {
		if strings.Contains(line, "averageFPS") {
			entries := strings.Split(line, "=")
			var fps float64
			var parseError error
			print(entries[1])
			if fps, parseError = strconv.ParseFloat(strings.TrimSpace(entries[1]), 64); err != nil {
				return errors.Wrap(parseError, "failed to parse fps")
			}
			f.framerates = append(f.framerates, fps)
			break
		}
	}
	return nil
}

// recentFrames saves all recent frames generated by SurfaceFlinger to the
// SurfaceFlingerMetrics struct.
func (f *SurfaceFlingerMetrics) recentFrames(ctx context.Context) error {
	/*
	   adb shell dumpsys SurfaceFlinger --latency <window name> prints some
	   information about the last 127 frames displayed in that window.
	   The data returned looks like this:
	   16954612
	   7657467895508   7657482691352   7657493499756
	   7657484466553   7657499645964   7657511077881
	   7657500793457   7657516600576   7657527404785
	   (...)

	   The first line is the refresh period (here 16.95 ms), it is followed
	   by 127 lines w/ 3 timestamps in nanosecond each:
	   A) when the app started to draw
	   B) the vsync immediately preceding SF submitting the frame to the h/w
	   C) timestamp immediately after SF submitted that frame to the h/w

	   We use the special "SurfaceView" window name because the statistics for
	   the activity's main window are not updated when the main web content is
	   composited into a SurfaceView.

	   Additionally, the timestamps can be of value higher than Math.MaxInt32, so
	   they must be stored with int64 variables.
	*/
	// "/system/bin/sh -c {CMD}" executes "{CMD}" as is by escaping it, which then
	// gets passed off the shell as the original command. On certain devices, the
	// shell unquotes more layers of quotes than usual, which causes the SurfaceView
	// name to be ultimately different; by escaping the entire command with this
	// method, the unquoting will be negated.
	out, err := f.arc.Command(ctx, "/system/bin/sh", "-c", "dumpsys SurfaceFlinger --latency \""+f.surfaceName+"\"").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to execute SurfaceFlinger start command")
	}
	lines := strings.Split(string(out), "\n")
	// Store all recent frames into the SurfaceFlingerMetrics object.
	for _, line := range lines {
		entries := strings.Split(line, "\t")

		// If the line does not contain three entries or if the first entry is "0", skip;
		// there are various situations in which an entry could be "0", e.g: when the
		// SF stats are cleared and immediately this function is called.
		if len(entries) != 3 || entries[0] == "0" {
			continue
		}

		// Parse each line into ints.
		var draw, vsync, submit int64
		var parseError error
		if draw, parseError = strconv.ParseInt(entries[0], 10, 64); err != nil {
			return errors.Wrap(parseError, "failed to parse draw timestamp")
		}
		if vsync, parseError = strconv.ParseInt(entries[1], 10, 64); err != nil {
			return errors.Wrap(parseError, "failed to parse vsync timestamp")
		}
		if submit, parseError = strconv.ParseInt(entries[2], 10, 64); err != nil {
			return errors.Wrap(parseError, "failed to parse submit timestamp")
		}

		// A max integer denotes that a frame is still pending; continue if encountered.
		if draw == math.MaxInt64 || vsync == math.MaxInt64 || submit == math.MaxInt64 {
			continue
		}

		// If the frame is recent, append.
		if draw > f.lastTimestamp {
			f.frames = append(f.frames, frame{draw, vsync, submit})
		}
	}
	if len(f.frames) > 0 {
		// Reassign the latest timestamp.
		f.lastTimestamp = f.frames[len(f.frames)-1].draw
	}
	return nil
}

// calculateMetrics uses the collected frame data to calculate FPS and average latency respectively.
func (f *SurfaceFlingerMetrics) calculateMetrics() (fps, latency float64, err error) {
	// We need to normalize to seconds (divide by 1 billion).
	numFrames := len(f.frames)
	if numFrames < 2 {
		return 0, 0, errors.Errorf("Only %v frames recorded", numFrames)
	}
	// Calculate FPS using harmonic mean
	var timestatsFps float64

	// Calculate harmonic mean
	var reciprocalSum float64 = 0
	for i := 0; i < len(f.framerates); i++ {
		reciprocalSum += (1 / f.framerates[i])
	}
	timestatsFps = float64(len(f.framerates)) / reciprocalSum

	// Calculate average latency.
	latencies := 0.0
	for _, f := range f.frames {
		latencies += float64(f.submit-f.draw) / float64(time.Second)
	}
	latency = latencies / float64(numFrames)

	return timestatsFps, latency, nil
}

func (f *SurfaceFlingerMetrics) detectAndSetupSurfaceName(ctx context.Context) error {
	// Execute SurfaceFlinger list command.
	out, err := f.arc.Command(ctx, "dumpsys", "SurfaceFlinger", "--list").Output()
	if err != nil {
		return errors.Wrap(err, "failed to execute SurfaceFlinger list command")
	}

	// Check the surface view cases that may return multiple results.
	surfaceViewRegexps := []*regexp.Regexp{
		// Some devices may use the "SurfaceView[<appPkgName>/...](BLAST)#<digits>" format, which should be favored.
		// Ex: SurfaceView[air.com.lunime.gachaclub/air.com.lunime.gachaclub.AppEntry](BLAST)#205
		regexp.MustCompile(fmt.Sprintf(`(?m)^(SurfaceView\[%s\/.+\]\(BLAST\)\#\d+)`, f.appPkgName)),
		// Other use "SurfaceView - <appPkgName>".
		// Ex: SurfaceView - air.com.lunime.gachaclub/air.com.lunime.gachaclub.AppEntry#1
		regexp.MustCompile(fmt.Sprintf(`(?m)^(SurfaceView - %s[^\n]*)`, f.appPkgName)),
	}

	for _, r := range surfaceViewRegexps {
		groups := r.FindAllStringSubmatch(string(out), -1)
		if len(groups) > 0 {
			// UE4 games have at least two SurfaceView surfaces. The one that seems to in the foreground is the last one.
			lastGroup := groups[len(groups)-1]
			f.surfaceName = lastGroup[len(lastGroup)-1]
			testing.ContextLogf(ctx, "Using surface name: %s", f.surfaceName)
			return nil
		}
	}

	// Didn't find the first pattern, moving onto the second pattern.
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^(%s[^\n]*)`, f.appPkgName))
	match := re.FindString(string(out))
	if match == "" {
		return errors.Errorf("no matches found for app package name %s", f.appPkgName)
	}
	f.surfaceName = match
	testing.ContextLogf(ctx, "Using surface name: %s", f.surfaceName)
	return nil
}
