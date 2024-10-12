package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/porebric/logger"
)

const (
	directoryEnv = "JAEGER_STORAGE"
	daysOld      = 2
)

func CleanJaeger(ctx context.Context) {
	for {
		err := cleanOldFiles(ctx)
		if err != nil {
			logger.Error(ctx, err, "get old files")
		} else {
			logger.Info(ctx, "get old files")
		}

		time.Sleep(1 * time.Hour)
	}
}

func cleanOldFiles(ctx context.Context) error {
	now := time.Now()
	rootDir := os.Getenv(directoryEnv)

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error(ctx, err, "failed to access path")
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if now.Sub(info.ModTime()).Hours() > float64(daysOld*24) {
			if err := os.Remove(path); err != nil {
				logger.Error(ctx, err, "failed to delete file", "file", path)
			}
		}
		return nil
	})
}
