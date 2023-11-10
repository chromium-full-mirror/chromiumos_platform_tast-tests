// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvsutils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	ssherrs "golang.org/x/crypto/ssh"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// Test verdicts
const (
	// PrintedResultFail is the string identifier for failed tests.
	PrintedResultFail = "FAIL"
	// PrintedResultError is the string identifier for errored tests.
	PrintedResultError = "ERROR"
	// PrintedResultPass is the string identifier for passing tests.
	PrintedResultPass = "PASS"
	// PrintedResultNotRun is the string identifier for tests not run.
	PrintedResultNotRun = "NOT RUN"
	// PrintedResultNA is the string identifier for tests with no results.
	PrintedResultNA = "TEST NA"
)

// Requirement levels
const (
	// PrintedResultNotRun is the string identifier for MUST requirements.
	RequirementLevelMust = "\\(MUST\\)"
	// RequirementLevelMay is the string identifier for MAY requirements.
	RequirementLevelMay = "\\(MAY\\)"
	// RequirementLevelShould is the string identifier for SHOULD requirements.
	RequirementLevelShould = "\\(SHOULD\\)"
)

// Misc
const (
	TmpReuseTLEDir = "/tmp/pvs_tool_tle"
)

// RepeatedWord represents a regex and the number of times that regex is
// expected in some string.
type RepeatedWord struct {
	Pattern string
	Count   int
}

// PVSRunner represents a pvs runtime environment.
type PVSRunner struct {
	Dut         *ssh.Conn
	ContainerID string
	Env         RuntimeEnv
}

// RuntimeEnv represents the environment variables that will be set during
// an execution of the SHoP and PVS CLI's.
type RuntimeEnv struct {
	ReuseTLEDir             string
	SimulatedDut            bool
	SimulatedTestRunner     bool
	SimulatedDutInfo        string
	SimulatedTestsSkip      string
	SimulatedTestsError     string
	SimulatedTestsNoResult  string
	SimulatedTestsFail      string
	SimulatedTestKeyval     string
	ForceDlmSkuID           string
	PvsImageTag             string
	ShopRef                 string
	IgnoreGitAccessFailures bool
	SkipPvsVersionValidation bool
}

// EnsurePass runs the given subtest and fatally errors if it fails.
func EnsurePass(ctx context.Context, s *testing.State, subtestName string, subtest func(context.Context, *testing.State)) {
	if result := s.Run(ctx, subtestName, subtest); !result {
		s.Fatal()
	}
}

// RunPVSCommand runs the given pvs command with the given options and fatally
// errors if a no exit error is thrown, otherwise  stdout, stderr is returned.
func (p PVSRunner) RunPVSCommand(ctx context.Context, s *testing.State, subcommand string, options ...string) (string, string) {
	stdout, stderr, err := p.RunPVSCommandNonfatal(ctx, subcommand, options...)
	exitErr := &ssherrs.ExitError{}
	if err != nil && !errors.As(err, &exitErr) {
		s.Fatal("Non exit-code error received when running pvs: ", err)
	}
	return stdout, stderr
}

// RunPVSCommandNonfatal runs the given pvs command with the given options and
// returns the corresponding error and stdout, stderr.
func (p PVSRunner) RunPVSCommandNonfatal(ctx context.Context, subcommand string, options ...string) (string, string, error) {
	pvsCommand := fmt.Sprintf(`pvs %v %v`, subcommand, strings.Join(options, " "))
	return p.RunCommandInContainer(ctx, pvsCommand)
}

// RunCommandInContainer runs the given command in the pvs container and returns
// the corresponding error and stdout, stderr.
func (p PVSRunner) RunCommandInContainer(ctx context.Context, command string) (string, string, error) {
	env := p.Env.generateEnvMap()
	var envArgs []string
	for key, val := range env {
		envArgs = append(envArgs, fmt.Sprintf("-e %v=%v", key, val))
	}
	dockerExec := fmt.Sprintf(
		`docker exec %v %q /usr/bin/gosu pvs %v`,
		strings.Join(envArgs, " "),
		p.ContainerID,
		command,
	)
	stdout, stderr, err := RunAsChronos(ctx, p.Dut, dockerExec)
	return processControlChars(stdout), stderr, err
}

