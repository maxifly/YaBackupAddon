package haoperate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

const (
	CoreBaseURL         string = "http://supervisor/core/api"
	AddonsBaseURL       string = "http://supervisor/addons"
	BackupBaseURL       string = "http://supervisor/backups"
	EntityIdPrefix      string = "sensor."
	DefaultEntityId     string = "yandex_backup_state"
	localEntityCopyPath string = "/data/entity-copy.json"
	addonIconPath       string = "/app/yabackup/internal/pkg/rest/ui/static/appicons"
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
	State            Status
	OkUpload         int
	ErrorUpload      int
	OkDelete         int
	ErrorDelete      int
	LocalFiles       int
	RemoteFiles      int
	LocalSize        types.FileSize
	RemoteSize       types.FileSize
	RemoteFreeSpace  types.FileSize
	LastUploadedTime CustomTime
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
	OkUploadAmount    int        `json:"success_upload_files"`
	ErrorUploadAmount int        `json:"error_upload_files"`
	OkDeleteAmount    int        `json:"success_delete_files"`
	ErrorDeleteAmount int        `json:"error_delete_files"`
	RemoteFiles       int        `json:"remote_files"`
	LocalFiles        int        `json:"local_files"`
	RemoteFileSize    int64      `json:"remote_file_size"`
	LocalFileSize     int64      `json:"local_file_size"`
	RemoteFreeSpace   int64      `json:"remote_free_space"`
	LastUploadTime    CustomTime `json:"last_upload_time"`
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

func (haApi *HaApiClient) SetEntityState(entityState EntityState) error {
	return haApi.innerSetEntityState(entityState, true)
}

func (haApi *HaApiClient) GetEntityState() (*EntityState, error) {
	haApi.logger.DebugLog.Println("Get entity request")
	url := fmt.Sprintf("%s/states/%s", CoreBaseURL, haApi.entity_id)
	var sensor getEntityStateResponse
	err := haApi.getJson(url, &sensor)
	if err != nil {
		resultError := fmt.Errorf("error when get entity: %v", err)
		return nil, resultError
	}

	entityState := NewEntityState(sensor.State, sensor.Attributes)

	return entityState, nil
}

func (haApi *HaApiClient) EnsureEntityState() error {
	haApi.logger.DebugLog.Printf("Check state entity existence")
	state, err := haApi.GetEntityState()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error read current state entity %v", err)
	}

	if state != nil {
		haApi.logger.DebugLog.Printf("State entity already exists")
		return nil
	}

	haApi.logger.DebugLog.Printf("State entity not exists")
	entityCopy, err := readLocalEntityCopy()
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error read current state entity local copy %v", err)
		return err
	}

	entityStateCopy := NewEntityState(entityCopy.State, entityCopy.Attributes)

	err = haApi.innerSetEntityState(*entityStateCopy, false)
	if err != nil {
		haApi.logger.ErrorLog.Printf("Error write state entity from local copy %v", err)
		return err
	}

	haApi.logger.InfoLog.Printf("State entity restored")

	return nil

}

func (haApi *HaApiClient) GetAddonList() (*Addons, error) {
	haApi.logger.DebugLog.Println("Get addons request")
	url := fmt.Sprintf("%s", AddonsBaseURL)
	var addonsList getAddonsResult
	err := haApi.getJson(url, &addonsList)
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

	err := haApi.getJson(url, &result)

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

	err := haApi.getJson(url, &result)

	if err != nil {
		resultError := fmt.Errorf("error when get backups: %v", err)
		return nil, resultError
	}

	return &result.HaBackupInformation, nil
}

func (haApi *HaApiClient) getJson(url string, result interface{}) error {
	haApi.logger.DebugLog.Printf("Execute get request %s", url)

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
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
	if resp.StatusCode != http.StatusOK {
		resultError := fmt.Errorf("failed to fetch data: %s", resp.Status)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read body: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return resultError
	}

	//haApi.logger.DebugLog.Printf("*** Result body: %s", string(body))

	// Декодируем JSON-ответ
	if err := json.Unmarshal(body, result); err != nil {
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

	//// Проверяем статус код
	//if resp.StatusCode != http.StatusOK {
	//	resultError := fmt.Errorf("failed to fetch data: %s", resp.Status)
	//	haApi.logger.ErrorLog.Println(resultError)
	//	return 123, resultError
	//}
	//
	//// Читаем тело ответа
	//body, err := io.ReadAll(resp.Body)
	//if err != nil {
	//	resultError := fmt.Errorf("error when read body: %v", err)
	//	haApi.logger.ErrorLog.Println(resultError)
	//	return 123, resultError
	//}
	//
	//// Декодируем JSON-ответ
	//var addonsList getAddonsResult
	//if err := json.Unmarshal(body, &addonsList); err != nil {
	//	resultError := fmt.Errorf("error when parse body: %v", err)
	//	haApi.logger.ErrorLog.Println(resultError)
	//	return 123, resultError
	//}
	//haApi.logger.DebugLog.Printf("Get addons result %v", addonsList)
	//return 124, nil
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
			OkUploadAmount:    entityState.OkUpload,
			ErrorUploadAmount: entityState.ErrorUpload,
			OkDeleteAmount:    entityState.OkDelete,
			ErrorDeleteAmount: entityState.ErrorDelete,
			RemoteFiles:       entityState.RemoteFiles,
			LocalFiles:        entityState.LocalFiles,
			RemoteFileSize:    int64(entityState.RemoteSize),
			LocalFileSize:     int64(entityState.LocalSize),
			RemoteFreeSpace:   int64(entityState.RemoteFreeSpace),
			LastUploadTime:    entityState.LastUploadedTime,
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
		State:            state,
		OkUpload:         attributes.OkUploadAmount,
		ErrorUpload:      attributes.ErrorUploadAmount,
		OkDelete:         attributes.OkDeleteAmount,
		ErrorDelete:      attributes.ErrorDeleteAmount,
		LocalFiles:       attributes.LocalFiles,
		RemoteFiles:      attributes.RemoteFiles,
		LocalSize:        types.FileSize(attributes.LocalFileSize),
		RemoteSize:       types.FileSize(attributes.RemoteFileSize),
		RemoteFreeSpace:  types.FileSize(attributes.RemoteFreeSpace),
		LastUploadedTime: attributes.LastUploadTime,
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
