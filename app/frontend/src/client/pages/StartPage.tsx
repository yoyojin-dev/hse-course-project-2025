import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';

const readCookie = (name: string) => {
  const parts = document.cookie.split(';').map((part) => part.trim());
  const match = parts.find((part) => part.startsWith(`${name}=`));
  if (!match) return null;
  return decodeURIComponent(match.split('=')[1] || '');
};

const deleteCookie = (name: string) => {
  document.cookie = `${name}=; Path=/; Max-Age=0; SameSite=Lax`;
};

const StartPage: React.FC = () => {
  const [highlight, setHighlight] = useState(false);
  const [gameCode, setGameCode] = useState('');
  const [error, setError] = useState('');
  const [wsReady, setWsReady] = useState(false);
  const [ws, setWs] = useState<WebSocket | null>(null);
  const [showCreateSetup, setShowCreateSetup] = useState(false);
  const navigate = useNavigate();

  const [teamNameInput, setTeamNameInput] = useState('');
  const [teams, setTeams] = useState<string[]>(['Синяя', 'Зеленая', 'Желтая']);

  const handleAddTeam = () => {
    const name = teamNameInput.trim();
    if (name && !teams.includes(name)) {
      setTeams([...teams, name]);
      setTeamNameInput('');
    }
  };

  const handleRemoveTeam = (index: number) => {
    setTeams(teams.filter((_, i) => i !== index));
  };

  const handleCreateSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (teams.length < 1) {
      setError('Нужна хотя бы одна команда.');
      return;
    }
    
    try {
      const res = await fetch('/api/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ team_names: teams })
      });
      if (res.ok) {
        const body = await res.json();
        navigate(`/created/${body.game_code}?facilitator_id=${body.facilitator_id}`);
      } else {
        setError('Ошибка создания игры');
      }
    } catch {
      setError('Сетевая ошибка');
    }
  };

  useEffect(() => {
    const flash = readCookie('flash');
    if (flash === 'notfound') {
      setHighlight(true);
      deleteCookie('flash');
      const timer = window.setTimeout(() => setHighlight(false), 5000);
      return () => window.clearTimeout(timer);
    }
    return undefined;
  }, []);

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/lobby`);

    socket.addEventListener('open', () => setWsReady(true));
    socket.addEventListener('close', () => setWsReady(false));
    socket.addEventListener('error', () => setWsReady(false));
    socket.addEventListener('message', (event) => {
      try {
        const payload = JSON.parse(event.data) as {
          type?: string;
          ok?: boolean;
          redirect_to?: string;
          error?: string;
        };
        if (payload.type === 'join_redirect') {
          if (payload.ok && payload.redirect_to) {
            navigate(payload.redirect_to);
            return;
          }
          setError(payload.error || 'Игра не найдена. Проверьте код.');
          setHighlight(true);
          window.setTimeout(() => setHighlight(false), 5000);
        }
      } catch {
        setError('Некорректный ответ сервера.');
      }
    });

    setWs(socket);
    return () => socket.close();
  }, []);

  const inputClass = useMemo(() => (highlight ? 'field input-invalid' : 'field'), [highlight]);

  const handleJoinSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    const code = gameCode.trim();
    if (!code) {
      event.preventDefault();
      setError('Введите код игры.');
      setHighlight(true);
      return;
    }

    if (wsReady && ws) {
      event.preventDefault();
      setError('');
      ws.send(JSON.stringify({ type: 'join_redirect', game_code: code }));
    }
  };

  return (
    <div className="page">
      <div className="shell" style={{ maxWidth: 400 }}>
        <div className="card">
          <div className="stack">
            <div>
              <h2 style={{ margin: 0, fontFamily: 'IBM Plex Serif, serif' }}>Присоединиться к игре</h2>
              <div className="help">Введите код комнаты или создайте новую игру.</div>
            </div>
            {!showCreateSetup && (
              <form className="stack" action="/api/" method="post" onSubmit={handleJoinSubmit}>
                <input
                  className={inputClass}
                  type="text"
                  name="game_code"
                  placeholder="Введите код..."
                  value={gameCode}
                  onChange={(event) => setGameCode(event.target.value)}
                />
                <button className="btn" type="submit">Присоединиться</button>
              </form>
            )}
            {!showCreateSetup ? (
              <button
                className="btn secondary"
                type="button"
                onClick={() => {
                  setError('');
                  setShowCreateSetup(true);
                }}
              >
                Создать игру
              </button>
            ) : (
              <form className="stack" onSubmit={handleCreateSubmit}>
                <div style={{ display: 'flex', gap: 8, flexDirection: 'column' }}>
                  <div>Команды:</div>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    {teams.map((t, idx) => (
                      <span key={idx} style={{ background: '#7952d5ff', padding: '4px 8px', borderRadius: 4 }}>
                        {t} <button type="button" onClick={() => handleRemoveTeam(idx)} style={{ border: 'none', background: 'none', cursor: 'pointer' }}>x</button>
                      </span>
                    ))}
                  </div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <input
                      className="field"
                      type="text"
                      placeholder="Название команды"
                      value={teamNameInput}
                      onChange={(e) => setTeamNameInput(e.target.value)}
                    />
                    <button type="button" className="btn" onClick={handleAddTeam}>+</button>
                  </div>
                </div>
                <div className="actions">
                  <button className="btn secondary" type="submit">Подтвердить создание</button>
                  <button className="btn" type="button" onClick={() => setShowCreateSetup(false)}>Назад</button>
                </div>
              </form>
            )}
            {error && <div className="error">{error}</div>}
          </div>
        </div>
      </div>
    </div>
  );
};

export default StartPage;
