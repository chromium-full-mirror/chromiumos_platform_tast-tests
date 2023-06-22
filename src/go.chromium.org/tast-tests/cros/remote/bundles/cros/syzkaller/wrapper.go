// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package syzkaller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// syzkallerRunDuration represents the overall run duration of the fuzzer.
	syzkallerRunDuration = 50 * time.Minute

	// defaultProcs is the number of syz-executor instances running on the DUT.
	defaultProcs = 5

	// gsURL points to the GCS bucket for syzkaller artifacts.
	gsURL = "gs://syzkaller-ctp-corpus"

	// syzUnknownEnabled is an error string to look out for in the fuzzer run logs.
	syzUnknownEnabled = "unknown enabled syscall"

	// syzNewInput is an informational string indicating that a new fuzz input was
	// found.
	syzNewInput = "new input from vm-0 for syscall "

	// syzCorpusCorrupt is an informational string indicating that a corrupt corpus
	// database was found.
	syzCorpusCorrupt = "failed to deserialize database header"

	// syzSeedProcessed indicates that the seed corpus has been fully processed.
	syzSeedProcessed = "triageQLen 0"
)

// A global runtime variable to indicate the test is running locally.
var isLocal = testing.RegisterVarString(
	"syzkaller.isLocal",
	"false",
	"A variable to indicate the test is running locally.",
)

const startupScriptContents = `
sysctl -w kernel.panic_on_warn=%v
dmesg --clear
`

var driversToModprobe = [...]string{
	"bluetooth",
	"bridge",
	"cdc_ether",
	"fuse",
	"hci_vhci",
	"ip6table_nat",
	"r8153_ecm",
	"rfcomm",
	"tun",
	"uinput",
	"usbnet",
	"veth",
	"xfrm_interface",
	"xt_cgroup",
	"xt_MASQUERADE",
	"zram",
}

const (
	arm   = "arm"
	arm64 = "arm64"
	amd64 = "amd64"
)

type targetArch struct {
	user   string
	kernel string
}

var syzArchMapping = map[string]string{
	"x86_64":  amd64,
	"aarch64": arm64,
}

// dutConfig represents information related to the DUT configuration;
// commands that need to be executed before each fuzzing
// iteration, directory from which to execute syz-executor, whether
// or not to perform a reboot after reading pstore contents.
type dutConfig struct {
	Targets       []string `json:"targets"`
	TargetDir     string   `json:"target_dir"`
	TargetReboot  bool     `json:"target_reboot"`
	StartupScript string   `json:"startup_script"`
	Pstore        bool     `json:"pstore"`
}

type syzkallerConfig struct {
	Name           string    `json:"name"`
	HubClient      string    `json:"hub_client"`
	HubAddr        string    `json:"hub_addr"`
	HubKey         string    `json:"hub_key"`
	Target         string    `json:"target"`
	Reproduce      bool      `json:"reproduce"`
	HTTP           string    `json:"http"`
	Workdir        string    `json:"workdir"`
	Syzkaller      string    `json:"syzkaller"`
	Type           string    `json:"type"`
	SSHKey         string    `json:"sshkey"`
	Procs          int       `json:"procs"`
	DUTConfig      dutConfig `json:"vm"`
	EnableSyscalls []string  `json:"enable_syscalls"`
}

type fuzzEnvConfig struct {
	// Driver or subsystem.
	Driver string `json:"driver"`
	// Boards specifies the boards to run a set of syscalls on. Boards can
	// be empty.
	Boards []string `json:"boards"`
	// ExcludeBoards specifies boards on which to not fuzz certain syscalls. ExcludeBoards
	// can be empty.
	ExcludeBoards []string `json:"exclude_boards"`
	// Startup commands specific to this subsystem.
	StartupCmds []string `json:"startup_cmds"`
	// Syscalls belonging to the driver or subsystem.
	Syscalls []string `json:"syscalls"`
}

