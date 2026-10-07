package contracts

import (
	"time"

	"example.com/solo-0021-metrology-assurance-suite/internal/domain"
)

type RegisterDeviceRequest struct {
	AssetTag string `json:"asset_tag"`
	Model    string `json:"model"`
	Room     string `json:"room"`
	Steward  string `json:"steward"`
}

type OpenCaseRequest struct {
	DeviceID  string  `json:"device_id"`
	Nominal   float64 `json:"nominal"`
	Tolerance float64 `json:"tolerance"`
	Steward   string  `json:"steward"`
}

type RecordMeasurementRequest struct {
	Observed         float64  `json:"observed"`
	Technician       string   `json:"technician"`
	ReferenceBatchID *string  `json:"reference_batch_id,omitempty"`
	ReferenceAmount  *float64 `json:"reference_amount,omitempty"`
}

type ReopenCaseRequest struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

type SealCaseRequest struct {
	Reviewer string `json:"reviewer"`
	Summary  string `json:"summary"`
}

type RegisterReferenceBatchRequest struct {
	Name       string    `json:"name"`
	Lot        string    `json:"lot"`
	Unit       string    `json:"unit"`
	Allowance  float64   `json:"allowance"`
	ExpiresAt  time.Time `json:"expires_at"`
	CheckBasis string    `json:"check_basis"`
	Owner      string    `json:"owner"`
}

type RecordReferenceCheckRequest struct {
	Verdict string `json:"verdict"`
	Basis   string `json:"basis"`
	Actor   string `json:"actor"`
	Note    string `json:"note"`
}

type DeviceResponse struct {
	Device domain.Device `json:"device"`
}

// ReferenceBatchView is the current derived state of a reference batch plus its
// append-only verification history and audit trail.
type ReferenceBatchView struct {
	domain.ReferenceBatch
	Remaining float64                     `json:"remaining"`
	Status    domain.ReferenceBatchStatus `json:"status"`
	Checks    []domain.ReferenceCheck     `json:"checks"`
	Events    []domain.AuditEvent         `json:"events"`
}

type ReferenceBatchResponse struct {
	Batch ReferenceBatchView `json:"batch"`
}

type ReferenceBatchesResponse struct {
	Batches []ReferenceBatchView `json:"batches"`
}

type CaseResponse struct {
	Case         domain.VerificationCase `json:"case"`
	Measurements []domain.Measurement    `json:"measurements"`
	Seal         *domain.EvidenceSeal    `json:"seal,omitempty"`
	Events       []domain.AuditEvent     `json:"events"`
}

type EventsResponse struct {
	Events []domain.AuditEvent `json:"events"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
