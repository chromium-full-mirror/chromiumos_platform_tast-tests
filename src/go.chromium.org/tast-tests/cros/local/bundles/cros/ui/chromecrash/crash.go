// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chromecrash contains functionality shared by tests that
// exercise Chrome crash-dumping.
package chromecrash

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/chromeproc"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosinfo"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// CryptohomePattern is a glob pattern that matches all of the cryptohome links
	// inside of /run/daemon-store
	CryptohomePattern = "/run/daemon-store/crash/*"

	// TestModeSuccessfulFile is the special file that crash_sender creates if it
	// successfully got the crash report. MUST MATCH kTestModeSuccessfulFile in
	// crash_sender_util.cc
	TestModeSuccessfulFile = "/var/spool/crash/crash_sender_test_mode_successful"

	// vModuleFlag is passed to Chrome when testing Chrome crashes. It allows us
	// to debug certain failures, particularly cases where consent didn't get set
	// up correctly, as well as any problems with the upcoming crashpad changeover.
	vModuleFlag = "--vmodule=chrome_crash_reporter_client=1,breakpad_linux=1,crashpad=1,crashpad_linux=1,broker_process=3,sandbox_linux=3,stats_reporting_controller=1,autotest_private_api=1"

	// nanosecondsPerMillisecond helps convert ns to ms. Needed to deal with
	// gopsutil/process which reports creation times in milliseconds-since-UNIX-epoch,
	// while golang's time can only be constructed using nanoseconds-since-UNIX-epoch.
	nanosecondsPerMillisecond = 1000 * 1000

	// chromeConfPath is the path to the Chrome config file. Extra entries in that
	// file occasionally mess up Chrome crash tests.
	chromeConfPath = "/etc/chrome_dev.conf"

	// crashReporterExecPath is the path to the crash_reporter executable.
	crashReporterExecPath = "/sbin/crash_reporter"

	// ashChromeCrashpadExecPath is the path to the ash-Chrome crashpad binary.
	ashChromeCrashpadExecPath = "/opt/google/chrome/chrome_crashpad_handler"

	// lacrosCrashpadExecName is the name of the crashpad binary in the Lacros directory.
	lacrosCrashpadExecName = "chrome_crashpad_handler"

	// chromeCrashFilePatternWithPid contains fmt.Sprintf format string that
	// expects an integer PID of a chrome process. fmt.Sprintf will return a regex
	// string that, when concatenated with an extension, matches Chrome crashes
	// from that PID.
	chromeCrashFilePatternWithPid = `chrome\.\d{8}\.\d{6}\.\d+\.%d\.`

	// breakpadDmpFileRegexp is the regexp we use to find a BreakpadDmp file.
	// Unlike normal .meta files, the breakpad .dmp files do not include
	// the PID in the filename.
	breakpadDmpFileRegexp = `chromium-.*-minidump-.*\.dmp`
)

// CrashHandler indicates which crash handler the test wants Chrome to use:
// breakpad or crashpad.
type CrashHandler int

const (
	// Breakpad indicates Chrome should install the older breakpad crash handler. Breakpad
	// is an in-process crash handler.
	Breakpad CrashHandler = iota
	// Crashpad indicates Chrome should install the new crashpad crash handler. Crashpad
	// runs as a separate "chrome_crashpad_handler" process which monitors the chrome processes.
	Crashpad
)

// GetExtraArgs gives the list of arguments we should pass via chrome.ExtraArgs into the
// chrome.New() function.
func GetExtraArgs(handler CrashHandler, consentType crash.ConsentType) []string {
	// Breakpad needs an extra argument to do mock consent. Crashpad doesn't care
	// about consent (at least on ChromeOS); it passes all crashes along to
	// crash_reporter and lets crash_reporter decide on consent. So we don't need
	// any extra arguments if the crash handler is Crashpad.
	if handler == Breakpad && consentType == crash.MockConsent {
		return []string{vModuleFlag, "--enable-crash-reporter-for-testing"}
	}

	return []string{vModuleFlag}
}

// ProcessType is an enum listed the types of Chrome processes we can kill.
type ProcessType int

const (
	// Browser indicates the root Chrome process, the one without a --type flag.
	Browser ProcessType = iota
	// GPUProcess indicates a process with --type=gpu-process. We use GPUProcess
	// to stand in for most types of non-Browser processes, including renderer and
	// zygote; given the comments above Chrome's NonBrowserCrashHandler class, the
	// code path should be similar enough that we don't need separate tests for
	// those process types.
	GPUProcess
	// Broker indicates a process with --type=broker. Broker processes go through
	// a special code path because they are forked directly.
	Broker
)

