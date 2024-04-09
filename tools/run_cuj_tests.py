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
  (3) Make sure the test device can be ssh into without passwords, see
  go/chromeos-lab-duts-ssh#setup-private-key-and-ssh-config
  (4) Install corp-ssh-helper-helper, see
  https://chromium.googlesource.com/chromiumos/platform/dev-util/+/HEAD/contrib/corp-ssh-helper-helper/README.md#corp_ssh_helper_helper
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
DEFAULT_BUCKET_NAME = "sw-perf-cuj-experiment"
THIS_FILE = Path(__file__).resolve()
CHROMEOS_CHECKOUT_PATH = THIS_FILE.parent.parent.parent.parent.parent
DEFAULT_SSH_LOCAL_PORT = 2222


def get_model(dut: str) -> str:
    """Get the model of the DUT.

    Args:
        dut: The DUT host name.

    Returns:
        The model of the DUT. For example, "voxel".
    """
    cros_config_command = [
        "ssh",
        dut,
        "cros_config",
        "/",
        "name",
    ]
    process = subprocess.run(
        cros_config_command,
        stdout=subprocess.PIPE,
        check=True,
    )
    return process.stdout.decode("utf-8").strip()


def get_builder_path(dut: str) -> str:
    """Get the builder path of the DUT.

    Args:
        dut: The DUT host name.

    Returns:
        The builder path of the DUT.
        For example, `volteer-release/R124-15815.0.0`
    """
    cros_config_command = [
        "ssh",
        dut,
        "cat",
        "/etc/lsb-release",
    ]
    process = subprocess.run(
        cros_config_command,
        stdout=subprocess.PIPE,
        check=True,
    )
    builder_path_matches = re.findall(
        "CHROMEOS_RELEASE_BUILDER_PATH=(.*)\n", process.stdout.decode("utf-8")
    )
    if len(builder_path_matches) == 0:
        return ""
    return builder_path_matches[0].strip()


def run_ssh_helper() -> None:
    subprocess.run(
        [
            "src/platform/dev/contrib/corp-ssh-helper-helper/corp-ssh-helper-helper-server.py"
        ],
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=False,
    )


def kill_ssh_helper() -> None:
    subprocess.run(
        [
            "src/platform/dev/contrib/corp-ssh-helper-helper/corp-ssh-helper-helper-server.py",
            "--kill",
        ],
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=False,
    )


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
            logging.info(f"[Cloud] {remote_path} uploaded")
        else:
            upload_local_directory_to_gcs(
                entry,
                bucket,
                remote_path,
            )


def crosfleet_dut_lease(hostname: str, dims: str, minutes: int) -> str:
    """Lease a DUT with crosfleet dut lease.

    Args:
        hostname: The hostname of the DUT to lease.
        dims: The dimensions of the DUT to lease.
        minutes: The duration of the lease in minutes.

    Returns:
        The hostname of the leased DUT.
    """
    lease_command = [
        "crosfleet",
        "dut",
        "lease",
        "-minutes",
        str(minutes),
    ]
    if hostname:
        lease_command += ["-host", hostname]
    elif dims:
        lease_command += ["-dims", dims]
    logging.info(f"[Crosfleet] Leasing DUT with {lease_command}")
    process = subprocess.run(
        lease_command,
        stderr=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stdin=subprocess.PIPE,
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=False,
    )
    stderr = process.stderr.decode("utf-8")
    logging.info(stderr)
    if process.returncode != 0:
        raise ValueError(f"Failed to run {lease_command}: {stderr}")
    hostname_matches = re.findall(
        "DUT_HOSTNAME=(.*)",
        stderr,
    )
    if len(hostname_matches) == 0:
        raise ValueError(f"Failed to find DUT_HOSTNAME from {stderr}")
    else:
        dut = hostname_matches[0]
    return dut


def crosfleet_dut_abandon():
    """Abandon all DUT leased with crosfleet dut lease"""
    logging.info("[Crosfleet] Abandoning all scheduled builds...")
    abandon_command = ["crosfleet", "dut", "abandon"]
    subprocess.run(
        abandon_command,
        stderr=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stdin=subprocess.PIPE,
        check=False,
    )


