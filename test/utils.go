package test

import (
	"github.com/brianvoe/gofakeit/v6"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/google/uuid"
	"time"
)

func GenerateExecuteOnceTask(fireAt *time.Time) pkg.TaskDefinition {
	id := uuid.New()
	if fireAt == nil {
		generatedTime := time.Now().Add(time.Duration(gofakeit.IntRange(1, 5)) * time.Second).UTC()
		fireAt = &generatedTime
	}
	trigger := &pkg.ExecuteOnceTrigger{FireAt: *fireAt}
	return pkg.TaskDefinition{
		Id:                 &id,
		ExecuteOnceTrigger: trigger,
	}
}