// String returns a string naming the given ProcessType, suitable for displaying
// in log and error messages.
func (ptype ProcessType) String() string {
	switch ptype {
	case Browser:
		return "Browser"
	case GPUProcess:
		return "GPUProcess"
	case Broker:
		return "Broker"
	default:
		return "Unknown ProcessType " + strconv.Itoa(int(ptype))
	}
}

// CrashFileType is an enum listing the types of crash output files the crash
// system might produce.
type CrashFileType int

const (
	// MetaFile refers to the .meta files created by crash_reporter. This is the
	// normal crash file type.
	MetaFile CrashFileType = iota
	// BreakpadDmp indicates the .dmp files generated directly by breakpad and
	// crashpad. We only see these when we are skipping crash_reporter and having
	// breakpad / crashpad dump directly.
	BreakpadDmp
	// NoCrashFile indicates that we don't expect to see any crash files from the
	// kill. Note that this means KillAndGetCrashFiles() may not wait for anything
	// after sending a signal to the target process; the target process will
	// often not be completely finished crashing after KillAndGetCrashFiles()
	// returns.
	NoCrashFile
)

// String returns a string naming the given CrashFileType, suitable for displaying
// in log and error messages.
func (cfType CrashFileType) String() string {
	switch cfType {
	case MetaFile:
		return "MetaFile"
	case BreakpadDmp:
		return "BreakpadDmp"
	case NoCrashFile:
		return "NoCrashFile"
	default:
		return "Unknown CrashFileType " + strconv.Itoa(int(cfType))
	}
}

// crashReporterRunning returns true if any crash_reporter processes are
// currently running, false otherwise.
func crashReporterRunning(ctx context.Context) (bool, error) {
	all, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return false, err
	}

	for _, process := range all {
		if exe, err := process.Exe(); err == nil && exe == crashReporterExecPath {
			return true, nil
		}
		// else ignore the error. If a process exited, or we otherwise can't
		// get its executable path, we want to keep going and looking for
		// crash_reporter processes.
	}

	return false, nil
}

// CrashTester maintains state between different parts of the Chrome crash tests.
// It should be created (via NewCrashTester) before chrome.New is called. Close should be
// called at the end of the test.
type CrashTester struct {
	ptype       ProcessType
	browserType browser.Type
	waitFor     CrashFileType
	tconn       *chrome.TestConn
	lacrosPath  string
	killedPID   int32
}

// NewCrashTester returns a CrashTester. This must be called before chrome.New.
// ptype indicates the type of process the KillAndGetCrashFiles will kill. For
// some process types, we wait for the crash file to appear. In these cases,
// waitFor indicates the type of file we are waiting for.
func NewCrashTester(ctx context.Context, ptype ProcessType, browserType browser.Type, waitFor CrashFileType) (*CrashTester, error) {
	if ptype < Browser || ptype > Broker {
		return nil, errors.Errorf("ptype out of range: %v", ptype)
	}
	if browserType != browser.TypeAsh && browserType != browser.TypeLacros {
		return nil, errors.Errorf("browserType %v unknown", browserType)
	}
	if waitFor < MetaFile || waitFor > NoCrashFile {
		return nil, errors.Errorf("waitFor out of range: %v", waitFor)
	}

	return &CrashTester{
		ptype:       ptype,
		browserType: browserType,
		waitFor:     waitFor,
	}, nil
}

// AssociateWithChrome should be called once a stable ash-Chrome connection is
// created via chrome.New and (if applicable) Lacros has been launched. (Or after
// browserfixt,SetUpWithNewChrome has been called.) It is not needed if
// ash-Chrome won't be up long enough to make a stable connection.
func (ct *CrashTester) AssociateWithChrome(ctx context.Context, cr *chrome.Chrome) error {
	if ct.browserType == browser.TypeAsh {
		return nil
	}

	var err error
	ct.tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "could not associate with chrome: could not get test API")
	}

	info, err := lacrosinfo.Snapshot(ctx, ct.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve lacrosinfo")
	}
	if len(info.LacrosPath) == 0 {
		return errors.New("lacros is not running (received empty LacrosPath)")
	}
	ct.lacrosPath = info.LacrosPath
	return nil
}

