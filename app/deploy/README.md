# Простой деплой на VPS

Упрощенный вариант автодеплоя:

- Деплой запускается только при изменениях в `app/**`.
- GitHub Actions берет данные подключения из `app/deploy/.env.deploy`.
- На сервер копируется `app/deploy/deploy.sh` и запускается по SSH.

## Как это работает

1. Push в `main` с изменениями в `app/**`.
2. Workflow читает `app/deploy/.env.deploy`.
3. По SSH выполняет:
   - принудительное выравнивание на `origin/main` в `/opt/featureban`
   - `docker compose up --build -d` в `/opt/featureban/app`

## Ручной запуск на сервере

```bash
cd /opt/featureban/app
bash deploy/deploy.sh
```

## Важно

Этот вариант небезопасен, так как пароль хранится в репозитории в открытом виде.

Скрипт деплоя принудительно делает `checkout -f main` и `reset --hard origin/main`.
Это значит, что любые локальные изменения в `/opt/featureban` на VPS будут потеряны при деплое.
