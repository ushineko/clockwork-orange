#!/usr/bin/env python3
"""Capture golden fixtures from the Python 2.9.5 implementation (spec 010 R9.2).

Run from the repo root while the Python sources are still present:
    python3 tests/golden/capture.py
Outputs land next to this script. Deterministic: fixture images are generated
from fixed pixel data; timestamps in DBs are fixed constants.
"""
import hashlib
import io
import json
import os
import shutil
import sqlite3
import sys
import tempfile
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))

from PIL import Image  # noqa: E402

import config_migrations  # noqa: E402
import platform_utils  # noqa: E402
from plugins.blacklist import BlacklistManager  # noqa: E402
from plugins.history import HistoryManager  # noqa: E402
from plugins.wallhaven import WallhavenPlugin  # noqa: E402

FIXED_TS = 1700000000.5


def make_images():
    specs = {
        "landscape_1920x1080.png": ("RGB", (1920, 1080)),
        "portrait_1080x1920.png": ("RGB", (1080, 1920)),
        "square_600x600.png": ("RGB", (600, 600)),
        "small_800x600.jpg": ("RGB", (800, 600)),
        "rgba_400x300.png": ("RGBA", (400, 300)),
    }
    paths = []
    for name, (mode, size) in specs.items():
        img = Image.new(mode, size)
        px = img.load()
        w, h = size
        for y in range(0, h, 8):
            for x in range(0, w, 8):
                v = (x * 7 + y * 13) % 256
                colour = (v, (v * 3) % 256, (v * 5) % 256) + ((128,) if mode == "RGBA" else ())
                for yy in range(y, min(y + 8, h)):
                    for xx in range(x, min(x + 8, w)):
                        px[xx, yy] = colour
        p = OUT / "images" / name
        if name.endswith(".jpg"):
            img.save(p, "JPEG", quality=90)
        else:
            img.save(p, "PNG")
        paths.append(p)
    return paths


def capture_hashes(paths):
    bl = BlacklistManager(storage_dir=tempfile.mkdtemp())
    out = {}
    for p in paths:
        thumb = bl.generate_thumbnail(str(p))
        timg = Image.open(io.BytesIO(thumb))
        out[p.name] = {
            "md5": hashlib.md5(p.read_bytes()).hexdigest(),
            "sha256": bl.get_image_hash(str(p)),
            "thumbnail_size": list(timg.size),
            "thumbnail_format": timg.format,
        }
    (OUT / "images" / "hashes.json").write_text(json.dumps(out, indent=2, sort_keys=True) + "\n")


def capture_dbs(paths):
    dbdir = OUT / "db"
    for f in ("history.db", "blacklist.db"):
        (dbdir / f).unlink(missing_ok=True)
    with mock.patch("plugins.history.datetime") as dt:
        dt.now.return_value.timestamp.return_value = FIXED_TS
        h = HistoryManager(db_path=dbdir / "history.db")
        assert h.add_entry("https://example.com/a.jpg", str(paths[0]), source="wallhaven")
        assert h.add_entry("https://example.com/b.jpg", str(paths[1]), source="duckduckgo_images")
        assert h.add_entry("https://example.com/c.jpg", str(paths[0]), source="imported")
        assert not h.add_entry("https://example.com/a.jpg", str(paths[2]), source="dup")
    with mock.patch("plugins.blacklist.datetime") as dt:
        dt.now.return_value.timestamp.return_value = FIXED_TS
        dt.fromtimestamp.side_effect = __import__("datetime").datetime.fromtimestamp
        b = BlacklistManager(storage_dir=str(dbdir))
        b.add_to_blacklist(plugin_name="local", file_path=str(paths[3]))
        b.add_to_blacklist(image_hash="deadbeef" * 8, plugin_name="manual")
    expected = {
        "history": {
            "stats": h.get_stats(),
            "seen_url": {"https://example.com/a.jpg": True, "https://example.com/zzz.jpg": False},
            "seen_image": {paths[0].name: True, paths[2].name: False},
        },
        "blacklist": {
            "items": [
                {k: v for k, v in it.items() if k != "thumbnail"} | {"has_thumbnail": it["thumbnail"] is not None}
                for it in b.get_blacklist_items()
            ],
            "is_blacklisted": {b.get_image_hash(str(paths[3])): True, "deadbeef" * 8: True, "00" * 32: False},
        },
    }
    expected["history"]["stats"].pop("db_size_bytes", None)
    (dbdir / "expected.json").write_text(json.dumps(expected, indent=2, sort_keys=True, default=str) + "\n")