// Close closes a CrashTester. It must be called on all CrashTesters returned from NewCrashTester.
func (ct *CrashTester) Close() {
}

// verifyChromeConfFile verifies that the Chrome configuration file
// (/etc/chrome_dev.conf) doesn't have any non-comment entries.
//
// We've had problems with extraneous crash configuration entries left over from
// other tests causing crash tests to flake. To avoid breaking developers'
// workflows (they may have some options in there and still want to run tast
// tests), this will just print warnings about extra entries; it doesn't return
// an error and doesn't fail the test. That should be enough information to
// understand the problem.
func verifyChromeConfFile(ctx context.Context) {
	file, err := os.Open(chromeConfPath)
	if err != nil {
		testing.ContextLogf(ctx, "Could not read %s: %v", chromeConfPath, err)
		return
	}
	defer file.Close()

	var extraOptions []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && line[0] != '#' {
			extraOptions = append(extraOptions, line)
		}
	}

	if len(extraOptions) > 0 {
		testing.ContextLogf(ctx, "Extra options in %s: %v", chromeConfPath, extraOptions)
	}
}

func cryptohomeCrashDirs(ctx context.Context) ([]string, error) {
	paths, err := filepath.Glob(CryptohomePattern)
	if err != nil {
		return nil, err
	}

	return paths, nil
}

// deleteFiles deletes the supplied paths.
func deleteFiles(ctx context.Context, paths []string) {
	for _, p := range paths {
		testing.ContextLog(ctx, "Removing new crash file ", p)
		if err := os.Remove(p); err != nil {
			testing.ContextLogf(ctx, "Unable to remove %v: %v", p, err)
		}
	}
}

// FindCrashFilesIn looks through the list of files returned from KillAndGetCrashFiles,
// expecting to find the crash output files written by crash_reporter after a Chrome crash.
// In particular, it expects to find a .meta file and a matching .dmp file.
// dirPattern is a glob-style pattern indicating where the crash files should be found.
// FindCrashFilesIn returns an error if the files are not found in the expected
// directory; otherwise, it returns nil.
func FindCrashFilesIn(dirPattern string, files []string) error {
	filePattern := filepath.Join(dirPattern, "chrome*.meta")
	var meta string
	for _, file := range files {
		if match, _ := filepath.Match(filePattern, file); match {
			meta = file
			break
		}
	}

	if meta == "" {
		return errors.Errorf("could not find crash's meta file in %s (possible files: %v)", dirPattern, files)
	}

	dump := strings.TrimSuffix(meta, "meta") + "dmp"
	for _, file := range files {
		if file == dump {
			return nil
		}
	}

	return errors.Errorf("did not find the dmp file %s corresponding to the crash meta file", dump)
}

// FindBreakpadDmpFilesIn looks through the list of files returned from KillAndGetCrashFiles,
// expecting to find .dmp output files written by breakpad if it writes the dump
// directly (without invoking crash_reporter).
func FindBreakpadDmpFilesIn(dirPattern string, files []string) error {
	filePatternStr := "^" + filepath.Join(dirPattern, breakpadDmpFileRegexp) + "$"
	filePattern, err := regexp.Compile(filePatternStr)
	if err != nil {
		return errors.Wrapf(err, "invalid file pattern %s", filePatternStr)
	}
	for _, file := range files {
		if filePattern.MatchString(file) {
			return nil
		}
	}

	return errors.Errorf("could not find breakpad's .dmp file in %s (possible files: %v)", dirPattern, files)
}

// waitForChromeProcesses waits for the Chrome to be up and then
// returns a list of Processes of chrome and its crashpad_handlers.
func (ct *CrashTester) waitForChromeProcesses(ctx context.Context) ([]*process.Process, error) {
	var procs []*process.Process
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if ct.browserType == browser.TypeAsh {
			procs, err = ashproc.Processes()
		} else {
			procs, err = lacrosproc.ProcsFromPath(ctx, ct.lacrosPath)
		}
		if err != nil {
			return testing.PollBreak(err)
		}
		// TODO(hidehiko): Move this out from Chrome processes, because in precise
		// crashpad handler is not chrome.
		chProcs, err := ct.crashpadHandlerProcesses()
		if err != nil {
			return testing.PollBreak(err)
		}
		procs = append(procs, chProcs...)

		if len(procs) == 0 {
			return errors.New("no Chrome processes found")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return nil, errors.Wrap(err, "failed to get Chrome processes")
	}

	return procs, nil
}