func (p RuntimeEnv) generateEnvMap() map[string]string {
	env := make(map[string]string)
	if p.ReuseTLEDir != "" {
		env["REUSE_TLE_DIR"] = p.ReuseTLEDir
	}
	if p.SimulatedDut {
		env["SIMULATED_DUT"] = "1"
	}
	if p.SimulatedTestRunner {
		env["SIMULATED_TEST_RUNNER"] = "1"
	}
	if p.SimulatedDutInfo != "" {
		env["SIMULATED_DUT_INFO"] = p.SimulatedDutInfo
	}
	if p.SimulatedTestsSkip != "" {
		env["SIMULATED_TESTS_SKIP"] = p.SimulatedTestsSkip
	}
	if p.SimulatedTestsError != "" {
		env["SIMULATED_TESTS_ERROR"] = p.SimulatedTestsError
	}
	if p.SimulatedTestsNoResult != "" {
		env["SIMULATED_TESTS_NO_RESULT"] = p.SimulatedTestsNoResult
	}
	if p.SimulatedTestsFail != "" {
		env["SIMULATED_TESTS_FAIL"] = p.SimulatedTestsFail
	}
	if p.SimulatedTestKeyval != "" {
		env["SIMULATED_TEST_KEYVAL"] = p.SimulatedTestKeyval
	}
	if p.ForceDlmSkuID != "" {
		env["FORCE_DLM_SKU_ID"] = p.ForceDlmSkuID
	}
	if p.PvsImageTag != "" {
		env["PVS_IMAGE_TAG"] = p.PvsImageTag
	}
	if p.ShopRef != "" {
		env["SHOP_REF"] = p.ShopRef
	}
	if p.IgnoreGitAccessFailures {
		env["IGNORE_GIT_ACCESS_FAILURES"] = "1"
	}
	if p.SkipPvsVersionValidation {
		env["SKIP_PVS_VERSION_VALIDATION"] = "1"
	}
	return env
}

func processControlChars(output string) string {
	const deletePreviousLineControl = "\033[A\033[2K"
	var b strings.Builder
	previous := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.Index(line, deletePreviousLineControl) == 0 {
			previous = ""
			line = strings.Replace(line, deletePreviousLineControl, "", 1)
		}
		if len(previous) > 0 {
			b.WriteString(previous)
			b.WriteString("\n")
		}
		previous = line
	}
	b.WriteString(previous)
	return b.String()
}

// sanitize removes the control characters that are added to stout and sterr for
// some commands
func sanitize(output string) string {
	// Remove the control characters that are generated from the sudo command
	// called via RunAsChronos
	santizedOutput := strings.TrimPrefix(output, string([]rune("\x1b[?25h\x1b[?0c")))
	// Remove the newline character that is appended to the end of stout
	// but is not actually part of the stdout of the command
	santizedOutput = strings.TrimSuffix(santizedOutput, string([]rune("\n")))
	return santizedOutput
}

// ValidateOutputContains throws a testing error if the given output string
// does not contains all the wants strings.
func ValidateOutputContains(s *testing.State, output string, wants []string) {
	for _, want := range wants {
		if !strings.Contains(output, want) {
			s.Errorf("Expected %q in command output, but not found", want)
		}
	}
}

// TestResultPattern combines the test name and results string to create the PVS
// output string for a test result.
func TestResultPattern(testName, testResult string) string {
	return fmt.Sprintf("%s.*%s", testName, testResult)
}

// CountTestCases ensures the output contains all the patterns the specified
// number of times.
func CountTestCases(s *testing.State, output string, wants []RepeatedWord) {
	for _, want := range wants {
		countTestCase(s, output, want)
	}
}

func countTestCase(s *testing.State, output string, want RepeatedWord) {
	findWord := regexp.MustCompile(want.Pattern)
	matches := findWord.FindAllString(output, -1)
	actualCount := len(matches)

	if actualCount != want.Count {
		s.Errorf("Expected to find %q matches %v times in command output, but found %v times", want.Pattern, want.Count, actualCount)
	}
}

// copyDirToTastOutputDir copies the given path on the dut to the tast output
// directory on the host
func copyDirToTastOutputDir(ctx context.Context, dutfsClient *dutfs.Client, fromPath string) error {
	if _, err := dutfsClient.Stat(ctx, fromPath); os.IsNotExist(err) {
		// If the directory doesn't exist log warning but don't fail
		testing.ContextLogf(ctx, "WARNING: %v does not exist; unable to copy directory to tast logs", fromPath)
		return nil
	} else if err != nil {
		return err
	}
	tastOutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("no tast output directory found")
	}
	destDir := path.Join(tastOutDir, path.Base(fromPath))
	return copyDirFromDut(ctx, dutfsClient, fromPath, destDir)
}