def flash_image(image_path: str, dut: str):
    """Flash image from `image` to `dut` via port `local_port`

    The image path has to be a xbuddy path or local path. For example,
    xbuddy://remote/hatch-release/R113-15372.0.0/test,
    xbuddy://remote/chrome-atom-release-afdo-verify-toolchain/R113-15393.16.0-1-8784488478843532081/test
    or ~/chromiumos/tmp/test_image.bin.
    Notice that the image has to be a test image, and if it is a
    local image, the image .bin file has to be placed inside of
    chromiumos checkout.
    """
    dut_builder_path = get_builder_path(dut)
    if dut_builder_path in image_path:
        logging.info(
            "[Flash] No need to flash image %s since DUT's builder is %s",
            image_path,
            dut_builder_path,
        )
        return
    logging.info(
        "[Flash] Flashing image from %s to DUT %s...",
        image_path,
        dut_builder_path,
    )
    logging.info(
        "[Flash] The SSH tunnel might disconnect after flashing. Please"
        " reconnect with the same SSH arguments.\n"
    )
    flash_command = [
        "cros",
        "flash",
        "--no-ping",
        f"ssh://{dut}",
        image_path,
    ]
    subprocess.run(
        flash_command,
        stdin=subprocess.PIPE,
        cwd=CHROMEOS_CHECKOUT_PATH,
        check=False,
    )
    logging.info("[Flash] Flashed image from %s to DUT\n", image_path)

    logging.info(
        "[Flash] Waiting for 60 seconds and reconnecting to the DUT..."
    )
    time.sleep(60)
    dut_builder_path = get_builder_path(dut)
    if dut_builder_path not in image_path:
        raise ValueError(
            f"[Flash] Flash failed. Expected {image_path} but got {dut_builder_path}."
        )


def reboot_dut(dut: str):
    reboot_dut_command = [
        "ssh",
        dut,
        "reboot",
    ]
    logging.info("[DUT] Rebooting %s...", dut)
    subprocess.run(
        reboot_dut_command,
        check=False,
    )


def run_tast_tests(
    dut: str,
    no_build: bool,
    tests: list,
    results_dir: str = None,
    bundle: str = None,
    variables: list = [],
):
    """Run `tests` on DUT"""
    logging.info(f"[Tast] Running tests {tests}...")
    tast_run_command = [
        "cros_sdk",
        "tast",
        "run",
    ]
    if no_build:
        tast_run_command.extend(
            ["-build=false", "-downloadprivatebundles=true"]
        )
    if variables:
        for var in variables:
            tast_run_command.append(f"-var={var}")
    if bundle:
        tast_run_command.append(f"-buildbundle={bundle}")
    if results_dir:
        time_string = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        tast_run_command.append(f"-resultsdir={results_dir}/{time_string}")
    tast_run_command.append(dut)

    tast_run_command.extend(tests)
    logging.info(f"Tast command: {tast_run_command}")
    process = subprocess.run(
        tast_run_command,
        stdout=subprocess.PIPE,
        stdin=subprocess.PIPE,
        cwd=CHROMEOS_CHECKOUT_PATH,
    )
    stdout = process.stdout.decode("utf-8")
    logging.info(stdout)
    tests_results_dir_matches = re.findall(
        "Results saved to \/(.*)",
        stdout,
    )
    if len(tests_results_dir_matches) > 0:
        tests_results_dir = tests_results_dir_matches[0]
    else:
        raise ValueError("Failed to find tests results.")
    tests_statuses = []
    for line in io.BytesIO(process.stdout).readlines():
        if re.search("\[( PASS | FAIL | SKIP |NOTRUN)\]", line.decode("utf-8")):
            tests_statuses.append(line.decode("utf-8"))
    return [CHROMEOS_CHECKOUT_PATH / "out" / tests_results_dir, tests_statuses]


def check_cpu_usage(dut: str, timeout: int = 60, interval: int = 1) -> None:
    """Check current cpu usage on DUT"""
    logging.info("[DUT] Checking DUT's current cpu usage")
    top_command = [
        "ssh",
        dut,
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
            check=True,
        )
        matches = re.findall(
            "%Cpu\(s\):(.*)\\n", process.stdout.decode("utf-8")
        )
        if len(matches) >= 1:
            cpu_usage = matches[0].strip()
            break
        time.sleep(interval)
    logging.info(f"[DUT] DUT's current cpu usage is: {cpu_usage}")


def write_local_dut_info(results_dir: Path, label: str) -> str:
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
        "label": label,
    }

    local_dut_info_json = json.dumps(local_dut_info, indent=4)
    with open(
        results_dir / "local_dut_info.txt", "w", encoding="utf-8"
    ) as local_dut_info_file:
        local_dut_info_file.write(local_dut_info_json)

    return product


