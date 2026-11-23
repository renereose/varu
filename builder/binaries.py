"""The `launcher` and `kiwix` steps: everything that ends up in bin/."""

from __future__ import annotations

import os
import shutil
import subprocess
import tarfile
import zipfile
from pathlib import Path

from . import __version__
from .context import Context, StepError
from .fetch import FetchError, download

START_WINDOWS = (
    "@echo off\r\n"
    "rem Starts Varu. Keep the window that opens; closing it stops Varu.\r\n"
    'start "Varu" "%~dp0bin\\windows-amd64\\varu.exe"\r\n'
)

START_MACOS = """#!/bin/sh
# Starts Varu. Keep this window open; closing it stops Varu.
here="$(cd "$(dirname "$0")" && pwd)"
case "$(uname -m)" in
  arm64) arch=arm64 ;;
  *) arch=amd64 ;;
esac
exec "$here/bin/darwin-$arch/varu" -root "$here"
"""

START_LINUX = """#!/bin/sh
# Starts Varu. Keep this window open; closing it stops Varu.
here="$(cd "$(dirname "$0")" && pwd)"
case "$(uname -m)" in
  aarch64|arm64) arch=arm64 ;;
  *) arch=amd64 ;;
esac
"$here/bin/linux-$arch/varu" -root "$here"
status=$?
if [ "$status" -eq 126 ]; then
  echo
  echo "This drive is mounted without permission to run programs."
  echo "Copy the drive's contents to your home folder and start it from there,"
  echo "or remount the drive with the 'exec' option."
fi
exit "$status"
"""

README_TXT = """VARU
====

EESTI KEELES
  Windows:  topeltklõps failil Start-Windows.bat
  macOS:    topeltklõps failil Start-macOS.command
  Linux:    käivita terminalis:  sh Start-Linux.sh

  Avaneb aken ja seejärel veebilehitseja. Internetti pole vaja.
  Hoia avanenud aken lahti; selle sulgemine peatab Varu.

IN ENGLISH
  Windows:  double-click Start-Windows.bat
  macOS:    double-click Start-macOS.command
  Linux:    in a terminal, run:  sh Start-Linux.sh

  A window opens, then your web browser. No internet is needed.
  Keep the window open; closing it stops Varu.

  To let phones and other computers on the same Wi-Fi use this drive,
  start the program in bin/ with the option  -lan  and open the "Network"
  address it prints.

Licences and credits are in the licenses folder.
"""


def step_launcher(ctx: Context) -> None:
    targets: list[str] = ctx.sources["launcher"]["targets"]

    if ctx.dry_run:
        how = f"copy from {ctx.launcher_dir}" if ctx.launcher_dir else "compile with Go"
        ctx.log(f"would {how} for {', '.join(targets)} and write the start scripts")
        return

    if ctx.launcher_dir:
        _copy_prebuilt(ctx, targets)
    else:
        _compile(ctx, targets)

    scripts = {
        "Start-Windows.bat": START_WINDOWS,
        "Start-macOS.command": START_MACOS,
        "Start-Linux.sh": START_LINUX,
        "README.txt": README_TXT,
    }
    for name, body in scripts.items():
        path = ctx.out / name
        # newline="" keeps the .bat file's CRLF and the shell scripts' LF as written.
        with path.open("w", encoding="utf-8", newline="") as handle:
            handle.write(body)
        if not name.endswith((".txt", ".bat")):
            path.chmod(0o755)  # has no effect on exFAT, which is fine
    ctx.log("wrote start scripts")


def _binary_name(target: str, base: str) -> str:
    return base + (".exe" if target.startswith("windows-") else "")


def go_command(hint: str = "") -> str:
    go = shutil.which("go")
    if go is None:
        raise StepError("Go is not installed. Install it from https://go.dev/dl/" + hint)
    return go


def _compile(ctx: Context, targets: list[str]) -> None:
    go = go_command(" or pass --launcher-dir with prebuilt launchers.")
    for target in targets:
        goos, goarch = target.split("-", 1)
        output = ctx.out / "bin" / target / _binary_name(target, "varu")
        output.parent.mkdir(parents=True, exist_ok=True)
        result = subprocess.run(
            [go, "build", "-trimpath", "-ldflags", f"-s -w -X main.version={__version__}",
             "-o", str(output), "."],
            cwd=ctx.repo / "launcher",
            env={**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"},
            capture_output=True, text=True,
        )
        if result.returncode != 0:
            raise StepError(f"go build for {target} failed:\n{result.stderr.strip()}")
        ctx.log(f"built launcher for {target}")


def _copy_prebuilt(ctx: Context, targets: list[str]) -> None:
    for target in targets:
        name = _binary_name(target, "varu")
        source = ctx.launcher_dir / target / name
        if not source.is_file():
            raise StepError(f"--launcher-dir: {source} not found")
        output = ctx.out / "bin" / target / name
        output.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, output)
        output.chmod(0o755)
        ctx.log(f"copied launcher for {target}")


def step_kiwix(ctx: Context) -> None:
    tools = ctx.sources["kiwix_tools"]
    version = tools["version"]
    failures = []
    for target, pattern in tools["targets"].items():
        filename = pattern.format(version=version)
        url = f"{tools['base_url'].rstrip('/')}/{filename}"
        if ctx.dry_run:
            ctx.log(f"would download {url}")
            continue
        try:
            archive = download(url, ctx.cache / "kiwix-tools" / filename, log=ctx.log)
            count = extract_kiwix_serve(archive, ctx.out / "bin" / target)
        except (FetchError, tarfile.TarError, zipfile.BadZipFile, StepError) as err:
            failures.append(f"{target}: {err}")
            continue
        ctx.log(f"installed kiwix-serve for {target} ({count} file{'s' if count != 1 else ''})")
    if failures:
        raise StepError("the library reader is missing for some computers "
                        "(check the file names in sources.toml):\n  " + "\n  ".join(failures))


def extract_kiwix_serve(archive: Path, dest: Path) -> int:
    """Pull kiwix-serve (and any DLLs shipped beside it) out of a release archive."""

    def wanted(name: str) -> bool:
        base = name.rsplit("/", 1)[-1].lower()
        return base in ("kiwix-serve", "kiwix-serve.exe") or base.endswith(".dll")

    dest.mkdir(parents=True, exist_ok=True)
    count = 0
    if archive.name.endswith(".zip"):
        with zipfile.ZipFile(archive) as bundle:
            for info in bundle.infolist():
                if info.is_dir() or not wanted(info.filename):
                    continue
                target = dest / info.filename.rsplit("/", 1)[-1]
                with bundle.open(info) as source, target.open("wb") as out:
                    shutil.copyfileobj(source, out)
                count += 1
    else:
        with tarfile.open(archive) as bundle:
            for member in bundle:
                if not member.isfile() or not wanted(member.name):
                    continue
                source = bundle.extractfile(member)
                if source is None:
                    continue
                target = dest / member.name.rsplit("/", 1)[-1]
                with source, target.open("wb") as out:
                    shutil.copyfileobj(source, out)
                target.chmod(0o755)
                count += 1
    if not any((dest / name).is_file() for name in ("kiwix-serve", "kiwix-serve.exe")):
        raise StepError(f"{archive.name} does not contain kiwix-serve")
    return count
