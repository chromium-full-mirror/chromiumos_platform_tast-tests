// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bootperf provides constants and common utilities for test platform.BootPerf.
package bootperf

import (
	"context"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	uptimePrefix = "uptime-"
	diskPrefix   = "disk-"

	// Directory where the current statistics are stored.
	bootstatCurrentDir = "/run/bootstat"

	// The chromeos_shutdown script archives bootstat files under shutdown.TIMESTAMP directory. The timestamp is generated using `date '+%Y%m%d%H%M%S'`.
	bootstatArchiveGlob = "/var/log/metrics/shutdown.[0-9]*"

	// disk usage bootstat numbers are sectors. Convert to bytes by multiplying |sectorSize|.
	sectorSize = 512

	// The path of boot ID on the proc filesystem.
	currentBootIDPath = "/proc/sys/kernel/random/boot_id"
)

type metricRequirement int

const (
	// An Optional metric may be collected, but we will not wait for it.
	metricOptional metricRequirement = iota
	// We will wait a reasonable amount of time for a Recommended metric, but won't abort if it's not found.
	metricRecommended
	// We will wait for a Required metric, and abort if it's not found.
	metricRequired
)

var (
	// Names of metrics, their associated bootstat events, and their recommendation status.
	// The test fails if a Required event is not found.
	// A Recommended event will cause the test to wait for some reasonable time to allow the event to be recorded,
	// but not fail the test if it doesn't show up. This can allow for some level of flake in an underlying event
	// (e.g., WiFi hardware failure) without making it completely optional (which would otherwise fail to report if
	// the event is slower than the slowest Required event).
	// Each event samples statistics measured since kernel startup at a specific moment on the boot critical path:
	//   pre-startup - The start of the `chromeos_startup` script;
	//     roughly, the time when /sbin/init emits the `startup`
	//     Upstart event.
	//   post-startup - Completion of the `chromeos_startup` script.
	//   chrome-exec - The moment when session_manager exec's the
	//     first Chrome process.
	//   chrome-main - The moment when the first Chrome process
	//     begins executing in main().
	//   kernel_to_signin_start - The moment when LoadPage(loginScreenURL)
	//     is called, i.e. initialization starts.
	//   kernel_to_signin_wait - The moment when UI thread has finished signin
	//     screen initialization and now waits until JS sends "ready" event.
	//   kernel_to_signin_users - The moment when UI thread receives "ready" from
	//     JS code. So V8 is initialized and running, etc...
	//   kernel_to_login - The moment when user can actually see signin UI.
	//   kernel_to_android_start - The moment when Android is started.
	//   kernel_to_cellular_registered - The moment when Shill detects a
	//     cellular device.
	//   kernel_to_wifi_registered - The moment when Shill detects a WiFi device.
	eventMetrics = []struct {
		MetricName  string
		EventName   string
		Requirement metricRequirement
	}{
		{"kernel_to_startup", "pre-startup", metricRequired},
		{"kernel_to_startup_done", "post-startup", metricRequired},
		{"kernel_to_chrome_exec", "chrome-exec", metricRequired},
		// TODO(b/180082486): Change to optional temporarily, because
		// this file is written by Chrome, we have to wait for the fix
		// in Chrome.
		{"kernel_to_chrome_main", "chrome-main", metricOptional},
		// These two events do not happen if device is in OOBE.
		{"kernel_to_signin_start", "login-start-signin-screen", metricOptional},
		{"kernel_to_signin_wait", "login-wait-for-signin-state-initialize", metricOptional},
		// This event doesn't happen if device has no users.
		{"kernel_to_signin_users", "login-send-user-list", metricOptional},
		{"kernel_to_login", "login-prompt-visible", metricRequired},
		// Not all boards support ARC.
		{"kernel_to_android_start", "android-start", metricOptional},
		// Not all devices have cellular.
		{"kernel_to_cellular_registered", "network-cellular-registered", metricOptional},
		// All should have WiFi, but we still don't want to fail (e.g., if there are hardware issues).
		{"kernel_to_wifi_registered", "network-wifi-registered", metricRecommended},
	}

	uptimeFileGlob = filepath.Join(bootstatCurrentDir, uptimePrefix+"*")
	diskFileGlob   = filepath.Join(bootstatCurrentDir, diskPrefix+"*")

	syncRtcTlsdatedStart = "sync-rtc-tlsdated-start"
	syncRtcTlsdatedStop  = "sync-rtc-tlsdated-stop"

	// The name of this file has changed starting with linux-3.19.
	// Use a glob to snarf up all existing records.
	ramOopsFileGlob = "/sys/fs/pstore/console-ramoops*"
)

