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
	Observed      float64 `json:"observed"`
	Technician    string  `json:"technician"`
	BatchID       string  `json:"batch_id,omitempty"`
	BatchQuantity int     `json:"batch_quantity,omitempty"`
}

type ReopenCaseRequest struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

type SealCaseRequest struct {
	Reviewer string `json:"reviewer"`
	Summary  string `json:"summary"`
}

type RegisterBatchRequest struct {
	MaterialName string    `json:"material_name"`
	LotNumber    string    `json:"lot_number"`
	Steward      string    `json:"steward"`
	CheckBasis   string    `json:"check_basis"`
	MaxUsage     int       `json:"max_usage"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type BatchTransitionRequest struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

type DeviceResponse struct {
	Device domain.Device `json:"device"`
}

type CaseResponse struct {
	Case         domain.VerificationCase `json:"case"`
	Measurements []domain.Measurement    `json:"measurements"`
	Seal         *domain.EvidenceSeal    `json:"seal,omitempty"`
	Events       []domain.AuditEvent     `json:"events"`
}

type BatchResponse struct {
	Batch  domain.ReferenceBatch `json:"batch"`
	Events []domain.AuditEvent   `json:"events"`
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
