package types

import (
	"time"
)

type TokenInfo struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

type DiskInfo struct {
	TotalSpace FileSize
	UsedSpace  FileSize
}

type GeneralFileInfo struct {
	Name     string
	Size     FileSize
	Modified FileModified
}

type RemoteFileInfo GeneralFileInfo

type BackupFileInfo struct {
	GeneralInfo    GeneralFileInfo
	BackupArchInfo *BackupArchInfo
	BackupSlug     string
	BackupName     string
	RemoteFileName string
	IsLocal        bool
	IsRemote       bool
}

type LocalBackupFileInfo struct {
	GeneralInfo    GeneralFileInfo
	BackupArchInfo *BackupArchInfo
	BackupSlug     string
	BackupName     string
	Path           string
}

type HaBackupInfo struct {
	Slug          string                `json:"slug"`
	Name          string                `json:"name"`
	BackupType    string                `json:"type"`
	HaVersion     string                `json:"supervisor_version"`
	HaCoreInfo    HaCoreInfo            `json:"homeassistant"`
	BackupCreated CustomTimeRFC3339Nano `json:"date"`
	Folders       []string              `json:"folders"`
	Addons        []HaAddonInfo         `json:"addons"`
}
type HaAddonInfo struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type HaCoreInfo struct {
	Version string `json:"version"`
}

type BackupArchInfo struct {
	Slug          string
	Name          string
	BackupType    string
	HaVersion     string
	CoreInfo      HaCoreInfo
	BackupCreated FileModified
	Folders       []string
	Addons        []HaAddonInfo
}

type ForUploadFileInfo struct {
	LocalFileInfo  GeneralFileInfo
	RemoteFileName string
}
type ForDeleteFileInfo struct {
	RemoteFileName string
	FileInfo       GeneralFileInfo
	MD5            string
}
