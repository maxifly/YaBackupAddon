package main

import "time"

type fileSize int64
type fileModified time.Time

type TokenInfo struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

type GeneralFileInfo struct {
	Name     string
	Size     fileSize
	Modified fileModified
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
	MD5            string
}
