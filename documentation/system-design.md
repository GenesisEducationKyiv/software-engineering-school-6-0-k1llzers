# GitHub Release Notifier System Design

## Overview

GitHub Release Notifier is a backend service that allows users to subscribe to GitHub repositories by email, confirm the subscription, and receive email notifications when a new release appears.

The system is implemented as a single Go service with PostgreSQL as the primary datastore.
It integrates with:

- GitHub REST API for repository validation and latest release checks.
- SMTP for email delivery.

## System Requirements

### Functional Requirements

- User can subscribe to a GitHub repository by email.
- System validates that the repository exists.
- System sends a confirmation email before activating the subscription.
- User can confirm or cancel the subscription by tokenized link.
- System periodically checks subscribed repositories for new releases.
- System sends release notifications to confirmed subscribers.
- User can list subscriptions by email.

### Non-Functional Requirements

- Email delivery requests must not be lost after business state is committed.
- System should tolerate temporary SMTP and network failures.
- Release checks should continue operating under transient GitHub API failures.
- Request latency should not depend on SMTP delivery latency.

### Constraints

- The system uses PostgreSQL as the primary datastore.
- Release detection is polling-based.
- Email delivery uses SMTP.
- The current architecture is a single Go service.
- User identity is based on email, without authentication or user accounts.
- Delivery semantics are at-least-once, not exactly-once.

## Capacity Estimates

### Users and Subscription Shape

- Registered users: 10,000
- Average repositories per user: 5
- Unique tracked repositories: 60

### Incoming HTTP Load

- API requests: 1,000 RPS(max)
- Sustained HTTP throughput: 100 RPS
- Average subscribe requests: 30/day
- Peak subscribe requests: 10 RPS
- Average confirmation and unsubscribe requests: 30/day
- Average list requests: 2,000/day

### Email Delivery Load

- Average confirmation emails: 30/day
- Peak confirmation emails: 200/day
- Average release notification emails: 3,500/day
- Peak release notification emails: 15,000/day

### External API Load

- External GitHub API requests from subscription flow: 60/day
- External GitHub API requests from release polling: 86,400/day

### Bandwidth

- Incoming HTTP traffic: 2 GB/month
- Outgoing SMTP traffic: 1 GB/month

### Data Storage

- Persistent data in PostgreSQL without outbox history: 20 MB
- Annual outbox storage growth with sent-email retention: 4 GB/year
- Total annual data growth including indexes and metadata: 5 GB/year

## High-Level Architecture

```text
+--------+     HTTP      +----------------+
| Client | -----------> | Subscriber API |
+--------+              +--------+-------+
                                  |
                                  v
                        +-------------------+
                        |    PostgreSQL     |
                        | users             |
                        | subscriptions     |
                        | tracked_repos     |
                        | mail_outbox       |
                        +----+---------+----+
                             ^         ^
                             |         |
              read/write state         | claim and update outbox
                             |         |
                +------------+         +------------------+
                |                                       |
                |                                       |
      +---------+---------+                   +---------+---------------+
      |   Release Monitor |                   | Email Outbox Dispatcher |
      +---------+---------+                   +------------+------------+
                |                                          |
                v                                          v
         +--------------+                           +-------------+
         |  GitHub API  |                           | SMTP Server |
         +--------------+                           +-------------+
```

## Main Components

### 1. HTTP API

The HTTP API exposes subscription-related endpoints:

- `POST /api/subscribe`
- `GET /api/confirm/{token}`
- `GET /api/unsubscribe/{token}`
- `GET /api/subscriptions?email=...`

Responsibilities:

- validate input;
- translate HTTP requests into application service calls;
- map domain errors to HTTP status codes.

The HTTP layer is intentionally thin and does not contain business logic.

### 2. Application Services

There are two main application services:

- `SubscriptionService`
- `ReleaseMonitor`

`SubscriptionService` handles:

- repository name validation;
- GitHub repository existence check;
- initial release state lookup;
- user creation;
- repository tracking creation;
- subscription creation;
- enqueueing of confirmation emails.

`ReleaseMonitor` handles:

- loading confirmed subscriptions;
- grouping them by tracked repository;
- polling GitHub for the latest release;
- enqueueing release notification emails;
- updating repository release state.

Application services own the main use-case orchestration and define transaction boundaries.

### 3. PostgreSQL

PostgreSQL stores:

- users;
- tracked repositories;
- subscriptions;
- email outbox records;
- schema migration state.

The database is the source of truth for subscription state and queued email work.

### 4. GitHub Client

The GitHub client is responsible for:

- checking whether a repository exists;
- fetching the latest release;

### 5. Mail Service

The mail service:

- renders confirmation and release templates;
- builds confirm and unsubscribe URLs;
- writes rendered email payloads into the outbox.

### 6. Outbox Dispatcher

The outbox dispatcher is a background loop that:

- claims the next pending email from `mail_outbox`;
- attempts SMTP delivery;
- marks successful deliveries as sent;
- releases failed deliveries for retry.

This component provides durable retry behavior and isolates SMTP failures from the main request path.

## Data Model

### `users`

Stores unique email addresses.

### `tracked_repositories`

Stores unique repositories by `owner + name` and the current `last_seen_tag`.

Important design choice:

- release state is stored per repository, not per subscription.

This reduces GitHub API calls because one repository is checked once for all confirmed subscribers.
The trade-off is that release-tracking state is shared across subscribers.

### `subscriptions`

Connects a user to a tracked repository and stores:

- confirmation status;
- confirmation token;
- cancellation token.

The uniqueness constraint on `(user_id, tracked_repository_id)` prevents duplicate subscriptions for the same user and repository.

### `mail_outbox`

Stores rendered emails before delivery.

Important fields:

- recipient email;
- subject;
- HTML body;
- attempts;
- `processing_started_at`;
- `sent_at`;
- `last_error`.

This table is the basis of reliable delivery and retryability.

## Operational Concerns

The most important things to monitor are:

- GitHub rate limit failures;
- outbox queue growth;
- repeated SMTP delivery failures;
- dispatcher availability;
- migration failures on startup.

Basic operational practices should include:

- logging all outbox send failures;
- tracking retry counts and stuck messages;
- monitoring release-check duration and error rate;
- monitoring database connectivity.

## Security and Abuse Considerations

- Subscription confirmation prevents immediate unsolicited notifications.
- Unsubscribe tokens allow one-click cancellation.
- Email addresses are stored as primary user identity, so database access must be protected.
- GitHub token should be stored in configuration securely.
- SMTP credentials should be treated as secrets.

## Current Limitations

- No authentication or account ownership model.
- No per-user notification preferences.
- No dead-letter queue for permanently failing emails.
- No explicit cleanup strategy for old outbox rows.
- No per-subscriber release cursor.

