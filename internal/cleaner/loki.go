package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/porebric/logger"
)

const (
	lokiDataDir = "LOKI_DATA_DIR" // путь к директории с логами Loki (устанавливай через переменные окружения)
	lokiDaysOld = 14              // срок жизни логов
)

func CleanLoki(ctx context.Context) {
	for {
		err := cleanOldFilesLoki(ctx)
		if err != nil {
			logger.Error(ctx, err, "failed to clean old Loki logs")
		} else {
			logger.Info(ctx, "Loki logs cleanup completed successfully")
		}

		// Запуск очистки раз в час
		time.Sleep(1 * time.Hour)
	}
}

func cleanOldFilesLoki(ctx context.Context) error {
	now := time.Now()
	rootDir := os.Getenv(lokiDataDir)

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error(ctx, err, "failed to access path")
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if now.Sub(info.ModTime()).Hours() > float64(lokiDaysOld*24) {
			if err := os.Remove(path); err != nil {
				logger.Error(ctx, err, "failed to delete file", "file", path)
			}
		}
		return nil
	})
}