type periodicConfig struct {
	// Board against which Cmd need to be run periodically.
	Board string `json:"board"`
	// Periodicity specifies in seconds how often Cmd should run against
	// the DUT.
	Periodicity int `json:"periodicity"`
	// Cmd to run against the DUT while fuzzing.
	Cmd string `json:"cmd"`
}

const (
	enabledSyscallsBrya    string = "enabled_syscalls_brya.json"
	enabledSyscallsNonBrya string = "enabled_syscalls.json"

	syzManagerHost string = "localhost"
	syzManagerPort int    = 56701
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Wrapper,
		Desc:     "Wrapper test that runs Syzkaller",
		Contacts: []string{"chromeos-kernel@google.com", "zsm@chromium.org"},
		// ChromeOS > Platform > System > Kernel > Syzkaller > Syzkaller-Dev > CTP-Infra
		BugComponent: "b:1047538",
		SoftwareDeps: []string{"pstore", "reboot"},
		// This wrapper runs syzkaller against the DUT for a duration of 50 minutes before
		// stopping. The overall test duration is 60 minutes. Note that this test does not
		// run on CQ, only on postsubmit.
		Timeout: syzkallerRunDuration + 10*time.Minute,
		Attr:    []string{"group:syzkaller"},
		Data:    []string{"testing_rsa", "periodic.json"},
		VarDeps: []string{"syzkaller.Wrapper.botoCredSection"},
		Params: []testing.Param{
			{
				// This testcase should only run on brya devices.
				Name:              "brya_cellular",
				Val:               enabledSyscallsBrya,
				ExtraData:         []string{enabledSyscallsBrya},
				ExtraHardwareDeps: hwdep.D(hwdep.Cellular(), hwdep.Model(bryaModels()...)),
			},
			{
				// This testcase should only run on non-brya devices.
				Name:              "non_brya",
				Val:               enabledSyscallsNonBrya,
				ExtraData:         []string{enabledSyscallsNonBrya},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(bryaModels()...)),
			},
		},
	})
}

func bryaModels() []string {
	return []string{
		"brya",
		"anahera",
		"banshee",
		"crota360",
		"felwinter",
		"gimble",
		"kano",
		"primus",
		"redrix",
		"taeko",
		"taeland",
		"taniks",
		"tarlo",
		"vell",
		"volmar",
		"zavala",
	}
}

var boardProcs = map[string]int{
	"strongbad": 10,
	"trogdor":   8,
}

