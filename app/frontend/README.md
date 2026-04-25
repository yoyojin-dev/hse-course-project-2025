# Frontend proxy

Простой TypeScript-сервер для обслуживания статики и проксирования запросов в backend.

Поведением:
- Статика: `app/frontend/static` доступна на корне `/`.
- Все запросы к `/api/*` проксируются на `http://backend:8080`.
- WebSocket-префикс `/ws/*` проксируется на `http://backend:8080` с поддержкой upgrade.

Команды:

```bash
cd app/frontend
npm install
npm run dev    # запуск в режиме разработки (ts-node-dev)
npm run build  # сборка в dist/
npm run start  # запуск собранного приложения
```

Docker: слушать на 0.0.0.0 и использовать `target: http://backend:8080` в прокси (видим `backend` как имя сервиса в compose).

написано нейросестью