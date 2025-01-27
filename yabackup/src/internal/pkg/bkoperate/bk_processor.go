package bkoperate

import (
	"fmt"
	"sort"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/pkg/yadiskoperate"
	"ybg/internal/types"
)

const BACKUP_PATH = "/backup"

type BkProcessor struct {
	YaDProcessor               *yadiskoperate.YaDProcessor
	remoteMaximumFilesQuantity int
	logger                     *mylogger.Logger
}

func NewBkProcessor(yaDP *yadiskoperate.YaDProcessor, remoteMaximumFilesQuantity int, logger *mylogger.Logger) *BkProcessor {
	return &BkProcessor{
		YaDProcessor:               yaDP,
		remoteMaximumFilesQuantity: remoteMaximumFilesQuantity,
		logger:                     logger,
	}
}

func (bkp *BkProcessor) GetFilesInfo() ([]types.BackupFileInfo, error) {
	bkp.logger.DebugLog.Println("Start get files")
	bkp.logger.DebugLog.Printf("Token expiry %v", bkp.YaDProcessor.TokenInfo.Expiry)
	remoteFiles, err := bkp.YaDProcessor.GetRemoteFiles()
	if err != nil {
		return make([]types.BackupFileInfo, 0), err
	}
	localFiles, err := getLocalBackupFiles(bkp.logger)
	if err != nil {
		return make([]types.BackupFileInfo, 0), err
	}

	return intersectFiles(localFiles, remoteFiles)
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
