"""Wrapper tests; run from wrappers/pypi with
``python3 -m unittest discover -s tests``."""

import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))
import skillxp  # noqa: E402

FAKE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "fake_skillxp.py")


class WrapperTest(unittest.TestCase):
    def setUp(self):
        self._saved = os.environ.copy()

    def tearDown(self):
        os.environ.clear()
        os.environ.update(self._saved)

    def test_binary_path_honors_override(self):
        os.environ["SKILLXP_BINARY"] = FAKE
        self.assertEqual(skillxp.binary_path(), FAKE)

    def test_binary_path_without_bundled_binary_raises(self):
        # The repo checkout has no src/skillxp/bin/, so resolution must
        # fail with the actionable NotInstalledError.
        os.environ.pop("SKILLXP_BINARY", None)
        with self.assertRaises(skillxp.NotInstalledError):
            skillxp.binary_path()


if __name__ == "__main__":
    unittest.main()
