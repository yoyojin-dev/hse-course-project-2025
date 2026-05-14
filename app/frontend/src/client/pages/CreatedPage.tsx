import React from 'react';
import { useParams, useSearchParams } from 'react-router-dom';

const CreatedPage: React.FC = () => {
  const { gamecode = '' } = useParams();
  const [params] = useSearchParams();
  const facilitatorId = params.get('facilitator_id') || '';

  const facilitatorLink = facilitatorId
    ? `/game/${encodeURIComponent(gamecode)}?player_id=${encodeURIComponent(facilitatorId)}`
    : '';

  return (
    <div className="page">
      <div className="shell" style={{ maxWidth: 520 }}>
        <div className="card">
          <div className="stack">
            <div>
              <h2 style={{ margin: 0, fontFamily: 'IBM Plex Serif, serif' }}>Игра успешно создана!</h2>
              <div className="help" style={{ marginTop: 6 }}>
                Код игры: <strong style={{ color: 'var(--accent)' }}>{gamecode}</strong>
              </div>
            </div>
            <div className="help">
              Игроки могут подключиться по ссылке: <a href={`/joining/${gamecode}`}>/joining/{gamecode}</a>
            </div>
            <button
              className="btn"
              type="button"
              onClick={() => {
                if (facilitatorLink) {
                  window.location.href = facilitatorLink;
                } else {
                  window.location.href = '/start';
                }
              }}
            >
              Открыть панель ведущего
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default CreatedPage;