// WaitUntilBootComplete is a helper function to wait until boot complete and
// we are ready to collect boot metrics.
func WaitUntilBootComplete(ctx context.Context) error {
	// Defines the running states of upstart jobs the test waits for.
	upstartJobTargets := []struct {
		JobName     string
		TargetState upstartcommon.State
	}{
		{"bootchart", upstartcommon.WaitingState},
		{"system-services", upstartcommon.RunningState},
		{"tlsdated", upstartcommon.RunningState},
	}

	pollOnce := func(ctx context.Context, recommendation metricRequirement) error {
		// Check that bootstat files are available.
		for _, k := range eventMetrics {
			if k.Requirement < recommendation {
				continue
			}

			for _, prefix := range []string{uptimePrefix, diskPrefix} {
				key := filepath.Join(bootstatCurrentDir, prefix+k.EventName)
				if _, err := os.Stat(key); err != nil {
					return errors.Wrapf(err, "error in waiting for bootstat file %s", key)
				}
			}
		}

		// Wait until upstart jobs enter the desired states.
		for _, job := range upstartJobTargets {
			_, state, _, err := upstart.JobStatus(ctx, job.JobName)
			if err != nil {
				return errors.Wrapf(err, "failed to get status of job %q", job.JobName)
			}
			if state != job.TargetState {
				return errors.Errorf("unexpected %s state: want: %s, got %s", job.JobName, job.TargetState, state)
			}
		}

		return nil
	}

	// Wait for Recommended and Required events for the first 60 seconds.
	if err := testing.Poll(ctx, func(context.Context) error {
		return pollOnce(ctx, metricRecommended)
	}, &testing.PollOptions{
		Timeout:  60 * time.Second,
		Interval: time.Second,
	}); err == nil {
		return nil
	}

	// Try one last time with only Required metrics, in case a Recommended metric wasn't found.
	return pollOnce(ctx, metricRequired)
}

// EnsureTlsdatedStopped stops tlsdated and checks if the sync-rtc-tlsdated-* bootstat files are present.
func EnsureTlsdatedStopped(ctx context.Context) error {
	syncRtcTlsdatedStartFile := filepath.Join(bootstatCurrentDir, syncRtcTlsdatedStart)
	syncRtcTlsdatedStopFile := filepath.Join(bootstatCurrentDir, syncRtcTlsdatedStop)

	// tlsdated should be running.
	_, state, _, err := upstart.JobStatus(ctx, "tlsdated")
	if err != nil {
		return errors.Wrap(err, "failed to get job status of tlsdated")
	}
	if state != upstartcommon.RunningState {
		return errors.Errorf("tlsdated in %s state", state)
	}

	// sync-rtc-tlsdated-start is generated before tlsdated starts. See /etc/init/tlsdated.conf for more details.
	if _, err := os.Stat(syncRtcTlsdatedStartFile); err != nil {
		return errors.Wrapf(err, "failed to check the existence of %s", syncRtcTlsdatedStartFile)
	}
	if err := upstart.StopJob(ctx, "tlsdated"); err != nil {
		return err
	}
	// sync-rtc-tlsdated-stop is generated after tlsdated stops.
	if _, err := os.Stat(syncRtcTlsdatedStopFile); err != nil {
		return errors.Wrapf(err, "failed to check the existence of %s", syncRtcTlsdatedStopFile)
	}

	return nil
}

