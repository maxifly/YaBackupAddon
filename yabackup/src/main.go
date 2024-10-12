package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"github.com/gorilla/mux"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"ybg/internal/haoperate"
	"ybg/internal/maintypes"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

const FILE_PATH_OPTIONS = "/data/options.json"
const FILE_PATH_TOKEN = "/data/tokenInfo.json"
const BACKUP_PATH = "/backup"

type Application struct {
	appData *maintypes.AppData
}

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
type StartUploadResponse struct {
	AlertMessages []AlertMessage
}

func (app *Application) indexHandler(w http.ResponseWriter, r *http.Request) {
	app.appData.Logger.InfoLog.Println("indexHandler")
	files := []string{
		"./ui/html/index.html",
		"./ui/html/base.html",
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.appData.Logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	if isTokenEmpty(app.appData.TokenInfo) {
		alertMessages = append(alertMessages, AlertMessage{Message: "Token does not exists"})
	} else if !isTokenValid(app.appData.TokenInfo) {
		alertMessages = append(alertMessages, AlertMessage{Message: "Token is not valid or expired"})
	}

	app.refreshTokenIsNeed()
	filesInfo, err := GetFilesInfo(app.appData)
	if err != nil {
		alertMessages = append(alertMessages, AlertMessage{Message: err.Error()})
	}

	data := BackupResponse{BFiles: filesInfo, AlertMessages: alertMessages, IsDarkTheme: app.appData.Options.IsUseDarkTheme()}

	err = ts.Execute(w, data)
	if err != nil {
		app.appData.Logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Application) getTokenForm(w http.ResponseWriter, r *http.Request) {
	app.appData.Logger.InfoLog.Println("getTokenForm")
	app.renderTokenForm(w, r, "")
}
func (app *Application) renderTokenForm(w http.ResponseWriter, r *http.Request, errorMessage string) {
	app.appData.Logger.InfoLog.Println("getTokenForm")
	files := []string{
		"./ui/html/get_token.html",
		"./ui/html/base.html",
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.appData.Logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	if errorMessage != "" {
		alertMessages = append(alertMessages, AlertMessage{Message: errorMessage})
	}

	data := GetTokenResponse{CheckCodeUrl: GetCheckCodeUrl(app.appData.Options.ClientId), AlertMessages: alertMessages, IsDarkTheme: app.appData.Options.IsUseDarkTheme()}
	err = ts.Execute(w, data)
	if err != nil {
		app.appData.Logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Application) getToken(w http.ResponseWriter, r *http.Request) {
	app.appData.Logger.InfoLog.Println("getToken")
	checkCode := r.PostFormValue("check_code")
	if checkCode == "" {
		app.renderTokenForm(w, r, "Check code is required!")
	} else {
		tokenInfo, err := CreateToken(
			app.appData.Options.ClientId,
			app.appData.Options.ClientSecret,
			r.PostFormValue("check_code"))
		if err != nil {
			app.appData.Logger.ErrorLog.Printf("Get token error. %v", err.Error())
			http.Error(w, "Create TokenInfo Error", 500)
		}
		app.appData.Logger.DebugLog.Printf("Create token success")
		err = writeToken(tokenInfo)
		if err == nil {
			app.appData.Logger.DebugLog.Printf("Write token success.")
			app.appData.TokenInfo = tokenInfo
		} else {
			app.appData.Logger.ErrorLog.Printf("Save token error. %v", err)
		}

		app.ensureYandexDisk()
		uri := r.Header.Get("X-Ingress-Path")
		http.Redirect(w, r, uri+"/", http.StatusSeeOther)
	}
}

func (app *Application) startUpload(w http.ResponseWriter, r *http.Request) {
	app.appData.Logger.InfoLog.Println("startUpload")
	files := []string{
		"./ui/html/start_upload.html",
		"./ui/html/base.html",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.appData.Logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	data := GetTokenResponse{CheckCodeUrl: GetCheckCodeUrl(app.appData.Options.ClientId), AlertMessages: alertMessages, IsDarkTheme: app.appData.Options.IsUseDarkTheme()}

	err = ts.Execute(w, data)
	if err != nil {
		app.appData.Logger.ErrorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Application) upload1(w http.ResponseWriter, r *http.Request) {
	app.appData.Logger.InfoLog.Println("upload1")
	UploadTask(app)
	uri := r.Header.Get("X-Ingress-Path")
	http.Redirect(w, r, uri+"/", http.StatusSeeOther)
}

func UploadTask(app *Application) {
	app.refreshTokenIsNeed()
	filesInfo, err := GetFilesInfo(app.appData)
	if err != nil {
		app.appData.Logger.ErrorLog.Printf("Error get backup files %s", err)
	}
	filesToUpload := ChooseFilesToUpload(app.appData, filesInfo)
	uploadedFileAmount := len(filesToUpload)

	uploadResult := ProcessedFilesResult{}
	if len(filesToUpload) > 0 {
		uploadResult, err = UploadFiles(app.appData, filesToUpload)
		if err != nil {
			app.appData.Logger.ErrorLog.Printf("Error upload files %s", err)
			uploadedFileAmount = 0
		}
	}
	filesToDelete := ChooseFilesToDelete(app.appData, filesInfo, uploadedFileAmount)

	app.appData.Logger.DebugLog.Printf("FilesToDelete %v", filesToDelete)
	deletedResult, err := DeleteFiles(app.appData, filesToDelete)

	if err != nil {
		app.appData.Logger.ErrorLog.Printf("Error delete files %s", err)
	}

	localFileSize := types.FileSize(0)
	remoteFileSize := types.FileSize(0)

	// Get file sizes for start state
	for _, file := range filesInfo {
		if file.IsRemote {
			remoteFileSize += file.GeneralInfo.Size
		}

		if file.IsLocal {
			localFileSize += file.GeneralInfo.Size
		}
	}

	// Calculate file sizes for end state
	remoteFileSize = remoteFileSize - deletedResult.ProcessedSize + uploadResult.ProcessedSize

	// Get disk info
	diskInfo, err := getDiskInfo(app.appData)
	if err != nil {
		app.appData.Logger.ErrorLog.Printf("Error get disk info %s", err)
	}

	// Save entity
	state := haoperate.OK
	if uploadResult.Error > 0 || deletedResult.Error > 0 {
		state = haoperate.ERROR
	}

	err = app.appData.HaApi.SetEntityState(
		haoperate.EntityState{
			State:            state,
			OkUpload:         uploadResult.Ok,
			ErrorUpload:      uploadResult.Error,
			OkDelete:         deletedResult.Ok,
			ErrorDelete:      deletedResult.Error,
			LocalSize:        localFileSize,
			RemoteSize:       remoteFileSize,
			RemoteFreeSpace:  diskInfo.TotalSpace - diskInfo.UsedSpace,
			LastUploadedTime: haoperate.CustomTime{Time: time.Now()},
		})
	if err != nil {
		app.appData.Logger.ErrorLog.Printf("Error save entity state %s", err)
	}
}

func readOptions() (maintypes.ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data maintypes.ApplOptions
	err := json.Unmarshal(plan, &data)
	return data, err
}

func (app *Application) ensureYandexDisk() {
	if !isTokenEmpty(app.appData.TokenInfo) {
		disk, err := NewYandexDisk(app.appData.TokenInfo.AccessToken)
		if err != nil {
			app.appData.Logger.ErrorLog.Printf("Error when create YaDisk %v", err)
			return
		}
		app.appData.YaDisk = &disk
	}
}

func (app *Application) ensureTokenInfo() {
	if isTokenEmpty(app.appData.TokenInfo) {
		token, err := readToken()
		if err != nil {
			app.appData.Logger.ErrorLog.Printf("Error read token info %v", err)
			return
		}
		app.appData.TokenInfo = token
	}
}

func (app *Application) ensureHaApiClient() {
	if app.appData.HaApi == nil {
		supervisorToken := os.Getenv("SUPERVISOR_TOKEN")
		if supervisorToken == "" {
			app.appData.Logger.ErrorLog.Println("Supervisor token not found")
			return
		}

		api, err := haoperate.NewHaApi(context.Background(), http.DefaultClient, supervisorToken, app.appData.Logger)
		if err != nil {
			app.appData.Logger.ErrorLog.Printf("Error when create ha_api client: %v", err)
			return
		}
		fmt.Printf("%+v\n", api)
		app.appData.HaApi = api
	}
}

func (app *Application) refreshTokenIsNeed() bool {
	if app.appData.TokenInfo.Expiry.After(time.Now().Add(time.Duration(240) * time.Hour)) {
		app.appData.Logger.DebugLog.Printf("Not need refresh token")
		return false
	}

	tokenInfo, err := RefreshToken(app.appData.Options.ClientId, app.appData.Options.ClientSecret, app.appData.TokenInfo)
	if err != nil {
		app.appData.Logger.ErrorLog.Printf("Error when refresh token %v", err)
		return false
	}
	app.appData.Logger.InfoLog.Printf("%+v", tokenInfo)

	err = writeToken(*tokenInfo)
	if err != nil {
		app.appData.Logger.ErrorLog.Printf("Error when write token %v", err)
		return false
	}

	app.appData.TokenInfo = *tokenInfo
	app.appData.Logger.InfoLog.Printf("Refresh token done")
	app.ensureYandexDisk()
	return true
}

func (app *Application) downloadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileName, ok := vars["fileName"]
	if !ok {
		app.appData.Logger.ErrorLog.Printf("fileName is missing in parameters")
	}
	app.appData.Logger.DebugLog.Printf("filename: %s", fileName)
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(fileName))
	http.ServeFile(w, r, fileName)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	debugLog := log.New(mylogger.NewNullWriter(), "DEBUG\t", log.Ldate|log.Ltime|log.Lshortfile)
	infoLog := log.New(mylogger.NewNullWriter(), "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	scheduleLogLevel := gocron.LogLevelWarn

	// Test log messages

	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Options: %v", err))
	}

	if options.LogLevel == "DEBUG" {
		debugLog = log.New(os.Stdout, "DEBUG\t", log.Ldate|log.Ltime|log.Lshortfile)
		infoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
		scheduleLogLevel = gocron.LogLevelDebug

	}
	if options.LogLevel == "INFO" {
		infoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
		scheduleLogLevel = gocron.LogLevelInfo
	}

	debugLog.Println("hello")
	infoLog.Println("hello")
	errorLog.Println("hello")

	// Инициализируем новую структуру с зависимостями приложения.
	logger := mylogger.Logger{ErrorLog: errorLog,
		InfoLog:  infoLog,
		DebugLog: debugLog}

	appData := &maintypes.AppData{
		Options: options,
		Logger:  &logger,
	}

	app := &Application{
		appData: appData,
	}

	router := mux.NewRouter()
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fileServer))

	router.HandleFunc("/", app.indexHandler).Methods("GET")
	router.HandleFunc("/index", app.indexHandler).Methods("GET")
	router.HandleFunc("/get_token", app.getTokenForm).Methods("GET")
	router.HandleFunc("/get_token", app.getToken).Methods("POST")
	router.HandleFunc("/start_upload", app.startUpload).Methods("GET")
	router.HandleFunc("/upload1", app.upload1).Methods("GET")
	router.HandleFunc("/download/{fileName}", app.downloadFile).Methods("GET")

	errorLog.Printf("(It is not error!!!) Run WEB-Server on http://127.0.0.1:%s", port)

	app.ensureTokenInfo()
	app.refreshTokenIsNeed()
	app.ensureYandexDisk()
	app.ensureHaApiClient()

	scheduler, err := gocron.NewScheduler(gocron.WithLocation(time.UTC),
		gocron.WithLogger(
			gocron.NewLogger(scheduleLogLevel),
		))
	defer func() { _ = scheduler.Shutdown() }()

	_, err = scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			app.appData.Options.Schedule,
			false,
		),
		gocron.NewTask(
			func() { UploadTask(app) },
		),
	)

	if err != nil {
		errorLog.Printf("Error when create upload task job. %v", err)
	}
	infoLog.Printf("Add job for %s schedule", app.appData.Options.Schedule)
	scheduler.Start()

	err = http.ListenAndServe(":"+port, router)
	log.Fatal(err)
}