// crashpadHandlerProcesses returns all of Chrome's crashpad handler processes.
func (ct *CrashTester) crashpadHandlerProcesses() ([]*process.Process, error) {
	ps, err := process.Processes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain processes")
	}

	var crashpadExecPath string
	if ct.browserType == browser.TypeAsh {
		crashpadExecPath = ashChromeCrashpadExecPath
	} else {
		crashpadExecPath = filepath.Join(ct.lacrosPath, lacrosCrashpadExecName)
	}

	var ret []*process.Process
	for _, p := range ps {
		// Identify by exec path. Ignore errors.
		// Because of timing issue, the process may be gone between the
		// Process instance creation and Exe invocation.
		if exe, err := p.Exe(); err != nil || exe != crashpadExecPath {
			continue
		}
		ret = append(ret, p)
	}
	return ret, nil
}

// getNonBrowserProcess returns a Process structure of a single Chrome process
// of the indicated type. If more than one such process exists, the first one is
// returned. Does not wait for the process to come up -- if none exist, this
// just returns an error.
func (ct *CrashTester) getNonBrowserProcess(ctx context.Context) (*process.Process, error) {
	var err error
	var processes []*process.Process
	switch ct.ptype {
	case GPUProcess:
		if ct.browserType == browser.TypeAsh {
			processes, err = chromeproc.GetGPUProcesses()
		} else {
			processes, err = lacrosproc.GPUProcesses(ctx, ct.tconn)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "error looking for Chrome %s", ct.ptype)
		}
		if len(processes) == 0 {
			return nil, errors.Errorf("no Chrome %s's found", ct.ptype)
		}
		return processes[0], nil
	case Broker:
		if ct.browserType == browser.TypeAsh {
			processes, err = chromeproc.GetBrokerProcesses()
		} else {
			processes, err = lacrosproc.BrokerProcesses(ctx, ct.tconn)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "error looking for Chrome %s", ct.ptype)
		}
		if len(processes) == 0 {
			return nil, errors.Errorf("no Chrome %s's found", ct.ptype)
		}
		return processes[0], nil
	default:
		return nil, errors.Errorf("unexpected ProcessType %s", ct.ptype)
	}
}

// waitForMetaFile waits for a .meta file corresponding to the given pid to appear
// in one of the directories.
// Return nil if the file is found.
func waitForMetaFile(ctx context.Context, pid int, dirs []string) error {
	ending := fmt.Sprintf(chromeCrashFilePatternWithPid+"meta", pid)
	results, err := crash.WaitForCrashFiles(ctx, dirs, []string{ending})
	if err != nil {
		return errors.Wrap(err, "error waiting for .meta file")
	}

	// Investigating test flakes -- why do some parts of the test see the meta
	// files and others don't? Dump exactly what WaitForCrashFiles saw.
	// TODO(crbug.com/1080365): Remove this once flake is resolved; it's kinda
	// spammy.
	testing.ContextLog(ctx, "crash.WaitForCrashFiles returned ", results)

	return nil
}

// waitForBreakpadDmpFile waits for a .dmp file corresponding to the given pid
// to appear in one of the directories.
// Return nil if the file is found.
func waitForBreakpadDmpFile(ctx context.Context, pid int, dirs []string) error {
	err := testing.Poll(ctx, func(c context.Context) error {
		files, err := crash.WaitForCrashFiles(ctx, dirs, []string{breakpadDmpFileRegexp})
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "error waiting for .dmp file"))
		}

		// We don't want to immediately fail if we can't parse a .dmp file. .dmp files
		// may be in the middle of being written out when this code runs, in which
		// case we'll get all sorts of odd errors. There's also a (small) possibility
		// of one malformed .dmp file (maybe from crash_reporter) matching, in
		// which case we just want to look at the other .dmp files. Remember any
		// errors for later debugging, but don't stop looking and, above all,
		// don't testing.PollBreak.
		errorList := make([]error, 0)
		for _, fileName := range files[breakpadDmpFileRegexp] {
			if found, err := crash.IsBreakpadDmpFileForPID(fileName, pid); err != nil {
				errorList = append(errorList, errors.Wrap(err, "error scanning "+fileName))
			} else if found {
				// Success, ignore all other errors.
				return nil
			}
		}
		if len(errorList) == 0 {
			return errors.Errorf("could not find dmp file with PID %d in %v", pid, files)
		}
		if len(errorList) == 1 {
			return errorList[0]
		}
		return errors.Errorf("multiple errors found scanning .dmp files: %v", errorList)
	}, nil)
	if err != nil {
		return errors.Wrap(err, "error waiting for .dmp file")
	}
	return nil
}