// Wrapper runs Syzkaller against DUTs with KASAN and KCOV enabled.
func Wrapper(ctx context.Context, s *testing.State) {
	d := s.DUT()
	runLocal := strings.ToLower(isLocal.Value()) == "true"

	board, err := findBoard(ctx, d)
	if err != nil {
		s.Fatal("Unable to find board: ", err)
	}
	s.Log("Board found to be: ", board)

	syzArch, err := findSyzArch(ctx, d, board)
	if err != nil {
		s.Fatal("Unable to find syzkaller arch: ", err)
	}
	s.Logf("syzArch found to be: (%v/%v)", syzArch.user, syzArch.kernel)

	kernelCommit, err := findKernelCommit(ctx, d)
	if err != nil {
		s.Fatal("Unable to find kernel commit: ", err)
	}
	s.Log("kernelCommit found to be: ", kernelCommit)

	syzkallerTastDir, err := ioutil.TempDir("", "tast-syzkaller")
	if err != nil {
		s.Fatal("Unable to create tast temporary directory: ", err)
	}
	defer os.RemoveAll(syzkallerTastDir)

	artifactsDir := filepath.Join(syzkallerTastDir, "artifacts")
	if err := os.Mkdir(artifactsDir, 0755); err != nil {
		s.Fatal("Unable to create temp artifacts dir: ", err)
	}

	// Fetch syz-* binaries. Run syzkaller without vmlinux.
	if err := fetchFuzzArtifacts(ctx, d, artifactsDir, syzArch); err != nil {
		s.Fatal("Encountered error fetching fuzz artifacts: ", err)
	}

	// Create a syzkaller working directory.
	syzkallerWorkdir := filepath.Join(syzkallerTastDir, "workdir")
	if err := os.Mkdir(syzkallerWorkdir, 0755); err != nil {
		s.Fatal("Unable to create temp workdir: ", err)
	}
	if !runLocal {
		if err := loadCorpus(
			ctx,
			s.RequiredVar("syzkaller.Wrapper.botoCredSection"),
			board,
			syzkallerWorkdir,
		); err != nil {
			s.Fatal("Unable to load corpus: ", err)
		}
	}

	// TODO(b/260624085):
	// Debugging block. Remove after diagnozing issue. Ensure ssh to DUT works without
	// specifying the key.
	{
		s.Log("KeyFile: ", d.KeyFile())
		s.Log("KeyDir: ", d.KeyDir())
		if _, err := os.Stat(filepath.Join(d.KeyDir(), "config")); err != nil {
			if os.IsNotExist(err) {
				s.Log("Config file does not exist")
			} else {
				s.Log("Unexpected error stat config file: ", err)
			}
		}
		hostname, port := d.HostName(), "22"
		parts := strings.Split(d.HostName(), ":")
		if len(parts) == 2 {
			hostname, port = parts[0], parts[1]
		}
		out, err := exec.Command("ssh", "-p", port, "root@"+hostname, "ls").CombinedOutput()
		if err != nil {
			s.Logf("SSH [%v] failed: %v: %v", d.HostName(), err, string(out))
		}
		s.Logf("SSH [%v] success: %v", d.HostName(), string(out))
	}

	param := s.Param().(string)
	s.Log("Loading enabled syscalls from: ", param)

	// Read enabled_syscalls.
	drivers, enabledSyscalls, scriptContents, err := loadEnabledSyscalls(s.DataPath(param), board)
	if err != nil {
		s.Fatal("Unable to load enabled syscalls: ", err)
	}
	s.Log("Drivers: ", drivers)
	s.Log("Enabled syscalls: ", enabledSyscalls)
	s.Log("Script contents: ", scriptContents)

	// Load periodic commands.
	pCmd, err := loadPeriodic(s.DataPath("periodic.json"), board)
	if err != nil {
		s.Fatal("Unable to load periodic cmds: ", err)
	}
	s.Log("Periodic cmds: ", pCmd)

	// Create startup script.
	startupScript := filepath.Join(syzkallerTastDir, "startup_script")
	if err := ioutil.WriteFile(startupScript, []byte(scriptContents), 0755); err != nil {
		s.Fatal("Unable to create temp configfile: ", err)
	}

	target := fmt.Sprintf("linux/%v/%v", syzArch.kernel, syzArch.user)
	if syzArch.kernel == syzArch.user {
		target = fmt.Sprintf("linux/%v", syzArch.kernel)
	}
	s.Log("Target: ", target)

	procs := defaultProcs
	if p, ok := boardProcs[board]; ok {
		procs = p
	}
	s.Log("Using procs: ", procs)

	// Create syzkaller configuration file.
	// Generating reproducers is unlikely to work as :
	// [1] Corpus is not shared across two runs of the test.
	// [2] A test is run for a short duration(30 minutes).
	// Hence, set Reproduce:false.
	config := syzkallerConfig{
		Name:      board,
		Target:    target,
		Reproduce: false,
		HTTP:      fmt.Sprintf("%v:%v", syzManagerHost, syzManagerPort),
		Workdir:   syzkallerWorkdir,
		Syzkaller: artifactsDir,
		Type:      "isolated",
		Procs:     procs,
		DUTConfig: dutConfig{
			Targets:       []string{d.HostName()},
			TargetDir:     "/usr/local/tmp",
			TargetReboot:  false,
			StartupScript: startupScript,
			Pstore:        true,
		},
		EnableSyscalls: enabledSyscalls,
	}
	if runLocal {
		config.SSHKey = s.DataPath("testing_rsa")
	}

	configFile, err := os.Create(filepath.Join(syzkallerTastDir, "config"))
	if err != nil {
		s.Fatal("Unable to create syzkaller configfile: ", err)
	}
	defer configFile.Close()

	if err := json.NewEncoder(configFile).Encode(config); err != nil {
		s.Fatal("Invalid syzkaller configuration: ", err)
	}

	logFile, err := os.Create(filepath.Join(syzkallerTastDir, "logfile"))
	if err != nil {
		s.Fatal("Unable to create temp logfile: ", err)
	}
	defer logFile.Close()

	// Ensure that system logs(related to tests that might have run earlier)
	// are flushed to disk.
	rcmd := d.Conn().CommandContext(ctx, "sync")
	if err := rcmd.Run(); err != nil {
		s.Fatal("Unable to flush cached content to disk: ", err)
	}

	s.Log("Starting syzkaller with logfile at ", logFile.Name())
	syzManager := filepath.Join(artifactsDir, "syz-manager")
	cmdArgs := []string{"-config", configFile.Name(), "-vv", "10"}
	managerCmd := testexec.CommandContext(ctx, syzManager, cmdArgs...)
	managerCmd.Stdout = logFile
	managerCmd.Stderr = logFile

	if err := managerCmd.Start(); err != nil {
		s.Fatal("Running syz-manager failed: ", err)
	}

	defer func() {
		// Copy the syzkaller stdout/stderr logfile and the working directory
		// as part of the tast results directory. Set this action as deferred so
		// that it is performed even if the test fails with a call to s.Fatal().
		tastResultsDir := s.OutDir()
		s.Log("Copying syzkaller workdir to tast results directory")
		if err := exec.Command("cp", "-r", syzkallerWorkdir, tastResultsDir).Run(); err != nil {
			s.Fatal("Failed to copy syzkaller workdir: ", err)
		}
		s.Log("Copying syzkaller logfile to tast results directory")
		if err := exec.Command("cp", logFile.Name(), tastResultsDir).Run(); err != nil {
			s.Fatal("Failed to copy syzkaller logfile: ", err)
		}
		s.Log("Copying syzkaller config to tast results directory")
		if err := exec.Command("cp", configFile.Name(), tastResultsDir).Run(); err != nil {
			s.Fatal("Failed to copy syzkaller config: ", err)
		}
	}()

	done := make(chan bool)
	if pCmd != nil {
		go runPeriodic(ctx, d, done, pCmd)
	}

	// Gracefully shut down syzkaller.
	func() {
		defer func() {
			managerCmd.Process.Signal(os.Interrupt)
			managerCmd.Wait()
		}()

		// GoBigSleepLint: Sleep for run duration while syz-manager runs indefinitely.
		if err := testing.Sleep(ctx, syzkallerRunDuration); err != nil {
			managerCmd.Kill()
			s.Fatal("Failed to wait on syz-manager: ", err)
		}

		// Fetch coverage and modules info from syz-manager before stopping syz-manager.
		// Upload coverage only if the test is not running locally.
		if err := saveCoverage(
			ctx,
			s.RequiredVar("syzkaller.Wrapper.botoCredSection"),
			s.OutDir(),
			board,
			kernelCommit,
			!runLocal,
		); err != nil {
			s.Fatal("Failed to upload coverage info: ", err)
		}
	}()

	if pCmd != nil {
		done <- true
	}

	if err := logValidity(logFile.Name()); err != nil {
		s.Fatal("Error found in logfile: ", err)
	}

	if !runLocal {
		if err := saveCorpus(
			ctx,
			s.RequiredVar("syzkaller.Wrapper.botoCredSection"),
			board,
			filepath.Join(syzkallerWorkdir, "corpus.db"),
		); err != nil {
			s.Fatal("Failed to save corpus: ", err)
		}
	}

	s.Log("Done fuzzing, exiting")
}

