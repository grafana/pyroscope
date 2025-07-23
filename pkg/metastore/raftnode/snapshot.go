package raftnode

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-kit/log/level"
)

// importSnapshots moves snapshots from SnapshotsImportDir to SnapshotsDir.
//
// When initializing the snapshot store, Raft creates a "snapshots" subdirectory.
// To maintain consistency, this behavior is replicated here – so "snapshots" should
// not be included in the configured path.
//
// Note: the import process does not guarantee atomicity, as it may involve moving
// files across different file systems. If an error occurs during the import,
// both the source and destination directories may be left in an inconsistent state.
// The function does not attempt to recover from such a state, but it will try to
// continue copying on the next call.
func (n *Node) importSnapshots() error {
	if n.config.SnapshotsImportDir == "" {
		return nil
	}

	importDir := filepath.Join(n.config.SnapshotsImportDir, "snapshots")
	snapshotsDir := filepath.Join(n.config.SnapshotsDir, "snapshots")

	level.Info(n.logger).Log("msg", "importing snapshots")
	entries, err := fs.ReadDir(os.DirFS(importDir), ".")
	if err != nil {
		return fmt.Errorf("failed to read dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dstDir := filepath.Join(snapshotsDir, entry.Name())
		srcDir := filepath.Join(importDir, entry.Name())
		if err = n.copySnapshot(srcDir, dstDir); err != nil {
			return fmt.Errorf("failed to import snapshot %q: %w", entry.Name(), err)
		}
	}

	return nil
}

func (n *Node) copySnapshot(srcDir, dstDir string) error {
	entries, err := fs.ReadDir(os.DirFS(srcDir), ".")
	if err != nil {
		return fmt.Errorf("failed to read snapshot dir: %w", err)
	}
	var hasFiles bool
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		hasFiles = true
		break
	}
	if !hasFiles {
		return nil
	}

	level.Info(n.logger).Log("msg", "importing snapshot", "snapshot", srcDir)
	if err = os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if err = copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy snapshot file %q: %w", entry.Name(), err)
		}
		if err = os.Remove(srcPath); err != nil {
			level.Warn(n.logger).Log("msg", "failed to remove source file after copy", "file", srcPath, "err", err)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		_ = sourceFile.Close()
	}()
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}
	destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	if _, err = io.Copy(destFile, sourceFile); err != nil {
		_ = destFile.Close()
		return fmt.Errorf("failed to copy file contents: %w", err)
	}
	return destFile.Close()
}
