package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorIO           ErrorKind = "io"
	ErrorMalformed    ErrorKind = "malformed-metadata"
	ErrorUnsafeTarget ErrorKind = "unsafe-target"
)

type OperationError struct {
	Kind         ErrorKind
	Op, ID, Path string
	Err          error
}

func (e *OperationError) Error() string {
	where := e.Path
	if e.ID != "" {
		where = e.ID + " (" + where + ")"
	}
	if where == "" {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Op, e.Err)
	}
	return fmt.Sprintf("%s: %s %s: %v", e.Kind, e.Op, where, e.Err)
}
func (e *OperationError) Unwrap() error { return e.Err }
func operationError(kind ErrorKind, op, id, path string, err error) error {
	return &OperationError{Kind: kind, Op: op, ID: id, Path: path, Err: err}
}

type Record struct {
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"`
	Reason         string `json:"reason"`
	FileCount      int    `json:"file_count"`
	SizeBytes      int64  `json:"size_bytes"`
	AgeSeconds     int64  `json:"age_seconds"`
	Pinned         bool   `json:"pinned"`
	createdAt      time.Time
	directory      string
	manifestDigest string
}

// Inventory is the canonical successful inventory and JSON envelope.
type Inventory struct {
	Backups        []Record `json:"backups"`
	Count          int      `json:"count"`
	TotalSizeBytes int64    `json:"total_size_bytes"`
	canonicalRoot  string
	evaluatedAt    time.Time
}

func RootDir(homeDir string) string { return filepath.Join(homeDir, ".gentle-ai", "backups") }

func LoadInventory(root string, evaluatedAt time.Time) (Inventory, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Inventory{}, operationError(ErrorIO, "resolve backup root", "", root, err)
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyInventory(evaluatedAt), nil
	}
	if err != nil {
		return Inventory{}, operationError(ErrorIO, "stat backup root", "", absolute, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		absolute, err = filepath.EvalSymlinks(absolute)
		if err != nil {
			return Inventory{}, operationError(ErrorUnsafeTarget, "resolve backup root", "", root, err)
		}
		info, err = os.Stat(absolute)
		if err != nil {
			return Inventory{}, operationError(ErrorIO, "stat backup root", "", absolute, err)
		}
	}
	if !info.IsDir() {
		return Inventory{}, operationError(ErrorMalformed, "validate backup root", "", absolute, errors.New("backup root is not a directory"))
	}
	canonicalRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Inventory{}, operationError(ErrorIO, "canonicalize backup root", "", absolute, err)
	}
	entries, err := os.ReadDir(canonicalRoot)
	if err != nil {
		return Inventory{}, operationError(ErrorIO, "read backup root", "", canonicalRoot, err)
	}
	inv := Inventory{Backups: make([]Record, 0, len(entries)), canonicalRoot: canonicalRoot, evaluatedAt: evaluatedAt}
	for _, entry := range entries {
		candidate := filepath.Join(canonicalRoot, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return Inventory{}, operationError(ErrorUnsafeTarget, "inspect backup", entry.Name(), candidate, errors.New("backup entry is a symlink"))
		}
		if !entry.IsDir() {
			continue
		}
		record, size, err := loadRecord(candidate, entry.Name(), canonicalRoot, evaluatedAt)
		if err != nil {
			return Inventory{}, err
		}
		if inv.TotalSizeBytes > math.MaxInt64-size {
			return Inventory{}, operationError(ErrorIO, "sum backup sizes", record.ID, candidate, errors.New("backup size overflows int64"))
		}
		record.SizeBytes = size
		inv.TotalSizeBytes += size
		inv.Backups = append(inv.Backups, record)
	}
	sort.Slice(inv.Backups, func(i, j int) bool {
		if inv.Backups[i].createdAt.Equal(inv.Backups[j].createdAt) {
			return inv.Backups[i].ID < inv.Backups[j].ID
		}
		return inv.Backups[i].createdAt.After(inv.Backups[j].createdAt)
	})
	inv.Count = len(inv.Backups)
	return inv, nil
}

