// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdprocfsscraper contains shared code for scraping and parsing
// kernel procfs files.
package secagentdprocfsscraper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tklauser/go-sysconf"
	"google.golang.org/protobuf/proto"

	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	cmdLineRetryTimes = 5
	// https://man7.org/linux/man-pages/man5/proc.5.html
	statSize = 52
	// This corresponds to the number of distinct BPFs in use by secagentd and
	// needs to be kept in sync manually.
	// TODO(b:287180930): Increase to 3 after fieldtrial config for network lands.
	expSecagentdBpfMaps = 2
)

var (
	procNsRe     = regexp.MustCompile("(?m)^[a-z]+:\\[(?P<nsId>[[:digit:]]+)\\]")
	procStatusRe = regexp.MustCompile(
		"(?m)^Name:[[:blank:]]+(?P<name>[A-Za-z0-9_-]+)[[:blank:]]*[\\n\\r]+" +
			"^Umask:[[:blank:]]+(?P<umask>[[:digit:]]+)[[:blank:]]*[\\n\\r]+" +
			"^State:[[:blank:]]+(?P<state>[[:alpha:]]).*[\\n\\r]+" +
			"^Tgid:.*[\\n\\r]+" +
			"^Ngid:.*[\\n\\r]+" +
			"^Pid:[[:blank:]]+(?P<pid>[[:digit:]]+)[[:blank:]]*[\\n\\r]+" +
			"^PPid:[[:blank:]]+(?P<ppid>[[:digit:]]+)[[:blank:]]*[\\n\\r]+" +
			"^TracerPid:.*[\\n\\r]+" +
			"^Uid:[[:blank:]]+(?P<real_uid>[[:digit:]]+)[[:blank:]]*.*[\\n\\r]+" +
			"^Gid:[[:blank:]]+(?P<real_gid>[[:digit:]]+)[[:blank:]]*.*[\\n\\r]+")
)

// GetCmdLineParts returns a list of cmdline arguments for the given pid.
func GetCmdLineParts(ctx context.Context, pid uint64) ([]string, error) {
	var buff []byte
	// Procfs cmdline is read from the process' VM and may read an empty string
	// if the process is currently swapped out. Add a few retries to hopefully
	// catch the process while it's swapped in.
	if err := action.Retry(cmdLineRetryTimes, func(context.Context) error {
		cmdLine := fmt.Sprintf("/proc/%d/cmdline", pid)
		var err error
		buff, err = ioutil.ReadFile(cmdLine)
		if err != nil {
			return err
		}
		if len(buff) == 0 {
			return errors.Errorf("%s was read but found empty", cmdLine)
		}
		return nil
	}, 100*time.Millisecond)(ctx); err != nil {
		return nil, err
	}

	// Split on NUL and returns the parts.
	return strings.Split(string(buff), string(rune(0))), nil
}

// GetCmdLine returns a joined and quoted string of cmdline arguments for the
// given pid.
func GetCmdLine(ctx context.Context, pid uint64) (string, error) {
	parts, err := GetCmdLineParts(ctx, pid)
	if err != nil {
		return "", err
	}
	var quotedParts []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		quotedParts = append(quotedParts, fmt.Sprintf("'%s'", part))
	}
	return strings.Join(quotedParts, " "), nil
}

// FillNamespaces populates the given XDR proto with process namespace
// info for the requested pid.
func FillNamespaces(pid uint64, ns *xdr.Namespaces) error {
	validNamespaces := map[string]**uint64{
		"cgroup": &ns.CgroupNs, "ipc": &ns.IpcNs, "mnt": &ns.MntNs,
		"net": &ns.NetNs, "pid": &ns.PidNs,
		"user": &ns.UserNs, "uts": &ns.UtsNs}
	dirs, err := ioutil.ReadDir(fmt.Sprintf("/proc/%d/ns", pid))
	if err != nil {
		return err
	}
	for _, f := range dirs {
		field, ok := validNamespaces[f.Name()]
		if !ok {
			continue
		}
		nsSymlink := fmt.Sprintf("/proc/%d/ns/%s", pid, f.Name())
		rawNamespaceID, err := os.Readlink(nsSymlink)
		if err != nil {
			return err
		}
		m := procNsRe.FindStringSubmatch(rawNamespaceID)
		if m == nil {
			return errors.Errorf("unrecognized namespace ID format %s in symlink %s", rawNamespaceID, nsSymlink)
		}

		nsID := m[procNsRe.SubexpIndex("nsId")]
		val, err := strconv.ParseUint(nsID, 10, 64)
		if err != nil {
			return err
		}
		*field = proto.Uint64(val)
	}
	return nil
}

