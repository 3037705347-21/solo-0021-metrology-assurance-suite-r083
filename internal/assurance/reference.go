package assurance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"example.com/solo-0021-metrology-assurance-suite/internal/contracts"
	"example.com/solo-0021-metrology-assurance-suite/internal/domain"
	"example.com/solo-0021-metrology-assurance-suite/internal/repository"
)

// RegisterReferenceBatch enrolls a new reference material lot with its expiry,
// total allowed usage amount, and verification basis. Registration records an
// initial passing check so that a freshly registered lot is immediately
// usable.
func (s *Service) RegisterReferenceBatch(ctx context.Context, input contracts.RegisterReferenceBatchRequest) (contracts.ReferenceBatchView, error) {
	var result contracts.ReferenceBatchView
	err := s.repository.Mutate(ctx, func(m *repository.Mutation) error {
		lot := domain.NormalizeReferenceLot(input.Lot)
		if _, exists := m.ReferenceBatchByLot(lot); exists {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateReference, lot)
		}
		now := s.now()
		batch, err := domain.NewReferenceBatch(
			m.NextID("reference"),
			input.Name,
			input.Lot,
			input.Unit,
			input.Allowance,
			input.ExpiresAt,
			input.CheckBasis,
			input.Owner,
			now,
		)
		if err != nil {
			return err
		}
		if err := m.PutReferenceBatch(batch); err != nil {
			return err
		}
		check, err := domain.NewReferenceCheck(
			batch.ID,
			string(domain.ReferenceCheckPassed),
			batch.CheckBasis,
			batch.Owner,
			"initial verification recorded at registration",
			now,
		)
		if err != nil {
			return err
		}
		m.AppendReferenceCheck(check)
		event, err := domain.NewAuditEvent(
			m.NextID("event"),
			"reference_batch",
			batch.ID,
			"",
			"reference_batch_registered",
			batch.Owner,
			fmt.Sprintf("reference batch %s (%s) registered with %.4f %s allowance", batch.Name, batch.Lot, batch.Allowance, batch.Unit),
			now,
		)
		if err != nil {
			return err
		}
		m.AppendEvent(event)
		result = referenceBatchView(m, batch, now)
		return nil
	})
	return result, err
}

// ReferenceBatch returns the current derived state of one batch.
func (s *Service) ReferenceBatch(ctx context.Context, id string) (contracts.ReferenceBatchView, error) {
	var result contracts.ReferenceBatchView
	err := s.repository.Read(ctx, func(snapshot repository.Snapshot) error {
		batch, exists := snapshot.ReferenceBatch(strings.TrimSpace(id))
		if !exists {
			return fmt.Errorf("%w: reference batch %s", domain.ErrNotFound, id)
		}
		result = referenceBatchViewFromSnapshot(snapshot, batch, s.now())
		return nil
	})
	return result, err
}

// ReferenceBatches lists every registered batch with its current status.
func (s *Service) ReferenceBatches(ctx context.Context) ([]contracts.ReferenceBatchView, error) {
	var result []contracts.ReferenceBatchView
	err := s.repository.Read(ctx, func(snapshot repository.Snapshot) error {
		now := s.now()
		for _, batch := range snapshot.AllReferenceBatches() {
			result = append(result, referenceBatchViewFromSnapshot(snapshot, batch, now))
		}
		return nil
	})
	return result, err
}

// RecordReferenceCheck appends a verification result to a batch. A failed
// check immediately blocks further citations; a later passing check restores a
// batch that is still in date and has remaining allowance.
func (s *Service) RecordReferenceCheck(ctx context.Context, batchID string, input contracts.RecordReferenceCheckRequest) (contracts.ReferenceBatchView, error) {
	var result contracts.ReferenceBatchView
	err := s.repository.Mutate(ctx, func(m *repository.Mutation) error {
		id := strings.TrimSpace(batchID)
		batch, exists := m.ReferenceBatch(id)
		if !exists {
			return fmt.Errorf("%w: reference batch %s", domain.ErrNotFound, id)
		}
		now := s.now()
		check, err := domain.NewReferenceCheck(
			batch.ID,
			input.Verdict,
			input.Basis,
			input.Actor,
			input.Note,
			now,
		)
		if err != nil {
			return err
		}
		m.AppendReferenceCheck(check)
		kind := "reference_check_passed"
		detail := fmt.Sprintf("reference batch %s passed verification against %s", batch.Lot, check.Basis)
		if check.Verdict == domain.ReferenceCheckFailed {
			kind = "reference_check_failed"
			detail = fmt.Sprintf("reference batch %s failed verification against %s", batch.Lot, check.Basis)
		}
		event, err := domain.NewAuditEvent(
			m.NextID("event"),
			"reference_batch",
			batch.ID,
			"",
			kind,
			check.Actor,
			detail,
			now,
		)
		if err != nil {
			return err
		}
		m.AppendEvent(event)
		result = referenceBatchView(m, batch, now)
		return nil
	})
	return result, err
}

func latestCheck(checks []domain.ReferenceCheck) *domain.ReferenceCheck {
	if len(checks) == 0 {
		return nil
	}
	latest := checks[len(checks)-1]
	return &latest
}

func referenceBatchView(m *repository.Mutation, batch domain.ReferenceBatch, now time.Time) contracts.ReferenceBatchView {
	checks := m.ReferenceChecks(batch.ID)
	return contracts.ReferenceBatchView{
		ReferenceBatch: batch,
		Remaining:      batch.Remaining(),
		Status:         batch.Status(latestCheck(checks), now),
		Checks:         checks,
		Events:         m.SubjectEvents("reference_batch", batch.ID),
	}
}

func referenceBatchViewFromSnapshot(snapshot repository.Snapshot, batch domain.ReferenceBatch, now time.Time) contracts.ReferenceBatchView {
	checks := snapshot.ReferenceChecks(batch.ID)
	return contracts.ReferenceBatchView{
		ReferenceBatch: batch,
		Remaining:      batch.Remaining(),
		Status:         batch.Status(latestCheck(checks), now),
		Checks:         checks,
		Events:         snapshot.SubjectEvents("reference_batch", batch.ID),
	}
}
