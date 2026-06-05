# ADR 002: Modular Monolith Boundaries

## Status

Accepted

## Context

The current system is implemented as a single Go service with technical layering:

- `httpapi`
- `service`
- `storage`
- `mail`
- `github`
- `db`

This structure is workable for a small codebase, but it mixes several business areas inside shared packages. The main coupling points are:

- subscription lifecycle and release-tracking logic both participate in repository state decisions;
- one `storage` package serves multiple unrelated business capabilities;
- email preparation, durable queueing, and SMTP delivery are grouped together without a clear module boundary;
- the `domain` package acts mostly as a shared DTO/error container rather than a bounded domain model.

We need to refactor the application toward modular architecture first, while keeping a single deployable unit during the initial phase.
## Decision

The system will be refactored into a modular monolith before any microservice extraction.

The target internal modules are:

1. `subscriptions`
2. `release_tracking`
3. `notifications`
4. `platform`

No module will be extracted into a separate service yet. The immediate goal is to define clear ownership boundaries, reduce cross-module knowledge, and introduce explicit internal contracts that can later support service extraction if still needed.

## Module Boundaries

### 1. Subscriptions

Responsibilities:

- create subscription requests;
- confirm subscriptions;
- cancel subscriptions;
- list subscriptions for a user email;
- own subscription-specific validation and lifecycle rules.

Owns:

- subscription application API;
- subscription write model;
- subscription list query model;
- user-to-repository subscription relationships.

Must not own:

- GitHub release cursor state;
- release polling workflow;
- email delivery implementation details.

### 2. Release Tracking

Responsibilities:

- register repositories for tracking;
- poll GitHub for latest releases;
- detect release changes;
- own release cursor state for tracked repositories.

Owns:

- tracked repository model;
- release monitor workflow;
- repository release cursor such as `last_seen_tag` or its future replacement;
- projections required for release polling.

Must not own:

- HTTP subscription endpoints;
- SMTP delivery details;
- subscription confirmation rules.

### 3. Notifications

Responsibilities:

- build notification payloads;
- render email templates;
- persist durable notification work;
- dispatch notifications to SMTP;
- handle retry-oriented delivery workflow.

Owns:

- notification application API;
- outbox model and processing workflow;
- email template rendering;
- delivery adapters.

Must not own:

- subscription lifecycle decisions;
- GitHub polling logic;
- tracked repository release cursor.

### 4. Platform

Responsibilities:

- infrastructure and cross-cutting concerns only.

Owns:

- database connection and transaction primitives;
- configuration loading;
- logging;
- metrics;
- HTTP, GitHub, and SMTP adapters as infrastructure implementations used by modules.

Platform is not a business module. It provides infrastructure services to business modules and must not absorb business orchestration.

## Data Ownership

The intended ownership of current persistence structures is:

- `users` and `subscriptions`: `subscriptions`
- `tracked_repositories`: `release_tracking`
- `mail_outbox`: `notifications`

## Dependency Rules

The modular monolith must follow these dependency rules:

- `httpapi` talks to module application APIs, not directly to repositories.
- modules may depend on `platform`, but not on each other's storage internals.
- modules communicate through explicit interfaces, commands, or in-process events.
- read models required by one module must not be hidden inside another module's repository package.
- no shared `storage` package should remain as the long-term organization model.
- no shared `service` package should remain as the long-term organization model.

## Consequences

### Positive Consequences

- module responsibilities become explicit and reviewable;
- the team can refactor in trunk-friendly increments;
- future microservice extraction becomes optional rather than forced early;
- the notification domain remains a clean future candidate for extraction;
- business rules move closer to their owning modules.

### Negative Consequences

- the codebase will temporarily contain transition structures during the migration;
- some logic may exist behind compatibility facades before the old packages disappear;
- repository and query refactors will touch many files even without behavior changes.
