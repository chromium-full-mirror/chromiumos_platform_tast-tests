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
import io
import json
import logging
import math
import os
from pathlib import Path
import re
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


def reboot_dut(dut: str):
    reboot_dut_command = [
        "ssh",
        f"root@{dut}",
        "reboot",
    ]
    logging.info("[DUT] Rebooting root@%s...", dut)
    subprocess.run(
        reboot_dut_command,
        check=False,
    )


def run_tast_tests(
    local_port: int,
    tests: list,
    results_dir: str = None,
    bundle: str = None,
    vars: list = [],
) -> Path:
    """Run `tests` on DUT"""
    logging.info(f"[Tast] Running tests {tests}...")
    tast_run_command = [
        "cros_sdk",
        "tast",
        "run",
        "--build=false",
    ]
    if vars:
        for var in vars:
            tast_run_command.append(f"-var={var}")
    if bundle:
        tast_run_command.append(f"-buildbundle={bundle}")
    if results_dir:
        time_string = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        tast_run_command.append(f"-resultsdir={results_dir}/{time_string}")
    tast_run_command.append(f"localhost:{local_port}")

    tast_run_command.extend(tests)
    logging.info(tast_run_command)
    process = subprocess.run(
        tast_run_command,
        stdout=subprocess.PIPE,
        stdin=subprocess.PIPE,
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=True,
    )
    tests_results_dir = re.findall(
        "Results saved to \/(.*)",
        io.BytesIO(process.stdout).readlines()[-1].decode("utf-8"),
    )[0]
    return CHROMEOS_CHECKOUT_PATH / "out" / tests_results_dir


def check_cpu_usage(dut: str, timeout: int = 60, interval: int = 1) -> None:
    logging.info("[DUT] Checking DUT's current cpu usage")
    top_command = [
        "ssh",
        "-tt",
        f"root@{dut}",
        "top",
        "-b",
        "-n",
        "1",
    ]

    start = datetime.datetime.now()
    end = start + datetime.timedelta(seconds=timeout)
    cpu_usage = ""
    while datetime.datetime.now() < end:
        process = subprocess.run(
            top_command,
            stdout=subprocess.PIPE,
            stdin=subprocess.PIPE,
        )
        matches = re.findall(
            "%Cpu\(s\):(.*)\\n", process.stdout.decode("utf-8")
        )
        if len(matches) >= 1:
            cpu_usage = matches[0].strip()
            break
        time.sleep(interval)
    logging.info(f"[DUT] DUT's current cpu usage is: {cpu_usage}")


def write_local_dut_info(results_dir: Path) -> str:
    """Write DUT information to a json file"""
    #  Get the number of days since the Unix epoch.
    days_since_epoch = int(time.time() / 86400)

    with open(results_dir / "dut-info.txt", "r", encoding="utf-8") as dutinfo:
        dutinfo_text = dutinfo.read()

    product = re.findall('model: "(.*)"', dutinfo_text)[0].strip().lower()
    board = re.findall('platform: "(.*)"', dutinfo_text)[0].strip().lower()
    brand = re.findall('brand: "(.*)"', dutinfo_text)[0].strip()
    os_version = re.findall('os_version: "(.*)"', dutinfo_text)[0].strip()
    memory_gb = math.ceil(
        int(re.findall("size_megabytes:(.*)", dutinfo_text)[0]) / 1000
    )

    with open(
        results_dir / "system_logs/lscpu.txt", "r", encoding="utf-8"
    ) as lscpu:
        lspu_text = lscpu.read()

    cpu_model = re.sub(
        "[^a-zA-Z0-9 \n\.]",
        "",
        re.findall("Model name:(.*)", lspu_text)[0].strip(),
    )
    sku = "_".join(cpu_model.split() + [f"{memory_gb}GB"])

    with open(
        results_dir / "system_logs/lsb-release", "r", encoding="utf-8"
    ) as lsb_release:
        lsb_release_text = lsb_release.read()

    milestone = int(
        re.findall("CHROMEOS_RELEASE_CHROME_MILESTONE=(.*)", lsb_release_text)[
            0
        ].strip()
    )
    build_number = re.findall(
        "CHROMEOS_RELEASE_BUILD_NUMBER=(.*)", lsb_release_text
    )[0].strip()
    branch_number = re.findall(
        "CHROMEOS_RELEASE_BRANCH_NUMBER=(.*)", lsb_release_text
    )[0].strip()
    patch_number = re.findall(
        "CHROMEOS_RELEASE_PATCH_NUMBER=(.*)", lsb_release_text
    )[0].strip()
    cros_version = f"{milestone}.{build_number}.{branch_number}.{patch_number}"
    cros_version_int = int(
        (
            f"{milestone}{build_number.zfill(6)}{branch_number.zfill(3)}"
            f"{patch_number.zfill(3)}"
        )
    )

    with open(
        results_dir / "system_logs/hostname.txt", "r", encoding="utf-8"
    ) as hostname:
        hostname_text = hostname.readlines()[-1]

    dut_hostname = hostname_text.strip()

    local_dut_info = {
        "event_date": days_since_epoch,
        "sku": f"{product}_{sku}",
        "product": product,
        "board": board,
        "milestone": milestone,
        "cros_version": cros_version,
        "cros_version_int": cros_version_int,
        "variant": os_version,
        # It's not really a hwid, it's made of model name, board name and brand.
        "hwid": f"{product}-{board}-{brand}",
        # Likely to be `localhost`.
        "dut_hostname": dut_hostname,
    }

    local_dut_info_json = json.dumps(local_dut_info, indent=4)
    with open(
        results_dir / "local_dut_info.txt", "w", encoding="utf-8"
    ) as local_dut_info_file:
        local_dut_info_file.write(local_dut_info_json)

    return product


