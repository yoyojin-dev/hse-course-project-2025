from __future__ import annotations

import pytest


def create_game(api_client, *, team_names=None, max_days=10):
    payload = {
        "team_names": team_names or ["Blue", "Green"],
        "max_days": max_days,
    }
    status, data, _ = api_client.json("POST", "/api/create", payload)
    assert status == 201
    return data["game_code"], data["facilitator_id"]


def join_player(api_client, code: str, nickname: str, team_id: str):
    status, data, _ = api_client.json(
        "POST",
        "/api/join",
        {"game_code": code, "nickname": nickname, "team_id": team_id},
    )
    return status, data


def start_running_game(api_client, *, team_names=None, max_days=10):
    code, facilitator_id = create_game(
        api_client,
        team_names=team_names or ["Solo Team"],
        max_days=max_days,
    )
    for idx, _ in enumerate(team_names or ["Solo Team"], start=1):
        status, _ = join_player(api_client, code, f"player-{idx}", f"team-{idx}")
        assert status == 201

    status, _, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200
    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    assert status == 200
    return code, facilitator_id, state


def advance_to_retro(api_client, code: str, facilitator_id: str):
    state = None
    for _ in range(5):
        status, _, _ = api_client.json(
            "POST",
            f"/api/game/{code}/skip_turn",
            {"player_id": facilitator_id},
        )
        assert status == 200
        status, state, _ = api_client.json(
            "POST",
            f"/api/game/{code}/next_day",
            {"player_id": facilitator_id},
        )
        assert status == 200
    assert state is not None
    assert state["phase"] == "retro"
    return state


def create_game_with_coin_outcome(api_client, desired_coin: str, *, max_attempts=30):
    for _ in range(max_attempts):
        code, facilitator_id, state = start_running_game(api_client)
        player = state["teams"][0]["members"][0]
        if player.get("current_coin") == desired_coin:
            return code, facilitator_id, state

    pytest.fail(f"Could not get coin={desired_coin} after {max_attempts} attempts")


def get_player_with_coin(api_client, code: str, desired_coin: str):
    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200

    for team in state["teams"]:
        for player in team["members"]:
            if player.get("current_coin") == desired_coin:
                return player["id"]

    return None
