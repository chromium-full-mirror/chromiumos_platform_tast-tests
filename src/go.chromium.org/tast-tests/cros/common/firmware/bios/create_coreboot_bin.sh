#!/usr/bin/env bash

# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

set -euo pipefail

script_dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
readonly script_dir

main() {
  # Setup temporary working directory
  tmpdir=$(mktemp -d -p "${script_dir}")
  pushd "${tmpdir}" > /dev/null

  # Compile Flash Map
  fmaptool -R ./fmap.cbfs "${script_dir}/chromeos.fmd" ./fmap.fmap
  # Create image based on fmap.fmap, with CBFS in sections listed in fmap.cbfs
  cbfstool ./img.bin create -M ./fmap.fmap -r "$(cat ./fmap.cbfs)"

  # Add CBFS files
  cbfstool ./img.bin add -f "${script_dir}/chromeos.fmd" -n fmap_src -t raw
  cbfstool ./img.bin add-int -i 0xB0213707 -n int_file
  cbfstool ./img.bin add-int -i 0xAABBCCDD -n int_file_2
  cbfstool ./img.bin add -f "${script_dir}/chromeos.fmd" -n fmap_src_lzma \
    -c lzma -t raw

  # Extract size and offset of COREBOOT section
  section_size=$(cbfstool ./img.bin layout | \
    gawk $'match($0, /'COREBOOT'.*size ([0-9]+)/, m) { print m[1] }')
  section_offset=$(cbfstool ./img.bin layout | \
    gawk $'match($0, /'COREBOOT'.*offset ([0-9]+)/, m) { print m[1] }')

  # Extract COREBOOT/RO section
  dd if=./img.bin of="${script_dir}/coreboot.bin" \
    bs=1 skip="${section_offset}" count="${section_size}" status=progress

  popd > /dev/null
  rm --recursive --force -- "${tmpdir}"

  echo "DONE: coreboot.bin created"
}

main "${@}"
