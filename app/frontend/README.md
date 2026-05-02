# Frontend proxy + React SPA

Фронтенд состоит из двух частей:
- React SPA (Vite) — рендер страниц и UI.
- TypeScript-сервер (Express) — раздача собранной статики и прокси в backend.

Поведение:
- Статика генерируется в `app/frontend/static` (`npm run build`).
- Все запросы к `/api/*` проксируются на `http://backend:8080`.
- WebSocket-префикс `/ws/*` проксируется на `http://backend:8080` с поддержкой upgrade.

Команды:

```bash
cd app/frontend
npm install
npm run dev         # Vite dev-сервер для React
npm run build       # сборка React в static/ + сборка server.ts в dist/
npm run start       # запуск собранного Express-сервера
```

Docker: используется `npm run build` на этапе сборки, затем `node dist/server.js`.