// parseBootstat reads values from a bootstat event file. Each line of a
// bootstat event file represents one occurrence of the event. Each line is a
// copy of the content of /proc/uptime ("uptime-" files) or
// /sys/block/<dev>/stat ("disk-" files), captured at the time of the
// occurrence. For either kind of file, each line is a blank separated list of
// fields. The given event file can contain either uptime or disk data. This
// function reads all lines (occurrences) in the event file, and returns the
// value of the given field.
func parseBootstat(fileName string, fieldNum int) ([]float64, error) {
	var result []float64
	b, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	if len(b) == 0 {
		return nil, errors.Errorf("bootstat file %s is empty", fileName)
	}

	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		f := strings.Fields(line)
		if fieldNum >= len(f) {
			continue
		}
		s, err := strconv.ParseFloat(f[fieldNum], 64)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed bootstat content: %s", line)
		}
		result = append(result, s)
	}

	return result, nil
}

// parseUptime returns time since boot for a bootstat event.
func parseUptime(eventName, bootstatDir string, index int) (float64, error) {
	eventFile := filepath.Join(bootstatDir, uptimePrefix+eventName)
	val, err := parseBootstat(eventFile, 0)
	if err != nil {
		return 0.0, err
	}

	n := len(val)
	// Check for OOB access.
	if index < -n || index > n-1 {
		return 0.0, errors.Errorf("bootstat index out of bound. len=%d, index=%d", n, index)
	}

	if index >= 0 {
		return val[index], nil
	}
	// Like negative index in python.
	return val[n+index], nil
}

// GatherTimeMetrics reads and reports boot time metrics. It reads
// "seconds since kernel startup" from the bootstat files for the events named
// in |eventMetrics|, and stores the values as perf metrics.  The following
// metrics may be recorded:
//   - seconds_kernel_to_startup
//   - seconds_kernel_to_startup_done
//   - seconds_kernel_to_chrome_exec
//   - seconds_kernel_to_chrome_main
//   - seconds_kernel_to_signin_start
//   - seconds_kernel_to_signin_wait
//   - seconds_kernel_to_signin_users
//   - seconds_kernel_to_login
//   - seconds_kernel_to_android_start
//   - seconds_kernel_to_cellular_registered
//   - seconds_kernel_to_wifi_registered
//   - seconds_kernel_to_network
func GatherTimeMetrics(ctx context.Context, results *platform.GetBootPerfMetricsResponse) error {
	var missingNonRequiredEvennts []string
	for _, k := range eventMetrics {
		key := "seconds_" + k.MetricName
		val, err := parseUptime(k.EventName, bootstatCurrentDir, 0)
		if err != nil {
			if k.Requirement == metricRequired {
				return errors.Wrapf(err, "failed in gather time for %s", k.EventName)
			}
			// Failed in getting a non-required metric. Log and skip.
			missingNonRequiredEvennts = append(missingNonRequiredEvennts, k.EventName)
		} else {
			results.Metrics[key] = val
		}
	}
	if len(missingNonRequiredEvennts) != 0 {
		testing.ContextLogf(ctx, "Skip gathering time metrics for non-required event: %s", strings.Join(missingNonRequiredEvennts, ", "))
	}

	// Not all 'uptime-network-*-ready' files necessarily exist; probably there's only one.
	// We go through a list of possibilities and pick the earliest one we find.
	// We're not looking for 3G here, so we're not guaranteed to find any file.
	networkReadyEvents := []string{"network-wifi-ready", "network-ethernet-ready"}
	firstNetworkReadyTime := math.MaxFloat64
	for _, e := range networkReadyEvents {
		metricName := "seconds_kernel_to_" + strings.ReplaceAll(e, "-", "_")
		t, err := parseUptime(e, bootstatCurrentDir, 0)
		if err != nil {
			continue
		}

		results.Metrics[metricName] = t
		if t < firstNetworkReadyTime {
			firstNetworkReadyTime = t
		}
	}

	if firstNetworkReadyTime != math.MaxFloat64 {
		results.Metrics["seconds_kernel_to_network"] = firstNetworkReadyTime
	}

	return nil
}

// parseDiskstat returns sectors read since boot for a bootstat event.
func parseDiskstat(eventName, bootstatDir string, index int) (float64, error) {
	eventFile := filepath.Join(bootstatDir, diskPrefix+eventName)
	val, err := parseBootstat(eventFile, 2)
	if err != nil {
		return 0.0, err
	}
	return val[index], nil
}

