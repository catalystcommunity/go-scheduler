package models

import (
	"encoding/json"
	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/dariubs/gorm-jsonb"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type TaskDefinition struct {
	Id                  *uuid.UUID          `json:"id" gorm:"primaryKey"`
	CreatedAt           int64               `json:"created_at,string" gorm:"autoCreateTime:nano"`
	UpdatedAt           int64               `json:"updated_at,string" gorm:"autoUpdateTime:nano"`
	Metadata            gormjsonb.JSONB     `json:"metadata" gorm:"type:jsonb"`
	ExpireAfter         *time.Duration      `json:"expire_after"`
	ExpireAfterInterval *string             `json:"expire_after_interval"`
	InProgress          bool                `json:"in_progress_at"`
	LastFireTime        *time.Time          `json:"last_fire_time"`
	NextFireTime        *time.Time          `json:"next_fire_time"`
	ExecuteOnceTrigger  *ExecuteOnceTrigger `json:"execute_once_trigger" gorm:"foreignKey:Id"`
	CronTrigger         *CronTrigger        `json:"cron_trigger" gorm:"foreignKey:Id"`
	CompletedAt         *time.Time          `json:"completed_at"`
	TaskInstances       []TaskInstance      `json:"task_instances"`
	Recurring           bool
}

var nilUuidString = uuid.Nil.String()

func (t *TaskDefinition) BeforeCreate(tx *gorm.DB) error {
	// comparing to uuid.Nil directly doesn't work as expected and skips this condition when it shouldn't, hence the string comparison
	if t.Id == nil || t.Id.String() == nilUuidString {
		id := uuid.New()
		t.Id = &id
		logging.Log.Info("set new id on task definition during create")
	}
	return nil
}

func (t TaskDefinition) ToTaskDefinition() (pkg.TaskDefinition, error) {
	var task pkg.TaskDefinition
	taskModelJsonBytes, err := json.Marshal(t)
	if err != nil {
		return task, err
	}
	err = json.Unmarshal(taskModelJsonBytes, &task)
	if t.CronTrigger != nil {
		cronTrigger, err := pkg.NewCronTrigger(t.CronTrigger.Expression)
		if err != nil {
			return task, err
		}
		task.CronTrigger = cronTrigger
	}
	return task, err
}

func ToTaskDefinitions(models []TaskDefinition) ([]pkg.TaskDefinition, error) {
	tasks := []pkg.TaskDefinition{}
	for _, model := range models {
		task, err := model.ToTaskDefinition()
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func GetTaskDefinitionModelFromTaskDefinition(task pkg.TaskDefinition) (*TaskDefinition, error) {
	// marshal triggers
	executeOnceTriggerModel, err := GetExecuteOnceTriggerModelFromTrigger(task.ExecuteOnceTrigger)
	if err != nil {
		return nil, err
	}
	cronTriggerModel, err := GetCronTriggerModelFromTrigger(task.CronTrigger)
	if err != nil {
		return nil, err
	}
	// nullify triggers
	task.ExecuteOnceTrigger = nil
	task.CronTrigger = nil
	// marshal the task model
	taskJsonBytes, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	var taskModel *TaskDefinition
	err = json.Unmarshal(taskJsonBytes, &taskModel)
	if err != nil {
		return nil, err
	}
	// set triggers
	taskModel.ExecuteOnceTrigger = executeOnceTriggerModel
	if taskModel.ExecuteOnceTrigger != nil {
		taskModel.ExecuteOnceTrigger.TaskDefinition = taskModel
	}
	taskModel.CronTrigger = cronTriggerModel
	if taskModel.CronTrigger != nil {
		taskModel.CronTrigger.TaskDefinition = taskModel
	}
	// set expiration interval
	if taskModel.ExpireAfter != nil {
		interval := taskModel.ExpireAfter.String()
		taskModel.ExpireAfterInterval = &interval
	} else {
		taskModel.ExpireAfterInterval = nil
	}
	return taskModel, nil
}
