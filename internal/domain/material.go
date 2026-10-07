package domain

import (
	"fmt"
	"strings"
	"time"
)

type BatchStatus string

const (
	BatchActive  BatchStatus = "active"
	BatchFailed  BatchStatus = "failed"
	BatchRetired BatchStatus = "retired"
)

// ReferenceBatch is one registered lot of a reference material or standard
// artifact. The steward records its validity window, allowed usage budget,
// and the check basis that justifies trusting it in measurements.
type ReferenceBatch struct {
	ID           string      `json:"id"`
	MaterialName string      `json:"material_name"`
	LotNumber    string      `json:"lot_number"`
	Steward      string      `json:"steward"`
	CheckBasis   string      `json:"check_basis"`
	MaxUsage     int         `json:"max_usage"`
	UsageCount   int         `json:"usage_count"`
	Status       BatchStatus `json:"status"`
	ExpiresAt    time.Time   `json:"expires_at"`
	RegisteredAt time.Time   `json:"registered_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

func NewReferenceBatch(id, materialName, lotNumber, steward, checkBasis string, maxUsage int, expiresAt, at time.Time) (ReferenceBatch, error) {
	item := ReferenceBatch{
		ID:           strings.TrimSpace(id),
		MaterialName: strings.TrimSpace(materialName),
		LotNumber:    NormalizeLotNumber(lotNumber),
		Steward:      strings.TrimSpace(steward),
		CheckBasis:   strings.TrimSpace(checkBasis),
		MaxUsage:     maxUsage,
		Status:       BatchActive,
		ExpiresAt:    expiresAt.UTC(),
		RegisteredAt: at.UTC(),
		UpdatedAt:    at.UTC(),
	}
	if err := item.Validate(); err != nil {
		return ReferenceBatch{}, err
	}
	return item, nil
}

func NormalizeLotNumber(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

// BatchKey is the deduplication identity of a batch: one lot of one material
// may only be registered once.
func BatchKey(materialName, lotNumber string) string {
	return strings.ToUpper(strings.TrimSpace(materialName)) + "|" + NormalizeLotNumber(lotNumber)
}

func (b ReferenceBatch) Validate() error {
	if b.ID == "" || b.MaterialName == "" || b.LotNumber == "" || b.Steward == "" || b.CheckBasis == "" {
		return fmt.Errorf("%w: batch id, material name, lot number, steward, and check basis are required", ErrInvalidInput)
	}
	if b.MaxUsage < 1 {
		return fmt.Errorf("%w: batch max usage must be at least 1", ErrInvalidInput)
	}
	if b.UsageCount < 0 || b.UsageCount > b.MaxUsage {
		return fmt.Errorf("%w: batch usage count must be between 0 and max usage", ErrInvalidInput)
	}
	if b.ExpiresAt.IsZero() || b.RegisteredAt.IsZero() || b.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: batch expiry and timestamps are required", ErrInvalidInput)
	}
	switch b.Status {
	case BatchActive, BatchFailed, BatchRetired:
		return nil
	default:
		return fmt.Errorf("%w: unsupported batch state %q", ErrInvalidInput, b.Status)
	}
}

func (b ReferenceBatch) RemainingUsage() int {
	return b.MaxUsage - b.UsageCount
}

// CanConsume reports whether the batch may be consumed by a measurement at
// the given time. The checks mirror the rejection order of the workflow:
// lifecycle state, validity window, then remaining usage budget.
func (b ReferenceBatch) CanConsume(quantity int, at time.Time) error {
	if quantity < 1 {
		return fmt.Errorf("%w: batch quantity must be at least 1", ErrInvalidInput)
	}
	if b.Status != BatchActive {
		return fmt.Errorf("%w: batch %s is %s", ErrBatchNotActive, b.ID, b.Status)
	}
	if !at.UTC().Before(b.ExpiresAt) {
		return fmt.Errorf("%w: batch %s expired at %s", ErrBatchExpired, b.ID, b.ExpiresAt.Format(time.RFC3339))
	}
	if b.UsageCount+quantity > b.MaxUsage {
		return fmt.Errorf("%w: batch %s has %d of %d uses left", ErrBatchUsageExceeded, b.ID, b.RemainingUsage(), b.MaxUsage)
	}
	return nil
}

// Consume validates the request and records the usage in the same step so a
// caller cannot check and then forget to account for the consumption.
func (b *ReferenceBatch) Consume(quantity int, at time.Time) error {
	if err := b.CanConsume(quantity, at); err != nil {
		return err
	}
	b.UsageCount += quantity
	b.UpdatedAt = at.UTC()
	return nil
}

func (b *ReferenceBatch) FailCheck(at time.Time) error {
	if b.Status != BatchActive {
		return fmt.Errorf("%w: only an active batch can fail a check", ErrStateConflict)
	}
	b.Status = BatchFailed
	b.UpdatedAt = at.UTC()
	return nil
}

func (b *ReferenceBatch) Reinstate(at time.Time) error {
	if b.Status != BatchFailed {
		return fmt.Errorf("%w: only a failed batch can be reinstated", ErrStateConflict)
	}
	b.Status = BatchActive
	b.UpdatedAt = at.UTC()
	return nil
}

func (b *ReferenceBatch) Retire(at time.Time) error {
	if b.Status == BatchRetired {
		return fmt.Errorf("%w: batch is already retired", ErrStateConflict)
	}
	b.Status = BatchRetired
	b.UpdatedAt = at.UTC()
	return nil
}

// BatchUsage is the immutable snapshot of the batch exactly as it was
// consumed by one measurement. Later batch state changes never rewrite it.
type BatchUsage struct {
	BatchID      string    `json:"batch_id"`
	MaterialName string    `json:"material_name"`
	LotNumber    string    `json:"lot_number"`
	CheckBasis   string    `json:"check_basis"`
	Quantity     int       `json:"quantity"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewBatchUsage(batch ReferenceBatch, quantity int) (BatchUsage, error) {
	item := BatchUsage{
		BatchID:      batch.ID,
		MaterialName: batch.MaterialName,
		LotNumber:    batch.LotNumber,
		CheckBasis:   batch.CheckBasis,
		Quantity:     quantity,
		ExpiresAt:    batch.ExpiresAt,
	}
	if err := item.Validate(); err != nil {
		return BatchUsage{}, err
	}
	return item, nil
}

func (u BatchUsage) Validate() error {
	if u.BatchID == "" || u.MaterialName == "" || u.LotNumber == "" || u.CheckBasis == "" {
		return fmt.Errorf("%w: batch usage id, material, lot, and check basis are required", ErrInvalidInput)
	}
	if u.Quantity < 1 {
		return fmt.Errorf("%w: batch usage quantity must be at least 1", ErrInvalidInput)
	}
	if u.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: batch usage expiry is required", ErrInvalidInput)
	}
	return nil
}
