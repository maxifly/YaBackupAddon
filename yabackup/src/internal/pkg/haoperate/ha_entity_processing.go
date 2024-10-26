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
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

const (
	BaseURL             string = "http://supervisor/core/api"
	EntityIdPrefix      string = "sensor."
	DefaultEntityId     string = "yandex_backup_state"
	localEntityCopyPath string = "/data/entity-copy.json"
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
	url := fmt.Sprintf("%s/states/%s", BaseURL, haApi.entity_id)

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return nil, resultError
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", haApi.token))
	req.Header.Set("Accept", "application/json") // Ожидание ответа в формате JSON

	// Выполняем запрос
	resp, err := haApi.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return nil, resultError
	}

	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode == http.StatusNotFound {
		haApi.logger.InfoLog.Println("Entity not found")
		return nil, nil
	} else if resp.StatusCode != http.StatusOK {
		resultError := fmt.Errorf("failed to fetch data: %s", resp.Status)
		haApi.logger.ErrorLog.Println(resultError)
		return nil, resultError
	}

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read body: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
		return nil, resultError
	}

	// Декодируем JSON-ответ
	var sensor getEntityStateResponse
	if err := json.Unmarshal(body, &sensor); err != nil {
		resultError := fmt.Errorf("error when parse body: %v", err)
		haApi.logger.ErrorLog.Println(resultError)
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

func (haApi *HaApiClient) innerSetEntityState(entityState EntityState, saveLocalCopy bool) error {
	haApi.logger.DebugLog.Println("Set entity")

	url := fmt.Sprintf("%s/states/%s", BaseURL, haApi.entity_id)

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
