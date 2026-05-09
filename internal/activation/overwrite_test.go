package activation

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/db"
)

func TestClassifyTargetSafetyStates(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()

	missing := Operation{Type: OperationCopy, TargetPath: filepath.Join(root, "missing.txt")}
	safety, err := ClassifyTarget(ctx, database, missing)
	if err != nil {
		t.Fatalf("ClassifyTarget(missing) error = %v", err)
	}
	if safety.Class != SafetyMissing || !safety.Safe {
		t.Fatalf("missing safety = %+v", safety)
	}

	unmanagedFile := filepath.Join(root, "unmanaged.txt")
	writeFile(t, unmanagedFile, "local")
	safety, err = ClassifyTarget(ctx, database, Operation{Type: OperationCopy, TargetPath: unmanagedFile})
	if err != nil {
		t.Fatalf("ClassifyTarget(unmanaged file) error = %v", err)
	}
	if safety.Class != SafetyUnmanagedFile || safety.Safe {
		t.Fatalf("unmanaged file safety = %+v", safety)
	}

	unmanagedDir := filepath.Join(root, "unmanaged-dir")
	if err := os.MkdirAll(unmanagedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	safety, err = ClassifyTarget(ctx, database, Operation{Type: OperationCopy, TargetPath: unmanagedDir})
	if err != nil {
		t.Fatalf("ClassifyTarget(unmanaged dir) error = %v", err)
	}
	if safety.Class != SafetyUnmanagedDirectory || safety.Safe {
		t.Fatalf("unmanaged dir safety = %+v", safety)
	}

	managed := filepath.Join(root, "managed.txt")
	writeFile(t, managed, "managed")
	hash, err := HashPath(managed)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{Type: OperationCopy, TargetPath: managed, SourcePath: filepath.Join(root, "source.txt"), LayerName: "work", LayerKind: "core"}
	if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	safety, err = ClassifyTarget(ctx, database, op)
	if err != nil {
		t.Fatalf("ClassifyTarget(managed) error = %v", err)
	}
	if safety.Class != SafetyManagedFileHash || !safety.Safe {
		t.Fatalf("managed safety = %+v", safety)
	}
	writeFile(t, managed, "changed")
	safety, err = ClassifyTarget(ctx, database, op)
	if err != nil {
		t.Fatalf("ClassifyTarget(mismatch) error = %v", err)
	}
	if safety.Class != SafetyManagedHashMismatch || safety.Safe {
		t.Fatalf("mismatch safety = %+v", safety)
	}
}

func TestListManagedTargetsSorted(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	first := filepath.Join(root, "b.txt")
	second := filepath.Join(root, "a.txt")
	for _, path := range []string{first, second} {
		writeFile(t, path, path)
		hash, err := HashPath(path)
		if err != nil {
			t.Fatal(err)
		}
		op := Operation{Type: OperationCopy, TargetPath: path, SourcePath: path + ".source", LayerName: "work", LayerKind: "core"}
		if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
			t.Fatalf("UpsertManagedTarget() error = %v", err)
		}
	}
	records, err := ListManagedTargets(ctx, database)
	if err != nil {
		t.Fatalf("ListManagedTargets() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v", records)
	}
	if records[0].TargetPath != second || records[1].TargetPath != first {
		t.Fatalf("records not sorted by target path: %+v", records)
	}
	if records[0].Mode != string(OperationCopy) || records[0].LayerName != "work" || records[0].LayerKind != "core" || records[0].SourcePath == "" {
		t.Fatalf("record fields = %+v", records[0])
	}
}

func TestClassifyTargetBrokenSymlinkBlocks(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	link := filepath.Join(t.TempDir(), "broken")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	safety, err := ClassifyTarget(ctx, database, Operation{Type: OperationSymlink, TargetPath: link, SourcePath: filepath.Join(t.TempDir(), "source")})
	if err != nil {
		t.Fatalf("ClassifyTarget() error = %v", err)
	}
	if safety.Class != SafetyBrokenSymlink || safety.Safe {
		t.Fatalf("broken symlink safety = %+v", safety)
	}
}

func TestClassifyTargetSymlinkRequiresSymlinkRecord(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	link := filepath.Join(root, "target.txt")
	writeFile(t, source, "hello")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	copyRecord := Operation{Type: OperationCopy, SourcePath: source, TargetPath: link, LayerName: "work", LayerKind: "core"}
	if err := UpsertManagedTarget(ctx, database, copyRecord, "hash", time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget(copy) error = %v", err)
	}
	safety, err := ClassifyTarget(ctx, database, Operation{Type: OperationSymlink, SourcePath: source, TargetPath: link})
	if err != nil {
		t.Fatalf("ClassifyTarget(copy record) error = %v", err)
	}
	if safety.Safe {
		t.Fatalf("copy-backed symlink safety = %+v, want unsafe", safety)
	}
	symlinkRecord := Operation{Type: OperationSymlink, SourcePath: source, TargetPath: link, LayerName: "work", LayerKind: "core"}
	if err := UpsertManagedTarget(ctx, database, symlinkRecord, "hash", time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget(symlink) error = %v", err)
	}
	safety, err = ClassifyTarget(ctx, database, symlinkRecord)
	if err != nil {
		t.Fatalf("ClassifyTarget(symlink record) error = %v", err)
	}
	if !safety.Safe || safety.Class != SafetyManagedSymlink {
		t.Fatalf("symlink safety = %+v", safety)
	}
}

func activationDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Bootstrap(context.Background(), filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	return database
}
