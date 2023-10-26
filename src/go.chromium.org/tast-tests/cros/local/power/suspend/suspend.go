// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package suspend

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Param represents a parameter for a suspend request.
type Param interface {
	Name() string
	Value() (string, error)
}

type namedParam struct {
	name string
}

func (p *namedParam) Name() string {
	return p.name
}

type intParam struct {
	namedParam
	value int
}

func (p *intParam) Value() (string, error) {
	return strconv.Itoa(p.value), nil
}

// Delay is the time duration to wait before sending the dbus request, rounded
// down to the nearest second.
func Delay(t time.Duration) Param {
	return &intParam{namedParam{"delay"}, int(t.Seconds())}
}

// Timeout is a time duration, rounded down to the nearest second. If the DUT
// does not resume before this duration, then, when the DUT does resume,
// powerd_dbus_suspend will LOG(FATAL). The LOG(FATAL) is interpreted as an
// error by this library. Note that time spent suspended does not count for
// this timeout.
func Timeout(t time.Duration) Param {
	return &intParam{namedParam{"timeout"}, int(t.Seconds())}
}

// For is the time duration that powerd will attempt to suspend for, rounded
// down to the nearest second. If not provided (or zero), an indefinite
// suspension is requested, and the DUT will only resume when an interrupt from
// some wake source is received.
func For(t time.Duration) Param {
	return &intParam{namedParam{"suspend_for_sec"}, int(t.Seconds())}
}

// Default timeout for Timeout(...) Param. 30 seconds gives us enough time for
// suspend operations plus the freeze of userspace (from userspace) to timeout.
// kDefaultMaxSuspendDelayTimeout (for suspend delays from other processes) is
// 20 seconds and the freeze timeout, kFreezerTimeout, is 10 seconds.
const defaultTimeout = 30 * time.Second

// ResumeInfo holds information about the suspend+resume that just occurred.
type ResumeInfo struct {
	// TODO: add wake source and suspend timings
}

// For tests, the wakeup count param has one useful value - the count
// immediately prior to requesting suspend
type wakeupCountParam struct{}

func (p *wakeupCountParam) Name() string { return "wakeup_count" }
func (p *wakeupCountParam) Value() (string, error) {
	bytes, err := ioutil.ReadFile("/sys/power/wakeup_count")
	if err != nil {
		return "", err
	}
	return strings.Trim(string(bytes), "\n"), nil
}

// WithoutRetries specifies that suspend should not retry in case the suspend
// is cancelled prior to being requested.
func WithoutRetries() Param {
	return &wakeupCountParam{}
}

const (
	powerdLatestPath = "/var/log/power_manager/powerd.LATEST"
	flashromLockFile = "/run/lock/power_override/flashrom.lock"
)

func checkPowerdRet(ctx context.Context, reader *syslog.LineReader, lineCache *[]string) (int, error) {
	r := regexp.MustCompile(`powerd_suspend returned ([0-9])`)
	var strErr error
	ret := 0
	i := 0
	err := testing.Poll(ctx, func(ctx context.Context) error {
		line, err := reader.ReadLine()
		for err == nil {
			*lineCache = append(*lineCache, line)
			line, err = reader.ReadLine()
		}

		if err != nil && err != io.EOF {
			testing.ContextLog(ctx, "Encountered non-EOF error when reading powerd log: ", err)
		}

		// Only iterate over lines in the cache that weren't there in the prior
		// polling iteration by taking a sub-slice.
		for i, line = range (*lineCache)[i:] {
			if m := r.FindStringSubmatch(line); m != nil {
				if ret, strErr = strconv.Atoi(m[1]); strErr == nil {
					testing.ContextLogf(ctx, "powerd_suspend returned %d", ret)
					return nil
				}
			}
			// Increment i to not check the last line in the next polling
			// iteration.
			i++
		}
		return errors.New("timed out waiting for return value of powerd_suspend")
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})

	return ret, err
}

func checkKernelRet() (int, error) {
	// TODO just read /sys/power/suspend_stats/last_failed_errno after it exists
	// for all platforms (when all kernels are 5.4 or newer).
	out, err := ioutil.ReadFile("/sys/kernel/debug/suspend_stats")
	if err != nil {
		return 0, err
	}

	r := regexp.MustCompile(`last_failed_errno:\s*(-?[0-9]+)`)
	if m := r.FindStringSubmatch(string(out)); m == nil {
		return 0, errors.New("could not find last_failed_errno in suspend_stats")
	} else if ret, err := strconv.Atoi(m[1]); err == nil {
		return ret, nil
	} else {
		return 0, err
	}
}

const ecSuspendResultPath = "/sys/kernel/debug/cros_ec/last_resume_result"

