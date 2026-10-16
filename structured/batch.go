// Package structured provides batch processing for structured data.
package structured

import (
	"context"
	"encoding/csv"
	"io"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/anonymizer"
	"github.com/u007/dataprotect/recognizers"
)

// BatchProcessor handles tabular data anonymization.
type BatchProcessor struct {
	analyzer    *analyzer.Analyzer
	anonymizer  *anonymizer.Anonymizer
	columnTypes map[string]recognizers.EntityType
}

// NewBatch creates a new batch processor.
func NewBatch(analyzer *analyzer.Analyzer, anonymizer *anonymizer.Anonymizer) *BatchProcessor {
	return &BatchProcessor{
		analyzer:    analyzer,
		anonymizer:  anonymizer,
		columnTypes: make(map[string]recognizers.EntityType),
	}
}

// SetColumnType sets the expected entity type for a column.
func (bp *BatchProcessor) SetColumnType(column string, entityType recognizers.EntityType) {
	bp.columnTypes[column] = entityType
}

// ProcessCSV reads CSV from r, anonymizes PII, and writes the result to w.
//
// The returned count is the number of rows whose field count differed from the
// header. Such rows are padded with empty trailing cells rather than rejected,
// because ragged CSV is common in real exports and failing the whole file over
// one truncated row is worse than writing an empty cell — but the count lets a
// caller detect that it happened instead of silently trusting the output.
func (bp *BatchProcessor) ProcessCSV(ctx context.Context, r io.Reader, w io.Writer) (int, error) {
	reader := csv.NewReader(r)
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Read header
	header, err := reader.Read()
	if err != nil {
		return 0, err
	}
	if err := writer.Write(header); err != nil {
		return 0, err
	}

	scanned := bp.scannedColumns(header)

	// Ragged rows are tolerated rather than fatal, but NOT silently: a short row
	// is padded to the header width and reported through the returned count, so a
	// caller that cares can detect a truncated file. Relaxing FieldsPerRecord
	// alone would suppress csv.ErrFieldCount and make malformed input look like a
	// clean run.
	reader.FieldsPerRecord = -1

	ragged := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ragged, err
		}
		if len(record) != len(header) {
			ragged++
			// intentionally not logged: this is an in-process library with no logger.
			// The ragged count is returned to the caller, which owns the logging
			// and alerting policy; logging here would force a logging dependency on
			// every consumer and duplicate what the caller already receives.
			record = padRecord(record, len(header))
		}

		// Anonymize each configured cell
		for idx := range record {
			if !scanned[idx] {
				continue
			}
			cell := record[idx]
			entities, err := bp.analyzer.Analyze(ctx, cell)
			if err != nil {
				return ragged, err
			}
			if len(entities) > 0 {
				result, err := bp.anonymizer.Anonymize(ctx, cell, entities)
				if err != nil {
					return ragged, err
				}
				record[idx] = result
			}
		}

		if err := writer.Write(record); err != nil {
			return ragged, err
		}
	}

	return ragged, nil
}

// PseudonymizeCSV reads CSV from r, replaces each value in a configured column
// with a numbered placeholder such as [EMAIL_1], and writes the result to w. All
// cells share one mapping, so a value that recurs anywhere in the file gets the
// same placeholder. The returned map holds raw PII, so the caller must protect it.
//
// Only configured columns are scanned, as in ProcessCSV. The header and every
// other cell still pass the placeholder check, so a literal placeholder anywhere
// in the file is an error. Restoring the file would otherwise rewrite it.
//
// The file is read whole and nothing is written until the check passes, so a
// failure leaves w untouched. The returned count is the number of ragged rows,
// as in ProcessCSV.
func (bp *BatchProcessor) PseudonymizeCSV(ctx context.Context, r io.Reader, w io.Writer) (int, map[string]string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return 0, nil, err
	}
	if len(rows) == 0 {
		return 0, nil, io.EOF
	}

	header := rows[0]
	scanned := bp.scannedColumns(header)

	// Flatten every cell, header first, into one list so PseudonymizeAll can share
	// its mapping across the whole file. Only scanned cells carry entities.
	ragged := 0
	var texts []string
	var entities [][]recognizers.Entity
	for ri, row := range rows {
		if ri > 0 && len(row) != len(header) {
			ragged++
			row = padRecord(row, len(header))
		}
		for ci, cell := range row {
			var found []recognizers.Entity
			if ri > 0 && scanned[ci] {
				found, err = bp.analyzer.Analyze(ctx, cell)
				if err != nil {
					return ragged, nil, err
				}
			}
			texts = append(texts, cell)
			entities = append(entities, found)
		}
		rows[ri] = row
	}

	results, pseudonyms, err := bp.anonymizer.PseudonymizeAll(texts, entities)
	if err != nil {
		return ragged, nil, err
	}

	writer := csv.NewWriter(w)
	for ri := range rows {
		if err := writer.Write(results[ri*len(header) : (ri+1)*len(header)]); err != nil {
			return ragged, nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return ragged, nil, err
	}
	return ragged, pseudonyms, nil
}

// scannedColumns reports, for each header position, whether the column is one the
// caller configured. A name that appears more than once marks every copy, so a
// repeated column cannot pass unscanned.
func (bp *BatchProcessor) scannedColumns(header []string) []bool {
	scanned := make([]bool, len(header))
	for i, h := range header {
		_, scanned[i] = bp.columnTypes[h]
	}
	return scanned
}

// padRecord pads a short row with empty cells to width. A longer row is cut to
// width, as ProcessCSV always has.
func padRecord(record []string, width int) []string {
	padded := make([]string, width)
	copy(padded, record)
	return padded
}
