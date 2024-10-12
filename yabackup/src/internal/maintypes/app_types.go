package maintypes

import (
	"github.com/nikitaksv/yandex-disk-sdk-go"
	"log"
	"ybg/internal/pkg/haoperate"
	"ybg/internal/pkg/mylogger"
	"ybg/internal/types"
)

type AppData struct {
	errorLog  *log.Logger
	infoLog   *log.Logger
	debugLog  *log.Logger
	Logger    *mylogger.Logger
	Options   ApplOptions
	TokenInfo types.TokenInfo
	YaDisk    *yadisk.YaDisk
	HaApi     *haoperate.HaApiClient
}

type ApplOptions struct {
	ClientId                   string `json:"client_id"`
	ClientSecret               string `json:"client_secret"`
	RemotePath                 string `json:"remote_path"`
	RemoteMaximumFilesQuantity int    `json:"remote_maximum_files_quantity"`
	Schedule                   string `json:"schedule"`
	LogLevel                   string `json:"log_level"`
	Theme                      string `json:"theme" default:"Light"`
}

func (ao *ApplOptions) IsUseDarkTheme() bool {
	return ao.Theme == "Dark"
}
