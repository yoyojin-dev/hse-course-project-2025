from __future__ import annotations

import json
import re

import pytest

from tests.utils.flows import (
    advance_to_retro,
    create_game,
    join_player,
    start_running_game,
)


pytestmark = pytest.mark.unit


def test_hello_endpoint_returns_plain_text(api_client):
    status, body, _ = api_client.text("GET", "/api/hello")
    assert status == 200
    assert body.strip() == "hello from backend"


def test_create_game_returns_expected_structure(api_client):
    status, data, _ = api_client.json(
        "POST", "/api/create", {"team_names": ["QA", "Dev"], "max_days": 12}
    )
    assert status == 201
    assert re.fullmatch(r"[A-Z0-9]{6}", data["game_code"])
    assert re.fullmatch(
        r"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}",
        data["facilitator_id"],
    )
    code = data["game_code"]
    facilitator_id = data["facilitator_id"]

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


def test_create_game_defaults_when_team_names_empty(api_client):
    status, data, _ = api_client.json(
        "POST", "/api/create", payload={"team_names": [], "max_days": 20}
    )
    assert status == 201
    status, state, _ = api_client.json("GET", f"/api/game/{data['game_code']}")
    assert status == 200
    assert len(state["teams"]) == 4


def test_create_game_clamps_short_max_days(api_client):
    status, data, _ = api_client.json(
        "POST", "/api/create", payload={"team_names": ["Team A"], "max_days": 2}
    )
    assert status == 201
    status, state, _ = api_client.json("GET", f"/api/game/{data['game_code']}")
    assert status == 200
    assert state["max_days"] == 15


def test_create_game_get_returns_405(api_client):
    status, _, _ = api_client.request("GET", "/api/create", allow_error=True)
    assert status == 405


@pytest.mark.parametrize(
    "content_type",
    ["form", "json"],
    ids=["form-encoded", "json"],
)
def test_join_redirect_empty_code_returns_400(api_client, content_type):
    if content_type == "form":
        status, body, _ = api_client.form(
            "POST", "/api/", "game_code=", allow_error=True
        )
        error_msg = json.loads(body)["error"]
    else:
        status, data, _ = api_client.json(
            "POST", "/api/", payload={"game_code": ""}, allow_error=True
        )
        error_msg = data["error"]

    assert status == 400
    assert error_msg == "укажите код игры"


def test_join_redirect_get_method_not_allowed(api_client):
    status, _, _ = api_client.request("GET", "/api/", allow_error=True)
    assert status == 405


def test_join_redirect_existing_game_returns_303_to_joining(api_client):
    code, _ = create_game(api_client)
    status, _, headers = api_client.request_no_redirect(
        "POST", "/api/", form_payload=f"game_code={code}",
    )
    assert status == 303
    assert headers["Location"] == f"/joining/{code}"


def test_join_redirect_nonexistent_game_returns_303_to_root(api_client):
    status, _, headers = api_client.request_no_redirect(
        "POST", "/api/", form_payload="game_code=ZZZZZZ",
    )
    assert status == 303
    assert headers["Location"] == "/"
    assert "flash=notfound" in headers["Set-Cookie"]


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
    assert data["error"] == "неизвестная команда"


def test_join_unknown_game_returns_404(api_client):
    status, data, _ = api_client.json(
        "POST", "/api/join",
        payload={"game_code": "ZZZZZZ", "nickname": "alice", "team_id": "team-1"},
        allow_error=True,
    )
    assert status == 404
    assert data["error"] == "игра не найдена"


