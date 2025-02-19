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
	"ybg/internal/pkg/haoperate"
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

		if file.IsLocal {
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
		} else if file.IsNetwork {
			app.logger.DebugLog.Printf("Try upload network file %s ", file.NetworkFileInfo.Slug)
			err := app.YaDProcessor.UploadDataFromSlug(app.haApi, file.NetworkFileInfo.Slug, file.RemoteFileName)
			if err != nil {
				app.logger.ErrorLog.Printf("Error when upload network file %s. Err: %s", file.NetworkFileInfo.Slug, err)
				isError = true
				errorUploaded++
			} else {
				uploaded++
				processedSize += file.LocalFileInfo.Size
			}
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

type stringSet map[string]bool
type remoteFilesMap map[string]types.RemoteFileInfo

func intersectFiles(
	localFiles map[string]types.LocalBackupFileInfo,
	remoteFiles []types.RemoteFileInfo) ([]types.BackupFileInfo, error) {

	remoteFileNames := make(remoteFilesMap)
	processedRemoteFile := make(stringSet)

	for _, remoteFile := range remoteFiles {
		remoteFileNames[remoteFile.Name] = remoteFile
	}

	result := make([]types.BackupFileInfo, 0, len(localFiles))

	// Обработаем локальные файлы
	for _, localFile := range localFiles {
		remoteFileName := generateRemoteFileName(localFile)
		remoteFileInfo, isRemote := remoteFileNames[remoteFileName]

		backupFileInfo := types.BackupFileInfo{
			GeneralInfo:    localFile.GeneralInfo,
			BackupArchInfo: localFile.BackupArchInfo,
			BackupSlug:     localFile.BackupSlug,
			BackupName:     localFile.BackupName,
			RemoteFileName: remoteFileName,
			Location:       localFile.Location,
			IsLocal:        localFile.IsLocal,
			IsNetwork:      localFile.IsNetwork,
			IsRemote:       isRemote,
			IsProtected:    localFile.IsProtected,
		}

		if isRemote {
			backupFileInfo.Downloaded = remoteFileInfo.Created
		}

		result = append(result,
			backupFileInfo)

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
						Created:  remoteFile.Created,
					},
					BackupArchInfo: &types.BackupArchInfo{HaVersion: "???",
						Folders: make([]string, 0),
						Addons:  make([]types.HaAddonInfo, 0),
					},
					BackupName:     remoteFile.Name,
					RemoteFileName: remoteFile.Name,
					Downloaded:     remoteFile.Created,
					IsLocal:        false,
					IsRemote:       true,
				})
			processedRemoteFile[remoteFile.Name] = true
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return time.Time(getSortedTime(&result[i])).After(time.Time(getSortedTime(&result[j])))
	})
	return result, nil
}

func getSortedTime(backupFileInfo *types.BackupFileInfo) types.FileModified {
	if backupFileInfo.IsRemote {
		return backupFileInfo.Downloaded
	}
	return backupFileInfo.GeneralInfo.Modified
}

func generateRemoteFileName(localFile types.LocalBackupFileInfo) string {
	return strings.ReplaceAll(strings.ReplaceAll(localFile.BackupName+"_"+localFile.BackupSlug, " ", "-"), ":", "_")
}

func getLocalBackupFiles(haApi *haoperate.HaApiClient, logger *mylogger.Logger) (map[string]types.LocalBackupFileInfo, error) {
	fileNames, err := getAllFileNames(logger, BACKUP_PATH)
	if err != nil {
		return nil, err
	}

	list, err := haApi.GetBackupSlugsList()
	if err != nil {
		return nil, err
	}
	//result := make([]haoperate.HaBackupInfo, len(list.Backups))

	result := make(map[string]types.LocalBackupFileInfo)

	for _, element := range list.Backups {
		information, err := haApi.GetBackupInformation(element.Slug)
		if err != nil {
			logger.ErrorLog.Printf("Error when get information about backup %s. %s", element, err)
			continue
		}

		isLocal := information.Location == ""

		fileName := ""
		filePath := ""

		if isLocal {
			fileName, err = findFile(&fileNames, information.Slug)
			if err != nil {
				return nil, err
			}
			filePath = filepath.Join(BACKUP_PATH, fileName)
		}

		result[information.Slug] = types.LocalBackupFileInfo{
			GeneralInfo:    convertHaBackupInfoToGeneral(information, fileName),
			BackupArchInfo: convertHaBackupInfoToBackupArchInfo(information),
			BackupSlug:     information.Slug,
			BackupName:     information.Name,
			IsProtected:    information.Protected,
			IsNetwork:      !isLocal || hasNonEmptyElement(&information.Locations),
			IsLocal:        isLocal,
			Path:           filePath,
			Location:       strings.TrimSpace(information.Location),
		}

	}

	return result, nil

}

func hasNonEmptyElement(arr *[]string) bool {
	for _, str := range *arr {
		if str != "" {
			return true
		}
	}
	return false
}

func findFile(files *[]string, slug string) (string, error) {
	for _, str := range *files {
		if strings.Contains(str, slug) {
			return str, nil
		}
	}
	return "", fmt.Errorf("file not found")
}

func convertHaBackupInfoToGeneral(haFileInfo *haoperate.HaBackupInfo, fileName string) types.GeneralFileInfo {
	result := types.GeneralFileInfo{Name: fileName,
		Size:     types.FileSize((*haFileInfo).Size),
		Created:  types.FileModified((*haFileInfo).BackupCreated.Time),
		Modified: types.FileModified((*haFileInfo).BackupCreated.Time),
	}

	if fileName == "" {
		result.Name = (*haFileInfo).Name
	}

	return result
}

func convertHaBackupInfoToBackupArchInfo(haFileInfo *haoperate.HaBackupInfo) *types.BackupArchInfo {
	haCoreInfo := types.HaCoreInfo{
		Version: haFileInfo.HaCoreVersion,
	}

	return &types.BackupArchInfo{
		Slug:          haFileInfo.Slug,
		Name:          haFileInfo.Name,
		BackupType:    haFileInfo.BackupType,
		HaVersion:     haFileInfo.HaSupervisorVersion,
		CoreInfo:      haCoreInfo,
		BackupCreated: types.FileModified((*haFileInfo).BackupCreated.Time),
		Folders:       haFileInfo.Folders,
		Addons:        *convertAddonList(&haFileInfo.Addons),
	}
}

func convertAddonList(haAddons *[]haoperate.HaAddonInfo) *[]types.HaAddonInfo {
	result := make([]types.HaAddonInfo, len(*haAddons))

	for i, element := range *haAddons {
		result[i] = types.HaAddonInfo{
			Slug:    element.Slug,
			Name:    element.Name,
			Version: element.Version,
		}
	}

	return &result
}

func getAllFileNames(logger *mylogger.Logger, path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		logger.ErrorLog.Printf("Unable to read backup %s. %v", BACKUP_PATH, err)
		return nil, fmt.Errorf("error when read local backups")
	}

	result := make([]string, len(entries))
	for i, entry := range entries {
		result[i] = entry.Name()
	}
	return result, nil
}

func convertBkFileInfoToGeneral(bkFileInfo *fs.FileInfo) types.GeneralFileInfo {
	return types.GeneralFileInfo{Name: (*bkFileInfo).Name(),
		Size:     types.FileSize((*bkFileInfo).Size()),
		Created:  types.FileModified((*bkFileInfo).ModTime()),
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
