package appybg

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"log"
	"net/http"
	"os"
	"time"
	"ybg/internal/pkg/bkoperate"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	om "ybg/internal/pkg/operationmanager"
	"ybg/internal/pkg/rest"
	"ybg/internal/pkg/yadiskoperate"
)

const FILE_PATH_OPTIONS = "/data/options.json"
const restoreStateEntityInterval time.Duration = 1800
const restoreStateEntitySchedule = "*/30 * * * * *"
const clearTaskSchedule = "0 0 */6 * * *"
const operationHourDelta = 6
const oldTemporaryFileDayDelta = 6

type YbgApp struct {
	options          ApplOptions
	restObj          *rest.Rest
	haApi            *haoperate.HaApiClient
	operationManager *om.OperationManager
	logger           *mylogger.Logger
	scheduleLogLevel gocron.LogLevel
	scheduler        gocron.Scheduler
}

type ApplOptions struct {
	ClientId                       string                  `json:"client_id"`
	ClientSecret                   string                  `json:"client_secret"`
	RemotePath                     string                  `json:"remote_path"`
	RemoteMaximumFilesQuantity     int                     `json:"remote_maximum_files_quantity"`
	Schedule                       string                  `json:"schedule"`
	LogLevel                       string                  `json:"log_level"`
	Theme                          string                  `json:"theme" default:"Light"`
	EntityId                       string                  `json:"entity_id" default:"yandex_backup_state"`
	EnabledNetworkStorages         []EnabledNetworkStorage `json:"enabled_network_storages"`
	EnableUploadFromNetworkStorage bool                    `json:"upload_from_network_storage"`
}

type EnabledNetworkStorage struct {
	Name string `json:"name"`
}

func NewYbg(port string) *YbgApp {
	logFormat := log.Ldate | log.Ltime | log.Lshortfile

	debugLog := log.New(mylogger.NewNullWriter(), "DEBUG\t", logFormat)
	infoLog := log.New(mylogger.NewNullWriter(), "INFO\t", logFormat)
	errorLog := log.New(os.Stderr, "ERROR\t", logFormat)
	isDebudDisable := true

	scheduleLogLevel := gocron.LogLevelWarn

	// Test log messages

	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Options: %v", err))
	}

	if options.LogLevel == "DEBUG" {
		debugLog = log.New(os.Stdout, "DEBUG\t", logFormat)
		infoLog = log.New(os.Stdout, "INFO\t", logFormat)
		scheduleLogLevel = gocron.LogLevelDebug
		isDebudDisable = false

	}
	if options.LogLevel == "INFO" {
		infoLog = log.New(os.Stdout, "INFO\t", logFormat)
		scheduleLogLevel = gocron.LogLevelInfo
	}

	debugLog.Println("hello")
	infoLog.Println("hello")
	errorLog.Println("hello")

	// Инициализируем новую структуру с зависимостями приложения.
	logger := mylogger.New(errorLog, infoLog, debugLog)
	if isDebudDisable {
		logger.DisableDebug()
	}

	operationManager := om.New(logger)

	haApi, err := createHaApiClient(logger, options.EntityId)
	if err != nil {
		logger.ErrorLog.Printf("Error create HaApiClient %v", err)
		//panic(fmt.Sprintf("error create HaApiClient %v", err))
	}

	yaDP := yadiskoperate.NewYaDProcessor(options.ClientId, options.ClientSecret, options.RemotePath, operationManager, logger)

	enabledNetworkStorages := make([]string, len(options.EnabledNetworkStorages))

	for i, element := range options.EnabledNetworkStorages {
		enabledNetworkStorages[i] = element.Name
	}

	bkP := bkoperate.NewBkProcessor(yaDP, haApi, options.RemoteMaximumFilesQuantity, options.EnableUploadFromNetworkStorage, enabledNetworkStorages, logger)

	yaDP.EnsureTokenInfo()
	yaDP.RefreshTokenIsNeed()
	yaDP.EnsureYandexDisk()

	// Создаем рест
	restObj, err := rest.NewRest(port, yaDP, bkP, haApi, options.Theme, operationManager, logger)
	if err != nil {
		logger.ErrorLog.Printf("Error create Rest %v", err)
		panic(fmt.Sprintf("error create Rest %v", err))
	}

	return &YbgApp{
		options:          options,
		scheduleLogLevel: scheduleLogLevel,
		logger:           logger,
		restObj:          restObj,
		haApi:            haApi,
		operationManager: operationManager}
}

