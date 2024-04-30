package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"github.com/gorilla/mux"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const FILE_PATH_OPTIONS = "/data/options.json"
const FILE_PATH_TOKEN = "/data/tokenInfo.json"
const BACKUP_PATH = "/backup"

type ApplOptions struct {
	ClientId                   string `json:"client_id"`
	ClientSecret               string `json:"client_secret"`
	RemotePath                 string `json:"remote_path"`
	RemoteMaximumFilesQuantity int    `json:"remote_maximum_files_quantity"`
	Schedule                   string `json:"schedule"`
	LogLevel                   string `json:"log_level"`
	Theme                      string `json:"theme" default:"Light"`
}

type Application struct {
	errorLog  *log.Logger
	infoLog   *log.Logger
	debugLog  *log.Logger
	options   ApplOptions
	tokenInfo TokenInfo
	yaDisk    *yadisk.YaDisk
}

type AlertMessage struct {
	Message string
}
type BackupResponse struct {
	IsDarkTheme   bool
	AlertMessages []AlertMessage
	BFiles        []BackupFileInfo
}

type GetTokenResponse struct {
	IsDarkTheme   bool
	AlertMessages []AlertMessage
	CheckCodeUrl  string
}
type StartUploadResponse struct {
	AlertMessages []AlertMessage
}

func (ao *ApplOptions) IsUseDarkTheme() bool {
	return ao.Theme == "Dark"
}

