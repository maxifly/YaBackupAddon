package main

import (
	"context"
	"fmt"
	uploadbig "github.com/maxifly/upload-big-file"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"net/http"
	"time"
)

const itemTypeFile string = "file"

var minTime = time.Date(1990, time.January, 01, 12, 00, 0, 0, time.UTC)

func NewYandexDisk(accessToken string) (yadisk.YaDisk, error) {
	return yadisk.NewYaDisk(context.Background(), http.DefaultClient, &yadisk.Token{AccessToken: accessToken})

}

func getRemoteFiles(app *Application) ([]RemoteFileInfo, error) {
	app.infoLog.Printf("%v", app.options.RemotePath)
	if app.yaDisk == nil {
		return nil, fmt.Errorf("YandexDisk object is nil")
	}
	result := make([]RemoteFileInfo, 0)

	resource, err := (*app.yaDisk).GetResource(app.options.RemotePath, make([]string, 0), 10000, 0, false, "0", "name")
	if err != nil {
		app.errorLog.Printf("Error when get remote files from path %s. %v", app.options.RemotePath, err)
		return result, err
	}

	app.debugLog.Printf("Found %d remote items", len(resource.Embedded.Items))

	for _, item := range resource.Embedded.Items {
		if item.Type != itemTypeFile {
			continue
		}

		modifyedTime, err := convertDateString(item.Modified)
		if err != nil {
			app.errorLog.Printf("Can not parse data %s %v", item.Modified, err)
			modifyedTime = minTime
		}
		result = append(result, RemoteFileInfo{Name: item.Name,
			Size:     fileSize(item.Size),
			Modified: fileModified(modifyedTime)})

	}

	app.debugLog.Printf("Processing %d remote files", len(result))
	return result, nil

}

func uploadFile(app *Application, source string, destination string) error {
	link, err := (*app.yaDisk).GetResourceUploadLink(destination, nil, true)
	if err != nil {
		return err
	}
	app.debugLog.Printf("Get href %s", link.Href)

	httpClient := &http.Client{}

	logger := uploadbig.Logger{
		DebugLog: app.debugLog,
		InfoLog:  app.infoLog,
		ErrorLog: app.errorLog,
	}

	uploader := uploadbig.New(PUT, link.Href, source, httpClient, int(MiB), &logger)

	err = uploader.Init()
	if err != nil {
		return err
	}

	app.debugLog.Printf("Success load file %s", source)

	status, err := (*app.yaDisk).GetOperationStatus(link.OperationID, nil)
	if err != nil {
		return err
	}

	app.debugLog.Printf("Status %s", status.Status)

	return nil
}

func deleteFile(app *Application, fileName string, md5 string) error {
	_, err := (*app.yaDisk).DeleteResource(fileName, nil, false, md5, false)
	if err != nil {
		return err
	}
	app.infoLog.Printf("Success delete file %s", fileName)
	return nil
}
func convertDateString(modified string) (time.Time, error) {
	return time.Parse(time.RFC3339, modified)
}
