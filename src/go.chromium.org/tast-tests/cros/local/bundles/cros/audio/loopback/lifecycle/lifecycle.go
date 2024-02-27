// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lifecycle tests loopback device lifecycles
package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/testing"
)

const (
	rate     = 48000
	channels = 2

	// sox -n -t s16 -r 48000 /dev/null synth sine 440 vol 0.05 trim 0 10 stats
	volume              = 0.05
	expectedRMSLevelDB  = -29.03
	rmsLevelDBTolerance = 0.3

	// events happen within this time window will be treated as on schedule
	// used for logging only, does not cause tests to fail
	logTimingTolerance = 200 * time.Millisecond

	// allow a command to exceed this amount of time
	extraTimeout = time.Second
)

// Param for TestLifecycle
type Param struct {
	Playback     PlaybackAction // Action which plays the generated t.playbackRaw
	Capture      CaptureAction  // Action which captures audio to t.captureRaw
	ExtraActions []Action
	Checks       []Checker // Checks to perform on the captured file
}

// TestLifecycle configures a loopback device, plays synth audio
// captures using the flexible loopback device.
// It also checks captured samples if specified.
func TestLifecycle(ctx context.Context, s *testing.State, p *Param) {
	t := &tester{Param: p}
	t.Run(ctx, s)
}

type tester struct {
	*Param
	t0              time.Time // start time
	wg              *sync.WaitGroup
	playbackRaw     string
	captureRaw      string
	playbackWav     string
	captureWav      string
	testDurationSec int
}

func (t *tester) actions() []Action {
	var as []Action
	if t.Capture != nil {
		as = append(as, t.Capture)
	}
	if t.Playback != nil {
		as = append(as, t.Playback)
	}
	return append(as, t.ExtraActions...)
}

func (t *tester) Run(ctx context.Context, s *testing.State) {
	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Failed to restart CRAS: ", err)
	}

	if err := audio.SelectIODevices(ctx, cras, "ALSA_LOOPBACK", "ALSA_LOOPBACK"); err != nil {
		s.Fatal("Failed to set internal device as selected device: ", err)
	}

	t.playbackRaw = filepath.Join(s.OutDir(), "playback.raw")
	t.playbackWav = filepath.Join(s.OutDir(), "playback.wav")
	t.captureRaw = filepath.Join(s.OutDir(), "capture.raw")
	t.captureWav = filepath.Join(s.OutDir(), "capture.wav")

	t.testDurationSec = 0
	for _, tl := range t.actions() {
		if tl != nil {
			endSec := tl.getSchedule().endSec
			if t.testDurationSec < endSec {
				t.testDurationSec = endSec
			}
		}
	}

	testing.ContextLog(ctx, "Scheduled test timeline:")
	t.logTimelineHeader(ctx)
	for _, a := range t.actions() {
		a.maybeLogSchedule(ctx, t)
	}
	for _, c := range t.Checks {
		c.maybeLogSchedule(ctx, t)
	}

	var playbackRawData, captureRawData audio.TestRawData
	if t.Playback != nil {
		playbackRawData = audio.TestRawData{
			Path:          t.playbackRaw,
			BitsPerSample: 16,
			Channels:      channels,
			Rate:          rate,
			Frequencies:   []int{440, 440},
			Volume:        volume,
			Duration:      t.Playback.durationSec(),
		}
		// prepare playback sample
		if err = audio.GenerateTestRawData(ctx, playbackRawData); err != nil {
			s.Fatal("Failed to generate test audio data: ", err)
		}
	}

	t.wg = &sync.WaitGroup{}
	t.wg.Add(len(t.actions()))
	t.t0 = time.Now()

	// requestFloopMask changes the state of CRAS, restart it to avoid corruption
	defer audio.RestartCras(ctx)

	// Do playback & capture
	for _, a := range t.actions() {
		go func(a Action) {
			defer t.wg.Done()
			if a != nil {
				a.Do(ctx, s, t)
			}
		}(a)
	}

	t.wg.Wait()

	if t.Playback != nil {
		err = audio.ConvertRawToWav(ctx, playbackRawData, t.playbackWav)
		if err != nil {
			s.Error("Playback raw -> wav convertion failed: ", err)
		}
	}
	if t.Capture != nil {
		captureRawData = audio.TestRawData{
			Path:          t.captureRaw,
			BitsPerSample: 16,
			Rate:          rate,
			Channels:      channels,
		}
		err = audio.ConvertRawToWav(ctx, captureRawData, t.captureWav)
		if err != nil {
			s.Error("Capture raw -> wav convertion failed: ", err)
		}
	}

	for _, c := range t.Checks {
		c.Check(ctx, s, t)
	}
}

// logTimelineHeader prints the timeline axis for each lifecycle test plan.
// A typical output would be:
//
//	--- time -->: 0123456789
//
// for a 10-second-long test.
// The following rows for the schedule will follow this axis as a reference point for alignment.
func (t *tester) logTimelineHeader(ctx context.Context) {
	b := strings.Builder{}
	for i := 0; i < t.testDurationSec; i++ {
		b.WriteRune('0' + rune(i%10))
	}
	testing.ContextLogf(ctx, "%15s: %s", "--- time -->", b.String())
}

// logScheduleRow provide one line of the scheduled test plan given the name and the schedule.
// A typical output would be:
//
//	playback: --ppppp---
//
// that should be aligned with the output for logTimelineHeader provides.
func (t *tester) logScheduleRow(ctx context.Context, name string, c rune, s schedule) {
	b := strings.Builder{}
	i := 0
	for ; i < s.startSec; i++ {
		b.WriteRune('-')
	}
	for ; i < s.endSec; i++ {
		b.WriteRune(c)
	}
	for ; i < t.testDurationSec; i++ {
		b.WriteRune('-')
	}
	testing.ContextLogf(ctx, "%15s: %s", name, b.String())
}

// logAction provide one line of the actual action time with the message provided.
// If sleep is required, the log will sleep until targetSec before logging, thus also the action.
// A typical output would be:
//
//		Δt = 2.00000000s: start playback (on schedule)
//
//	 describing that at 2s after start time the playback started and is matching our schedule.
func (t *tester) logAction(ctx context.Context, targetSec int, msg string, sleep bool) {
	if sleep {
		// GoBigSleepLint: Sleep to make sure we start the log and action at targetSec seconds after starting time.
		testing.Sleep(
			ctx,
			t.t0.Add(time.Duration(targetSec)*time.Second).
				Sub(time.Now()),
		)
	}
	dt := time.Now().Sub(t.t0)
	overdue := dt - time.Duration(targetSec)*time.Second
	overdueMsg := "on schedule"
	if overdue > logTimingTolerance {
		overdueMsg = fmt.Sprintf("%s overdue", overdue)
	} else if overdue < -logTimingTolerance {
		overdueMsg = fmt.Sprintf("%s early", overdue)
	}
	testing.ContextLogf(ctx, "Δt = %s: %s (%s)", dt, msg, overdueMsg)
}
