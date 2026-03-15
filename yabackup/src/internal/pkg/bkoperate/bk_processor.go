package bkoperate

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	om "ybg/internal/pkg/operationmanager"
	"ybg/internal/pkg/utils"
	"ybg/internal/pkg/yadiskoperate"
	"ybg/internal/types"
)

const BACKUP_PATH = "/backup"

type Statistic struct {
	YaDisk         types.StorageStatistic
	LocalStorage   types.StorageStatistic
	NetworkStorage map[string]types.StorageStatistic
}

type HaStatistic struct {
	LocalStorage   types.StorageStatistic
	NetworkStorage map[string]types.StorageStatistic
}

type BkProcessor struct {
	YaDProcessor                   *yadiskoperate.YaDProcessor
	haApi                          *haoperate.HaApiClient
	operationManager               *om.OperationManager
	remoteMaximumFilesQuantity     int
	enabledNetworkStorages         map[string]struct{}
	enableUploadFromNetworkStorage bool
	statisticMu                    sync.RWMutex
	statistic                      Statistic
	isStatisticValid               bool
	logger                         *mylogger.Logger
	pollInterval                   time.Duration
	checkJobTimeout                time.Duration
	waitCreateBackupInterval       time.Duration
	waitCreateBackupTimeout        time.Duration
	deleteFilePattern              string
	maxLocalFileAmount             int
	applCtx                        context.Context
}

func NewBkProcessor(applCtx context.Context,
	yaDP *yadiskoperate.YaDProcessor, haApi *haoperate.HaApiClient, operationManager *om.OperationManager,
	remoteMaximumFilesQuantity int, enableUploadFromNetworkStorage bool,
	enabledNetworkStorages []string,
	maxLocalFileAmount int,
	logger *mylogger.Logger) *BkProcessor {

	m := make(map[string]struct{})
	for _, element := range enabledNetworkStorages {
		m[strings.TrimSpace(element)] = struct{}{}
	}
	return &BkProcessor{
		YaDProcessor:                      yaDP,
		haApi:                             haApi,
		operationManager:                  operationManager,
		remoteMaximumFilesQuantity:        remoteMaximumFilesQuantity,
		enableUploadFromNetworkStorage:    enableUploadFromNetworkStorage,
		enabledNetworkStorages:            m,
		logger:                            logger,
		isStatisticValid:                  false,
		pollInterval:                      time.Minute,
		checkJobTimeout:                   30 * time.Minute,
		waitCreateBackupInterval:          time.Minute,
		waitCreateBackupTimeout:           30 * time.Minute,
		deleteFilePattern:                 "Y_Backup",
		maxLocalFileAmount:                maxLocalFileAmount,
		applCtx:                           applCtx,
	}
}

func (bkp *BkProcessor) DeleteOldLocalFiles() error {
	files, err := bkp.GetOldLocalFiles(bkp.deleteFilePattern, bkp.maxLocalFileAmount)
	if err != nil {
		return err
	}

	bkp.logger.InfoLog.Printf("Old %d local files found", len(files))

	if len(files) == 0 {
		return nil
	}

	for _, file := range files {
		slug := file.BackupSlug
		fileName := file.BackupName
		bkp.logger.DebugLog.Printf("Deleting old file [slug: %s, name: %s]", slug, fileName)
		err = bkp.haApi.DeleteBackup(slug)

		if err != nil {
			bkp.logger.ErrorLog.Printf("Error when delete file [slug: %s, name: %s] %v", slug, fileName, err)
		} else {
			bkp.logger.InfoLog.Printf("Deleted old file [slug: %s, name: %s]", slug, fileName)
		}
	}

	return nil
}

func (bkp *BkProcessor) GetOldLocalFiles(nameMaskPattern string, maxFileAmount int) ([]types.LocalBackupFileInfo, error) {
	result := make([]types.LocalBackupFileInfo, 0)

	files, err := bkp.GetLocalFiles(nameMaskPattern)
	if err != nil {
		return result, err
	}

	if len(files) <= maxFileAmount {
		bkp.logger.InfoLog.Printf("files amount %d (max amount: %d)", len(files), maxFileAmount)
		return result, nil
	}

	// Сортируем по убыванию
	sort.Slice(files, func(i, j int) bool {
		return files[i].GeneralInfo.Created.After(files[j].GeneralInfo.Created)
	})

	// Берём всё, кроме первых N элементов
	result = append(result, files[maxFileAmount:]...)
	bkp.logger.DebugLog.Printf("Old %d local files in %d found [maxAmount: %d]", len(result), len(files), maxFileAmount)
	return result, nil

}

