// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sched

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/chromeproc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const simpleHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Simple Page</title>
</head>
<body>
    <h1>Simple Page</h1>
</body>
</html>
`

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeQoS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome processes works with resourced schedqos",
		Contacts:     []string{"cros-core-systems-perf@google.com", "kawasin@google.com"},
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromeLoggedInWithSchedQoS,
		Timeout:      30 * time.Second,
	})
}

func getCPUCgroup(pid int32) (string, error) {
	cgroupPath := filepath.Join("/proc", strconv.Itoa(int(pid)), "cgroup")
	cgroups, err := os.ReadFile(cgroupPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(cgroups), "\n") {
		line := strings.Split(line, ":")
		if line[1] == "cpu" {
			return line[2], nil
		}
	}
	return "", errors.New("cpu cgroup not found")
}

func getCPUSetCgroup(pid, tid int32) (string, error) {
	cgroupPath := filepath.Join("/proc", strconv.Itoa(int(pid)), "task", strconv.Itoa(int(tid)), "cgroup")
	cgroups, err := os.ReadFile(cgroupPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(cgroups), "\n") {
		line := strings.Split(line, ":")
		if line[1] == "cpuset" {
			return line[2], nil
		}
	}
	return "", errors.New("cpuset cgroup not found")
}

func checkCPUCgroup(pid int32) error {
	cgroup, err := getCPUCgroup(pid)
	if err != nil {
		return errors.Wrapf(err, "failed to get cpu cgroup for process %d", pid)
	}
	if cgroup == "/resourced/normal" || cgroup == "/resourced/background" {
		return nil
	}
	return errors.Errorf("unexpected cpu cgroup %s for process %d", cgroup, pid)
}

func checkCPUSetCgroup(pid, tid int32) error {
	cgroup, err := getCPUSetCgroup(pid, tid)
	if err != nil {
		return errors.Wrapf(err, "failed to get cpuset cgroup for process %d", pid)
	}
	if cgroup == "/resourced/all" || cgroup == "/resourced/efficient" {
		return nil
	}
	return errors.Errorf("unexpected cpuset cgroup %s for process %d, thread %d", cgroup, pid, tid)
}

func isQoSEligibleProcess(ctx context.Context, s *testing.State, p *process.Process) bool {
	cmdline, err := p.CmdlineWithContext(ctx)
	if err != nil {
		s.Fatal("Failed to get process name: ", err)
	}
	// Zygote and broker processes are out of schedqos control.
	return !strings.Contains(cmdline, "--type=zygote") && !strings.Contains(cmdline, "--type=broker")
}

func getBackgroundProcessIds(ctx context.Context, s *testing.State, processes []*process.Process) map[int32]struct{} {
	pids := make(map[int32]struct{})
	for _, p := range processes {
		if cgroup, err := getCPUCgroup(p.Pid); err != nil {
			s.Fatalf("Failed to get cpu cgroup for process %d: %v", p.Pid, err)
		} else if cgroup == "/resourced/background" {
			pids[p.Pid] = struct{}{}
		}
	}
	return pids
}

func ChromeQoS(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	br := cr.Browser()

	// Cgroups of chrome processes are updated via D-Bus messages. The updates can be delayed if the
	// system is busy. Poll the cgroups of Chrome processes here until they are stabilized.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		processes, err := ashproc.Processes()
		if err != nil {
			return errors.Wrap(err, "failed to get Chrome processes")
		} else if len(processes) == 0 {
			return errors.New("no Chrome processes found")
		}

		// Check cpu cgroups of all Chrome processes are managed by resourced.
		for _, process := range processes {
			if isQoSEligibleProcess(ctx, s, process) {
				if err := checkCPUCgroup(process.Pid); err != nil {
					return errors.Wrapf(err, "failed to check cpu cgroup for process %d", process.Pid)
				}
				if err := checkCPUSetCgroup(process.Pid, process.Pid); err != nil {
					return errors.Wrapf(err, "failed to check cpuset cgroup for process %d", process.Pid)
				}
			}
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to verify Chrome processes: ", err)
	}

	// Use data url to open simple pages. Each page is counted as a different origin and launches
	// different renderer processes.
	testURL := utils.CompileHTMLDataURL([]byte(simpleHTML))

	conn, err := br.NewTab(ctx, testURL)
	if err != nil {
		s.Fatal("Failed to open simple page: ", err)
	}
	defer conn.Close()

	rendererProcesses, err := chromeproc.GetRendererProcesses()
	if err != nil {
		s.Fatal("Failed to get renderer processes: ", err)
	}
	pidsBefore := getBackgroundProcessIds(ctx, s, rendererProcesses)

	// By opening a new tab, the existing page goes to background.
	conn2, err := br.NewTab(ctx, testURL)
	if err != nil {
		s.Fatal("Failed to open new tab: ", err)
	}
	defer conn2.Close()

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		rendererProcesses, err = chromeproc.GetRendererProcesses()
		if err != nil {
			s.Fatal("Failed to get renderer processes: ", err)
		}
		pidsAfter := getBackgroundProcessIds(ctx, s, rendererProcesses)

		for pid := range pidsAfter {
			if _, ok := pidsBefore[pid]; !ok {
				s.Logf("New background renderer process %d is found", pid)
				return nil
			}
		}

		return errors.Errorf("There is no new background renderer processes: before %v, after: %v", pidsBefore, pidsAfter)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to verify background renderer processes: ", err)
	}
}