// FillImage populates the given XDR proto with process image information
// for the given pid.
func FillImage(pid, mntNs uint64, i *xdr.FileImage) error {
	exeFilename := fmt.Sprintf("/proc/%d/exe", pid)
	imagePath, err := filepath.EvalSymlinks(exeFilename)
	if err != nil {
		return err
	}
	i.Pathname = proto.String(imagePath)
	image, err := ioutil.ReadFile(imagePath)
	if err != nil {
		return err
	}

	csum := sha256.New()
	if _, err := csum.Write(image); err != nil {
		return err
	}
	i.Sha256 = proto.String(strings.ToUpper(hex.EncodeToString(csum.Sum(nil))))

	fileInfo, err := os.Stat(imagePath)
	if err != nil {
		return nil
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.Errorf("failed to stat %s", imagePath)
	}
	i.CanonicalGid = proto.Uint64(uint64(stat.Gid))
	i.CanonicalUid = proto.Uint64(uint64(stat.Uid))
	i.Inode = proto.Uint64(stat.Ino)
	i.InodeDeviceId = proto.Uint64(stat.Dev)
	i.Mode = proto.Uint32(stat.Mode)
	i.MntNs = proto.Uint64(mntNs)

	return nil
}

// FillProc populates the given XDR proto using information from /proc/x/status
// and /proc/x/stat for the given pid.
func FillProc(pid uint64, p *xdr.Process) (uint64, error) {
	statusFilename := fmt.Sprintf("/proc/%d/status", pid)
	buff, err := ioutil.ReadFile(statusFilename)
	if err != nil {
		return 0, err
	}
	m := procStatusRe.FindStringSubmatch(string(buff))
	if m == nil {
		return 0, errors.Errorf("unable to match {%s} using regex {%s}",
			string(buff), procStatusRe)
	}
	ppid, err := strconv.ParseUint(m[procStatusRe.SubexpIndex("ppid")], 10, 64)
	if err != nil {
		return 0, err
	}
	uid, err := strconv.ParseUint(m[procStatusRe.SubexpIndex("real_uid")], 10, 64)
	if err != nil {
		return 0, err
	}
	p.CanonicalUid = proto.Uint64(uid)
	p.CanonicalPid = proto.Uint64(pid)

	ticksPerSecond, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		return 0, err
	}
	// Start time relative to boot time is found in the stat file.
	statFilename := fmt.Sprintf("/proc/%d/stat", pid)
	buff, err = ioutil.ReadFile(statFilename)
	if err != nil {
		return 0, err
	}
	statParts := strings.Split(string(buff), " ")

	if len(statParts) != statSize {
		return 0, errors.Errorf("stat file did not match expected length; got: %d, want: %d", len(statParts), statSize)
	}
	// 22nd entry in stat corresponds to start time which is the time the process
	// is started after system boot. It is expressed in clock ticks.
	startTimeTicks, err := strconv.ParseInt(statParts[21], 10, 64)
	if err != nil {
		return 0, err
	}
	startTimeSeconds := startTimeTicks / ticksPerSecond
	p.RelStartTimeS = proto.Int64(startTimeSeconds)
	return ppid, nil
}

// WaitForBpfMaps polls until secagentd reports to be using the expected number
// of BPF maps (corresponding to the number of BPFs initialized so far).
func WaitForBpfMaps(ctx context.Context, pid uint64) error {
	mapsFilename := fmt.Sprintf("/proc/%d/maps", pid)
	return testing.Poll(ctx, func(ctx context.Context) error {
		buff, err := ioutil.ReadFile(mapsFilename)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read proc map"))
		}
		if strings.Count(string(buff), "bpf-map") < expSecagentdBpfMaps {
			return errors.New("Did not find the expected number of BPF maps")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// GetOnlyChildPid returns the pid of the first child of the first task of the
// given pid. Returns an error if the first task has more than one child. Main
// application is to get secagentd pid given its minijail0 pid.
func GetOnlyChildPid(pid uint64) (uint64, error) {
	childrenFilename := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	buff, err := ioutil.ReadFile(childrenFilename)
	if err != nil {
		return 0, err
	}
	childPid, err := strconv.ParseUint(strings.TrimSpace(string(buff)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "task %d does not have exactly one child", pid)
	}
	return childPid, nil
}
