// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package inputlatency contains functions and structs used for measuring input latency on ARC.
package inputlatency

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	arcHostClockFmt = "arc-host-clock-client_%s_20200923"
	// InputLatencyPkgName is the package name for the input latency test application.
	inputLatencyPkgName = "org.chromium.arc.testapp.inputlatency"
)

var supportedArchs = []string{
	"x86",
	"x86_64",
	"armeabi-v7a",
	"arm64-v8a",
}

var appUIUpdatePollOptions = &testing.PollOptions{
	Timeout:  5 * time.Second,
	Interval: 10 * time.Millisecond,
}

func arcHostClockDest() string {
	return filepath.Join(adb.AndroidTmpDirPath, "arc-host-clock-client")
}

// AndroidData is the list of data dependencies that tests need to add to their
// testing.Test.Data in order to use arc-host-clock-client.
func AndroidData() []string {
	var data []string
	for _, arch := range supportedArchs {
		data = append(data, fmt.Sprintf(arcHostClockFmt, arch))
	}
	return data
}

// InstallArcHostClockClient installs the arc-host-clock-client test binary.
func InstallArcHostClockClient(ctx context.Context, a *arc.ARC, s *testing.State) error {
	out, err := a.Command(ctx, "getprop", "ro.product.cpu.abi").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to read android cpu abi")
	}
	arch := strings.TrimSpace(string(out))
	supported := false
	for _, a := range supportedArchs {
		if arch == a {
			supported = true
		}
	}
	if !supported {
		return errors.Errorf("unsupported Android abi %q", arch)
	}
	bin := s.DataPath(fmt.Sprintf(arcHostClockFmt, arch))
	dest := arcHostClockDest()
	if err := a.PushFile(ctx, bin, dest); err != nil {
		return errors.Wrapf(err, "unable to push %q to %q", bin, dest)
	}
	if err := a.Command(ctx, "chmod", "a+x", dest).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "unable to make %q executable", dest)
	}
	return nil
}

// InputEvent represents a single input event received by the helper app.
type InputEvent struct {
	// Time (in ns) that the event was sent by the kernel (filled by host).
	EventTimeNS int64
	// Time (in ns) that the event was received by the app.
	RecvTimeNS int64 `json:"receiveTimeNs"`
	// Event action that was received by the app.
	Action string `json:"action"`
}

// CalculateMetrics calculates mean, median, std dev, max and min for the given
// input events. The function getValue should return the value of the element
// corresponding to the given index.
func CalculateMetrics(events []InputEvent, getValue func(int) float64) (mean, median, stdDev, max, min float64) {
	n := len(events)
	sort.Slice(events, func(i, j int) bool { return getValue(i) < getValue(j) })
	min = getValue(0)
	max = getValue(n - 1)
	median = getValue(n / 2)
	sum := 0.
	for i := range events {
		sum += getValue(i)
	}
	mean = sum / float64(n)
	stdSum := 0.
	for i := range events {
		stdSum += math.Pow(getValue(i)-mean, 2)
	}
	stdDev = math.Sqrt(stdSum / float64(n))
	return
}

// WaitForEvents polls until the counter in the app UI is equal to count, then
// returns the input events from the helper app.
func WaitForEvents(ctx context.Context, d *ui.Device, count int, pkgName string) (string, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		v := d.Object(ui.ID(fmt.Sprintf("%s:id/event_count", pkgName)))
		txt, err := v.GetText(ctx)
		if err != nil {
			return err
		}
		num, err := strconv.ParseInt(txt, 10, 64)
		if err != nil {
			return err
		}
		if num != int64(count) {
			return errors.Errorf("unexpected event count; got %d, want %d", num, count)
		}
		return nil
	}, appUIUpdatePollOptions); err != nil {
		return "", err
	}

	// Press ESC key to finish event trace and generate JSON data.
	if err := d.PressKeyCode(ctx, ui.KEYCODE_ESCAPE, 0x0); err != nil {
		return "", err
	}

	var txt string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		v := d.Object(ui.ID(fmt.Sprintf("%s:id/event_json", pkgName)))
		var err error
		txt, err = v.GetText(ctx)
		if err != nil {
			return err
		}
		if txt == "" {
			return errors.New("waiting for generate JSON data")
		}
		return nil
	}, appUIUpdatePollOptions); err != nil {
		return "", err
	}
	return txt, nil
}

// WaitForClearUI clears the event data in ArcInputLatencyTest.apk to get ready for next event tracing.
func WaitForClearUI(ctx context.Context, d *ui.Device, pkgNamePtr *string) error {
	return WaitForClearUIWithWaitTime(ctx, d, pkgNamePtr, 0*time.Second)
}

