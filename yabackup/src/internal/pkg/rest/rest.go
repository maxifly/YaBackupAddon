package rest

import (
	"github.com/gorilla/mux"
	"html/template"
	"net/http"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/pkg/yadiskoperate"
	"ybg/internal/types"
)

type AlertMessage struct {
	Message string
}

type Rest struct {
	logger       *mylogger.Logger
	TokenInfo    types.TokenInfo
	yaDProcessor *yadiskoperate.YaDProcessor
	haApi        *haoperate.HaApiClient
	router       *mux.Router
	port         string
}

func NewRest(port string,
	yaDProcessor *yadiskoperate.YaDProcessor,
	haApi *haoperate.HaApiClient,
	logger *mylogger.Logger) (*Rest, error) {

	router := mux.NewRouter()
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fileServer))
	restObj := Rest{port: port, yaDProcessor: yaDProcessor, haApi: haApi, logger: logger}

	router.HandleFunc("/", restObj.indexHandler).Methods("GET")
	//router.HandleFunc("/index", app.indexHandler).Methods("GET")
	//router.HandleFunc("/get_token", app.getTokenForm).Methods("GET")
	//router.HandleFunc("/get_token", app.getToken).Methods("POST")
	//router.HandleFunc("/start_upload", app.startUpload).Methods("GET")
	//router.HandleFunc("/upload1", app.upload1).Methods("GET")
	//router.HandleFunc("/download/{fileName}", app.downloadFile).Methods("GET")

	logger.ErrorLog.Printf("(It is not error!!!) Run WEB-Server on http://127.0.0.1:%s", port)

	return &restObj, nil
}

func (rest *Rest) Start() error {
	return http.ListenAndServe(":"+rest.port, rest.router)
}

func (app *Rest) indexHandler(w http.ResponseWriter, r *http.Request) {
	app.logger.InfoLog.Println("indexHandler")
	files := []string{
		"./ui/html/index.html",
		"./ui/html/base.html",
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