// GatherDiskMetrics reads and reports disk read metrics.
// It reads "sectors read since kernel startup" from the bootstat files for the
// events named in |eventMetrics|, converts the values to "bytes read since
// boot", and stores the values as perf metrics. The following metrics are
// recorded:
//   - rdbytes_kernel_to_startup
//   - rdbytes_kernel_to_startup_done
//   - rdbytes_kernel_to_chrome_exec
//   - rdbytes_kernel_to_chrome_main
//   - rdbytes_kernel_to_login
//
// Disk statistics are reported in units of 512 byte sectors; we convert the
// metrics to bytes so that downstream consumers don't have to ask "How big is
// a sector?".
func GatherDiskMetrics(results *platform.GetBootPerfMetricsResponse) {
	// We expect an error when reading disk statistics for the "chrome-main" event because Chrome (not bootstat) generates that event, and it doesn't include the disk statistics.
	// We get around that by ignoring all errors.
	for _, k := range eventMetrics {
		key := "rdbytes_" + k.MetricName
		val, err := parseDiskstat(k.EventName, bootstatCurrentDir, 0)
		if err == nil {
			results.Metrics[key] = val * sectorSize
		} // else skip the error and continue.
	}
}

// GatherFirmwareBootTime gets firmware startup time from the firmware log or the
// timestamp file.
func GatherFirmwareBootTime(ctx context.Context, results *platform.GetBootPerfMetricsResponse) error {
	// Try getting firmware boot time from firmware log first.
	fw, err := getFirmwareLogBootTime(ctx)
	if err != nil {
		fw, err = getFirmwareTimestampBootTime(ctx)
	}
	if err != nil {
		return errors.New("failed to get firmware boot time from log or timestamp")
	}

	bootTime := results.Metrics["seconds_kernel_to_login"]
	results.Metrics["seconds_power_on_to_kernel"] = fw
	results.Metrics["seconds_power_on_to_login"] = fw + bootTime
	return nil
}

// getFirmwareTimestampBootTime reads firmware startup time from /tmp/firmware-boot-time.
func getFirmwareTimestampBootTime(ctx context.Context) (float64, error) {
	b, err := ioutil.ReadFile("/tmp/firmware-boot-time")
	for err != nil {
		return 0, err
	}
	l := strings.Split(string(b), "\n")[0]
	return strconv.ParseFloat(l, 64)
}

// getFirmwareLogBootTime gets firmware startup time by parsing the output of `cbmem -t`.
func getFirmwareLogBootTime(ctx context.Context) (float64, error) {
	stdout, err := readFirmwareTimestamps(ctx, false)
	if err != nil {
		return 0.0, err
	}

	// Parse firmware boot time from the output. `cbmem -t` reports firmware boot time with format of 'Total Time: {comma separated microseconds}'.
	// Example: Total Time: 4,819,016
	re := regexp.MustCompile(`Total Time:\s+([0-9,]+)`)
	stdoutStr := string(stdout)
	m := re.FindStringSubmatch(stdoutStr)
	if m == nil {
		return 0.0, errors.Errorf("failed to parse firmware boot time from cbmem output: %s", stdoutStr)
	}
	t := strings.ReplaceAll(m[1], ",", "") // Remove all commas.

	fwUsec, err := strconv.ParseUint(t, 10, 64)
	if err != nil {
		return 0.0, errors.Wrapf(err, "failed to parse firmware time %s", t)
	}
	fw := float64(fwUsec) / 1000000
	return fw, nil
}

// These are go-ified constants that match the ones with underscores and all caps in coreboot sources.
const (
	TsVbReadKernelDone    = 1050
	TsVbVbootDone         = 1100
	TsStartKernel         = 1101
	TsKernelDecompression = 1102
)

