package haoperate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

const UPLOAD_TEMP_DIR = "bfiles"
const (
	CoreBaseURL         string = "http://supervisor/core/api"
	AddonsBaseURL       string = "http://supervisor/addons"
	BackupBaseURL       string = "http://supervisor/backups"
	JobBaseURL          string = "http://supervisor/jobs"
	HostBaseURL         string = "http://supervisor/host"
	EntityIdPrefix      string = "sensor."
	DefaultEntityId     string = "yandex_backup_state"
	localEntityCopyPath string = "/data/entity-copy.json"
	addonIconPath       string = "/app/yabackup/internal/pkg/rest/ui/static/appicons"
	uploadBackup        string = "/new/upload"
)

type HaApiClient struct {
	entity_id  string
	ctx        context.Context
	httpClient *http.Client
	token      string
	logger     *mylogger.Logger
}

// Status Определяем Enum для статуса
type Status int

const (
	OK Status = iota
	ERROR
)

// Метод для возврата строкового представления статуса
func (s Status) String() string {
	return [...]string{"ok", "error"}[s]
}

// MarshalJSON Метод для сериализации статуса в JSON
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON Метод для десериализации статуса из JSON
func (s *Status) UnmarshalJSON(data []byte) error {
	var statusString string
	if err := json.Unmarshal(data, &statusString); err != nil {
		return err
	}

	// Переводим строку обратно в Enum
	switch statusString {
	case "ok":
		*s = OK
	case "error":
		*s = ERROR
	default:
		return fmt.Errorf("unknown status: %s", statusString)
	}
	return nil
}

type EntityState struct {
	State                       Status
	OkUpload                    int
	ErrorUpload                 int
	OkDelete                    int
	ErrorDelete                 int
	LocalFiles                  int
	RemoteFiles                 int
	LocalSize                   types.FileSize
	RemoteSize                  types.FileSize
	RemoteFreeSpace             types.FileSize
	LastUploadedTime            CustomTime
	LastCreateBackupTime        CustomTime
	LastCreateBackupErrorTime   CustomTime
	LastCreateBackupWithError   bool
	LastCreateBacupErrorMessage string
}

type Addon struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Icon bool   `json:"icon"`
}

type Addons struct {
	Addons []Addon `json:"addons"`
}

type getAddonsResult struct {
	Result string  `json:"result"`
	Data   *Addons `json:"data"`
}

// CustomTime - пользовательский тип, оборачивающий time.Time
type CustomTime struct {
	time.Time
}

func CustomTimeNow() CustomTime {
	return CustomTime{Time: time.Now()}
}

type getHostInfoResult struct {
	Result string    `json:"result"`
	Data   *HostInfo `json:"data,omitempty"`
}
type HostInfo struct {
	DiskTotal float64 `json:"disk_total"`
	DiskUsed  float64 `json:"disk_used"`
	DiskFree  float64 `json:"disk_free"`
}

type FileStatistic struct {
	FilesSize  types.FileSize
	FileAmount int
}

type createFullBacupRequest struct {
	Name            string `json:"name"`
	Compressed      bool   `json:"compressed"`
	Location        string `json:"location"`
	ExcludeDatabase bool   `json:"homeassistant_exclude_database"`
	Background      bool   `json:"background"`
}

type CreateBackupResult struct {
	Slug string `json:"slug"`
	Job  string `json:"job_id"`
}

type CreateBackupResponse struct {
	Result string              `json:"result"`
	Data   *CreateBackupResult `json:"data"`
}

type JobInfo struct {
	Name      string `json:"name"`      // Name of the job
	Reference string `json:"reference"` // A unique ID for instance the job is acting on
	UUID      string `json:"uuid"`      // Unique ID of the job
	Progress  int    `json:"progress"`  // Progress of the job
	Stage     string `json:"stage"`     // A name for the stage the job is in
	Done      bool   `json:"done"`      // Is the job complete
	Created   string `json:"created"`   // Date and time when job was created in ISO format
	//ChildJobs []JobInfo    `json:"child_jobs"`  // A list of child jobs started by this one
	Errors []string `json:"errors"` // A list of errors that occurred during execution
}

type JobInfoResponse struct {
	Result string   `json:"result"`
	Data   *JobInfo `json:"data"`
}

// MarshalJSON - пользовательская сериализация для CustomTime
func (c CustomTime) MarshalJSON() ([]byte, error) {
	// Здесь указываем нужный формат
	return json.Marshal(c.Time.Format(time.RFC3339Nano))
}

