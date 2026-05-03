package activation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func CopyPath(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("copy %s to %s: %w", source, target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", target, err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".loki-copy-*")
	if err != nil {
		return fmt.Errorf("create temp path for %s: %w", target, err)
	}
	defer os.RemoveAll(tmp)
	tmpTarget := filepath.Join(tmp, "payload")
	if err := copyPathContents(source, tmpTarget, info); err != nil {
		return err
	}
	if err := replacePath(tmpTarget, target); err != nil {
		return err
	}
	return nil
}

func copyPathContents(source, target string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(source)
		if err != nil {
			return fmt.Errorf("read symlink %s: %w", source, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", target, err)
		}
		if err := os.Symlink(link, target); err != nil {
			return fmt.Errorf("copy symlink %s to %s: %w", source, target, err)
		}
		return nil
	}
	if info.IsDir() {
		return copyDir(source, target, info.Mode())
	}
	return copyFile(source, target, info.Mode())
}

func copyFile(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", target, err)
	}
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", target, err)
	}
	return nil
}

func copyDir(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(target, mode.Perm()); err != nil {
		return fmt.Errorf("create directory %s: %w", target, err)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return copyPathContents(path, dest, info)
		}
		if entry.IsDir() {
			return os.MkdirAll(dest, info.Mode().Perm())
		}
		return copyFile(path, dest, info.Mode())
	})
}

func removeExisting(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat existing %s: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove existing %s: %w", path, err)
	}
	return nil
}

func replacePath(staged, target string) error {
	if _, err := os.Lstat(staged); err != nil {
		return fmt.Errorf("replace %s: staged path invalid: %w", target, err)
	}
	var backupRoot, backupPath string
	backedUp := false
	if _, err := os.Lstat(target); err == nil {
		var mkErr error
		backupRoot, mkErr = os.MkdirTemp(filepath.Dir(target), ".loki-replace-*")
		if mkErr != nil {
			return fmt.Errorf("create replacement backup for %s: %w", target, mkErr)
		}
		backupPath = filepath.Join(backupRoot, "backup")
		if err := os.Rename(target, backupPath); err != nil {
			_ = os.RemoveAll(backupRoot)
			return fmt.Errorf("backup existing %s: %w", target, err)
		}
		backedUp = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat existing %s: %w", target, err)
	}
	if err := os.Rename(staged, target); err != nil {
		if backedUp {
			if restoreErr := os.Rename(backupPath, target); restoreErr != nil {
				return fmt.Errorf("replace %s: %w; restore backup %s failed: %v; backup preserved under %s for manual recovery", target, err, backupPath, restoreErr, backupRoot)
			}
			_ = os.RemoveAll(backupRoot)
		}
		return fmt.Errorf("replace %s: %w", target, err)
	}
	if backedUp {
		_ = os.RemoveAll(backupRoot)
	}
	return nil
}

func HashPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	h := sha256.New()
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("hash symlink %s: %w", path, err)
		}
		_, _ = io.WriteString(h, "symlink\x00"+link)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	if info.IsDir() {
		err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if current == path {
				return nil
			}
			rel, err := filepath.Rel(path, current)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				link, err := os.Readlink(current)
				if err != nil {
					return err
				}
				_, _ = io.WriteString(h, "symlink\x00"+rel+"\x00"+link+"\n")
				return nil
			}
			if entry.IsDir() {
				_, _ = io.WriteString(h, "dir\x00"+rel+"\n")
				return nil
			}
			fileHash, err := hashFile(current)
			if err != nil {
				return err
			}
			_, _ = io.WriteString(h, strings.Join([]string{"file", rel, info.Mode().Perm().String(), fileHash}, "\x00")+"\n")
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("hash directory %s: %w", path, err)
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	return hashFile(path)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func HashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