// GatherFirmwareStageTimings gets timings for various stages of firmware like kernel verification time.
func GatherFirmwareStageTimings(ctx context.Context, results *platform.GetBootPerfMetricsResponse) error {
	timings, err := getFirmwareLogDiffTimings(ctx)
	if err != nil {
		return err
	}

	// x86 doesn't decompress the kernel in the firmware, so we don't
	// report that case. Furthermore, the event is recorded when firmware
	// starts decompressing the kernel, and the next event is assumed to be
	// when the kernel is started. There's not much code between
	// decompressing and starting the kernel, so we simply record the diff
	// time between these two events and assume that is decompression time
	// to keep things simple. If this changes in the future, we'll have to
	// update this test. Hopefully, such an event will be named
	// "decompression done" so we can simply use the diff time of that
	// event.
	if timings[TsStartKernel] > 0 && timings[TsKernelDecompression] > 0 {
		results.Metrics["seconds_kernel_decompression_relocation"] = timings[TsStartKernel]
	}
	if timings[TsVbVbootDone] > 0 {
		results.Metrics["seconds_vboot_kernel_verification"] = timings[TsVbVbootDone]
	}
	if timings[TsVbReadKernelDone] > 0 {
		results.Metrics["seconds_vboot_read_kernel"] = timings[TsVbReadKernelDone]
	}
	return nil
}

// getFirmwareLogDiffTimings gets the diff timing from each stage of `cbmem -T` and
// returns it in a map of timing ID -> seconds.
func getFirmwareLogDiffTimings(ctx context.Context) (map[uint64]float64, error) {
	stdout, err := readFirmwareTimestamps(ctx, true)
	if err != nil {
		return nil, err
	}

	v := make(map[uint64]float64)
	// Parse timings from the output. `cbmem -T` reports how long various stages take with the format of 'ID Start_Time Diff_Time "Human readable ID description"'.
	// Example: 99   2745224 38592 selfboot jump
	re := regexp.MustCompile(`^([0-9]+)\s+([0-9]+)\s+([0-9]+)`)
	stdoutStr := string(stdout)
	for _, line := range strings.Split(strings.TrimSuffix(stdoutStr, "\n"), "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			return nil, errors.Wrapf(err, "failed to match regex to %q", line)
		}

		id, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse ID from %q", line)
		}
		usecs, err := strconv.ParseUint(m[3], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse time from %q", line)
		}

		v[id] = float64(usecs) / 1000000
	}

	return v, nil
}

// calculateTimeOffset calculates the time offset between 2 different clock
// sources (e.g. RTC and uptime), assumed to have no drift (just a fixed
// offset).
//
// The input values |t0| and |t1| are two values read from clock source A,
// |tx| is read from clock source B.
// The three "t" values were sampled in the order |t0|, |tx|, |t1|.
//
// The first return value (`offset`) is the offset between clock A and B, adding
// this offset to a measurement in clock source B will convert it to an
// estimated measurement in clock source A (i.e. `tx + offset = (t0 + t1) / 2`).
// Conversely, subtracting the offset from a measurement in clock source A will
// convert it to an estimated measurement in clock source B.
// The second return value estimates the worst-case error based on the time
// elapsed between `t0` and `t1`.
// All values are floats.
func calculateTimeOffset(t0, t1, tx float64) (float64, float64) {
	offset := (t0+t1)/2 - tx
	error := (t1 - t0) / 2
	return offset, error
}

// parseSyncRtc parses a sync-rtc-* file, which has this format:
// `uptime0 uptime1 RTCDate RTCTime`, where uptimeT* are uptime measurements
// before and after the RTC measurement.
// For example: `7.558581153 7.559699999 2021-02-09 11:42:10`
// Returns `uptime0`, `uptime1` as floats, RTC time as a Unix timestamp.
func parseSyncRtc(rtcPath string) (float64, float64, int64, error) {
	c, err := ioutil.ReadFile(rtcPath)
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to read timestamp")
	}
	// In rare cases the sync-rtc-* files contain multiple entries (likely because tlsdated was restarted), parse the most recent entry (the last line) of the file.
	lines := strings.Split(strings.TrimSpace(string(c)), "\n")
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	timesStr := strings.Split(lastLine, " ")
	uptime0, err := strconv.ParseFloat(timesStr[0], 64)
	if err != nil {
		return 0, 0, 0, errors.Wrapf(err, "error in parsing timestamp value %s", timesStr[0])
	}
	uptime1, err := strconv.ParseFloat(timesStr[1], 64)
	if err != nil {
		return 0, 0, 0, errors.Wrapf(err, "error in parsing timestamp value %s", timesStr[1])
	}
	const rtcTimeFormat = "2006-01-02 15:04:05"
	rtcTimeStr := timesStr[2] + " " + timesStr[3]
	rtcTime, err := time.Parse(rtcTimeFormat, rtcTimeStr)
	if err != nil {
		return 0, 0, 0, errors.Wrapf(err, "failed in parsing RTC time %s", rtcTimeStr)
	}
	return uptime0, uptime1, rtcTime.Unix(), nil
}

