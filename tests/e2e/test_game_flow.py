from __future__ import annotations

import pytest


pytestmark = pytest.mark.e2e


def create_game(api_client, *, team_names=None, max_days=10):
    status, data, _ = api_client.json(
        "POST",
        "/api/create",
        {"team_names": team_names or ["QA Team"], "max_days": max_days},
    )
    assert status == 201
    return data["game_code"], data["facilitator_id"]


def join_player(api_client, code: str, nickname: str, team_id: str):
    status, data, _ = api_client.json(
        "POST",
        "/api/join",
        {"game_code": code, "nickname": nickname, "team_id": team_id},
    )
    assert status == 201
    return data["player_id"]


def test_full_game_cycle_to_retro_and_continue(api_client):
    code, facilitator_id = create_game(api_client, team_names=["QA Team"], max_days=10)
    player_id = join_player(api_client, code, "alice", "team-1")
    assert player_id

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200
    assert state["projects"][0]["started"] is True

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["phase"] == "running"
    assert state["started"] is True

    for _ in range(5):
        status, state, _ = api_client.json(
            "POST",
            f"/api/game/{code}/skip_turn",
            {"player_id": facilitator_id},
        )
        assert status == 200
        assert state["phase"] == "running"

        status, state, _ = api_client.json(
            "POST",
            f"/api/game/{code}/next_day",
            {"player_id": facilitator_id},
        )
        assert status == 200

    assert state["phase"] == "retro"
    assert state["current_day"] == 6

    team = state["teams"][0]
    previous_wip = team["wip_limit"]
    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team["id"], "wip_limit": previous_wip + 1},
    )
    assert status == 200
    updated_team = next(item for item in state["teams"] if item["id"] == team["id"])
    assert updated_team["wip_limit"] == previous_wip + 1

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/continue",
        {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["phase"] == "running"
    assert state["current_day"] == 6
    assert state["teams"][0]["wip_limit"] == previous_wip + 1


def test_permissions_and_phase_guards_during_running_game(api_client):
    code, facilitator_id = create_game(api_client, team_names=["QA", "Dev"], max_days=10)
    player_a = join_player(api_client, code, "alice", "team-1")
    join_player(api_client, code, "bob", "team-2")

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
    assert state["phase"] == "running"

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/next_day",
        {"player_id": player_a},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "only facilitator can do this"

    team1 = next(team for team in state["teams"] if team["id"] == "team-1")
    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team1["id"], "wip_limit": team1["wip_limit"] + 1},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "WIP can be changed only during retro phase"

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200
    team2 = next(team for team in state["teams"] if team["id"] == "team-2")
    foreign_task = team2["board"]["ready"][0]["id"]

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/drag",
        {"player_id": player_a, "task_id": foreign_task, "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "task belongs to another team"

    for _ in range(2):
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
    assert state["current_day"] == 2


