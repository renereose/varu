"""The `app` step: the viewer, its JavaScript libraries, map fonts and icons."""

from __future__ import annotations

import shutil
import subprocess
import tarfile
from pathlib import Path

from .context import Context, StepError
from .fetch import FetchError, download, fetch_bytes


def step_app(ctx: Context) -> None:
    app = ctx.out / "app"
    if ctx.dry_run:
        ctx.log(f"would copy viewer/ to {app}")
        for package in ctx.sources["npm"]:
            ctx.log(f"would vendor {package['name']} {package['version']}")
        ctx.log(f"would fetch map fonts and icons at {ctx.sources['assets']['commit'][:10]}")
        return

    ctx.log("copying viewer")
    shutil.copytree(ctx.repo / "viewer", app, dirs_exist_ok=True,
                    ignore=shutil.ignore_patterns(".*", "vendor", "assets"))

    licenses = ctx.out / "licenses"
    licenses.mkdir(parents=True, exist_ok=True)
    notices = [
        "Third-party software on this drive",
        "==================================",
        "",
    ]

    for package in ctx.sources["npm"]:
        _vendor_package(ctx, package, app / "vendor", licenses)
        notices.append(f"{package['name']} {package['version']} – {package['license']} – {package['homepage']}")

    _install_assets(ctx, app / "assets", licenses)
    assets = ctx.sources["assets"]
    notices += [
        f"Protomaps basemaps-assets ({assets['commit'][:10]}) – fonts: SIL OFL 1.1, icons: MIT – {assets['repo']}",
        "klauspost/compress (zstd, in bin/*/varu) – BSD-3-Clause – https://github.com/klauspost/compress",
        "",
        "Full licence texts are in this folder where the upstream project ships one.",
        "",
    ]
    (licenses / "THIRD-PARTY.txt").write_text("\n".join(notices), encoding="utf-8")


def extract_members(archive: Path, wanted: dict[str, Path]) -> None:
    """Copy named members of a tar archive to explicit destinations.

    Nothing is ever extracted by its own path, so a hostile archive cannot
    write outside the places we choose.
    """
    remaining = dict(wanted)
    with tarfile.open(archive) as tar:
        for member in tar:
            target = remaining.pop(member.name, None)
            if target is None or not member.isfile():
                continue
            source = tar.extractfile(member)
            if source is None:
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            with source, target.open("wb") as out:
                shutil.copyfileobj(source, out)
    if remaining:
        raise StepError(f"{archive.name}: missing {', '.join(sorted(remaining))}")


def _vendor_package(ctx: Context, package: dict, vendor: Path, licenses: Path) -> None:
    name, version = package["name"], package["version"]
    safe = name.replace("@", "").replace("/", "-")
    try:
        tarball = download(package["url"], ctx.cache / "npm" / f"{safe}-{version}.tgz",
                           integrity=package["integrity"], log=ctx.log)
    except FetchError as err:
        raise StepError(str(err)) from err

    wanted = {member: vendor / target for member, target in package["files"].items()}
    if "license_file" in package:
        wanted[package["license_file"]] = licenses / f"{safe}.txt"
    extract_members(tarball, wanted)

    if "license_url" in package:
        try:
            (licenses / f"{safe}.txt").write_bytes(fetch_bytes(package["license_url"]))
        except FetchError as err:
            ctx.log(f"warning: could not fetch the licence text for {name}: {err}")
    ctx.log(f"vendored {name} {version}")


def _install_assets(ctx: Context, dest: Path, licenses: Path) -> None:
    assets = ctx.sources["assets"]
    checkout = _assets_checkout(ctx, assets["repo"], assets["commit"])
    for rel in assets["paths"]:
        source = checkout / rel
        if not source.is_dir():
            raise StepError(f"basemaps-assets has no '{rel}' at {assets['commit'][:10]}")
        shutil.copytree(source, dest / rel, dirs_exist_ok=True)
    font_licence = checkout / "fonts" / "OFL.txt"
    if font_licence.is_file():
        shutil.copyfile(font_licence, licenses / "fonts-OFL.txt")
    ctx.log("installed map fonts and icons")


def _assets_checkout(ctx: Context, repo: str, commit: str) -> Path:
    """Return a cached copy of the assets repository at the pinned commit."""
    checkout = ctx.cache / "assets" / commit
    marker = checkout / ".complete"
    if marker.exists():
        return checkout
    if checkout.exists():
        shutil.rmtree(checkout)
    checkout.mkdir(parents=True)

    try:
        archive = download(f"{repo}/archive/{commit}.tar.gz",
                           ctx.cache / "assets" / f"{commit}.tar.gz", log=ctx.log)
        _extract_tree(archive, checkout)
    except (FetchError, tarfile.TarError) as err:
        # Some networks block archive downloads but allow git.
        ctx.log(f"archive download failed ({err}); trying git")
        git = shutil.which("git")
        if git is None:
            raise StepError("could not download map fonts and icons, and git is not installed") from err
        shutil.rmtree(checkout)
        checkout.mkdir(parents=True)
        for command in (
            [git, "init", "-q"],
            [git, "fetch", "-q", "--depth", "1", repo, commit],
            [git, "checkout", "-q", "FETCH_HEAD"],
        ):
            result = subprocess.run(command, cwd=checkout, capture_output=True, text=True)
            if result.returncode != 0:
                raise StepError(f"git failed: {result.stderr.strip() or result.stdout.strip()}") from err
    marker.touch()
    return checkout


def _extract_tree(archive: Path, dest: Path) -> None:
    """Unpack a GitHub source archive, dropping its single top-level folder."""
    root = dest.resolve()
    with tarfile.open(archive) as tar:
        for member in tar:
            parts = Path(member.name).parts[1:]
            if not parts or not (member.isfile() or member.isdir()):
                continue
            target = (dest / Path(*parts)).resolve()
            if root not in target.parents and target != root:
                raise StepError(f"{archive.name}: unsafe path {member.name!r}")
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            source = tar.extractfile(member)
            if source is None:
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            with source, target.open("wb") as out:
                shutil.copyfileobj(source, out)