func (bkp *BkProcessor) GetLocalFiles(nameMaskPattern string) ([]types.LocalBackupFileInfo, error) {
	result := make([]types.LocalBackupFileInfo, 0)

	re, err := regexp.Compile(nameMaskPattern)
	if err != nil {
		return result, err
	}

	files, err := getLocalBackupFiles(bkp.haApi, bkp.logger)
	if err != nil {
		bkp.logger.ErrorLog.Printf("error get local files: %s", err)
		return result, err
	}

	for _, file := range files {
		if file.IsLocal && re.MatchString(file.BackupName) {
			result = append(result, file)
		}
	}

	return result, nil

}

func (bkp *BkProcessor) GetFilesInfo() ([]types.BackupFileInfo, error) {
	bkp.logger.DebugLog.Println("Start get files")
	bkp.logger.DebugLog.Printf("Token expiry %v", bkp.YaDProcessor.TokenInfo.Expiry)
	remoteFiles, err := bkp.YaDProcessor.GetRemoteFiles()
	if err != nil {
		bkp.logger.ErrorLog.Printf("error get remote files: %s", err)
		return make([]types.BackupFileInfo, 0), err
	}
	localFiles, err := getLocalBackupFiles(bkp.haApi, bkp.logger)
	if err != nil {
		bkp.logger.ErrorLog.Printf("error get local files: %s", err)
		return make([]types.BackupFileInfo, 0), err
	}

	return intersectFiles(localFiles, remoteFiles)
}

func (bkp *BkProcessor) ChooseFilesToUpload(files []types.BackupFileInfo) []types.ForUploadFileInfo {
	result := make([]types.ForUploadFileInfo, 0)
	for _, file := range files {

		if !file.IsRemote {
			// Файл ещё не загружен
			if file.IsLocal {
				// Файл локальный. Грузится всегда
				result = append(result, types.ForUploadFileInfo{
					LocalFileInfo:  file.GeneralInfo,
					RemoteFileName: file.RemoteFileName,
					Slug:           file.BackupSlug,
					IsLocal:        true,
					IsNetwork:      false,
				})
			} else if file.IsNetwork && bkp.enableUploadFromNetworkStorage && bkp.isNetworkStorageEnabled(file.Location) {
				// Файл из сетевого хранилища. Разрешён к загрузке
				result = append(result, types.ForUploadFileInfo{
					LocalFileInfo:  file.GeneralInfo,
					RemoteFileName: file.RemoteFileName,
					NetworkFileInfo: types.NetworkFileInfo{
						Location: file.Location,
					},
					Slug:      file.BackupSlug,
					IsLocal:   false,
					IsNetwork: true,
				})
			}
		}
	}
	return result
}

func (bkp *BkProcessor) isNetworkStorageEnabled(storage string) bool {
	if len(bkp.enabledNetworkStorages) == 0 {
		bkp.logger.DebugLog.Println("Enabled upload from any network storage")
		return true
	}
	_, ok := bkp.enabledNetworkStorages[strings.TrimSpace(storage)]
	if ok {
		bkp.logger.DebugLog.Printf("Enabled upload from '%s' network storage", storage)
	} else {
		bkp.logger.DebugLog.Printf("Disabled upload from '%s' network storage", storage)
	}
	return ok
}

func (bkp *BkProcessor) UploadFiles(files []types.ForUploadFileInfo) (ProcessedFilesResult, error) {
	return UploadFiles(bkp, files)
}

