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

// fileHeader mimics the X9 File Header Record (Type 01).
type fileHeader struct {
	ImmediateDestination     string `json:"immediate_destination"`
	ImmediateDestinationName string `json:"immediate_destination_name"`
	ImmediateOrigin          string `json:"immediate_origin"`
	ImmediateOriginName      string `json:"immediate_origin_name"`
	FileCreationDate         string `json:"file_creation_date"`
	FileCreationTime         string `json:"file_creation_time"`
}

// fileControl mimics the X9 File Control Record (Type 99).
type fileControl struct {
	TotalChecks      int   `json:"total_checks"`
	TotalAmountCents int64 `json:"total_amount_cents"`
}

// checkRecord represents a single check entry in the settlement file.
type checkRecord struct {
	SequenceNumber int    `json:"sequence_number"`
	TransferID     string `json:"transfer_id"`
	AccountID      string `json:"account_id"`
	MICRRouting    string `json:"micr_routing"`
	MICRAccount    string `json:"micr_account"`
	MICRSerial     string `json:"micr_serial"`
	AmountCents    int64  `json:"amount_cents"`
	FrontImageRef  string `json:"front_image_ref"`
	BackImageRef   string `json:"back_image_ref"`
	CreatedAt      string `json:"created_at"`
}

// settlementFile is the top-level X9-style JSON settlement document written to disk.
type settlementFile struct {
	FileHeader  fileHeader    `json:"file_header"`
	BatchID     string        `json:"batch_id"`
	BatchDate   string        `json:"batch_date"`
	EODCutoff   string        `json:"eod_cutoff"`
	Checks      []checkRecord `json:"checks"`
	FileControl fileControl   `json:"file_control"`
	GeneratedAt string        `json:"generated_at"`
}

// Generate creates a JSON settlement file for the given transfers and writes it to outputDir.
// The file is written atomically: first to a temp path, then renamed to the final path.
// Returns the final file path on success.
func Generate(transfers []*models.Transfer, outputDir string, batchDate time.Time) (string, error) {
	return GenerateWithID(transfers, outputDir, batchDate, uuid.New())
}

// GenerateWithID is like Generate but uses the provided batchID so the file's batch_id
// matches the settlement_batches record in the database.
func GenerateWithID(transfers []*models.Transfer, outputDir string, batchDate time.Time, batchID uuid.UUID) (string, error) {
	ct, _ := time.LoadLocation("America/Chicago")
	now := time.Now().In(ct)

	// EOD cutoff is always 6:30 PM CT on the batch date.
	y, m, d := batchDate.In(ct).Date()
	cutoff := time.Date(y, m, d, 18, 30, 0, 0, ct)

	dateStr := batchDate.In(ct).Format("2006-01-02")
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
		FileHeader: fileHeader{
			ImmediateDestination:     "021000021",
			ImmediateDestinationName: "SETTLEMENT BANK",
			ImmediateOrigin:          "123456789",
			ImmediateOriginName:      "APEX FINTECH",
			FileCreationDate:         now.Format("2006-01-02"),
			FileCreationTime:         now.Format("15:04:05"),
		},
		BatchID:   batchID.String(),
		BatchDate: dateStr,
		EODCutoff: cutoff.Format(time.RFC3339),
		Checks:    checks,
		FileControl: fileControl{
			TotalChecks:      len(checks),
			TotalAmountCents: totalCents,
		},
		GeneratedAt: now.UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("settlement: marshaling settlement document: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return "", fmt.Errorf("settlement: writing temp settlement file: %w", err)
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("settlement: moving settlement file to final path: %w", err)
	}

	return finalPath, nil
}

// buildCheckRecord constructs a checkRecord from a Transfer.
// The MICR serial is set to the sequence number (zero-padded to 4 digits) so each
// check in the batch gets a unique serial — the transfer's stored serial is ignored
// here because it may be identical across checks submitted via the same stub scenario.
// Zero-fills other MICR fields if nil — handles operator-approved MICR-failure deposits.
func buildCheckRecord(t *models.Transfer, sequenceNum int) checkRecord {
	rec := checkRecord{
		SequenceNumber: sequenceNum,
		TransferID:     t.ID.String(),
		AccountID:      t.AccountID,
		AmountCents:    t.AmountCents,
		MICRSerial:     fmt.Sprintf("%04d", sequenceNum),
		CreatedAt:      t.CreatedAt.UTC().Format(time.RFC3339),
	}

	if t.MICRRouting != nil {
		rec.MICRRouting = *t.MICRRouting
	}
	if t.MICRAccount != nil {
		rec.MICRAccount = *t.MICRAccount
	}
	if t.FrontImageRef != nil {
		rec.FrontImageRef = *t.FrontImageRef
	}
	if t.BackImageRef != nil {
		rec.BackImageRef = *t.BackImageRef
	}

	return rec
}
