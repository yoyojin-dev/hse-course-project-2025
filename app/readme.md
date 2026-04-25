# Для разрабов

## Запуск

```shell
make up
# или
docker compose up --build
```

## Стоп

```shell
make down
# или
docker compose down
```

## Логи

```shell
make logs
# или
docker compose logs -f
```

## API

`GET /api/hello`

Основные маршруты игрового цикла:

- `POST /api/create` - создать игру
- `POST /api/join` - войти в игру по коду и никнейму
- `GET /api/game/{code}` - состояние игры
- `POST /api/game/{code}/start` - старт игры
- `POST /api/game/{code}/move` - сделать ход

## Как играть

1. Откройте главную страницу и создайте игру.
2. Передайте ссылку вида `/joining/{code}` другим игрокам.
3. Каждый игрок вводит никнейм и попадает на страницу игры.
4. После подключения минимум двух игроков нажмите `Старт игры`.
5. Игроки делают ходы по очереди кнопкой `Сделать ход (монетка)`.
6. Побеждает тот, кто первым наберет 5 очков.

## Деплой на VPS

Автодеплой через GitHub Actions описан в `./deploy/README.md`.

Ручной деплой на сервере:

```shell
cd /opt/featureban/app
bash deploy/deploy.sh
```
