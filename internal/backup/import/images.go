package backupimport

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (i *Importer) importImages(ctx context.Context, zr *zip.ReadCloser) (int, error) {
	count := 0

	for _, f := range zr.File {
		if ctx.Err() != nil {
			return count, ctx.Err()
		}

		// Only process files in images/ directory
		if !strings.HasPrefix(f.Name, "images/") {
			continue
		}

		// Determine destination path
		var destPath string
		if after, ok := strings.CutPrefix(f.Name, "images/covers/"); ok {
			// images/covers/{book_id}.ext -> {dataDir}/covers/{book_id}.ext
			relPath := after
			destPath = filepath.Join(i.dataDir, "covers", relPath)
		} else if after, ok := strings.CutPrefix(f.Name, "images/avatars/"); ok {
			// images/avatars/{user_id}.ext -> {dataDir}/avatars/{user_id}.ext
			relPath := after
			destPath = filepath.Join(i.dataDir, "avatars", relPath)
		} else {
			continue
		}

		if err := i.extractFile(f, destPath); err != nil {
			i.logger.Warn("failed to extract image",
				"path", f.Name,
				"dest", destPath,
				"error", err)
			continue
		}
		count++
	}

	return count, nil
}

func (i *Importer) extractFile(f *zip.File, destPath string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, rc)
	return err
}
