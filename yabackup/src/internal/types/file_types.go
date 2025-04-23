package types

import (
	"encoding/json"
	"fmt"
	"time"
)

const MiB = float64(1 << 20)
const PUT = "PUT"
const POST = "POST"

var currentLocation *time.Location = time.Now().Location()

type FileSize int64
type FileModified time.Time

func (fs FileSize) Convert2MbString() string {
	return fmt.Sprintf("%.2f", float64(fs)/MiB)
}

func (fm FileModified) Convert2String() string {
	return time.Time(fm).In(currentLocation).Format("02.01.2006 15:04:05 MST")
}

func (fm FileModified) IsZero() bool {
	return time.Time(fm).IsZero()
}

type CustomTimeRFC3339Nano struct {
	time.Time
}

func (c CustomTimeRFC3339Nano) MarshalJSON() ([]byte, error) {
	// Здесь указываем нужный формат
	return json.Marshal(c.Time.Format(time.RFC3339Nano))
}

func (c *CustomTimeRFC3339Nano) UnmarshalJSON(data []byte) error {
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
