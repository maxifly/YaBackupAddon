package main

import (
	"context"
	"fmt"
	uploadbig "github.com/maxifly/upload-big-file"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"net/http"
	"time"
	"ybg/internal/maintypes"
	"ybg/internal/types"
)

const itemTypeFile string = "file"

var minTime = time.Date(1990, time.January, 01, 12, 00, 0, 0, time.UTC)

func NewYandexDisk(accessToken string) (yadisk.YaDisk, error) {
	return yadisk.NewYaDisk(context.Background(), http.DefaultClient, &yadisk.Token{AccessToken: accessToken})

}

// TODO
func getDiskInfo(app *maintypes.AppData) (types.DiskInfo, error) {
	if app.YaDisk == nil {
		return types.DiskInfo{UsedSpace: 0, TotalSpace: 0}, fmt.Errorf("YandexDisk object is nil")
	}

	diskInfo, err := (*app.YaDisk).GetDisk([]string{"total_space", "used_space"})
	if err != nil {
		app.Logger.ErrorLog.Printf("Error when get remote disk info. %v", err)
		return types.DiskInfo{UsedSpace: 0, TotalSpace: 0}, fmt.Errorf("error get YandexDisk info")
	}

	result := types.DiskInfo{UsedSpace: types.FileSize(diskInfo.UsedSpace),
		TotalSpace: types.FileSize(diskInfo.TotalSpace)}
	app.Logger.DebugLog.Printf("Get disk info. %v", err)
	return result, nil
}

func getRemoteFiles(app *maintypes.AppData) ([]types.RemoteFileInfo, error) {
	app.Logger.InfoLog.Printf("%v", app.Options.RemotePath)
	if app.YaDisk == nil {
		return nil, fmt.Errorf("YandexDisk object is nil")
	}
	result := make([]types.RemoteFileInfo, 0)

	resource, err := (*app.YaDisk).GetResource(app.Options.RemotePath, make([]string, 0), 10000, 0, false, "0", "name")
	if err != nil {
		app.Logger.ErrorLog.Printf("Error when get remote files from path %s. %v", app.Options.RemotePath, err)
		return result, err
	}

	app.Logger.DebugLog.Printf("Found %d remote items", len(resource.Embedded.Items))

	for _, item := range resource.Embedded.Items {
		if item.Type != itemTypeFile {
			continue
		}

		modifyedTime, err := convertDateString(item.Modified)
		if err != nil {
			app.Logger.ErrorLog.Printf("Can not parse data %s %v", item.Modified, err)
			modifyedTime = minTime
		}
		result = append(result, types.RemoteFileInfo{Name: item.Name,
			Size:     types.FileSize(item.Size),
			Modified: types.FileModified(modifyedTime)})

	}

	app.Logger.DebugLog.Printf("Processing %d remote files", len(result))
	return result, nil

}

func uploadFile(app *maintypes.AppData, source string, destination string) error {
	link, err := (*app.YaDisk).GetResourceUploadLink(destination, nil, true)
	if err != nil {
		return err
	}
	app.Logger.DebugLog.Printf("Get href %s", link.Href)

	httpClient := &http.Client{}

	logger := uploadbig.Logger{
		DebugLog: app.Logger.DebugLog,
		InfoLog:  app.Logger.InfoLog,
		ErrorLog: app.Logger.ErrorLog,
	}

	uploader := uploadbig.New(types.PUT, link.Href, source, httpClient, int(types.MiB), &logger)

	err = uploader.Init()
	if err != nil {
		return err
	}

	app.Logger.DebugLog.Printf("Success load file %s", source)

	status, err := (*app.YaDisk).GetOperationStatus(link.OperationID, nil)
	if err != nil {
		return err
	}

	app.Logger.DebugLog.Printf("Status %s", status.Status)

	return nil
}

func deleteFile(app *maintypes.AppData, fileName string, md5 string) error {
	_, err := (*app.YaDisk).DeleteResource(fileName, nil, false, md5, false)
	if err != nil {
		return err
	}
	app.Logger.InfoLog.Printf("Success delete file %s", fileName)
	return nil
}
func convertDateString(modified string) (time.Time, error) {
	return time.Parse(time.RFC3339, modified)
}
