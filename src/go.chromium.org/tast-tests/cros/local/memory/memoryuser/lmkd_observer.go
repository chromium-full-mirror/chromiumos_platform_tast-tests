// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var lmkdKillRE = regexp.MustCompile(`^[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3} +[0-9]+ +[0-9]+ I lowmemorykiller: Kill '(?P<package>[^']+)' \((?P<pid>[0-9]+)\), uid (?P<uid>[0-9]+), oom_(?:score_)?adj (?P<oomscore>[0-9]+) `)

// LmkdKillInfo tracks information for one LMKD app kill.
type LmkdKillInfo struct {
	Time        time.Time
	Package     string
	PID         int
	UID         int
	OomScoreAdj int
}

func parseLmkdKillInfo(logcatLine string) *LmkdKillInfo {
	t, err := adb.ParseLogcatTimestamp(logcatLine)
	if err != nil {
		return nil
	}

	m := lmkdKillRE.FindStringSubmatch(logcatLine)
	if m == nil {
		return nil
	}

	pid, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}

	uid, err := strconv.Atoi(m[3])
	if err != nil {
		return nil
	}

	oomScoreAdj, err := strconv.Atoi(m[4])
	if err != nil {
		return nil
	}

	return &LmkdKillInfo{
		Time:        t,
		Package:     m[1],
		PID:         pid,
		UID:         uid,
		OomScoreAdj: oomScoreAdj,
	}
}

// VmmmsPriority provides the VmmmsPriority for an LMKD kill.
func (i LmkdKillInfo) VmmmsPriority() VmmmsPriority {
	if i.OomScoreAdj > 1000 {
		return -1
	} else if i.OomScoreAdj >= 900 { // ProcessList.CACHED_APP_MIN_ADJ
		return VmmmsCachedAppPriority
	} else if i.OomScoreAdj > 0 { // ProcessList.FOREGROUND_APP_ADJ
		return VmmmsPerceptibleAppPriority
	} else if i.OomScoreAdj == 0 {
		return VmmmsFocusedAppPriority
	} else {
		return -1
	}
}

// FirstLmkdKillOfPriority returns the first LmkdKillInfo of a given priority or
// nil if none exist.
func FirstLmkdKillOfPriority(log []*LmkdKillInfo, priority VmmmsPriority) *LmkdKillInfo {
	for _, info := range log {
		if info.VmmmsPriority() == priority {
			return info
		}
	}
	return nil
}

// LmkdObserver asynchronously observes Android Low Memory Killer kills in
// logcat. Logcat is monitored in the background to avoid missing lines that are
// pushed out of it's curcular buffer. Since high memory pressure can cause
// transient failures, we repeatedly dump logcat until the observer is closed.
type LmkdObserver struct {
	cancel          func()
	errChan         chan error
	resChan         chan []*LmkdKillInfo
	doneString      string
	didObserverExit bool
}

const logcatTimeParamLayout = "2006-01-02 15:04:05.000"

// lmkdObserverLogcatTimeout is the timeout used for a single call to logcat.
const lmkdObserverLogcatTimeout = 15 * time.Second

func logcatWithTimeout(ctx context.Context, a *arc.ARC, linesSince time.Time, timeout time.Duration) ([]string, error) {
	// Logcat can sometimes hang even after the passed context has closed.
	logcatCtx, logcatCtxCancel := context.WithTimeout(ctx, timeout)
	defer logcatCtxCancel()

	logcatErrChan := make(chan error, 1)
	logcatResChan := make(chan []byte, 1)
	go func() {
		stdout, err := a.Command(logcatCtx, "logcat", "-t", linesSince.Format(logcatTimeParamLayout)).Output()
		if err != nil {
			logcatErrChan <- err
		} else {
			logcatResChan <- stdout
		}
	}()

	select {
	case <-logcatCtx.Done():
		return nil, errors.New("timed out waiting for logcat")
	case err := <-logcatErrChan:
		return nil, err
	case res := <-logcatResChan:
		return strings.Split(string(res), "\n"), nil
	}
}

// NewLmkdObserver creates a LmkdObserver for observing Android app kills during
// high memory pressure.
func NewLmkdObserver(ctx context.Context, a *arc.ARC) *LmkdObserver {
	// Observation is done asynchronously with a different context.
	observeCtx, cancel := context.WithCancel(ctx)
	// The asynchronous result is returned via these channels.
	errChan := make(chan error, 1)
	resChan := make(chan []*LmkdKillInfo, 1)
	// To synchronize Close, we write a unique string to logcat which triggers the
	// async observe function to exit.
	doneString := fmt.Sprintf("LmkdObserver::Done::[%16x]", rand.Uint64())

	o := &LmkdObserver{cancel, errChan, resChan, doneString, false}
	go o.observe(observeCtx, a)
	return o
}

func (o *LmkdObserver) observe(ctx context.Context, a *arc.ARC) {
	defer func() {
		o.didObserverExit = true
	}()
	observedKills := make(map[string]bool)
	observedTime := time.Now()
	var kills []*LmkdKillInfo

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Logcat can take many minutes only to eventually fail anyways when
		// called under high memory pressure, even if the memory pressure is later
		// removed. Long timeouts here can cause LmkdObserver.Close to fail;
		// doneString will not be observed before the timeout in Close. So we use
		// a shorter timeout to give us a few retries when synchronizing Close.
		lines, err := logcatWithTimeout(ctx, a, observedTime, lmkdObserverLogcatTimeout)
		if err != nil {
			testing.ContextLog(ctx, "LmkdObserver logcat failed: ", err)
			return err
		}

		for _, line := range lines {
			if strings.Contains(line, o.doneString) {
				// Exit testing.Poll, we are done.
				return nil
			}

			t, err := adb.ParseLogcatTimestamp(line)
			if err != nil {
				// Some lines, like "--------- beginning of main" have no timestamp, so ignore failures.
				continue
			}
			observedTime = t

			if _, ok := observedKills[line]; ok {
				// Kill already observed, due to sharing an observedTime of the last line of the previous call to logcat.
				continue
			}
			observedKills[line] = true

			kill := parseLmkdKillInfo(line)
			if kill != nil {
				kills = append(kills, kill)
			}
		}

		return errors.New("still observing logcat")
	}, nil); err != nil {
		o.errChan <- err
	} else {
		o.resChan <- kills
	}
}

// Close stops a LmkdObserver and returns the list of kills observed.
func (o *LmkdObserver) Close(ctx context.Context, a *arc.ARC) ([]*LmkdKillInfo, error) {
	defer o.cancel()
	if o.didObserverExit {
		return nil, errors.New("LmkdObserver logcat goroutine exited before Close")
	}

	if err := a.Command(ctx, "log", o.doneString).Run(); err != nil {
		return nil, errors.Wrapf(err, "failed to log %q to notify LmkdObserver to finish", o.doneString)
	}

	// The observer should have enough time to recover from logcat timing out, so
	// use a longer timeout than the logcat calls.
	closeCtx, cancel := context.WithTimeout(ctx, 5*lmkdObserverLogcatTimeout)
	defer cancel()
	select {
	case <-closeCtx.Done():
		return nil, errors.Errorf("timed out waiting for LmkdObserver to observe %q, didObserverExit: %t", o.doneString, o.didObserverExit)
	case err := <-o.errChan:
		return nil, err
	case res := <-o.resChan:
		return res, nil
	}
}
