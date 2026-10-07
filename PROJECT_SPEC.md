# Metrology Assurance Suite

## Project Goal

Metrology Assurance Suite is a local HTTP service that keeps a traceable record
of laboratory measurement assurance work. It registers devices, opens a
verification case for one device, records technician measurements, requests a
repeat measurement when tolerance is exceeded, and seals the final evidence
record after review.

It also registers reference material / reference artifact batches with an
expiry, a total allowed usage amount, and a verification basis. Measurements
that consume reference material cite the actual batch used; unknown, expired,
exhausted, or failed-check batches cannot form a valid citation, and the
captured citation is frozen into the evidence so later batch changes never
rewrite a sealed measurement fact.

The service is intentionally self-contained. It uses an in-memory repository
with atomic copy-on-write mutations and does not require a database or an
external service.

## Users

- Laboratory steward: registers devices and opens verification cases, and
  registers reference batches and their verification checks.
- Measurement technician: records observed values for an open case, citing the
  reference batch actually consumed.
- Quality verifier: reviews accepted measurements and seals the case.

## Core Entities

- Device: a laboratory asset with a unique asset tag, model, room, steward, and
  lifecycle state.
- VerificationCase: one bounded assurance activity for a device, with a nominal
  value, tolerance, current state, version, and assigned steward.
- Measurement: an observed value, computed deviation, acceptance decision,
  technician, timestamp, and an optional immutable reference batch citation.
- ReferenceBatch: one registered lot of a reference material or artifact with a
  unique lot, unit, total allowed usage amount, accumulated used amount, expiry,
  verification basis, and owner.
- ReferenceCheck: an append-only verification result (passed/failed) for a
  batch; the most recent check is the current verdict.
- ReferenceCitation: the frozen snapshot captured when a measurement consumes a
  batch (lot, amount, allowance, expiry, check basis, and check time).
- EvidenceSeal: the immutable closing record for a reviewed case, including the
  selected accepted measurement and its reference citation.
- AuditEvent: an append-only trace of meaningful case, device, and reference
  batch changes.

## Workflows

### 1. Register Device

The steward sends `POST /devices` with an asset tag, model, room, and steward.
The service validates the request, normalizes the asset tag, rejects duplicate
tags, and creates an active device.

### 2. Open Verification Case

The steward sends `POST /cases` with a device ID, nominal value, tolerance, and
owner. The service verifies that the device exists, is active, and has no
unfinished case. A valid request creates a case in `awaiting_measurement`.

### 3. Record Measurement

A technician sends `POST /cases/{id}/measurements`. The case must be
`awaiting_measurement`. The service computes the signed deviation from the
nominal value. An in-tolerance measurement moves the case to
`review_ready`; an out-of-tolerance measurement moves it to
`recheck_required`.

### 4. Reopen Verification Case

The steward sends `POST /cases/{id}/reopen` after an out-of-tolerance result.
Only `recheck_required` cases may move back to `awaiting_measurement`. The
service records the actor and reason in the audit trail.

### 5. Seal Verification

The verifier sends `POST /cases/{id}/seal` after an accepted measurement. Only
`review_ready` cases may be sealed. The service selects the accepted
measurement with the smallest absolute deviation, creates exactly one evidence
seal, moves the case to `sealed`, and rejects later mutations.

### 6. Register Reference Batch

The steward sends `POST /reference-batches` with a name, unique lot, unit, total
allowed usage amount, expiry, verification basis, and owner. The service
normalizes the lot, rejects duplicate lots, creates an `active` batch, and
records an initial passing verification check.

### 7. Record Verification Check

The steward sends `POST /reference-batches/{id}/checks` with a verdict of
`passed` or `failed`, the basis, actor, and optional note. Checks are
append-only; the most recent check is the batch's current verdict. A failed
check blocks further citations until a later passing check restores the batch.

### 8. Cite A Reference Batch In A Measurement

When a measurement consumes reference material, the technician sends
`reference_batch_id` and `reference_amount` with the measurement. The service
rejects the request, recording nothing, when the batch is unknown, its latest
check has not passed, it is expired at use time, or the amount exceeds the
remaining allowance. A valid citation consumes the amount from the batch and
freezes a `ReferenceCitation` snapshot onto the measurement; the accepted
measurement's snapshot is copied into the evidence seal at sealing time.

## State And Rules

Device state:

- `active -> retired`
- `retired` is terminal.

VerificationCase state:

- `awaiting_measurement -> review_ready`
- `awaiting_measurement -> recheck_required`
- `recheck_required -> awaiting_measurement`
- `review_ready -> sealed`
- `sealed` is terminal.

ReferenceBatch status is derived, not stored. A batch is `check_failed` when its
most recent check failed, otherwise `expired` at/after its expiry, otherwise
`exhausted` when the used amount reaches the allowance, otherwise `active`.

Required invariants:

- Asset tags are unique after case-insensitive normalization.
- Reference lots are unique after case-insensitive normalization.
- A device has at most one unfinished verification case.
- Measurements are accepted only while a case is awaiting measurement.
- A tolerance must be positive.
- A reference batch's used amount never exceeds its allowed amount.
- A measurement citation requires a registered batch, a latest passing check, an
  in-date batch at use time, and an amount within the remaining allowance;
  otherwise no measurement and no consumption is recorded.
- Batch id and usage amount must be provided together.
- A case can have at most one evidence seal.
- A sealed case cannot accept measurements, reopen, or seal again.
- A citation snapshot on a measurement, and the copy on the evidence seal, is
  immutable; expiring, exhausting, or failing a batch later never rewrites a
  recorded measurement or a sealed record.
- Every mutation that changes a case or a reference batch also appends an audit
  event in the same repository commit.

## Modules

- `cmd/metrology-service`: process entrypoint and graceful HTTP shutdown.
- `cmd/workflowcheck`: bounded smoke checks that call the public HTTP API.
- `internal/httpapi`: routing, request decoding, and stable error mapping.
- `internal/assurance`: workflow orchestration and atomic decisions.
- `internal/domain`: entities, validation, state transitions, and errors.
- `internal/repository`: mutex-protected in-memory persistence with copy-on-write
  commits.
- `internal/contracts`: request and response contracts shared by the HTTP layer.

## Public Interfaces

- `GET /healthz`
- `POST /devices`
- `GET /devices/{id}`
- `POST /cases`
- `GET /cases/{id}`
- `POST /cases/{id}/measurements`
- `POST /cases/{id}/reopen`
- `POST /cases/{id}/seal`
- `GET /cases/{id}/events`
- `POST /reference-batches`
- `GET /reference-batches`
- `GET /reference-batches/{id}`
- `POST /reference-batches/{id}/checks`

## Validation Plan

The baseline build command is `go build ./...`. Each declared workflow has a
bounded production smoke command under `cmd/workflowcheck`. The command starts
an HTTP test server in the same process, calls the public routes, and verifies
both success and failure behavior. The seal check also races two concurrent
seal requests to prove that the repository mutation lock permits exactly one
seal. The reference checks verify batch registration and duplicate lots,
append-only passing/failing checks, the unknown/expired/exhausted/failed-check
citation gates with no partial consumption, and that a post-seal batch failure
does not rewrite the frozen citation.

Tests are intentionally deferred for this initialization baseline. The later
engineering-task stage will add unit and integration tests, including red/green
verification for the workflows and their failure paths.
