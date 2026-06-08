from __future__ import annotations

import asyncio
import json
from typing import Any, Dict

import pytest
import websockets

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


class WebSocketClient:
    def __init__(self, uri: str):
        self.uri = uri
        self.websocket = None

    async def connect(self):
        self.websocket = await websockets.connect(self.uri)
        return self

    async def send(self, message: Dict[str, Any]):
        await self.websocket.send(json.dumps(message))

    async def receive(self, timeout: float = 5.0):
        response = await asyncio.wait_for(self.websocket.recv(), timeout=timeout)
        return json.loads(response)

    async def close(self):
        if self.websocket:
            await self.websocket.close()


@pytest.fixture
def ws_base_url(backend_server):
    return backend_server.replace("http://", "ws://")


async def test_lobby_ws_join_redirect_existing_code(ws_base_url, api_client):
    status, data, _ = api_client.json("POST", "/api/create", {"team_names": ["Team A"], "max_days": 10})
    assert status == 201
    game_code = data["game_code"]

    client = WebSocketClient(f"{ws_base_url}/ws/lobby")
    await client.connect()
    try:
        await client.send({"type": "join_redirect", "game_code": game_code})
        response = await client.receive()
        assert response["type"] == "join_redirect"
        assert response["ok"] is True
        assert response["redirect_to"] == f"/joining/{game_code}"
    finally:
        await client.close()


async def test_lobby_ws_join_redirect_unknown_code(ws_base_url):
    client = WebSocketClient(f"{ws_base_url}/ws/lobby")
    await client.connect()
    try:
        await client.send({"type": "join_redirect", "game_code": "ZZZZZZ"})
        response = await client.receive()
        assert response["type"] == "join_redirect"
        assert response["ok"] is False
        assert response["error"] == "Игра не найдена"
    finally:
        await client.close()


async def test_lobby_ws_join_redirect_empty_code(ws_base_url):
    client = WebSocketClient(f"{ws_base_url}/ws/lobby")
    await client.connect()
    try:
        await client.send({"type": "join_redirect", "game_code": ""})
        response = await client.receive()
        assert response["type"] == "join_redirect"
        assert response["ok"] is False
        assert response["error"] == "Укажите код игры"
    finally:
        await client.close()


async def test_lobby_ws_ping_pong(ws_base_url):
    client = WebSocketClient(f"{ws_base_url}/ws/lobby")
    await client.connect()
    try:
        await client.send({"type": "ping"})
        response = await client.receive()
        assert response["type"] == "pong"
        assert response["ok"] is True
    finally:
        await client.close()


async def test_lobby_ws_unknown_type(ws_base_url):
    client = WebSocketClient(f"{ws_base_url}/ws/lobby")
    await client.connect()
    try:
        await client.send({"type": "unknown_type"})
        response = await client.receive()
        assert response["type"] == "unknown_type"
        assert response["ok"] is False
        assert response["error"] == "неизвестный тип сообщения"
    finally:
        await client.close()


async def test_game_ws_missing_code_rejected(ws_base_url):
    with pytest.raises(websockets.exceptions.InvalidStatus) as excinfo:
        await WebSocketClient(f"{ws_base_url}/ws/game").connect()
    assert excinfo.value.response.status_code == 400


async def test_game_ws_unknown_code_rejected(ws_base_url):
    with pytest.raises(websockets.exceptions.InvalidStatus) as excinfo:
        await WebSocketClient(f"{ws_base_url}/ws/game?code=ZZZZZZ").connect()
    assert excinfo.value.response.status_code == 404


async def test_game_ws_sends_state_on_connect(ws_base_url, api_client):
    status, data, _ = api_client.json("POST", "/api/create", {"team_names": ["Team A"], "max_days": 10})
    assert status == 201
    game_code = data["game_code"]

    client = WebSocketClient(f"{ws_base_url}/ws/game?code={game_code}")
    await client.connect()
    try:
        response = await client.receive()
        assert response["type"] == "state"
        assert "state" in response
        assert response["state"]["code"] == game_code
    finally:
        await client.close()


async def test_game_ws_ping_pong(ws_base_url, api_client):
    status, data, _ = api_client.json("POST", "/api/create", {"team_names": ["Team A"], "max_days": 10})
    assert status == 201
    game_code = data["game_code"]

    client = WebSocketClient(f"{ws_base_url}/ws/game?code={game_code}")
    await client.connect()
    try:
        _ = await client.receive()
        await client.send({"type": "ping"})
        response = await client.receive()
        assert response["type"] == "pong"
    finally:
        await client.close()
