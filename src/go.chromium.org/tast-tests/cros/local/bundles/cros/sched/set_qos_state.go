// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sched

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast-tests/cros/local/sched"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

const (
	normalCPUCgroupProcessesFile     = "/sys/fs/cgroup/cpu/resourced/normal/cgroup.procs"
	backgroundCPUCgroupProcessesFile = "/sys/fs/cgroup/cpu/resourced/background/cgroup.procs"
	allCPUSetCgroupThreadsFile       = "/sys/fs/cgroup/cpuset/resourced/all/tasks"
	efficientCPUSetCgroupThreadsFile = "/sys/fs/cgroup/cpuset/resourced/efficient/tasks"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetQoSState,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that schedqos in resourced works",
		Contacts:     []string{"cros-core-systems-perf@google.com", "kawasin@google.com"},
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      30 * time.Second,
	})
}

func containsID(path string, id uint32) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	idStr := strconv.Itoa(int(id))
	for _, s := range strings.Split(string(content), "\n") {
		if s == idStr {
			return true, nil
		}
	}
	return false, nil
}

func SetQoSState(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	pRoot, err := sched.CreateSampleProcessThreadPair(ctx, nil)
	if err != nil {
		s.Fatal("Failed to create process: ", err)
	}
	defer pRoot.KillAndWait()
	attr, err := sched.SysProcAttrOfUser("chronos")
	if err != nil {
		s.Fatal("Failed to get user: ", err)
	}
	pChronos, err := sched.CreateSampleProcessThreadPair(ctx, attr)
	if err != nil {
		s.Fatal("Failed to create process: ", err)
	}
	defer pChronos.KillAndWait()
	if b, err := containsID(normalCPUCgroupProcessesFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if process is in normal cpu cgroup: ", err)
	} else if b {
		s.Fatal("Process should not be in normal cpu cgroup")
	}

	originalEUID := unix.Geteuid()
	chronosEUID := int(attr.Credential.Uid)
	// Set chronos as the euid of tast test process because SetProcessState/SetThreadState validates
	// the ruid of the process/thread with the euid of the caller process.
	if err := syscall.Seteuid(chronosEUID); err != nil {
		s.Fatal("Failed to set euid to chronos: ", err)
	}
	defer syscall.Seteuid(originalEUID)

	// Setting qos state to processes of other users is rejected.
	if err := rm.SetProcessState(ctx, pRoot.Pid, resourced.QoSProcessNormal); err == nil {
		s.Fatal("SetProcessState for a process of a different user must fail")
	}

	// Set process state
	if err := rm.SetProcessState(ctx, pChronos.Pid, resourced.QoSProcessNormal); err != nil {
		s.Fatal("Failed to set process state: ", err)
	}

	if b, err := containsID(normalCPUCgroupProcessesFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if process is in normal cpu cgroup: ", err)
	} else if !b {
		s.Fatal("Process should be in normal cpu cgroup")
	}

	if b, err := containsID(allCPUSetCgroupThreadsFile, pChronos.Tid); err != nil {
		s.Fatal("Failed to check if thread is in all cpuset cgroup: ", err)
	} else if b {
		s.Fatal("Thread should not be registered to cgroup yet")
	}
	if b, err := containsID(efficientCPUSetCgroupThreadsFile, pChronos.Tid); err != nil {
		s.Fatal("Failed to check if thread is in efficient cpuset cgroup: ", err)
	} else if b {
		s.Fatal("Thread should not be registered to cgroup yet")
	}

	// Set thread state
	if err := rm.SetThreadState(ctx, pChronos.Pid, pChronos.Tid, resourced.QoSThreadUrgentBursty); err != nil {
		s.Fatal("Failed to set thread state: ", err)
	}
	// Set thread state for the main thread
	if err := rm.SetThreadState(ctx, pChronos.Pid, pChronos.Pid, resourced.QoSThreadBackground); err != nil {
		s.Fatal("Failed to set thread state: ", err)
	}
	if b, err := containsID(allCPUSetCgroupThreadsFile, pChronos.Tid); err != nil {
		s.Fatal("Failed to check if thread is in all cpuset cgroup: ", err)
	} else if !b {
		s.Fatal("Thread should be in all cpuset cgroup")
	}
	if b, err := containsID(efficientCPUSetCgroupThreadsFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if thread is in efficient cpuset cgroup: ", err)
	} else if !b {
		s.Fatal("Thread should be in efficient cpuset cgroup")
	}

	// Process become background
	if err := rm.SetProcessState(ctx, pChronos.Pid, resourced.QoSProcessBackground); err != nil {
		s.Fatal("Failed to set process state: ", err)
	}
	if b, err := containsID(backgroundCPUCgroupProcessesFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if process is in background cpu cgroup: ", err)
	} else if !b {
		s.Fatal("Process should be in background cpu cgroup")
	}
	// Threads are also moved to efficient cpuset cgroup.
	if b, err := containsID(efficientCPUSetCgroupThreadsFile, pChronos.Tid); err != nil {
		s.Fatal("Failed to check if thread is in efficient cpuset cgroup: ", err)
	} else if !b {
		s.Fatal("Thread should be in efficient cpuset cgroup")
	}
	if b, err := containsID(efficientCPUSetCgroupThreadsFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if thread is in efficient cpuset cgroup: ", err)
	} else if !b {
		s.Fatal("Thread should be in efficient cpuset cgroup")
	}

	// Process is back to normal
	if err := rm.SetProcessState(ctx, pChronos.Pid, resourced.QoSProcessNormal); err != nil {
		s.Fatal("Failed to set process state: ", err)
	}
	if b, err := containsID(normalCPUCgroupProcessesFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if process is in normal cpu cgroup: ", err)
	} else if !b {
		s.Fatal("Process should be in normal cpu cgroup")
	}
	// Threads which were in all cpuset cgroup are also back to all cpuset cgroup.
	if b, err := containsID(allCPUSetCgroupThreadsFile, pChronos.Tid); err != nil {
		s.Fatal("Failed to check if thread is in all cpuset cgroup: ", err)
	} else if !b {
		s.Fatal("Thread should be in all cpuset cgroup")
	}
	// Threads which were in efficient cpuset cgroup remains in efficient cpuset cgroup.
	if b, err := containsID(efficientCPUSetCgroupThreadsFile, pChronos.Pid); err != nil {
		s.Fatal("Failed to check if thread is in efficient cpuset cgroup: ", err)
	} else if !b {
		s.Fatal("Thread should be in efficient cpuset cgroup")
	}
}
