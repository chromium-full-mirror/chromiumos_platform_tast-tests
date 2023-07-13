#!/usr/bin/env vpython3
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Tools to run CUJ tests and upload results.

run_cuj_tests.py allows users to ssh into the prepared DUT, flash images, run
tast tests and upload the results to the Google Cloud bucket. The results on the
Cloud bucket will be processed by TPS data pipeline.

Before running this script:
  (1) Set up Google Cloud credentials, see
  https://chromium.googlesource.com/chromiumos/docs/+/HEAD/gsutil.md#setup
  (2) Make sure the Google Cloud bucket write permission is set
  (3) Make sure the test device can be ssh into without passwords
"""

# [VPYTHON:BEGIN]
# python_version: "3.8"
# wheel: <
#   name: "infra/python/wheels/google-cloud-storage-py3"
#   version: "version:2.1.0"
# >
# # google-cloud-storage dep
# wheel: <
#   name: "infra/python/wheels/google-resumable-media-py3"
#   version: "version:2.3.0"
# >
# # google-cloud-storage dep
# wheel: <
#   name: "infra/python/wheels/google-cloud-core-py3"
#   version: "version:2.2.2"
# >
# # google-cloud-storage dep
# wheel: <
#   name: "infra/python/wheels/google-crc32c/${vpython_platform}"
#   version: "version:1.3.0"
# >
# # google-cloud-storage dep
# wheel: <
#   name: "infra/python/wheels/google-api-core-py3"
#   version: "version:2.11.0"
# >
# # google-api-core-dep
# wheel: <
#   name: "infra/python/wheels/grpcio/${vpython_platform}"
#   version: "version:1.44.0"
# >
# # google-api-core-dep
# wheel: <
#   name: "infra/python/wheels/grpcio-status-py3"
#   version: "version:1.44.0"
# >
# # google-api-core dep
# wheel: <
#   name: "infra/python/wheels/requests-py2_py3"
#   version: "version:2.26.0"
# >
# # requests dep
# wheel: <
#   name: "infra/python/wheels/urllib3-py2_py3"
#   version: "version:1.26.6"
# >
# # requests dep
# wheel: <
#   name: "infra/python/wheels/certifi-py2_py3"
#   version: "version:2021.5.30"
# >
# # requests dep
# wheel: <
#   name: "infra/python/wheels/idna-py3"
#   version: "version:3.2"
# >
# # requests dep
# wheel: <
#   name: "infra/python/wheels/certifi-py2_py3"
#   version: "version:2021.5.30"
# >
# # requests dep
# wheel: <
#   name: "infra/python/wheels/charset_normalizer-py3"
#   version: "version:2.0.4"
# >
# # google-api-core dep
# wheel: <
#   name: "infra/python/wheels/googleapis-common-protos-py2_py3"
#   version: "version:1.59.0"
# >
# # google-api-core dep
# wheel: <
#   name: "infra/python/wheels/protobuf-py3"
#   version: "version:4.21.9"
# >
# wheel: <
#   name: "infra/python/wheels/google-auth-py3"
#   version: "version:2.16.2"
# >
# # google-auth dep
# wheel: <
#   name: "infra/python/wheels/rsa-py3"
#   version: "version:4.7.2"
# >
# # google-auth dep
# wheel: <
#   name: "infra/python/wheels/cachetools-py3"
#   version: "version:4.2.2"
# >
# # google-auth dep
# wheel: <
#   name: "infra/python/wheels/six-py2_py3"
#   version: "version:1.16.0"
# >
# # google-auth dep
# wheel: <
#   name: "infra/python/wheels/pyasn1_modules-py2_py3"
#   version: "version:0.2.8"
# >
# # pyasn1_modules dep
# wheel: <
#   name: "infra/python/wheels/pyasn1-py2_py3"
#   version: "version:0.4.8"
# >
# [VPYTHON:END]

import argparse
import datetime
import getpass
import logging
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
from typing import Optional

import google.auth
from google.cloud import storage


assert sys.version_info >= (3, 8), "Python 3.8+ required"

PROJECT_ID = "cros-perfmetrics-cuj"
DEFAULT_BUCKET_NAME = "cros-performance-sheriff"
THIS_FILE = Path(__file__).resolve()
CHROMEOS_CHECKOUT_PATH = THIS_FILE.parent.parent.parent.parent.parent
LATEST_TEST_DIR_PATH = (
    CHROMEOS_CHECKOUT_PATH / "out/tmp/tast/results/latest"
)
DEFAULT_SSH_LOCAL_PORT = 2222


def is_port_in_use(local_port: int) -> bool:
    """Check if port `local_port` is in use"""
    check_port_proc = subprocess.run(
        ["lsof", f"-i:{local_port}"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        # If no process is found, a non-zero status is return.
        check=False,
    )
    return bool(check_port_proc.stdout)


def wait_for_port(
    local_port: int, in_use: bool, interval: int = 1, timeout: int = 5
) -> bool:
    """Wait until `local_port` to be in the expected `in_use` state.

    Return true if `local_port` matches the expected `in_use` state before
    `timeout` seconds have elapsed. Otherwise, return false.
    """
    start = datetime.datetime.now()
    end = start + datetime.timedelta(seconds=timeout)
    while (
        is_port_in_use(local_port) != in_use and datetime.datetime.now() < end
    ):
        time.sleep(interval)
    return is_port_in_use(local_port) == in_use


def start_ssh_tunnel(dut: str, local_port: int) -> None:
    """Start a SSH tunnel in the background"""
    if not is_port_in_use(local_port):
        logging.info("[SSH] Port %d is not in use", local_port)
        ssh_tunnel_command = [
            "ssh",
            "-f",
            "-N",
            "-L",
            f"{local_port}:localhost:22",
            f"root@{dut}",
        ]
        subprocess.run(
            ssh_tunnel_command,
            check=True,
        )
        if not wait_for_port(local_port, in_use=True):
            # Exit if it fails to start the SSH tunnel when `local_port`
            # is available.
            sys.exit(f"[SSH] Failed to start SSH tunnel from port {local_port}")
        logging.info(
            "[SSH] Started a SSH tunnel from local port %d to port 22 of %s",
            local_port,
            dut,
        )


def kill_ssh_tunnel(local_port: int) -> None:
    """Kill the process running on port `local_port`"""
    proc = subprocess.run(
        ["lsof", "-i", f":{local_port}"],
        capture_output=True,
        encoding="utf-8",
        # If no process is found, a non-zero status is return.
        check=False,
    )
    for process in proc.stdout.split("\n")[1:]:
        data = [x for x in process.split(" ") if x]
        if len(data) <= 1:
            continue
        os.kill(int(data[1]), signal.SIGKILL)
    if not wait_for_port(local_port, in_use=False):
        # Exit if it fails to kill the SSH tunnel
        # when `local_port` is in use.
        sys.exit(f"[SSH] Failed to kill process on port {local_port}")
    logging.info("[SSH] Killed process on port %d", local_port)


def upload_local_directory_to_gcs(
    local_directory_path: Path, bucket: str, gcs_path: str
) -> None:
    """Upload local test results directory to Google Cloud Storage bucket

    Args:
        local_directory_path: The path to the local directory that will be
            uploaded
        bucket: The bucket name
        gcs_path: The path to the directory in the bucket where the local
            directory will be uploaded to
    """
    assert local_directory_path.is_dir()
    for entry in local_directory_path.glob("*"):
        remote_path = f"{gcs_path}/{entry.name}"
        if entry.is_file():
            blob = bucket.blob(remote_path)
            blob.upload_from_filename(entry)
            logging.info(
                "[Cloud] Uploaded to gs://%s/%s", bucket.name, remote_path
            )
        else:
            upload_local_directory_to_gcs(
                entry,
                bucket,
                remote_path,
            )


def flash_image(image: str, local_port: int, dut: str):
    """Flash image from `image` to `dut` via port `local_port`

    The image path has to be a xbuddy path or local path. For example,
    xbuddy://remote/hatch/R113-15372.0.0/test,
    xbuddy://remote/amd64-generic/latest-canary/test,
    xbuddy://remote/chrome-atom-release-afdo-verify-toolchain/R113-15393.16.0-1-8784488478843532081/test
    or ~/chromiumos/tmp/test_image.bin.
    Notice that the image has to be a test image, and if it is a
    local image, the image .bin file has to be placed inside of
    chromiumos checkout.
    """
    logging.info("[Flash] Flashing image from %s to DUT...", image)
    logging.info(
        "[Flash] The SSH tunnel might disconnect after flashing. Please"
        " reconnect with the same SSH arguments.\n"
    )
    flash_command = [
        "cros",
        "flash",
        "--no-ping",
        f"ssh://{dut}",
        f"{image}",
    ]
    subprocess.run(
        flash_command,
        stdin=subprocess.PIPE,
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=True,
    )
    logging.info("\n[Flash] Flashed image from %s to DUT\n", image)

    if dut and local_port:
        logging.info("[Flash] Reconnecting to the DUT...")
        kill_ssh_tunnel(local_port)
        start_ssh_tunnel(dut, local_port)


def run_tast_tests(local_port: int, tests: list) -> None:
    """Run `tests` on DUT"""
    logging.info("[Tast] Running tests {tests}...")
    tast_run_command = [
        "cros_sdk",
        "tast",
        "run",
        "--build=false",
        f"localhost:{local_port}",
    ]
    tast_run_command.extend(tests)
    logging.info(tast_run_command)
    subprocess.run(
        tast_run_command,
        stdin=subprocess.PIPE,
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=True,
    )


def upload_latest_tests_results(username: str, bucket_name: str) -> None:
    """Upload the latest tests results to Google Cloud bucket `bucket_name`"""
    credentials, _ = google.auth.default()
    client = storage.Client(credentials=credentials, project=PROJECT_ID)
    bucket = client.get_bucket(bucket_name)
    local_directory_path = LATEST_TEST_DIR_PATH
    local_directory_realpath = os.path.realpath(local_directory_path)
    test_run_id = f"{os.path.basename(local_directory_realpath)}-{username}"
    today_date_string = datetime.datetime.today().strftime("%Y-%m-%d")
    gcs_folder_path = f"{today_date_string}/{test_run_id}"
    upload_local_directory_to_gcs(local_directory_path, bucket, gcs_folder_path)


def parse_arguments(argv) -> argparse.Namespace:
    """Parse arguments and return the argparse.Namespace object"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dut",
        nargs="?",
        type=str,
        help=(
            "If set, a SSH tunnel will be opened at port --local-port;"
            " Otherwise user needs to guarantee a open SSH connection"
            " at port --local-port during this program"
        ),
    )
    parser.add_argument(
        "--local-port",
        nargs="?",
        type=int,
        default=DEFAULT_SSH_LOCAL_PORT,
        help=(
            "Mapped local port to DUT for accessing it inside chroot,"
            " default is %(default)s"
        ),
    )
    parser.add_argument(
        "--image",
        nargs="?",
        type=str,
        help=(
            "URI or path that cros flash understands."
            " If set, the image will be flashed to the test device;"
            " Otherwise no image will be flashed in this program"
        ),
    )
    parser.add_argument(
        "--bucket-name",
        nargs="?",
        type=str,
        default=DEFAULT_BUCKET_NAME,
        help=(
            "The Google Cloud Storage bucket name that test results"
            " will be uploaded; If not set, the default will be used"
        ),
    )
    parser.add_argument(
        "--upload",
        action="store_true",
        help=(
            "If set, the results will be uploaded to the Google Cloud"
            " bucket without asking after `tast run` finishes;"
            " Otherwise, the program will ask for user's permission"
            " to upload the results."
        ),
    )
    parser.add_argument(
        "patterns",
        nargs=argparse.REMAINDER,
        type=str,
        help=(
            "The tests patterns that will be used by <tast run>, e.g."
            " ui.IdlePerf.ash ui.IdlePerf.lacros or `(group:cuj ||"
            " group:cuj_experimental)`"
        ),
    )
    return parser.parse_args(argv)


def main(argv) -> Optional[int]:
    """Main function"""
    opts = parse_arguments(argv)

    logging.basicConfig(format="%(asctime)s %(message)s", level=logging.INFO)
    username = getpass.getuser()
    logging.info("[User] %s starts run_cuj_tests.py...\n", username)

    try:
        if opts.dut:
            kill_ssh_tunnel(opts.local_port)
            start_ssh_tunnel(opts.dut, opts.local_port)

        if opts.image:
            flash_image(opts.image, opts.local_port, opts.dut)
        else:
            logging.info("[Flash] Not flashing image")

        run_tast_tests(opts.local_port, opts.patterns)

        if not opts.upload:
            while True:
                user_input = input(
                    "[Cloud] Are you sure to upload test results to the bucket"
                    f" {opts.bucket_name}?(y/n):"
                ).lower()
                if user_input == "y":
                    upload = True
                    break
                elif user_input == "n":
                    break
                else:
                    logging.info("Enter y or n")
        if opts.upload or upload:
            upload_latest_tests_results(username, opts.bucket_name)
    finally:
        if opts.dut:
            kill_ssh_tunnel(opts.local_port)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
