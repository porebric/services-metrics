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

	files, err := os.ReadDir(os.Getenv(directoryEnv))
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(os.Getenv(directoryEnv), file.Name())

		info, err := os.Stat(filePath)
		if err != nil {
			logger.Error(ctx, err, "get file info")
			continue
		}

		if now.Sub(info.ModTime()).Hours() > float64(daysOld*24) {
			err := os.Remove(filePath)
			if err != nil {
				logger.Error(ctx, err, "delete file")
			} else {
				logger.Info(ctx, "delete file")
			}
		}
	}

	return nil
}