// UnmarshalJSON - пользовательская десериализация для CustomTime
func (c *CustomTime) UnmarshalJSON(data []byte) error {
	// Десериализация строки в формат времени
	var timeStr string
	if err := json.Unmarshal(data, &timeStr); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return err
	}
	c.Time = t
	return nil
}

// Определяем структуру, соответствующую JSON объекту

type EntityAttributes struct {
	OkUploadAmount              int        `json:"success_upload_files"`
	ErrorUploadAmount           int        `json:"error_upload_files"`
	OkDeleteAmount              int        `json:"success_delete_files"`
	ErrorDeleteAmount           int        `json:"error_delete_files"`
	RemoteFiles                 int        `json:"remote_files"`
	LocalFiles                  int        `json:"local_files"`
	RemoteFileSize              int64      `json:"remote_file_size"`
	LocalFileSize               int64      `json:"local_file_size"`
	RemoteFreeSpace             int64      `json:"remote_free_space"`
	LastUploadTime              CustomTime `json:"last_upload_time"`
	LastCreateBackupTime        CustomTime `json:"last_create_backup_time"`
	LastCreateBackupErrorTime   CustomTime `json:"last_create_backup_error_time"`
	LastCreateBackupWithError   bool       `json:"last_create_backup_with_error_time"`
	LastCreateBacupErrorMessage string     `json:"last_create_backup_with_error_message"`
}

type setEntityStateRequest struct {
	State      Status           `json:"state"`
	Attributes EntityAttributes `json:"attributes"`
}

type getEntityStateResponse struct {
	State      Status           `json:"state"`
	Attributes EntityAttributes `json:"attributes"`
}

type BackupSlugResponse struct {
	Slug string `json:"slug"`
}
type BackupsSlugResponse struct {
	Backups []BackupSlugResponse `json:"backups"`
}
type BackupsSlugDataResponse struct {
	Data BackupsSlugResponse `json:"data"`
}

type HaBackupInformationResponse struct {
	HaBackupInformation HaBackupInfo `json:"data"`
}
type StubData struct {
	Data string `json:"data,omitempty"`
}
type StubResponse struct {
	Data StubData `json:"data,omitempty"`
}
type HaBackupInfo struct {
	Slug                string                      `json:"slug"`
	Name                string                      `json:"name"`
	BackupType          string                      `json:"type"`
	HaSupervisorVersion string                      `json:"supervisor_version"`
	HaCoreVersion       string                      `json:"homeassistant"`
	BackupCreated       types.CustomTimeRFC3339Nano `json:"date"`
	Folders             []string                    `json:"folders"`
	Addons              []HaAddonInfo               `json:"addons"`
	Protected           bool                        `json:"protected"`
	Size                int64                       `json:"size_bytes"`
	Location            string                      `json:"location"`
	Locations           []string                    `json:"locations"`
}
type HaAddonInfo struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

func NewHaApi(entity_id string, ctx context.Context, client *http.Client, token string, logger *mylogger.Logger) (*HaApiClient, error) {
	if token == "" {
		return nil, errors.New("required token")
	}

	if entity_id == "" {
		entity_id = DefaultEntityId
	}
	entity_id = EntityIdPrefix + entity_id
	return &HaApiClient{entity_id: entity_id, ctx: ctx, httpClient: client, token: token, logger: logger}, nil
}

func (haApi *HaApiClient) SetLastBackupState(withError bool, errorText string) error {
	state, err := haApi.EnsureEntityState()
	if err != nil {
		return err
	}

	now := CustomTimeNow()
	state.LastCreateBackupTime = now
	if !withError {
		state.LastCreateBackupWithError = false
		state.LastCreateBacupErrorMessage = ""
	} else {
		state.LastCreateBackupErrorTime = now
		state.LastCreateBacupErrorMessage = errorText
	}
	return haApi.innerSetEntityState(*state, true)
}

func (haApi *HaApiClient) SetEntityState(entityState EntityState) error {
	return haApi.innerSetEntityState(entityState, true)
}

func (haApi *HaApiClient) GetEntityState() (*EntityState, error) {
	haApi.logger.DebugLog.Println("Get entity request")
	url := fmt.Sprintf("%s/states/%s", CoreBaseURL, haApi.entity_id)
	var sensor getEntityStateResponse
	err := haApi.getRequest(url, &sensor)
	if err != nil {
		resultError := fmt.Errorf("error when get entity: %v", err)
		return nil, resultError
	}

	entityState := NewEntityState(sensor.State, sensor.Attributes)

	return entityState, nil
}

