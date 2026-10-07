# Metrology Assurance Suite

## Project Goal

Metrology Assurance Suite is a local HTTP service that keeps a traceable record
of laboratory measurement assurance work. It registers devices, opens a
verification case for one device, records technician measurements, requests a
repeat measurement when tolerance is exceeded, and seals the final evidence
record after review.

The service is intentionally self-contained. It uses an in-memory repository
with atomic copy-on-write mutations and does not require a database or an
external service.

## Users

- Laboratory steward: registers devices and opens verification cases.
- Measurement technician: records observed values for an open case.
- Quality verifier: reviews accepted measurements and seals the case.

## Core Entities

- Device: a laboratory asset with a unique asset tag, model, room, steward, and
  lifecycle state.
- VerificationCase: one bounded assurance activity for a device, with a nominal
  value, tolerance, current state, version, and assigned steward.
- Measurement: an observed value, computed deviation, acceptance decision,
  technician, timestamp, and an optional immutable snapshot of the reference
  batch it consumed.
- ReferenceBatch: one registered lot of a reference material, with material
  name, normalized lot number, steward, check basis, expiry, allowed usage
  budget, consumed usage, and lifecycle state.
- EvidenceSeal: the immutable closing record for a reviewed case, including the
  selected accepted measurement.
- AuditEvent: an append-only trace of meaningful case, device, and batch
  changes.

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

When the measurement consumes a reference material, the request carries
`batch_id` and an optional `batch_quantity` (default 1). The service verifies
inside the same atomic commit that the batch exists, is `active`, is not
expired at the measurement time, and has enough remaining usage. Any violation
rejects the whole measurement: no measurement, no case transition, and no
usage deduction is committed. A valid request deducts the quantity from the
batch and stores an immutable batch snapshot (batch id, material, lot, check
basis, quantity, expiry) on the measurement.

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

The steward sends `POST /batches` with a material name, lot number, steward,
check basis, allowed usage count, and expiry time. The service normalizes the
lot number, rejects a duplicate lot for the same material, and creates an
`active` batch with zero consumed usage.

### 7. Transition Reference Batch

The steward sends `POST /batches/{id}/fail-check`, `/reinstate`, or `/retire`
with an actor and reason. A failed check blocks the batch from new
measurements until it is reinstated; retiring is terminal. Every transition
appends an audit event. Batch transitions never rewrite the batch snapshots
already stored on recorded measurements.

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

ReferenceBatch state:

- `active -> failed`
- `failed -> active`
- `active -> retired`
- `failed -> retired`
- `retired` is terminal.

Required invariants:

- Asset tags are unique after case-insensitive normalization.
- A device has at most one unfinished verification case.
- Measurements are accepted only while a case is awaiting measurement.
- A tolerance must be positive.
- A case can have at most one evidence seal.
- A sealed case cannot accept measurements, reopen, or seal again.
- A batch lot is unique per material after case-insensitive normalization.
- A measurement may cite a batch only when the batch exists, is active, is
  unexpired at the measurement time, and has enough remaining usage.
- Batch consumption, the measurement, the case transition, and the audit
  events commit atomically; a rejected citation deducts nothing.
- The batch snapshot stored on a measurement is immutable; later batch state
  changes never rewrite recorded measurement facts.
- Every mutation that changes a case also appends an audit event in the same
  repository commit.

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
- `POST /batches`
- `GET /batches/{id}`
- `POST /batches/{id}/fail-check`
- `POST /batches/{id}/reinstate`
- `POST /batches/{id}/retire`

## Validation Plan

The baseline build command is `go build ./...`. Each declared workflow has a
bounded production smoke command under `cmd/workflowcheck`. The command starts
an HTTP test server in the same process, calls the public routes, and verifies
both success and failure behavior. The seal check also races two concurrent
seal requests to prove that the repository mutation lock permits exactly one
seal.

Tests are intentionally deferred for this initialization baseline. The later
engineering-task stage will add unit and integration tests, including red/green
verification for the workflows and their failure paths.