// copyDirFromDut copies the given directory on the dut to the given path on the
// the host machine
func copyDirFromDut(ctx context.Context, dutfsClient *dutfs.Client, fromPath, toPath string) error {
	if err := os.MkdirAll(toPath, 0755); err != nil {
		return err
	}
	files, err := dutfsClient.ReadDir(ctx, fromPath)
	if err != nil {
		return err
	}
	for _, file := range files {
		newFromPath := path.Join(fromPath, file.Name())
		newToPath := path.Join(toPath, file.Name())
		if file.IsDir() {
			err = copyDirFromDut(ctx, dutfsClient, newFromPath, newToPath)
		} else {
			err = copyFileFromDut(ctx, dutfsClient, newFromPath, newToPath)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// copyFileFromDut copies the given file on the dut to the given path on the
// the host machine
func copyFileFromDut(ctx context.Context, dutfsClient *dutfs.Client, fromPath, toPath string) error {
	bytes, err := dutfsClient.ReadFile(ctx, fromPath)
	if err != nil {
		return err
	}
	return os.WriteFile(toPath, bytes, 0644)
}

// CopyToPVSOutputDir copies the given file to the dut and places it in the PVS output directory;
// the path to this file from the context of the container is returned.
func CopyToPVSOutputDir(ctx context.Context, s *testing.State, filepath string) string {
	data, err := os.ReadFile(filepath)
	if err != nil {
		s.Fatal("Error when reading file: ", err)
	}

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the local gRPC service on the DUT: ", err)
	}
	defer rpcClient.Close(ctx)
	dutfsClient := dutfs.NewClient(rpcClient.Conn)

	outputPath := path.Join(pvsOutputDir, path.Base(filepath))
	if err := dutfsClient.WriteFile(ctx, outputPath, data, 0644); err != nil {
		s.Fatal("Error when writing to file: ", err)
	}
	return path.Join(containerPVSOutputDir, path.Base(filepath))
}

// RunAsChronos runs the given command as the chronos user on the given dut.
func RunAsChronos(ctx context.Context, dut *ssh.Conn, cmd string) (string, string, error) {
	wrappedCmd := dut.CommandContext(ctx, "sudo", "--login", "-u", "chronos", "bash", "-c", cmd)
	return runAsRoot(ctx, wrappedCmd)
}

// RunAsRoot runs the given command as the root user on the given dut.
func RunAsRoot(ctx context.Context, dut *ssh.Conn, cmd string) (string, string, error) {
	cmdList := strings.Split(cmd, " ")
	if len(cmdList) == 0 {
		return "", "", errors.New("cannot execute an empty command")
	}
	rootCmd := dut.CommandContext(ctx, cmdList[0], cmdList[1:]...)
	return runAsRoot(ctx, rootCmd)
}

func runAsRoot(ctx context.Context, cmd *ssh.Cmd) (string, string, error) {
	var bstdout, bstderr bytes.Buffer
	cmd.Stdout = &bstdout
	cmd.Stderr = &bstderr
	err := cmd.Run()
	stdout := sanitize(string(bstdout.Bytes()))
	stderr := sanitize(string(bstderr.Bytes()))
	testing.ContextLog(ctx, "Stdout: ", stdout)
	testing.ContextLog(ctx, "Stderr: ", stderr)
	return stdout, stderr, err
}

func removeAsRoot(ctx context.Context, dut *ssh.Conn, path string) (string, string, error) {
	cmd := dut.CommandContext(ctx, "sudo", "rm", "-rf", path)
	return runAsRoot(ctx, cmd)
}

func runAsChronosWithStdin(ctx context.Context, dut *ssh.Conn, cmd, stdin string) (string, string, error) {
	wrappedCmd := dut.CommandContext(ctx, "sudo", "--login", "-u", "chronos", "bash", "-c", cmd)
	wrappedCmd.Stdin = strings.NewReader(stdin)
	return runAsRoot(ctx, wrappedCmd)
}

func writeToFileAsChronos(ctx context.Context, dut *ssh.Conn, content, path string) (string, string, error) {
	writeToFile := fmt.Sprintf(`cat > %v`, path)
	return runAsChronosWithStdin(ctx, dut, writeToFile, content)
}
