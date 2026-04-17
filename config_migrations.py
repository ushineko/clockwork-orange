#!/usr/bin/env python3
"""
One-time config migrations applied on every config load.

Each migration is idempotent: running a second time after the change has landed
is a no-op. Migrations mutate the passed-in dict and return True if anything
changed, so callers know whether to re-persist the file.
"""

from pathlib import Path

import yaml

OLD_GOOGLE_IMAGES_DEFAULT_DIR = str(
    Path.home() / "Pictures" / "Wallpapers" / "GoogleImages"
)


def migrate_google_to_duckduckgo(config: dict) -> bool:
    """Rename plugins.google_images -> plugins.duckduckgo_images.

    Preserves every field in the block. If the old block had no explicit
    download_dir, pins it to the old Google Images default path so any
    wallpapers the user already downloaded remain visible to the wallpaper
    engine (which iterates enabled plugins' download_dirs).

    Returns True iff the config dict was mutated.
    """
    plugins = config.get("plugins")
    if not isinstance(plugins, dict):
        return False
    if "google_images" not in plugins or "duckduckgo_images" in plugins:
        return False

    block = plugins.pop("google_images")
    if isinstance(block, dict) and not block.get("download_dir"):
        block["download_dir"] = OLD_GOOGLE_IMAGES_DEFAULT_DIR
    plugins["duckduckgo_images"] = block
    return True


_MIGRATIONS = [
    migrate_google_to_duckduckgo,
]


def apply_migrations(config: dict) -> bool:
    """Run every registered migration. Returns True if any mutated the config."""
    mutated = False
    for migration in _MIGRATIONS:
        if migration(config):
            mutated = True
    return mutated


def load_and_migrate(config_path: Path) -> dict:
    """Load a YAML config file, apply migrations, and persist changes back.

    On a persist failure we log to stderr and return the in-memory migrated
    config. The on-disk file still has the old key, so the migration will be
    retried on the next load.
    """
    import sys

    with open(config_path, "r") as f:
        config = yaml.safe_load(f) or {}

    if apply_migrations(config):
        try:
            with open(config_path, "w") as f:
                yaml.safe_dump(config, f, sort_keys=True)
            print(
                f"[config-migration] Persisted migrated config to {config_path}",
                file=sys.stderr,
            )
        except Exception as e:
            print(
                f"[config-migration] Failed to persist migrated config to "
                f"{config_path}: {e}. Next startup will retry the migration.",
                file=sys.stderr,
            )

    return config
