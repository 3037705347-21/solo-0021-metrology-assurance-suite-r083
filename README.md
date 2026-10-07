# Metrology Assurance Suite

Metrology Assurance Suite is a Go HTTP service for bounded laboratory
measurement assurance work. It registers devices, opens verification cases,
records measurements, handles failed tolerance decisions, and seals traceable
evidence records.

## Run

```bash
go run ./cmd/metrology-service --addr 127.0.0.1:18121
```

The service listens on `127.0.0.1:18121` by default.

## Build And Verify

```bash
go build ./...
go run ./cmd/workflowcheck --workflow register-device
go run ./cmd/workflowcheck --workflow open-verification-case
go run ./cmd/workflowcheck --workflow record-measurement
go run ./cmd/workflowcheck --workflow reopen-verification-case
go run ./cmd/workflowcheck --workflow seal-verification
go run ./cmd/workflowcheck --workflow register-reference-batch
go run ./cmd/workflowcheck --workflow reference-verification
go run ./cmd/workflowcheck --workflow reference-measurement
go run ./cmd/workflowcheck --workflow seal-reference-record
```

The workflow runner starts a fresh in-process HTTP service and exercises the
public API. It is a production smoke command, not a unit test.

Tests are intentionally deferred for this initialization baseline. The later
engineering-task stage will add the unit and integration test suite and replace
or extend these smoke checks with red/green verification tests.

## API

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

## Layout

- `cmd/metrology-service`: executable HTTP server.
- `cmd/workflowcheck`: bounded workflow verification commands.
- `internal/httpapi`: route and JSON boundary.
- `internal/assurance`: workflow orchestration.
- `internal/domain`: state rules and invariants.
- `internal/repository`: copy-on-write in-memory persistence.
- `internal/contracts`: API request and response models.
- `internal/bootstrap`: concrete dependency wiring.

## Inputs And Outputs

Inputs are JSON HTTP requests. Outputs are JSON response envelopes with either a
resource view or a stable error object:

```json
{
  "error": {
    "code": "state_conflict",
    "message": "case is not awaiting measurement"
  }
}
```

## Reference Material Batches

Measurements that consume a reference material or reference artifact cite a
registered batch instead of a free-text name. A steward registers a batch with
its lot, expiry, total allowed usage amount, unit, and verification basis
(`POST /reference-batches`); registration records an initial passing check.

A measurement passes `reference_batch_id` and `reference_amount`. The service
gates the citation atomically before recording anything:

- the batch id must be registered (`reference_unknown` otherwise),
- the latest verification check must exist and have passed
  (`reference_check_failed`),
- the batch must be in date at use time (`reference_expired`),
- the amount must fit within the remaining allowance (`reference_exhausted`).

A valid citation consumes the amount and snapshots the batch id, name, lot,
unit, amount, total allowance, expiry, check basis, and check time onto the
measurement. The accepted measurement's snapshot is copied into the evidence
seal at sealing time. These snapshots are facts at the moment of use: expiring a
batch, exhausting it, or recording a later failed check changes the batch's
current status but never rewrites a recorded measurement or a sealed record.

Verification checks are append-only (`POST /reference-batches/{id}/checks`);
the most recent check is the current verdict, so a later passing check can
restore an in-date batch that still has allowance.

The service has no environment variables and no external dependencies.
