"""Regression tests for the measurement tool, without model weights."""

import base64
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
import wave


MODULE = Path(__file__).parents[1] / "benchmark_voxcpm.py"


class BenchmarkTests(unittest.TestCase):
    def load(self):
        self.assertTrue(MODULE.exists(), "measurement tool is not implemented")
        spec = importlib.util.spec_from_file_location("benchmark_voxcpm", MODULE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def stream(self, events):
        return io.BytesIO(b"".join(json.dumps(e).encode() + b"\n" for e in events))

    def events(self):
        return [
            {"type": "start", "version": 1, "sample_rate": 16000, "channels": 1, "encoding": "pcm_s16le"},
            {"type": "audio", "seq": 0, "data": base64.b64encode(b"\x01\x00" * 1600).decode()},
            {"type": "end", "chunks": 1, "samples": 1600},
        ]

    def test_stream_is_measured_before_end_and_saved_as_valid_wav(self):
        tool = self.load()
        ticks = iter([12.0])
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "speech.wav"
            result = tool.read_stream(self.stream(self.events()), output, started=10.0, clock=lambda: next(ticks))
            self.assertEqual(result["first_pcm_seconds"], 2.0)
            self.assertEqual(result["audio_seconds"], 0.1)
            with wave.open(str(output)) as audio:
                self.assertEqual(audio.getnframes(), 1600)
                self.assertEqual(audio.getframerate(), 16000)

    def test_truncated_or_corrupt_stream_does_not_publish_listening_artifact(self):
        tool = self.load()
        mutations = [self.events()[:-1], self.events()]
        mutations[1][-1]["samples"] = 1599
        bad_sequence = self.events()
        bad_sequence[1]["seq"] = 2
        mutations.append(bad_sequence)
        bad_pcm = self.events()
        bad_pcm[1]["data"] = "!bad!"
        mutations.append(bad_pcm)
        for events in mutations:
            with self.subTest(events=events[-1]), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / "speech.wav"
                with self.assertRaises(ValueError):
                    tool.read_stream(self.stream(events), output, started=0)
                self.assertFalse(output.exists())

    def test_manifest_has_ten_cases_per_length_group_and_stable_content_hashes(self):
        tool = self.load()
        cases = tool.load_cases(Path(__file__).parents[2] / "documents/benchmarks/voxcpm-cases.json")
        self.assertEqual(len(cases), 30)
        self.assertEqual([sum(c["group"] == group for c in cases) for group in ("short", "medium", "long")], [10, 10, 10])
        for case in cases:
            self.assertEqual(len(case["sha256"]), 64)


if __name__ == "__main__":
    unittest.main()
