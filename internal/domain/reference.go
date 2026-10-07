package domain

import (
	"fmt"
	"strings"
	"time"
)

type ReferenceCheckVerdict string

const (
	ReferenceCheckPassed ReferenceCheckVerdict = "passed"
	ReferenceCheckFailed ReferenceCheckVerdict = "failed"
)

type ReferenceBatchStatus string

const (
	ReferenceActive            ReferenceBatchStatus = "active"
	ReferenceExpired           ReferenceBatchStatus = "expired"
	ReferenceExhausted         ReferenceBatchStatus = "exhausted"
	ReferenceStatusCheckFailed ReferenceBatchStatus = "check_failed"
)

// ReferenceBatch is one registered lot of a reference material or reference
// artifact. The batch constrains which measurements may cite it: citations are
// only valid while the batch is in date, has remaining allowance, and its most
// recent verification check passed.
type ReferenceBatch struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Lot          string    `json:"lot"`
	Unit         string    `json:"unit"`
	Allowance    float64   `json:"allowance"`
	UsedAmount   float64   `json:"used_amount"`
	ExpiresAt    time.Time `json:"expires_at"`
	CheckBasis   string    `json:"check_basis"`
	Owner        string    `json:"owner"`
	RegisteredAt time.Time `json:"registered_at"`
}

func NewReferenceBatch(id, name, lot, unit string, allowance float64, expiresAt time.Time, checkBasis, owner string, at time.Time) (ReferenceBatch, error) {
	item := ReferenceBatch{
		ID:           strings.TrimSpace(id),
		Name:         strings.TrimSpace(name),
		Lot:          NormalizeReferenceLot(lot),
		Unit:         strings.TrimSpace(unit),
		Allowance:    allowance,
		ExpiresAt:    expiresAt.UTC(),
		CheckBasis:   strings.TrimSpace(checkBasis),
		Owner:        strings.TrimSpace(owner),
		RegisteredAt: at.UTC(),
	}
	if err := item.Validate(); err != nil {
		return ReferenceBatch{}, err
	}
	return item, nil
}

