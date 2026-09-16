# Golden fixtures (spec 010 R9.2)

Captured from the Python 2.9.5 implementation at commit `67ad8d2` by running,
from the repository root while the Python sources are still present:

```
python3 tests/golden/capture.py
```

The Go tests read these files; they never regenerate them. Contents:

| Path | What |
|------|------|
| `images/*.png`, `*.jpg` | Deterministic fixture images (pixel data from a fixed formula). |
| `images/hashes.json` | MD5 (history) and streamed SHA-256 (blacklist) of each image, plus the Pillow thumbnail size/format (`thumbnail((128,128))`, JPEG q70). |
| `db/history.db`, `db/blacklist.db` | SQLite files written by `plugins/history.py` / `plugins/blacklist.py` with fixed timestamps (`1700000000.5`). |
| `db/expected.json` | What the Python managers report for those DBs (`get_stats`, `seen_url`, `seen_image`, `get_blacklist_items`, `is_blacklisted`). `date` strings were captured with `TZ=UTC`; Go tests must format in UTC. `db_size_bytes` is omitted (driver-dependent). |
| `config/pre_migration*.yml`, `post_migration*.yml`, `post_migration.json` | `config_migrations.load_and_migrate` input/output for the `google_images` -> `duckduckgo_images` migration, including an unknown `stable_diffusion` block and an unknown top-level key. |
| `platform/kde_commands.json` | Exact `qdbus6` and `kwriteconfig6` argv for single, multi-monitor and lock-screen sets. `paths` lists the absolute paths that were interpolated; Go tests substitute their own paths for those strings before comparing. |
| `plugins/wallhaven_params.json` | `_build_api_params` output table. |
| `plugins/wallhaven_parse_queries.json` | `_parse_queries` output for comma strings, dict lists and empties. |
| `plugins/ddg_filenames.json` | DuckDuckGo `md5(url).jpg` filename derivation. |

`capture.py` is the last Python in the repository allowed to survive until the
Phase 7 cutover, when it is deleted along with the sources it imports; the
fixtures it produced stay.
