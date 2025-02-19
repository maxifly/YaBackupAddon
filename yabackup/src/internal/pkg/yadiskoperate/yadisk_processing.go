package yadiskoperate

import (
	"context"
	yadisk "github.com/nikitaksv/yandex-disk-sdk-go"
	"net/http"
	"time"
)

const itemTypeFile string = "file"

var minTime = time.Date(1990, time.January, 01, 12, 00, 0, 0, time.UTC)

func NewYandexDisk(accessToken string) (yadisk.YaDisk, error) {
	return yadisk.NewYaDisk(context.Background(), http.DefaultClient, &yadisk.Token{AccessToken: accessToken})

}

func convertDateString(modified string) (time.Time, error) {
	return time.Parse(time.RFC3339, modified)
}
