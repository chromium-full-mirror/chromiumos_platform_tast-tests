// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sched contains scheduler-related ChromeOS tests
package sched

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/chromeproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CoreSchedTag,
		Desc: "Ensures renderers scheduling cookies are assigned correctly",
		Contacts: []string{
			"baseos-perf@google.com",
			"joelaf@google.com",
		},
		BugComponent: "b:167279",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "arc", "coresched"},
		HardwareDeps: hwdep.D(hwdep.CPUSupportsSMT(), hwdep.CPUNeedsCoreScheduling()),
		Timeout:      3 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func getProcCookie(p *process.Process) (int64, error) {
	path := filepath.Join("/proc", fmt.Sprint(p.Pid), "sched")

	re, err := regexp.Compile(`core_cookie\s*:\s*(.+)`)
	if err != nil {
		return 0, errors.Wrap(err, "failed to compile cookie regex")
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}

	res := re.FindAllSubmatch(data, -1)
	if len(res) == 0 {
		return 0, errors.New("failed to find core_cookie in sched file")
	}

	f, err := strconv.ParseInt(string(res[0][1]), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert cookie to int")
	}
	return f, nil
}

func getThreadsFromProcess(p *process.Process) ([]*process.Process, error) {
	path := filepath.Join("/proc", fmt.Sprint(p.Pid), "task")
	var ret []*process.Process

	finfos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, finfo := range finfos {
		fname := finfo.Name()

		pid, err := strconv.ParseInt(fname, 10, 32)
		if err != nil {
			// if not numeric name, just skip
			continue
		}

		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			continue
		}

		ret = append(ret, proc)
	}

	return ret, nil
}

// verifyTags verifies the tags of all renderer and ARC processes (TODO: add ARC).
// Make sure that the ones in containsPids are scanned (This is to ensure that
// chrome is among the processes scanned.)
func verifyTags(ctx context.Context, tconn *chrome.TestConn) error {
	cookieMap := make(map[int64]bool)

	procs, err := chromeproc.GetRendererProcesses()
	if err != nil {
		return errors.Wrap(err, "failed to get renderer processes")
	}

	for _, proc := range procs {
		parentCookie, _ := getProcCookie(proc)
		if parentCookie == 0 {
			return errors.Errorf("main thread of %d not tagged", proc.Pid)
		}

		if _, ok := cookieMap[parentCookie]; ok {
			return errors.New("duplicate renderer cookie or collision")
		}
		cookieMap[parentCookie] = true

		threads, err := getThreadsFromProcess(proc)
		if err != nil {
			return errors.Wrap(err, "failed to get threads in process")
		}

		for _, thread := range threads {
			cookie, err := getProcCookie(thread)
			if err != nil {
				return errors.Wrap(err, "failed to get cookie")
			}

			if cookie == 0 {
				return errors.New("renderer thread not tagged")
			}

			if cookie != parentCookie {
				return errors.New("main thread and renderer thread mismatch")
			}
		}
	}

	return nil
}

// CoreSchedTag : Function to test core scheduling cookies on ChromeOS
func CoreSchedTag(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(ctx)
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	settingsConn, err := br.NewConn(ctx, "chrome://settings")
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer settingsConn.Close()

	if err := webutil.WaitForQuiescence(ctx, settingsConn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for chrome://settings to achieve quiescence: ", err)
	}

	versionConn, err := br.NewConn(ctx, "chrome://version")
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer versionConn.Close()

	if err := webutil.WaitForQuiescence(ctx, versionConn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for chrome://settings to achieve quiescence: ", err)
	}

	if err := verifyTags(ctx, tconn); err != nil {
		s.Fatal("Failed to verify tags: ", err)
	}
}
