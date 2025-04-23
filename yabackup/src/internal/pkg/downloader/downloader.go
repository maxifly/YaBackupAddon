package downloader

import (
	"context"
	"fmt"
	grab "github.com/cavaliergopher/grab/v3"
	"math"
	"sync"
	"time"
	"ybg/internal/pkg/mylogger"
	om "ybg/internal/pkg/operationmanager"
)

type Downloader struct {
	operationManager *om.OperationManager
	logger           *mylogger.Logger
}

func New(operationManager *om.OperationManager, logger *mylogger.Logger) *Downloader {
	return &Downloader{
		operationManager: operationManager,
		logger:           logger,
	}
}

func (dwn *Downloader) Download(fileURL string,
	fileName string,
	id string,
	statusSuffix string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	wg.Add(1)

	go func(url, name, id string) {
		defer wg.Done()
		err := dwn.downloadInner(url, name, id, statusSuffix)
		if err != nil {
			errChan <- err
		}
	}(fileURL, fileName, id)

	// Ожидаем завершения всех загрузок
	wg.Wait()

	close(errChan)

	// Проверяем, была ли ошибка
	if err, ok := <-errChan; ok {
		return err
	}

	return nil
}

func (dwn *Downloader) downloadInner(fileURL string,
	fileName string,
	id string,
	statusSuffix string) error {
	dwn.operationManager.ChangeStatusAndProgress(id, "downloading "+statusSuffix, 0)

	// Создаем новый запрос
	client := grab.NewClient()

	req, err := grab.NewRequest(fileName, fileURL)
	if err != nil {
		dwn.logger.ErrorLog.Printf("Error when create request: %v", err)
		dwn.operationManager.UpdateOperationInfo(id, func(info *om.OperationInfo) {
			info.Status = fmt.Sprintf("Error when create request: %v", err)
			info.IsError = true
			info.IsDone = true
		})
		return err
	}

	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)

	// Выполняем запрос

	dwn.logger.DebugLog.Printf("Start download %v...\n", req.URL())
	resp := client.Do(req)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	dwn.logger.DebugLog.Printf("Start for")

	for {
		select {
		case <-ticker.C:
			progress := resp.Progress()
			dwn.logger.DebugLog.Printf("Download progress: %v", progress)
			dwn.operationManager.ChangeProgress(id, int(math.Floor(progress*100)))

		case <-resp.Done:
			if err := resp.Err(); err != nil {
				dwn.operationManager.ErrorDone(id, fmt.Sprintf("Error when download file: %v", err))
				return fmt.Errorf("error when download file: %v", err)
			} else {
				dwn.operationManager.ChangeProgress(id, 90)
			}
			dwn.logger.DebugLog.Printf("Done download")
			return nil
		}
	}

}