func gsutilCmd(ctx context.Context, cred string, args ...string) *testexec.Cmd {
	gsutilArgs := append([]string{"-o", cred}, args...)
	cmd := testexec.CommandContext(ctx, "gsutil", gsutilArgs...)
	cmd.Env = append(os.Environ(), "BOTO_CONFIG= ")
	return cmd
}

// loadCorpus should only be used when running the test as scheduled in the lab.
func loadCorpus(ctx context.Context, cred, board, syzkallerWorkdir string) error {
	out, err := gsutilCmd(ctx, cred, "ls", gsURL).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to list corpus bucket")
	}
	objects := strings.Split(string(out), "\n")
	var url string
	for _, object := range objects {
		if strings.Contains(object, board) {
			url = object
		}
	}
	if url == "" {
		testing.ContextLog(ctx, "No pre-existing corpus found for board: ", board)
		return nil
	}
	testing.ContextLog(ctx, "Fetching ", url)
	// Note: No corpus is downloaded when running this test locally.
	if err = gsutilCmd(ctx, cred, "cp", url, filepath.Join(syzkallerWorkdir, "corpus.db")).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to fetch: %v", url)
	}
	testing.ContextLog(ctx, "Fetched ", url)
	return nil
}

// saveCorpus should only be used when running the test as scheduled in the lab.
func saveCorpus(ctx context.Context, cred, board, corpusPath string) error {
	timestamp := time.Now().Format("2006-01-02-15:04:05")
	url := fmt.Sprintf("%s/corpus-%v-%v.db", gsURL, board, timestamp)
	testing.ContextLog(ctx, "Uploading ", url)
	// Note: No corpus is uploaded when running this test locally.
	if err := gsutilCmd(ctx, cred, "copy", corpusPath, url).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to save corpus.db")
	}
	testing.ContextLog(ctx, "Uploaded ", url)
	return nil
}