func (bkp *BkProcessor) ChooseFilesToDelete(files []types.BackupFileInfo, uploadFileCount int) []types.ForDeleteFileInfo {
	result := make([]types.ForDeleteFileInfo, 0)
	remoteFiles := make([]types.BackupFileInfo, 0)
	for _, file := range files {
		if file.IsRemote {
			remoteFiles = append(remoteFiles, file)

		}
	}

	fileAmount := uploadFileCount + len(remoteFiles)

	if bkp.remoteMaximumFilesQuantity >= fileAmount {
		bkp.logger.InfoLog.Printf("Not need delete files")
		return result
	}

	// Отсортируем. Старые файлы идут первыми.
	sort.Slice(remoteFiles, func(i, j int) bool {
		return time.Time(remoteFiles[i].GeneralInfo.Modified).Before(time.Time(remoteFiles[j].GeneralInfo.Modified))
	})

	for _, file := range remoteFiles {
		result = append(result, types.ForDeleteFileInfo{RemoteFileName: file.RemoteFileName,
			MD5:      "",
			FileInfo: file.GeneralInfo})
		fileAmount--
		if bkp.remoteMaximumFilesQuantity >= fileAmount {
			break
		}
	}

	bkp.logger.InfoLog.Printf("Need delete %d files", len(result))
	return result
}

func (bkp *BkProcessor) DeleteFiles(files []types.ForDeleteFileInfo) (ProcessedFilesResult, error) {
	isError := false
	deleted := 0
	errorDeleted := 0
	processedSize := types.FileSize(0)
	//TODO Add real Md5
	for _, file := range files {
		bkp.logger.DebugLog.Printf("Try delete %s", file.RemoteFileName)
		err := bkp.YaDProcessor.DeleteFile(file.RemoteFileName, file.MD5, true)
		if err != nil {
			bkp.logger.ErrorLog.Printf("Error when delete file %s. Err: %s", file.RemoteFileName, err)
			isError = true
			errorDeleted++
		} else {
			deleted++
			processedSize += file.FileInfo.Size
		}
	}
	err := fmt.Errorf("plug")
	err = nil
	if isError {
		err = fmt.Errorf("error when delete files")
	}
	return ProcessedFilesResult{Ok: deleted,
			Error:         errorDeleted,
			ProcessedSize: processedSize},
		err
}

func (bkp *BkProcessor) GetStatistic() (Statistic, error) {
	bkp.statisticMu.RLock()
	defer bkp.statisticMu.RUnlock()

	if bkp.isStatisticValid {
		return bkp.statistic, nil
	}

	bkp.statisticMu.RUnlock()
	bkp.UpdateAndGetStatistic()
	bkp.statisticMu.RLock()
	return bkp.statistic, nil
}

func (bkp *BkProcessor) EnsureStatistic() error {
	if !bkp.isStatisticValid {
		_, err := bkp.UpdateAndGetStatistic()
		if err != nil {
			return err
		}
	}

	return nil
}

func (bkp *BkProcessor) InvalidateStatistic() {
	bkp.statisticMu.Lock()
	defer bkp.statisticMu.Unlock()
	bkp.isStatisticValid = false
}

func (bkp *BkProcessor) UpdateAndGetStatistic() (Statistic, error) {
	bkp.statisticMu.Lock()
	defer bkp.statisticMu.Unlock()
	isError := false
	result := Statistic{
		YaDisk:         types.StorageStatistic{FileAmount: -1, FilesSize: 0, FreeSpace: 0},
		LocalStorage:   types.StorageStatistic{FileAmount: -1, FilesSize: 0, FreeSpace: 0},
		NetworkStorage: make(map[string]types.StorageStatistic),
	}

	statistic, err := bkp.YaDProcessor.GetStorageStatistic()
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error get yandex disc statistic %s", err)
		isError = true

	} else {
		result.YaDisk = statistic
	}

	haStatistic, err := bkp.GetHaStatistic()
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error get local storage statistic %s", err)
		isError = true
	}

	result.NetworkStorage = haStatistic.NetworkStorage
	result.LocalStorage = haStatistic.LocalStorage
	bkp.statistic = result
	bkp.isStatisticValid = !isError
	return result, nil
}

func (bkp *BkProcessor) GetHaStatistic() (HaStatistic, error) {
	result := HaStatistic{
		LocalStorage:   types.StorageStatistic{FileAmount: -1, FilesSize: 0, FreeSpace: 0},
		NetworkStorage: make(map[string]types.StorageStatistic),
	}

	localHaStorageStatistic, err := bkp.haApi.GetStorageStatistic()
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error get local storage statistic %s", err)
		return result, err
	}
	if ls, ok := localHaStorageStatistic["local"]; ok {
		result.LocalStorage = ls
	}

	delete(localHaStorageStatistic, "local")
	result.NetworkStorage = localHaStorageStatistic
	return result, nil

}