// WaitForClearUIWithWaitTime waits for events from previous action to be received, and then
// clears the event data in ArcInputLatencyTest.apk to get ready for next event tracing.
func WaitForClearUIWithWaitTime(ctx context.Context, d *ui.Device, pkgNamePtr *string, ms time.Duration) error {
	// GoBigSleepLint: Wait for previous events to be received. Currently there's no good way to
	// detect if all events generated from the previous action are received. Waiting briefly seems
	// to be the best solution at the moment, which is also being used in WaitForNextEventTime.
	if err := testing.Sleep(ctx, ms*time.Millisecond); err != nil {
		return errors.Wrap(err, "timeout while waiting to generate next event time")
	}

	var pkgName string
	// Use default package name if nil.
	if pkgNamePtr != nil {
		pkgName = *pkgNamePtr
	} else {
		pkgName = inputLatencyPkgName
	}

	if err := d.PressKeyCode(ctx, ui.KEYCODE_DEL, 0x0); err != nil {
		return err
	}

	// Check whether events are cleared.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		v := d.Object(ui.ID(fmt.Sprintf("%s:id/event_count", pkgName)))
		txt, err := v.GetText(ctx)
		if err != nil {
			return err
		}
		num, err := strconv.ParseInt(txt, 10, 64)
		if err != nil {
			return err
		}
		if num != 0 {
			return errors.Errorf("failed to clean events; got %d, want 0", num)
		}
		return nil
	}, appUIUpdatePollOptions); err != nil {
		return err
	}
	return nil
}

// EvaluateLatency gets event data, calculates the latency, and adds the result to performance metrics.
func EvaluateLatency(ctx context.Context, s *testing.State, d *ui.Device,
	numEvents int, eventTimes []int64, perfName string, pkgNamePtr *string, pv *perf.Values) error {
	s.Log("Collecting results")

	var pkgName string
	// Use default package name if nil.
	if pkgNamePtr != nil {
		pkgName = *pkgNamePtr
	} else {
		pkgName = inputLatencyPkgName
	}

	txt, err := WaitForEvents(ctx, d, numEvents, pkgName)
	if err != nil {
		return errors.Wrap(err, "unable to wait for events")
	}
	var events []InputEvent
	if err := json.Unmarshal([]byte(txt), &events); err != nil {
		return errors.Wrap(err, "could not ummarshal events from app")
	}

	// Assign event RTC time.
	for i := range events {
		events[i].EventTimeNS = eventTimes[i]
	}

	mean, median, stdDev, max, min := CalculateMetrics(events, func(i int) float64 {
		return float64(events[i].RecvTimeNS-events[i].EventTimeNS) / 1000000.
	})
	s.Logf("Latency (ms): mean %f median %f std %f max %f min %f", mean, median, stdDev, max, min)

	pv.Set(perf.Metric{
		Name:      perfName,
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, mean)

	rawPv := perf.NewValues()
	for _, event := range events {
		rawPv.Append(perf.Metric{
			Name:      event.Action,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(event.RecvTimeNS-event.EventTimeNS)/1000000.)
	}

	rawPvSaveDir := filepath.Join(s.OutDir(), "raw."+perfName)
	// Save raw data for debugging purposes.
	if err = os.Mkdir(rawPvSaveDir, 0755); err != nil {
		s.Fatalf("Failed to create path %s", rawPvSaveDir)
	}
	if err := rawPv.Save(rawPvSaveDir); err != nil {
		s.Fatal("Failed saving raw data: ", err)
	}
	return nil
}

// Now returns the current time in nanoseconds from CLOCK_BOOTTIME.
func Now() (int64, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return 0, errors.Wrap(err, "clock_gettime(CLOCK_BOOTTIME) call failed")
	}
	return ts.Nano(), nil
}

// WaitForNextEventTime generates next event time with specific time interval in millisecond.
func WaitForNextEventTime(ctx context.Context, a *arc.ARC, eventTimes *[]int64, ms time.Duration) error {
	// GoBigSleepLint: Wait to generate next event time.
	if err := testing.Sleep(ctx, ms*time.Millisecond); err != nil {
		return errors.Wrap(err, "timeout while waiting to generate next event time")
	}

	diff := int64(0)
	if vmEnabled, err := arc.VMEnabled(); err != nil {
		return errors.Wrap(err, "unable to check install type of ARC")
	} else if vmEnabled {
		// Get current boottime diff between guest and host. Since the diff changes
		// value as the guest clock drifts, we must get the current diff before each
		// event.
		d, err := vmTimeDiff(ctx, a)
		if err != nil {
			return errors.Wrap(err, "unable to get VM time diff")
		}
		diff = d
	}

	now, err := Now()
	if err != nil {
		return errors.Wrap(err, "unable to get current time")
	}
	*eventTimes = append(*eventTimes, now-diff)
	return nil
}

// vmTimeDiff runs arc-host-clock-client in the VM to determine the time difference
// between ARCVM and the host.
func vmTimeDiff(ctx context.Context, a *arc.ARC) (int64, error) {
	out, err := a.Command(ctx, arcHostClockDest()).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to run arc-host-clock-client")
	}
	i, err := strconv.ParseInt(string(out), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "unable to convert arc-host-clock-client output")
	}
	return i, nil
}
