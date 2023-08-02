// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvsutils

import (
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
	Env         PVSRuntimeEnv
}

// PVSRuntimeEnv represents the environment variables that will be set during
// an execution of the PVS CLI.
type PVSRuntimeEnv struct {
	ReuseTLEDir         string
	ForceDlmSkuID       string
	SimulatedDut        bool
	SimulatedTestRunner bool
	SimulatedDutInfo    string
}

// EnsurePass runs the given subtest and fatally errors if it fails.
func EnsurePass(ctx context.Context, s *testing.State, subtestName string, subtest func(context.Context, *testing.State)) {
	if result := s.Run(ctx, subtestName, subtest); !result {
		s.Fatal()
	}
}

// RunPVSCommand runs the given pvs command with the given options and fatally
// errors if a no exit error is thrown, otherwise the combined Stdout and Stderr
// are returned.
func (p PVSRunner) RunPVSCommand(ctx context.Context, s *testing.State, subcommand string, options ...string) string {
	output, err := p.RunPVSCommandNonfatal(ctx, subcommand, options...)
	exitErr := &ssherrs.ExitError{}
	if err != nil && !errors.As(err, &exitErr) {
		s.Fatal("Non exit-code error received when running pvs: ", err)
	}
	return output
}

// RunPVSCommandNonfatal runs the given pvs command with the given options and
// returns the corresponding error and combined Stdout and Stderr.
func (p PVSRunner) RunPVSCommandNonfatal(ctx context.Context, subcommand string, options ...string) (string, error) {
	env := p.Env.generateEnvMap()
	var envArgs []string
	for key, val := range env {
		envArgs = append(envArgs, fmt.Sprintf("-e %v=%v", key, val))
	}
	pvsCommand := fmt.Sprintf(
		`docker exec %v %q /usr/bin/gosu pvs pvs %v %v`,
		strings.Join(envArgs, " "),
		p.ContainerID,
		subcommand,
		strings.Join(options, " "),
	)
	output, err := RunAsChronos(ctx, p.Dut, pvsCommand)
	return processControlChars(output), err
}

func (p PVSRuntimeEnv) generateEnvMap() map[string]string {
	env := make(map[string]string)
	if p.ReuseTLEDir != "" {
		env["REUSE_TLE_DIR"] = p.ReuseTLEDir
	}
	if p.ForceDlmSkuID != "" {
		env["FORCE_DLM_SKU_ID"] = p.ForceDlmSkuID
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
func RunAsChronos(ctx context.Context, dut *ssh.Conn, cmd string) (string, error) {
	wrappedCmd := dut.CommandContext(ctx, "sudo", "--login", "-u", "chronos", "bash", "-c", cmd)
	return runAsRoot(ctx, wrappedCmd)
}

func runAsRoot(ctx context.Context, cmd *ssh.Cmd) (string, error) {
	testing.ContextLogf(ctx, "Running command: `%v`", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	testing.ContextLog(ctx, "Output from command: ", string(out))
	return string(out), err
}

func removeAsRoot(ctx context.Context, dut *ssh.Conn, path string) (string, error) {
	cmd := dut.CommandContext(ctx, "sudo", "rm", "-rf", path)
	return runAsRoot(ctx, cmd)
}

func runAsChronosWithStdin(ctx context.Context, dut *ssh.Conn, cmd, stdin string) (string, error) {
	wrappedCmd := dut.CommandContext(ctx, "sudo", "--login", "-u", "chronos", "bash", "-c", cmd)
	wrappedCmd.Stdin = strings.NewReader(stdin)
	return runAsRoot(ctx, wrappedCmd)
}

func writeToFileAsChronos(ctx context.Context, dut *ssh.Conn, content, path string) (string, error) {
	writeToFile := fmt.Sprintf(`cat > %v`, path)
	return runAsChronosWithStdin(ctx, dut, writeToFile, content)
}
