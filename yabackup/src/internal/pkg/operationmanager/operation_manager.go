package operationmanager

import (
	"context"
	"errors"
	"runtime"
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
	applCtx       context.Context
}

func New(applCtx context.Context, logger *mylogger.Logger) *OperationManager {
	return &OperationManager{
		operationsMap: &sync.Map{},
		logger:        logger,
		applCtx:       applCtx,
	}
}

func (dwn *OperationManager) ChangeProgress(id string, newProgress int) {
	dwn.updateOperationInfo(id, func(info *OperationInfo) {
		info.Progress = newProgress
	})
}

func (dwn *OperationManager) StartOperation(id string, newStatus string) {
	dwn.updateOperationInfo(id, func(info *OperationInfo) {
		info.Status = newStatus
		info.Progress = 0
		info.IsDone = false
		info.IsError = false
	})
}

func (dwn *OperationManager) ChangeStatusAndProgress(id string, newStatus string, newProgress int) {
	dwn.updateOperationInfo(id, func(info *OperationInfo) {
		info.Status = newStatus
		info.Progress = newProgress
	})
}

func (dwn *OperationManager) SuccessDone(id string) {
	dwn.updateOperationInfo(id, func(info *OperationInfo) {
		info.Progress = 100.
		info.IsDone = true
	})
}

func (dwn *OperationManager) ErrorDone(id string, errorStatus string) {
	dwn.updateOperationInfo(id, func(info *OperationInfo) {
		info.Status = errorStatus
		info.IsError = true
		info.IsDone = true
	})
}

func (dwn *OperationManager) updateOperationInfo(id string, updateFunc func(*OperationInfo)) {
	// Функция для обновления информации о загрузке

	for {
		val, exists := dwn.operationsMap.Load(id)

		// Объявляем переменную info заранее
		var info, orig *OperationInfo

		if exists {
			// Приводим тип и сохраняем в info
			orig = val.(*OperationInfo)
			info = &OperationInfo{Progress: orig.Progress,
				Status:      orig.Status,
				IsError:     orig.IsError,
				IsDone:      orig.IsDone,
				LastUpdated: orig.LastUpdated}

		} else {
			// Если запись не существует, создаем новую
			info = &OperationInfo{0., "created", false, false, time.Now()}
		}

		// Вызываем updateFunc с указателем на info
		updateFunc(info)
		info.LastUpdated = time.Now()

		// Сохраняем обновленное значение обратно в operationsMap
		oldAny, _ := dwn.operationsMap.Swap(id, info)
		var old *OperationInfo
		if oldAny != nil {
			old = oldAny.(*OperationInfo)
		} else {
			old = nil
		}

		dwn.logger.DebugLog.Printf("orig %+v, old %+v", orig, old)

		if old == orig {
			return
		}

		dwn.logger.DebugLog.Printf("repeat")
		runtime.Gosched()
	}
}

func (dwn *OperationManager) ClearOperations(hourDelta int) error {
	threshold := time.Now().Add(time.Duration(-1*hourDelta) * time.Hour)
	dwn.operationsMap.Range(func(key, value interface{}) bool {
		if structData, ok := value.(*OperationInfo); ok {
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
		v, ok2 := value.(*OperationInfo)
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

func (dwn *OperationManager) WaitOperationDone(id string, interval time.Duration, timeout time.Duration) (bool, *OperationInfoResponse) {
	ctx, cancel := context.WithTimeout(dwn.applCtx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ok, operationInfo := dwn.GetOperation(id)
	if ok {
		if operationInfo.IsDone {
			dwn.logger.InfoLog.Printf("Operation completed immediatly. [operationId %s]", id)
			return true, operationInfo
		}
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				dwn.logger.DebugLog.Printf("Stop periodical operation checker by timeout (%v) [operationId %s]", timeout, id)

			} else {
				dwn.logger.DebugLog.Printf("Stop periodical operation checker by shutdown [operationId %s]", id)
			}

			return false, nil
		case <-ticker.C:
			ok, operationInfo = dwn.GetOperation(id)
			if ok {
				if operationInfo.IsDone {
					dwn.logger.InfoLog.Printf("Operation completed. [operationId %s]", id)
					return true, operationInfo
				}
				dwn.logger.ErrorLog.Printf("Operation not complete. [operationId %s]", id)
			} else {
				dwn.logger.ErrorLog.Printf("Operation not found. [operationId %s]", id)
			}
		}
	}

}

func (dwn *OperationManager) GetOperation(id string) (bool, *OperationInfoResponse) {

	if val, ok := dwn.operationsMap.Load(id); ok {
		v, ok := val.(*OperationInfo)
		if !ok {
			dwn.logger.InfoLog.Printf("Value type incorrect")
			return false, nil
		}

		result := transformItem(id, v)

		return true, &result

	}

	return false, nil

}

func transformItem(id string, item *OperationInfo) OperationInfoResponse {
	return OperationInfoResponse{
		Id:       id,
		Progress: item.Progress,
		Status:   item.Status,
		IsError:  item.IsError,
		IsDone:   item.IsDone,
	}
}