func (app *YbgApp) Start() {

	scheduler, err := gocron.NewScheduler(gocron.WithLocation(time.UTC),
		gocron.WithLogger(
			gocron.NewLogger(app.scheduleLogLevel),
		))
	app.scheduler = scheduler

	// Upload backup task

	_, err = app.scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			app.options.Schedule,
			false,
		),
		gocron.NewTask(
			func() { rest.UploadTask(app.restObj) },
		),
	)

	if err != nil {
		app.logger.ErrorLog.Printf("Error when create upload task job. %v", err)
	}
	app.logger.InfoLog.Printf("Add upload job for to %s schedule", app.options.Schedule)

	// Restore HA entitystate task
	_, err = app.scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			restoreStateEntitySchedule,
			true,
		),
		gocron.NewTask(
			func() {
				err := app.haApi.EnsureEntityState()
				if err != nil {
					app.logger.ErrorLog.Printf("Error when restore entity state. %v", err)
				}
			},
		),
	)

	if err != nil {
		app.logger.ErrorLog.Printf("Error when create restore state entity task job. %v", err)
	}
	app.logger.InfoLog.Printf("Add restore state entity job for to %s schedule (cron with seconds!!!)", restoreStateEntitySchedule)

	// Clear task
	_, err = app.scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			clearTaskSchedule,
			true,
		),
		gocron.NewTask(
			func() {
				err := app.operationManager.ClearOperations(operationHourDelta)
				if err != nil {
					app.logger.ErrorLog.Printf("Error when clear operation. %v", err)
				}
				err = app.haApi.DeleteOldTemporaryFiles(oldTemporaryFileDayDelta)
				if err != nil {
					app.logger.ErrorLog.Printf("Error when delete old temporary files %s", err)
				}
			},
		),
	)

	if err != nil {
		app.logger.ErrorLog.Printf("Error when create restore state entity task job. %v", err)
	}
	app.logger.InfoLog.Printf("Add restore state entity job for to %s schedule (cron with seconds!!!)", restoreStateEntitySchedule)

	// Запуск планировщика в отдельной горутине
	go func() {
		app.scheduler.Start()
	}()

	// Запуск восстановления EntityState
	restoreEntityTask := func() bool {
		err := app.haApi.EnsureEntityState()
		if err != nil {
			app.logger.ErrorLog.Printf("Error restore state entity %v", err)
			return false
		}
		return true
	}

	await(restoreEntityTask, app.logger, restoreStateEntityInterval)

	list, err := app.haApi.GetAddonList()
	if err != nil {
		app.logger.ErrorLog.Printf("Error read addons %v", err)
	} else {
		app.logger.InfoLog.Printf("Addons: %v", list)
	}

	err = app.restObj.Start()
	log.Fatal(err)
}

func (app *YbgApp) Stop() {
	_ = app.scheduler.Shutdown()
}

func await(task func() bool,
	logger *mylogger.Logger,
	timeIntervalSec time.Duration) {
	cutOfTime := time.Now().Add(timeIntervalSec * time.Second)

	go func() {
		result := false
		for {
			if time.Now().After(cutOfTime) {
				logger.ErrorLog.Printf("Task completed unsuccessfully by cut of time")
				break
			}
			result = task()
			if result {
				logger.InfoLog.Printf("Task completed successfully")
				break
			}
			logger.ErrorLog.Printf("Task iteration unsuccessfully")
			time.Sleep(5 * time.Second)
		}
	}()
}

func readOptions() (ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data ApplOptions
	err := json.Unmarshal(plan, &data)
	return data, err
}

func createHaApiClient(logger *mylogger.Logger, entity_id string) (*haoperate.HaApiClient, error) {

	supervisorToken := os.Getenv("SUPERVISOR_TOKEN")
	if supervisorToken == "" {
		logger.ErrorLog.Println("Supervisor token not found")
		return nil, fmt.Errorf("supervisor token not found")
	}

	api, err := haoperate.NewHaApi(entity_id, context.Background(), http.DefaultClient, supervisorToken, logger)
	if err != nil {
		logger.ErrorLog.Printf("Error when create ha_api client: %v", err)
		return nil, fmt.Errorf("error when create ha_api client: %v", err)
	}
	return api, nil
}
