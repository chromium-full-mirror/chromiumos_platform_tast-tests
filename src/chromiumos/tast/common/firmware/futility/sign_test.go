// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"testing"
)

var signPositionalArgs = []string{testFutilityPath, "sign"}

func Test_SignBIOS_NoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)
	opts := NewSignBIOSOptions("")

	_, err := i.SignBIOS(context.Background(), opts)
	if want := "no file to sign"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %q, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_SignBIOS_Opts(t *testing.T) {
	for _, p := range []struct {
		args   []string
		setter func(o *signBIOSOptions) *signBIOSOptions
	}{
		{[]string{"--keyset", "/keyset/dir/is/here"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithKeysetPath("/keyset/dir/is/here") }},
		{[]string{"--signprivate", "sign.pvkey"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithSignPrivatePath("sign.pvkey") }},
		{[]string{"--keyblock", "key.block"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithKeyBlockPath("key.block") }},
		{[]string{"--kernelkey", "kernel.key"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithKernelKeyPath("kernel.key") }},
		{[]string{"--version", "7"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithVersion(7) }},
		{[]string{"--flags", "0x4039"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithFlags(0x4039) }},
		{[]string{"--loemdir", "/this/is/loem/dir"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithLocalOEMDir("/this/is/loem/dir") }},
		{[]string{"--loemid", "loemid-it-is"}, func(o *signBIOSOptions) *signBIOSOptions { return o.WithLocalOEMID("loemid-it-is") }},
	} {
		i := newTestInstance(nil, nil, nil)
		opts := p.setter(NewSignBIOSOptions("input.bin").WithOutputFile("output.bin"))
		positionalArgs := append(signPositionalArgs, "input.bin", "output.bin")

		if _, err := i.SignBIOS(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		ti := i.commandRunner.(*testCommandRunner)
		if err := ti.assertCalledWith(positionalArgs, append([][]string{{"--type", "bios"}}, p.args), nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}

func Test_SignRWSig_NoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)
	opts := NewSignRWSigOptions("")

	_, err := i.SignRWSig(context.Background(), opts)
	if want := "no file to sign"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %q, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_SignRWSig_Opts(t *testing.T) {
	for _, p := range []struct {
		args   []string
		setter func(o *signRWSigOptions) *signRWSigOptions
	}{
		{[]string{"--prikey", "key.pvkey"}, func(o *signRWSigOptions) *signRWSigOptions { return o.WithPrivateKeyPath("key.pvkey") }},
		{[]string{"--version", "3"}, func(o *signRWSigOptions) *signRWSigOptions { return o.WithVersion(3) }},
		{[]string{"--sig_size", "1024"}, func(o *signRWSigOptions) *signRWSigOptions { return o.WithSignatureSize(1024) }},
		{[]string{"--data_size", "204800"}, func(o *signRWSigOptions) *signRWSigOptions { return o.WithDataSize(204800) }},
	} {
		i := newTestInstance(nil, nil, nil)
		opts := p.setter(NewSignRWSigOptions("input.bin").WithOutputFile("output.bin"))
		positionalArgs := append(signPositionalArgs, "input.bin", "output.bin")

		if _, err := i.SignRWSig(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		ti := i.commandRunner.(*testCommandRunner)
		if err := ti.assertCalledWith(positionalArgs, append([][]string{{"--type", "rwsig"}}, p.args), nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}
