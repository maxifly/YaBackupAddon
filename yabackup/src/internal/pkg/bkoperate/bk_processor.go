package bkoperate

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/pkg/yadiskoperate"
	"ybg/internal/types"
)

const BACKUP_PATH = "/backup"

type Statistic struct {
	YaDisk         types.StorageStatistic
	LocalStorage   types.StorageStatistic
	NetworkStorage map[string]types.StorageStatistic
}

type BkProcessor struct {
	YaDProcessor                   *yadiskoperate.YaDProcessor
	haApi                          *haoperate.HaApiClient
	remoteMaximumFilesQuantity     int
	enabledNetworkStorages         map[string]struct{}
	enableUploadFromNetworkStorage bool
	statisticMu                    sync.RWMutex
	statistic                      Statistic
	isStatisticValid               bool
	logger                         *mylogger.Logger
}

func NewBkProcessor(yaDP *yadiskoperate.YaDProcessor, haApi *haoperate.HaApiClient,
	remoteMaximumFilesQuantity int, enableUploadFromNetworkStorage bool,
	enabledNetworkStorages []string,
	logger *mylogger.Logger) *BkProcessor {

	m := make(map[string]struct{})
	for _, element := range enabledNetworkStorages {
		m[strings.TrimSpace(element)] = struct{}{}
	}
	return &BkProcessor{
		YaDProcessor:                   yaDP,
		haApi:                          haApi,
		remoteMaximumFilesQuantity:     remoteMaximumFilesQuantity,
		enableUploadFromNetworkStorage: enableUploadFromNetworkStorage,
		enabledNetworkStorages:         m,
		logger:                         logger,
		isStatisticValid:               false,
	}
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
	bkp.UpdateStatistic()
	bkp.statisticMu.RLock()
	return bkp.statistic, nil
}

func (bkp *BkProcessor) EnsureStatistic() error {
	if !bkp.isStatisticValid {
		_, err := bkp.UpdateStatistic()
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

func (bkp *BkProcessor) UpdateStatistic() (Statistic, error) {
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

	localHaStorageStatistic, err := bkp.haApi.GetStorageStatistic()
	if err != nil {
		bkp.logger.ErrorLog.Printf("Error get yandex disc statistic %s", err)
		localHaStorageStatistic = make(map[string]types.StorageStatistic)
		isError = true
	}
	if ls, ok := localHaStorageStatistic["local"]; ok {
		result.LocalStorage = ls
	}

	delete(localHaStorageStatistic, "local")
	result.NetworkStorage = localHaStorageStatistic
	bkp.statistic = result
	bkp.isStatisticValid = !isError
	return result, nil
}
