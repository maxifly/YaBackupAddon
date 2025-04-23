package rest

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"net/http"
	"path/filepath"
	"time"
	"ybg/internal/pkg/bkoperate"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	om "ybg/internal/pkg/operationmanager"
	"ybg/internal/pkg/yadiskoperate"
	"ybg/internal/types"
)

const headerYbaOperationId = "yba-operation-id"

type AlertMessage struct {
	Message string
}
type BackupResponse struct {
	IsDarkTheme   bool
	AlertMessages []AlertMessage
	BFiles        []types.BackupFileInfo
	AddonIcons    map[string]string
}
type GetTokenResponse struct {
	IsDarkTheme   bool
	AlertMessages []AlertMessage
	CheckCodeUrl  string
}

type Rest struct {
	logger           *mylogger.Logger
	operationManager *om.OperationManager
	TokenInfo        types.TokenInfo
	yaDProcessor     *yadiskoperate.YaDProcessor
	bKProcessor      *bkoperate.BkProcessor
	haApi            *haoperate.HaApiClient
	router           *mux.Router
	port             string
	theme            string
	icons            map[string]string
}

func NewRest(port string,
	yaDProcessor *yadiskoperate.YaDProcessor,
	bKProcessor *bkoperate.BkProcessor,
	haApi *haoperate.HaApiClient,
	theme string,
	operationManager *om.OperationManager,
	logger *mylogger.Logger) (*Rest, error) {

	router := mux.NewRouter()
	fileServer := http.FileServer(http.Dir("./internal/pkg/rest/ui/static/"))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fileServer))
	restObj := Rest{port: port,
		yaDProcessor:     yaDProcessor,
		bKProcessor:      bKProcessor,
		haApi:            haApi,
		theme:            theme,
		router:           router,
		logger:           logger,
		operationManager: operationManager,
		icons:            make(map[string]string)}

	router.HandleFunc("/", restObj.indexHandler).Methods("GET")
	router.HandleFunc("/index", restObj.indexHandler).Methods("GET")
	router.HandleFunc("/get_token", restObj.getTokenForm).Methods("GET")
	router.HandleFunc("/get_token", restObj.getToken).Methods("POST")
	router.HandleFunc("/start_upload", restObj.startUpload).Methods("GET")
	router.HandleFunc("/upload1", restObj.upload1).Methods("GET")
	router.HandleFunc("/download/{fileName}", restObj.downloadFile).Methods("GET")
	router.HandleFunc("/operation/status/all", restObj.allOperationStatus).Methods("GET")
	router.HandleFunc("/load-to-ha/{fileName}", restObj.uploadFileToHa).Methods("POST")
	router.HandleFunc("/delete-from-yd/{fileName}", restObj.deleteFromYd).Methods("DELETE")
	router.HandleFunc("/delete-from-ha/{slug}", restObj.deleteFromHa).Methods("DELETE")

	router.HandleFunc("/{path1}/{path2}/{path3}", restObj.notFoundHandler)
	router.HandleFunc("/{path1}/{path2}", restObj.notFoundHandler)
	router.HandleFunc("/{path}", restObj.notFoundHandler)

	logger.ErrorLog.Printf("(It is not error!!!) Run WEB-Server on http://127.0.0.1:%s", port)

	return &restObj, nil
}

func (rest *Rest) Start() error {
	return http.ListenAndServe(":"+rest.port, rest.router)
}

func (app *Rest) isUseDarkTheme() bool {
	return app.theme == "Dark"
}

// Обработчик для несовпадающих маршрутов
func (app *Rest) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	url := r.URL.Path

	// Логируем URL и заголовки
	app.logger.InfoLog.Printf("Unhandled request. Method: %s URL requested: %s", method, url)
	app.logHeaders(r)

	// Возвращаем 404 ответ
	http.Error(w, "404 Handler Not Found", http.StatusNotFound)
}

// Функция для логирования заголовков
func (app *Rest) logHeaders(r *http.Request) {
	for key, values := range r.Header {
		for _, value := range values {
			app.logger.DebugLog.Printf("Header: %s = %s", key, value)
		}
	}
}

func (app *Rest) ensureLoadAddonIcons() {
	if len(app.icons) == 0 {
		app.logger.DebugLog.Printf("Load addon icons")
		app.reloadAddonIcons()
	} else {
		app.logger.DebugLog.Printf("Addon icons already loaded")
	}
}

