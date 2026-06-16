import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { getJson, postJson } from '../lib/http';
import { useGameSocket } from '../lib/useGameSocket';
import type { GameState, HistoryEntry, Member, Project, Task, Team } from '../types';


type DragState = {
  taskId: string;
  teamId: string;
  fromStage: string;
};

const STAGES = ['ready', 'in_progress', 'review', 'done'];
const TEAM_STAGE_LABELS: Record<string, string> = {
  ready: 'ready',
  in_progress: 'in_progress',
  review: 'review',
  done: 'done'
};
const PROJECT_STAGE_LABELS: Record<string, string> = {
  todo: 'В процессе',
  integration: 'Интеграция',
  done: 'Готово'
};

const STAGES_OPEN: Array<'ready' | 'in_progress' | 'review'> = ['ready', 'in_progress', 'review'];

const TEAM_CHIP_COLORS = ['#4a8ad4', '#3fab6f', '#d4a826', '#d45656', '#7d63c9', '#4ec4b3'];

function openTaskCountForProject(teams: Team[], projectId: string, teamId: string): number {
  const team = teams.find((x) => x.id === teamId);
  if (!team?.board) return 0;
  let n = 0;
  for (const stage of STAGES_OPEN) {
    for (const task of team.board[stage] || []) {
      if (task.project_id === projectId) n++;
    }
  }
  return n;
}

function teamColor(teams: Team[], teamId: string): string {
  const idx = teams.findIndex((t) => t.id === teamId);
  const i = idx >= 0 ? idx : 0;
  return TEAM_CHIP_COLORS[i % TEAM_CHIP_COLORS.length];
}

function ownerLabel(team: Team, ownerId?: string): string {
  if (!ownerId) return 'свободна';
  const member = (team.members || []).find((m) => m.id === ownerId);
  return member?.nickname || ownerId;
}

type CoinIconProps = {
  coin?: string;
};

const CoinIcon: React.FC<CoinIconProps> = ({ coin }) => {
  if (coin === 'heads') {
    return (
      <img
        src="/coins/heads.png"
        srcSet="/coins/heads@2x.png 2x"
        width={30}
        height={30}
        alt="Орёл"
        className="coin-icon coin-icon-heads"
        title="Орёл"
      />
    );
  }
  if (coin === 'tails') {
    return (
      <img
        src="/coins/tails.png"
        srcSet="/coins/tails@2x.png 2x"
        width={28}
        height={28}
        alt="Решка"
        className="coin-icon"
        title="Решка"
      />
    );
  }
  return null;
};

type TeamMembersProps = {
  members: Member[];
  currentTeamId?: string;
  allTeams?: { id: string; name: string }[];
  canSwitch?: boolean;
  onSwitch?: (memberId: string, newTeamId: string) => void;
};