func NormalizeReferenceLot(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func (b ReferenceBatch) Validate() error {
	if b.ID == "" || b.Name == "" || b.Lot == "" || b.Unit == "" || b.CheckBasis == "" || b.Owner == "" {
		return fmt.Errorf("%w: batch id, name, lot, unit, check basis, and owner are required", ErrInvalidInput)
	}
	if len(b.Lot) > 64 {
		return fmt.Errorf("%w: reference lot is too long", ErrInvalidInput)
	}
	if !isFinite(b.Allowance) || b.Allowance <= 0 {
		return fmt.Errorf("%w: allowed usage amount must be positive", ErrInvalidInput)
	}
	if !isFinite(b.UsedAmount) || b.UsedAmount < 0 || b.UsedAmount > b.Allowance {
		return fmt.Errorf("%w: used amount must stay within the allowed amount", ErrInvalidInput)
	}
	if b.ExpiresAt.IsZero() || b.RegisteredAt.IsZero() {
		return fmt.Errorf("%w: batch registration and expiry times are required", ErrInvalidInput)
	}
	return nil
}

func (b ReferenceBatch) Remaining() float64 {
	return b.Allowance - b.UsedAmount
}

func (b ReferenceBatch) IsExpired(now time.Time) bool {
	return !now.UTC().Before(b.ExpiresAt)
}

// Status derives the current batch status from its latest check verdict and the
// evaluation time. A failed check dominates date and allowance, and an expired
// batch dominates exhaustion.
func (b ReferenceBatch) Status(latest *ReferenceCheck, now time.Time) ReferenceBatchStatus {
	if latest != nil && latest.Verdict == ReferenceCheckFailed {
		return ReferenceStatusCheckFailed
	}
	if b.IsExpired(now) {
		return ReferenceExpired
	}
	if b.UsedAmount >= b.Allowance {
		return ReferenceExhausted
	}
	return ReferenceActive
}

// ValidateUse enforces the citation gates at measurement time. The latest check
// must exist and have passed, the batch must be in date, and the requested
// amount must fit within the remaining allowance.
func (b ReferenceBatch) ValidateUse(latest *ReferenceCheck, amount float64, now time.Time) error {
	if !isFinite(amount) || amount <= 0 {
		return fmt.Errorf("%w: reference usage amount must be positive", ErrInvalidInput)
	}
	if latest == nil || latest.Verdict != ReferenceCheckPassed {
		return fmt.Errorf("%w: reference batch %s has no passing verification check", ErrReferenceCheckFailed, b.ID)
	}
	if b.IsExpired(now) {
		return fmt.Errorf("%w: reference batch %s expired at %s", ErrReferenceExpired, b.ID, b.ExpiresAt.Format(time.RFC3339))
	}
	if b.UsedAmount+amount > b.Allowance {
		return fmt.Errorf("%w: reference batch %s remaining allowance %.4f is below requested %.4f", ErrReferenceExhausted, b.ID, b.Remaining(), amount)
	}
	return nil
}

// Consume records an amount of the batch as used. Callers must gate the amount
// with ValidateUse first.
func (b *ReferenceBatch) Consume(amount float64) error {
	if !isFinite(amount) || amount <= 0 {
		return fmt.Errorf("%w: reference usage amount must be positive", ErrInvalidInput)
	}
	if b.UsedAmount+amount > b.Allowance {
		return fmt.Errorf("%w: reference batch %s would exceed its allowed amount", ErrReferenceExhausted, b.ID)
	}
	b.UsedAmount += amount
	return nil
}

// ReferenceCheck is one verification result for a batch. Checks are append
// only; the most recent check is the current verdict for gating purposes.
type ReferenceCheck struct {
	BatchID   string                `json:"batch_id"`
	Verdict   ReferenceCheckVerdict `json:"verdict"`
	Basis     string                `json:"basis"`
	Actor     string                `json:"actor"`
	Note      string                `json:"note"`
	CreatedAt time.Time             `json:"created_at"`
}

func NewReferenceCheck(batchID, verdict, basis, actor, note string, at time.Time) (ReferenceCheck, error) {
	item := ReferenceCheck{
		BatchID:   strings.TrimSpace(batchID),
		Verdict:   ReferenceCheckVerdict(strings.TrimSpace(verdict)),
		Basis:     strings.TrimSpace(basis),
		Actor:     strings.TrimSpace(actor),
		Note:      strings.TrimSpace(note),
		CreatedAt: at.UTC(),
	}
	if err := item.Validate(); err != nil {
		return ReferenceCheck{}, err
	}
	return item, nil
}

func (c ReferenceCheck) Validate() error {
	if c.BatchID == "" || c.Basis == "" || c.Actor == "" {
		return fmt.Errorf("%w: check batch id, basis, and actor are required", ErrInvalidInput)
	}
	switch c.Verdict {
	case ReferenceCheckPassed, ReferenceCheckFailed:
	default:
		return fmt.Errorf("%w: unsupported check verdict %q", ErrInvalidInput, c.Verdict)
	}
	if c.CreatedAt.IsZero() {
		return fmt.Errorf("%w: check time is required", ErrInvalidInput)
	}
	return nil
}

// ReferenceCitation is the immutable evidence snapshot captured when a
// measurement actually consumes a batch. It is copied into the measurement and
// again into the evidence seal; later batch changes never rewrite it.
type ReferenceCitation struct {
	BatchID    string    `json:"batch_id"`
	BatchName  string    `json:"batch_name"`
	Lot        string    `json:"lot"`
	Unit       string    `json:"unit"`
	AmountUsed float64   `json:"amount_used"`
	Allowance  float64   `json:"allowance"`
	ExpiresAt  time.Time `json:"expires_at"`
	CheckBasis string    `json:"check_basis"`
	CheckedAt  time.Time `json:"checked_at"`
	CitedAt    time.Time `json:"cited_at"`
}

func NewReferenceCitation(batch ReferenceBatch, latest ReferenceCheck, amount float64, at time.Time) (ReferenceCitation, error) {
	item := ReferenceCitation{
		BatchID:    batch.ID,
		BatchName:  batch.Name,
		Lot:        batch.Lot,
		Unit:       batch.Unit,
		AmountUsed: amount,
		Allowance:  batch.Allowance,
		ExpiresAt:  batch.ExpiresAt,
		CheckBasis: latest.Basis,
		CheckedAt:  latest.CreatedAt,
		CitedAt:    at.UTC(),
	}
	if err := item.Validate(); err != nil {
		return ReferenceCitation{}, err
	}
	return item, nil
}

func (c ReferenceCitation) Validate() error {
	if c.BatchID == "" || c.BatchName == "" || c.Lot == "" || c.Unit == "" || c.CheckBasis == "" {
		return fmt.Errorf("%w: citation batch, name, lot, unit, and check basis are required", ErrInvalidInput)
	}
	if !isFinite(c.AmountUsed) || c.AmountUsed <= 0 || !isFinite(c.Allowance) || c.Allowance <= 0 {
		return fmt.Errorf("%w: citation amounts must be positive", ErrInvalidInput)
	}
	if c.ExpiresAt.IsZero() || c.CheckedAt.IsZero() || c.CitedAt.IsZero() {
		return fmt.Errorf("%w: citation times are required", ErrInvalidInput)
	}
	return nil
}