func (haApi *HaApiClient) EnsureEntityState() (*EntityState, error) {
	haApi.logger.DebugLog.Printf("Check state entity existence")
	state, err := haApi.GetEntityState()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error read current state entity %v", err)
	}

	if state != nil {
		haApi.logger.DebugLog.Printf("State entity already exists")
		return state, nil
	}

	haApi.logger.DebugLog.Printf("State entity not exists")
	entityCopy, err := readLocalEntityCopy()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error read current state entity local copy %v", err)
		return nil, err
	}

	entityStateCopy := NewEntityState(entityCopy.State, entityCopy.Attributes)

	err = haApi.innerSetEntityState(*entityStateCopy, false)
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error write state entity from local copy %v", err)
		return nil, err
	}

	haApi.logger.InfoLog.Printf("State entity restored")

	return entityStateCopy, nil

}

func (haApi *HaApiClient) GetAddonList() (*Addons, error) {
	haApi.logger.DebugLog.Println("Get addons request")
	url := fmt.Sprintf("%s", AddonsBaseURL)
	var addonsList getAddonsResult
	err := haApi.getRequest(url, &addonsList)
	if err != nil {
		resultError := fmt.Errorf("error when get addons: %v", err)
		return nil, resultError
	}
	haApi.logger.DebugLog.Printf("Get addons result %v", addonsList)
	return addonsList.Data, nil
}

func (haApi *HaApiClient) GetBackupSlugsList() (*BackupsSlugResponse, error) {
	haApi.logger.DebugLog.Println("Get backup slugs request")
	url := fmt.Sprintf("%s", BackupBaseURL)
	var result BackupsSlugDataResponse

	err := haApi.getRequest(url, &result)

	if err != nil {
		resultError := fmt.Errorf("error when get backups: %v", err)
		return nil, resultError
	}

	return &result.Data, nil
}

func (haApi *HaApiClient) GetBackupInformation(slug string) (*HaBackupInfo, error) {
	haApi.logger.DebugLog.Println("Get backup slugs request")
	url := fmt.Sprintf("%s/%s/info", BackupBaseURL, slug)
	var result HaBackupInformationResponse

	err := haApi.getRequest(url, &result)

	if err != nil {
		resultError := fmt.Errorf("error when get backups: %v", err)
		return nil, resultError
	}

	return &result.HaBackupInformation, nil
}

func (haApi *HaApiClient) GetHostInformation() (*HostInfo, error) {
	haApi.logger.DebugLog.Println("Get backup slugs request")
	url := fmt.Sprintf("%s/info", HostBaseURL)
	var result getHostInfoResult

	err := haApi.getRequest(url, &result)

	if err != nil {
		resultError := fmt.Errorf("error when get host information: %v", err)
		return nil, resultError
	}

	return result.Data, nil
}

func (haApi *HaApiClient) GetStorageStatistic() (map[string]types.StorageStatistic, error) {
	info, err := haApi.GetHostInformation()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error when get host info. %v", err)
	}

	fileStatistics, err := haApi.GetFileStatistic()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error when get amount files. %v", err)
	}

	result := make(map[string]types.StorageStatistic)

	for k, v := range fileStatistics {
		if k == "local" {
			//result["local"] = types.StorageStatistic{FreeSpace: types.FileSize(math.Round(info.DiskFree) * types.MiB),
			//result["local"] = types.StorageStatistic{FreeSpace: types.FileSize(math.Round(info.DiskFree) * 1024 * 1024),
			result["local"] = types.StorageStatistic{FreeSpace: types.GiBToFileSize(info.DiskFree),
				FilesSize:  v.FilesSize,
				FileAmount: v.FileAmount}
		} else {
			result[k] = types.StorageStatistic{FreeSpace: -1,
				FilesSize:  v.FilesSize,
				FileAmount: v.FileAmount}
		}
	}
	return result, nil
}

func (haApi *HaApiClient) GetFileStatistic() (map[string]*FileStatistic, error) {
	slugs, err := haApi.GetBackupSlugsList()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error when get slugs. %v", err)
		return nil, err
	}

	result := make(map[string]*FileStatistic)

	for _, slug := range slugs.Backups {

		information, err := haApi.GetBackupInformation(slug.Slug)
		if err != nil {
			haApi.logger.ErrorLog.Printf("Error when get information about slug %v. %v", slug.Slug, err)
			return nil, err
		}

		key := "local"

		if information.Location != "" {
			key = information.Location
		}

		if v, exists := result[key]; exists {
			v.FileAmount++
			v.FilesSize += types.FileSize(information.Size)
		} else {
			result[key] = &FileStatistic{FileAmount: 1, FilesSize: types.FileSize(information.Size)}
		}
	}

	return result, nil

}

