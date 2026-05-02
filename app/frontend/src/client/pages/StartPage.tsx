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
  const navigate = useNavigate();

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
            <form action="/api/create" method="post">
              <button className="btn secondary" type="submit">Создать игру</button>
            </form>
            {error && <div className="error">{error}</div>}
            <div className="help">После создания вы получите ссылку для игроков и ведущего.</div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default StartPage;
