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
go run ./cmd/workflowcheck --workflow record-measurement-with-batch
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
- `POST /batches`
- `GET /batches/{id}`
- `POST /batches/{id}/fail-check`
- `POST /batches/{id}/reinstate`
- `POST /batches/{id}/retire`

## Reference Batches

A steward registers each lot of reference material with `POST /batches`,
declaring its check basis, expiry, and allowed usage budget. A measurement
that consumes reference material cites the batch with `batch_id` and an
optional `batch_quantity` (default 1) on `POST /cases/{id}/measurements`:

```json
{
  "observed": 25.2,
  "technician": "Rui",
  "batch_id": "batch-0001",
  "batch_quantity": 1
}
```

The citation is validated and deducted in the same atomic commit as the
measurement. The request fails without recording anything when the batch is
unknown (`not_found`), exhausted (`batch_usage_exceeded`), expired
(`batch_expired`), or blocked by a failed check (`batch_not_active`). The
measurement stores an immutable snapshot of the batch as consumed, so later
batch transitions — reinstatement, retirement, or expiry — never rewrite
recorded or sealed measurement facts.

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

The service has no environment variables and no external dependencies.