// saveCoverage should only be used when running the test as scheduled in the lab.
func saveCoverage(ctx context.Context, cred, outDir, board, kernelCommit string, uploadCover bool) error {
	timestamp := time.Now().Format("2006-01-02-15:04:05")
	artifacts := []struct {
		name string
		url  string
	}{
		{
			name: fmt.Sprintf("rawcover-%v-%v-%v", board, timestamp, kernelCommit),
			url:  fmt.Sprintf("http://%v:%v/rawcover32", syzManagerHost, syzManagerPort),
		},
		{
			name: fmt.Sprintf("modules-%v-%v-%v", board, timestamp, kernelCommit),
			url:  fmt.Sprintf("http://%v:%v/modules", syzManagerHost, syzManagerPort),
		},
	}
	for _, art := range artifacts {
		var uploadURL string
		artFile := filepath.Join(outDir, art.name)

		if uploadCover {
			uploadURL = fmt.Sprintf("%s/rawcover32/%s", gsURL, art.name)
		}
		if err := saveRunArtifact(ctx, cred, artFile, art.url, uploadURL); err != nil {
			return err
		}
	}
	return nil
}

func saveRunArtifact(ctx context.Context, cred, artFile, url, uploadURL string) error {
	testing.ContextLogf(ctx, "Retrieving from [%v] to [%v]", url, artFile)
	if err := testexec.CommandContext(ctx, "wget", url, "-O", artFile).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "unable to retrieve info from [%v]", url)
	}
	if uploadURL != "" {
		// Note: No info is uploaded when running this test locally.
		testing.ContextLog(ctx, "Uploading to ", uploadURL)
		if err := gsutilCmd(ctx, cred, "copy", artFile, uploadURL).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to upload to [%v]", uploadURL)
		}
		testing.ContextLog(ctx, "Uploaded to ", uploadURL)
	}
	return nil
}