func (haApi *HaApiClient) GetJobInfo(jobId string) (*JobInfo, error) {
	haApi.logger.DebugLog.Println("Get job info request %s", jobId)
	url := fmt.Sprintf("%s/%s", JobBaseURL, jobId)
	var result JobInfoResponse

	err := haApi.getRequest(url, &result)

	if err != nil {
		resultError := fmt.Errorf("error when get job info %s: %v", jobId, err)
		return nil, resultError
	}

	haApi.logger.LogStruct("Job info response %s", result, haApi.logger.DebugLog)

	return result.Data, nil
}

func (haApi *HaApiClient) DeleteBackup(slug string) error {
	haApi.logger.DebugLog.Println("Delete backup request")
	url := fmt.Sprintf("%s/%s", BackupBaseURL, slug)
	var result StubResponse

	err := haApi.deleteRequest(url, &result)

	if err != nil {
		resultError := fmt.Errorf("error when delete backup: %v", err)
		return resultError
	}

	return nil
}

func (haApi *HaApiClient) CreateFullBackup(backupName string) (*CreateBackupResult, error) {
	haApi.logger.DebugLog.Println("Create full backup request")
	url := fmt.Sprintf("%s/new/full", BackupBaseURL)
	var result CreateBackupResponse

	body := createFullBacupRequest{
		Name:            "Full_Y_Backup_" + time.Now().Format(time.DateTime),
		Compressed:      true,
		ExcludeDatabase: false,
		Background:      true,
	}

	err := haApi.postRequest(url, body, &result)

	if err != nil {
		resultError := fmt.Errorf("error when create full backup: %v", err)
		return nil, resultError
	}

	haApi.logger.LogStruct("Backup response %s", result, haApi.logger.DebugLog)
	return result.Data, nil
}

func GetTemporaryFilePath(fileName string) string {
	return UPLOAD_TEMP_DIR + "/" + fileName
}

func (haApi *HaApiClient) RemoveTemporaryFile(fileName string) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		// Файл не существует, это не ошибка
		haApi.logger.DebugLog.Printf("File %s does not exist, nothing to delete.", fileName)
		return
	} else if err != nil {
		haApi.logger.ErrorLog.Printf("Error when delete file %s.", fileName)
	}

	// Удаляем файл
	if err := os.Remove(fileName); err != nil {
		haApi.logger.ErrorLog.Printf("Error when delete file %s.", fileName)
		return
	}

	haApi.logger.DebugLog.Printf("File %s deleted successfully.", fileName)
	return
}

func (haApi *HaApiClient) DeleteOldTemporaryFiles(days int) error {
	threshold := time.Now().AddDate(0, 0, -days)

	err := filepath.Walk(UPLOAD_TEMP_DIR, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Если это директория, пропускаем
			return nil
		}

		if info.ModTime().Before(threshold) {
			haApi.logger.DebugLog.Printf("Deleting file: %s\n", path)
			if err := os.Remove(path); err != nil {
				haApi.logger.ErrorLog.Printf("Error when delete file %s.", path)
				return fmt.Errorf("error when delete file %s", path)
			}
		}

		return nil
	})

	if err != nil {
		haApi.logger.ErrorLog.Printf("Error when get directory info %s.", err)
		return fmt.Errorf("error when get directory info %s", err)
	}

	return nil
}

func (haApi *HaApiClient) postRequest(url string, body interface{}, result interface{}) error {
	return haApi.innerRequest("POST", url, http.StatusOK, body, result)
}

func (haApi *HaApiClient) deleteRequest(url string, result interface{}) error {
	return haApi.innerRequest("DELETE", url, http.StatusOK, nil, result)
}

func (haApi *HaApiClient) getRequest(url string, result interface{}) error {
	return haApi.innerRequest("GET", url, http.StatusOK, nil, result)
}

