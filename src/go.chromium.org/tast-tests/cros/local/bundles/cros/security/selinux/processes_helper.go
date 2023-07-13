// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package selinux

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/sandboxing"
	"go.chromium.org/tast/core/errors"
)

// Process represents a running process with an SELinux context.
type Process struct {
	Pid       int32
	Cmdline   string
	Exe       string
	Comm      string
	Euid      uint32 // effective UID
	Ecaps     uint64 // effective capabilities
	SEContext string
}

// ProcessFilter specifies which processes GetProcess() should return.
type ProcessFilter int

const (
	// All returns all processes.
	All ProcessFilter = iota
	// PrivilegedOnly returns processes running as root or with CAP_SYS_ADMIN.
	PrivilegedOnly
)

const capSysAdminMask = 1 << sandboxing.CapSysAdmin

// String returns a human-readable string representation for struct Process.
func (p Process) String() string {
	// Exe and Cmdline can differ if the initial executable is a symlink.
	// Since some checks match Exe and others match Cmdline, print both, so that
	// exceptions can be easily added.
	return fmt.Sprintf("[exe=%q cmdline=%q uid=%d domain=%s]", p.Exe, p.Cmdline, p.Euid, p.SEContext)
}

// GetProcesses returns currently-running processes.
func GetProcesses(filter ProcessFilter) ([]Process, error) {
	ps, err := process.Processes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list processes")
	}
	var processes []Process
	for _, p := range ps {
		proc := Process{Pid: p.Pid}

		// Ignore all errors returned by gopsutil while reading process data; these typically
		// indicate that this is a kernel process (in the case of exe) or that the process
		// disappeared mid-test. gopsutil doesn't make any promises that the error that it
		// returns in the process-disappeared case will be os.ErrNotExist: https://crbug.com/918499
		if proc.Exe, err = p.Exe(); err != nil {
			continue
		}
		if proc.Cmdline, err = p.Cmdline(); err != nil {
			continue
		}

		if uids, err := p.Uids(); err != nil {
			continue
		} else {
			proc.Euid = uint32(uids[1])
		}

		if comm, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/comm", proc.Pid)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		} else {
			proc.Comm = string(comm)
		}

		// Read additional info from /proc/<pid>/status.
		if status, err := sandboxing.ReadProcStatus(proc.Pid); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, errors.Wrap(err, "failed reading /proc/<pid>/status")
		} else if proc.Ecaps, err = strconv.ParseUint(status["CapEff"], 16, 64); err != nil {
			return nil, errors.Wrapf(err, "failed parsing effective caps %q", status["CapEff"])
		}

		// PrivilegedOnly retains processes running as root (i.e. proc.Euid == 0) or
		// with CAP_SYS_ADMIN (i.e. proc.Ecaps&capSysAdminMask != 0).
		if filter == PrivilegedOnly && (proc.Euid != 0 || proc.Ecaps&capSysAdminMask == 0) {
			continue
		}

		if secontext, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/attr/current", proc.Pid)); err != nil {
			// ESRCH 3 No such process is returned for syscall read, if process dies after open succeeds.
			// TODO: check ESRCH instead of string when golang updates to have such builtin functions.
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such process") {
				continue
			}
			return nil, err
		} else if len(secontext) == 0 || secontext[len(secontext)-1] != 0 {
			return nil, errors.Errorf("invalid secontext %q", secontext)
		} else {
			proc.SEContext = string(secontext[:len(secontext)-1])
		}

		processes = append(processes, proc)
	}
	return processes, nil
}

// FindProcessesByExe returns processes from ps with Exe fields matching exeRegex.
func FindProcessesByExe(ps []Process, exeRegex string, reverse bool) ([]Process, error) {
	var found []Process
	for _, proc := range ps {
		matched, err := regexp.MatchString("^"+exeRegex+"$", proc.Exe)
		if err != nil {
			return nil, err
		}
		if matched != reverse {
			found = append(found, proc)
		}
	}
	return found, nil
}

// FindProcessesByCmdline returns processes from ps with Cmdline fields
// matching(reverse=false) or not matching(reverse=true) partial regular
// expression cmdlineRegex.
func FindProcessesByCmdline(ps []Process, cmdlineRegex string, reverse bool) ([]Process, error) {
	var found []Process
	for _, proc := range ps {
		matched, err := regexp.MatchString("^"+cmdlineRegex+"$", proc.Cmdline)
		if err != nil {
			return nil, err
		}
		if matched != reverse {
			found = append(found, proc)
		}
	}
	return found, nil
}

// ProcessContextRegexp returns a regexp from context, by wrapping it like "^u:r:xxx:.*$".
func ProcessContextRegexp(context string) (*regexp.Regexp, error) {
	return regexp.Compile("^u:r:" + context + ":.*$")
}
