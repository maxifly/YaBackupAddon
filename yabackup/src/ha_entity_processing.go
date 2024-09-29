package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	BaseURL  string = "http://supervisor/core/api"
	EntityId string = "sensor.yba_test"
)

type HA_API_CLIENT struct {
	ctx        context.Context
	httpClient *http.Client
	token      string
	logger     *Logger
}

type EntityState struct {
	state  string
	attrV1 string
	attrV2 string
}

// Определяем структуру, соответствующую JSON объекту

type EntityAttributes struct {
	NextRising  string `json:"next_rising"`
	NextSetting string `json:"next_setting"`
}

type SetEntityStateRequest struct {
	State      string           `json:"state"`
	Attributes EntityAttributes `json:"attributes"`
}

func NewHaApi(ctx context.Context, client *http.Client, token string, logger *Logger) (*HA_API_CLIENT, error) {
	if token == "" {
		return nil, errors.New("required token")
	}

	return &HA_API_CLIENT{ctx: ctx, httpClient: client, token: token, logger: logger}, nil
}

func (haApi *HA_API_CLIENT) setEntityState(entityState EntityState) error {
	haApi.logger.DebugLog.Println("Set entity request")
	url := fmt.Sprintf("%s/states/%s", BaseURL, EntityId)

	data := SetEntityStateRequest{
		State: entityState.state,
		Attributes: EntityAttributes{
			NextRising:  entityState.attrV1,
			NextSetting: entityState.attrV2,
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
