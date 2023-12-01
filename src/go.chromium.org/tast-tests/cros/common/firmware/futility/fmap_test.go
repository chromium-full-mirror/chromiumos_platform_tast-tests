// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"go.chromium.org/tast/core/errors"
)

var dumpFmapPositionalArgs = []string{testFutilityPath, "dump_fmap"}
var dumpFmapExtraArgs = [][]string{
	{"-p"},
	{"-h"},
	{"-H"},
	{"-F"},
}

func Test_DumpFmap_ContextDeadlineExceeded(t *testing.T) {
	i := newTestInstance(nil, nil, context.DeadlineExceeded)

	fmapSections, out, err := i.DumpFmap(context.Background(), "inFile", nil)
	if len(fmapSections) > 0 {
		t.Errorf("no FlashMAP sections expected, got: %q", fmapSections)
	}
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpFmap_NoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	fmapSections, out, err := i.DumpFmap(context.Background(), "", nil)
	if len(fmapSections) > 0 {
		t.Errorf("no FlashMAP sections expected, got: %q", fmapSections)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("command output not expected, got: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpFmap_NoFutilityOutput(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	fmapSections, out, err := i.DumpFmap(context.Background(), "input.bin", nil)
	if len(fmapSections) > 0 {
		t.Errorf("no FlashMAP sections expected, got: %q", fmapSections)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("command output not expected, got: %q", string(out))
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	ti := i.commandRunner.(*testCommandRunner)
	if err = ti.assertCalledWith(append(dumpFmapPositionalArgs, "input.bin"), nil, dumpFmapExtraArgs); err != nil {
		t.Error(err)
	}
}

func Test_DumpFmap_ParseDataOK(t *testing.T) {
	testData := []byte("area1\t1\t0x123123\n" +
		"area2 2 88776655\n" +
		"\tarea3\t 0x3 \t 774466\t\n")

	expectedFields := []FMapSection{
		{Name: "area1", Offset: 1, Size: 0x123123},
		{Name: "area2", Offset: 2, Size: 88776655},
		{Name: "area3", Offset: 0x3, Size: 774466},
	}

	i := newTestInstance(testData, nil, nil)

	fmapSections, out, err := i.DumpFmap(context.Background(), "input.bin", nil)
	if !reflect.DeepEqual(fmapSections, expectedFields) {
		t.Errorf("incorrect FlashMAP sections returned, expected %q, got %q", expectedFields, fmapSections)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected command output, got nothing")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	ti := i.commandRunner.(*testCommandRunner)
	if err = ti.assertCalledWith(append(dumpFmapPositionalArgs, "input.bin"), nil, dumpFmapExtraArgs); err != nil {
		t.Error(err)
	}
}

func Test_DumpFmap_ParseDataInvalid(t *testing.T) {
	for _, input := range [][]byte{
		[]byte("invalid-1 offset size\n"),
		[]byte("also_invalid 0 size\n"),
		[]byte("invalid_2 offset 33333\n"),
		[]byte("too_much_fields\t123\t456\t789\n"),
		[]byte("not_enough_fields\n777\n"),
	} {

		i := newTestInstance(input, nil, nil)

		fmapSections, out, err := i.DumpFmap(context.Background(), "input.bin", nil)
		if len(fmapSections) > 0 {
			t.Errorf("no FlashMAP sections expected, got: %q", fmapSections)
		}
		if len(strings.TrimSpace(string(out))) == 0 {
			t.Error("expected command output, got nothing")
		}
		if err == nil {
			t.Error("error expected, got nothing")
		}

		ti := i.commandRunner.(*testCommandRunner)
		if err = ti.assertCalledWith(append(dumpFmapPositionalArgs, "input.bin"), nil, dumpFmapExtraArgs); err != nil {
			t.Error(err)
		}
	}
}

func Test_DumpFmap_ParseDataSelective(t *testing.T) {
	// Test implementation does not perform filtering and parsing of the data,
	// so provide only matching data as an input.
	testData := []byte("area1 1 11223344\n" +
		"area3 3 998877\n")

	expectedFields := []FMapSection{
		{Name: "area1", Offset: 1, Size: 11223344},
		{Name: "area3", Offset: 3, Size: 998877},
	}

	i := newTestInstance(testData, nil, nil)

	fmapSections, out, err := i.DumpFmap(context.Background(), "input.bin", []string{"area1", "area3"})
	if !reflect.DeepEqual(fmapSections, expectedFields) {
		t.Errorf("incorrect FlashMAP sections returned, expected %q but got %q", expectedFields, fmapSections)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected command output, got nothing.")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	ti := i.commandRunner.(*testCommandRunner)
	if err = ti.assertCalledWith(append(dumpFmapPositionalArgs, "input.bin", "area1", "area3"), nil, dumpFmapExtraArgs); err != nil {
		t.Error(err)
	}
}

func Test_DumpFmapExtract_ContextDeadlineExceeded(t *testing.T) {
	i := newTestInstance(nil, nil, context.DeadlineExceeded)

	out, err := i.DumpFmapExtract(context.Background(), "inFile", map[string]string{"SECTION": "path.bin"})
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpFmapExtract_NoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	out, err := i.DumpFmapExtract(context.Background(), "", map[string]string{"SECTION": "path.bin"})
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("command output not expected, got: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpFmapExtract_NoSectionsProvided(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	out, err := i.DumpFmapExtract(context.Background(), "inFile", nil)
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpFmapExtract_CallError(t *testing.T) {
	i := newTestInstance(nil, nil, errors.New("call failure"))

	out, err := i.DumpFmapExtract(context.Background(), "inFile", map[string]string{"SECTION": "path.bin"})
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_DumpFmapExtract_Success(t *testing.T) {
	expectedOutput := []byte("SUCCESS")
	i := newTestInstance(expectedOutput, nil, nil)

	out, err := i.DumpFmapExtract(context.Background(), "input.bin", map[string]string{"SECTION": "path.bin"})
	if strings.TrimSpace(string(out)) != strings.TrimSpace(string(expectedOutput)) {
		t.Errorf("unexpected command output: %q, expected: %q", string(out), string(expectedOutput))
	}
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := [][]string{
		{"-x", "input.bin"},
	}

	if err = ti.assertCalledWith(append(dumpFmapPositionalArgs, "SECTION:path.bin"), optionArgs, dumpFmapExtraArgs); err != nil {
		t.Error(err)
	}
}

var loadFmapPositionalArgs = []string{testFutilityPath, "load_fmap"}

func Test_LoadFmap_ContextDeadlineExceeded(t *testing.T) {
	i := newTestInstance(nil, nil, context.DeadlineExceeded)

	out, err := i.LoadFmap(context.Background(), "inFile", "", map[string]string{"SECTION": "path.bin"})
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_LoadFmap_NoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	out, err := i.LoadFmap(context.Background(), "", "", map[string]string{"SECTION": "path.bin"})
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("command output not expected, got: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_LoadFmap_NoSectionsProvided(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	out, err := i.LoadFmap(context.Background(), "inFile", "", nil)
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_LoadFmap_CallError(t *testing.T) {
	i := newTestInstance(nil, nil, errors.New("call failure"))

	out, err := i.LoadFmap(context.Background(), "inFile", "", map[string]string{"SECTION": "path.bin"})
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_LoadFmap_Success(t *testing.T) {
	expectedOutput := []byte("SUCCESS")
	i := newTestInstance(expectedOutput, nil, nil)

	out, err := i.LoadFmap(context.Background(), "input.bin", "output.bin", map[string]string{"SECTION": "path.bin"})
	if strings.TrimSpace(string(out)) != strings.TrimSpace(string(expectedOutput)) {
		t.Errorf("unexpected command output: %q, expected: %q", string(out), string(expectedOutput))
	}
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := [][]string{
		{"-o", "output.bin"},
	}

	if err = ti.assertCalledWith(append(loadFmapPositionalArgs, "SECTION:path.bin"), optionArgs, nil); err != nil {
		t.Error(err)
	}
}
