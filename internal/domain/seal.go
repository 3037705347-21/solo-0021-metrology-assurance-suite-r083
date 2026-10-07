package domain

import (
	"fmt"
	"strings"
	"time"
)

type EvidenceSeal struct {
	ID            string             `json:"id"`
	CaseID        string             `json:"case_id"`
	DeviceID      string             `json:"device_id"`
	MeasurementID string             `json:"measurement_id"`
	Reviewer      string             `json:"reviewer"`
	Summary       string             `json:"summary"`
	Reference     *ReferenceCitation `json:"reference,omitempty"`
	SealedAt      time.Time          `json:"sealed_at"`
}

func NewEvidenceSeal(id, caseID, deviceID, measurementID, reviewer, summary string, reference *ReferenceCitation, at time.Time) (EvidenceSeal, error) {
	item := EvidenceSeal{
		ID:            strings.TrimSpace(id),
		CaseID:        strings.TrimSpace(caseID),
		DeviceID:      strings.TrimSpace(deviceID),
		MeasurementID: strings.TrimSpace(measurementID),
		Reviewer:      strings.TrimSpace(reviewer),
		Summary:       strings.TrimSpace(summary),
		Reference:     reference,
		SealedAt:      at.UTC(),
	}
	if err := item.Validate(); err != nil {
		return EvidenceSeal{}, err
	}
	return item, nil
}

func (s EvidenceSeal) Validate() error {
	if s.ID == "" || s.CaseID == "" || s.DeviceID == "" || s.MeasurementID == "" || s.Reviewer == "" || s.Summary == "" {
		return fmt.Errorf("%w: seal id, case, device, measurement, reviewer, and summary are required", ErrInvalidInput)
	}
	if s.Reference != nil {
		if err := s.Reference.Validate(); err != nil {
			return err
		}
	}
	if s.SealedAt.IsZero() {
		return fmt.Errorf("%w: seal time is required", ErrInvalidInput)
	}
	return nil
}
