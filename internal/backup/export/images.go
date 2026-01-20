package export

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/listenupapp/listenup-server/internal/domain"
)

func (e *Exporter) exportImages(ctx context.Context, zw *zip.Writer) (int, error) {
	count := 0

	// Export book covers
	for book, err := range e.store.StreamBooks(ctx) {
		if err != nil {
			return count, err
		}
		if ctx.Err() != nil {
			return count, ctx.Err()
		}

		if book.CoverImage == nil || book.CoverImage.Path == "" {
			continue
		}

		ext := filepath.Ext(book.CoverImage.Path)
		archivePath := fmt.Sprintf("images/covers/%s%s", book.ID, ext)

		if err := e.copyFileToZip(zw, book.CoverImage.Path, archivePath); err != nil {
			// Log but don't fail - images are optional
			continue
		}
		count++
	}

	// Export user avatars from profiles
	for profile, err := range e.store.StreamProfiles(ctx) {
		if err != nil {
			return count, err
		}
		if ctx.Err() != nil {
			return count, ctx.Err()
		}

		if profile.AvatarType != domain.AvatarTypeImage || profile.AvatarValue == "" {
			continue
		}

		ext := filepath.Ext(profile.AvatarValue)
		if ext == "" {
			ext = ".png"
		}
		archivePath := fmt.Sprintf("images/avatars/%s%s", profile.UserID, ext)

		// Avatar path might be relative to data dir
		avatarPath := profile.AvatarValue
		if !filepath.IsAbs(avatarPath) {
			avatarPath = filepath.Join(e.dataDir, avatarPath)
		}

		if err := e.copyFileToZip(zw, avatarPath, archivePath); err != nil {
			continue
		}
		count++
	}

	return count, nil
}

func (e *Exporter) copyFileToZip(zw *zip.Writer, srcPath, archivePath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := zw.Create(archivePath)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	return err
}
