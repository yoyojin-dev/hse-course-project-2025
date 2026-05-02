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
            <div className="help">
              Ссылка для ведущего:{' '}
              {facilitatorLink ? (
                <a href={facilitatorLink}>открыть панель ведущего</a>
              ) : (
                <span>id ведущего не передан</span>
              )}
            </div>
            <button
              className="btn"
              type="button"
              onClick={() => {
                if (gamecode) {
                  window.location.href = `/joining/${encodeURIComponent(gamecode)}`;
                } else {
                  window.location.href = '/start';
                }
              }}
            >
              Перейти в лобби
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default CreatedPage;
