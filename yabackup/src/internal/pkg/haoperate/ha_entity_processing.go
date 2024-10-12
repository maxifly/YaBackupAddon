package haoperate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

const (
	BaseURL  string = "http://supervisor/core/api"
	EntityId string = "sensor.yba_test"
)

type HaApiClient struct {
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
	OkUploadAmount    int        `json:"ok_upload_amount"`
	ErrorUploadAmount int        `json:"error_upload_amount"`
	OkDeleteAmount    int        `json:"ok_delete_amount"`
	ErrorDeleteAmount int        `json:"error_delete_amount"`
	RemoteFileSize    int64      `json:"remote_file_size"`
	LocalFileSize     int64      `json:"local_file_size"`
	RemoteFreeSpace   int64      `json:"remote_free_space"`
	LastUploadTime    CustomTime `json:"last_upload_time"`
}

type SetEntityStateRequest struct {
	State      Status           `json:"state"`
	Attributes EntityAttributes `json:"attributes"`
}

func NewHaApi(ctx context.Context, client *http.Client, token string, logger *mylogger.Logger) (*HaApiClient, error) {
	if token == "" {
		return nil, errors.New("required token")
	}

	return &HaApiClient{ctx: ctx, httpClient: client, token: token, logger: logger}, nil
}

func (haApi *HaApiClient) SetEntityState(entityState EntityState) error {
	haApi.logger.DebugLog.Println("Set entity request")
	url := fmt.Sprintf("%s/states/%s", BaseURL, EntityId)

	data := SetEntityStateRequest{
		State: entityState.State,
		Attributes: EntityAttributes{
			OkUploadAmount:    entityState.OkUpload,
			ErrorUploadAmount: entityState.ErrorUpload,
			OkDeleteAmount:    entityState.OkDelete,
			ErrorDeleteAmount: entityState.ErrorDelete,
			RemoteFileSize:    int64(entityState.RemoteSize),
			LocalFileSize:     int64(entityState.LocalSize),
			RemoteFreeSpace:   int64(entityState.RemoteFreeSpace),
			LastUploadTime:    entityState.LastUploadedTime,
		},
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
