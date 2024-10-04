package main

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
	"ybg/internal/maintypes"
	"ybg/internal/types"
)

func GetFilesInfo(application *maintypes.AppData) ([]types.BackupFileInfo, error) {
	application.Logger.DebugLog.Println("Start get files")
	application.Logger.DebugLog.Printf("Token expiry %v", application.TokenInfo.Expiry)
	remoteFiles, err := getRemoteFiles(application)
	if err != nil {
		return make([]types.BackupFileInfo, 0), err
	}
	localFiles, err := getLocalBackupFiles(application)
	if err != nil {
		return make([]types.BackupFileInfo, 0), err
	}

	return intersectFiles(application, localFiles, remoteFiles)
}

func UploadFiles(app *maintypes.AppData, files []types.ForUploadFileInfo) error {
	sort.Slice(files, func(i, j int) bool {
		return time.Time(files[i].LocalFileInfo.Modified).Before(time.Time(files[j].LocalFileInfo.Modified))
	})

	isError := false

	for _, file := range files {
		destinationName := app.Options.RemotePath + "/" + file.RemoteFileName
		sourceName := BACKUP_PATH + "/" + file.LocalFileInfo.Name
		app.Logger.DebugLog.Printf("Try upload %s into %s", sourceName, destinationName)
		err := uploadFile(app, sourceName, destinationName)
		if err != nil {
			app.Logger.ErrorLog.Printf("Error when upload file %s. Err: %s", sourceName, err)
			isError = true
		}
	}
	if isError {
		return fmt.Errorf("error when upload files")
	}
	return nil
}

func DeleteFiles(app *maintypes.AppData, files []types.ForDeleteFileInfo) error {
	isError := false
	//TODO Add real Md5
	for _, file := range files {
		remoteName := app.Options.RemotePath + "/" + file.RemoteFileName
		app.Logger.DebugLog.Printf("Try delete %s", remoteName)
		err := deleteFile(app, remoteName, file.MD5)
		if err != nil {
			app.Logger.ErrorLog.Printf("Error when delete file %s. Err: %s", remoteName, err)
			isError = true
		}
	}
	if isError {
		return fmt.Errorf("error when delete files")
	}
	return nil
}

func ChooseFilesToUpload(app *maintypes.AppData, files []types.BackupFileInfo) []types.ForUploadFileInfo {
	result := make([]types.ForUploadFileInfo, 0)
	for _, file := range files {
		if file.IsLocal && !file.IsRemote {
			result = append(result, types.ForUploadFileInfo{
				LocalFileInfo:  file.GeneralInfo,
				RemoteFileName: file.RemoteFileName,
			})
		}
	}
	app.Logger.InfoLog.Printf("Need upload %d files", len(result))
	return result
}

func ChooseFilesToDelete(app *maintypes.AppData, files []types.BackupFileInfo, uploadFileCount int) []types.ForDeleteFileInfo {
	result := make([]types.ForDeleteFileInfo, 0)
	remoteFiles := make([]types.BackupFileInfo, 0)
	for _, file := range files {
		if file.IsRemote {
			remoteFiles = append(remoteFiles, file)

		}
	}

	fileAmount := uploadFileCount + len(remoteFiles)

	if app.Options.RemoteMaximumFilesQuantity >= fileAmount {
		app.Logger.InfoLog.Printf("Not need delete files")
		return result
	}

	// Отсортируем. Старые файлы идут первыми.
	sort.Slice(remoteFiles, func(i, j int) bool {
		return time.Time(remoteFiles[i].GeneralInfo.Modified).Before(time.Time(remoteFiles[j].GeneralInfo.Modified))
	})

	for _, file := range remoteFiles {
		result = append(result, types.ForDeleteFileInfo{RemoteFileName: file.RemoteFileName, MD5: ""})
		fileAmount--
		if app.Options.RemoteMaximumFilesQuantity >= fileAmount {
			break
		}
	}

	app.Logger.InfoLog.Printf("Need delete %d files", len(result))
	return result
}

type stringSet map[string]bool

func intersectFiles(app *maintypes.AppData,
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
	return strings.ReplaceAll(strings.ReplaceAll(localFile.BackupName+"_"+localFile.Slug, " ", "-"), ":", "_")
}

func getLocalBackupFiles(app *maintypes.AppData) (map[string]types.LocalBackupFileInfo, error) {

	entries, err := os.ReadDir(BACKUP_PATH)
	if err != nil {
		app.Logger.ErrorLog.Printf("Unable to read backup %s. %v", BACKUP_PATH, err)
		return nil, fmt.Errorf("error when read local backups")
	}
	result := make(map[string]types.LocalBackupFileInfo)
	for _, entry := range entries {
		app.Logger.DebugLog.Printf("entry %+v", entry)
		info, err := entry.Info()
		if err != nil {
			app.Logger.ErrorLog.Printf("Error read file info %v", err)
			continue
		}
		app.Logger.DebugLog.Printf("info: %+v", info)

		if info.IsDir() {
			continue
		}

		filePath := filepath.Join(BACKUP_PATH, info.Name())
		app.Logger.DebugLog.Printf("Read %s", filePath)
		archInfo, err := extractArchInfo(app, filePath)
		if err != nil {
			app.Logger.ErrorLog.Printf("Error extract slug from %s %v", info.Name(), err)
			continue
		}

		result[archInfo.Slug] = types.LocalBackupFileInfo{
			GeneralInfo: convertBkFileInfoToGeneral(&info),
			Slug:        archInfo.Slug,
			BackupName:  archInfo.Name,
			Path:        filePath,
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

func extractArchInfo(app *maintypes.AppData, tarfile string) (*types.BackupArchInfo, error) {
	reader, err := os.Open(tarfile)
	if err != nil {
		return nil, fmt.Errorf("ERROR: cannot read tar file, error=[%v]\n", err)
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			app.Logger.ErrorLog.Printf("Can not close reader, error=[%v]", err)
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
			var data types.BackupArchInfo
			plan, err := io.ReadAll(tarReader)

			if err != nil {
				return nil, fmt.Errorf("cannot read backup info, error=[%v]", err)

			}

			err = json.Unmarshal(plan, &data)
			app.Logger.DebugLog.Printf("data= %+v\n", data)
			if err != nil {
				return nil, fmt.Errorf("cannot parse backup info, error=[%v]", err)
			}
			if data.Slug == "" || data.Name == "" {
				return nil, fmt.Errorf("cannot parse backup info. Necessary field not found")
			}
			return &data, nil
		}
	}
	return nil, fmt.Errorf("backup info not found")
}
