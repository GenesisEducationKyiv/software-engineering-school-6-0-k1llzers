# GitHub Release Notifier

Сервіс, який дозволяє підписати email на релізи GitHub-репозиторію, підтвердити підписку, регулярно перевіряє нові релізи та надсилає сповіщення підписникам.

## API:
- `POST /api/subscribe`
- `GET /api/confirm/{token}`
- `GET /api/unsubscribe/{token}`
- `GET /api/subscriptions?email=...`

## Що робить сервіс

Користувач підписується на GitHub-репозиторій у форматі `owner/repo`, підтверджує email і після цього отримує листи про нові релізи.

Ключова бізнес-логіка:
- при створенні підписки сервіс перевіряє, що репозиторій існує в GitHub;
- для кожного репозиторію в БД зберігається `last_seen_tag`;
- фоновий scanner регулярно перевіряє всі підтверджені підписки;
- якщо з'явився новий тег релізу, сервіс ставить листи в outbox і оновлює `last_seen_tag`;
- фактична відправка email виконується окремим dispatcher-ом.

## Outbox Pattern

Надійна доставка сповіщень є core-функцією цього застосунку, тому email не відправляються прямо всередині бізнес-логіки.

Використовується `mail_outbox`:
- під час підтвердження підписки або знаходження нового релізу сервіс записує лист у таблицю outbox в тій самій транзакції, що й дані про підписку;
- тільки після успішного commit фоновий dispatcher забирає лист із outbox і намагається його відправити;
- якщо відправка впала, запис не втрачається: він лишається в outbox з інформацією про помилку і може бути повторно оброблений.

Це дає гарантію: якщо підписка або новий тег вже збережені, сповіщення має бути обов'язково доставлене.

## Конфігурація

Застосунок читає конфіг з `config.yaml` у корені проєкту. Якщо файла немає, частина значень буде взята з дефолтів, але для запуску mail-конфіг треба задати явно.

Приклад `config.yaml`:

```yaml
server:
  port: "8080"

database:
  url: "postgres://admin:1111@localhost:5433/github_release_notifier?sslmode=disable"

github:
  token: ""

mail:
  host: "smtp.gmail.com"
  port: 587
  username: "your-email@gmail.com"
  password: "your-app-password"
  from: "your-email@gmail.com"
  api_base_url: "http://localhost:8080/api"
```

### Властивості

- `server.port` - порт HTTP API.
- `database.url` - рядок підключення до PostgreSQL.
- `github.token` - GitHub token. Необов'язковий, але дуже бажаний, бо без токена ліміт запитів значно нижчий.
- `mail.host` - SMTP host.
- `mail.port` - SMTP port.
- `mail.username` - SMTP username.
- `mail.password` - SMTP password або app password.
- `mail.from` - email відправника.
- `mail.api_base_url` - базовий URL, який вставляється у листи для confirm/unsubscribe посилань.

## Запуск локально

### 1. Підняти PostgreSQL

Можна локально або через Docker. Якщо використовувати `docker-compose`, база буде доступна на `localhost:5433`.

### 2. Створити `config.yaml`

Заповни `database.url` і SMTP-параметри. Якщо хочете уникнути жорсткого rate limit GitHub, додайте `github.token`.

### 3. Запустити застосунок

```bash
go run ./cmd/app
```

## Запуск через Docker Compose

У репозиторії є `Dockerfile` і `docker-compose.yaml`.

Поточний `docker-compose.yaml` очікує, що застосунок всередині контейнера читає `config.yaml`, а база доступна під hostname `db`.

Для Docker значення в `config.yaml` мають виглядати так:

```yaml
server:
  port: "8080"

database:
  url: "postgres://admin:1111@db:5432/github_release_notifier?sslmode=disable"

github:
  token: ""

mail:
  host: "smtp.gmail.com"
  port: 587
  username: "your-email@gmail.com"
  password: "your-app-password"
  from: "your-email@gmail.com"
  api_base_url: "http://localhost:8080/api"
```


Запуск:

```bash
docker compose up --build
```

Після цього:
- API буде доступне на `http://localhost:8080/api`;
- PostgreSQL буде доступний на `localhost:5433`.

### Rate limit GitHub

Якщо GitHub повертає rate limit, scanner не продовжує безкінечно полити зовнішнє API. Він логує проблему і робить паузу перед наступною спробою. Це знижує ризик нескінченного циклу помилок.

## Що варто покращити далі

### Граничний кейс з новим підписником після релізу

Поточна модель працює на рівні репозиторію через спільний `last_seen_tag`. Це означає такий сценарій:
- користувач 1 уже підписаний на репозиторій `A`;
- у репозиторії `A` виходить новий реліз;
- scanner ще не встиг його обробити;
- користувач 2 підписується на `A`;
- якщо під час підписки сервіс бачить уже новий тег, то цей тег стає базовим станом tracked repository;
- після цього можлива ситуація де користувач 2 отримає сповіщення про реліз який відбувся до його підписки.