def capture_config():
    cdir = OUT / "config"
    pre = {
        "dual_wallpapers": True,
        "default_wait": 120,
        "plugins": {
            "google_images": {"enabled": True, "query": "mountains", "limit": 5},
            "stable_diffusion": {"enabled": False, "prompt": [{"term": "a castle", "enabled": True}], "steps": 30},
            "local": {"enabled": True, "path": "~/Pictures"},
        },
        "unknown_top_level": {"keep": "me"},
    }
    import yaml
    (cdir / "pre_migration.yml").write_text(yaml.dump(pre, default_flow_style=False, sort_keys=True))
    tmp = Path(tempfile.mkdtemp()) / "clockwork-orange.yml"
    shutil.copy(cdir / "pre_migration.yml", tmp)
    migrated = config_migrations.load_and_migrate(tmp)
    (cdir / "post_migration.yml").write_text(tmp.read_text())
    (cdir / "post_migration.json").write_text(json.dumps(migrated, indent=2, sort_keys=True) + "\n")
    # Already-migrated config with a download_dir must not be re-pinned.
    pre2 = {"plugins": {"google_images": {"enabled": True, "download_dir": "/data/wp"}}}
    tmp2 = tmp.parent / "two.yml"
    tmp2.write_text(yaml.dump(pre2, default_flow_style=False, sort_keys=True))
    config_migrations.load_and_migrate(tmp2)
    (cdir / "pre_migration_with_dir.yml").write_text(yaml.dump(pre2, default_flow_style=False, sort_keys=True))
    (cdir / "post_migration_with_dir.yml").write_text(tmp2.read_text())


def capture_platform(paths):
    pdir = OUT / "platform"
    calls = []

    def fake_run(cmd, **kw):
        calls.append(list(cmd))
        return mock.Mock(returncode=0, stdout="", stderr="")

    with mock.patch.object(platform_utils.subprocess, "run", fake_run):
        platform_utils._set_wallpaper_linux(paths[0])
        platform_utils._set_wallpaper_multi_monitor_linux([str(paths[0]), str(paths[1])])
        platform_utils._set_lockscreen_wallpaper_linux(paths[2])
    (pdir / "kde_commands.json").write_text(json.dumps(
        {"paths": [str(paths[0].resolve()), str(paths[1].resolve()), str(paths[2].resolve())],
         "single": calls[0], "multi": calls[1], "lockscreen": calls[2], "reload": calls[3]},
        indent=2) + "\n")


def capture_plugins():
    pdir = OUT / "plugins"
    wh = WallhavenPlugin()
    cases = {
        "defaults": {},
        "toplist": {"sorting": "toplist", "top_range": "1w"},
        "toplist_default_range": {"sorting": "toplist"},
        "no_people_nsfw": {"category_people": False, "purity_nsfw": True, "api_key": "KEY"},
        "empty_optionals": {"resolutions": "  ", "atleast": "", "ratios": " "},
        "all_optionals": {"resolutions": "1920x1080,2560x1440", "atleast": "3840x2160", "ratios": "16x9,21x9"},
    }
    table = {name: wh._build_api_params(cfg, "landscape") for name, cfg in cases.items()}
    parse = {
        "comma": wh._parse_queries("a, b ,c"),
        "list_dicts": wh._parse_queries([{"term": "x", "enabled": True}, {"term": "y", "enabled": False}, "z"]),
        "empty": wh._parse_queries(""),
        "empty_list": wh._parse_queries([]),
    }
    urls = ["https://example.com/img.jpg", "https://i.example.org/a/b/c?x=1&y=2", "http://x/ü.png"]
    ddg = {u: hashlib.md5(u.encode()).hexdigest() + ".jpg" for u in urls}
    (pdir / "wallhaven_params.json").write_text(json.dumps(table, indent=2, sort_keys=True) + "\n")
    (pdir / "wallhaven_parse_queries.json").write_text(json.dumps(parse, indent=2) + "\n")
    (pdir / "ddg_filenames.json").write_text(json.dumps(ddg, indent=2, sort_keys=True, ensure_ascii=False) + "\n")


def main():
    os.environ["TZ"] = "UTC"  # blacklist "date" strings are local-time; pin to UTC
    import time
    time.tzset()
    paths = make_images()
    capture_hashes(paths)
    capture_dbs(paths)
    capture_config()
    capture_platform(paths)
    capture_plugins()
    print("golden fixtures written to", OUT)


if __name__ == "__main__":
    main()
