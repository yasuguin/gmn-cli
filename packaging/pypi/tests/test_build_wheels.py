"""build_wheels.py の版の正規化のテスト（標準ライブラリの unittest）。

実行: python3 -m unittest discover -s packaging/pypi/tests
"""

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from build_wheels import normalize_version  # noqa: E402


class NormalizeVersionTest(unittest.TestCase):
    def test_正式版はそのまま(self):
        self.assertEqual(normalize_version("0.1.0"), "0.1.0")

    def test_プレリリースはPEP440の形にする(self):
        cases = {
            "0.1.0-rc.1": "0.1.0rc1",
            "0.1.0-rc1": "0.1.0rc1",
            "0.1.0-alpha.2": "0.1.0a2",
            "0.1.0-beta.3": "0.1.0b3",
            "1.2.3-a.1": "1.2.3a1",
            "1.2.3-b1": "1.2.3b1",
        }
        for given, want in cases.items():
            with self.subTest(given):
                self.assertEqual(normalize_version(given), want)

    def test_PEP440にできない版は止める(self):
        for bad in ["v0.1.0", "gmn-v0.1.0", "0.1", "0.1.0-next", "0.1.0-rc", "0.1.0+build", "0.1.0-rc.1.2", ""]:
            with self.subTest(bad):
                with self.assertRaises(SystemExit):
                    normalize_version(bad)


if __name__ == "__main__":
    unittest.main()
