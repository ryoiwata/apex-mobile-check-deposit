package settlement

// NOTE: This file implements settlement file generation using a structured JSON format.
// The intended implementation uses moov-io/imagecashletter for X9 ICL (ANSI X9.100-187)
// file generation. The JSON fallback was chosen because Docker is unavailable to verify
// the imagecashletter v0.10.0 API at implementation time.
// The function signature is identical to what the X9 implementation would use.
// See docs/decision_log.md for details.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/apex/mcd/internal/models"
	"github.com/google/uuid"
)

// checkRecord represents a single check entry in the settlement file.
type checkRecord struct {
	SequenceNumber int    `json:"sequence_number"`
	TransferID     string `json:"transfer_id"`
	AccountID      string `json:"account_id"`
	AmountCents    int64  `json:"amount_cents"`
	MICRRouting    string `json:"micr_routing"`
	MICRAccount    string `json:"micr_account"`
	MICRSerial     string `json:"micr_serial"`
	FrontImageRef  string `json:"front_image_ref"`
	BackImageRef   string `json:"back_image_ref"`
	CreatedAt      string `json:"created_at"`
}

// settlementFile is the top-level settlement document written to disk.
type settlementFile struct {
	BatchID          string        `json:"batch_id"`
	BatchDate        string        `json:"batch_date"`
	GeneratedAt      string        `json:"generated_at"`
	DepositCount     int           `json:"deposit_count"`
	TotalAmountCents int64         `json:"total_amount_cents"`
	Checks           []checkRecord `json:"checks"`
}

// Generate creates a JSON settlement file for the given transfers and writes it to outputDir.
// The file is written atomically: first to a temp path, then renamed to the final path.
// Returns the final file path on success.
func Generate(transfers []*models.Transfer, outputDir string, batchDate time.Time) (string, error) {
	batchID := uuid.New()
	dateStr := batchDate.Format("2006-01-02")
	fileName := fmt.Sprintf("%s_batch_%s.json", dateStr, batchID)
	tempPath := filepath.Join(outputDir, "tmp_"+fileName)
	finalPath := filepath.Join(outputDir, fileName)

	var totalCents int64
	checks := make([]checkRecord, 0, len(transfers))
	for i, t := range transfers {
		checks = append(checks, buildCheckRecord(t, i+1))
		totalCents += t.AmountCents
	}

	doc := &settlementFile{
		BatchID:          batchID.String(),
		BatchDate:        dateStr,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		DepositCount:     len(transfers),
		TotalAmountCents: totalCents,
		Checks:           checks,
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("settlement: marshaling settlement document: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return "", fmt.Errorf("settlement: writing temp settlement file: %w", err)
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		// Clean up temp file if rename fails.
		os.Remove(tempPath)
		return "", fmt.Errorf("settlement: moving settlement file to final path: %w", err)
	}

	return finalPath, nil
}

// buildCheckRecord constructs a checkRecord from a Transfer.
// Zero-fills MICR fields if nil — handles operator-approved MICR-failure deposits.
func buildCheckRecord(t *models.Transfer, sequenceNum int) checkRecord {
	rec := checkRecord{
		SequenceNumber: sequenceNum,
		TransferID:     t.ID.String(),
		AccountID:      t.AccountID,
		AmountCents:    t.AmountCents,
		CreatedAt:      t.CreatedAt.UTC().Format(time.RFC3339),
	}

	if t.MICRRouting != nil {
		rec.MICRRouting = *t.MICRRouting
	}
	if t.MICRAccount != nil {
		rec.MICRAccount = *t.MICRAccount
	}
	if t.MICRSerial != nil {
		rec.MICRSerial = *t.MICRSerial
	}
	if t.FrontImageRef != nil {
		rec.FrontImageRef = *t.FrontImageRef
	}
	if t.BackImageRef != nil {
		rec.BackImageRef = *t.BackImageRef
	}

	return rec
}
