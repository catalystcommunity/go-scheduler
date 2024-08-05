package models

import (
	"encoding/json"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/google/uuid"
)

type CronTrigger struct {
	Id               string          `json:"id" gorm:"primaryKey"`
	CreatedAt        int64           `json:"created_at,string" gorm:"autoCreateTime:nano"`
	UpdatedAt        int64           `json:"updated_at,string" gorm:"autoUpdateTime:nano"`
	Expression       string          `json:"expression"`
	TaskDefinitionId *uuid.UUID      `json:"task_definition_id"`
	TaskDefinition   *TaskDefinition `json:"task"`
}

func GetCronTriggerModelFromTrigger(trigger *pkg.CronTrigger) (*CronTrigger, error) {
	triggerJsonBytes, err := json.Marshal(trigger)
	if err != nil {
		return nil, err
	}
	if triggerJsonBytes == nil {
		return nil, nil
	}
	var triggerModel *CronTrigger
	err = json.Unmarshal(triggerJsonBytes, &triggerModel)
	return triggerModel, err
}