func checkECRet() (uint32, error) {
	if _, err := os.Stat(ecSuspendResultPath); err != nil {
		// EC suspend result doesn't exist on this platform.
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrapf(err, "failed to stat EC suspend result path %s", ecSuspendResultPath)
	}
	out, err := ioutil.ReadFile(ecSuspendResultPath)
	if err != nil {
		return 0, err
	}

	// Convert the string to the format that ParseUint expects.
	strOut := strings.Replace(string(out), "0x", "", -1)
	strOut = strings.Replace(strOut, "0X", "", -1)
	strOut = strings.TrimSpace(strOut)

	// The last_resume_result file contains a 32-bit hex int. The most
	// significant bit shows if the EC timed out waiting for the SoC to enter
	// S0ix. This is the only bit we care about, so mask out the rest.
	ret, err := strconv.ParseUint(strOut, 16, 32)
	if err != nil {
		return 0, err
	}

	return uint32(ret & (1 << 31)), nil
}

func waitForFlashromLock(ctx context.Context) error {
	if err := testing.Poll(ctx, func(context.Context) error {
		_, readErr := os.Stat(flashromLockFile)
		if readErr == nil {
			return errors.New("flashrom lock still exists")
		}
		if !os.IsNotExist(readErr) {
			return errors.New("could not read file info through os")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  120 * time.Second,
		Interval: time.Second,
	}); err != nil {
		return err
	}
	return nil
}

// Return values for the powerd_suspend script
const (
	powerdResultSuccess       = 0
	powerdResultFailure       = 1
	powerdResultCanceledEarly = 2
	powerdResultCanceledLate  = 3
)

// Request requests a suspension using powerd_dbus_suspend.
// See https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/power_manager/tools/powerd_dbus_suspend.cc
func Request(ctx context.Context, params ...Param) (ResumeInfo, error) {
	// Convert request params into args for powerd_dbus_suspend.
	args := make([]string, 0)
	for _, p := range params {
		val, err := p.Value()
		if err != nil {
			return ResumeInfo{}, err
		}
		args = append(args, fmt.Sprintf("--%s=%s", p.Name(), val))
	}
	if err := waitForFlashromLock(ctx); err != nil {
		testing.ContextLog(ctx, "Still attempt suspend even though fail to wait for flashrom lock clearance: ", err)
	}

	// Open the powerd log and seek to the end. The logs on suspend should soon
	// follow after that point.
	powerdReader, err := syslog.NewLineReader(ctx, powerdLatestPath, false, nil)
	if err != nil {
		return ResumeInfo{}, err
	}
	defer powerdReader.Close()

	testing.ContextLogf(ctx, "Running powerd_dbus_suspend %s", strings.Join(args, " "))
	cmd := testexec.CommandContext(ctx, "powerd_dbus_suspend", args...)
	err = cmd.Run(testexec.DumpLogOnError)
	// Just checking the return value of powerd_dbus_suspend is not enough, so
	// we need to continue after this to see what powerd and the kernel logged.
	if err == nil {
		testing.ContextLog(ctx, "Suspend request succeeded")
	} else {
		testing.ContextLog(ctx, "Suspend request failed with: ", err)
	}

	// TODO: Use line cache of powerd log for checking the wake source and
	// timings.
	powerdLineCache := make([]string, 0)
	powerdRet, err := checkPowerdRet(ctx, powerdReader, &powerdLineCache)
	if err != nil {
		return ResumeInfo{}, err
	}
	switch powerdRet {
	case powerdResultSuccess:
		// Even if powerd reports a success, it might later fail due to the EC
		// reporting a suspend to idle timeout.
		ecRet, err := checkECRet()
		if err != nil {
			return ResumeInfo{}, errors.Wrap(err, "failed reading EC suspend result")
		} else if ecRet != 0 {
			return ResumeInfo{}, errors.Errorf("EC reported suspend to idle timeout: 0x%x", ecRet)
		}
		return ResumeInfo{}, nil
	case powerdResultFailure:
		kernelRet, err := checkKernelRet()
		if err != nil {
			return ResumeInfo{}, errors.Wrap(err, "reading kernel errno failed on suspend failure")
		}

		return ResumeInfo{}, errors.Errorf("suspend failed with kernel errno: %d", kernelRet)
	case powerdResultCanceledEarly:
		return ResumeInfo{}, errors.New("suspend canceled before entering the kernel")
	case powerdResultCanceledLate:
		return ResumeInfo{}, errors.New("suspend canceled after entering the kernel")
	default:
		return ResumeInfo{}, errors.Errorf("unknown return value from powerd_suspend script: %d", powerdRet)
	}
}

// ForDuration requests a suspension for a given number of seconds. Also sets
// the failure timeout with a leeway of 10 seconds over the requested suspension
// time.
func ForDuration(ctx context.Context, t time.Duration) (ResumeInfo, error) {
	return Request(ctx, For(t), Timeout(defaultTimeout))
}
