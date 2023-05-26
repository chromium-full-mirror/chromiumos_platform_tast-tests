// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"testing"
)

func Test_DumpKernelConfig_NoPartition(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	cfg, _, err := i.DumpKernelConfig(context.Background(), "", 0)
	if cfg != "" {
		t.Errorf("unexpected config contents %q", cfg)
	}
	if want := "cannot dump kernel config: kernelPartition is empty"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %q, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpKernelConfig_Kloadaddr(t *testing.T) {
	data := "config_contents"
	i := newTestInstance([]byte(data), nil, nil)

	cfg, _, err := i.DumpKernelConfig(context.Background(), "/dev/sdb", 0x1234)
	if cfg == "" {
		t.Error("config contents expected, got nothing")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	positionalArgs := []string{testFutilityPath, "dump_kernel_config", "/dev/sdb"}
	optionArgs := [][]string{{"--kloadaddr", "0x1234"}}
	if err = ti.assertCalledWith(positionalArgs, optionArgs, nil); err != nil {
		t.Error(err)
	}

	cfg, _, err = i.DumpKernelConfig(context.Background(), "/dev/sdb", ^uint64(0))
	if cfg == "" {
		t.Error("config contents expected, got nothing")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err = ti.assertCalledWith(positionalArgs, nil, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}