// canonicalizeBootID removes the newline and "-" from the boot ID string and make the output contain only hex characters.
func canonicalizeBootID(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "-", "")
}

// getCurrentBootID returns canonicalized boot ID of the current boot.
func getCurrentBootID() (string, error) {
	b, err := ioutil.ReadFile(currentBootIDPath)
	if err != nil {
		return "", err
	}
	return canonicalizeBootID(string(b)), nil
}

// getPreviousBootIDFromLog returns the boot ID of previous boot from /var/log/boot_id.log.
func getPreviousBootIDFromLog() (string, error) {
	b, err := ioutil.ReadFile("/var/log/boot_id.log")
	if err != nil {
		return "", errors.New("failed to read boot_id.log")
	}

	bootIDLogLines := strings.Split(strings.TrimSpace(string(b)), "\n")
	nlines := len(bootIDLogLines)
	if nlines < 2 {
		return "", errors.New("invalid boot_id.log. Expect at least two lines to get the previous boot ID")
	}

	// Sample boot_id.log: 2023-03-22T06:04:30.841000Z INFO boot_id: c1e96fd9fa5f4e46bb7fc56bc0a51b81
	re := regexp.MustCompile(`^.*boot_id:\s([0-9a-f]+)$`)
	// Correctness check: current boot ID should be the same in both boot_id.log and from proc filesystem.
	m := re.FindStringSubmatch(bootIDLogLines[nlines-1])
	if m == nil {
		return "", errors.Errorf("unable to parse boot_id from the last line of boot_id.log: %s", bootIDLogLines[nlines-1])
	}
	bootID, err := getCurrentBootID()
	if err != nil {
		return "", errors.Wrap(err, "failed to read the current boot ID")
	}
	if m[1] != bootID {
		return "", errors.Errorf("unexpected boot_id: want: %q, got: %q", bootID, m[1])
	}

	// The 2nd last line of boot_id.log contains the boot ID of previous boot. Match and return the ID part of the log entry.
	m = re.FindStringSubmatch(bootIDLogLines[nlines-2])
	if m == nil {
		return "", errors.Errorf("unable to parse boot_id from the last-1 line of boot_id.log: %s", bootIDLogLines[nlines-2])
	}
	return m[1], nil
}

// findMostRecentBootstatArchivePath returns the path of the bootstat archive
// generated from the most recent successful shutdown.
func findMostRecentBootstatArchivePath() (string, error) {
	bootstatArchives, _ := filepath.Glob(bootstatArchiveGlob) // filepath.Glob() only returns error on malformed glob patterns.
	if len(bootstatArchives) == 0 {
		return "", errors.New("failed to list bootstat archive directories")
	}

	previousBootID, err := getPreviousBootIDFromLog()
	if err != nil {
		return "", errors.Wrap(err, "failed to get previous boot ID from boot_id.log")
	}
	// Sort bootstatArchives to order the archive directories (almost) chronologically.
	// It's likely but not guaranteed that the directory with the largest timestamp will be the one of previous boot: time adjustments may rewind the clock.
	// We need to search for the directory with a matching boot_id.
	sort.Strings(bootstatArchives)
	for i := len(bootstatArchives) - 1; i >= 0; i-- {
		bootstatDir := bootstatArchives[i]

		bootIDPath := filepath.Join(bootstatDir, "boot_id")
		b, err := ioutil.ReadFile(bootIDPath)
		if err == nil && canonicalizeBootID(string(b)) == previousBootID {
			return bootstatDir, nil
		}
	}
	return "", errors.New("failed to find the bootstat archive for the latest shutdown")
}

// readFirmwareTimestamps reads firmware timestamp data from `cbmem -t/-T`.
func readFirmwareTimestamps(ctx context.Context, machine bool) ([]byte, error) {
	arg := "-t"
	if machine {
		arg = "-T"
	}

	stdout, err := testexec.CommandContext(ctx, "/usr/bin/cbmem", arg).Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute read firmware timestamps from `cbmem -t`")
	}
	return stdout, nil
}

