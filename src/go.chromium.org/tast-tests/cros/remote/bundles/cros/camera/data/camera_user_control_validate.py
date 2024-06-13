#!/usr/bin/env python3
# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""A tool to use image recognition technique to validate camera user control on a DUT."""

import argparse
import os
import sys

import cv2
from google.protobuf import text_format
import manual_control_image_config_pb2 as pb
import numpy as np


def calculateBrightnessMetrics(img: np.ndarray) -> float:
    hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
    _, _, v = cv2.split(hsv)
    return np.mean(v)


def calculateContrastMetrics(img: np.ndarray) -> float:
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    return np.std(gray)


def calculateSaturationMetrics(img: np.ndarray) -> float:
    hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
    _, s, _ = cv2.split(hsv)
    return np.mean(s)


def calculateHueMetrics(img: np.ndarray) -> float:
    hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
    h, s, _ = cv2.split(hsv)

    # Only consider if saturation is high enough for hue to be meaningful.
    mask = s >= 128
    valid_h = h[mask]

    if valid_h.size == 0:
        return 0

    # Transform the range of hue from 0-180 to 0-360 so we can do a vector
    # averaging.
    valid_h_radians = np.radians(valid_h * 2)
    valid_h_x_mean = np.mean(np.cos(valid_h_radians))
    valid_h_y_mean = np.mean(np.sin(valid_h_radians))
    return np.degrees(np.arctan2(valid_h_y_mean, valid_h_x_mean)) % 360


def calculateSharpnessMetrics(img: np.ndarray) -> float:
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    laplacian = cv2.Laplacian(img, cv2.CV_64F)
    return np.var(laplacian)


def calculateWhiteBalanceMetrics(img: np.ndarray) -> float:
    avg_b, avg_g, avg_r = np.mean(img, axis=(0, 1))
    # Gray Word Assumption
    avg_gray = (avg_b + avg_g + avg_r) / 3
    red_gain = avg_gray / avg_r
    blue_gain = avg_gray / avg_b
    temperature = 4500 + (blue_gain - red_gain) * 2000
    return temperature


def calculateMetricsByImage(
    control_name: pb.Control, image_path_prefix: str, level: int
) -> float:
    img = cv2.imread(image_path_prefix + "_" + str(level) + ".jpg")
    if control_name == pb.Control.BRIGHTNESS:
        return calculateBrightnessMetrics(img)
    if control_name == pb.Control.CONTRAST:
        return calculateContrastMetrics(img)
    if control_name == pb.Control.SATURATION:
        return calculateSaturationMetrics(img)
    if control_name == pb.Control.SHARPNESS:
        return calculateSharpnessMetrics(img)
    if control_name == pb.Control.WHITE_BALANCE:
        return calculateWhiteBalanceMetrics(img)
    if control_name == pb.Control.HUE:
        return calculateHueMetrics(img)
    raise RuntimeError("Unknown control name: %s", control_name)


def validateByLevelDistanceMetrics(
    control_name: pb.Control, metrics: list, level_distance: float
) -> bool:
    # Directly let the test pass if the level distance is 0.
    if level_distance <= 1e-7:
        return True

    if control_name in [
        pb.Control.BRIGHTNESS,
        pb.Control.CONTRAST,
        pb.Control.SATURATION,
        pb.Control.SHARPNESS,
    ]:
        return all(
            metrics[i + 1] - metrics[i] >= level_distance
            for i in range(len(metrics) - 1)
        )
    if control_name == pb.Control.WHITE_BALANCE:
        return all(
            metrics[i] - metrics[i + 1] >= level_distance
            for i in range(len(metrics) - 1)
        )
    if control_name == pb.Control.HUE:
        return all(
            abs(metrics[i] - metrics[i + 1]) >= level_distance
            for i in range(len(metrics) - 1)
        )
    raise RuntimeError("Unknown control name: %s", control_name)


def validateByMetrics(
    control_name: pb.Control,
    metrics: list,
    level_distance: float,
    min_max_distance: float,
    report: pb.ManualControlReport,
) -> None:
    report.level_distance_test_result = validateByLevelDistanceMetrics(
        control_name, metrics, level_distance
    )
    report.min_max_distance_test_result = (
        max(metrics) - min(metrics) >= min_max_distance
    )


def validateByConfig(
    config: pb.ManualControlImageConfig,
) -> pb.ManualControlReport:
    report = pb.ManualControlReport()
    report.control_name = config.control_name
    report.vid_pid = config.vid_pid

    metrics = []
    for i in range(config.num_level):
        metrics.append(
            calculateMetricsByImage(
                config.control_name, config.image_path_prefix, i
            )
        )
    report.raw_metrics.extend(metrics)

    validateByMetrics(
        config.control_name,
        metrics,
        config.level_distance,
        config.min_max_distance,
        report,
    )
    return report


def validateByConfigs(config_path: str) -> None:
    configs = pb.ManualControlImageConfigs()
    reports = pb.ManualControlReports()
    with open(config_path, "r") as f:
        text_format.Merge(f.read(), configs)
    for config in configs.configs:
        reports.reports.append(validateByConfig(config))
    with open(configs.report_output_path, "w") as f:
        text_format.PrintMessage(reports, f)


def main(argv: list):
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--config_path",
        action="store",
        type=str,
        required=True,
        help="Configs to read images and parameters.",
    )

    args = parser.parse_args(argv)
    validateByConfigs(args.config_path)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
