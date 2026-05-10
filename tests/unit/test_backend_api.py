from __future__ import annotations

import re

import pytest


pytestmark = pytest.mark.unit


def create_game(api_client, *, team_names=None, max_days=10):
    payload = {
        "team_names": team_names or ["Blue", "Green"],
        "max_days": max_days,
    }
    status, data, _ = api_client.json("POST", "/api/create", payload)
    assert status == 201
    assert re.fullmatch(r"\d{6}", data["game_code"])
    assert re.fullmatch(r"p\d+", data["facilitator_id"])
    return data["game_code"], data["facilitator_id"]


def join_player(api_client, code: str, nickname: str, team_id: str):
    status, data, _ = api_client.json(
        "POST",
        "/api/join",
        {"game_code": code, "nickname": nickname, "team_id": team_id},
    )
    return status, data


def test_hello_endpoint_returns_plain_text(api_client):
    status, body, _ = api_client.text("GET", "/api/hello")
    assert status == 200
    assert body.strip() == "hello from backend"


def test_create_game_returns_expected_structure(api_client):
    code, facilitator_id = create_game(api_client, team_names=["QA", "Dev"], max_days=12)

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200
    assert state["code"] == code
    assert state["phase"] == "setup"
    assert state["started"] is False
    assert state["finished"] is False
    assert state["max_days"] == 12
    assert state["facilitator_id"] == facilitator_id
    assert len(state["teams"]) == 2
    assert len(state["projects"]) == 15


def test_join_same_nickname_is_idempotent(api_client):
    code, _ = create_game(api_client)

    status, first_join = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    assert first_join["game_code"] == code
    first_player_id = first_join["player_id"]

    status, second_join = join_player(api_client, code, "alice", "team-2")
    assert status == 200
    assert second_join["player_id"] == first_player_id
    assert second_join["redirect_to"] == f"/game/{code}?player_id={first_player_id}"

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200
    assert len(state["teams"][0]["members"]) == 1
    assert len(state["teams"][1]["members"]) == 0


def test_join_unknown_team_is_rejected(api_client):
    code, _ = create_game(api_client)

    status, data, _ = api_client.json(
        "POST",
        "/api/join",
        {"game_code": code, "nickname": "bob", "team_id": "team-99"},
        allow_error=True,
    )
    assert status == 400
    assert data["error"] == "unknown team"


def test_start_project_populates_ready_tasks(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    join_player(api_client, code, "alice", "team-1")
    join_player(api_client, code, "bob", "team-2")

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200
    project = next(project for project in state["projects"] if project["id"] == "PR-01")
    assert project["started"] is True
    assert project["started_day"] == 1
    assert project["total_tasks"] == sum(project["tasks_by_team"].values())

    team1 = next(team for team in state["teams"] if team["id"] == "team-1")
    team2 = next(team for team in state["teams"] if team["id"] == "team-2")
    assert len(team1["board"]["ready"]) == project["tasks_by_team"]["team-1"]
    assert len(team2["board"]["ready"]) == project["tasks_by_team"]["team-2"]


def test_start_game_rejects_if_not_all_teams_have_players(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    join_player(api_client, code, "alice", "team-1")
    api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "each team must have at least 1 player"


def start_running_game(api_client, *, team_names=None, max_days=10):
    code, facilitator_id = create_game(
        api_client,
        team_names=team_names or ["Solo Team"],
        max_days=max_days,
    )
    for idx, _ in enumerate(team_names or ["Solo Team"], start=1):
        join_player(api_client, code, f"player-{idx}", f"team-{idx}")

    api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    assert status == 200
    return code, facilitator_id, state


def advance_to_retro(api_client, code: str, facilitator_id: str):
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
    assert state["phase"] == "retro"
    return state


def test_start_game_requires_started_project(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    join_player(api_client, code, "alice", "team-1")

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "start at least one project first"


def test_start_game_forbidden_for_non_facilitator(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]
    assert player_id != facilitator_id

    api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": player_id},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "only facilitator can do this"


def test_next_day_requires_all_teams_done(api_client):
    code, facilitator_id, _ = start_running_game(api_client)

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/next_day",
        {"player_id": facilitator_id},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "cannot start next day: not all teams finished actions"


def test_skip_turn_then_next_day_advances_day_counter(api_client):
    code, facilitator_id, _ = start_running_game(api_client)

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
    assert state["phase"] == "running"


def test_set_wip_rejected_outside_retro(api_client):
    code, facilitator_id, state = start_running_game(api_client)
    team_id = state["teams"][0]["id"]

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team_id, "wip_limit": 3},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "WIP can be changed only during retro phase"


def test_set_wip_validates_range(api_client):
    code, facilitator_id, _ = start_running_game(api_client)
    retro_state = advance_to_retro(api_client, code, facilitator_id)
    team_id = retro_state["teams"][0]["id"]

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team_id, "wip_limit": 0},
        allow_error=True,
    )
    assert status == 400
    assert data["error"] == "wip_limit must be in range 1..10"


def test_set_wip_updates_team_during_retro(api_client):
    code, facilitator_id, _ = start_running_game(api_client)
    retro_state = advance_to_retro(api_client, code, facilitator_id)
    team_id = retro_state["teams"][0]["id"]
    old_wip = retro_state["teams"][0]["wip_limit"]

    status, updated_state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team_id, "wip_limit": old_wip + 1},
    )
    assert status == 200
    updated_team = next(team for team in updated_state["teams"] if team["id"] == team_id)
    assert updated_team["wip_limit"] == old_wip + 1


def test_drag_rejects_task_from_another_team(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    _, team1_join = join_player(api_client, code, "alice", "team-1")
    player1_id = team1_join["player_id"]
    join_player(api_client, code, "bob", "team-2")

    api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200
    team2 = next(team for team in state["teams"] if team["id"] == "team-2")
    foreign_task_id = team2["board"]["ready"][0]["id"]

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/drag",
        {"player_id": player1_id, "task_id": foreign_task_id, "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "task belongs to another team"


