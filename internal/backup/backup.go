package backup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Writer struct {
	dir string
}

func NewWriter(dir string) *Writer {
	return &Writer{dir: strings.TrimSpace(dir)}
}

func (w *Writer) WriteDatabase(ctx context.Context, db *sql.DB, backupAt time.Time) (string, error) {
	if w == nil {
		return "", fmt.Errorf("backup writer is nil")
	}
	if w.dir == "" {
		return "", fmt.Errorf("backup directory is required")
	}
	if db == nil {
		return "", fmt.Errorf("database is required")
	}

	stamp := backupAt.In(time.Local)
	if stamp.IsZero() {
		stamp = time.Now().In(time.Local)
	}
	dayDir := filepath.Join(w.dir, stamp.Format("2006-01-02"))
	if err := os.MkdirAll(dayDir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	if err := os.Chmod(dayDir, 0o700); err != nil {
		return "", fmt.Errorf("restrict backup directory permissions: %w", err)
	}

	fileName := fmt.Sprintf("database_%s.db", stamp.Format("20060102T150405.000000000"))
	fullPath := filepath.Join(dayDir, fileName)
	tempPath := fullPath + ".tmp"
	_ = os.Remove(tempPath)
	if err := copySQLiteDatabase(ctx, db, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("restrict backup file permissions: %w", err)
	}
	if err := os.Rename(tempPath, fullPath); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("finalize backup file: %w", err)
	}
	return fullPath, nil
}

// copySQLiteDatabase uses one consistent VACUUM snapshot without a CGO backup handle.
func copySQLiteDatabase(ctx context.Context, sourceDB *sql.DB, destPath string) (copyErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	defer func() {
		// SQLite reports interrupted operations with a driver error; retain the
		// caller's cancellation identity without hiding that underlying error.
		if copyErr != nil && ctx.Err() != nil {
			copyErr = errors.Join(copyErr, ctx.Err())
		}
	}()
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing backup database: %w", err)
	}
	conn, err := sourceDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open source database connection: %w", err)
	}
	defer conn.Close()
	var queryOnly int
	if err := conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil {
		return fmt.Errorf("read backup source protection: %w", err)
	}
	if queryOnly != 0 {
		// VACUUM writes its attached destination. The source remains mode=ro;
		// restore this connection-local guard before returning it to the reader pool.
		defer func() {
			if _, err := conn.ExecContext(context.Background(), "PRAGMA query_only=ON"); err != nil {
				copyErr = errors.Join(copyErr, fmt.Errorf("restore backup source protection: %w", err))
				// Never return an unprotected connection to the pool after failed cleanup.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}()
		if _, err := conn.ExecContext(ctx, "PRAGMA query_only=OFF"); err != nil {
			return fmt.Errorf("prepare backup destination writes: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("copy sqlite backup: %w", err)
	}
	return nil
}

func (w *Writer) Cleanup(retentionDays int, now time.Time) (int, error) {
	if w == nil {
		return 0, fmt.Errorf("backup writer is nil")
	}
	return Cleanup(w.dir, retentionDays, now)
}

func Cleanup(dir string, retentionDays int, now time.Time) (int, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" || retentionDays <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read backup directory: %w", err)
	}

	localNow := now.In(time.Local)
	localDayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	keepStart := localDayStart.AddDate(0, 0, -(retentionDays - 1))
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		backupDay, err := time.ParseInLocation("2006-01-02", entry.Name(), time.Local)
		if err != nil {
			continue
		}
		if backupDay.Before(keepStart) {
			if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				return removed, fmt.Errorf("remove expired backup directory %s: %w", entry.Name(), err)
			}
			removed++
		}
	}

	return removed, nil
}

func ListFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".db") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
