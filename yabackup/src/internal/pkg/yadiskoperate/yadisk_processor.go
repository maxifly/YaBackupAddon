package yadiskoperate

import (
	"fmt"
	uploadbig "github.com/maxifly/upload-big-file"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"io"
	"net/http"
	"time"
	"ybg/internal/pkg/downloader"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	om "ybg/internal/pkg/operationmanager"
	"ybg/internal/types"
)

type YaDProcessor struct {
	clientId     string
	clientSecret string
	remotePath   string
	TokenInfo    types.TokenInfo
	yaDisk       *yadisk.YaDisk
	downloader   *downloader.Downloader
	logger       *mylogger.Logger
}

func NewYaDProcessor(clientId string,
	clientSecret string,
	remotePath string,
	operationManager *om.OperationManager,
	logger *mylogger.Logger) *YaDProcessor {
	return &YaDProcessor{
		clientId:     clientId,
		clientSecret: clientSecret,
		remotePath:   remotePath,
		downloader:   downloader.New(operationManager, logger),
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

		modifiedTime, err := convertDateString(item.Modified)
		if err != nil {
			app.logger.ErrorLog.Printf("Can not parse data %s %v", item.Modified, err)
			modifiedTime = minTime
		}

		result = append(result, types.RemoteFileInfo{Name: item.Name,
			Size:     types.FileSize(item.Size),
			Created:  types.FileModified(modifiedTime),
			Modified: types.FileModified(modifiedTime)})

	}

	app.logger.DebugLog.Printf("Processing %d remote files", len(result))
	return result, nil

}

func (app *YaDProcessor) DownloadFile(sourceFileName, destination, id string) error {
	source := app.remotePath + "/" + sourceFileName
	app.logger.DebugLog.Printf("Download file: %s to %s", source, destination)

	link, err := (*app.yaDisk).GetResourceDownloadLink(source, nil)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when get download link for file: %w", err)
		return fmt.Errorf("error when get download link for file: %w", err)
	}

	err = app.downloader.Download(link.Href, destination, id, "from YD")
	if err != nil {
		app.logger.ErrorLog.Printf("Error when download file: %w", err)
		return fmt.Errorf("error when download file: %w", err)
	}
	app.logger.DebugLog.Printf("File: %s downloaded to %s", source, destination)
	return nil

}

func (app *YaDProcessor) UploadFile(source string, destinationFileName string) error {
	return app.innerUpload(source, nil, 0, destinationFileName)
}
func (app *YaDProcessor) UploadDataFromSlug(haApi *haoperate.HaApiClient, slug string, destinationFileName string) error {
	size, body, err := haApi.GetDownloadBackupBody(slug)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when upload network file: %w", err)
		return fmt.Errorf("error when upload network file: %w", err)
	}
	defer body.Close()

	if size == 0 {
		app.logger.ErrorLog.Printf("Can not upload network file with 0 size.")
		return fmt.Errorf("ean not upload network file with 0 size")
	}

	var reader io.Reader = body

	err = app.innerUpload("", &reader, size, destinationFileName)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when upload network file %w", err)
		return err
	}
	return nil

}

func (app *YaDProcessor) innerUpload(source string, reader *io.Reader, size int64, destinationFileName string) error {
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

	var uploader *uploadbig.UploadData = nil

	switch {
	case source != "":
		uploader = uploadbig.NewUploaderFromFile(types.PUT, link.Href, source, nil, httpClient, int(types.MiB), &logger)
	case reader != nil:
		uploader = uploadbig.NewUploaderFromReader(types.PUT, link.Href, reader, size, nil, httpClient, int(types.MiB), &logger)
	default:
		return fmt.Errorf("unsupported upload source")
	}

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

func (app *YaDProcessor) DeleteFile(remoteFileName string, md5 string, permanently bool) error {
	remoteName := app.remotePath + "/" + remoteFileName
	app.logger.DebugLog.Printf("Try delete %s", remoteName)

	_, err := (*app.yaDisk).DeleteResource(remoteName, nil, false, md5, permanently)
	if err != nil {
		return err
	}
	if permanently {
		app.logger.InfoLog.Printf("Success permanently delete file %s", remoteName)
	} else {
		app.logger.InfoLog.Printf("Success delete file %s", remoteName)
	}

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
