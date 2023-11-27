// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sched contains test helpers for sched testing.
package sched

import (
	"context"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const script = `import time
import threading

def loop_forever():
    while True:
        time.sleep(1)

threading.Thread(target=loop_forever).start()

loop_forever()
`

// SampleProcess represents a process with two threads.
type SampleProcess struct {
	Pid uint32
	Tid uint32
	cmd *exec.Cmd
}

// KillAndWait kills the process and waits for it to exit.
func (p *SampleProcess) KillAndWait() error {
	if err := p.cmd.Process.Kill(); err != nil {
		return errors.Wrap(err, "failed to kill process")
	}
	p.cmd.Wait()
	return nil
}

func loadThreadID(ctx context.Context, p *process.Process) (uint32, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if n, err := p.NumThreadsWithContext(ctx); err != nil {
			return testing.PollBreak(err)
		} else if n == 2 {
			return nil
		} else if status, err := p.StatusWithContext(ctx); err != nil {
			return testing.PollBreak(err)
		} else if status[0] == process.Zombie {
			return testing.PollBreak(errors.New("process is zombie"))
		} else {
			return errors.Errorf("expected 2 threads, got %d", n)
		}
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 100 * time.Second}); err != nil {
		return 0, errors.Wrap(err, "failed to wait for threads")
	}

	threads, err := p.ThreadsWithContext(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get threads")
	}
	if len(threads) != 2 {
		return 0, errors.Errorf("expected 2 threads, got %d", len(threads))
	}
	var tid uint32
	for id := range threads {
		if id == p.Pid {
			continue
		}
		tid = uint32(id)
		break
	}
	return tid, nil
}

// SysProcAttrOfUser creates syscall.SysProcAttr specifying the user.
func SysProcAttrOfUser(name string) (*syscall.SysProcAttr, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to lookup chronos user")
	}
	uidInt, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert uid to int")
	}
	gidInt, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert gid to int")
	}
	var attr syscall.SysProcAttr
	attr.Credential = &syscall.Credential{Uid: uint32(uidInt), Gid: uint32(gidInt)}
	return &attr, nil
}

// CreateSampleProcessThreadPair creates a process with two threads.
func CreateSampleProcessThreadPair(ctx context.Context, attr *syscall.SysProcAttr) (*SampleProcess, error) {
	cmd := exec.Command("python")

	cmd.SysProcAttr = attr

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create stdin pipe")
	}
	defer pipe.Close()

	script := []byte(script)
	n, err := pipe.Write(script)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write to stdin")
	} else if n != len(script) {
		return nil, errors.New("failed to write all bytes to stdin")
	} else if err = pipe.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to close stdin")
	}

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start python")
	}

	p, err := process.NewProcessWithContext(ctx, int32(cmd.Process.Pid))
	if err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		return nil, errors.Wrap(err, "failed to get process")
	}
	tid, err := loadThreadID(ctx, p)
	if err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		return nil, errors.Wrap(err, "failed to load thread id")
	}
	pid := uint32(p.Pid)

	return &SampleProcess{
		Pid: pid,
		Tid: tid,
		cmd: cmd,
	}, nil
}
