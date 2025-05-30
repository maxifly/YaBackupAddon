package operationmanager

import (
	"sync"
	"time"
	"ybg/internal/pkg/mylogger"
)

type OperationInfo struct {
	Progress    int
	Status      string
	IsError     bool
	IsDone      bool
	LastUpdated time.Time
}

type OperationInfoResponse struct {
	Id       string `json:"id"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
	IsError  bool   `json:"is_error"`
	IsDone   bool   `json:"is_done"`
}

type OperationManager struct {
	operationsMap *sync.Map
	logger        *mylogger.Logger
}

func New(logger *mylogger.Logger) *OperationManager {
	return &OperationManager{
		operationsMap: &sync.Map{},
		logger:        logger,
	}
}

func (dwn *OperationManager) ChangeProgress(id string, newProgress int) {
	dwn.UpdateOperationInfo(id, func(info *OperationInfo) {
		info.Progress = newProgress
	})
}

func (dwn *OperationManager) StartOperation(id string, newStatus string) {
	dwn.UpdateOperationInfo(id, func(info *OperationInfo) {
		info.Status = newStatus
		info.Progress = 0
		info.IsDone = false
		info.IsError = false
	})
}

func (dwn *OperationManager) ChangeStatusAndProgress(id string, newStatus string, newProgress int) {
	dwn.UpdateOperationInfo(id, func(info *OperationInfo) {
		info.Status = newStatus
		info.Progress = newProgress
	})
}

func (dwn *OperationManager) SuccessDone(id string) {
	dwn.UpdateOperationInfo(id, func(info *OperationInfo) {
		info.Progress = 100.
		info.IsDone = true
	})
}

func (dwn *OperationManager) ErrorDone(id string, errorStatus string) {
	dwn.UpdateOperationInfo(id, func(info *OperationInfo) {
		info.Status = errorStatus
		info.IsError = true
		info.IsDone = true
	})
}

func (dwn *OperationManager) UpdateOperationInfo(id string, updateFunc func(*OperationInfo)) {
	// Функция для обновления информации о загрузке

	val, exists := dwn.operationsMap.Load(id)

	// Объявляем переменную info заранее
	var info OperationInfo

	if exists {
		// Приводим тип и сохраняем в info
		info = val.(OperationInfo)

	} else {
		// Если запись не существует, создаем новую
		info = OperationInfo{0., "created", false, false, time.Now()}
	}

	// Вызываем updateFunc с указателем на info
	updateFunc(&info)
	info.LastUpdated = time.Now()

	// Сохраняем обновленное значение обратно в operationsMap
	dwn.operationsMap.Store(id, info)
}
func (dwn *OperationManager) ClearOperations(hourDelta int) error {
	threshold := time.Now().Add(time.Duration(-1*hourDelta) * time.Hour)
	dwn.operationsMap.Range(func(key, value interface{}) bool {
		if structData, ok := value.(OperationInfo); ok {
			if structData.LastUpdated.Before(threshold) {
				dwn.operationsMap.Delete(key)
			}
		}
		return true
	})
	return nil
}

func (dwn *OperationManager) GetAllOperations() []OperationInfoResponse {
	var targetList []OperationInfoResponse

	dwn.operationsMap.Range(func(key, value interface{}) bool {
		k, ok1 := key.(string)
		v, ok2 := value.(OperationInfo)
		if ok1 && ok2 {
			targetItem := transformItem(k, v)
			targetList = append(targetList, targetItem)
		} else {
			dwn.logger.InfoLog.Printf("Key or value type incorrect")
		}
		return true
	})
	return targetList
}

func transformItem(id string, item OperationInfo) OperationInfoResponse {
	return OperationInfoResponse{
		Id:       id,
		Progress: item.Progress,
		Status:   item.Status,
		IsError:  item.IsError,
		IsDone:   item.IsDone,
	}
}
