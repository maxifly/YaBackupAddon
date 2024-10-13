package yadiskoperate

import (
	"fmt"
	uploadbig "github.com/maxifly/upload-big-file"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"net/http"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

type YaDProcessor struct {
	clientId     string
	clientSecret string
	remotePath   string
	TokenInfo    types.TokenInfo
	yaDisk       *yadisk.YaDisk
	logger       *mylogger.Logger
}

func NewYaDProcessor(clientId string,
	clientSecret string,
	remotePath string,
	logger *mylogger.Logger) *YaDProcessor {
	return &YaDProcessor{
		clientId:     clientId,
		clientSecret: clientSecret,
		remotePath:   remotePath,
		logger:       logger,
	}
}

func (app *YaDProcessor) EnsureTokenInfo() {
	if isTokenEmpty(app.TokenInfo) {
		token, err := readToken()
		if err != nil {
			app.logger.ErrorLog.Printf("Error read token info %v", err)
			return
		}
		app.TokenInfo = token
	}
}

func (app *YaDProcessor) RefreshTokenIsNeed() bool {
	if app.TokenInfo.Expiry.After(time.Now().Add(time.Duration(240) * time.Hour)) {
		app.logger.DebugLog.Printf("Not need refresh token")
		return false
	}

	tokenInfo, err := RefreshToken(app.clientId, app.clientSecret, app.TokenInfo)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when refresh token %v", err)
		return false
	}
	app.logger.InfoLog.Printf("%+v", tokenInfo)

	err = writeToken(*tokenInfo)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when write token %v", err)
		return false
	}

	app.TokenInfo = *tokenInfo
	app.logger.InfoLog.Printf("Refresh token done")
	app.EnsureYandexDisk()
	return true
}

func (app *YaDProcessor) CreateToken(checkCode string) (types.TokenInfo, error) {
	tokenInfo, err := CreateToken(app.clientId, app.clientSecret, checkCode)
	if err != nil {
		app.logger.ErrorLog.Printf("Get token error. %v", err.Error())
		return tokenInfo, err
	}
	app.logger.DebugLog.Printf("Create token success")
	err = writeToken(tokenInfo)
	if err == nil {
		app.logger.DebugLog.Printf("Write token success.")
		app.TokenInfo = tokenInfo
	} else {
		app.logger.ErrorLog.Printf("Save token error. %v", err)
	}

	return tokenInfo, err
}

func (app *YaDProcessor) EnsureYandexDisk() {
	if !isTokenEmpty(app.TokenInfo) {
		disk, err := NewYandexDisk(app.TokenInfo.AccessToken)
		if err != nil {
			app.logger.ErrorLog.Printf("Error when create YaDisk %v", err)
			return
		}
		app.yaDisk = &disk
	}
}
func (app *YaDProcessor) GetCheckCodeUrl() string {
	return GetCheckCodeUrl(app.clientId)
}

func (app *YaDProcessor) IsTokenEmpty() bool {
	return isTokenEmpty(app.TokenInfo)
}

func (app *YaDProcessor) IsTokenValid() bool {
	return isTokenValid(app.TokenInfo)
}

func (app *YaDProcessor) GetRemoteFiles() ([]types.RemoteFileInfo, error) {
	app.logger.InfoLog.Printf("%v", app.remotePath)
	if app.yaDisk == nil {
		return nil, fmt.Errorf("YandexDisk object is nil")
	}
	result := make([]types.RemoteFileInfo, 0)

	resource, err := (*app.yaDisk).GetResource(app.remotePath, make([]string, 0), 10000, 0, false, "0", "name")
	if err != nil {
		app.logger.ErrorLog.Printf("Error when get remote files from path %s. %v", app.remotePath, err)
		return result, err
	}

	app.logger.DebugLog.Printf("Found %d remote items", len(resource.Embedded.Items))

	for _, item := range resource.Embedded.Items {
		if item.Type != itemTypeFile {
			continue
		}

		modifyedTime, err := convertDateString(item.Modified)
		if err != nil {
			app.logger.ErrorLog.Printf("Can not parse data %s %v", item.Modified, err)
			modifyedTime = minTime
		}
		result = append(result, types.RemoteFileInfo{Name: item.Name,
			Size:     types.FileSize(item.Size),
			Modified: types.FileModified(modifyedTime)})

	}

	app.logger.DebugLog.Printf("Processing %d remote files", len(result))
	return result, nil

}

func (app *YaDProcessor) UploadFile(source string, destinationFileName string) error {
	destination := app.remotePath + "/" + destinationFileName
	app.logger.DebugLog.Printf("Try upload %s into %s", source, destination)
	link, err := (*app.yaDisk).GetResourceUploadLink(destination, nil, true)
	if err != nil {
		return err
	}
	app.logger.DebugLog.Printf("Get href %s", link.Href)

	httpClient := &http.Client{}

	logger := uploadbig.Logger{
		DebugLog: app.logger.DebugLog,
		InfoLog:  app.logger.InfoLog,
		ErrorLog: app.logger.ErrorLog,
	}

	uploader := uploadbig.New(types.PUT, link.Href, source, httpClient, int(types.MiB), &logger)

	err = uploader.Init()
	if err != nil {
		return err
	}

	app.logger.DebugLog.Printf("Success load file %s", source)

	status, err := (*app.yaDisk).GetOperationStatus(link.OperationID, nil)
	if err != nil {
		return err
	}

	app.logger.DebugLog.Printf("Status %s", status.Status)

	return nil
}

func (app *YaDProcessor) DeleteFile(remoteFileName string, md5 string) error {
	remoteName := app.remotePath + "/" + remoteFileName
	app.logger.DebugLog.Printf("Try delete %s", remoteName)

	_, err := (*app.yaDisk).DeleteResource(remoteName, nil, false, md5, false)
	if err != nil {
		return err
	}
	app.logger.InfoLog.Printf("Success delete file %s", remoteName)
	return nil
}

func (app *YaDProcessor) GetDiskInfo() (types.DiskInfo, error) {
	if app.yaDisk == nil {
		return types.DiskInfo{UsedSpace: 0, TotalSpace: 0}, fmt.Errorf("YandexDisk object is nil")
	}

	diskInfo, err := (*app.yaDisk).GetDisk([]string{"total_space", "used_space"})
	if err != nil {
		app.logger.ErrorLog.Printf("Error when get remote disk info. %v", err)
		return types.DiskInfo{UsedSpace: 0, TotalSpace: 0}, fmt.Errorf("error get YandexDisk info")
	}

	result := types.DiskInfo{UsedSpace: types.FileSize(diskInfo.UsedSpace),
		TotalSpace: types.FileSize(diskInfo.TotalSpace)}
	app.logger.DebugLog.Printf("Get disk info. %v", err)
	return result, nil
}
