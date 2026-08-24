package backup

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadInventory(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 24, 12, 0, 0, 123456789, time.UTC)
	older := now.Add(-90 * time.Second)
	newer := now.Add(-10 * time.Second)
	writeInventoryBackup(t, root, "z-old", older, Manifest{
		ID: "z-old", CreatedAt: older, RootDir: filepath.Join(root, "z-old"),
		Source: BackupSourceSync, Entries: []ManifestEntry{{OriginalPath: "/a", Existed: true}, {OriginalPath: "/b", Existed: false}},
	}, map[string]string{"plain.txt": "legacy"})
	writeInventoryBackup(t, root, "a-new", newer, Manifest{
		ID: "a-new", CreatedAt: newer, RootDir: filepath.Join(root, "a-new"),
		Description: "更新 ✨", Pinned: true, Entries: []ManifestEntry{{OriginalPath: "/c", Existed: true}},
	}, map[string]string{"snapshot.tar.gz": "archive", "nested/data": "more"})
	inv, err := LoadInventory(root, now)
	if err != nil {
		t.Fatalf("LoadInventory() error = %v", err)
	}
	if inv.Count != 2 || len(inv.Backups) != 2 {
		t.Fatalf("inventory count = %d/%d, want 2/2", inv.Count, len(inv.Backups))
	}
	if got := []string{inv.Backups[0].ID, inv.Backups[1].ID}; !reflect.DeepEqual(got, []string{"a-new", "z-old"}) {
		t.Fatalf("IDs = %v, want newest-first order", got)
	}
	newRecord := inv.Backups[0]
	if newRecord.Timestamp != newer.Format(time.RFC3339Nano) || newRecord.Reason != "更新 ✨" || newRecord.FileCount != 1 || newRecord.AgeSeconds != 10 || !newRecord.Pinned {
		t.Errorf("new record = %+v, want normalized metadata", newRecord)
	}
	oldRecord := inv.Backups[1]
	if oldRecord.Reason != "sync" || oldRecord.FileCount != 1 || oldRecord.AgeSeconds != 90 {
		t.Errorf("old record = %+v, want source fallback and logical count", oldRecord)
	}
	if newRecord.SizeBytes <= int64(len("archive"))+int64(len("more")) || oldRecord.SizeBytes <= int64(len("legacy")) {
		t.Errorf("record sizes must include manifest and payload files: new=%d old=%d", newRecord.SizeBytes, oldRecord.SizeBytes)
	}
	if inv.TotalSizeBytes != newRecord.SizeBytes+oldRecord.SizeBytes || inv.TotalSizeBytes <= 0 {
		t.Errorf("total size = %d, records = %d + %d", inv.TotalSizeBytes, newRecord.SizeBytes, oldRecord.SizeBytes)
	}
}
func TestLoadInventoryOrdersEqualTimestampsByIDAndClampsFutureAge(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"b", "a"} {
		writeInventoryBackup(t, root, id, when, Manifest{ID: id, CreatedAt: when, RootDir: filepath.Join(root, id)}, nil)
	}
	future := when.Add(time.Hour)
	writeInventoryBackup(t, root, "future", future, Manifest{ID: "future", CreatedAt: future, RootDir: filepath.Join(root, "future")}, nil)
	inv, err := LoadInventory(root, when)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{inv.Backups[0].ID, inv.Backups[1].ID, inv.Backups[2].ID}
	if !reflect.DeepEqual(got, []string{"future", "a", "b"}) {
		t.Fatalf("IDs = %v, want timestamp then ID ordering", got)
	}
	if inv.Backups[0].AgeSeconds != 0 {
		t.Errorf("future age = %d, want clamped zero", inv.Backups[0].AgeSeconds)
	}
}
func TestLoadInventoryMissingRootIsEmpty(t *testing.T) {
	inv, err := LoadInventory(filepath.Join(t.TempDir(), "missing"), time.Now())
	if err != nil {
		t.Fatalf("missing root error = %v", err)
	}
	if inv.Count != 0 || len(inv.Backups) != 0 || inv.TotalSizeBytes != 0 {
		t.Fatalf("missing root inventory = %+v, want empty", inv)
	}
}
func TestLoadInventoryRejectsMalformedAndUnsafeBackupsWithoutPartialResult(t *testing.T) {
	root := t.TempDir()
	writeInventoryBackup(t, root, "valid", time.Now().UTC(), Manifest{ID: "valid", CreatedAt: time.Now().UTC(), RootDir: filepath.Join(root, "valid")}, nil)
	bad := filepath.Join(root, "bad")
	mustInventory(t, os.Mkdir(bad, 0o755))
	mustInventory(t, os.WriteFile(filepath.Join(bad, ManifestFilename), []byte("{"), 0o644))
	inv, err := LoadInventory(root, time.Now())
	if err == nil || inv.Count != 0 || len(inv.Backups) != 0 {
		t.Fatalf("malformed result = %+v, error = %v; want typed failure and no partial inventory", inv, err)
	}
	wantInventoryError(t, err, ErrorMalformed)
}
func TestLoadInventoryRejectsMissingManifestAsMalformed(t *testing.T) {
	root := t.TempDir()
	mustInventory(t, os.Mkdir(filepath.Join(root, "missing-manifest"), 0o755))
	_, err := LoadInventory(root, time.Now())
	wantInventoryError(t, err, ErrorMalformed)
}

func TestLoadInventoryRejectsRootMismatchAndDirectChildSymlink(t *testing.T) {
	root := t.TempDir()
	id := "mismatch"
	dir := filepath.Join(root, id)
	mustInventory(t, os.Mkdir(dir, 0o755))
	writeManifestFile(t, dir, Manifest{ID: id, CreatedAt: time.Now().UTC(), RootDir: filepath.Join(root, "elsewhere")})
	_, err := LoadInventory(root, time.Now())
	wantInventoryError(t, err, ErrorMalformed)
	root = t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err = LoadInventory(root, time.Now())
	wantInventoryError(t, err, ErrorUnsafeTarget)
}
func wantInventoryError(t *testing.T, err error, kind ErrorKind) {
	var opErr *OperationError
	if !errors.As(err, &opErr) || opErr.Kind != kind {
		t.Fatalf("error = %v, want %s", err, kind)
	}
}
func writeInventoryBackup(t *testing.T, root, id string, created time.Time, manifest Manifest, files map[string]string) {
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		mustInventory(t, os.MkdirAll(filepath.Dir(path), 0o755))
		mustInventory(t, os.WriteFile(path, []byte(content), 0o644))
	}
	manifest.ID = id
	manifest.CreatedAt = created
	manifest.RootDir = dir
	writeManifestFile(t, dir, manifest)
}
func writeManifestFile(t *testing.T, dir string, manifest Manifest) {
	mustInventory(t, WriteManifest(filepath.Join(dir, ManifestFilename), manifest))
}
func mustInventory(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