const TeamMembers: React.FC<TeamMembersProps> = ({ members, currentTeamId, allTeams, canSwitch, onSwitch }) => {
  const players = members.filter((m) => m.role !== 'facilitator');
  if (!players.length) {
    return <span className="help">Участники: —</span>;
  }
  const otherTeams = (allTeams || []).filter((t) => t.id !== currentTeamId);
  return (
    <div className="team-members">
      <span className="team-members-label">Участники:</span>
      <div className="team-members-list">
        {players.map((member) => (
          <span key={member.id} className="team-member-item">
            <span className="team-member-name">{member.nickname}</span>
            <CoinIcon coin={member.current_coin} />
            {canSwitch && otherTeams.length > 0 && (
              <select
                className="switch-team-select"
                value=""
                title="Переместить в другую команду"
                onChange={(e) => { if (e.target.value) onSwitch?.(member.id, e.target.value); }}
              >
                <option value="" disabled>→</option>
                {otherTeams.map((t) => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            )}
          </span>
        ))}
      </div>
    </div>
  );
};

// Inline-editable WIP pill. When isEditable=true, clicking the limit opens an input.
// Auto-saves on blur or Enter; cancels on Escape.
const WipPill: React.FC<{
  count: number;
  limit: number | undefined;
  isEditable?: boolean;
  onSave?: (val: number) => void;
}> = ({ count, limit, isEditable, onSave }) => {
  const [editing, setEditing] = useState(false);
  const [val, setVal] = useState(String(limit ?? 1));

  const open = () => { setVal(String(limit ?? 1)); setEditing(true); };
  const commit = (raw: string) => {
    const n = parseInt(raw, 10);
    if (!isNaN(n) && n >= 1) onSave?.(n);
    setEditing(false);
  };

  const isOver = count > (limit ?? 1);

  if (editing) {
    return (
      <span className="wip-pill wip-editing" onClick={(e) => e.stopPropagation()}>
        ({count}/
        <input
          className="wip-inline-input"
          type="number"
          min={1}
          max={30}
          value={val}
          onChange={(e) => setVal(e.target.value)}
          onBlur={(e) => commit(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') { e.preventDefault(); commit(val); }
            if (e.key === 'Escape') setEditing(false);
          }}
          autoFocus
        />)
      </span>
    );
  }

  if (isEditable) {
    return (
      <span
        className={`wip-pill wip-editable ${isOver ? 'over' : ''}`}
        onClick={open}
        title="Нажмите, чтобы изменить WIP-лимит"
      >
        ({count}/{limit ?? '—'})
      </span>
    );
  }

  return (
    <span className={`wip-pill ${isOver ? 'over' : ''}`}>
      ({count}/{limit ?? '—'})
    </span>
  );
};

type ProjectKanbanProps = {
  teams: Team[];
  projects: Project[];
  wipLimits: Record<string, number>;
  isRetro?: boolean;
  canEditWip?: boolean;
  onSetProjectWip?: (col: string, limit: number) => void;
};

const ProjectKanban: React.FC<ProjectKanbanProps> = ({ teams, projects, wipLimits, isRetro, canEditWip, onSetProjectWip }) => {
  const colTodo = projects.filter(
    (p) => p.started && !p.completed && (!p.board_stage || p.board_stage === 'todo')
  );
  const colIntegration = projects.filter(
    (p) => p.started && !p.completed && p.board_stage === 'integration'
  );
  const colDone = projects.filter((p) => p.completed);
  const projectCounts = {
    todo: colTodo.length,
    integration: colIntegration.length,
    done: colDone.length
  };

  const renderCard = (p: Project, column: 'todo' | 'integration' | 'done') => {
    const days = p.days_in_integration ?? 0;
    const tickStr =
      column === 'integration' && days > 0
        ? `${'●'.repeat(Math.min(days, 14))}${days > 14 ? ` +${days - 14}` : ''}`
        : '';

    return (
      <div
        key={p.id}
        className={`project-hub-card ${p.completed ? 'is-done' : p.started ? 'is-active' : ''}`}
      >
        <div className="project-hub-head">
          <span className="project-hub-title">{p.name}</span>
        </div>
        <div className="team-stars-grid" aria-label="Задачи по командам">
          {teams.map((t) => {
            const n = openTaskCountForProject(teams, p.id, t.id);
            return (
              <div key={t.id} className="team-stars-row">
                <span
                  className="team-dot"
                  style={{ background: teamColor(teams, t.id) }}
                  aria-hidden
                />
                <span className="team-stars">{n > 0 ? '★'.repeat(n) : '—'}</span>
              </div>
            );
          })}
        </div>
        {column === 'todo' && (
          <div className="project-hub-foot help">
            Прогресс: {p.done_tasks}/{p.total_tasks}
          </div>
        )}
        {column === 'integration' && (
          <div className="project-hub-foot">
            <span className="help">Дней в интеграции: </span>
            <span className="integration-ticks" title={`${days} игровых дней`}>
              {days === 0 ? '0 (сегодня вошли)' : tickStr}
            </span>
            {p.penalty_issued ? <span className="penalty-tag">штраф</span> : null}
          </div>
        )}
        {column === 'done' && (
          <div className="project-hub-foot help">
            Завершён: {p.done_tasks}/{p.total_tasks}
            {p.done_day != null ? ` · день ${p.done_day}` : ''}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="project-kanban">
      <div className="project-kanban-cols">
        <div className="project-kanban-col">
          <h3>
            {PROJECT_STAGE_LABELS.todo}
            <WipPill
              count={projectCounts.todo}
              limit={wipLimits.todo}
              isEditable={isRetro && canEditWip}
              onSave={(v) => onSetProjectWip?.('todo', v)}
            />
          </h3>
          {colTodo.length === 0 && <div className="col-empty">Пусто</div>}
          {colTodo.map((p) => renderCard(p, 'todo'))}
        </div>
        <div className="project-kanban-col">
          <h3>
            {PROJECT_STAGE_LABELS.integration}
            <WipPill
              count={projectCounts.integration}
              limit={wipLimits.integration}
              isEditable={isRetro && canEditWip}
              onSave={(v) => onSetProjectWip?.('integration', v)}
            />
          </h3>
          {colIntegration.length === 0 && <div className="col-empty">Пусто</div>}
          {colIntegration.map((p) => renderCard(p, 'integration'))}
        </div>
        <div className="project-kanban-col">
          <h3>
            {PROJECT_STAGE_LABELS.done}
          </h3>
          {colDone.length === 0 && <div className="col-empty">Пока нет</div>}
          {colDone.map((p) => renderCard(p, 'done'))}
        </div>
      </div>
    </div>
  );
};

const GamePage: React.FC = () => {
  const { gamecode = '' } = useParams();
  const [params] = useSearchParams();
  const playerId = params.get('player_id') || '';

  const [state, setState] = useState<GameState | null>(null);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<{ text: string; type: 'ok' | 'err' | '' }>({
    text: '',
    type: ''
  });
  const [selectedTaskId, setSelectedTaskId] = useState('');
  const [flashTaskId, setFlashTaskId] = useState('');
  const dragRef = useRef<DragState>({ taskId: '', teamId: '', fromStage: '' });

  const { connected: wsConnected } = useGameSocket(gamecode, (nextState: GameState) => {
    setState(nextState);
  });

  const setStatusMessage = (text: string, type: 'ok' | 'err' | '' = '') => {
    setStatus({ text, type });
  };

  const playerRecord = useMemo(() => {
    if (!state) return null;
    for (const team of state.teams || []) {
      for (const member of team.members || []) {
        if (member.id === playerId) return member;
      }
    }
    if (state.facilitator_id === playerId) {
      return { id: playerId, role: 'facilitator', nickname: 'facilitator' } as Member;
    }
    return null;
  }, [state, playerId]);

  const isFacilitator = playerRecord?.role === 'facilitator';

  const loadState = useCallback(async (silent: boolean) => {
    if (!gamecode) return;
    try {
      const data = await getJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}`);
      setState(data);
    } catch (err) {
      if (!silent) setStatusMessage('Сервер недоступен.', 'err');
    }
  }, [gamecode]);

  useEffect(() => {
    loadState(true);
    if (wsConnected) {
      return () => undefined;
    }
    const interval = window.setInterval(() => loadState(true), 2500);
    return () => window.clearInterval(interval);
  }, [loadState, wsConnected]);

  const canAct = (teamId: string) => {
    if (!playerId || busy || state?.phase !== 'running') return false;
    if (isFacilitator) return false;
    if (playerRecord?.team_id !== teamId) return false;
    return !state?.turn_action_done?.[playerId];
  };

  const handleDrop = async (teamId: string, toStage: string) => {
    if (!gamecode) return;
    const drag = dragRef.current;
    if (!drag.taskId || drag.teamId !== teamId || !canAct(teamId)) return;

    // Dropping a card back into the same column shouldn't trigger a "stage transition".
    // On backend this causes `invalid stage transition for tails` for normal (non-blocked) tasks.
    // We intentionally NO-OP here for non-blocked tasks, but still allow the request for `blocked`
    // tasks since the backend uses `from == to` to perform "unblock" action.
    const task = (() => {
      if (!state?.teams) return null;
      for (const t of state.teams) {
        for (const stage of STAGES) {
          const tasks = t.board?.[stage] || [];
          const found = tasks.find((x) => x.id === drag.taskId);
          if (found) return found;
        }
      }
      return null;
    })();

    if (drag.fromStage === toStage) {

      if (!task?.blocked) {
        dragRef.current = { taskId: '', teamId: '', fromStage: '' };
        return;
      }
    }
    const team = state?.teams?.find((t) => t.id === teamId);
    const myCoin = playerRecord?.current_coin;
    if (myCoin === 'tails' && task && !task.blocked) {
      const validForward =
        (drag.fromStage === 'ready' && toStage === 'in_progress') ||
        (drag.fromStage === 'in_progress' && toStage === 'review') ||
        (drag.fromStage === 'review' && toStage === 'done');
      if (!validForward) {
        setStatusMessage('При решке это действие недоступно: попробуйте продвинуть, разблокировать или взять новую задачу.', 'err');
        dragRef.current = { taskId: '', teamId: '', fromStage: '' };
        return;
      }
    }

    try {
      setBusy(true);
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/drag`, {
        player_id: playerId,
        task_id: drag.taskId,
        to_stage: toStage
      });
      setState(data);
      setFlashTaskId(drag.taskId);
      if (isFacilitator) {
        setStatusMessage('Карточка перемещена.', 'ok');
      } else {
        setStatusMessage('', '');
      }
      window.setTimeout(() => setFlashTaskId(''), 550);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Ошибка перемещения.';
      setStatusMessage(message, 'err');
    } finally {
      setBusy(false);
      dragRef.current = { taskId: '', teamId: '', fromStage: '' };
    }
  };

  const toggleBlocked = async (task: Task) => {
    if (!gamecode) return;
    try {
      setBusy(true);
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/drag`, {
        player_id: playerId,
        task_id: task.id,
        to_stage: task.stage
      });
      setState(data);
      setFlashTaskId(task.id);
      if (isFacilitator) {
        setStatusMessage('Статус блокировки обновлен.', 'ok');
      } else {
        setStatusMessage('', '');
      }
      window.setTimeout(() => setFlashTaskId(''), 550);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Ошибка обновления.';
      setStatusMessage(message, 'err');
    } finally {
      setBusy(false);
    }
  };

  const makeMove = async () => {
    if (!gamecode) return;
    try {
      setBusy(true);
      const payload: { player_id: string; task_id?: string } = { player_id: playerId };
      if (selectedTaskId) payload.task_id = selectedTaskId;
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/move`, payload);
      setState(data);
      setSelectedTaskId('');
      setStatusMessage('Ход выполнен.', 'ok');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Ошибка хода.';
      setStatusMessage(message, 'err');
    } finally {
      setBusy(false);
    }
  };

  const switchTeam = async (targetPlayerId: string, newTeamId: string) => {
    try {
      setBusy(true);
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/switch_team`, {
        player_id: playerId,
        target_player_id: targetPlayerId,
        new_team_id: newTeamId,
      });
      setState(data);
      setStatusMessage('Игрок переведён в другую команду.', 'ok');
    } catch (err) {
      setStatusMessage(err instanceof Error ? err.message : 'Ошибка запроса', 'err');
    } finally {
      setBusy(false);
    }
  };

  const setTeamWip = async (teamId: string, stage: string, limit: number) => {
    try {
      setBusy(true);
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/set_wip`, {
        player_id: playerId,
        team_id: teamId,
        stage,
        wip_limit: limit,
      });
      setState(data);
      setStatusMessage('WIP-лимит обновлён.', 'ok');
    } catch (err) {
      setStatusMessage(err instanceof Error ? err.message : 'Ошибка запроса', 'err');
    } finally {
      setBusy(false);
    }
  };

  const setProjectWipInline = async (col: string, limit: number) => {
    try {
      setBusy(true);
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/set_project_wip`, {
        player_id: playerId,
        column: col,
        wip_limit: limit,
      });
      setState(data);
      setStatusMessage('WIP проектной доски обновлён.', 'ok');
    } catch (err) {
      setStatusMessage(err instanceof Error ? err.message : 'Ошибка запроса', 'err');
    } finally {
      setBusy(false);
    }
  };

  const visibleTeams = useMemo(() => {
    if (!state?.teams) return [];
    if (isFacilitator) return state.teams;
    return state.teams.filter((t) => t.id === playerRecord?.team_id);
  }, [state?.teams, isFacilitator, playerRecord]);
  const myTeam = useMemo(
    () => state?.teams?.find((t) => t.id === playerRecord?.team_id) || null,
    [state?.teams, playerRecord]
  );
  const myTeamStageLimits = myTeam?.wip_limits || {};

  const metaLine = useMemo(() => {
    if (!state) return '';
    return `День ${state.current_day} | Завершено проектов: ${state.projects_done}/${state.projects?.length || 0} | Циклов ретро: ${state.cycles_completed}`;
  }, [state, playerRecord]);
  const myActionsHint = useMemo(() => {
    if (!myTeam || state?.phase !== 'running') return '';
    if (playerId && state?.turn_action_done?.[playerId]) return 'Ваш ход на этот день уже завершен.';
    if (playerRecord?.current_coin === 'tails') {
      return 'Решка: взять себе задачу ИЛИ продвинуть свою задачу ИЛИ разблокировать свою задачу.';
    }
    if (playerRecord?.current_coin === 'heads') {
      return 'Орёл: заблокировать свою задачу И взять себе задачу.';
    }
    return 'Ожидаем ваш бросок монетки.';
  }, [myTeam, state?.phase, state?.turn_action_done, playerId, playerRecord]);

  const roleLabel = useMemo(() => {
    if (!state) return 'Роль: ---';
    const role = playerRecord?.role === 'facilitator' ? 'Ведущий' : playerRecord ? 'Игрок' : 'Наблюдатель';
    if (playerRecord?.role === 'facilitator') {
        return `Роль: ${role}`;
    }
    const meName = playerRecord?.nickname || playerId || 'без id';
    return `Роль: ${role} (${meName})`;
  }, [state, playerRecord, playerId]);

  const phaseLabel = `Фаза: ${state?.phase === 'setup' ? 'Подготовка' : state?.phase === 'running' ? 'Игра' : state?.phase === 'retro' ? 'Ретро' : state?.phase === 'finished' ? 'Конец' : '---'}`;
  const turnLabel = state?.phase === 'running'
    ? 'Все игроки ходят одновременно'
    : 'Ход: -';

  return (
    <div className="page" style={{ alignItems: 'stretch' }}>
      <div className="shell">
        <div className="card" style={{ marginBottom: 16 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', alignItems: 'center'}}>
            <div>
              <h1 style={{ margin: 0, fontFamily: 'IBM Plex Serif, serif' }}>
                Мультикомандный Featureban: <span>{gamecode || 'unknown'}</span>
              </h1>
              <div className="help">{metaLine}</div>
            </div>
            <div className="badge-row">
              <span className="badge-chip">{roleLabel}</span>
              <span className="badge-chip">{phaseLabel}</span>
              <span className="badge-chip turn">{turnLabel}</span>
            </div>
          </div>
        </div>

        <div className="layout-grid">
          <div className="card">
            <h2 style={{ marginTop: 0 }}>{isFacilitator ? "Командные доски" : "Доска задач"}</h2>
            {!isFacilitator && myActionsHint && (
              <div className="player-actions-hint">
                <strong>Доступные действия:</strong> {myActionsHint}
              </div>
            )}
            <div className={`status ${status.type}`}>{status.text}</div>
            <div style={{ height: 16 }} />
            {visibleTeams.map((team) => (
              <div key={team.id} className="card compact" style={{ marginBottom: 12 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
                  <span className="team-board-title">
                    <span
                      className="team-dot team-dot-board"
                      style={{ background: teamColor(state?.teams || [], team.id) }}
                      aria-hidden
                    />
                    <strong>{team.name}</strong>
                  </span>
                  <span className="help">Участников: {(team.members || []).length}</span>
                </div>
                <TeamMembers
                  members={team.members || []}
                  currentTeamId={team.id}
                  allTeams={state?.teams || []}
                  canSwitch={(state?.phase === 'retro' || state?.phase === 'setup') && isFacilitator}
                  onSwitch={switchTeam}
                />
                <div className="columns" style={{ marginTop: 10 }}>
                  {STAGES.map((stage) => (
                    <div
                      key={stage}
                      className="col"
                      onDragOver={(event) => {
                        if (!dragRef.current.taskId || !canAct(team.id)) return;
                        event.preventDefault();
                      }}
                      onDragEnter={(event) => {
                        if (!dragRef.current.taskId || !canAct(team.id)) return;
                        event.preventDefault();
                        event.currentTarget.classList.add('drop-target');
                      }}
                      onDragLeave={(event) => {
                        event.currentTarget.classList.remove('drop-target');
                      }}
                      onDrop={(event) => {
                        event.preventDefault();
                        event.currentTarget.classList.remove('drop-target');
                        handleDrop(team.id, stage);
                      }}
                    >
                      <h4>
                        {TEAM_STAGE_LABELS[stage] || stage}
                        {stage !== 'done' && (
                          <WipPill
                            count={team.counts?.[stage] || 0}
                            limit={team.wip_limits?.[stage]}
                            isEditable={
                              (state?.phase === 'retro' || state?.phase === 'setup') &&
                              (isFacilitator || team.id === playerRecord?.team_id)
                            }
                            onSave={(v) => setTeamWip(team.id, stage, v)}
                          />
                        )}
                      </h4>
                      {(team.board?.[stage] || []).map((task) => {
                        const isDone = stage === 'done';
                        const canDrag = canAct(team.id) && !isDone;
                        const canActOnTask = canAct(team.id) && !isDone;
                        const owner = ownerLabel(team, task.owner_id);
                        const taskClass = [
                          'task',
                          task.blocked ? 'blocked' : '',
                          isDone ? 'task-done' : '',
                          selectedTaskId === task.id ? 'sel' : '',
                          flashTaskId === task.id ? 'flash' : ''
                        ].join(' ');

                        return (
                          <div
                            key={task.id}
                            className={taskClass}
                            draggable={canDrag}
                            onClick={() => {
                              if (isDone) return;
                              setSelectedTaskId(selectedTaskId === task.id ? '' : task.id);
                            }}
                            onDragStart={(event) => {
                              if (isDone) return;
                              dragRef.current = { taskId: task.id, teamId: team.id, fromStage: stage };
                              event.currentTarget.classList.add('dragging');
                              event.dataTransfer.effectAllowed = 'move';
                              event.dataTransfer.setData('text/plain', task.id);
                            }}
                            onDragEnd={(event) => {
                              event.currentTarget.classList.remove('dragging');
                              dragRef.current = { taskId: '', teamId: '', fromStage: '' };
                              document.querySelectorAll('.col.drop-target').forEach((col) => col.classList.remove('drop-target'));
                            }}
                          >
                            <strong>{task.id}</strong> / {task.project_id}
                            {task.penalty ? ' [штраф]' : ''} {task.blocked ? '[blocked]' : ''}
                            <span className="task-owner">Ответственный: {owner}</span>
                            {!isDone && (task.owner_id || task.blocked) && (
                              <div className="task-actions">
                                <button
                                  type="button"
                                  className="task-btn"
                                  disabled={!canActOnTask}
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    toggleBlocked(task);
                                  }}
                                >
                                  {task.blocked ? 'Разблокировать' : 'Заблокировать'}
                                </button>
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  ))}
                </div>
              </div>
            ))}

            <div className="card compact" style={{ marginTop: 16 }}>
              <h3 style={{ marginTop: 0 }}>История</h3>
              <ul className="history">
                {(state?.history || []).length ? (
                  (state?.history || []).slice().reverse().map((entry, index) => (
                    <li key={`${entry.day}-${index}`}>
                      [День {entry.day}] {entry.message}
                    </li>
                  ))
                ) : (
                  <li>История пустая.</li>
                )}
              </ul>
            </div>
          </div>

          <div className="game-sidebar">
            {isFacilitator && state && (
              <div className="card">
                <h2 style={{ marginTop: 0 }}>Панель ведущего</h2>
                <FacilitatorPanel
                  busy={busy}
                  state={state}
                  playerId={playerId}
                  gamecode={gamecode}
                  setState={setState}
                  setStatus={setStatusMessage}
                  setBusy={setBusy}
                />
              </div>
            )}
            {!isFacilitator && state?.phase === 'retro' && (
              <div className="card">
                <p className="help" style={{ margin: 0 }}>
                  Ретро: вы можете провести рефлексию с коллегами и, если хотите, поменять WIP-лимиты, нажав на него в заголовке колонки.
                </p>
              </div>
            )}

            <div className="card">
              <h2 style={{ marginTop: 0 }}>Проектная доска</h2>
              <ProjectKanban
                teams={state?.teams || []}
                projects={state?.projects || []}
                wipLimits={state?.project_wip_limits || { todo: 3, integration: 2, done: 99 }}
                isRetro={state?.phase === 'retro' || state?.phase === 'setup'}
                canEditWip={isFacilitator}
                onSetProjectWip={setProjectWipInline}
              />
            </div>
            
            {isFacilitator && state?.started && state?.metrics && state?.phase === 'retro' && (
              <div className="metrics-panel">
                <h2>Игровые метрики</h2>
                <div className="metrics-grid">
                  <div className="metric-card">
                    <span className="metric-title">Lead Time (Ср. дней)</span>
                    <span className="metric-value">{state.metrics.lead_time.toFixed(1)}</span>
                  </div>
                  <div className="metric-card">
                    <span className="metric-title">Velocity (Задач в день)</span>
                    <span className="metric-value">{state.metrics.velocity.toFixed(1)}</span>
                  </div>
                  <div className="metric-card">
                    <span className="metric-title">WIP (В работе)</span>
                    <span className="metric-value">{state.metrics.wip}</span>
                  </div>
                  <div className="metric-card">
                    <span className="metric-title">Blocked (Ожидают)</span>
                    <span className="metric-value">{state.metrics.blocked}</span>
                  </div>
                  <div className="metric-card">
                    <span className="metric-title">Ретро проведены</span>
                    <span className="metric-value">{state.metrics.retro_days}</span>
                  </div>
                  <div className="metric-card">
                    <span className="metric-title">Дни блокировки (Суммарно)</span>
                    <span className="metric-value">{state.metrics.total_blocked_days}</span>
                  </div>
                  <div className="metric-card" title="Количество штрафов за задержку интеграции и другие нарушения">
                    <span className="metric-title">Штрафы (Суммарно)</span>
                    <span className="metric-value">{state.metrics.total_penalties}</span>
                  </div>
                </div>
                <div className="metrics-cfd">
                  <h3>CFD (Текущий статус)</h3>
                  <div className="cfd-row">
                    <span className="cfd-label">Ready:</span> <span className="cfd-val">{state.metrics.cfd?.ready || 0}</span>
                  </div>
                  <div className="cfd-row">
                    <span className="cfd-label">In Progress:</span> <span className="cfd-val">{state.metrics.cfd?.in_progress || 0}</span>
                  </div>
                  <div className="cfd-row">
                    <span className="cfd-label">Review:</span> <span className="cfd-val">{state.metrics.cfd?.review || 0}</span>
                  </div>
                  <div className="cfd-row">
                    <span className="cfd-label">Done:</span> <span className="cfd-val">{state.metrics.cfd?.done || 0}</span>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

type FacilitatorProps = {
  busy: boolean;
  state: GameState;
  playerId: string;
  gamecode: string;
  setState: React.Dispatch<React.SetStateAction<GameState | null>>;
  setStatus: (text: string, type?: 'ok' | 'err' | '') => void;
  setBusy: React.Dispatch<React.SetStateAction<boolean>>;
};

const FacilitatorPanel: React.FC<FacilitatorProps> = ({
  busy,
  state,
  playerId,
  gamecode,
  setState,
  setStatus,
  setBusy
}) => {
  const [projectId, setProjectId] = useState(state.projects?.[0]?.id || '');
  const currentPlayer = useMemo(() => {
    for (const team of state.teams || []) {
      const found = (team.members || []).find((member) => member.id === playerId);
      if (found) return found;
    }
    if (state.facilitator_id === playerId) {
      return { id: playerId, role: 'facilitator' };
    }
    return null;
  }, [state, playerId]);
  const isFacilitatorPanel = currentPlayer?.role === 'facilitator';
  const allPlayersDone = useMemo(() => {
    const players = (state.teams || []).flatMap((team) =>
      (team.members || []).filter((m) => m.role !== 'facilitator')
    );
    if (!players.length) return false;
    return players.every((member) => !!state.turn_action_done?.[member.id]);
  }, [state.teams, state.turn_action_done]);
  const pendingTeams = useMemo(
    () =>
      (state.teams || [])
        .filter((team) => {
          const players = (team.members || []).filter((m) => m.role !== 'facilitator');
          return players.some((m) => !state.turn_action_done?.[m.id]);
        })
        .map((team) => team.name),
    [state.teams, state.turn_action_done]
  );
  const canStartNewProject = useMemo(
    () => (state.teams || []).some((team) => (team.counts?.ready ?? 0) === 0),
    [state.teams]
  );

  const nextDayIsRetro = !!state.next_day_is_retro;
  const joinLink = useMemo(() => {
    const path = `/joining/${encodeURIComponent(gamecode)}`;
    if (typeof window !== 'undefined') {
      return `${window.location.origin}${path}`;
    }
    return path;
  }, [gamecode]);

  const launchableProjects = useMemo(
    () => (state.projects || []).filter((p) => !p.started && !p.completed),
    [state.projects]
  );

  useEffect(() => {
    if (!launchableProjects.length) {
      setProjectId('');
      return;
    }
    const ids = new Set(launchableProjects.map((p) => p.id));
    if (!projectId || !ids.has(projectId)) {
      setProjectId(launchableProjects[0].id);
    }
  }, [launchableProjects, projectId]);

  const runAction = async (url: string, payload: Record<string, unknown>, okText: string) => {
    try {
      setBusy(true);
      const data = await postJson<GameState>(url, payload);
      setState(data);
      setStatus(okText, 'ok');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Ошибка запроса';
      setStatus(message, 'err');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack">
      {isFacilitatorPanel && !state.started && (
        <>
          <p className="help" style={{ margin: '0 0 4px' }}>
            При желании вы можете изменить начальные WIP-лимиты, нажав на них в заголовке любой колонки.
          </p>
          <div className="facilitator-join-link">
            <span className="help">Ссылка для участников:</span>
            <a className="facilitator-join-url" href={joinLink}>
              {joinLink}
            </a>
          </div>
          <div className="actions">
            <button
              className="btn"
              type="button"
              disabled={busy}
              onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/start`, { player_id: playerId }, 'Игра запущена.')}
            >
              Старт игры
            </button>
          </div>
        </>
      )}

      {isFacilitatorPanel && (
      <div className="actions project-launch-actions">
        {launchableProjects.length === 0 ? (
          <p className="help" style={{ margin: 0 }}>
            Все проекты уже запущены.
          </p>
        ) : (
          <select
            className="field project-launch-select"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
            aria-label="Проект для запуска"
          >
            {launchableProjects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        )}
        <button
          className="btn"
          type="button"
          disabled={busy || !canStartNewProject || !projectId}
          title={
            canStartNewProject
              ? undefined
              : 'Сначала освободите «Сделать» хотя бы у одной команды (все карточки должны быть взяты в работу или отсутствовать).'
          }
          onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/start_project`, {
            player_id: playerId,
            project_id: projectId
          }, 'Проект запущен.')}
        >
          Запустить проект
        </button>
        {!canStartNewProject && (
          <span className="help">Нет команды с пустой «Сделать» — новый проект сейчас добавить нельзя.</span>
        )}
      </div>
      )}

      {isFacilitatorPanel && state.started && state.phase === 'running' && (
        <div className="actions">
          <div className="facilitator-day-controls">
            {!nextDayIsRetro && (
              <>
                <button
                  className="btn facilitator-day-btn"
                  type="button"
                  disabled={busy}
                  onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/next_day`, { player_id: playerId }, 'Новый день начался. Монетки брошены.')}
                >
                  Начать новый день
                </button>
                <span className="help facilitator-next-day-hint">
                  {allPlayersDone ? 'Все игроки завершили действия.' : `Ожидаем команды: ${pendingTeams.join(', ') || '—'}`}
                </span>
              </>
            )}
            <button
              className="btn retro facilitator-day-btn"
              type="button"
              disabled={busy}
              onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/start_retro`, { player_id: playerId }, 'Ретро началось.')}
            >
              Начать ретро
            </button>
            {nextDayIsRetro && (
              <span className="help facilitator-next-day-hint">
                {allPlayersDone ? 'По расписанию наступило ретро.' : `По расписанию ретро. Ожидаем команды: ${pendingTeams.join(', ') || '—'}`}
              </span>
            )}
          </div>
        </div>
      )}

      {state.started && state.phase === 'retro' && (
        <div className="retro-facilitator-block">
          <p className="help" style={{ margin: '0 0 8px' }}>
            Нажмите на лимит в заголовке любой колонки, чтобы изменить его прямо на доске.
          </p>
          <div className="actions">
            <button
              className="btn"
              type="button"
              disabled={busy}
              onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/continue`, { player_id: playerId }, 'Ретро завершено.')}
            >
              Завершить ретро и продолжить
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export default GamePage;