// killNonBrowser implements the killing heart of KillAndGetCrashFiles for any
// process type OTHER than the root Browser type.
func (ct *CrashTester) killNonBrowser(ctx context.Context, dirs []string) error {
	testing.ContextLog(ctx, "Hunting for a ", ct.ptype)
	var toKill *process.Process
	// It's possible that the root browser process just started and hasn't created
	// the GPU or broker process yet. Also, if the target process disappears during
	// the 3 second stabilization period, we'd be willing to try a different one.
	// Retry until we manage to successfully send a SEGV.
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		toKill, err = ct.getNonBrowserProcess(ctx)
		if err != nil {
			return errors.Wrapf(err, "could not find Chrome %s to kill", ct.ptype)
		}

		// Sleep briefly after the Chrome process we want starts so it has time to set
		// up crashpad.
		const delay = 3 * time.Second
		createTimeMS, err := toKill.CreateTime()
		if err != nil {
			return errors.Wrap(err, "could not get create time of process")
		}
		createTime := time.Unix(0, createTimeMS*nanosecondsPerMillisecond)
		timeToSleep := delay - time.Since(createTime)
		if timeToSleep > 0 {
			testing.ContextLogf(ctx, "Sleeping %v to wait for Chrome to stabilize", timeToSleep)
			// GoBigSleepLint: Sleep briefly after the Chrome process we want starts
			// so it has time to set up breakpad / crashpad.
			// TODO(b/292578303): Improve by making it easier for this test to tell
			// when crashpad is set up.
			if err := testing.Sleep(ctx, timeToSleep); err != nil {
				return testing.PollBreak(errors.Wrap(err, "timed out while waiting for Chrome startup"))
			}
		}

		testing.ContextLogf(ctx, "Sending SIGSEGV to target Chrome %s pid %d", ct.ptype, toKill.Pid)
		if err := toKill.SendSignal(unix.SIGSEGV); err != nil {
			if errno, ok := err.(unix.Errno); ok && errno == unix.ESRCH {
				return errors.Errorf("target process %d does not exist", toKill.Pid)
			}
			return testing.PollBreak(errors.Wrapf(err, "could not kill target process %d", toKill.Pid))
		}
		ct.killedPID = toKill.Pid

		return nil
	}, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to find & kill a Chrome %s", ct.ptype)
	}

	// We don't wait for the process to exit here. Most non-Browser processes
	// don't actually exit when sent a SIGSEGV from outside the process. (This
	// is the expected behavior -- see
	// https://groups.google.com/a/chromium.org/d/msg/chromium-dev/W_vGBMHxZFQ/wPkqHBgBAgAJ)
	// Instead, we wait for the crash file to be written out.
	switch ct.waitFor {
	case MetaFile:
		testing.ContextLog(ctx, "Waiting for meta file to appear")
		if err = waitForMetaFile(ctx, int(toKill.Pid), dirs); err != nil {
			return errors.Wrap(err, "failed waiting for target to write crash meta files")
		}
	case BreakpadDmp:
		testing.ContextLog(ctx, "Waiting for dmp file to appear")
		if err = waitForBreakpadDmpFile(ctx, int(toKill.Pid), dirs); err != nil {
			return errors.Wrap(err, "failed waiting for target to write crash dmp files")
		}
	case NoCrashFile:
		return nil
	default:
		return errors.Errorf("unexpected CrashFileType %d", ct.waitFor)
	}

	return nil
}

