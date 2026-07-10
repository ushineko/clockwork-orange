#!/usr/bin/env python
"""Tests for the DuckDuckGo Images plugin content filters.

Covers the changes that stop non-wallpaper content (ads, portrait photos of
people, square product/toy shots, hotlink placeholders) from being downloaded
and saved:

  - the landscape/aspect shape gate (`_is_wallpaper_shaped`)
  - the discovery filter (`_filter_results`): resolution + aspect + dedup, and
    carrying the source-page URL as a Referer
  - `_process_image` sending that Referer on the download and rejecting a
    non-landscape decoded image before it is force-cropped into a wallpaper

No network access: downloads are stubbed with a fake requests session and
PIL-generated images.
"""
import sys
import unittest
from contextlib import contextmanager
from io import BytesIO
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest import mock

from PIL import Image

REPO_ROOT = Path(__file__).resolve().parent.parent
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from plugins.duckduckgo_images import DuckDuckGoImagesPlugin


def _png_bytes(width: int, height: int) -> bytes:
    buf = BytesIO()
    Image.new("RGB", (width, height), (10, 20, 30)).save(buf, "PNG")
    return buf.getvalue()


class _FakeResponse:
    def __init__(self, content: bytes, status_code: int = 200):
        self.content = content
        self.status_code = status_code


class _FakeSession:
    """Records the last GET's headers and returns a canned image response."""

    def __init__(self, content: bytes):
        self._content = content
        self.last_headers = None

    @contextmanager
    def get(self, url, headers=None, timeout=None):
        self.last_headers = headers or {}
        yield _FakeResponse(self._content)


class TestShapeGate(unittest.TestCase):
    def test_landscape_shapes_accepted(self):
        for w, h in [(3840, 2160), (1920, 1080), (1920, 1200), (2560, 1080)]:
            self.assertTrue(
                DuckDuckGoImagesPlugin._is_wallpaper_shaped(w, h), f"{w}x{h}"
            )

    def test_non_wallpaper_shapes_rejected(self):
        # square (product/toy), portrait (person), ultra-wide, degenerate
        for w, h in [(2000, 2000), (1920, 2560), (5120, 1440), (0, 0)]:
            self.assertFalse(
                DuckDuckGoImagesPlugin._is_wallpaper_shaped(w, h), f"{w}x{h}"
            )


class TestFilterResults(unittest.TestCase):
    def setUp(self):
        self.plugin = DuckDuckGoImagesPlugin()

    def test_resolution_and_aspect_and_dedup(self):
        results = [
            {"image": "a.jpg", "url": "http://p/a", "width": 3840, "height": 2160},
            {"image": "sq.jpg", "url": "http://p/sq", "width": 2000, "height": 2000},
            {"image": "por.jpg", "url": "http://p/por", "width": 1920, "height": 2560},
            {"image": "lo.jpg", "url": "http://p/lo", "width": 800, "height": 600},
            {"image": "a.jpg", "url": "http://p/a2", "width": 3840, "height": 2160},
            {"image": "nd.jpg", "url": "http://p/nd"},  # no dims -> deferred
        ]
        out = self.plugin._filter_results(results)
        self.assertEqual([c["image"] for c in out], ["a.jpg", "nd.jpg"])

    def test_referer_is_source_page(self):
        out = self.plugin._filter_results(
            [{"image": "a.jpg", "url": "http://page/a", "width": 3840, "height": 2160}]
        )
        self.assertEqual(out[0]["referer"], "http://page/a")

    def test_missing_source_page_yields_empty_referer(self):
        out = self.plugin._filter_results(
            [{"image": "a.jpg", "width": 3840, "height": 2160}]
        )
        self.assertEqual(out[0]["referer"], "")


class TestProcessImage(unittest.TestCase):
    def setUp(self):
        self.plugin = DuckDuckGoImagesPlugin()
        # Managers are dedup/side-channel only; stub them out so no SQLite or
        # real hashing runs and every image is treated as new + not blacklisted.
        self.plugin.history_manager = mock.Mock()
        self.plugin.history_manager.seen_url.return_value = False
        self.plugin.history_manager.seen_image.return_value = False
        self.plugin.blacklist_manager = mock.Mock()
        self.plugin.blacklist_manager.get_image_hash.return_value = "hash"
        self.plugin.blacklist_manager.is_blacklisted.return_value = False

    def test_referer_header_sent_and_landscape_saved(self):
        self.plugin._session = _FakeSession(_png_bytes(3840, 2160))
        with TemporaryDirectory() as d:
            ok = self.plugin._process_image(
                "http://cdn/x.jpg", Path(d), referer="http://page/x"
            )
            self.assertTrue(ok)
            self.assertEqual(len(list(Path(d).glob("*.jpg"))), 1)
        self.assertEqual(self.plugin._session.last_headers.get("Referer"), "http://page/x")

    def test_non_landscape_download_rejected(self):
        # A portrait substitute (e.g. a hotlink placeholder) that reported no
        # dimensions at discovery must be rejected here, not force-cropped.
        self.plugin._session = _FakeSession(_png_bytes(1500, 3000))
        with TemporaryDirectory() as d:
            ok = self.plugin._process_image("http://cdn/y.jpg", Path(d))
            self.assertFalse(ok)
            self.assertEqual(list(Path(d).glob("*.jpg")), [])

    def test_content_duplicate_records_url_then_removes_file(self):
        # A landscape image whose *content* is already in history: the URL must
        # still be recorded (so it is skipped without re-download next run) and
        # the redundant file removed. The real HistoryManager.add_entry hashes
        # the file, so it must be called while the file still exists — deleting
        # first (the old bug) would make it raise FileNotFoundError.
        self.plugin.history_manager.seen_image.return_value = True
        existed_at_call = {}

        def _record(url, path, source=None):
            existed_at_call["value"] = Path(path).exists()

        self.plugin.history_manager.add_entry.side_effect = _record
        self.plugin._session = _FakeSession(_png_bytes(3840, 2160))
        with TemporaryDirectory() as d:
            ok = self.plugin._process_image(
                "http://cdn/dup.jpg", Path(d), referer="http://page/dup"
            )
            self.assertFalse(ok)
            self.assertEqual(list(Path(d).glob("*.jpg")), [])  # cleaned up
        self.plugin.history_manager.add_entry.assert_called_once()
        # The ordering guarantee: file present when the URL was recorded.
        self.assertTrue(existed_at_call.get("value"))


if __name__ == "__main__":
    unittest.main()