def test_join_after_started_returns_409(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    status, data, _ = api_client.json(
        "POST", "/api/join",
        payload={"game_code": code, "nickname": "bob", "team_id": "team-1"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "игра уже началась"


def test_join_fills_team_to_max_then_rejects_sixth(api_client):
    code, _ = create_game(api_client, team_names=["Team A"])
    for i in range(5):
        status, data = join_player(api_client, code, f"player{i}", "team-1")
        assert status == 201
    status, data, _ = api_client.json(
        "POST", "/api/join",
        {"game_code": code, "nickname": "player5", "team_id": "team-1"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "команда заполнена (не более 5 участников)"


def test_join_missing_fields_returns_400(api_client):
    code, _ = create_game(api_client)
    status, _, _ = api_client.json(
        "POST", "/api/join",
        payload={"game_code": code, "nickname": "", "team_id": "team-1"},
        allow_error=True,
    )
    assert status == 400
    status, _, _ = api_client.json(
        "POST", "/api/join",
        payload={"game_code": code, "nickname": "alice", "team_id": ""},
        allow_error=True,
    )
    assert status == 400


def test_start_project_populates_ready_tasks(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    status, _ = join_player(api_client, code, "bob", "team-2")
    assert status == 201

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


def test_start_project_forbidden_for_non_facilitator(api_client):
    code, facilitator_id = create_game(api_client)
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]
    assert player_id != facilitator_id

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        payload={"player_id": player_id, "project_id": "PR-01"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "это может сделать только ведущий"


def test_start_project_unknown_id_returns_404(api_client):
    code, facilitator_id = create_game(api_client)
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        payload={"player_id": facilitator_id, "project_id": "PR-99"},
        allow_error=True,
    )
    assert status == 404
    assert data["error"] == "проект не найден"


def test_start_project_already_started_returns_409(api_client):
    code, facilitator_id = create_game(api_client)
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    status, _, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        payload={"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200
    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        payload={"player_id": facilitator_id, "project_id": "PR-01"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "проект уже запущен"


def test_start_project_blocked_when_all_ready_columns_full(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    status, _ = join_player(api_client, code, "bob", "team-2")
    assert status == 201

    status, _, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        payload={"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        payload={"player_id": facilitator_id, "project_id": "PR-02"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "новый проект можно добавить только если колонка «Сделать» пуста хотя бы у одной команды"


def test_start_game_rejects_if_not_all_teams_have_players(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
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
    assert data["error"] == "В каждой команде должен быть хотя бы один игрок"


def test_start_game_requires_started_project(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "сначала запустите хотя бы один проект"


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
    assert data["error"] == "это может сделать только ведущий"


def test_start_game_unknown_code_returns_404(api_client):
    status, data, _ = api_client.json(
        "POST", "/api/game/ZZZZZZ/start",
        payload={"player_id": "some-id"},
        allow_error=True,
    )
    assert status == 404
    assert data["error"] == "игра не найдена"


def test_start_game_twice_returns_409(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    status, _, _ = api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    assert status == 200
    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "игра уже началась"


def test_start_game_missing_player_id_returns_400(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/start",
        payload={},
        allow_error=True,
    )
    assert status == 400
    assert data["error"] == "не указан идентификатор игрока"


def test_drag_rejects_task_from_another_team(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A", "Team B"])
    _, team1_join = join_player(api_client, code, "alice", "team-1")
    player1_id = team1_join["player_id"]
    status, _ = join_player(api_client, code, "bob", "team-2")
    assert status == 201

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200
    team2 = next(t for t in state["teams"] if t["id"] == "team-2")
    foreign_task_id = team2["board"]["ready"][0]["id"]

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        {"player_id": player1_id, "task_id": foreign_task_id, "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "задача принадлежит другой команде"


def test_drag_rejects_unknown_task(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": player_id, "task_id": "unknown-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 404
    assert data["error"] == "задача не найдена"


def test_drag_by_facilitator_is_forbidden(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": facilitator_id, "task_id": "some-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "ведущий не может двигать карточки"


def test_drag_before_game_started(api_client):
    code, facilitator_id = create_game(api_client)
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": player_id, "task_id": "some-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "игра ещё не начата"


def test_drag_in_retro_phase(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    advance_to_retro(api_client, code, facilitator_id)

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": player_id, "task_id": "some-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "ходы разрешены только в игровой фазе"


def test_drag_unknown_player_id(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"])
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": "unknown-player", "task_id": "some-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "игрок не в этой игре"


def test_drag_after_finished_returns_409(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"], max_days=5)
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    for _ in range(5):
        api_client.json(
            "POST", f"/api/game/{code}/skip_turn",
            {"player_id": facilitator_id},
        )
        api_client.json(
            "POST", f"/api/game/{code}/next_day",
            {"player_id": facilitator_id},
        )

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert state["finished"] is True

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": player_id, "task_id": "some-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "игра уже завершена"


def test_next_day_requires_all_teams_done(api_client):
    code, facilitator_id, _ = start_running_game(api_client)

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/next_day",
        {"player_id": facilitator_id},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "нельзя начать новый день: не все игроки завершили действия"


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
    assert data["error"] == "лимит WIP можно менять только на ретро"


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
    assert data["error"] == "лимит WIP должен быть от 1 до 10"


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