// killBrowser implements the specialized logic for killing & waiting for the
// root Browser process. The principal difference between this and killNonBrowser
// is that this waits for the SEGV'ed process to die instead of waiting for a
// .meta file. We can't wait for a file to be created because ChromeCrashLoop
// doesn't create files at all on one of its kills.
func (ct *CrashTester) killBrowser(ctx context.Context) error {
	// Sleep briefly after Chrome starts so it has time to set up crashpad.
	// (Also needed for https://crbug.com/906690)
	const delay = 3 * time.Second
	testing.ContextLogf(ctx, "Sleeping %v to wait for Chrome to stabilize", delay)
	// GoBigSleepLint: Sleep briefly after the Chrome process we want starts
	// so it has time to set up breakpad or crashpad. (Also needed for
	// https://crbug.com/906690)
	// TODO(b/292578303): Improve by making it easier for this test to tell
	// when crashpad is set up.
	if err := testing.Sleep(ctx, delay); err != nil {
		return errors.Wrap(err, "timed out while waiting for Chrome startup")
	}

	procs, err := ct.waitForChromeProcesses(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find Chrome process IDs")
	}

	// The root Chrome process (i.e. the one that doesn't have another Chrome process
	// as its parent) is the browser process. It's not sandboxed, so it should be able
	// to write a minidump file when it crashes.
	var rp *process.Process
	if ct.browserType == browser.TypeAsh {
		rp, err = ashproc.Root()
	} else {
		rp, err = lacrosproc.Root(ctx, ct.tconn)
	}
	if err != nil {
		return errors.Wrap(err, "failed to get Chrome root process")
	}
	testing.ContextLog(ctx, "Sending SIGSEGV to root Chrome process ", rp)
	if err := rp.SendSignal(unix.SIGSEGV); err != nil {
		return errors.Wrap(err, "failed to kill process")
	}
	ct.killedPID = rp.Pid

	// Wait for all the processes to die (not just the root one). This avoids
	// messing up other killNonBrowser tests that might try to kill an orphaned
	// process. It also ensures that chrome_crashpad_handler has exited; this is
	// important since chrome_crashpad_handler can survive the root Chrome process and
	// still be in the middle of spawning crash_reporter.
	testing.ContextLogf(ctx, "Waiting for %d Chrome process(es) to exit", len(procs))
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		for _, p := range procs {
			r, err := p.IsRunning()
			if err != nil {
				// IsRunning is racey and sometimes produces spurious errors. Fail but
				// don't end the loop; just try again.
				return errors.Wrap(err, "failed to stat process")
			}
			if r {
				return errors.New("processes still exist")
			}
		}
		return nil
	}, nil); err != nil {
		return err
	}

	// Now wait for all running crash_reporter processes to exit. It's possible
	// for chrome_crashpad_handler to exit before crash_reporter is finished, and if
	// crash_reporter is still running, the meta file might not exist yet.
	// Fortunately, crash_reporter never takes too long to run.
	testing.ContextLog(ctx, "Waiting for all crash_reporters to exit")
	err = testing.Poll(ctx, func(ctx context.Context) error {
		if stillRunning, err := crashReporterRunning(ctx); err != nil {
			return testing.PollBreak(errors.Wrap(err, "error scanning for crash_reporter"))
		} else if stillRunning {
			return errors.New("crash_reporter still running")
		}
		return nil
	}, nil)
	if err != nil {
		return errors.Wrap(err, "crash_reporter didn't exit")
	}

	return nil
}

// metaFileContains checks that each value in |expectedValues| appears in
// |metaFile|. Return nil if each value is found.
func metaFileContains(ctx context.Context, metaFile string, expectedValues map[string]string) error {
	b, err := ioutil.ReadFile(metaFile)
	if err != nil {
		return errors.Wrapf(err, "couldn't read meta file %s contents", metaFile)
	}

	contents := string(b)
	for key, value := range expectedValues {
		expectedValue := "upload_var_client_" + key + "=" + value
		if !strings.Contains(contents, expectedValue) {
			return errors.New(".meta file did not contain expected " + key)
		}
	}
	return nil
}

// validateComputedSeverity checks the computed_severity and computed_product
// values in |metaFile|. Return nil if each value is found.
func (ct *CrashTester) validateComputedSeverity(ctx context.Context, metaFile string) error {
	expectedValues := map[string]string{}
	if ct.browserType == browser.TypeAsh {
		expectedValues["computed_product"] = "Ui"
	} else {
		expectedValues["computed_product"] = "Lacros"
	}
	// TODO(crbug.com/1466932): Set computed_severity to "INFO" for Broker after
	// Broker process crashes are reported with ptype=broker.
	if ct.ptype == Browser {
		expectedValues["computed_severity"] = "FATAL"
	} else if ct.ptype == GPUProcess {
		expectedValues["computed_severity"] = "ERROR"
	}

	return metaFileContains(ctx, metaFile, expectedValues)
}

