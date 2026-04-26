package models

import (
	"encoding/json"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/google/uuid"
	"time"
)

type ExecuteOnceTrigger struct {
	Id               string          `json:"id" gorm:"primaryKey"`
	CreatedAt        int64           `json:"created_at,string" gorm:"autoCreateTime:nano"`
	UpdatedAt        int64           `json:"updated_at,string" gorm:"autoUpdateTime:nano"`
	FireAt           time.Time       `json:"fire_at"`
	TaskDefinitionId *uuid.UUID      `json:"task_definition_id"`
	TaskDefinition   *TaskDefinition `json:"task"`
}

func GetExecuteOnceTriggerModelFromTrigger(trigger *pkg.ExecuteOnceTrigger) (*ExecuteOnceTrigger, error) {
	triggerJsonBytes, err := json.Marshal(trigger)
	if err != nil {
		return nil, err
	}
	if triggerJsonBytes == nil {
		return nil, nil
	}
	var triggerModel *ExecuteOnceTrigger
	err = json.Unmarshal(triggerJsonBytes, &triggerModel)
	return triggerModel, err
}