def check_experiment_id_existance(
    bucket_name: str, username: str, experiment_id: str
) -> bool:
    """Check if the experiment id already exists.

    Args:
        bucket_name: The GCS bucket name
        username: The username
        experiment_id: The experiment id

    Returns:
        True if the {username}/{experiment_id} directory
        already exists, otherwise False.
    """
    credentials, _ = google.auth.default()
    client = storage.Client(credentials=credentials, project=PROJECT_ID)
    bucket = client.get_bucket(bucket_name)

    return list(
        client.list_blobs(bucket, prefix=f"{username}/{experiment_id}/")
    )


def upload_latest_tests_results(
    bucket_name: str,
    username: str,
    experiment_id: str,
    dut_model: str,
    local_directory_path: Path,
) -> None:
    """Upload the latest tests results to Google Cloud bucket `bucket_name`"""
    credentials, _ = google.auth.default()
    client = storage.Client(credentials=credentials, project=PROJECT_ID)
    bucket = client.get_bucket(bucket_name)
    gcs_folder_path = f"{username}/{experiment_id}/{dut_model}/{os.path.basename(local_directory_path)}"
    upload_local_directory_to_gcs(local_directory_path, bucket, gcs_folder_path)


def parse_arguments(argv) -> argparse.Namespace:
    """Parse arguments and return the argparse.Namespace object"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--lease-dims",
        nargs="?",
        type=str,
        help=(
            "Dimensions of an individual DUT to lease, seperated by comma."
            " For example, --lease-dims label-model:kench,label-board:fizz"
        ),
    )
    parser.add_argument(
        "--lease-hostname",
        nargs="?",
        type=str,
        help=(
            "Hostname of an individual DUT to lease. For example,"
            " chromeos8-row4-rack15-host32"
        ),
    )
    parser.add_argument(
        "--lease-minutes",
        nargs="?",
        type=int,
        default=60,
        help=("Duration of lease in minutes," " default is %(default)s"),
    )
    parser.add_argument(
        "--dut-host",
        nargs="?",
        type=str,
        help=(
            "The host of the DUT. For example, 100.000.0.001 or"
            " chromeos8-row4-rack15-host32"
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
            " will be uploaded; If not set, the default %(default)s"
            " will be used"
        ),
    )
    parser.add_argument(
        "--experiment-id",
        nargs="?",
        type=str,
        default="",
        help=(
            "The unique experiment identifier string. Use the same id for"
            " the same experiment. If uploaded to GCS, the test results"
            " and logs will be uploaded to"
            " gs://{bucket-name}/{experiment-id}/{dut-model}/,"
            " for example,"
            " gs://sw-perf-cuj-experiment/yz-lacros-vs-ash-R124-round1/"
            "voxel/20240131-151301/..."
        ),
    )
    parser.add_argument(
        "--label",
        nargs="?",
        type=str,
        default="",
        help=(
            "An arbitrary label that could be used to find the results"
            " of the run. e.g. 'my_great_test_without_cpu_cooling'."
            " This will be used to distinguish runs if other collected"
            " information can't be the unique identifier"
        ),
    )
    parser.add_argument(
        "--auto-upload",
        action="store_true",
        help=(
            "If set, the results will be automatically uploaded to"
            " the Google Cloud bucket without checking or asking;"
            " Otherwise, the program will check the experiment id and"
            " ask for user's permission to upload the results."
        ),
    )
    parser.add_argument(
        "--no-upload",
        action="store_true",
        help=(
            "If set, the results will only be in local and"
            "not uploaded to Google Cloud."
        ),
    )
    parser.add_argument(
        "--repeat",
        nargs="?",
        type=int,
        default=1,
        help=(
            "If set, `tast run` will be repeated for {--repeat} times,"
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
        "--reboot-wait",
        nargs="?",
        type=int,
        default=60,
        help=(
            "If set, wait --reboot-wait seconds for reboot,"
            " default is %(default)s."
        ),
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
        "--no-build",
        action="store_true",
        help=(
            "If set, Tast will use not build"
            " and use the test bundled with DUT chromeos."
        ),
    )
    parser.add_argument(
        "--results-dir",
        nargs="?",
        type=str,
        help=("Directory for test results."),
    )
    parser.add_argument(
        "pattern",
        nargs=argparse.REMAINDER,
        type=str,
        help=(
            "The tests pattern that will be used by <tast run>, e.g."
            " ui.IdlePerf.ash ui.IdlePerf.lacros or `(group:cuj ||"
            " group:cuj_experimental)`"
        ),
    )
    return parser.parse_args(argv)


def verify_arguments(opts, username):
    """Verify arguments values"""
    if not opts.experiment_id:
        raise ValueError("Experiment ID is required.")
    if not opts.pattern:
        raise ValueError("Tests pattern is required.")
    if opts.no_upload and opts.auto_upload:
        raise ValueError(
            "--no_upload and --auto_upload" " are mutually exclusive."
        )
    if not opts.lease_dims and not opts.lease_hostname and not opts.dut_host:
        raise ValueError(
            "Need to specify --lease_dims or --lease_hostname or --dut_host."
        )

    if not opts.auto_upload and check_experiment_id_existance(
        opts.bucket_name, username, opts.experiment_id
    ):
        while True:
            user_input = input(
                f"[Cloud] {username}/{opts.experiment_id} already exists,"
                " are you sure to use this experiment id?(y/n):"
            ).lower()
            if user_input == "y":
                break
            elif user_input == "n":
                raise ValueError("Try again with another experiment id.")
            else:
                logging.info("Enter y or n")


def main(argv) -> Optional[int]:
    """Main function"""
    opts = parse_arguments(argv)
    username = getpass.getuser()
    verify_arguments(opts, username)

    logging.basicConfig(format="%(asctime)s %(message)s", level=logging.INFO)
    logging.info("[User] %s starts run_cuj_tests.py...", username)

    try:
        run_ssh_helper()

        if opts.lease_dims or opts.lease_hostname:
            dut = crosfleet_dut_lease(
                opts.lease_hostname, opts.lease_dims, opts.lease_minutes
            )
        else:
            dut = opts.dut_host
        if not dut:
            raise ValueError("No DUT specified.")

        dut_model = get_model(dut)
        results_dir = f"/tmp/tast/results/{dut_model}"
        if opts.results_dir:
            results_dir = opts.results_dir
        if opts.reboot:
            reboot_dut(dut)
            logging.info(
                "[DUT] Waiting %s seconds for DUT to reboot", opts.reboot_wait
            )
            time.sleep(opts.reboot_wait)
        if opts.cooldown > 0:
            logging.info(
                "[DUT] Waiting %s seconds for DUT to cooldown", opts.cooldown
            )
            time.sleep(opts.cooldown)
        if opts.image:
            flash_image(opts.image, dut)
        else:
            logging.info("[Flash] Not flashing image")

        check_cpu_usage(dut)

        test_statuses = []
        results_dir_paths = []
        for i in range(opts.repeat):
            logging.info(f"[Tast] #{i+1} Running tests {opts.pattern}...")
            results_dir_path, statuses = run_tast_tests(
                dut,
                opts.no_build,
                opts.pattern,
                results_dir,
                opts.bundle,
                opts.vars,
            )
            write_local_dut_info(results_dir_path, opts.label)
            results_dir_paths.append(results_dir_path)
            test_statuses.extend(statuses)

        logging.info(
            "----------------------------------------"
            "----------------------------------------"
        )
        logging.info("[Tast] All tests completed:\n")
        for i in range(len(test_statuses)):
            logging.info(f"[Tast] *{i+1}: {test_statuses[i]}")
        logging.info(
            "----------------------------------------"
            "----------------------------------------"
        )

        if opts.no_upload:
            logging.info(
                "[Cloud] Skip uploading to Google Cloud"
                " because --no-upload is set"
            )
        else:
            upload = False
            if not opts.auto_upload:
                while True:
                    user_input = input(
                        "[Cloud] Are you sure to upload all test results to"
                        f" gs://{opts.bucket_name}/{username}/{opts.experiment_id}?(y/n):"
                    ).lower()
                    if user_input == "y":
                        upload = True
                        break
                    elif user_input == "n":
                        break
                    else:
                        logging.info("Enter y or n")
            if opts.auto_upload or upload:
                logging.info(
                    "[Cloud] Uploading to Google Cloud "
                    f"gs://{opts.bucket_name}/{username}/{opts.experiment_id}..."
                )
                for results_dir_path in results_dir_paths:
                    upload_latest_tests_results(
                        opts.bucket_name,
                        username,
                        opts.experiment_id,
                        dut_model,
                        results_dir_path,
                    )

    finally:
        crosfleet_dut_abandon()
        kill_ssh_helper()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