// GatherRebootMetrics reads and reports shutdown and reboot times. The shutdown
// process saves all bootstat files in /var/log, plus it saves a timestamp file
// that can be used to convert "time since boot" into times in UTC.  Read the
// saved files from the most recent shutdown, and use them to calculate the time
// spent from the start of that shutdown until the completion of the most recent
// boot.
// This function records these metrics:
//   - seconds_shutdown_time
//   - seconds_reboot_time
//   - seconds_reboot_error
func GatherRebootMetrics(results *platform.GetRebootMetricsResponse) error {
	bootstatDir, err := findMostRecentBootstatArchivePath()
	if err != nil {
		return err
	}

	// Time values can come from 3 different sources. To reduce confusion, we suffix the variables as follows:
	//  - *Uptime: uptime in seconds (a.k.a. clock_gettime with CLOCK_BOOTTIME)
	//  - *SystemTime: seconds since epoch, system/wall clock time (a.k.a. clock_gettime with CLOCK_REALTIME, time.Now().Unix())
	//  - *RtcTime: seconds since epoch, but obtained from an RTC source

	shutdownUptime, err := parseUptime("ui-post-stop", bootstatDir, -1)
	if err != nil {
		return errors.Wrap(err, "failed in parsing uptime of event ui-post-stop")
	}

	// Compute reboot time using system time (quite inaccurate).
	// TODO(b:181084968): Remove this once we are convinced RTC code works better.
	timestampPath := filepath.Join(bootstatDir, "timestamp")
	b, err := ioutil.ReadFile(timestampPath)
	if err != nil {
		return errors.Wrap(err, "failed to read timestamp")
	}
	archiveSystemTimeStr := strings.Split(string(b), "\n")
	archiveUptime, err := parseUptime("archive", bootstatDir, 0)
	if err != nil {
		return errors.Wrap(err, "failed in parsing uptime of event archive")
	}
	archiveSystemTime0, err := strconv.ParseFloat(archiveSystemTimeStr[0], 64)
	if err != nil {
		return errors.Wrapf(err, "error in parsing timestamp value %s", archiveSystemTimeStr[0])
	}
	archiveSystemTime1, err := strconv.ParseFloat(archiveSystemTimeStr[1], 64)
	if err != nil {
		return errors.Wrapf(err, "error in parsing timestamp value %s", archiveSystemTimeStr[1])
	}

	archiveSystemTimeOffset, archiveError := calculateTimeOffset(archiveSystemTime0, archiveSystemTime1, archiveUptime)
	shutdownSystemTime := shutdownUptime + archiveSystemTimeOffset

	nowSystemTime0 := time.Now().Unix()
	nowUptimeStr, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		return errors.Wrap(err, "failed to read system uptime")
	}
	nowSystemTime1 := time.Now().Unix()
	nowUptime, err := strconv.ParseFloat(strings.Fields(string(nowUptimeStr))[0], 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse system uptime %s", string(nowUptimeStr))
	}
	nowSystemTimeOffset, nowError := calculateTimeOffset(float64(nowSystemTime0), float64(nowSystemTime1), nowUptime)
	bootSystemTime := results.Metrics["seconds_kernel_to_login"] + nowSystemTimeOffset

	rebootTime := bootSystemTime - shutdownSystemTime
	poweronTime := results.Metrics["seconds_power_on_to_login"]
	shutdownTime := rebootTime - poweronTime

	results.Metrics["seconds_reboot_time"] = rebootTime
	results.Metrics["seconds_reboot_error"] = archiveError + nowError
	// TODO(b:181084548): This is somewhat inaccurate, as this excludes power sequencing.
	results.Metrics["seconds_shutdown_time"] = shutdownTime

	// Compute reboot time using RTC (much better accuracy)
	rtcStopPath := filepath.Join(bootstatDir, "sync-rtc-tlsdated-stop")
	stopUptime0, stopUptime1, stopRtcTime, err := parseSyncRtc(rtcStopPath)
	if err != nil {
		return errors.Wrap(err, "failed to parse RTC sync stop time")
	}
	rtcStartPath := filepath.Join(bootstatCurrentDir, "sync-rtc-tlsdated-start")
	startUptime0, startUptime1, startRtcTime, err := parseSyncRtc(rtcStartPath)
	if err != nil {
		return errors.Wrap(err, "failed to parse RTC sync start time")
	}

	stopRtcTimeOffset, stopError := calculateTimeOffset(stopUptime0, stopUptime1, float64(stopRtcTime))
	shutdownTimeRtc := shutdownUptime - stopRtcTimeOffset
	startRtcTimeOffset, startError := calculateTimeOffset(startUptime0, startUptime1, float64(startRtcTime))
	bootTimeRtc := results.Metrics["seconds_kernel_to_login"] - startRtcTimeOffset

	rebootTimeRtc := bootTimeRtc - shutdownTimeRtc

	// TODO(b:181084548): Drop the "_rtc" suffix once we remove the system time metrics above.
	results.Metrics["seconds_reboot_time_rtc"] = rebootTimeRtc
	results.Metrics["seconds_reboot_error_rtc"] = stopError + startError

	return nil
}