// validateBuildTime ensures that the meta files from Lacros crashes have valid
// upload_var_build_time_millis in them. Lacros needs to populate
// upload_var_build_time_millis because crash_sender's
// SenderBase::EvaluateMetaFileMinimal() relies on it for age checks.
func (ct *CrashTester) validateBuildTime(ctx context.Context, metaFile string) error {
	if ct.browserType == browser.TypeAsh {
		// Only Lacros has build time included in crash reports.
		return nil
	}

	contents, err := ioutil.ReadFile(metaFile)
	if err != nil {
		return errors.Wrapf(err, "couldn't read meta file %s contents", metaFile)
	}

	re := regexp.MustCompile("upload_var_build_time_millis=[0-9]+\n")
	if re.Find(contents) == nil {
		return errors.Errorf("Did not find upload_var_build_time_millis in %q", contents)
	}
	return nil
}

// getMetaFilename returns the meta filename found in |matches|. Only one .meta file
// should exist.
func getMetaFilename(matches map[string][]string, metaRegex string) (string, error) {
	metaFiles := matches[metaRegex]
	if len(metaFiles) == 0 {
		return "", errors.New("expected a .meta file but found none")
	}
	if len(metaFiles) > 1 {
		return "", errors.New("found more than one meta file")
	}

	return metaFiles[0], nil
}

// getDmpFilename returns the .dmp filename found in |matches|. Only one .dmp file
// should exist.
func getDmpFilename(matches map[string][]string, dmpRegex string) (string, error) {
	dmpFiles := matches[dmpRegex]
	if len(dmpFiles) == 0 {
		return "", errors.New("expected a .dmp file but found none")
	}
	if len(dmpFiles) > 1 {
		return "", errors.New("found more than one .dmp file")
	}

	return dmpFiles[0], nil
}

