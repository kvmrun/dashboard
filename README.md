# kvmrun-dashboard

Веб-интерфейс для [kvmrun](https://github.com/0xef53/kvmrun) — визуализация и
управление тем же, что показывает CLI `vmm`, через gRPC API демона `kvmrund`.

## Архитектура

- **Без базы данных**: все данные берутся live из демона `kvmrund` (mutual TLS,
  `unix:@/run/kvmrund.sock` или TCP `:9393`).
- **gRPC-клиент**: [go-grpc](https://github.com/0xef53/go-grpc) (interceptors
  request-ID и логирования уже встроены) + готовые сгенерированные stubs из
  самого модуля `github.com/0xef53/kvmrun` (`api/services/*/v2`,
  `api/types/v2`) — локальная кодогенерация не требуется.
- **Бэкенд**: Go + [Gin](https://gin-gonic.com) — сервер-рендеринг страниц плюс
  небольшой JSON API для интерактивности.
- **Авторизация**: PAM через cgo ([`internal/auth`](internal/auth)) — логин
  принимается любой существующий системный пользователь; сессии —
  in-memory с cookie.
- **Фронтенд**: встроенные статические ресурсы (CSS/JS) через `go:embed`;
  результат — один самодостаточный бинарник.

## Структура

```
cmd/dashboard/            # точка входа: флаги, подключение к демону, HTTP-сервер
internal/
  auth/                   # PAM-авторизация (cgo/libpam) + in-memory сессии
  config/                 # конфигурация (listen addr, адрес демона, TLS-сертификаты)
  daemon/                 # gRPC-клиент к kvmrund (на базе go-grpc) + сервисные клиенты
  model/                  # DTO, общие для хендлеров и фронтенда
server/
  server.go               # Gin-движок, маршруты, статика, graceful shutdown
  middleware/             # логирование, recovery, RequireAuth (сессии)
  handlers/               # HTTP-хендлеры, по одному файлу на домен kvmrun
  templates/              # встроенные HTML-шаблоны (layout + страницы)
web/static/               # встроенные ресурсы фронтенда (css/, js/)
Makefile
```

## Запуск

```sh
make build
./bin/dashboard --listen :8080
```

Основные флаги:

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `--listen` | `:8080` | адрес HTTP-сервера |
| `--daemon` | `unix:@/run/kvmrund.sock` | адрес демона kvmrund |
| `--cert-dir` | `/usr/share/kvmrun/tls` | каталог с `client.crt`/`client.key` |
| `--pam-service` | `login` | PAM-сервис для проверки паролей (`/etc/pam.d/<name>`) |
| `--session-ttl` | `12h` | время жизни сессии |
| `--cookie-name` | `kvmrun-dashboard-session` | имя cookie сессии |

Dashboard подключается к демону с теми же настройками, что и CLI vmm
по умолчанию; без сертификатов в `--cert-dir` подключение идёт без TLS
(то же поведение, что у `vmm`).

## Авторизация

- Страница `GET/POST /login` — форма, пароль проверяется PAM-стеком хоста
  (`pam_authenticate` с сервисом из `--pam-service`).
- После успешного входа ставится HttpOnly-cookie с сессионным ID (32 байта
  crypto/rand); сессии живут `--session-ttl` и удаляются при перезапуске.
- Все маршруты, кроме `/login`, `/logout`, `/static/*` и `/healthz`,
  требуют сессии: браузеры редиректят на `/login`, JSON API отвечает 401.

## Статус

- Список машин (`/machines`, `GET /api/v1/machines`), детализация
  (`/machines/:name`, `GET /api/v1/machines/:name`) — работают.
- Действия: Start/Stop/Restart/Reset (кнопки на страницах), VNC-активация
  (`POST /api/v1/machines/:name/vnc`), задачи (`GET /api/v1/tasks`),
  конфиг демона (`GET /api/v1/system`) — работают.
- VNC-консоль на странице ВМ (концепт 10): встроенный noVNC v1.7.0
  (embed в `web/novnc`, отдаётся на `/novnc/*`) и WS-прокси
  `GET /api/v1/machines/:name/vnc-ws?port=N` (mini-websockify, без внешнего
  websockify; дайлит `127.0.0.2:<port>` — дефолтный `VNCHost` демона — с
  фолбэком на `127.0.0.1`). Консоль открывается для RUNNING-машин, пароль
  идёт во fragment URL, при остановке ВМ консоль закрывается автоматически.
- SSH-консоль на странице ВМ (концепт 11): встроенный xterm.js 5.5.0
  (embed в `web/xterm`, отдаётся на `/xterm/*`) и WS-прокси
  `GET /api/v1/machines/:name/ssh-ws` к встроенному SSH-серверу
  phoenix-guest-agent (AF_VSOCK, порт 4949). Ключ пользователя берётся у
  агента по gRPC (vsock-порт 8383, mTLS, клиентский сертификат встроенного
  в бинарник агентского PKI — пакет `cert`, отдельного от PKI демона; файлы
  `CA.crt`/`client.crt`/`client.key` не коммитятся, но должны быть рядом с
  `cert/embed.go` при сборке) — агент пересоздаёт ключ при каждом старте.
  Терминальный ввод/вывод
  идёт бинарными WS-фреймами, resize — текстовым фреймом
  `{"type":"resize","cols":N,"rows":N}`. Кнопка Console стала dropdown
  (VNC / Agent Built-in SSH); SSH-пункт недоступен для машин без vsock.
  При остановке ВМ консоль закрывается автоматически, Disconnect завершает
  SSH-сессию.
- TODO: storage (диски), network, cloudinit, hardware.