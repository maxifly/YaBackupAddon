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
	BackupName     string
	RemoteFileName string
	IsLocal        bool
	IsRemote       bool
}

type LocalBackupFileInfo struct {
	GeneralInfo GeneralFileInfo
	BackupName  string
	Slug        string
	Path        string
}

type BackupArchInfo struct {
	Slug string
	Name string
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
