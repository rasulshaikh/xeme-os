// Package csvkit provides CSV reading/writing utilities for Xeme OS.
package csvkit

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
)

// Read reads a CSV file and returns rows as map[string]string keyed by column.
func Read(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // allow variable
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	var rows []map[string]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(rec) {
				row[col] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Write writes rows to a CSV file. Header is inferred from the first row's keys.
func Write(path string, rows []map[string]string) error {
	if len(rows) == 0 {
		return fmt.Errorf("no rows to write")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header from first row keys (stable order)
	header := make([]string, 0, len(rows[0]))
	seen := make(map[string]bool)
	for k := range rows[0] {
		if !seen[k] {
			header = append(header, k)
			seen[k] = true
		}
	}
	if err := w.Write(header); err != nil {
		return err
	}

	// Rows
	for _, row := range rows {
		rec := make([]string, len(header))
		for i, k := range header {
			rec[i] = row[k]
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

// Count returns the number of data rows (excluding header) in a CSV.
func Count(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	n := 0
	if _, err := r.Read(); err != nil { // skip header
		return 0, err
	}
	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}
