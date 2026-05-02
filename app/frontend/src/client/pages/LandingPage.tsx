import React from 'react';

const LandingPage: React.FC = () => {
  return (
    <div className="page">
      <div className="shell">
        <div className="badge">
          <span className="dot" /> Командный симулятор продуктовой разработки
        </div>
        <div className="hero">
          <div>
            <h1 className="title">Featureban — игра про баланс фич, долга и командной динамики</h1>
            <p className="subtitle">
              Вы управляете потоком задач, реагируете на события и ищете компромисс между скоростью,
              качеством и ценностью. Подходит для воркшопов, ретро и учебных сессий.
            </p>
            <div className="inline">
              <a className="btn" href="/start">Начать игру</a>
            </div>
          </div>
          <div className="card">
            <h3 style={{ marginTop: 0 }}>Что внутри</h3>
            <p className="help" style={{ margin: 0 }}>
              4 колонки прогресса, ограничение WIP, события дня и совместное принятие решений. Легко
              объяснить за 3 минуты, сложно выиграть без согласованности.
            </p>
          </div>
        </div>

        <div style={{ marginTop: 24 }} className="layout-grid">
          <div className="card compact">
            <h4 style={{ marginTop: 0 }}>Роли</h4>
            <p className="help">Ведущий задает контекст, команды выбирают, что делать дальше.</p>
          </div>
          <div className="card compact">
            <h4 style={{ marginTop: 0 }}>Цель</h4>
            <p className="help">Завершить проекты до конца цикла и не утонуть в техническом долге.</p>
          </div>
          <div className="card compact">
            <h4 style={{ marginTop: 0 }}>Формат</h4>
            <p className="help">Онлайн-лобби, быстрый старт, одна ссылка для игроков и для ведущего.</p>
          </div>
        </div>

        <div style={{ marginTop: 18 }} className="help">
          Если у вас уже есть код комнаты, нажмите «Начать игру» и введите его.
        </div>
      </div>
    </div>
  );
};

export default LandingPage;