func (haApi *HaApiClient) innerRequest(method string, url string, expectedStatus int, body any, result interface{}) error {
	haApi.logger.DebugLog.Printf("Execute %s request %s", method, url)

	// Преобразуем структуру в JSON

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			resultError := fmt.Errorf("error when data marshalling: %v", err)
			haApi.logger.ErrorLog.Println(resultError)
			return resultError
		}
		bodyReader = bytes.NewReader(jsonData) // эффективнее для чтения
	}

	// Создаём запрос
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		resultError := fmt.Errorf("error when create request: %w", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	// Устанавливаем заголовок для JSON, если тело было
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", haApi.token))
	req.Header.Set("Accept", "application/json") // Ожидание ответа в формате JSON

	// Выполняем запрос
	resp, err := haApi.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode != expectedStatus {
		resultError := fmt.Errorf("failed to fetch data: %s", resp.Status)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	// Читаем тело ответа
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read body: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	//haApi.logger.ErrorLog.Printf("*** Result body: %s", string(respBody))

	// Декодируем JSON-ответ
	if err := json.Unmarshal(respBody, result); err != nil {
		resultError := fmt.Errorf("error when parse body: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}
	haApi.logger.DebugLog.Printf("Get result %v", result)
	return nil
}

func (haApi *HaApiClient) GetDownloadBackupBody(slug string) (int64, io.ReadCloser, error) {
	haApi.logger.DebugLog.Println("Get addons request")
	url := fmt.Sprintf("%s/%s/download", BackupBaseURL, slug)

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return 0, nil, resultError
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", haApi.token))
	//req.Header.Set("Accept", "application/json") // Ожидание ответа в формате JSON

	// Выполняем запрос
	resp, err := haApi.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return 0, nil, resultError
	}

	// Получаем значение заголовка Content-Length
	contentLengthStr := resp.Header.Get("Content-Length")
	if contentLengthStr == "" {
		resp.Body.Close()
		return 0, nil, fmt.Errorf("can not get Content-Length headre from response")
	}

	// Преобразуем Content-Length из строки в число
	contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
	if err != nil {
		resp.Body.Close()
		return 0, nil, fmt.Errorf("error when convert Content-Length value to integer: %w", err)
	}

	return contentLength, resp.Body, nil
}

func (haApi *HaApiClient) SaveAddonIcon(slug string) (string, error) {
	haApi.logger.DebugLog.Println("Save addon icon")
	url := fmt.Sprintf("%s/%s/icon", AddonsBaseURL, slug)
	outputFileName := "f" + slug + ".ico"
	outputFile := addonIconPath + "/" + outputFileName

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return "", resultError
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", haApi.token))

	// Выполняем запрос
	resp, err := haApi.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return "", resultError
	}

	defer resp.Body.Close()

	// Открываем (или создаем) файл для записи
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Println("Ошибка при создании файла:", err)
		return "", err
	}
	defer file.Close()

	// Записываем данные из ответа напрямую в файл
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Println("Ошибка при записи в файл:", err)
		return "", err
	}

	return outputFileName, nil

}

func (haApi *HaApiClient) innerSetEntityState(entityState EntityState, saveLocalCopy bool) error {
	haApi.logger.DebugLog.Println("Set entity")

	url := fmt.Sprintf("%s/states/%s", CoreBaseURL, haApi.entity_id)

	data := setEntityStateRequest{
		State: entityState.State,
		Attributes: EntityAttributes{
			OkUploadAmount:              entityState.OkUpload,
			ErrorUploadAmount:           entityState.ErrorUpload,
			OkDeleteAmount:              entityState.OkDelete,
			ErrorDeleteAmount:           entityState.ErrorDelete,
			RemoteFiles:                 entityState.RemoteFiles,
			LocalFiles:                  entityState.LocalFiles,
			RemoteFileSize:              int64(entityState.RemoteSize),
			LocalFileSize:               int64(entityState.LocalSize),
			RemoteFreeSpace:             int64(entityState.RemoteFreeSpace),
			LastUploadTime:              entityState.LastUploadedTime,
			LastCreateBackupTime:        entityState.LastCreateBackupTime,
			LastCreateBackupErrorTime:   entityState.LastCreateBackupErrorTime,
			LastCreateBackupWithError:   entityState.LastCreateBackupWithError,
			LastCreateBacupErrorMessage: entityState.LastCreateBacupErrorMessage,
		},
	}

	if saveLocalCopy {
		// Сделаем локальную копию
		haApi.logger.DebugLog.Printf("Save state entity local copy")
		err := writeLocalEntityCopy(data)
		if err != nil {
			haApi.logger.ErrorLog.Printf("Error write local state entity copy %v", err)
		}
	}

	// Преобразуем структуру в JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		resultError := fmt.Errorf("error when data marshalling: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	// Выполняем POST запрос

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	// Устанавливаем заголовок Content-Type
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", haApi.token))

	haApi.logger.DebugLog.Printf("Send request %v", req)
	// Отправляем запрос
	resp, err := haApi.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		haApi.logger.ErrorLog.Println("Ошибка при чтении ответа: %v", err)
	}

	haApi.logger.DebugLog.Printf("Request result %d: %s", resp.StatusCode, body)

	if resp.StatusCode >= 400 {
		resultError := fmt.Errorf("request perform with error: %d and body %s", resp.StatusCode, body)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	return nil
}

