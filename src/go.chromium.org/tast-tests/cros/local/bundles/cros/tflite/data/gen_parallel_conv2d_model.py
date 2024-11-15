#!/usr/bin/env python3
# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""A helper script to generate the test model."""

import argparse
import pathlib
import sys
from typing import List, Optional

import ai_edge_torch
from ai_edge_torch.quantize import pt2e_quantizer
from ai_edge_torch.quantize import quant_config
import torch
from torch.ao.quantization import quantize_pt2e
import torch.nn as nn


class ParallelConv2d(nn.Module):
    def __init__(
        self,
        num_conv2d: int = 4,
        in_channel: int = 6,
        out_channel: int = 16,
        kernel_size: int = 16,
    ) -> None:
        super(ParallelConv2d, self).__init__()

        self.num_conv2d = num_conv2d
        for i in range(self.num_conv2d):
            setattr(
                self,
                f"conv2ds_{i}",
                nn.Conv2d(
                    in_channel,
                    out_channel,
                    kernel_size,
                    padding=kernel_size - 1,
                    bias=False,
                ),
            )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        outputs = [
            getattr(self, f"conv2ds_{i}")(x) for i in range(self.num_conv2d)
        ]
        return sum(outputs)


def setup_argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        formatter_class=argparse.ArgumentDefaultsHelpFormatter
    )
    parser.add_argument(
        "--output",
        default="model.tflite",
        type=pathlib.Path,
        help="output path to the generated model",
    )
    parser.add_argument(
        "--seed",
        default=1234,
        type=int,
        help="random seed for initializing the model",
    )
    parser.add_argument(
        "--num_conv2d",
        default=4,
        type=int,
        help="number of the conv2d operation in the model",
    )
    parser.add_argument(
        "--in_channel", default=6, type=int, help="in_channel in Conv2d"
    )
    parser.add_argument(
        "--out_channel", default=16, type=int, help="out_channel in Conv2d"
    )
    parser.add_argument(
        "--kernel_size", default=16, type=int, help="kernel_size in Conv2d"
    )
    parser.add_argument(
        "--batch_size",
        default=32,
        type=int,
        help="batch size of the input tensor",
    )
    parser.add_argument(
        "--input_size", default=384, type=int, help="size of the input tensor"
    )
    return parser


def main(argv: Optional[List[str]] = None) -> Optional[int]:
    parser = setup_argument_parser()
    args = parser.parse_args(argv)

    torch.manual_seed(args.seed)

    model = ParallelConv2d(
        args.num_conv2d,
        args.in_channel,
        args.out_channel,
        args.kernel_size,
    )
    input_shape = (
        args.batch_size,
        args.in_channel,
        args.input_size,
        args.input_size,
    )
    sample_input = torch.rand(*input_shape)

    edge_model = ai_edge_torch.convert(model.eval(), (sample_input,))
    edge_model.export(args.output)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
