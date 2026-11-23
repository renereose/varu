"""Downloads: resumable, checksum-verified, and careful with disk space.

Standard library only, so the build runs anywhere Python does.
"""

from __future__ import annotations

import base64
import hashlib
import shutil
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Callable

from . import __version__

USER_AGENT = f"varu-builder/{__version__} (+https://github.com/renereose/varu)"
CHUNK = 1024 * 1024
Log = Callable[[str], None]


class FetchError(RuntimeError):
    pass


def _request(url: str, *, headers: dict | None = None, data: bytes | None = None, method: str | None = None):
    merged = {"User-Agent": USER_AGENT, **(headers or {})}
    return urllib.request.Request(url, headers=merged, data=data, method=method)


def fetch_bytes(url: str, *, data: bytes | None = None, timeout: int = 120, tries: int = 3) -> bytes:
    """Small API responses, with a few patient retries."""
    last: Exception | None = None
    for attempt in range(tries):
        try:
            with urllib.request.urlopen(_request(url, data=data), timeout=timeout) as response:
                return response.read()
        except urllib.error.HTTPError as err:
            # 4xx will not get better by asking again, except rate limiting.
            if err.code not in (429, 500, 502, 503, 504):
                raise FetchError(f"{url}: HTTP {err.code}") from err
            last = err
        except (urllib.error.URLError, TimeoutError, ConnectionError) as err:
            last = err
        time.sleep(5 * (attempt + 1))
    raise FetchError(f"{url}: {last}")


def exists(url: str, *, timeout: int = 30) -> bool:
    try:
        with urllib.request.urlopen(_request(url, method="HEAD"), timeout=timeout) as response:
            return 200 <= response.status < 300
    except (urllib.error.URLError, TimeoutError, ConnectionError):
        return False


def final_url(url: str, *, timeout: int = 60) -> str:
    """Where url redirects to, such as a "latest" link to today's dated file."""
    try:
        with urllib.request.urlopen(_request(url, method="HEAD"), timeout=timeout) as response:
            return response.url
    except (urllib.error.URLError, TimeoutError, ConnectionError) as err:
        raise FetchError(f"{url}: {err}") from err


def sri_matches(path: Path, integrity: str) -> bool:
    """Check a file against a Subresource-Integrity string such as "sha512-<base64>"."""
    algorithm, _, expected = integrity.partition("-")
    if algorithm not in ("sha256", "sha384", "sha512") or not expected:
        raise FetchError(f"unsupported integrity value: {integrity!r}")
    digest = hashlib.new(algorithm)
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(CHUNK), b""):
            digest.update(block)
    return base64.b64encode(digest.digest()).decode() == expected


def _total_size(response, already: int) -> int | None:
    content_range = response.headers.get("Content-Range", "")
    if "/" in content_range and not content_range.endswith("/*"):
        return int(content_range.rsplit("/", 1)[1])
    length = response.headers.get("Content-Length")
    return int(length) + already if length else None


def download(url: str, dest: Path, *, integrity: str | None = None, log: Log = print) -> Path:
    """Download url to dest.

    * A finished file is kept (and re-verified when an integrity value is given).
    * An interrupted download continues from dest + ".part".
    * Refuses to start when the target drive is too small for the file.
    """
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists():
        if integrity is None or sri_matches(dest, integrity):
            log(f"have {dest.name}")
            return dest
        log(f"{dest.name} fails its checksum, downloading again")
        dest.unlink()

    part = dest.with_name(dest.name + ".part")
    already = part.stat().st_size if part.exists() else 0
    headers = {"Range": f"bytes={already}-"} if already else {}

    try:
        response = urllib.request.urlopen(_request(url, headers=headers), timeout=120)
    except urllib.error.HTTPError as err:
        if err.code == 416 and already:
            # The server says there is nothing after what we have: complete.
            response = None
        else:
            raise FetchError(f"{url}: HTTP {err.code}") from err
    except (urllib.error.URLError, TimeoutError, ConnectionError) as err:
        raise FetchError(f"{url}: {err}") from err

    if response is not None:
        with response:
            if already and response.status != 206:
                already = 0  # server ignored the Range header: start over
            total = _total_size(response, already)
            if total is not None:
                free = shutil.disk_usage(dest.parent).free
                if total - already > free:
                    raise FetchError(
                        f"{dest.name} needs {_human(total - already)} but only {_human(free)} is free on the target"
                    )
            log(f"downloading {dest.name}" + (f" ({_human(total)})" if total else "")
                + (f", resuming at {_human(already)}" if already else ""))
            done = already
            next_report = done + max(50 * CHUNK, (total or 0) // 10)
            try:
                with part.open("ab" if already else "wb") as out:
                    for block in iter(lambda: response.read(CHUNK), b""):
                        out.write(block)
                        done += len(block)
                        if done >= next_report:
                            log(f"  {dest.name}: {_human(done)}" + (f" of {_human(total)}" if total else ""))
                            next_report = done + max(50 * CHUNK, (total or 0) // 10)
            except (urllib.error.URLError, TimeoutError, ConnectionError, OSError) as err:
                raise FetchError(f"{url}: interrupted at {_human(done)} ({err}); run again to resume") from err
            if total is not None and done != total:
                raise FetchError(f"{url}: got {_human(done)} of {_human(total)}; run again to resume")

    if integrity is not None and not sri_matches(part, integrity):
        part.unlink()
        raise FetchError(f"{dest.name}: checksum mismatch, refusing to use it")
    part.replace(dest)
    return dest


def _human(size: int | float) -> str:
    for unit in ("B", "kB", "MB", "GB"):
        if abs(size) < 1000:
            return f"{size:.0f} {unit}" if unit == "B" else f"{size:.1f} {unit}"
        size /= 1000
    return f"{size:.1f} TB"