func NewEntityState(state Status, attributes EntityAttributes) *EntityState {
	return &EntityState{
		State:                       state,
		OkUpload:                    attributes.OkUploadAmount,
		ErrorUpload:                 attributes.ErrorUploadAmount,
		OkDelete:                    attributes.OkDeleteAmount,
		ErrorDelete:                 attributes.ErrorDeleteAmount,
		LocalFiles:                  attributes.LocalFiles,
		RemoteFiles:                 attributes.RemoteFiles,
		LocalSize:                   types.FileSize(attributes.LocalFileSize),
		RemoteSize:                  types.FileSize(attributes.RemoteFileSize),
		RemoteFreeSpace:             types.FileSize(attributes.RemoteFreeSpace),
		LastUploadedTime:            attributes.LastUploadTime,
		LastCreateBackupTime:        attributes.LastCreateBackupTime,
		LastCreateBackupErrorTime:   attributes.LastCreateBackupErrorTime,
		LastCreateBackupWithError:   attributes.LastCreateBackupWithError,
		LastCreateBacupErrorMessage: attributes.LastCreateBacupErrorMessage,
	}
}

func writeLocalEntityCopy(entityState setEntityStateRequest) error {
	jsonData, err := json.Marshal(entityState)
	if err != nil {
		return err
	}

	err = os.WriteFile(localEntityCopyPath, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func readLocalEntityCopy() (setEntityStateRequest, error) {
	plan, _ := os.ReadFile(localEntityCopyPath)
	var data setEntityStateRequest
	err := json.Unmarshal(plan, &data)
	return data, err
}

func (app *HaApiClient) UploadBackup(source string, destinationFileName string) error {
	app.logger.DebugLog.Printf("Try upload %s into %s", source, destinationFileName)

	url := fmt.Sprintf("%s/new/upload", BackupBaseURL)
	return app.uploadFileMultipart(url, source, app.token)
}

func (app *HaApiClient) uploadFileMultipart(url, filePath, token string) error {
	// Открываем файл для чтения
	file, err := os.Open(filePath)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when open file: %v", err)
		return fmt.Errorf("error when open file: %v", err)
	}
	defer file.Close()

	// Создаем буфер для тела запроса
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Создаем часть для файла
	part, err := writer.CreateFormFile("file", filePath)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when create form: %v", err)
		return fmt.Errorf("error when create form: %v", err)
	}

	// Читаем файл по частям и записываем в часть формы
	buf := make([]byte, 8192) // Буфер размером 8KB
	for {
		n, err := file.Read(buf)
		if n > 0 {
			_, err := part.Write(buf[:n])
			if err != nil {
				app.logger.ErrorLog.Printf("Error when write part to form: %v", err)
				return fmt.Errorf("error when write part to form: %v", err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			app.logger.ErrorLog.Printf("Error when read part from file: %v", err)
			return fmt.Errorf("error when read part from file: %v", err)
		}
	}

	// Закрываем писатель, чтобы завершить формирование тела запроса
	err = writer.Close()
	if err != nil {
		app.logger.ErrorLog.Printf("Error when close writer: %v", err)
		return fmt.Errorf("error when close writer: %v", err)
	}

	// Создаем новый запрос
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when create request: %v", err)
		return fmt.Errorf("error when create request: %v", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	// Создаем HTTP-клиент
	client := &http.Client{}

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when execute request: %v", err)
		return fmt.Errorf("error when execute request: %v", err)
	}
	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Читаем тело ответа для получения сообщения об ошибке
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			app.logger.ErrorLog.Printf("Error when read response: %v", err)
			return fmt.Errorf("error when read response: %v", err)
		}
		app.logger.ErrorLog.Printf("Unexpected status: %s, response: %v", resp.Status, string(responseBody))
		return fmt.Errorf("unexpected status: %s, response: %v", resp.Status, string(responseBody))
	}

	app.logger.InfoLog.Printf("File uploaded: %s", filePath)
	return nil
}