func findKernelCommit(ctx context.Context, d *dut.DUT) (string, error) {
	kernelRelease, err := d.Conn().CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to find uname")
	}
	// Release for devices with a debug kernel should look something as follows.
	// "5.10.141-lockdep-19696-gb7597b887eec".
	parts := strings.Split(strings.TrimSpace(string(kernelRelease)), "-")
	if len(parts) < 2 {
		return "", errors.Errorf("unexpected release in uname [%v]", string(kernelRelease))
	}
	commit := parts[len(parts)-1]
	if !strings.HasPrefix(commit, "g") {
		return "", errors.Errorf("unexpected commit [%v] for uname [%v]", commit, kernelRelease)
	}
	return commit[1:], nil
}

func findBoard(ctx context.Context, d *dut.DUT) (string, error) {
	board, err := reporters.New(d).Board(ctx)
	if err != nil {
		return "", errors.Wrap(err, "unable to find board")
	}
	return board, nil
}

func findSyzArch(ctx context.Context, d *dut.DUT, board string) (*targetArch, error) {
	output, err := d.Conn().CommandContext(ctx, "uname", "-m").Output()
	if err != nil {
		return nil, errors.Wrap(err, "uname -m failed")
	}
	unameArch := strings.TrimSuffix(string(output), "\n")
	karch, ok := syzArchMapping[unameArch]
	if !ok {
		return nil, errors.Errorf("unexpected unameArch: %v", unameArch)
	}
	// On non-arm64 boards, user and kernel arch are the same.
	if karch != arm64 {
		return &targetArch{karch, karch}, nil
	}
	// On arm64 boards, check the file type of /usr/local/bin/wget to determine
	// userspace arch.
	uarch := arm
	output, err = d.Conn().CommandContext(ctx, "file", "/usr/local/bin/wget").Output()
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine wget filetype")
	}
	if bytes.Contains(output, []byte("ARM aarch64")) {
		uarch = arm64
	}
	return &targetArch{uarch, karch}, nil
}

func fetchFuzzArtifacts(ctx context.Context, d *dut.DUT, artifactsDir string, syzArch *targetArch) error {
	binDirUser := fmt.Sprintf("bin/linux_%v", syzArch.user)
	binDirKern := fmt.Sprintf("bin/linux_%v", syzArch.kernel)
	for _, binDir := range []string{binDirUser, binDirKern} {
		if err := os.MkdirAll(filepath.Join(artifactsDir, binDir), 0755); err != nil {
			return err
		}
	}

	// Get syz-manager, syz-fuzzer, syz-execprog and syz-executor from the DUT image.
	if err := linuxssh.GetFile(ctx, d.Conn(), "/usr/local/bin/syz-manager", filepath.Join(artifactsDir, "syz-manager"), linuxssh.PreserveSymlinks); err != nil {
		return err
	}

	// syz-manager expects syz-executor to be at <artifactsDir>/linux_<syzArch.user>/syz-executor.
	// syz-manager expects {syz-fuzzer,syz-execprog} to be at <artifactsDir>/linux_<syzArch.kernel>/syz-{fuzzer,execprog}.
	artifacts := []struct {
		binary string
		dir    string
	}{
		{"syz-executor", binDirUser},
		{"syz-fuzzer", binDirKern},
		{"syz-execprog", binDirKern},
	}
	for _, a := range artifacts {
		src := filepath.Join("/usr/local/bin", a.binary)
		dest := filepath.Join(artifactsDir, a.dir, a.binary)
		if err := linuxssh.GetFile(ctx, d.Conn(), src, dest, linuxssh.PreserveSymlinks); err != nil {
			return err
		}
	}

	return nil
}

