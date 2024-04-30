// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "enableDisablePsrFixture",
		Desc:            "Makes PSR status to DEEP_SLEEP",
		Contacts:        []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent:    "b:157291", // ChromeOS > External > Intel
		Impl:            &psr{},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 2 * time.Minute,
	})
}

type psr struct {
	psrState string
}

// SetUp will enable PSR and reboots the DUT if required.
// This fixture will enable PSR2 which is a reversible action, which
// can be cleaned in Teardown.
func (i *psr) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	d := s.DUT()
	// Attempt to connect to those DUTs that aren't already connected.
	if !d.Connected(ctx) {
		s.Log("Attempting to connect to DUT")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return d.Connect(ctx)
		}, &testing.PollOptions{Interval: time.Second, Timeout: 30 * time.Second}); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}
		s.Log("Connected to DUT")
	}

	// Connect to local gRPC services, and keep connection alive until after
	rpcClient, err := rpc.Dial(s.FixtContext(), s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the local gRPC service on the DUT: ", err)
	}
	defer func() {
		if err := rpcClient.Close(ctx); err != nil {
			s.Fatal("Failed to close gRPC connection to DUT: ", err)
		}
	}()
	client := dutfs.NewClient(rpcClient.Conn)

	psrState, err := enablePSR(ctx, d, client)
	if err != nil {
		s.Fatal("Failed to enable PSR: ", err)
	}
	i.psrState = psrState
	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	return nil
}

// editKernelArgs is a helper function for editing kernel args. Function |f|
// performs the editing action by transforming the content of saved config.
func editKernelArgs(ctx context.Context, dut *dut.DUT, client *dutfs.Client, f func([]byte) ([]byte, error)) error {
	// Save the current boot config to |prefix|.|part| (make_dev_ssd.sh saves the content to a file named |prefix|.|part|).
	prefix := "/tmp/kargs" + uuid.New().String()
	err := dut.Conn().CommandContext(ctx, "/usr/share/vboot/bin/make_dev_ssd.sh", "--save_config", prefix, "--partitions", "2").Run()
	if err != nil {
		return errors.Wrap(err, "failed to save boot config")
	}

	savedKArgsFile := prefix + "." + "2"
	defer client.Remove(ctx, savedKArgsFile)

	savedKArgs, err := client.ReadFile(ctx, savedKArgsFile)
	if err != nil {
		return errors.Wrap(err, "failed to read saved kernel config")
	}

	// Transform the content.
	savedKArgs, err = f(savedKArgs)
	if err != nil {
		return errors.Wrap(err, "failed to transform content in boot config")
	}

	err = client.WriteFile(ctx, savedKArgsFile, savedKArgs, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to edit saved kernel config")
	}

	err = dut.Conn().CommandContext(ctx, "/usr/share/vboot/bin/make_dev_ssd.sh", "--set_config", prefix, "--partitions", "2").Run()
	if err != nil {
		return errors.Wrap(err, "failed to set boot config")
	}
	return nil
}

// enablePSR adds args to kernel cmdline to enable PSR 2.
func enablePSR(ctx context.Context, dut *dut.DUT, client *dutfs.Client) (string, error) {
	var psrValue string
	if err := editKernelArgs(ctx, dut, client, func(b []byte) ([]byte, error) {
		info := string(b)
		re := regexp.MustCompile(`\b` + `i915.enable_psr=(\d{1})` + `\b`)
		enableVal := re.FindStringSubmatch(info)
		if len(enableVal) < 1 {
			return nil, errors.New("failed to get i915.enable_psr value")
		}
		psrValue = enableVal[1]
		info = re.ReplaceAllString(info, "i915.enable_psr=2")
		return []byte(info), nil
	}); err != nil {
		return "", errors.Wrap(err, "fail to enable PSR")
	}
	return psrValue, nil
}

// disablePSR removes args from kernel cmdline to revert back to initial PSR.
func disablePSR(ctx context.Context, dut *dut.DUT, psrValue string, client *dutfs.Client) error {
	if err := editKernelArgs(ctx, dut, client, func(b []byte) ([]byte, error) {
		info := string(b)
		re := regexp.MustCompile(`\bi915.enable_psr=\d{1}` + `\b`)
		if psrValue != "" {
			info = re.ReplaceAllString(info, fmt.Sprintf("i915.enable_psr=%s", psrValue))
		}
		return []byte(info), nil
	}); err != nil {
		return errors.Wrap(err, "failed to disable PSR")
	}
	return nil
}

func (i *psr) TearDown(ctx context.Context, s *testing.FixtState) {
	d := s.DUT()
	// Attempt to connect to those DUTs that aren't already connected.
	if !d.Connected(ctx) {
		s.Log("Attempting to connect to DUT")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return d.Connect(ctx)
		}, &testing.PollOptions{Interval: time.Second, Timeout: 30 * time.Second}); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}
	}

	// Connect to local gRPC services, and keep connection alive until after
	// TearDown is called by using the fixture context.
	rpcClient, err := rpc.Dial(s.FixtContext(), s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the local gRPC service on the DUT: ", err)
	}
	defer func() {
		if err := rpcClient.Close(ctx); err != nil {
			s.Fatal("Failed to close gRPC connection to DUT: ", err)
		}
	}()
	client := dutfs.NewClient(rpcClient.Conn)

	if err := disablePSR(ctx, d, i.psrState, client); err != nil {
		s.Fatal("Failed to disable PSR and revert back to initial PSR: ", err)
	}
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
}

func (i *psr) Reset(ctx context.Context) error {
	return nil
}

func (i *psr) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (i *psr) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}
