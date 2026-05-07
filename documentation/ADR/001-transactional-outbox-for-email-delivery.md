# ADR 001: Transactional Outbox for Email Delivery

## Status

Accepted

## Context

The system sends emails for subscription confirmation and release notifications.
Email delivery is part of the core business flow, because a subscription is not useful if the user does not receive confirmation and release messages.

We need an email sending approach that minimizes the risk of losing messages in failure scenarios such as:

- a temporary network problem during SMTP communication;
- SMTP server unavailability;
- application shutdown while an email is being sent;
- a failure after business data has already been committed to the database.

The main risk is the following inconsistency:

1. the application persists business state successfully;
2. the application tries to send an email directly;
3. the process crashes or the SMTP call fails;
4. the database says the action happened, but the user never receives the email.

For this project, losing an email is worse than sending the same email more than once. The design therefore should favor durability and retryability over exactly-once delivery semantics.

## Considered Options

### Option 1: Send email as part of the same database transaction that persists the success state

The application performs the business operation and tries to send the email in the same database transaction that persists the successful state.
This means the successful state is not supposed to be persisted unless the email is also sent successfully.

#### Pros

- If the subscription flow cannot send the confirmation email, the user gets a clear error and the subscription is not stored.
- No additional database schema changes.
- No background dispatcher job is required.

#### Cons

- SMTP is an external side effect and cannot participate in the database transaction in an atomic way.
- In release notification flows, partial delivery can still happen. For example, if 100 users are subscribed to a repository and the system successfully sends emails to 50 of them before the next send fails, the successful state will not be persisted even though part of the audience has already received the notification. On the next check, some users may receive duplicate emails while others still receive none.
- Request latency starts to depend on SMTP responsiveness and availability.
- Delivery problems for specific users are harder to debug because the system has no durable record of pending, failed, or partially processed email work.
- Application shutdown, process crash, or transient network failure at the wrong moment can still leave the system in an inconsistent state relative to actual email delivery.

### Option 2: Use a transactional outbox table in database and a background dispatcher

The application writes an email record to an outbox table in the same database transaction as the business data.
After commit, a separate dispatcher process reads pending outbox records, attempts delivery, marks successful records as sent, and leaves failed records available for retry.

#### Pros

- Strong consistency between business changes and email scheduling.
- Emails are durable once the transaction is committed.
- Retries are possible after SMTP failures, network errors, or shutdowns.
- Works with existing PostgreSQL infrastructure.
- Decouples SMTP latency from the request path.
- If higher email throughput is needed later, email delivery can be extracted into a separate service, and the outbox can be forwarded to a message broker for inter-service communication.

#### Cons

- More complex than direct sending.
- Requires a background dispatcher.
- Delivery becomes eventually consistent rather than immediate.
- Duplicate delivery is still possible in some failure windows, so consumers must tolerate at-least-once behavior.

## Decision

We will use the transactional outbox pattern for email delivery.

The application will not send emails directly from the main business flow.
Instead, when the system creates a subscription or detects a new release, it will insert an email payload into a `mail_outbox` table in the same PostgreSQL transaction as the related business state change.

After the transaction commits successfully, a background dispatcher will:

- claim the next pending outbox record;
- attempt to send the email through SMTP;
- mark the record as sent on success;
- release the record for later retry on failure, preserving the error information.

This gives us durable email scheduling and retry support without introducing external messaging infrastructure.

## Consequences

### Positive Consequences

- If business data is committed, the corresponding email is also durably recorded.
- Temporary SMTP or network failures do not lose emails.
- Application shutdown during delivery does not destroy queued messages.
- Email sending is retriable and observable through the outbox table.
- The request/business flow is isolated from SMTP latency.
- The solution is easier to scale, because email delivery can be processed independently of the request path and can later be moved behind a dedicated mail service if needed.

### Negative Consequences

- The system is more complex than direct SMTP sending.
- Emails may be delivered with a delay because sending is asynchronous.
- The dispatcher must be monitored and kept running.
- Some failure windows can still produce duplicate sends, so the delivery model is at-least-once, not exactly-once.
- Outbox records require lifecycle management and operational visibility.

