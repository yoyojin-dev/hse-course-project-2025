from __future__ import annotations

import json
import os
import shutil
import subprocess
import time
from pathlib import Path
from typing import Any, Dict, Iterable, Optional, Tuple
from urllib import error, request as urllib_request

import pytest

ROOT = Path(__file__).resolve().parents[1]
BACKEND_DIR = ROOT / "app" / "backend"


def _kill_process_tree(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        return
    if os.name == "nt":
        subprocess.run(
            ["taskkill", "/PID", str(proc.pid), "/T", "/F"],
            capture_output=True,
            text=True,
            check=False,
        )
    else:
        proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)


@pytest.fixture(scope="session")
def backend_binary(tmp_path_factory: pytest.TempPathFactory) -> Path:
    go = shutil.which("go")
    if not go:
        pytest.fail("Go toolchain is not available in PATH")

    bin_dir = tmp_path_factory.mktemp("backend-bin")
    binary_name = "backend-test.exe" if os.name == "nt" else "backend-test"
    binary_path = bin_dir / binary_name

    build = subprocess.run(
        [go, "build", "-o", str(binary_path), "."],
        cwd=BACKEND_DIR,
        capture_output=True,
        text=True,
        check=False,
    )
    if build.returncode != 0:
        pytest.fail(
            "backend build failed\n"
            f"stdout:\n{build.stdout}\n"
            f"stderr:\n{build.stderr}"
        )

    return binary_path


@pytest.fixture(scope="session")
def backend_server(backend_binary: Path, tmp_path_factory: pytest.TempPathFactory) -> Iterable[str]:
    log_dir = tmp_path_factory.mktemp("backend-logs")
    log_path = log_dir / "server.log"
    with log_path.open("w", encoding="utf-8") as log_file:
        proc = subprocess.Popen(
            [str(backend_binary)],
            cwd=BACKEND_DIR,
            stdout=log_file,
            stderr=subprocess.STDOUT,
            text=True,
        )
        try:
            deadline = time.time() + 30
            last_error: Optional[Exception] = None
            while time.time() < deadline:
                try:
                    with urllib_request.urlopen("http://127.0.0.1:8080/api/hello", timeout=1) as resp:
                        if resp.status == 200 and resp.read().decode("utf-8").strip() == "hello from backend":
                            break
                except Exception as exc:  # pragma: no cover - only used on startup failures
                    last_error = exc
                    time.sleep(0.2)
            else:
                _kill_process_tree(proc)
                log_text = log_path.read_text(encoding="utf-8", errors="replace")
                pytest.fail(
                    "backend server did not start in time\n"
                    f"last error: {last_error!r}\n"
                    f"log:\n{log_text}"
                )

            yield "http://127.0.0.1:8080"
        finally:
            _kill_process_tree(proc)


class APIClient:
    def __init__(self, base_url: str) -> None:
        self.base_url = base_url.rstrip("/")

    def request(
        self,
        method: str,
        path: str,
        payload: Optional[Dict[str, Any]] = None,
        *,
        headers: Optional[Dict[str, str]] = None,
        allow_error: bool = False,
    ) -> Tuple[int, str, Dict[str, str]]:
        req_headers = {"Accept": "application/json"}
        if headers:
            req_headers.update(headers)

        data = None
        if payload is not None:
            req_headers.setdefault("Content-Type", "application/json")
            data = json.dumps(payload).encode("utf-8")

        req = urllib_request.Request(
            f"{self.base_url}{path}",
            data=data,
            headers=req_headers,
            method=method,
        )
        try:
            with urllib_request.urlopen(req, timeout=10) as resp:
                body = resp.read().decode("utf-8")
                return resp.status, body, dict(resp.headers)
        except error.HTTPError as exc:
            body = exc.read().decode("utf-8")
            if allow_error:
                return exc.code, body, dict(exc.headers)
            raise AssertionError(f"{method} {path} -> {exc.code}: {body}") from exc

    def json(
        self,
        method: str,
        path: str,
        payload: Optional[Dict[str, Any]] = None,
        *,
        headers: Optional[Dict[str, str]] = None,
        allow_error: bool = False,
    ) -> Tuple[int, Any, Dict[str, str]]:
        status, body, resp_headers = self.request(
            method,
            path,
            payload,
            headers=headers,
            allow_error=allow_error,
        )
        parsed = json.loads(body) if body else None
        return status, parsed, resp_headers

    def text(self, method: str, path: str) -> Tuple[int, str, Dict[str, str]]:
        return self.request(method, path, headers={"Accept": "text/plain"})


@pytest.fixture()
def api_client(backend_server: str) -> APIClient:
    return APIClient(backend_server)