func (bkp *BkProcessor) CreateFullBackupSync() (bool, error) {
	operationId, err := bkp.CreateFullBackupAsync()
	if err != nil {
		return true, err
	}

	ok, response := bkp.operationManager.WaitOperationDone(operationId, bkp.waitCreateBackupInterval, bkp.waitCreateBackupTimeout)

	if !ok {
		bkp.logger.ErrorLog.Printf("Error waiting for create backup operation")
		return true, fmt.Errorf("error waiting for create backup operation")
	}
	bkp.logger.DebugLog.Printf("Create backup operation finished. %+v", response)
	return response.IsError, nil
}

func (bkp *BkProcessor) CreateFullBackupAsync() (string, error) {
	backupName := "Full_Y_Backup_" + time.Now().Format(time.DateTime)
	createBackupResult, err := bkp.haApi.CreateFullBackup(backupName)
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error create full backup %s", err)
		return "", err
	}
	const operationId = "create_backup"
	bkp.operationManager.StartOperation(operationId, "backup creating")
	go bkp.backgroundPolling(bkp.applCtx, createBackupResult.Job, operationId,
		func(withError bool, errorMessage string) bool {
			bkp.registerBackupResult(withError, errorMessage)
			return true
		})
	return operationId, nil
}
func (bkp *BkProcessor) registerBackupResult(withError bool, errorMessage string) {
	err := bkp.haApi.SetLastBackupState(withError, errorMessage)
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error register backup result %s", err)
	}
}

func (bkp *BkProcessor) backgroundPolling(parentCtx context.Context, jobID string, operationId string,
	finishedFunction func(isHaveErrors bool, errMessage string) bool) {
	// Защита от паники, чтобы не упал весь процесс
	defer func() {
		if rec := recover(); rec != nil {
			bkp.logger.ErrorLog.Printf("Panic in background process %s: %v", jobID, rec)
		}
	}()

	bkp.logger.DebugLog.Printf("Start periodical job checker [JobId %s] (interval: %v)", jobID, bkp.pollInterval)
	ctx, cancel := context.WithTimeout(parentCtx, bkp.checkJobTimeout)
	defer cancel()

	ticker := time.NewTicker(bkp.pollInterval)
	defer ticker.Stop() // Освобождаем ресурсы

	// Делаем первый запрос сразу, не дожидаясь первой минуты

	finished, withErrors, errorMessages := bkp.checkStatus(ctx, jobID, operationId)
	if finished {
		bkp.logger.DebugLog.Printf("Job finished immediate [JobId %s]", jobID)
		finishedFunction(withErrors, errorMessages)
		return
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				bkp.logger.DebugLog.Printf("Stop periodical job checker by timeout (%v) [JobId %s]", bkp.checkJobTimeout, jobID)

			} else {
				bkp.logger.DebugLog.Printf("Stop periodical job checker by shutdown [JobId %s]", jobID)
			}

			return
		case <-ticker.C:
			finished, withErrors, errorMessages = bkp.checkStatus(ctx, jobID, operationId)
			if finished {
				bkp.logger.DebugLog.Printf("Job finished [JobId %s]", jobID)
				finishedFunction(withErrors, errorMessages)
				return
			}
		}
	}
}

// checkStatus - выполняет второй REST запрос и проверяет условие
func (bkp *BkProcessor) checkStatus(ctx context.Context, jobId string, operationId string) (bool, bool, string) {
	jobInfo, err := bkp.haApi.GetJobInfo(jobId)
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error get job info %s", err)
		return false, false, ""
	}

	if len(jobInfo.Errors) > 0 {
		errors := utils.ConcatAndTruncateUnicode(jobInfo.Errors, ", ", 100)
		bkp.logger.ErrorLog.Printf("Error when execute job. [JobId %s]: %s", jobId, strings.Join(jobInfo.Errors, ", "))
		bkp.operationManager.ErrorDone(operationId, errors)
		return true, true, errors
	}
	if jobInfo.Done {
		bkp.operationManager.SuccessDone(operationId)
		return true, false, ""
	}

	bkp.logger.DebugLog.Printf("Job steel work. [JobId %s]: %s", jobId)
	return false, false, ""
}