// KillAndGetCrashFiles sends SIGSEGV to the given Chrome process, waits for it to
// crash, finds all the new crash files, and then deletes them and returns their paths.
func (ct *CrashTester) KillAndGetCrashFiles(ctx context.Context) ([]string, error) {
	verifyChromeConfFile(ctx)

	dirs, err := cryptohomeCrashDirs(ctx)
	if err != nil {
		return nil, err
	}
	dirs = append(dirs, crash.DefaultDirs()...)

	if ct.ptype == Browser {
		if err = ct.killBrowser(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to kill Browser process")
		}
	} else {
		if err = ct.killNonBrowser(ctx, dirs); err != nil {
			return nil, errors.Wrapf(err, "failed to kill Chrome %s", ct.ptype)
		}
	}

	// For reasons unknown, we sometimes don't see the files under the cryptohome
	// when we do GetCrashes. Retry several times to avoid brief flakes.
	// TODO(crbug.com/1139494) Find root cause and remove this workaround.
	testing.ContextLog(ctx, "Scanning crash directories")
	// NOTE: in some tests (particularly ChromeCrashLoop), we don't know if the
	// files will be written out or not, so we can't just loop forever looking
	// for the files.
	const timeout = 5 * time.Second
	opts := []crash.WaitForCrashFilesOpt{crash.Timeout(timeout)}
	var regexes []string
	switch ct.waitFor {
	case NoCrashFile:
		// We don't expect to find anything; we go through the WaitForCrashFiles
		// loop once just to make sure there aren't unexpected files being written
		// out.
		opts = append(opts, crash.OptionalRegexes([]string{".*"}))
	case BreakpadDmp:
		// Filename doesn't include the PID, so just accept any dmp file as the
		// expected one. (The problems we've seen have always been the entire
		// directory not being present, so this should be OK)
		regexes = []string{breakpadDmpFileRegexp}
	case MetaFile:
		meta := fmt.Sprintf(chromeCrashFilePatternWithPid+"meta", ct.killedPID)
		dmp := fmt.Sprintf(chromeCrashFilePatternWithPid+"dmp", ct.killedPID)
		regexes = []string{meta, dmp}
		// Logs and i915_error_state may be skipped if the dmp file is too large,
		// and we don't have control of the dmp file size, so leave them as
		// optional. Also, some platforms don't have an i915_error_state, so we
		// can't block on that.
		logs := fmt.Sprintf(chromeCrashFilePatternWithPid+"chrome.txt.gz", ct.killedPID)
		i915 := fmt.Sprintf(chromeCrashFilePatternWithPid+"i915_error_state.log.xz", ct.killedPID)
		opts = append(opts, crash.OptionalRegexes([]string{logs, i915}))
		if ct.browserType == browser.TypeAsh {
			opts = append(opts, crash.MetaString("upload_var_prod=Chrome_ChromeOS"))
		} else {
			opts = append(opts, crash.MetaString("upload_var_prod=Chrome_Lacros"))
		}
	}

	var matches map[string][]string
	if matches, err = crash.WaitForCrashFiles(ctx, dirs, regexes, opts...); err != nil {
		notFoundErr, ok := err.(crash.RegexesNotFound)
		if !ok {
			return nil, errors.Wrap(err, "WaitForCrashFiles failed")
		}
		for _, expectedFiles := range notFoundErr.PartialMatches {
			deleteFiles(ctx, expectedFiles)
		}
		// Some tests (like ChromeCrashLoop) don't know in advance if they will get
		// crash files. Don't return an error if we didn't find the crash files;
		// instead, return the list of files we found to the caller and let them
		// decide if it was an error.
		return notFoundErr.Files, nil
	}
	var files []string
	for _, fileList := range matches {
		files = append(files, fileList...)
	}
	defer deleteFiles(ctx, files)

	// Check that the .meta file has the correct computed_severity and computed_product values.
	if ct.waitFor == MetaFile {
		metaFile, fileErr := getMetaFilename(matches, fmt.Sprintf(chromeCrashFilePatternWithPid+"meta", ct.killedPID))
		if fileErr != nil {
			return nil, errors.Wrap(fileErr, "failed to get meta file")
		}
		dmpFile, fileErr := getDmpFilename(matches, fmt.Sprintf(chromeCrashFilePatternWithPid+"dmp", ct.killedPID))
		if fileErr != nil {
			return nil, errors.Wrap(fileErr, "failed to get .dmp file")
		}

		if validateErr := ct.validateComputedSeverity(ctx, metaFile); validateErr != nil {
			if outDir, outDirExists := testing.ContextOutDir(ctx); outDirExists {
				if moveErr := crash.MoveFilesToOut(ctx, outDir, metaFile); moveErr != nil {
					testing.ContextLog(ctx, "Failed to save the meta file: ", moveErr)
				}
				// The dmp file contains the CrashKeys records as well, so it is useful
				// for debugging missing CrashKeys.
				if moveErr := crash.MoveFilesToOut(ctx, outDir, dmpFile); moveErr != nil {
					testing.ContextLog(ctx, "Failed to save the .dmp file: ", moveErr)
				}
			}
			return nil, errors.Wrap(validateErr, "failed to validate meta file severity")
		}

		if validateErr := ct.validateBuildTime(ctx, metaFile); validateErr != nil {
			if outDir, outDirExists := testing.ContextOutDir(ctx); outDirExists {
				if moveErr := crash.MoveFilesToOut(ctx, outDir, metaFile); moveErr != nil {
					testing.ContextLog(ctx, "Failed to save the meta file: ", moveErr)
				}
				// The dmp file contains the CrashKeys records as well, so it is useful
				// for debugging missing CrashKeys.
				if moveErr := crash.MoveFilesToOut(ctx, outDir, dmpFile); moveErr != nil {
					testing.ContextLog(ctx, "Failed to save the .dmp file: ", moveErr)
				}
			}
			return nil, errors.Wrap(validateErr, "failed to validate build time in meta file")
		}
	}
	return files, nil
}

// KillCrashpad kills all chrome_crashpad_handler processes running
// in the system (for either ash-Chrome or Lacros, depending on ct.browser). It
// returns when there are no more chrome_crashpad_handler processes running for
// the indicated browser type.
func (ct *CrashTester) KillCrashpad(ctx context.Context) error {
	testing.ContextLog(ctx, "Hunting and killing chrome_crashpad_handler processes")
	return testing.Poll(ctx, func(ctx context.Context) error {
		processes, err := ct.crashpadHandlerProcesses()
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "could not get list of processes"))
		}
		if len(processes) == 0 {
			return nil
		}

		for _, process := range processes {
			testing.ContextLog(ctx, "Sending SIGKILL to chrome_crashpad_handler process ", process.Pid)
			if err = unix.Kill(int(process.Pid), unix.SIGKILL); err != nil {
				// Mostly ignore the error. If a process exited, we want to keep going.
				testing.ContextLog(ctx, "Failed to kill chrome_crashpad_handler process: ", err, "; continuing anyways")
			}
		}

		return errors.New("some chrome_crashpad_handler processes were still alive; rescanning to ensure all have died before continuing")
	}, nil)
}
