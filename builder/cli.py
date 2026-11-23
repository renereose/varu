"""Command line: python3 -m builder --region ee --out /path/to/drive"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

from . import __version__
from .app import step_app
from .binaries import step_kiwix, step_launcher
from .config import ConfigError, load_region, load_sources
from .context import Context, StepError
from .data import step_documents, step_library, step_maps, step_pois, step_routing
from .manifest import step_manifest

# Order matters only for manifest, which describes what the others produced.
STEPS = {
    "app": "the viewer, its libraries, map fonts and icons",
    "launcher": "the start program for Windows, macOS and Linux",
    "kiwix": "the library reader (kiwix-serve) for each system",
    "maps": "the region's map, cut from the Protomaps planet build",
    "pois": "hospitals, pharmacies and other place layers from OpenStreetMap",
    "routing": "the road network for offline directions, built from an OpenStreetMap extract",
    "library": "Wikipedia and other ZIM archives from the Kiwix catalogue",
    "documents": "documents listed in the region file",
    "manifest": "manifest.json, the list of what is on the drive",
}


def default_cache() -> Path:
    base = os.environ.get("XDG_CACHE_HOME") or (Path.home() / ".cache")
    return Path(base) / "varu"


def parse_steps(value: str) -> list[str]:
    names = [name.strip() for name in value.split(",") if name.strip()]
    unknown = [name for name in names if name not in STEPS]
    if unknown:
        raise argparse.ArgumentTypeError(
            f"unknown step(s): {', '.join(unknown)} (choose from {', '.join(STEPS)})")
    return names


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python3 -m builder",
        description="Assemble a Varu offline drive for one region.",
        epilog="Steps: " + "; ".join(f"{name} – {text}" for name, text in STEPS.items()),
    )
    parser.add_argument("--region", required=True, help="region id (regions/<id>.toml) or a path to a region file")
    parser.add_argument("--out", required=True, type=Path,
                        help="where to assemble the drive: the mounted USB drive, or any folder")
    parser.add_argument("--only", type=parse_steps, metavar="STEPS", help="run only these steps (comma-separated)")
    parser.add_argument("--skip", type=parse_steps, metavar="STEPS", default=[], help="leave these steps out")
    parser.add_argument("--refresh", action="store_true", help="fetch maps, place layers and road data again even if present")
    parser.add_argument("--dry-run", action="store_true", help="show what would be done without downloading anything")
    parser.add_argument("--planet-build", metavar="YYYYMMDD", help="use this Protomaps daily build instead of the newest")
    parser.add_argument("--launcher-dir", type=Path, metavar="DIR",
                        help="use prebuilt launchers from DIR/<os>-<arch>/ instead of compiling with Go")
    parser.add_argument("--cache", type=Path, default=default_cache(), help="download cache (default: %(default)s)")
    parser.add_argument("--version", action="version", version=f"varu builder {__version__}")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    repo = Path(__file__).resolve().parent.parent

    try:
        region = load_region(args.region, repo)
        sources = load_sources(repo)
    except (ConfigError, OSError) as err:
        print(f"error: {err}", file=sys.stderr)
        return 2

    ctx = Context(
        repo=repo, out=args.out.resolve(), cache=args.cache.resolve(),
        region=region, sources=sources, dry_run=args.dry_run,
        planet_build=args.planet_build, launcher_dir=args.launcher_dir,
    )

    selected = [name for name in STEPS if name in (args.only or STEPS) and name not in args.skip]
    runners = {
        "app": step_app,
        "launcher": step_launcher,
        "kiwix": step_kiwix,
        "maps": lambda c: step_maps(c, refresh=args.refresh),
        "pois": lambda c: step_pois(c, refresh=args.refresh),
        "routing": lambda c: step_routing(c, refresh=args.refresh),
        "library": step_library,
        "documents": step_documents,
        "manifest": step_manifest,
    }

    print(f"Building '{region.id}' into {ctx.out}" + (" (dry run)" if ctx.dry_run else ""))
    if not ctx.dry_run:
        ctx.out.mkdir(parents=True, exist_ok=True)

    failed: dict[str, str] = {}
    for name in selected:
        ctx.step = name
        try:
            runners[name](ctx)
        except StepError as err:
            failed[name] = str(err)
            ctx.log(f"FAILED: {err}")
        except KeyboardInterrupt:
            print("\nInterrupted. Run the same command again to continue; finished downloads are kept.")
            return 130
    ctx.step = ""

    print()
    if failed:
        print(f"Finished with problems in: {', '.join(failed)}.")
        print("The drive still works with whatever was built. Fix the cause and run again; "
              "completed parts are not repeated.")
        return 1
    print("Dry run complete." if ctx.dry_run else f"Done. Start it with the Start-* file in {ctx.out}")
    return 0
