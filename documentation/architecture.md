# Application Architecture

GitHub Release Notifier consists of two application services:

- `app`: public HTTP API, subscription logic, release monitor, and integration outbox publisher.
- `notification-service`: RabbitMQ consumer that renders notification emails and sends them through SMTP.

## Container View

```mermaid
flowchart LR
    user["User / Browser"]
    github["GitHub API"]
    smtp["SMTP provider"]

    subgraph runtime["Application runtime"]
        app["app\nHTTP API + release monitor + outbox publisher"]
        notification["notification-service\nRabbitMQ consumer + email delivery"]
    end

    subgraph data["Data and messaging"]
        appdb[("PostgreSQL\napp database")]
        notificationdb[("PostgreSQL\nnotification database")]
        rabbit[("RabbitMQ\nnotifications exchange")]
    end

    subgraph observability["Observability"]
        prometheus["Prometheus"]
        grafana["Grafana"]
        loki["Loki"]
        alloy["Grafana Alloy"]
    end

    user -->|"HTTP: subscribe, confirm, unsubscribe, list"| app
    app -->|"REST: validate repository, read releases"| github
    app -->|"SQL: users, repositories, subscriptions, outbox"| appdb
    app -->|"publish notification messages"| rabbit
    notification -->|"consume notification messages"| rabbit
    notification -->|"SQL: inbox / idempotency"| notificationdb
    notification -->|"SMTP"| smtp

    prometheus -->|"scrape metrics"| app
    prometheus -->|"scrape metrics"| notification
    alloy -->|"collect container logs"| app
    alloy -->|"collect container logs"| notification
    alloy -->|"push logs"| loki
    grafana -->|"query metrics"| prometheus
    grafana -->|"query logs"| loki
```

## App Components

```mermaid
flowchart TB
    subgraph app["cmd/app"]
        router["Gin router\n/api/*"]
        subscriptionService["Subscription service"]
        releaseMonitor["Release monitor"]
        notificationQueue["Notification queue service\nwrites integration outbox"]
        outboxPublisher["Integration outbox publisher"]
        stores["Repositories\nusers, tracked repositories, subscriptions, outbox"]
        githubClient["GitHub client"]
        metrics["Metrics"]
    end

    postgres[("App PostgreSQL")]
    rabbit["RabbitMQ"]
    github["GitHub API"]

    router --> subscriptionService
    subscriptionService --> stores
    subscriptionService --> githubClient
    subscriptionService --> notificationQueue
    releaseMonitor --> stores
    releaseMonitor --> githubClient
    releaseMonitor --> notificationQueue
    notificationQueue --> stores
    outboxPublisher --> stores
    outboxPublisher --> rabbit
    stores --> postgres
    githubClient --> github
    router --> metrics
    releaseMonitor --> metrics
```

## Notification Service Components

```mermaid
flowchart TB
    subgraph notification["cmd/notification-service"]
        consumer["RabbitMQ consumer"]
        inboxHandler["Message inbox handler"]
        renderer["Email template renderer"]
        sender["SMTP sender"]
        metrics["Notification metrics HTTP handler"]
    end

    rabbit["RabbitMQ"]
    notificationdb[("Notification PostgreSQL")]
    smtp["SMTP provider"]
    prometheus["Prometheus"]

    rabbit --> consumer
    consumer --> inboxHandler
    inboxHandler --> notificationdb
    inboxHandler --> renderer
    inboxHandler --> sender
    sender --> smtp
    prometheus --> metrics
```

## Subscribe Flow

```mermaid
sequenceDiagram
    actor User
    participant API as app HTTP API
    participant GitHub as GitHub API
    participant AppDB as App PostgreSQL
    participant Outbox as Outbox publisher
    participant Rabbit as RabbitMQ
    participant Mail as notification-service
    participant NotificationDB as Notification PostgreSQL
    participant SMTP as SMTP provider

    User->>API: POST /api/subscribe
    API->>GitHub: Check repository exists
    API->>AppDB: Create user, tracked repository, subscription, and outbox message
    API-->>User: HTTP 200
    Outbox->>AppDB: Claim pending SubscriptionConfirmationRequested
    Outbox->>Rabbit: Publish notification message
    Rabbit->>Mail: Deliver message
    Mail->>NotificationDB: Record inbox/idempotency state
    Mail->>SMTP: Send confirmation email
```

## Confirm And Unsubscribe Flows

```mermaid
sequenceDiagram
    actor User
    participant API as app HTTP API
    participant DB as App PostgreSQL

    User->>API: GET /api/confirm/{token}
    API->>DB: Set subscription confirmed by confirmation token
    API-->>User: HTTP 200

    User->>API: GET /api/unsubscribe/{token}
    API->>DB: Delete subscription by cancellation token
    API-->>User: HTTP 200
```

## Release Notification Flow

```mermaid
sequenceDiagram
    participant Monitor as app release monitor
    participant AppDB as App PostgreSQL
    participant GitHub as GitHub API
    participant Outbox as Outbox publisher
    participant Rabbit as RabbitMQ
    participant Mail as notification-service
    participant NotificationDB as Notification PostgreSQL
    participant SMTP as SMTP provider

    Monitor->>AppDB: Load confirmed subscriptions grouped by repository
    Monitor->>GitHub: Get latest release for repository
    alt first scan for repository
        Monitor->>AppDB: Initialize last_seen_tag
    else new release tag found
        Monitor->>AppDB: Write ReleaseNotificationRequested messages and update last_seen_tag
        Outbox->>AppDB: Claim pending outbox message
        Outbox->>Rabbit: Publish notification message
        Rabbit->>Mail: Deliver message
        Mail->>NotificationDB: Record inbox/idempotency state
        Mail->>SMTP: Send release email
    else no new release
        Monitor->>AppDB: Keep current last_seen_tag
    end
```

## Persistence Boundaries

- `app` owns users, tracked repositories, subscriptions, and integration outbox tables.
- `notification-service` owns notification inbox/idempotency state.
- RabbitMQ is the asynchronous boundary between notification-producing logic and email delivery.
- PostgreSQL is a shared server in Docker Compose, but the app and notification service use separate migration sets and logical databases.

## Reliability Notes

- Subscription confirmation emails and release notification emails are written to the app database before publishing.
- The integration outbox publisher claims pending outbox messages and publishes them to RabbitMQ.
- The notification consumer records incoming messages in its inbox store before delivery, so repeated RabbitMQ deliveries can be handled idempotently.
- The release monitor stores `last_seen_tag` per tracked repository to avoid repeatedly notifying for the same release.
- Prometheus scrapes `app` and `notification-service`; Alloy collects logs from the same two application containers and pushes them to Loki.
