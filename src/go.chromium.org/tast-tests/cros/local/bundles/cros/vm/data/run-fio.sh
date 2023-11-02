#!/bin/bash
# Copyright 2020 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This script is meant to be run as PID 1 inside a VM.
set -e

die() {
  echo "$1"
  exit 1
}

usage() {
  die "Usage: $(basename "$0") " \
      "<block|block_packed|block_tpq|block_packed_tpq|" \
      "virtiofs|virtiofs_dax|p9|scsi> " \
      "<src> <mountpoint> <output> <jobs>"
}

main() {
  local kind="$1"
  local src="$2"
  local mountpoint="$3"
  local output="$4"

  [[ "$$" == "1" ]] || die "Not running as PID 1"

  [[ $# -ge 5 ]] || usage

  shift 4
  local jobs=( "$@" )

  [[ -d "${mountpoint}" ]] || die "${mountpoint} is not a directory"

  # We are running as pid 1.  Mount some necessary file systems.
  mount -t proc proc /proc
  mount -t sysfs sys /sys
  mount -t tmpfs tmp /tmp
  mount -t tmpfs run /run

  # Same mount options as ARCVM's
  # `/device/google/bertha/fstab.bertha.virtio_blk_data` that cat affect
  # performance.
  BLK_MOUNT_OPTIONS="rw,noatime,discard"
  # Use the same mount options as `/device/google/bertha/fstab.bertha`
  # that can affect performance.
  FS_MOUNT_OPTIONS="rw,noatime"
  case "${kind}" in
    block | block_packed | block_tpq | block_packed_tpq | scsi | pmem)
      [[ -b "${src}" ]] || die "${src} is not a block device"
      mkfs.ext4 "${src}"
      mount -o "${BLK_MOUNT_OPTIONS}" "${src}" "${mountpoint}"
      ;;
    virtiofs)
      mount -t virtiofs -o "${FS_MOUNT_OPTIONS}" "${src}" "${mountpoint}"
      ;;
    virtiofs_dax)
      mount -t virtiofs -o "${FS_MOUNT_OPTIONS},dax" "${src}" "${mountpoint}"
      ;;
    *)
      die "Unknown storage type: ${kind}"
  esac

  fio \
      --directory="${mountpoint}" \
      --runtime=1m \
      --iodepth=16 \
      --size=512M \
      --direct=0 \
      --blocksize=4K \
      --output="${output}" \
      --output-format=json \
      --filename=fio-file \
      "${jobs[@]}"
}

main "$@"
exec poweroff -f
