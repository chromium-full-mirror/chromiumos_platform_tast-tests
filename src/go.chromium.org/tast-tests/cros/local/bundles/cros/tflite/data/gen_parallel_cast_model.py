#!/usr/bin/env python3
# Copyright 2025 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""A helper script to generate the model with multiple cast operations."""

import argparse
import pathlib
import sys
from typing import List, Optional

import ai_edge_torch
import torch
import torch.nn as nn


class CastModel(nn.Module):
    def __init__(self, num_ops: int = 2) -> None:
        super(CastModel, self).__init__()
        self.num_ops = num_ops

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        xs = []
        for i in range(self.num_ops):
            xs.append(x[i].to(torch.float32))
        return sum(xs)


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
        "--num_ops",
        default=2,
        type=int,
        help="number of the cast operation in the model",
    )
    # We make the input tensor 2D (w/o the outer dimension) to increase MVPU's load.
    # 2D tensors have more MVPU utilization than 1D tensor is observed in local experiemnts.
    parser.add_argument(
        "--input_size1",
        default=512,
        type=int,
        help="size of the first dimension of the input tensor",
    )
    parser.add_argument(
        "--input_size2",
        default=512,
        type=int,
        help="size of the second dimension of the input tensor",
    )

    return parser


def main(argv: Optional[List[str]] = None) -> Optional[int]:
    parser = setup_argument_parser()
    args = parser.parse_args(argv)

    model = CastModel(args.num_ops)
    input_shape = (
        args.num_ops,
        args.input_size1,
        args.input_size2,
    )
    sample_input = torch.zeros(*input_shape, dtype=torch.int32)

    model = CastModel()
    edge_model = ai_edge_torch.convert(model.eval(), (sample_input,))
    edge_model.export(args.output)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
