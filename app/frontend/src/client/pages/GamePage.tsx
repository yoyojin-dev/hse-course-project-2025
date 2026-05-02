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
      if (!silent) setStatusMessage('Состояние обновлено.', 'ok');
    } catch (err) {
      if (!silent) setStatusMessage('Сервер недоступен.', 'err');
    }
  }, [gamecode]);

  useEffect(() => {
    loadState(false);
    if (wsConnected) {
      return () => undefined;
    }
    const interval = window.setInterval(() => loadState(true), 2500);
    return () => window.clearInterval(interval);
  }, [loadState, wsConnected]);

  const canActTeam = (teamId: string) => {
    return !!playerId && !busy && state?.phase === 'running' && state.current_turn_team_id === teamId;
  };

  const handleDrop = async (teamId: string, toStage: string) => {
    if (!gamecode) return;
    const drag = dragRef.current;
    if (!drag.taskId || drag.teamId !== teamId || !canActTeam(teamId)) return;

    try {
      setBusy(true);
      const data = await postJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}/drag`, {
        player_id: playerId,
        task_id: drag.taskId,
        to_stage: toStage
      });
      setState(data);
      setFlashTaskId(drag.taskId);
      setStatusMessage('Карточка перемещена.', 'ok');
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
      setStatusMessage('Статус блокировки обновлен.', 'ok');
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

  const moveEnabled = useMemo(() => {
    if (!state || !playerRecord) return false;
    const myTeam = playerRecord.team_id || '';
    return (
      playerRecord.role === 'player' &&
      state.phase === 'running' &&
      state.current_turn_team_id === myTeam &&
      !busy &&
      !state.current_coin
    );
  }, [state, playerRecord, busy]);

  const metaLine = useMemo(() => {
    if (!state) return '';
    const coinMeta = state.current_coin ? `, монетка: ${state.current_coin}` : '';
    return `День ${state.current_day} из ${state.max_days}, завершено проектов: ${state.projects_done}/${state.projects?.length || 0}, циклов ретро: ${state.cycles_completed}${coinMeta}`;
  }, [state]);

  const roleLabel = useMemo(() => {
    if (!state) return 'Роль: ...';
    const role = playerRecord?.role === 'facilitator' ? 'Ведущий' : playerRecord ? 'Игрок' : 'Наблюдатель';
    const meName = playerRecord?.nickname || playerId || 'без id';
    return `Роль: ${role} (${meName})`;
  }, [state, playerRecord, playerId]);

  const phaseLabel = `Фаза: ${state?.phase || '...'}`;
  const turnLabel = state?.phase === 'running' && state.current_turn_team_name
    ? `Ход команды: ${state.current_turn_team_name}`
    : 'Ход: -';

  const facilitator = isFacilitator && state;

  return (
    <div className="page" style={{ alignItems: 'stretch' }}>
      <div className="shell">
        <div className="card" style={{ marginBottom: 16 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
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
            <h2 style={{ marginTop: 0 }}>Командные доски</h2>
            {(state?.teams || []).map((team) => (
              <div key={team.id} className="card compact" style={{ marginBottom: 12 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
                  <strong>{team.name}</strong>
                  <span className="help">WIP={team.wip_limit}, участников: {(team.members || []).length}</span>
                </div>
                <div className="help" style={{ marginTop: 6 }}>
                  Участники: {(team.members || []).map((m) => m.nickname).join(', ')}
                </div>
                <div className="columns" style={{ marginTop: 10 }}>
                  {STAGES.map((stage) => (
                    <div
                      key={stage}
                      className="col"
                      onDragOver={(event) => {
                        if (!dragRef.current.taskId || !canActTeam(team.id)) return;
                        event.preventDefault();
                      }}
                      onDragEnter={(event) => {
                        if (!dragRef.current.taskId || !canActTeam(team.id)) return;
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
                        {stage} ({team.counts?.[stage] || 0})
                      </h4>
                      {(team.board?.[stage] || []).map((task) => {
                        const canDrag = canActTeam(team.id);
                        const canAct = canActTeam(team.id);
                        const owner = task.owner_id || 'свободна';
                        const taskClass = [
                          'task',
                          task.blocked ? 'blocked' : '',
                          selectedTaskId === task.id ? 'sel' : '',
                          flashTaskId === task.id ? 'flash' : ''
                        ].join(' ');

                        return (
                          <div
                            key={task.id}
                            className={taskClass}
                            draggable={canDrag}
                            onClick={() => setSelectedTaskId(selectedTaskId === task.id ? '' : task.id)}
                            onDragStart={(event) => {
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
                            <strong>{task.id}</strong> / {task.project_id} {task.blocked ? '[blocked]' : ''}
                            <span className="task-owner">Ответственный: {owner}</span>
                            <div className="task-actions">
                              <button
                                type="button"
                                className="task-btn"
                                disabled={!canAct}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  toggleBlocked(task);
                                }}
                              >
                                {task.blocked ? 'Разблокировать' : 'Заблокировать'}
                              </button>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  ))}
                </div>
              </div>
            ))}

            <div className="actions">
              <button className="btn" type="button" disabled={!moveEnabled} onClick={makeMove}>
                Сделать ход (монетка)
              </button>
              <button className="btn secondary" type="button" onClick={() => loadState(false)}>
                Обновить
              </button>
            </div>
            <div className={`status ${status.type}`}>{status.text}</div>
          </div>

          <div style={{ display: 'grid', gap: 16 }}>
            <div className="card">
              <h2 style={{ marginTop: 0 }}>Панель ведущего</h2>
              {!facilitator && <div className="help">Доступно только ведущему.</div>}
              {facilitator && (
                <FacilitatorPanel
                  busy={busy}
                  state={state}
                  playerId={playerId}
                  gamecode={gamecode}
                  setState={setState}
                  setStatus={setStatusMessage}
                  setBusy={setBusy}
                />
              )}
            </div>

            <div className="card">
              <h2 style={{ marginTop: 0 }}>Проектная доска</h2>
              <div className="projects">
                {(state?.projects || []).map((project) => (
                  <div
                    key={project.id}
                    className={`project ${project.completed ? 'done' : project.started ? 'started' : ''}`}
                  >
                    <div className="project-title">{project.name} ({project.id})</div>
                    <div className="project-meta">
                      Статус: {project.completed ? 'done' : project.started ? 'in progress' : 'backlog'}, задачи: {project.done_tasks}/{project.total_tasks}
                    </div>
                  </div>
                ))}
              </div>
            </div>

            <div className="card">
              <h2 style={{ marginTop: 0 }}>История</h2>
              <ul className="history">
                {(state?.history || []).length ? (
                  (state?.history || []).slice().reverse().map((entry, index) => (
                    <li key={`${entry.day}-${index}`}>
                      [day {entry.day}] {entry.category}: {entry.message}
                    </li>
                  ))
                ) : (
                  <li>История пустая.</li>
                )}
              </ul>
            </div>
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
  const [teamId, setTeamId] = useState(state.teams?.[0]?.id || '');
  const [wip, setWip] = useState('2');

  useEffect(() => {
    if (!projectId && state.projects?.[0]?.id) setProjectId(state.projects[0].id);
  }, [state.projects, projectId]);

  useEffect(() => {
    if (!teamId && state.teams?.[0]?.id) setTeamId(state.teams[0].id);
  }, [state.teams, teamId]);

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
      <div className="actions">
        <button
          className="btn"
          type="button"
          disabled={busy || !!state.started}
          onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/start`, { player_id: playerId }, 'Игра запущена.')}
        >
          Старт игры
        </button>
      </div>

      <div className="actions">
        <select className="field" value={projectId} onChange={(event) => setProjectId(event.target.value)}>
          {(state.projects || []).map((project) => (
            <option key={project.id} value={project.id}>
              {project.name} ({project.started ? 'started' : 'backlog'})
            </option>
          ))}
        </select>
        <button
          className="btn"
          type="button"
          disabled={busy}
          onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/start_project`, {
            player_id: playerId,
            project_id: projectId
          }, 'Проект запущен.')}
        >
          Запустить проект
        </button>
      </div>

      <div className="actions">
        <button
          className="btn"
          type="button"
          disabled={busy || state.phase !== 'running' || !state.current_turn_team_id}
          onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/skip_turn`, { player_id: playerId }, 'Ход пропущен ведущим.')}
        >
          Пропустить текущий ход
        </button>
      </div>

      <div className="actions">
        <select className="field" value={teamId} onChange={(event) => setTeamId(event.target.value)}>
          {(state.teams || []).map((team) => (
            <option key={team.id} value={team.id}>
              {team.name}
            </option>
          ))}
        </select>
        <input
          className="field"
          style={{ maxWidth: 120 }}
          type="number"
          min={1}
          max={10}
          value={wip}
          onChange={(event) => setWip(event.target.value)}
        />
        <button
          className="btn"
          type="button"
          disabled={busy || state.phase !== 'retro'}
          onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/set_wip`, {
            player_id: playerId,
            team_id: teamId,
            wip_limit: Number(wip || '2')
          }, 'WIP обновлен.')}
        >
          Изменить WIP
        </button>
      </div>

      <div className="actions">
        <button
          className="btn"
          type="button"
          disabled={busy || state.phase !== 'retro'}
          onClick={() => runAction(`/api/game/${encodeURIComponent(gamecode)}/continue`, { player_id: playerId }, 'Ретро завершено.')}
        >
          Завершить ретро и продолжить
        </button>
      </div>
    </div>
  );
};

export default GamePage;
