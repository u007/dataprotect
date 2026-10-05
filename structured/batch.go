// Package structured provides batch processing for structured data.
package structured

import (
	"context"
	"encoding/csv"
	"io"

	"dataprotect/analyzer"
	"dataprotect/anonymizer"
	"dataprotect/recognizers"
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

	// Map column names to indices
	colIndex := make(map[string]int)
	for i, h := range header {
		colIndex[h] = i
	}

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
			padded := make([]string, len(header))
			copy(padded, record)
			record = padded
		}

		// Anonymize each cell
		for col := range bp.columnTypes {
			idx, ok := colIndex[col]
			if !ok || idx >= len(record) {
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