func loadEnabledSyscalls(fpath, board string) (drivers, enabledSyscalls []string, scriptContents string, err error) {
	contains := func(aList []string, item string) bool {
		for _, each := range aList {
			if each == item {
				return true
			}
		}
		return false
	}

	contents, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, nil, "", err
	}

	var feconfig []fuzzEnvConfig
	err = json.Unmarshal([]byte(contents), &feconfig)
	if err != nil {
		return nil, nil, "", err
	}

	panicOnWarn := time.Now().Day() % 2
	scriptContents = fmt.Sprintf(startupScriptContents, panicOnWarn)
	for _, config := range feconfig {
		if len(config.Boards) > 0 && len(config.ExcludeBoards) > 0 {
			return nil, nil, "", errors.Errorf("non-empty Boards and ExcludeBoards found for [%v]", config.Driver)
		}
		// Enable syscalls of a driver if |Boards| contains the DUT board.
		// Enable syscalls of a driver if |Boards| is empty, and the DUT board is not present in |ExcludeBoards|.
		ok := !contains(config.ExcludeBoards, board)
		if len(config.Boards) > 0 {
			ok = contains(config.Boards, board)
		}
		if ok {
			enabledSyscalls = append(enabledSyscalls, config.Syscalls...)
			drivers = append(drivers, config.Driver)
			scriptContents = scriptContents + strings.Join(config.StartupCmds, "\n") + "\n"
		}
	}
	for _, d := range driversToModprobe {
		scriptContents = scriptContents + fmt.Sprintf("modprobe %v\n", d)
	}

	return drivers, enabledSyscalls, scriptContents, nil
}

func loadPeriodic(fpath, board string) (*periodicConfig, error) {
	contents, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, err
	}
	var peconfig []*periodicConfig
	err = json.Unmarshal([]byte(contents), &peconfig)
	if err != nil {
		return nil, err
	}
	for _, config := range peconfig {
		if board == config.Board {
			return config, nil
		}
	}
	return nil, nil
}

func runPeriodic(ctx context.Context, d *dut.DUT, done chan bool, cfg *periodicConfig) {
	for {
		// Non-blocking check to see if we should stop running
		// Cmd periodically.
		select {
		case <-done:
			return
		default:
		}
		// Do not fail the test if the command fails to execute, only log.
		// Fuzzing can cause spurious device reboots.
		cmd := []string{"bash", "-c", cfg.Cmd}
		testing.ContextLog(ctx, "Going to run: ", cmd)
		if err := d.Conn().CommandContext(ctx, cmd[0], cmd[1:]...).Run(); err != nil {
			testing.ContextLogf(ctx, "Failed to run [%v]: %v", cmd, err)
		}
		// GoBigSleepLint: Poll is not used as device might reboot during fuzzing.
		testing.Sleep(ctx, time.Duration(cfg.Periodicity)*time.Second)
	}
}

func logValidity(fname string) error {
	logs, err := ioutil.ReadFile(fname)
	if err != nil {
		return errors.Wrapf(err, "unable to read logfile at [%v]", fname)
	}

	lines := strings.Split(string(logs), "\n")
	if len(lines) < 2 {
		return errors.New("logfile unexpectedly empty")
	}

	var unknown []string
	var newInpFound, corpusCorrupt, seedProcessed bool
	for _, line := range lines {
		if strings.Contains(line, syzUnknownEnabled) {
			unknown = append(unknown, line)
		}
		if strings.Contains(line, syzNewInput) {
			newInpFound = true
		}
		if strings.Contains(line, syzCorpusCorrupt) {
			corpusCorrupt = true
		}
		if strings.Contains(line, syzSeedProcessed) {
			seedProcessed = true
		}
	}
	if len(unknown) != 0 {
		return errors.Errorf("unsupported enabled syscall[s] found: [%v]", unknown)
	}
	if !newInpFound {
		return errors.New("no new input found in fuzzing run")
	}
	if corpusCorrupt {
		return errors.New("corpus found to be corrupted")
	}
	if !seedProcessed {
		return errors.New("did not finish executing the seed corpus")
	}

	return nil
}