func (app *Rest) reloadAddonIcons() {
	newMap := make(map[string]string)

	addons, err := app.haApi.GetAddonList()
	if err != nil {
		app.logger.ErrorLog.Printf("Can not reload icons. %v", err)
		return
	}

	for _, addonInfo := range addons.Addons {
		if addonInfo.Icon {
			icon, err := app.haApi.SaveAddonIcon(addonInfo.Slug)
			if err != nil {
				app.logger.ErrorLog.Printf("Error when save icon for addon %v. %v", addonInfo.Slug, err)
			} else {
				newMap[addonInfo.Slug] = icon
			}
		}
	}

	app.icons = newMap

}
func (app *Rest) indexHandler(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("indexHandler")
	files := []string{
		"./internal/pkg/rest/ui/html/index.html",
		"./internal/pkg/rest/ui/html/base.html",
	}

	app.ensureLoadAddonIcons()

	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	if app.yaDProcessor.IsTokenEmpty() {
		alertMessages = append(alertMessages, AlertMessage{Message: "Token does not exists"})
	} else if !app.yaDProcessor.IsTokenValid() {
		alertMessages = append(alertMessages, AlertMessage{Message: "Token is not valid or expired"})
	}

	app.yaDProcessor.RefreshTokenIsNeed()
	filesInfo, err := app.bKProcessor.GetFilesInfo()
	if err != nil {
		alertMessages = append(alertMessages, AlertMessage{Message: err.Error()})
	}

	// Calculate data for entity state
	localFileSize := types.FileSize(0)
	remoteFileSize := types.FileSize(0)
	localFiles := 0
	remoteFiles := 0

	// Get file sizes for start state
	for _, file := range filesInfo {
		if file.IsRemote {
			remoteFileSize += file.GeneralInfo.Size
			remoteFiles++
		}

		if file.IsLocal {
			localFileSize += file.GeneralInfo.Size
			localFiles++
		}
	}

	// Get disk info
	diskInfo, err := app.yaDProcessor.GetDiskInfo()
	if err != nil {
		app.logger.ErrorLog.Printf("Error get disk info %s", err)
	}

	// Update entity state
	entityState, err := app.haApi.GetEntityState()
	if err != nil {
		app.logger.ErrorLog.Printf("Error read state entity %s", err)
	}

	if entityState == nil {
		entityState = &haoperate.EntityState{
			State:           haoperate.OK,
			LocalFiles:      localFiles,
			RemoteFiles:     remoteFiles,
			LocalSize:       localFileSize,
			RemoteSize:      remoteFileSize,
			RemoteFreeSpace: diskInfo.TotalSpace - diskInfo.UsedSpace,
		}
	} else {
		entityState.LocalFiles = localFiles
		entityState.RemoteFiles = remoteFiles
		entityState.LocalSize = localFileSize
		entityState.RemoteSize = remoteFileSize
		entityState.RemoteFreeSpace = diskInfo.TotalSpace - diskInfo.UsedSpace
	}

	err = app.haApi.SetEntityState(
		*entityState)
	if err != nil {
		app.logger.ErrorLog.Printf("Error save entity state %s", err)
	}

	app.logger.DebugLog.Printf("%+v\n", filesInfo)

	data := BackupResponse{BFiles: filesInfo,
		AlertMessages: alertMessages,
		IsDarkTheme:   app.isUseDarkTheme(),
		AddonIcons:    app.icons}

	err = ts.Execute(w, data)
	if err != nil {
		app.logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Rest) getTokenForm(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("getTokenForm")
	app.renderTokenForm(w, r, "")
}
func (app *Rest) renderTokenForm(w http.ResponseWriter, r *http.Request, errorMessage string) {
	app.logger.InfoLog.Println("getTokenForm")
	files := []string{
		"./internal/pkg/rest/ui/html/get_token.html",
		"./internal/pkg/rest/ui/html/base.html",
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	if errorMessage != "" {
		alertMessages = append(alertMessages, AlertMessage{Message: errorMessage})
	}

	data := GetTokenResponse{CheckCodeUrl: app.yaDProcessor.GetCheckCodeUrl(),
		AlertMessages: alertMessages,
		IsDarkTheme:   app.isUseDarkTheme()}
	err = ts.Execute(w, data)
	if err != nil {
		app.logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Rest) getToken(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("getToken")
	checkCode := r.PostFormValue("check_code")
	if checkCode == "" {
		app.renderTokenForm(w, r, "Check code is required!")
	} else {
		_, err := app.yaDProcessor.CreateToken(
			r.PostFormValue("check_code"))
		if err != nil {
			app.logger.ErrorLog.Printf("Get token error. %v", err.Error())
			http.Error(w, "Create TokenInfo Error", 500)
		}

		app.yaDProcessor.EnsureYandexDisk()
		uri := r.Header.Get("X-Ingress-Path")
		http.Redirect(w, r, uri+"/", http.StatusSeeOther)
	}
}

func (app *Rest) startUpload(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("startUpload")
	files := []string{
		"./internal/pkg/rest/ui/html/start_upload.html",
		"./internal/pkg/rest/ui/html/base.html",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	data := GetTokenResponse{CheckCodeUrl: app.yaDProcessor.GetCheckCodeUrl(),
		AlertMessages: alertMessages,
		IsDarkTheme:   app.isUseDarkTheme()}

	err = ts.Execute(w, data)
	if err != nil {
		app.logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Rest) upload1(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("upload1")
	UploadTask(app)
	uri := r.Header.Get("X-Ingress-Path")
	http.Redirect(w, r, uri+"/", http.StatusSeeOther)
}

func (app *Rest) allOperationStatus(w http.ResponseWriter, r *http.Request) {
	app.logger.DebugLog.Println("allOperationStatus")
	w.Header().Set("Content-Type", "application/json")
	operations := app.operationManager.GetAllOperations()

	if app.logger.IsDebugEnabled() {
		for _, operation := range operations {
			app.logger.DebugLog.Printf("Operation: %+v", operation)
		}
	}

	if err := json.NewEncoder(w).Encode(operations); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *Rest) uploadFileToHa(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("uploadToHa")
	vars := mux.Vars(r)
	fileName, ok := vars["fileName"]
	if !ok {
		app.logger.ErrorLog.Printf("fileName is missing in parameters")
	}

	operationId := r.Header.Get(headerYbaOperationId)

	if operationId == "" {
		operationId = "emptyOperationId"
	}

	innerUploadFile(app, fileName, operationId)
	w.WriteHeader(http.StatusOK)

}
func (app *Rest) deleteFromHa(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("deleteFromHa")
	vars := mux.Vars(r)
	fileName, ok := vars["slug"]
	if !ok {
		app.logger.ErrorLog.Printf("slug is missing in parameters")
	}

	operationId := r.Header.Get(headerYbaOperationId)

	if operationId == "" {
		operationId = "emptyOperationId"
	}

	err := innerDeleteFileFromHa(app, fileName, operationId)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else {
		w.WriteHeader(http.StatusOK)
	}

}
func (app *Rest) deleteFromYd(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("deleteFromYd")
	vars := mux.Vars(r)
	fileName, ok := vars["fileName"]
	if !ok {
		app.logger.ErrorLog.Printf("fileName is missing in parameters")
	}

	operationId := r.Header.Get(headerYbaOperationId)

	if operationId == "" {
		operationId = "emptyOperationId"
	}

	err := innerDeleteFileFromYd(app, fileName, operationId)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else {
		w.WriteHeader(http.StatusOK)
	}

}

func innerDeleteFileFromHa(app *Rest, slug, id string) error {

	app.operationManager.StartOperation(id, "delete file")
	app.operationManager.ChangeStatusAndProgress(id, "delete file", 10)
	//app.yaDProcessor.EnsureYandexDisk()
	err := app.haApi.DeleteBackup(slug)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when delete file %v", err)
		app.operationManager.ErrorDone(id, fmt.Sprintf("Error when delete file %v", err))
		return err
	}
	app.operationManager.SuccessDone(id)
	return nil
}
func innerDeleteFileFromYd(app *Rest, filename, id string) error {

	app.operationManager.StartOperation(id, "delete file")
	app.operationManager.ChangeStatusAndProgress(id, "delete file", 10)
	//app.yaDProcessor.EnsureYandexDisk()
	err := app.yaDProcessor.DeleteFile(filename, "", false)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when delete file %v", err)
		app.operationManager.ErrorDone(id, fmt.Sprintf("Error when delete file %v", err))
		return err
	}
	app.operationManager.SuccessDone(id)
	return nil
}
func innerUploadFile(app *Rest, filename, id string) {
	app.operationManager.StartOperation(id, "uploading to HA")

	dst := haoperate.GetTemporaryFilePath(filename + ".tar")
	app.haApi.RemoveTemporaryFile(dst)
	err := app.haApi.DeleteOldTemporaryFiles(1)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when delete old temporary files %s", err)
	}

	err = app.yaDProcessor.DownloadFile(filename, dst, id)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when download file %s", err)
		app.operationManager.ErrorDone(id, "Error download file")
		return
	}
	app.logger.InfoLog.Printf("Downloaded file %s to %s", filename, dst)

	app.operationManager.ChangeStatusAndProgress(id, "uploading to HA", 90)
	err = app.haApi.UploadBackup(dst, "slug")
	if err != nil {
		app.logger.ErrorLog.Printf("Error when upload file to HA %s", err)
		app.operationManager.ErrorDone(id, "Error upload to HA")
		return
	}
	app.haApi.RemoveTemporaryFile(dst)

	app.operationManager.SuccessDone(id)
}

func UploadTask(app *Rest) {
	app.yaDProcessor.RefreshTokenIsNeed()
	filesInfo, err := app.bKProcessor.GetFilesInfo()
	if err != nil {
		app.logger.ErrorLog.Printf("Error get backup files %s", err)
	}
	filesToUpload := app.bKProcessor.ChooseFilesToUpload(filesInfo)
	app.logger.InfoLog.Printf("Need upload %d files", len(filesToUpload))

	uploadedFileAmount := len(filesToUpload)

	uploadResult := bkoperate.ProcessedFilesResult{}
	if len(filesToUpload) > 0 {
		uploadResult, err = app.bKProcessor.UploadFiles(filesToUpload)
		if err != nil {
			app.logger.ErrorLog.Printf("Error upload files %s", err)
			uploadedFileAmount = 0
		}
	}
	filesToDelete := app.bKProcessor.ChooseFilesToDelete(filesInfo, uploadedFileAmount)

	app.logger.DebugLog.Printf("FilesToDelete %v", filesToDelete)
	deletedResult, err := app.bKProcessor.DeleteFiles(filesToDelete)

	if err != nil {
		app.logger.ErrorLog.Printf("Error delete files %s", err)
	}

	localFileSize := types.FileSize(0)
	remoteFileSize := types.FileSize(0)
	localFiles := 0
	remoteFiles := 0

	// Get new file list

	filesInfo, err = app.bKProcessor.GetFilesInfo()
	if err != nil {
		app.logger.ErrorLog.Printf("Error get backup files %s", err)
	}

	// Get file sizes for start state
	for _, file := range filesInfo {
		if file.IsRemote {
			remoteFileSize += file.GeneralInfo.Size
			remoteFiles++
		}

		if file.IsLocal {
			localFileSize += file.GeneralInfo.Size
			localFiles++
		}
	}

	// Get disk info
	diskInfo, err := app.yaDProcessor.GetDiskInfo()
	if err != nil {
		app.logger.ErrorLog.Printf("Error get disk info %s", err)
	}

	// Save entity
	state := haoperate.OK
	if uploadResult.Error > 0 || deletedResult.Error > 0 {
		state = haoperate.ERROR
	}

	// Update entity state
	entityState, err := app.haApi.GetEntityState()
	if err != nil {
		app.logger.ErrorLog.Printf("Error read entity state %s", err)
	}

	if entityState == nil {
		entityState = &haoperate.EntityState{
			State:            state,
			OkUpload:         uploadResult.Ok,
			ErrorUpload:      uploadResult.Error,
			OkDelete:         deletedResult.Ok,
			ErrorDelete:      deletedResult.Error,
			LocalFiles:       localFiles,
			RemoteFiles:      remoteFiles,
			LocalSize:        localFileSize,
			RemoteSize:       remoteFileSize,
			RemoteFreeSpace:  diskInfo.TotalSpace - diskInfo.UsedSpace,
			LastUploadedTime: haoperate.CustomTime{Time: time.Now()},
		}
	} else {
		entityState.State = state
		entityState.OkUpload = uploadResult.Ok
		entityState.ErrorUpload = uploadResult.Error
		entityState.OkDelete = deletedResult.Ok
		entityState.ErrorDelete = deletedResult.Error
		entityState.LocalFiles = localFiles
		entityState.RemoteFiles = remoteFiles
		entityState.LocalSize = localFileSize
		entityState.RemoteSize = remoteFileSize
		entityState.RemoteFreeSpace = diskInfo.TotalSpace - diskInfo.UsedSpace
		entityState.LastUploadedTime = haoperate.CustomTime{Time: time.Now()}

	}

	err = app.haApi.SetEntityState(
		*entityState)
	if err != nil {
		app.logger.ErrorLog.Printf("Error save entity state %s", err)
	}
}

func (app *Rest) downloadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileName, ok := vars["fileName"]
	if !ok {
		app.logger.ErrorLog.Printf("fileName is missing in parameters")
	}
	app.logger.DebugLog.Printf("filename: %s", fileName)
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(fileName))
	http.ServeFile(w, r, fileName)
}
