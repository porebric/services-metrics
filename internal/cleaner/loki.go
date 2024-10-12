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

	files, err := os.ReadDir(os.Getenv(lokiDataDir))
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(os.Getenv(lokiDataDir), file.Name())

		info, err := os.Stat(filePath)
		if err != nil {
			logger.Error(ctx, err, "failed to get file info")
			continue
		}

		if now.Sub(info.ModTime()).Hours() > float64(lokiDaysOld*24) {
			if err = os.Remove(filePath); err != nil {
				logger.Error(ctx, err, "failed to delete file")
			} else {
				logger.Info(ctx, "file deleted", "file", filePath)
			}
		}
	}

	return nil
}
