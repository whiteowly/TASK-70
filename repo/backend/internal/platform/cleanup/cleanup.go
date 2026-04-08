// Package cleanup provides reusable functions for removing expired
// operational data. Used by the worker and integration tests.
package cleanup

import (
	"database/sql"
	"os"
)

// ExpiredSessions deletes auth sessions that are past their absolute expiry
// or have been idle for more than 8 hours.
func ExpiredSessions(db *sql.DB) (int64, error) {
	res, err := db.Exec(`DELETE FROM auth_sessions WHERE expires_at < NOW() OR last_active_at < NOW() - INTERVAL '8 hours'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ExpiredIdempotencyKeys deletes idempotency keys past their expiry window.
func ExpiredIdempotencyKeys(db *sql.DB) (int64, error) {
	res, err := db.Exec(`DELETE FROM idempotency_keys WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ExpiredEvidence deletes work order evidence rows whose retention has expired,
// and removes the associated files from disk (best effort).
func ExpiredEvidence(db *sql.DB) (int64, error) {
	rows, err := db.Query(`SELECT id, file_path FROM work_order_evidence WHERE retention_expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			continue
		}
		os.Remove(path) // best effort file deletion
		if _, err := db.Exec(`DELETE FROM work_order_evidence WHERE id = $1`, id); err == nil {
			count++
		}
	}
	return count, rows.Err()
}
