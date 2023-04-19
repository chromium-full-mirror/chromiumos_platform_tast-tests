// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"bufio"
	"context"
	"strconv"
	"strings"

	"chromiumos/tast/errors"
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