// CalculateDiff generates metrics from existing ones. Metrics of time and
// rdbytes are calculated from kernel startup. For example, metric
// "seconds_startup_to_chrome_exec" that represents time from the "startup"
// stage to Chrome execution begins, is calculated from subtracting
// "seconds_kernel_to_chrome_exec" to "seconds_kernel_to_startup".
func CalculateDiff(results *platform.GetBootPerfMetricsResponse) {
	barriers := []string{"startup", "chrome_exec", "login"}
	types := []string{"seconds", "rdbytes"}
	for i, b := range barriers[:len(barriers)-1] {
		for _, t := range types {
			begin := t + "_kernel_to_" + b
			end := t + "_kernel_to_" + barriers[i+1]

			rb, ok1 := results.Metrics[begin]
			if re, ok2 := results.Metrics[end]; ok1 && ok2 {
				diffName := t + "_" + b + "_to_" + barriers[i+1]
				results.Metrics[diffName] = re - rb
			}
		}
	}
}

// GatherRebootRawDataFiles gathers content of raw data files used to calculate
// reboot timing metrics.
func GatherRebootRawDataFiles(raw map[string][]byte) error {
	// Collect sync-rtc-tlsdated-start of the current boot and  sync-rtc-tlsdated-stop of previous boot.
	files := []string{filepath.Join(bootstatCurrentDir, "sync-rtc-tlsdated-start")}
	lastBootstatArchive, err := findMostRecentBootstatArchivePath()
	if err != nil {
		return err
	}
	files = append(files, filepath.Join(lastBootstatArchive, "sync-rtc-tlsdated-stop"))

	for _, f := range files {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			return errors.Wrapf(err, "failed to read from %s", f)
		}
		raw[filepath.Base(f)] = b
	}

	return nil
}

// GatherMetricRawDataFiles gathers content of raw data files to be returned to
// the client.
func GatherMetricRawDataFiles(raw map[string][]byte) error {
	files := []string{currentBootIDPath}
	for _, glob := range []string{uptimeFileGlob, diskFileGlob} {
		list, _ := filepath.Glob(glob) // filepath.Glob() only returns error on malformed glob patterns.
		files = append(files, list...)
	}

	for _, f := range files {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			return errors.Wrapf(err, "failed to read from %s", f)
		}
		raw[filepath.Base(f)] = b
	}

	return nil
}

// GatherConsoleRamoops gathers console_ramoops from previous reboot.
func GatherConsoleRamoops(raw map[string][]byte) error {
	list, _ := filepath.Glob(ramOopsFileGlob) // filepath.Glob() only returns error on malformed glob patterns.
	for _, f := range list {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			return errors.Wrapf(err, "failed to read from %s", f)
		}
		raw[filepath.Base(f)] = b
	}

	return nil
}

// StoreFirmwareTimestamps stores the raw firmware timestamps from `cbmem -t`.
func StoreFirmwareTimestamps(ctx context.Context, raw map[string][]byte) {
	stdout, err := readFirmwareTimestamps(ctx, false)
	if err != nil {
		// Don't err on `cbmem -t` failure. It doesn't work on every device.
		return
	}
	raw["cbmem-timestamps"] = stdout
}
