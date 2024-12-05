package bkoperate

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

type ProcessedFilesResult struct {
	Ok            int
	Error         int
	ProcessedSize types.FileSize
}

func UploadFiles(app *BkProcessor, files []types.ForUploadFileInfo) (ProcessedFilesResult, error) {
	sort.Slice(files, func(i, j int) bool {
		return time.Time(files[i].LocalFileInfo.Modified).Before(time.Time(files[j].LocalFileInfo.Modified))
	})

	isError := false
	uploaded := 0
	errorUploaded := 0
	processedSize := types.FileSize(0)

	for _, file := range files {
		sourceName := BACKUP_PATH + "/" + file.LocalFileInfo.Name
		app.logger.DebugLog.Printf("Try upload %s ", sourceName)
		err := app.YaDProcessor.UploadFile(sourceName, file.RemoteFileName)
		if err != nil {
			app.logger.ErrorLog.Printf("Error when upload file %s. Err: %s", sourceName, err)
			isError = true
			errorUploaded++
		} else {
			uploaded++
			processedSize += file.LocalFileInfo.Size
		}
	}

	err := fmt.Errorf("plug")
	err = nil
	if isError {
		err = fmt.Errorf("error when upload files")
	}
	return ProcessedFilesResult{Ok: uploaded,
			Error:         errorUploaded,
			ProcessedSize: processedSize},
		err
}

func ChooseFilesToUpload(files []types.BackupFileInfo) []types.ForUploadFileInfo {
	result := make([]types.ForUploadFileInfo, 0)
	for _, file := range files {
		if file.IsLocal && !file.IsRemote {
			result = append(result, types.ForUploadFileInfo{
				LocalFileInfo:  file.GeneralInfo,
				RemoteFileName: file.RemoteFileName,
			})
		}
	}
	return result
}

type stringSet map[string]bool

func intersectFiles(
	localFiles map[string]types.LocalBackupFileInfo,
	remoteFiles []types.RemoteFileInfo) ([]types.BackupFileInfo, error) {

	remoteFileNames := make(stringSet)
	processedRemoteFile := make(stringSet)

	for _, remoteFile := range remoteFiles {
		remoteFileNames[remoteFile.Name] = true
	}

	result := make([]types.BackupFileInfo, 0, len(localFiles))

	// Обработаем локальные файлы
	for _, localFile := range localFiles {
		remoteFileName := generateRemoteFileName(localFile)
		_, isRemote := remoteFileNames[remoteFileName]

		result = append(result,
			types.BackupFileInfo{
				GeneralInfo:    localFile.GeneralInfo,
				BackupArchInfo: localFile.BackupArchInfo,
				BackupSlug:     localFile.BackupSlug,
				BackupName:     localFile.BackupName,
				RemoteFileName: remoteFileName,
				IsLocal:        true,
				IsRemote:       isRemote,
			})

		processedRemoteFile[remoteFileName] = true
	}

	for _, remoteFile := range remoteFiles {
		if _, isProcessing := processedRemoteFile[remoteFile.Name]; !isProcessing {
			result = append(result,
				types.BackupFileInfo{
					GeneralInfo: types.GeneralFileInfo{
						Name:     remoteFile.Name,
						Size:     remoteFile.Size,
						Modified: remoteFile.Modified,
					},
					BackupArchInfo: &types.BackupArchInfo{HaVersion: "???",
						Folders: make([]string, 0),
						Addons:  make([]types.HaAddonInfo, 0),
					},
					BackupName:     remoteFile.Name,
					RemoteFileName: remoteFile.Name,
					IsLocal:        false,
					IsRemote:       true,
				})
			processedRemoteFile[remoteFile.Name] = true
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return time.Time(result[i].GeneralInfo.Modified).After(time.Time(result[j].GeneralInfo.Modified))
	})
	return result, nil
}

// localFile.CreateDate.Format("02.01.2006 15:04:05 MST"),
func generateRemoteFileName(localFile types.LocalBackupFileInfo) string {
	return strings.ReplaceAll(strings.ReplaceAll(localFile.BackupName+"_"+localFile.BackupSlug, " ", "-"), ":", "_")
}

func getLocalBackupFiles(logger *mylogger.Logger) (map[string]types.LocalBackupFileInfo, error) {

	entries, err := os.ReadDir(BACKUP_PATH)
	if err != nil {
		logger.ErrorLog.Printf("Unable to read backup %s. %v", BACKUP_PATH, err)
		return nil, fmt.Errorf("error when read local backups")
	}
	result := make(map[string]types.LocalBackupFileInfo)
	for _, entry := range entries {
		logger.DebugLog.Printf("entry %+v", entry)
		info, err := entry.Info()
		if err != nil {
			logger.ErrorLog.Printf("Error read file info %v", err)
			continue
		}
		logger.DebugLog.Printf("info: %+v", info)

		if info.IsDir() {
			continue
		}

		filePath := filepath.Join(BACKUP_PATH, info.Name())
		logger.DebugLog.Printf("Read %s", filePath)
		archInfo, err := extractArchInfo(logger, filePath)
		if err != nil {
			logger.ErrorLog.Printf("Error extract slug from %s %v", info.Name(), err)
			continue
		}

		result[archInfo.Slug] = types.LocalBackupFileInfo{
			GeneralInfo:    convertBkFileInfoToGeneral(&info),
			BackupArchInfo: archInfo,
			BackupSlug:     archInfo.Slug,
			BackupName:     archInfo.Name,
			Path:           filePath,
		}
	}
	return result, nil

}

func convertBkFileInfoToGeneral(bkFileInfo *fs.FileInfo) types.GeneralFileInfo {
	return types.GeneralFileInfo{Name: (*bkFileInfo).Name(),
		Size:     types.FileSize((*bkFileInfo).Size()),
		Modified: types.FileModified((*bkFileInfo).ModTime()),
	}
}

func extractArchInfo(logger *mylogger.Logger, tarfile string) (*types.BackupArchInfo, error) {
	reader, err := os.Open(tarfile)
	if err != nil {
		return nil, fmt.Errorf("ERROR: cannot read tar file, error=[%v]\n", err)
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			logger.ErrorLog.Printf("Can not close reader, error=[%v]", err)
		}
	}(reader)

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("cannot read tar file, error=[%v]", err)
		}

		_, err = json.Marshal(header)
		if err != nil {
			return nil, fmt.Errorf("cannot parse header, error=[%v]", err)
		}

		info := header.FileInfo()
		if info.IsDir() || info.Name() != "backup.json" {
			continue
		} else {
			var data types.HaBackupInfo
			plan, err := io.ReadAll(tarReader)

			if err != nil {
				return nil, fmt.Errorf("cannot read backup info, error=[%v]", err)

			}

			err = json.Unmarshal(plan, &data)
			logger.DebugLog.Printf("data= %+v\n", data)
			if err != nil {
				return nil, fmt.Errorf("cannot parse backup info, error=[%v]", err)
			}
			if data.Slug == "" || data.Name == "" {
				return nil, fmt.Errorf("cannot parse backup info. Necessary field not found")
			}

			result := types.BackupArchInfo{
				Slug:       data.Slug,
				Name:       data.Name,
				BackupType: data.BackupType,
				HaVersion:  data.HaVersion,
				CoreInfo:   data.HaCoreInfo,
				Folders:    data.Folders,
				Addons:     data.Addons,
			}

			if &data.BackupCreated != nil {
				result.BackupCreated = types.FileModified(data.BackupCreated.Time)
			}
			return &result, nil
		}
	}
	return nil, fmt.Errorf("backup info not found")
}
