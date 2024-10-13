package yadiskoperate

import (
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"time"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

type YaDProcessor struct {
	clientId     string
	clientSecret string
	TokenInfo    types.TokenInfo
	yaDisk       *yadisk.YaDisk
	logger       *mylogger.Logger
}

func NewYaDProcessor(clientId string,
	clientSecret string,
	logger *mylogger.Logger) *YaDProcessor {
	return &YaDProcessor{
		clientId:     clientId,
		clientSecret: clientSecret,
		logger:       logger,
	}
}

func (app *YaDProcessor) EnsureTokenInfo() {
	if isTokenEmpty(app.TokenInfo) {
		token, err := readToken()
		if err != nil {
			app.logger.ErrorLog.Printf("Error read token info %v", err)
			return
		}
		app.TokenInfo = token
	}
}

func (app *YaDProcessor) RefreshTokenIsNeed() bool {
	if app.TokenInfo.Expiry.After(time.Now().Add(time.Duration(240) * time.Hour)) {
		app.logger.DebugLog.Printf("Not need refresh token")
		return false
	}

	tokenInfo, err := RefreshToken(app.clientId, app.clientSecret, app.TokenInfo)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when refresh token %v", err)
		return false
	}
	app.logger.InfoLog.Printf("%+v", tokenInfo)

	err = writeToken(*tokenInfo)
	if err != nil {
		app.logger.ErrorLog.Printf("Error when write token %v", err)
		return false
	}

	app.TokenInfo = *tokenInfo
	app.logger.InfoLog.Printf("Refresh token done")
	app.EnsureYandexDisk()
	return true
}

func (app *YaDProcessor) EnsureYandexDisk() {
	if !isTokenEmpty(app.TokenInfo) {
		disk, err := NewYandexDisk(app.TokenInfo.AccessToken)
		if err != nil {
			app.logger.ErrorLog.Printf("Error when create YaDisk %v", err)
			return
		}
		app.yaDisk = &disk
	}
}

func (app *YaDProcessor) IsTokenEmpty() bool {
	return isTokenEmpty(app.TokenInfo)
}

func (app *YaDProcessor) IsTokenValid() bool {
	return isTokenValid(app.TokenInfo)
}