func emptyInventory(evaluatedAt time.Time) Inventory {
	return Inventory{Backups: []Record{}, evaluatedAt: evaluatedAt}
}

func loadRecord(directory, name, canonicalRoot string, evaluatedAt time.Time) (Record, int64, error) {
	manifestPath := filepath.Join(directory, ManifestFilename)
	manifestBytes, err := readManifest(manifestPath)
	if err != nil {
		return Record{}, 0, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Record{}, 0, operationError(ErrorMalformed, "decode manifest", name, manifestPath, err)
	}
	if err := validateManifest(manifest, name, directory, canonicalRoot); err != nil {
		return Record{}, 0, err
	}
	logicalCount := 0
	for _, entry := range manifest.Entries {
		if entry.Existed {
			logicalCount++
		}
	}
	if manifest.FileCount < 0 || manifest.FileCount > 0 && manifest.FileCount != logicalCount {
		return Record{}, 0, operationError(ErrorMalformed, "validate manifest", name, manifestPath, errors.New("file count contradicts manifest entries"))
	}
	size, err := apparentSize(directory, name)
	if err != nil {
		return Record{}, 0, err
	}
	digest := sha256.Sum256(manifestBytes)
	age := int64(0)
	if evaluatedAt.After(manifest.CreatedAt) {
		age = int64(evaluatedAt.Sub(manifest.CreatedAt) / time.Second)
	}
	reason := strings.TrimSpace(manifest.Description)
	if reason == "" {
		reason = manifest.Source.Label()
	}
	return Record{ID: name, Timestamp: manifest.CreatedAt.UTC().Format(time.RFC3339Nano), Reason: reason,
		FileCount: logicalCount, AgeSeconds: age, Pinned: manifest.Pinned, createdAt: manifest.CreatedAt,
		directory: directory, manifestDigest: hex.EncodeToString(digest[:])}, size, nil
}

func validateManifest(manifest Manifest, name, directory, canonicalRoot string) error {
	path := filepath.Join(directory, ManifestFilename)
	if manifest.ID == "" || manifest.ID != name || manifest.CreatedAt.IsZero() || manifest.RootDir == "" {
		return operationError(ErrorMalformed, "validate manifest", name, path, errors.New("required backup metadata is missing or inconsistent"))
	}
	root, err := filepath.Abs(manifest.RootDir)
	if err != nil {
		return operationError(ErrorMalformed, "resolve manifest root", name, path, err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(directory) || filepath.Dir(resolved) != filepath.Clean(canonicalRoot) {
		if err == nil {
			err = errors.New("manifest root does not identify its backup directory")
		}
		return operationError(ErrorMalformed, "validate manifest root", name, path, err)
	}
	return nil
}

func readManifest(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		kind := ErrorIO
		if errors.Is(err, fs.ErrNotExist) {
			kind = ErrorMalformed
		}
		return nil, operationError(kind, "read manifest", "", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, operationError(ErrorUnsafeTarget, "read manifest", "", path, errors.New("manifest is not a regular file"))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, operationError(ErrorIO, "read manifest", "", path, err)
	}
	return content, nil
}

func apparentSize(directory, id string) (int64, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, operationError(ErrorIO, "size backup", id, directory, err)
	}
	var total int64
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return 0, operationError(ErrorIO, "size backup", id, path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return 0, operationError(ErrorUnsafeTarget, "size backup", id, path, errors.New("backup contains a symlink"))
		}
		var size int64
		if info.IsDir() {
			size, err = apparentSize(path, id)
		} else if info.Mode().IsRegular() {
			size = info.Size()
		} else {
			err = errors.New("backup contains a special file")
		}
		if err != nil {
			if opErr, ok := err.(*OperationError); ok {
				return 0, opErr
			}
			return 0, operationError(ErrorUnsafeTarget, "size backup", id, path, err)
		}
		if size < 0 || total > math.MaxInt64-size {
			return 0, operationError(ErrorIO, "sum backup size", id, path, errors.New("backup size overflows int64"))
		}
		total += size
	}
	return total, nil
}