def upload_latest_tests_results(
    username: str, bucket_name: str, dut_model: str, results_dir: Path
) -> None:
    """Upload the latest tests results to Google Cloud bucket `bucket_name`"""
    credentials, _ = google.auth.default()
    client = storage.Client(credentials=credentials, project=PROJECT_ID)
    bucket = client.get_bucket(bucket_name)
    local_directory_path = results_dir
    local_directory_realpath = os.path.realpath(local_directory_path)
    test_run_id = (
        f"{os.path.basename(local_directory_realpath)}-{username}-{dut_model}"
    )
    today_date_string = datetime.datetime.today().strftime("%Y-%m-%d")
    gcs_folder_path = f"{today_date_string}/{test_run_id}"
    logging.info("[Cloud] Uploading to gs://%s/...", gcs_folder_path)
    upload_local_directory_to_gcs(local_directory_path, bucket, gcs_folder_path)


def parse_arguments(argv) -> argparse.Namespace:
    """Parse arguments and return the argparse.Namespace object"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dut",
        nargs=1,
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
        "--local",
        action="store_true",
        help=(
            "If set, the results will only be in local and"
            "not uploaded to the Google Cloud."
        ),
    )
    parser.add_argument(
        "--repeat",
        nargs="?",
        type=int,
        default=1,
        help=(
            "If set, `tast run` will be repeated for --repeat times,"
            " default is 1 (no repeat)."
        ),
    )
    parser.add_argument(
        "--cooldown",
        nargs="?",
        type=int,
        default=0,
        help=(
            "If set, sleep for --cooldown seconds before running `tast run`,"
            " default is 0."
        ),
    )
    parser.add_argument(
        "--reboot",
        action="store_true",
        help=("If set, the dut will be rebooted before running `tast run`."),
    )
    parser.add_argument(
        "--vars",
        nargs="+",
        help="Tast runtime variables.",
    )
    parser.add_argument(
        "--bundle",
        nargs="?",
        type=str,
        help=("Tast test bundle name."),
    )
    parser.add_argument(
        "--results-dir",
        nargs="?",
        type=str,
        help=("Directory for test results."),
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
    logging.info("[User] %s starts run_cuj_tests.py...", username)

    try:
        if opts.reboot:
            reboot_dut(opts.dut[0])
            logging.info("[DUT] Waiting 60 seconds for DUT to reboot")
            time.sleep(60)
            start_ssh_tunnel(opts.dut[0], opts.local_port)
        if opts.cooldown > 0:
            logging.info(
                "[DUT] Waiting %s seconds for DUT to cooldown", opts.cooldown
            )
            time.sleep(opts.cooldown)

        check_cpu_usage(opts.dut[0])
        kill_ssh_tunnel(opts.local_port)
        start_ssh_tunnel(opts.dut[0], opts.local_port)

        if opts.image:
            flash_image(opts.image, opts.local_port, opts.dut[0])
        else:
            logging.info("[Flash] Not flashing image")

        for i in range(opts.repeat):
            logging.info(f"[Tast] #{i+1} Running tests {opts.patterns}...")
            results_dir_path = run_tast_tests(
                opts.local_port,
                opts.patterns,
                opts.results_dir,
                opts.bundle,
                opts.vars,
            )
            dut_model = write_local_dut_info(results_dir_path)

            if not opts.local:
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
                    upload_latest_tests_results(
                        username, opts.bucket_name, dut_model, results_dir_path
                    )

    finally:
        kill_ssh_tunnel(opts.local_port)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
