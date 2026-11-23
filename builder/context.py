"""What every build step needs to know."""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

from .config import Region


class StepError(RuntimeError):
    """A step could not finish. The build carries on with the other steps."""


@dataclass
class Context:
    repo: Path                 # repository checkout
    out: Path                  # drive root being assembled
    cache: Path                # downloads that are reusable between builds
    region: Region
    sources: dict
    dry_run: bool = False
    planet_build: str | None = None
    launcher_dir: Path | None = None
    step: str = ""

    def log(self, message: str) -> None:
        print(f"[{self.step}] {message}" if self.step else message, flush=True)

    # Small facts that one step learns and the manifest step needs later
    # (which planet build the map came from, the titles of the ZIM files).
    @property
    def _info_path(self) -> Path:
        return self.out / "build-info.json"

    def read_info(self) -> dict:
        try:
            return json.loads(self._info_path.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            return {}

    def update_info(self, **changes) -> None:
        info = self.read_info()
        info.update(changes)
        self.out.mkdir(parents=True, exist_ok=True)
        self._info_path.write_text(json.dumps(info, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
