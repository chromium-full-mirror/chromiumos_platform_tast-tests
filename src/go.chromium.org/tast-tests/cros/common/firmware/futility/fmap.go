// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// FMapSection describes FlashMap.
type FMapSection struct {
	Name   string
	Offset uint32
	Size   uint32
}

// DumpFmap gets FlashMap sections from infile.
//
// If optional parameter regionNames is provided, only given regions are
// returned if present. For nil whole FlashMap will be returned.
//
// On success returns a list of FMAP sections and the futility output.
func (i *Instance) DumpFmap(ctx context.Context, inFile string, regionNames []string) ([]FMapSection, []byte, error) {
	if inFile == "" {
		return nil, nil, errors.New("futility cannot dump FlashMap: inFile is empty")
	}

	cmdArgs := append(i.futilityCmdArgs(), "dump_fmap", "-p", inFile)
	cmdArgs = append(cmdArgs, regionNames...)

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return nil, fullOut, errors.Wrapf(err, "error while dumping FlashMap with arguments %v", cmdArgs)
	}

	var fmapSections []FMapSection
	scanner := bufio.NewScanner(strings.NewReader(string(stdout)))
	for scanner.Scan() {
		// The format of the output is:
		// SECTION OFFSET SIZE
		line := string(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fullOut, errors.Errorf("incorrect fields number, expected 3, got %v for line %q", len(fields), line)
		}

		name := fields[0]
		offset, err := strconv.ParseInt(fields[1], 0, 32)
		if err != nil {
			return nil, fullOut, errors.Errorf("incorrect offset field value for line %q", line)
		}

		size, err := strconv.ParseInt(fields[2], 0, 32)
		if err != nil {
			return nil, fullOut, errors.Errorf("incorrect size field value for line %q", line)
		}

		fmapSections = append(fmapSections, FMapSection{
			Name:   name,
			Offset: uint32(offset),
			Size:   uint32(size),
		})
	}

	return fmapSections, fullOut, nil
}

// DumpFmapExtract extracts selected sections from the inFile to specified paths.
//
// inFile is path to the input firmware file.
// sections is a map where keys are section names and values are their output paths.
//
// Returns futility output if available.
func (i *Instance) DumpFmapExtract(ctx context.Context, inFile string, sections map[string]string) ([]byte, error) {
	if inFile == "" {
		return nil, errors.New("futility cannot extract FlashMap sections: inFile is empty")
	}

	if len(sections) == 0 {
		return nil, errors.New("futility cannot extract FlashMap sections: no sections provided")
	}

	cmdArgs := append(i.futilityCmdArgs(), "dump_fmap", "-x", inFile)

	for sec, outFile := range sections {
		cmdArgs = append(cmdArgs, fmt.Sprintf("%s:%s", sec, outFile))
	}

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "error while extracting FlashMap sections with arguments %v", cmdArgs)
	}
	return fullOut, nil
}

// LoadFmap loads selected sections from specified paths.
//
// inFile is path to the input firmware file.
// outFile is optional path to output file.
// sections is a map where keys are section names and values are their input paths.
//
// Returns futility output if available.
func (i *Instance) LoadFmap(ctx context.Context, inFile, outFile string, sections map[string]string) ([]byte, error) {
	if inFile == "" {
		return nil, errors.New("futility cannot load FlashMap sections: inFile is empty")
	}

	if len(sections) == 0 {
		return nil, errors.New("futility cannot load FlashMap sections: no sections provided")
	}

	cmdArgs := append(i.futilityCmdArgs(), "load_fmap")

	if outFile != "" {
		cmdArgs = append(cmdArgs, "-o", outFile)
	}

	cmdArgs = append(cmdArgs, inFile)

	for sec, outFile := range sections {
		cmdArgs = append(cmdArgs, fmt.Sprintf("%s:%s", sec, outFile))
	}

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "error while loading FlashMap sections with arguments %v", cmdArgs)
	}
	return fullOut, nil
}
