package rest

import (
	"github.com/gorilla/mux"
	"html/template"
	"net/http"
	"path/filepath"
	"time"
	"ybg/internal/pkg/bkoperate"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/pkg/yadiskoperate"
	"ybg/internal/types"
)

type AlertMessage struct {
	Message string
}
type BackupResponse struct {
	IsDarkTheme   bool
	AlertMessages []AlertMessage
	BFiles        []types.BackupFileInfo
}
type GetTokenResponse struct {
	IsDarkTheme   bool
	AlertMessages []AlertMessage
	CheckCodeUrl  string
}

type Rest struct {
	logger       *mylogger.Logger
	TokenInfo    types.TokenInfo
	yaDProcessor *yadiskoperate.YaDProcessor
	bKProcessor  *bkoperate.BkProcessor
	haApi        *haoperate.HaApiClient
	router       *mux.Router
	port         string
	theme        string
}

func NewRest(port string,
	yaDProcessor *yadiskoperate.YaDProcessor,
	bKProcessor *bkoperate.BkProcessor,
	haApi *haoperate.HaApiClient,
	theme string,
	logger *mylogger.Logger) (*Rest, error) {

	router := mux.NewRouter()
	fileServer := http.FileServer(http.Dir("./internal/pkg/rest/ui/static/"))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fileServer))
	restObj := Rest{port: port,
		yaDProcessor: yaDProcessor,
		bKProcessor:  bKProcessor,
		haApi:        haApi,
		theme:        theme,
		router:       router,
		logger:       logger}

	router.HandleFunc("/", restObj.indexHandler).Methods("GET")
	router.HandleFunc("/index", restObj.indexHandler).Methods("GET")
	router.HandleFunc("/get_token", restObj.getTokenForm).Methods("GET")
	router.HandleFunc("/get_token", restObj.getToken).Methods("POST")
	router.HandleFunc("/start_upload", restObj.startUpload).Methods("GET")
	router.HandleFunc("/upload1", restObj.upload1).Methods("GET")
	router.HandleFunc("/download/{fileName}", restObj.downloadFile).Methods("GET")

	logger.ErrorLog.Printf("(It is not error!!!) Run WEB-Server on http://127.0.0.1:%s", port)

	return &restObj, nil
}

func (rest *Rest) Start() error {
	return http.ListenAndServe(":"+rest.port, rest.router)
}

func (app *Rest) isUseDarkTheme() bool {
	return app.theme == "Dark"
}

func (app *Rest) indexHandler(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("indexHandler")
	files := []string{
		"./internal/pkg/rest/ui/html/index.html",
		"./internal/pkg/rest/ui/html/base.html",
	}

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
		app.logger.ErrorLog.Printf("Error read entity state %s", err)
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

	data := BackupResponse{BFiles: filesInfo, AlertMessages: alertMessages, IsDarkTheme: app.isUseDarkTheme()}

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

// UploadTask TODO Может перенести в bk_processor
func UploadTask(app *Rest) {
	app.yaDProcessor.RefreshTokenIsNeed()
	filesInfo, err := app.bKProcessor.GetFilesInfo()
	if err != nil {
		app.logger.ErrorLog.Printf("Error get backup files %s", err)
	}
	filesToUpload := bkoperate.ChooseFilesToUpload(filesInfo)
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
