package yadiskoperate

import (
	"context"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"net/http"
	"time"
)

const itemTypeFile string = "file"

var minTime = time.Date(1990, time.January, 01, 12, 00, 0, 0, time.UTC)

func NewYandexDisk(accessToken string) (yadisk.YaDisk, error) {
	return yadisk.NewYaDisk(context.Background(), http.DefaultClient, &yadisk.Token{AccessToken: accessToken})

}

//// TODO
//func getDiskInfo(appybg *maintypes.AppData) (types.DiskInfo, error) {
//	if appybg.YaDisk == nil {
//		return types.DiskInfo{UsedSpace: 0, TotalSpace: 0}, fmt.Errorf("YandexDisk object is nil")
//	}
//
//	diskInfo, err := (*appybg.YaDisk).GetDisk([]string{"total_space", "used_space"})
//	if err != nil {
//		appybg.Logger.ErrorLog.Printf("Error when get remote disk info. %v", err)
//		return types.DiskInfo{UsedSpace: 0, TotalSpace: 0}, fmt.Errorf("error get YandexDisk info")
//	}
//
//	result := types.DiskInfo{UsedSpace: types.FileSize(diskInfo.UsedSpace),
//		TotalSpace: types.FileSize(diskInfo.TotalSpace)}
//	appybg.Logger.DebugLog.Printf("Get disk info. %v", err)
//	return result, nil
//}

//func uploadFile(appybg *maintypes.AppData, source string, destination string) error {
//	link, err := (*appybg.YaDisk).GetResourceUploadLink(destination, nil, true)
//	if err != nil {
//		return err
//	}
//	appybg.Logger.DebugLog.Printf("Get href %s", link.Href)
//
//	httpClient := &http.Client{}
//
//	logger := uploadbig.Logger{
//		DebugLog: appybg.Logger.DebugLog,
//		InfoLog:  appybg.Logger.InfoLog,
//		ErrorLog: appybg.Logger.ErrorLog,
//	}
//
//	uploader := uploadbig.New(types.PUT, link.Href, source, httpClient, int(types.MiB), &logger)
//
//	err = uploader.Init()
//	if err != nil {
//		return err
//	}
//
//	appybg.Logger.DebugLog.Printf("Success load file %s", source)
//
//	status, err := (*appybg.YaDisk).GetOperationStatus(link.OperationID, nil)
//	if err != nil {
//		return err
//	}
//
//	appybg.Logger.DebugLog.Printf("Status %s", status.Status)
//
//	return nil
//}

func convertDateString(modified string) (time.Time, error) {
	return time.Parse(time.RFC3339, modified)
}
