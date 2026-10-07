package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"example.com/solo-0021-metrology-assurance-suite/internal/bootstrap"
)

type apiResponse struct {
	Status int
	Body   map[string]any
	Raw    string
}

type harness struct {
	baseURL string
	client  *http.Client
	server  *httptest.Server
}

func main() {
	workflow := flag.String("workflow", "", "workflow id to verify")
	flag.Parse()

	checks := map[string]func() error{
		"register-device":          checkRegisterDevice,
		"open-verification-case":   checkOpenVerificationCase,
		"record-measurement":       checkRecordMeasurement,
		"reopen-verification-case": checkReopenVerificationCase,
		"seal-verification":        checkSealVerification,
		"register-reference-batch": checkRegisterReferenceBatch,
		"reference-verification":   checkReferenceVerification,
		"reference-measurement":    checkReferenceMeasurement,
		"seal-reference-record":    checkSealReferenceRecord,
	}
	run, ok := checks[*workflow]
	if !ok {
		fmt.Fprintf(flag.CommandLine.Output(), "unknown workflow %q\n", *workflow)
		flag.Usage()
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Printf("workflow %s: failed: %v\n", *workflow, err)
		os.Exit(1)
	}
	fmt.Printf("workflow %s: ok\n", *workflow)
}

func checkRegisterDevice() error {
	h := newHarness()
	defer h.close()

	response, err := h.request(http.MethodPost, "/devices", map[string]any{
		"asset_tag": " met-1001 ",
		"model":     "Bench Meter",
		"room":      "Room A",
		"steward":   "Lin",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return err
	}
	id, err := nestedString(response.Body, "device", "id")
	if err != nil {
		return err
	}
	tag, err := nestedString(response.Body, "device", "asset_tag")
	if err != nil {
		return err
	}
	if tag != "MET-1001" {
		return fmt.Errorf("asset tag was not normalized: %q", tag)
	}

	read, err := h.request(http.MethodGet, "/devices/"+id, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}
	readID, err := nestedString(read.Body, "device", "id")
	if err != nil {
		return err
	}
	if readID != id {
		return fmt.Errorf("read returned device %q, expected %q", readID, id)
	}

	duplicate, err := h.request(http.MethodPost, "/devices", map[string]any{
		"asset_tag": "MET-1001",
		"model":     "Other Meter",
		"room":      "Room B",
		"steward":   "Mira",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(duplicate, http.StatusConflict); err != nil {
		return err
	}
	return expectErrorCode(duplicate, "duplicate_asset_tag")
}

func checkOpenVerificationCase() error {
	h := newHarness()
	defer h.close()

	deviceID, err := h.createDevice("MET-1101")
	if err != nil {
		return err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return err
	}
	state, err := nestedString(opened.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "awaiting_measurement" {
		return fmt.Errorf("opened case state = %q", state)
	}

	read, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}

	second, err := h.request(http.MethodPost, "/cases", map[string]any{
		"device_id": deviceID,
		"nominal":   25.0,
		"tolerance": 0.5,
		"steward":   "Qiao",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(second, http.StatusConflict); err != nil {
		return err
	}
	return expectErrorCode(second, "active_case_exists")
}

func checkRecordMeasurement() error {
	h := newHarness()
	defer h.close()

	deviceID, err := h.createDevice("MET-1201")
	if err != nil {
		return err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return err
	}
	accepted, err := h.recordMeasurement(caseID, 25.2, "Rui")
	if err != nil {
		return err
	}
	if err := expectStatus(accepted, http.StatusCreated); err != nil {
		return err
	}
	state, err := nestedString(accepted.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "review_ready" {
		return fmt.Errorf("accepted measurement state = %q", state)
	}
	acceptedValue, err := nestedBool(accepted.Body, "measurements", 0, "accepted")
	if err != nil {
		return err
	}
	if !acceptedValue {
		return errors.New("in-tolerance measurement was not accepted")
	}

	second, err := h.recordMeasurement(caseID, 25.1, "Rui")
	if err != nil {
		return err
	}
	if err := expectStatus(second, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(second, "state_conflict"); err != nil {
		return err
	}

	rejectDeviceID, err := h.createDevice("MET-1202")
	if err != nil {
		return err
	}
	rejectCase, err := h.openCase(rejectDeviceID)
	if err != nil {
		return err
	}
	rejectCaseID, err := nestedString(rejectCase.Body, "case", "id")
	if err != nil {
		return err
	}
	rejected, err := h.recordMeasurement(rejectCaseID, 26.5, "Rui")
	if err != nil {
		return err
	}
	if err := expectStatus(rejected, http.StatusCreated); err != nil {
		return err
	}
	rejectedState, err := nestedString(rejected.Body, "case", "state")
	if err != nil {
		return err
	}
	if rejectedState != "recheck_required" {
		return fmt.Errorf("out-of-tolerance state = %q", rejectedState)
	}
	rejectedValue, err := nestedBool(rejected.Body, "measurements", 0, "accepted")
	if err != nil {
		return err
	}
	if rejectedValue {
		return errors.New("out-of-tolerance measurement was accepted")
	}
	return nil
}

func checkReopenVerificationCase() error {
	h := newHarness()
	defer h.close()

	caseID, err := h.rejectedCase("MET-1301")
	if err != nil {
		return err
	}
	reopened, err := h.request(http.MethodPost, "/cases/"+caseID+"/reopen", map[string]any{
		"actor":  "Lin",
		"reason": "measurement setup was replaced",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(reopened, http.StatusOK); err != nil {
		return err
	}
	state, err := nestedString(reopened.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "awaiting_measurement" {
		return fmt.Errorf("reopened case state = %q", state)
	}
	measurements, err := nestedSlice(reopened.Body, "measurements")
	if err != nil {
		return err
	}
	if len(measurements) != 1 {
		return fmt.Errorf("reopen changed measurement history: %d entries", len(measurements))
	}

	second, err := h.request(http.MethodPost, "/cases/"+caseID+"/reopen", map[string]any{
		"actor":  "Lin",
		"reason": "duplicate request",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(second, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(second, "state_conflict"); err != nil {
		return err
	}

	events, err := h.request(http.MethodGet, "/cases/"+caseID+"/events", nil)
	if err != nil {
		return err
	}
	if err := expectStatus(events, http.StatusOK); err != nil {
		return err
	}
	if !hasEvent(events.Body, "case_reopened") {
		return errors.New("reopen audit event is missing")
	}
	return nil
}

func checkSealVerification() error {
	h := newHarness()
	defer h.close()

	caseID, err := h.readyCase("MET-1401")
	if err != nil {
		return err
	}
	type outcome struct {
		response apiResponse
		err      error
	}
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			response, err := h.request(http.MethodPost, "/cases/"+caseID+"/seal", map[string]any{
				"reviewer": fmt.Sprintf("Reviewer-%d", index+1),
				"summary":  "accepted measurement sealed for review",
			})
			results <- outcome{response: response, err: err}
		}(index)
	}
	wait.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for item := range results {
		if item.err != nil {
			return item.err
		}
		switch item.response.Status {
		case http.StatusCreated:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			return fmt.Errorf("unexpected concurrent seal status %d: %s", item.response.Status, item.response.Raw)
		}
	}
	if successes != 1 || conflicts != 1 {
		return fmt.Errorf("concurrent seal outcomes: %d success, %d conflict", successes, conflicts)
	}

	read, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}
	state, err := nestedString(read.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "sealed" {
		return fmt.Errorf("sealed case state = %q", state)
	}
	sealID, err := nestedString(read.Body, "seal", "id")
	if err != nil {
		return err
	}
	if sealID == "" {
		return errors.New("evidence seal is missing")
	}
	return nil
}

func checkRegisterReferenceBatch() error {
	h := newHarness()
	defer h.close()

	response, err := h.registerReference("RM-2001", "Standard Solution", 100.0, futureDate(30), "certificate of analysis C-2001")
	if err != nil {
		return err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return err
	}
	batchID, err := nestedString(response.Body, "batch", "id")
	if err != nil {
		return err
	}
	lot, err := nestedString(response.Body, "batch", "lot")
	if err != nil {
		return err
	}
	if lot != "RM-2001" {
		return fmt.Errorf("reference lot was not normalized: %q", lot)
	}
	status, err := nestedString(response.Body, "batch", "status")
	if err != nil {
		return err
	}
	if status != "active" {
		return fmt.Errorf("fresh batch status = %q", status)
	}
	batchObject, err := nestedObject(response.Body, "batch")
	if err != nil {
		return err
	}
	checks, err := nestedSlice(batchObject, "checks")
	if err != nil {
		return err
	}
	if len(checks) != 1 {
		return fmt.Errorf("expected one initial check, got %d", len(checks))
	}

	read, err := h.request(http.MethodGet, "/reference-batches/"+batchID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}

	missing, err := h.request(http.MethodGet, "/reference-batches/reference-9999", nil)
	if err != nil {
		return err
	}
	if err := expectStatus(missing, http.StatusNotFound); err != nil {
		return err
	}
	if err := expectErrorCode(missing, "not_found"); err != nil {
		return err
	}

	duplicate, err := h.registerReference(" rm-2001 ", "Other", 5.0, futureDate(10), "other basis")
	if err != nil {
		return err
	}
	if err := expectStatus(duplicate, http.StatusConflict); err != nil {
		return err
	}
	return expectErrorCode(duplicate, "duplicate_reference_lot")
}

func checkReferenceVerification() error {
	h := newHarness()
	defer h.close()

	registered, err := h.registerReference("RM-2101", "Calibration Artifact", 50.0, futureDate(60), "artifact verification A-2101")
	if err != nil {
		return err
	}
	batchID, err := nestedString(registered.Body, "batch", "id")
	if err != nil {
		return err
	}

	failed, err := h.request(http.MethodPost, "/reference-batches/"+batchID+"/checks", map[string]any{
		"verdict": "failed",
		"basis":   "intermediate drift check D-2101",
		"actor":   "Mira",
		"note":    "observed drift beyond limit",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(failed, http.StatusCreated); err != nil {
		return err
	}
	status, err := nestedString(failed.Body, "batch", "status")
	if err != nil {
		return err
	}
	if status != "check_failed" {
		return fmt.Errorf("batch status after failed check = %q", status)
	}
	failedBatch, err := nestedObject(failed.Body, "batch")
	if err != nil {
		return err
	}
	checks, err := nestedSlice(failedBatch, "checks")
	if err != nil {
		return err
	}
	if len(checks) != 2 {
		return fmt.Errorf("expected append-only history of 2 checks, got %d", len(checks))
	}
	if !hasEvent(failedBatch, "reference_check_failed") {
		return errors.New("failed verification audit event is missing")
	}

	passed, err := h.request(http.MethodPost, "/reference-batches/"+batchID+"/checks", map[string]any{
		"verdict": "passed",
		"basis":   "re-verification after replacement",
		"actor":   "Mira",
		"note":    "drift cleared",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(passed, http.StatusCreated); err != nil {
		return err
	}
	status, err = nestedString(passed.Body, "batch", "status")
	if err != nil {
		return err
	}
	if status != "active" {
		return fmt.Errorf("batch status after restoring passed check = %q", status)
	}
	passedBatch, err := nestedObject(passed.Body, "batch")
	if err != nil {
		return err
	}
	checks, err = nestedSlice(passedBatch, "checks")
	if err != nil {
		return err
	}
	if len(checks) != 3 {
		return fmt.Errorf("expected append-only history of 3 checks, got %d", len(checks))
	}
	return nil
}

func checkReferenceMeasurement() error {
	h := newHarness()
	defer h.close()

	// Registered batch with a tight allowance of 2 units.
	registered, err := h.registerReference("RM-2201", "Consumable Standard", 2.0, futureDate(30), "certificate C-2201")
	if err != nil {
		return err
	}
	batchID, err := nestedString(registered.Body, "batch", "id")
	if err != nil {
		return err
	}

	caseID, err := h.openReadyCaseWithReference("MET-2201", batchID, 1.0, 25.2)
	if err != nil {
		return err
	}

	read, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}
	citedLot, err := nestedString(read.Body, "measurements", 0, "reference", "lot")
	if err != nil {
		return err
	}
	if citedLot != "RM-2201" {
		return fmt.Errorf("measurement cited lot = %q", citedLot)
	}
	citedAmount, err := nestedFloat(read.Body, "measurements", 0, "reference", "amount_used")
	if err != nil {
		return err
	}
	if citedAmount != 1.0 {
		return fmt.Errorf("cited amount = %v, expected 1", citedAmount)
	}
	batchRead, err := h.request(http.MethodGet, "/reference-batches/"+batchID, nil)
	if err != nil {
		return err
	}
	remaining, err := nestedFloat(batchRead.Body, "batch", "remaining")
	if err != nil {
		return err
	}
	if remaining != 1.0 {
		return fmt.Errorf("remaining allowance = %v, expected 1", remaining)
	}

	// A second batch whose allowance is fully consumed by one citation must
	// reject any further citation as exhausted.
	secondRegistered, err := h.registerReference("RM-2202", "Consumable Standard", 1.0, futureDate(30), "certificate C-2202")
	if err != nil {
		return err
	}
	secondID, err := nestedString(secondRegistered.Body, "batch", "id")
	if err != nil {
		return err
	}
	rejectDeviceID, err := h.createDevice("MET-2202")
	if err != nil {
		return err
	}
	rejectCase, err := h.openCase(rejectDeviceID)
	if err != nil {
		return err
	}
	rejectCaseID, err := nestedString(rejectCase.Body, "case", "id")
	if err != nil {
		return err
	}
	over, err := h.recordMeasurementWithReference(rejectCaseID, 25.2, "Rui", secondID, 1.5)
	if err != nil {
		return err
	}
	if err := expectStatus(over, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(over, "reference_exhausted"); err != nil {
		return err
	}
	// The rejected citation must not have consumed any allowance.
	secondRead, err := h.request(http.MethodGet, "/reference-batches/"+secondID, nil)
	if err != nil {
		return err
	}
	remaining, err = nestedFloat(secondRead.Body, "batch", "remaining")
	if err != nil {
		return err
	}
	if remaining != 1.0 {
		return fmt.Errorf("failed citation changed remaining allowance to %v", remaining)
	}
	// The rejected measurement must not exist.
	caseRead, err := h.request(http.MethodGet, "/cases/"+rejectCaseID, nil)
	if err != nil {
		return err
	}
	measurements, err := nestedSlice(caseRead.Body, "measurements")
	if err != nil {
		return err
	}
	if len(measurements) != 0 {
		return fmt.Errorf("rejected citation created %d measurements", len(measurements))
	}

	// Unknown batch id cannot form a valid citation.
	unknown, err := h.recordMeasurementWithReference(rejectCaseID, 25.2, "Rui", "reference-9999", 0.5)
	if err != nil {
		return err
	}
	if err := expectStatus(unknown, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(unknown, "reference_unknown"); err != nil {
		return err
	}

	// A failed check blocks citation even with remaining allowance.
	failedRegistered, err := h.registerReference("RM-2203", "Consumable Standard", 5.0, futureDate(30), "certificate C-2203")
	if err != nil {
		return err
	}
	failedID, err := nestedString(failedRegistered.Body, "batch", "id")
	if err != nil {
		return err
	}
	failedCheck, err := h.request(http.MethodPost, "/reference-batches/"+failedID+"/checks", map[string]any{
		"verdict": "failed",
		"basis":   "drift check",
		"actor":   "Mira",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(failedCheck, http.StatusCreated); err != nil {
		return err
	}
	failedMeasure, err := h.recordMeasurementWithReference(rejectCaseID, 25.2, "Rui", failedID, 0.5)
	if err != nil {
		return err
	}
	if err := expectStatus(failedMeasure, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(failedMeasure, "reference_check_failed"); err != nil {
		return err
	}

	// An expired batch blocks citation.
	expiredRegistered, err := h.registerReference("RM-2204", "Consumable Standard", 5.0, pastDate(1), "certificate C-2204")
	if err != nil {
		return err
	}
	expiredID, err := nestedString(expiredRegistered.Body, "batch", "id")
	if err != nil {
		return err
	}
	expiredMeasure, err := h.recordMeasurementWithReference(rejectCaseID, 25.2, "Rui", expiredID, 0.5)
	if err != nil {
		return err
	}
	if err := expectStatus(expiredMeasure, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(expiredMeasure, "reference_expired"); err != nil {
		return err
	}

	// Batch id without amount is invalid input.
	half, err := h.request(http.MethodPost, "/cases/"+rejectCaseID+"/measurements", map[string]any{
		"observed":           25.2,
		"technician":         "Rui",
		"reference_batch_id": batchID,
	})
	if err != nil {
		return err
	}
	if err := expectStatus(half, http.StatusBadRequest); err != nil {
		return err
	}
	return expectErrorCode(half, "invalid_input")
}

func checkSealReferenceRecord() error {
	h := newHarness()
	defer h.close()

	registered, err := h.registerReference("RM-2301", "Sealed Standard", 10.0, futureDate(30), "certificate C-2301")
	if err != nil {
		return err
	}
	batchID, err := nestedString(registered.Body, "batch", "id")
	if err != nil {
		return err
	}
	caseID, err := h.openReadyCaseWithReference("MET-2301", batchID, 1.5, 25.2)
	if err != nil {
		return err
	}

	sealed, err := h.request(http.MethodPost, "/cases/"+caseID+"/seal", map[string]any{
		"reviewer": "Reviewer-R",
		"summary":  "accepted measurement with reference citation sealed",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(sealed, http.StatusCreated); err != nil {
		return err
	}

	read, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}
	sealedLot, err := nestedString(read.Body, "seal", "reference", "lot")
	if err != nil {
		return err
	}
	if sealedLot != "RM-2301" {
		return fmt.Errorf("seal cited lot = %q", sealedLot)
	}
	sealedAmount, err := nestedFloat(read.Body, "seal", "reference", "amount_used")
	if err != nil {
		return err
	}
	if sealedAmount != 1.5 {
		return fmt.Errorf("seal cited amount = %v, expected 1.5", sealedAmount)
	}

	// After sealing, change the batch's state: mark its check failed. The sealed
	// measurement and seal must keep their captured snapshot unchanged.
	failed, err := h.request(http.MethodPost, "/reference-batches/"+batchID+"/checks", map[string]any{
		"verdict": "failed",
		"basis":   "post-seal drift check",
		"actor":   "Mira",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(failed, http.StatusCreated); err != nil {
		return err
	}

	again, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(again, http.StatusOK); err != nil {
		return err
	}
	againLot, err := nestedString(again.Body, "seal", "reference", "lot")
	if err != nil {
		return err
	}
	againAmount, err := nestedFloat(again.Body, "seal", "reference", "amount_used")
	if err != nil {
		return err
	}
	if againLot != sealedLot || againAmount != sealedAmount {
		return fmt.Errorf("sealed citation changed after batch state change: %q %v", againLot, againAmount)
	}
	measurementAmount, err := nestedFloat(again.Body, "measurements", 0, "reference", "amount_used")
	if err != nil {
		return err
	}
	if measurementAmount != 1.5 {
		return fmt.Errorf("measurement citation changed after sealing: %v", measurementAmount)
	}
	batchStatus, err := nestedString(failed.Body, "batch", "status")
	if err != nil {
		return err
	}
	if batchStatus != "check_failed" {
		return fmt.Errorf("current batch status = %q, expected check_failed", batchStatus)
	}
	return nil
}

func newHarness() *harness {
	server := httptest.NewServer(bootstrap.NewHandler())
	return &harness{
		baseURL: server.URL,
		client:  server.Client(),
		server:  server,
	}
}

func (h *harness) close() {
	h.server.Close()
}

func (h *harness) request(method, path string, body any) (apiResponse, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return apiResponse{}, err
		}
		reader = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, reader)
	if err != nil {
		return apiResponse{}, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := h.client.Do(request)
	if err != nil {
		return apiResponse{}, err
	}
	defer response.Body.Close()
	rawBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return apiResponse{}, err
	}
	result := apiResponse{Status: response.StatusCode, Raw: string(rawBytes)}
	if len(bytes.TrimSpace(rawBytes)) > 0 {
		if err := json.Unmarshal(rawBytes, &result.Body); err != nil {
			return apiResponse{}, fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
	}
	return result, nil
}

func (h *harness) createDevice(tag string) (string, error) {
	response, err := h.request(http.MethodPost, "/devices", map[string]any{
		"asset_tag": tag,
		"model":     "Bench Meter",
		"room":      "Room A",
		"steward":   "Lin",
	})
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return nestedString(response.Body, "device", "id")
}

func (h *harness) openCase(deviceID string) (apiResponse, error) {
	response, err := h.request(http.MethodPost, "/cases", map[string]any{
		"device_id": deviceID,
		"nominal":   25.0,
		"tolerance": 0.5,
		"steward":   "Qiao",
	})
	if err != nil {
		return apiResponse{}, err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return apiResponse{}, err
	}
	return response, nil
}

func (h *harness) recordMeasurement(caseID string, observed float64, technician string) (apiResponse, error) {
	return h.request(http.MethodPost, "/cases/"+caseID+"/measurements", map[string]any{
		"observed":   observed,
		"technician": technician,
	})
}

func (h *harness) registerReference(lot, name string, allowance float64, expiresAt time.Time, basis string) (apiResponse, error) {
	return h.request(http.MethodPost, "/reference-batches", map[string]any{
		"name":        name,
		"lot":         lot,
		"unit":        "ml",
		"allowance":   allowance,
		"expires_at":  expiresAt.UTC().Format(time.RFC3339),
		"check_basis": basis,
		"owner":       "Lin",
	})
}

func (h *harness) recordMeasurementWithReference(caseID string, observed float64, technician, batchID string, amount float64) (apiResponse, error) {
	return h.request(http.MethodPost, "/cases/"+caseID+"/measurements", map[string]any{
		"observed":           observed,
		"technician":         technician,
		"reference_batch_id": batchID,
		"reference_amount":   amount,
	})
}

// openReadyCaseWithReference opens a case, records one accepted measurement that
// consumes the given batch, and returns the case id.
func (h *harness) openReadyCaseWithReference(deviceTag, batchID string, amount, observed float64) (string, error) {
	deviceID, err := h.createDevice(deviceTag)
	if err != nil {
		return "", err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return "", err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return "", err
	}
	response, err := h.recordMeasurementWithReference(caseID, observed, "Rui", batchID, amount)
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return caseID, nil
}

func futureDate(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, days)
}

func pastDate(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, -days)
}

func (h *harness) rejectedCase(tag string) (string, error) {
	deviceID, err := h.createDevice(tag)
	if err != nil {
		return "", err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return "", err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return "", err
	}
	response, err := h.recordMeasurement(caseID, 26.5, "Rui")
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return caseID, nil
}

func (h *harness) readyCase(tag string) (string, error) {
	deviceID, err := h.createDevice(tag)
	if err != nil {
		return "", err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return "", err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return "", err
	}
	response, err := h.recordMeasurement(caseID, 25.2, "Rui")
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return caseID, nil
}

func expectStatus(response apiResponse, expected int) error {
	if response.Status != expected {
		return fmt.Errorf("status = %d, expected %d: %s", response.Status, expected, response.Raw)
	}
	return nil
}

func expectErrorCode(response apiResponse, expected string) error {
	code, err := nestedString(response.Body, "error", "code")
	if err != nil {
		return err
	}
	if code != expected {
		return fmt.Errorf("error code = %q, expected %q", code, expected)
	}
	return nil
}

func nestedString(root map[string]any, path ...any) (string, error) {
	value, err := nestedAny(root, path...)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%v is not a string", path)
	}
	return text, nil
}

func nestedBool(root map[string]any, path ...any) (bool, error) {
	value, err := nestedAny(root, path...)
	if err != nil {
		return false, err
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%v is not a boolean", path)
	}
	return flag, nil
}

func nestedFloat(root map[string]any, path ...any) (float64, error) {
	value, err := nestedAny(root, path...)
	if err != nil {
		return 0, err
	}
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("%v is not a number", path)
	}
	return number, nil
}

func nestedSlice(root map[string]any, key string) ([]any, error) {
	value, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("missing %s", key)
	}
	if value == nil {
		// Empty slices serialize to null.
		return []any{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an array", key)
	}
	return items, nil
}

func nestedObject(root map[string]any, key string) (map[string]any, error) {
	value, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("missing %s", key)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", key)
	}
	return object, nil
}

func hasEvent(root map[string]any, kind string) bool {
	items, err := nestedSlice(root, "events")
	if err != nil {
		return false
	}
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["kind"] == kind {
			return true
		}
	}
	return false
}

func nestedAny(root map[string]any, path ...any) (any, error) {
	var current any = root
	for _, part := range path {
		switch typed := part.(type) {
		case string:
			object, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s is not an object", typed)
			}
			value, ok := object[typed]
			if !ok {
				return nil, fmt.Errorf("missing %s", typed)
			}
			current = value
		case int:
			items, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("index %d is not inside an array", typed)
			}
			if typed < 0 || typed >= len(items) {
				return nil, fmt.Errorf("index %d is out of range", typed)
			}
			current = items[typed]
		default:
			return nil, fmt.Errorf("unsupported path element %T", part)
		}
	}
	return current, nil
}
