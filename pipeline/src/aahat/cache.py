"""Resumable local cache for whole remote files (DEM tiles), robust to flaky links.

Downloads in parallel byte-range chunks, each retried on its own, and records finished chunks
in a sidecar file so an interrupted download resumes where it stopped.
"""

from __future__ import annotations

import json
import logging
import os
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

log = logging.getLogger(__name__)

CHUNK = 2 * 1024 * 1024


def cache_dir() -> Path:
    d = Path(os.environ.get("AAHAT_CACHE_DIR", Path.home() / ".cache" / "aahat"))
    d.mkdir(parents=True, exist_ok=True)
    return d


def _content_length(url: str) -> int | None:
    """Size in bytes, or None if the object does not exist."""
    for attempt in range(6):
        try:
            req = urllib.request.Request(url, method="HEAD")
            with urllib.request.urlopen(req, timeout=30) as r:
                return int(r.headers["Content-Length"])
        except urllib.error.HTTPError as e:
            if e.code in (403, 404):
                return None
            err = e
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            err = e
        time.sleep(min(2**attempt, 20))
    raise RuntimeError(f"HEAD failed for {url}") from err


def _fetch_range(url: str, start: int, end: int, attempts: int = 10, deadline_s: float = 120) -> bytes:
    for attempt in range(attempts):
        try:
            req = urllib.request.Request(url, headers={"Range": f"bytes={start}-{end}"})
            t0, buf = time.monotonic(), bytearray()
            with urllib.request.urlopen(req, timeout=30) as r:
                while block := r.read(64 * 1024):
                    buf += block
                    if time.monotonic() - t0 > deadline_s:  # a trickling connection: drop it, retry fresh
                        raise TimeoutError(f"chunk slower than {deadline_s}s")
            if len(buf) == end - start + 1:
                return bytes(buf)
            log.warning("short read %d/%d bytes, retrying", len(buf), end - start + 1)
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            log.warning("range %d-%d failed (%s), retry %d", start, end, e, attempt + 1)
        time.sleep(min(2**attempt, 20))
    raise RuntimeError(f"could not fetch bytes {start}-{end} of {url}")


def cached_file(url: str, workers: int = 6) -> Path | None:
    """Local path for `url`, downloading it first if needed. None if the remote object is missing."""
    dest = cache_dir() / url.split("://", 1)[1]
    if dest.exists():
        return dest
    size = _content_length(url)
    if size is None:
        return None
    dest.parent.mkdir(parents=True, exist_ok=True)
    part, done_path = dest.with_suffix(dest.suffix + ".part"), dest.with_suffix(dest.suffix + ".done.json")
    done: set[int] = set(json.loads(done_path.read_text())) if done_path.exists() and part.exists() else set()
    if not part.exists():
        with open(part, "wb") as f:
            f.truncate(size)
    starts = [s for s in range(0, size, CHUNK) if s not in done]
    log.info("downloading %s (%.1f MB, %d chunks left)", dest.name, size / 1e6, len(starts))
    fd = os.open(part, os.O_WRONLY)
    try:

        def work(start: int) -> int:
            data = _fetch_range(url, start, min(start + CHUNK, size) - 1)
            os.pwrite(fd, data, start)
            return start

        with ThreadPoolExecutor(workers) as pool:
            for fut in as_completed([pool.submit(work, s) for s in starts]):
                done.add(fut.result())
                done_path.write_text(json.dumps(sorted(done)))
    finally:
        os.close(fd)
    part.rename(dest)
    done_path.unlink(missing_ok=True)
    return dest