func (app *Application) indexHandler(w http.ResponseWriter, r *http.Request) {
	app.infoLog.Println("indexHandler")
	files := []string{
		"./ui/html/index.html",
		"./ui/html/base.html",
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.errorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	if isTokenEmpty(app.tokenInfo) {
		alertMessages = append(alertMessages, AlertMessage{Message: "Token does not exists"})
	} else if !isTokenValid(app.tokenInfo) {
		alertMessages = append(alertMessages, AlertMessage{Message: "Token is not valid or expired"})
	}

	app.refreshTokenIsNeed()
	filesInfo, err := GetFilesInfo(app)
	if err != nil {
		alertMessages = append(alertMessages, AlertMessage{Message: err.Error()})
	}

	data := BackupResponse{BFiles: filesInfo, AlertMessages: alertMessages, IsDarkTheme: app.options.IsUseDarkTheme()}

	err = ts.Execute(w, data)
	if err != nil {
		app.errorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Application) getTokenForm(w http.ResponseWriter, r *http.Request) {
	app.infoLog.Println("getTokenForm")
	app.renderTokenForm(w, r, "")
}
func (app *Application) renderTokenForm(w http.ResponseWriter, r *http.Request, errorMessage string) {
	app.infoLog.Println("getTokenForm")
	files := []string{
		"./ui/html/get_token.html",
		"./ui/html/base.html",
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.errorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	if errorMessage != "" {
		alertMessages = append(alertMessages, AlertMessage{Message: errorMessage})
	}

	data := GetTokenResponse{CheckCodeUrl: GetCheckCodeUrl(app.options.ClientId), AlertMessages: alertMessages, IsDarkTheme: app.options.IsUseDarkTheme()}
	err = ts.Execute(w, data)
	if err != nil {
		app.errorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Application) getToken(w http.ResponseWriter, r *http.Request) {
	app.infoLog.Println("getToken")
	checkCode := r.PostFormValue("check_code")
	if checkCode == "" {
		app.renderTokenForm(w, r, "Check code is required!")
	} else {
		tokenInfo, err := CreateToken(
			app.options.ClientId,
			app.options.ClientSecret,
			r.PostFormValue("check_code"))
		if err != nil {
			app.errorLog.Printf("Get token error. %v", err.Error())
			http.Error(w, "Create TokenInfo Error", 500)
		}
		app.debugLog.Printf("Create token success")
		err = writeToken(tokenInfo)
		if err == nil {
			app.debugLog.Printf("Write token success.")
			app.tokenInfo = tokenInfo
		} else {
			app.errorLog.Printf("Save token error. %v", err)
		}

		app.ensureYandexDisk()
		uri := r.Header.Get("X-Ingress-Path")
		http.Redirect(w, r, uri+"/", http.StatusSeeOther)
	}
}

func (app *Application) startUpload(w http.ResponseWriter, r *http.Request) {
	app.infoLog.Println("startUpload")
	files := []string{
		"./ui/html/start_upload.html",
		"./ui/html/base.html",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.errorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
		return
	}

	alertMessages := make([]AlertMessage, 0)
	data := GetTokenResponse{CheckCodeUrl: GetCheckCodeUrl(app.options.ClientId), AlertMessages: alertMessages, IsDarkTheme: app.options.IsUseDarkTheme()}

	err = ts.Execute(w, data)
	if err != nil {
		app.errorLog.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}

func (app *Application) upload1(w http.ResponseWriter, r *http.Request) {
	app.infoLog.Println("upload1")
	UploadTask(app)
	uri := r.Header.Get("X-Ingress-Path")
	http.Redirect(w, r, uri+"/", http.StatusSeeOther)
}

func UploadTask(app *Application) {
	app.refreshTokenIsNeed()
	filesInfo, err := GetFilesInfo(app)
	if err != nil {
		app.errorLog.Printf("Error get backup files %s", err)
	}
	filesToUpload := ChooseFilesToUpload(app, filesInfo)
	uploadedFileAmount := len(filesToUpload)

	if len(filesToUpload) > 0 {
		err := UploadFiles(app, filesToUpload)
		if err != nil {
			app.errorLog.Printf("Error upload files %s", err)
			uploadedFileAmount = 0
		}
	}
	filesToDelete := ChooseFilesToDelete(app, filesInfo, uploadedFileAmount)
	app.debugLog.Printf("FilesToDelete %v", filesToDelete)
	err = DeleteFiles(app, filesToDelete)
	if err != nil {
		app.errorLog.Printf("Error delete files %s", err)
		uploadedFileAmount = 0
	}
}

func readOptions() (ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data ApplOptions
	err := json.Unmarshal(plan, &data)
	return data, err
}

func (app *Application) ensureYandexDisk() {
	if !isTokenEmpty(app.tokenInfo) {
		disk, err := NewYandexDisk(app.tokenInfo.AccessToken)
		if err != nil {
			app.errorLog.Printf("Error when create YaDisk %v", err)
			return
		}
		app.yaDisk = &disk
	}
}

func (app *Application) ensureTokenInfo() {
	if isTokenEmpty(app.tokenInfo) {
		token, err := readToken()
		if err != nil {
			app.errorLog.Printf("Error read token info %v", err)
			return
		}
		app.tokenInfo = token
	}
}

func (app *Application) refreshTokenIsNeed() bool {
	if app.tokenInfo.Expiry.After(time.Now().Add(time.Duration(240) * time.Hour)) {
		app.debugLog.Printf("Not need refresh token")
		return false
	}

	tokenInfo, err := RefreshToken(app.options.ClientId, app.options.ClientSecret, app.tokenInfo)
	if err != nil {
		app.errorLog.Printf("Error when refresh token %v", err)
		return false
	}
	app.infoLog.Printf("%+v", tokenInfo)

	err = writeToken(*tokenInfo)
	if err != nil {
		app.errorLog.Printf("Error when write token %v", err)
		return false
	}

	app.tokenInfo = *tokenInfo
	app.infoLog.Printf("Refresh token done")
	app.ensureYandexDisk()
	return true
}

func (app *Application) downloadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileName, ok := vars["fileName"]
	if !ok {
		app.errorLog.Printf("fileName is missing in parameters")
	}
	app.debugLog.Printf("filename: %s", fileName)
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(fileName))
	http.ServeFile(w, r, fileName)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	debugLog := log.New(NewNullWriter(), "DEBUG\t", log.Ldate|log.Ltime|log.Lshortfile)

	infoLog := log.New(NewNullWriter(), "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	scheduleLogLevel := gocron.LogLevelWarn

	// Test log messages

	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read options: %v", err))
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
	app := &Application{
		options:  options,
		errorLog: errorLog,
		infoLog:  infoLog,
		debugLog: debugLog,
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

	scheduler, err := gocron.NewScheduler(gocron.WithLocation(time.UTC),
		gocron.WithLogger(
			gocron.NewLogger(scheduleLogLevel),
		))
	defer func() { _ = scheduler.Shutdown() }()

	_, err = scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			app.options.Schedule,
			false,
		),
		gocron.NewTask(
			func() { UploadTask(app) },
		),
	)

	if err != nil {
		errorLog.Printf("Error when create upload task job. %v", err)
	}
	infoLog.Printf("Add job for %s schedule", app.options.Schedule)
	scheduler.Start()

	err = http.ListenAndServe(":"+port, router)
	log.Fatal(err)
}
