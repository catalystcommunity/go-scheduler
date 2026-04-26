package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const oncePerSecondCron = "* * * * * * *"
const oncePerMinuteCronFormat = "%d * * * * * *"
const everyNSecondsCronFormat = "0/%d * * * * * *"

type TestMetaData struct {
	Message string
}

func TestTaskDefinitionCrud(t *testing.T, store pkg.StoreInterface) {
	// create one task of each trigger type
	expectedExecuteOnceTask := generateRandomTaskWithExecuteOnceTrigger(time.Now().Add(5*time.Second), 0)
	expectedCronTask, err := generateRandomTaskWithCronTrigger("@hourly", 0)
	require.NoError(t, err)
	err = store.UpsertTaskDefinition(expectedExecuteOnceTask)
	require.NoError(t, err)
	err = store.UpsertTaskDefinition(expectedCronTask)
	require.NoError(t, err)
	tasks, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assertTaskEquality(t, expectedExecuteOnceTask, tasks[0])
	assertTaskEquality(t, expectedCronTask, tasks[1])
	require.NotNil(t, tasks[0].GetTrigger())
	require.NotNil(t, tasks[1].GetTrigger())
	// update execute once task
	updatedExecuteOnceTask := tasks[0]
	expectedExpireAfter := 10 * time.Second
	expectedMetaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	updatedExecuteOnceTask.ExpireAfter = expectedExpireAfter
	updatedExecuteOnceTask.Metadata = expectedMetaData
	updatedExecuteOnceTask.ExecuteOnceTrigger = pkg.NewExecuteOnceTrigger(time.Now().Add(20 * time.Second))
	err = store.UpsertTaskDefinition(updatedExecuteOnceTask)
	require.NoError(t, err)
	// verify update
	fetchedExecuteOnceTask, err := store.GetTaskDefinition(updatedExecuteOnceTask.Id)
	require.NoError(t, err)
	assertTaskEquality(t, updatedExecuteOnceTask, fetchedExecuteOnceTask)
	// update cron task
	updatedCronTask := tasks[1]
	updatedCronExpression := "@daily"
	updatedCronTask.CronTrigger, err = pkg.NewCronTrigger(updatedCronExpression)
	require.NoError(t, err)
	updatedCronTask.ExpireAfter = expectedExpireAfter
	updatedCronTask.Metadata = expectedMetaData
	err = store.UpsertTaskDefinition(updatedCronTask)
	require.NoError(t, err)
	// verify update
	fetchedCronTask, err := store.GetTaskDefinition(updatedCronTask.Id)
	require.NoError(t, err)
	assertTaskEquality(t, updatedCronTask, fetchedCronTask)
	// test list offset/limit
	tasks, err = store.ListTaskDefinitions(0, 1, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, updatedExecuteOnceTask.Id, tasks[0].Id)
	tasks, err = store.ListTaskDefinitions(1, 1, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, updatedCronTask.Id, tasks[0].Id)
	tasks, err = store.ListTaskDefinitions(2, 10, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 0)
	// delete task definitions
	err = store.DeleteTaskDefinition(updatedExecuteOnceTask.Id)
	require.NoError(t, err)
	fetchedExecuteOnceTask, err = store.GetTaskDefinition(updatedExecuteOnceTask.Id)
	require.ErrorContains(t, err, "record not found")
	err = store.DeleteTaskDefinition(updatedCronTask.Id)
	require.NoError(t, err)
	fetchedCronTask, err = store.GetTaskDefinition(updatedCronTask.Id)
	require.ErrorContains(t, err, "record not found")
}

func TestTaskInstanceCrud(t *testing.T, store pkg.StoreInterface) {
	// create one task of each trigger type
	expectedExecuteOnceTask := generateRandomTaskWithExecuteOnceTrigger(time.Now().Add(5*time.Second), 0)
	expireAfter := 2 * time.Second
	expectedExecuteOnceTask.ExpireAfter = expireAfter
	expectedCronTask, err := generateRandomTaskWithCronTrigger("@hourly", 0)
	expectedCronTask.ExpireAfter = expireAfter
	require.NoError(t, err)
	err = store.UpsertTaskDefinition(expectedExecuteOnceTask)
	require.NoError(t, err)
	err = store.UpsertTaskDefinition(expectedCronTask)
	require.NoError(t, err)
	tasks, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	fetchedExecuteOnceTask := tasks[0]
	fetchedCronTask := tasks[1]
	// create a task instance for each task
	executeOnceTaskInstance := createTaskInstanceFromTaskDefinition(fetchedExecuteOnceTask)
	err = store.UpsertTaskInstance(executeOnceTaskInstance)
	require.NoError(t, err)
	cronTaskInstance := createTaskInstanceFromTaskDefinition(fetchedCronTask)
	err = store.UpsertTaskInstance(cronTaskInstance)
	require.NoError(t, err)
	// list task instances
	listedTaskInstances, err := store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 2)
	listedExecuteOnceTaskInstance := listedTaskInstances[0]
	listedCronTaskInstance := listedTaskInstances[1]
	require.Equal(t, fetchedExecuteOnceTask.Id, listedExecuteOnceTaskInstance.TaskDefinition.Id)
	require.Equal(t, fetchedCronTask.Id, listedCronTaskInstance.TaskDefinition.Id)
	// update task instances
	completedAt := time.Now().UTC()
	listedExecuteOnceTaskInstance.CompletedAt = &completedAt
	listedCronTaskInstance.CompletedAt = &completedAt
	err = store.UpsertTaskInstance(listedExecuteOnceTaskInstance)
	require.NoError(t, err)
	err = store.UpsertTaskInstance(listedCronTaskInstance)
	require.NoError(t, err)
	// fetch by id and verify the update
	fetchedExecuteOnceTaskInstance, err := store.GetTaskInstance(listedExecuteOnceTaskInstance.Id)
	require.NoError(t, err)
	assertTaskInstanceEquality(t, listedExecuteOnceTaskInstance, fetchedExecuteOnceTaskInstance)
	fetchedCronTaskInstance, err := store.GetTaskInstance(listedCronTaskInstance.Id)
	require.NoError(t, err)
	assertTaskInstanceEquality(t, listedCronTaskInstance, fetchedCronTaskInstance)
	// verify list with offest/limit
	listedTaskInstances, err = store.ListTaskInstances(0, 1)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	require.Equal(t, listedTaskInstances[0].Id, listedExecuteOnceTaskInstance.Id)
	listedTaskInstances, err = store.ListTaskInstances(1, 1)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	require.Equal(t, listedTaskInstances[0].Id, listedCronTaskInstance.Id)
	// delete
	err = store.DeleteTaskInstance(listedExecuteOnceTaskInstance.Id)
	require.NoError(t, err)
	err = store.DeleteTaskInstance(listedCronTaskInstance.Id)
	require.NoError(t, err)
	// verify delete
	_, err = store.GetTaskInstance(listedExecuteOnceTaskInstance.Id)
	require.ErrorContains(t, err, "record not found")
	_, err = store.GetTaskInstance(listedCronTaskInstance.Id)
	require.ErrorContains(t, err, "record not found")
}

func TestGetTaskInstancesToRunNotInProgressNotExpired(t *testing.T, store pkg.StoreInterface) {
	// create task definition
	executeAt := time.Now().Add(5 * time.Minute)
	taskDefinition := generateRandomTaskWithExecuteOnceTrigger(executeAt, 0)
	expireAfter := 1 * time.Minute
	taskDefinition.ExpireAfter = expireAfter
	err := store.UpsertTaskDefinition(taskDefinition)
	require.NoError(t, err)
	listedTaskDefinitions, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedTaskDefinition := listedTaskDefinitions[0]
	// create task instance
	taskInstance := createTaskInstanceFromTaskDefinition(listedTaskDefinition)
	err = store.UpsertTaskInstance(taskInstance)
	require.NoError(t, err)
	// list task instances so we have a reference
	listedTaskInstances, err := store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	listedTaskInstance := listedTaskInstances[0]
	// query for tasks to run with a limit of now(), shouldn't come back
	taskInstancesToRun, err := store.GetTaskInstancesToRun(time.Now())
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 0)
	// query for tasks to run in the next 5 minutes, should get the task instance back
	taskInstancesToRun, err = store.GetTaskInstancesToRun(time.Now().Add(5 * time.Minute))
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 1)
	taskInstanceToRun := taskInstancesToRun[0]
	require.Equal(t, listedTaskInstance.Id, taskInstanceToRun.Id)
}

func TestGetTaskInstancesToRunInProgressNotExpired(t *testing.T, store pkg.StoreInterface) {
	// create task definition
	executeAt := time.Now().Add(5 * time.Minute)
	taskDefinition := generateRandomTaskWithExecuteOnceTrigger(executeAt, 0)
	expireAfter := 1 * time.Minute
	taskDefinition.ExpireAfter = expireAfter
	err := store.UpsertTaskDefinition(taskDefinition)
	require.NoError(t, err)
	listedTaskDefinitions, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedTaskDefinition := listedTaskDefinitions[0]
	// create task instance that is in progress
	taskInstance := createTaskInstanceFromTaskDefinition(listedTaskDefinition)
	taskInstance.StartedAt = taskInstance.ExecuteAt
	err = store.UpsertTaskInstance(taskInstance)
	require.NoError(t, err)
	// list task instances so we have a reference
	listedTaskInstances, err := store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	// query for tasks to run with a limit of now(), shouldn't come back
	taskInstancesToRun, err := store.GetTaskInstancesToRun(time.Now())
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 0)
	// query for tasks to run in the next 5 minutes, shouldn't get the task instance back because it's already in progress
	taskInstancesToRun, err = store.GetTaskInstancesToRun(time.Now().Add(5 * time.Minute))
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 0)
}

func TestGetTaskInstancesToRunInProgressAndExpired(t *testing.T, store pkg.StoreInterface) {
	// create task definition
	executeAt := time.Now()
	taskDefinition := generateRandomTaskWithExecuteOnceTrigger(executeAt, 0)
	expireAfter := 5 * time.Second
	taskDefinition.ExpireAfter = expireAfter
	err := store.UpsertTaskDefinition(taskDefinition)
	require.NoError(t, err)
	listedTaskDefinitions, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedTaskDefinition := listedTaskDefinitions[0]
	// create task instance that is in progress
	taskInstance := createTaskInstanceFromTaskDefinition(listedTaskDefinition)
	taskInstance.StartedAt = taskInstance.ExecuteAt
	err = store.UpsertTaskInstance(taskInstance)
	require.NoError(t, err)
	// list task instances so we have a reference
	listedTaskInstances, err := store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	listedTaskInstance := listedTaskInstances[0]
	// query for tasks to run with a limit of now(), shouldn't come back
	taskInstancesToRun, err := store.GetTaskInstancesToRun(time.Now())
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 0)
	// sleep until the expiration, then query for tasks to run in the next 5 minutes, should get the instance back because it's expired
	time.Sleep(time.Until(*listedTaskInstance.ExpiresAt))
	taskInstancesToRun, err = store.GetTaskInstancesToRun(time.Now().Add(5 * time.Minute))
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 1)
	require.Equal(t, listedTaskInstance.Id, taskInstancesToRun[0].Id)
}

func TestGetTaskInstancesToRun(t *testing.T, store pkg.StoreInterface) {
	// create task definition to run in 5 minutes
	executeAt := time.Now().Add(5 * time.Minute)
	expireAfter := 2 * time.Second
	expectedExecuteOnceTask := generateRandomTaskWithExecuteOnceTrigger(executeAt, expireAfter)
	err := store.UpsertTaskDefinition(expectedExecuteOnceTask)
	require.NoError(t, err)
	listedTaskDefinitions, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedTaskDefinition := listedTaskDefinitions[0]
	// create task instance
	executeOnceTaskInstance := createTaskInstanceFromTaskDefinition(listedTaskDefinition)
	err = store.UpsertTaskInstance(executeOnceTaskInstance)
	require.NoError(t, err)
	// list task instances so we have a reference
	listedTaskInstances, err := store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	listedTaskInstance := listedTaskInstances[0]
	// query for tasks to run, shouldn't come back yet
	taskInstancesToRun, err := store.GetTaskInstancesToRun(time.Now())
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 0)
	// query for tasks to run in the next 5 minutes, should get the task instance
	taskInstancesToRun, err = store.GetTaskInstancesToRun(time.Now().Add(5 * time.Minute))
	require.NoError(t, err)
	require.Len(t, taskInstancesToRun, 1)
	taskInstanceToRun := taskInstancesToRun[0]
	require.Equal(t, listedTaskInstance.Id, taskInstanceToRun.Id)
}

func TestMarkCompleted(t *testing.T, store pkg.StoreInterface) {
	// non-recurring triggers should also mark the task definition complete when the instance is marked complete
	expectedExecuteOnceTaskDefinition := generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 0)
	err := store.UpsertTaskDefinition(expectedExecuteOnceTaskDefinition)
	require.NoError(t, err)
	expectedExecuteOnceTaskInstance := generateRandomTaskInstance(expectedExecuteOnceTaskDefinition)
	err = store.UpsertTaskInstance(expectedExecuteOnceTaskInstance)
	require.NoError(t, err)
	// ensure neither are marked complete, just in case
	listedTaskDefinitions, err := store.ListTaskDefinitions(0, 100, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedExecuteOnceTaskDefinition := listedTaskDefinitions[0]
	require.Nil(t, listedExecuteOnceTaskDefinition.CompletedAt)
	listedTaskInstances, err := store.ListTaskInstances(0, 100)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedExecuteOnceTaskInstance := listedTaskInstances[0]
	require.Nil(t, listedExecuteOnceTaskInstance.CompletedAt)
	err = store.MarkTaskInstanceComplete(listedExecuteOnceTaskInstance)
	require.NoError(t, err)
	// verify the instance is marked complete
	fetchedExecuteOnceTaskInstance, err := store.GetTaskInstance(listedExecuteOnceTaskInstance.Id)
	require.NoError(t, err)
	require.NotNil(t, fetchedExecuteOnceTaskInstance.CompletedAt)
	require.False(t, fetchedExecuteOnceTaskInstance.CompletedAt.IsZero())
	//verify the definition is marked complete
	fetchedExecuteOnceTaskDefinition, err := store.GetTaskDefinition(listedExecuteOnceTaskDefinition.Id)
	require.NoError(t, err)
	require.NotNil(t, fetchedExecuteOnceTaskDefinition.CompletedAt)
	require.False(t, fetchedExecuteOnceTaskDefinition.CompletedAt.IsZero())
	// delete the instance and definition
	err = store.DeleteCompletedTaskInstances()
	require.NoError(t, err)
	err = store.DeleteCompletedTaskDefinitions()
	require.NoError(t, err)

	// recurring triggers should not mark the task definition complete when the instance is marked complete
	expectedCronTaskDefinition, err := generateRandomTaskWithCronTrigger("", 0)
	require.NoError(t, err)
	err = store.UpsertTaskDefinition(expectedCronTaskDefinition)
	require.NoError(t, err)
	expectedCronTaskInstance := generateRandomTaskInstance(expectedCronTaskDefinition)
	err = store.UpsertTaskInstance(expectedCronTaskInstance)
	require.NoError(t, err)
	// ensure neither are marked complete, just in case
	listedTaskDefinitions, err = store.ListTaskDefinitions(0, 100, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedCronTaskDefinition := listedTaskDefinitions[0]
	require.Nil(t, listedCronTaskDefinition.CompletedAt)
	listedTaskInstances, err = store.ListTaskInstances(0, 100)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	listedCronTaskInstance := listedTaskInstances[0]
	require.Nil(t, listedCronTaskInstance.CompletedAt)
	err = store.MarkTaskInstanceComplete(listedCronTaskInstance)
	require.NoError(t, err)
	// verify the instance is marked complete
	fetchedCronTaskInstance, err := store.GetTaskInstance(listedCronTaskInstance.Id)
	require.NoError(t, err)
	require.NotNil(t, fetchedCronTaskInstance.CompletedAt)
	require.False(t, fetchedCronTaskInstance.CompletedAt.IsZero())
	//verify the definition is not marked complete
	fetchedCronTaskDefinition, err := store.GetTaskDefinition(listedCronTaskDefinition.Id)
	require.NoError(t, err)
	require.Nil(t, fetchedCronTaskDefinition.CompletedAt)
}

func TestCleanup(t *testing.T, store pkg.StoreInterface) {
	// create two task definitions
	err := store.UpsertTaskDefinition(generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 0))
	require.NoError(t, err)
	err = store.UpsertTaskDefinition(generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 0))
	require.NoError(t, err)
	listedTaskDefinitions, err := store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	firstTaskDefinition := listedTaskDefinitions[0]
	secondTaskDefinition := listedTaskDefinitions[1]
	// create two task instances
	err = store.UpsertTaskInstance(generateRandomTaskInstance(firstTaskDefinition))
	require.NoError(t, err)
	err = store.UpsertTaskInstance(generateRandomTaskInstance(secondTaskDefinition))
	require.NoError(t, err)
	// list task instances so we have a reference
	listedTaskInstances, err := store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 2)
	firstTaskInstance := listedTaskInstances[0]
	secondTaskInstance := listedTaskInstances[1]
	// mark first task instance completed
	err = store.MarkTaskInstanceComplete(firstTaskInstance)
	require.NoError(t, err)
	// ensure the task definition was also marked completed since it's a non-recurring task
	fetchedTaskDefinition, err := store.GetTaskDefinition(firstTaskDefinition.Id)
	require.NoError(t, err)
	require.NotNil(t, fetchedTaskDefinition.CompletedAt)
	require.False(t, fetchedTaskDefinition.CompletedAt.IsZero())
	// delete completed task instances
	err = store.DeleteCompletedTaskInstances()
	require.NoError(t, err)
	// make sure there are still 2 task definitions, but the task instance no longe exists
	listedTaskDefinitions, err = store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 2)
	listedTaskInstances, err = store.ListTaskInstances(0, 1000)
	require.NoError(t, err)
	require.Len(t, listedTaskInstances, 1)
	require.Equal(t, secondTaskInstance.Id, listedTaskInstances[0].Id)
	// delete completed task definitions
	err = store.DeleteCompletedTaskDefinitions()
	// ensure the task definition was deleted
	listedTaskDefinitions, err = store.ListTaskDefinitions(0, 1000, nil)
	require.NoError(t, err)
	require.Len(t, listedTaskDefinitions, 1)
	require.Equal(t, secondTaskDefinition.Id, listedTaskDefinitions[0].Id)
}

func TestExecuteOnceTriggerHappyPath(t *testing.T, store pkg.StoreInterface) {
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		return nil
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	executeAt := time.Now().Add(1 * time.Second)
	expectedDefinition := generateRandomTaskWithExecuteOnceTrigger(executeAt, 0)
	err = scheduler.UpsertTaskDefinition(expectedDefinition)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	require.NoError(t, err)
	time.Sleep(5 * time.Second)
	require.Equal(t, 1, executionCount)
}

func TestExecuteOnceTriggerTasksRunInOrder(t *testing.T, store pkg.StoreInterface) {
	// this tests that tasks are executed in the right order. 3 tasks are scheduled with execution times nearer to now than the last
	// resulting in scheduling the last running task first, and the first running task last. The first running task should
	// be executed first even though it was scheduled last
	executedTaskDefinitions := []pkg.TaskDefinition{}
	handler := func(task pkg.TaskInstance) error {
		executedTaskDefinitions = append(executedTaskDefinitions, pkg.TaskDefinition{Id: task.TaskDefinition.Id})
		return nil
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	// create three tasks that should run in reverse order of when they're scheduled
	// task1
	task1Id := uuid.New()
	task1MetaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	task1ExecuteAt := time.Now().Add(7 * time.Second)
	task1 := pkg.TaskDefinition{
		Id:                 &task1Id,
		Metadata:           task1MetaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(task1ExecuteAt),
	}
	err = scheduler.UpsertTaskDefinition(task1)
	require.NoError(t, err)

	// task2
	task2Id := uuid.New()
	task2MetaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	task2ExecuteAt := time.Now().Add(5 * time.Second)
	task2 := pkg.TaskDefinition{
		Id:                 &task2Id,
		Metadata:           task2MetaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(task2ExecuteAt),
	}
	err = scheduler.UpsertTaskDefinition(task2)
	require.NoError(t, err)

	// task3
	task3Id := uuid.New()
	task3MetaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	task3ExecuteAt := time.Now().Add(3 * time.Second)
	task3 := pkg.TaskDefinition{
		Id:                 &task3Id,
		Metadata:           task3MetaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(task3ExecuteAt),
	}
	err = scheduler.UpsertTaskDefinition(task3)
	require.NoError(t, err)

	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(10 * time.Second)
	require.Len(t, executedTaskDefinitions, 3)
	require.Equal(t, task3.Id, executedTaskDefinitions[0].Id)
	require.Equal(t, task2.Id, executedTaskDefinitions[1].Id)
	require.Equal(t, task1.Id, executedTaskDefinitions[2].Id)
}

func TestExecuteOnceTriggerLongRunningTaskExpired(t *testing.T, store pkg.StoreInterface) {
	// first task sleeps longer than the window and expiration, simulating a long running task that eventually completes successfully
	// this should result in the task expiring and being run twice.
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		time.Sleep(3 * time.Second)
		return nil
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	executeAt := time.Now().Add(2 * time.Second)
	expireAfter := 2 * time.Second
	task := pkg.TaskDefinition{
		Id:                 &id,
		Metadata:           metaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(executeAt),
		ExpireAfter:        expireAfter,
	}
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(10 * time.Second)
	require.Equal(t, 2, executionCount)
}

func TestExecuteOnceTriggerLongRunningTaskNotExpired(t *testing.T, store pkg.StoreInterface) {
	// first task sleeps longer than the window but less than the expiration, simulating a long running task that eventually completes successfully before the expiration time
	// this should result in the task being run once.
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		time.Sleep(3 * time.Second)
		return nil
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	executeAt := time.Now().Add(2 * time.Second)
	expireAfter := 4 * time.Second
	task := pkg.TaskDefinition{
		Id:                 &id,
		Metadata:           metaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(executeAt),
		ExpireAfter:        expireAfter,
	}
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(10 * time.Second)
	require.Equal(t, 1, executionCount)
}

func TestExecuteOnceTriggerRetry(t *testing.T, store pkg.StoreInterface) {
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		return errors.New("fayl")
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	executeAt := time.Now().Add(2 * time.Second)
	task := pkg.TaskDefinition{
		Id:                 &id,
		Metadata:           metaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(executeAt),
	}
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(10 * time.Second)
	require.GreaterOrEqual(t, executionCount, 5) // there is a timing issue here, we need to make sure that the task was retried but this will
	// have different timing based on the system running it and resources etc. I tried to test it with exactly 3 retries but it's difficult because
	// sometimes it will run longer and have executed 4 times, sometimes 2. So I settled on waiting 10 seconds which is much longer
	// than should be required, and verifying it's retried 5 times
}

func TestExecuteOnceTriggerNoRetry(t *testing.T, store pkg.StoreInterface) {
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		return errors.New("fayl")
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	executeAt := time.Now().Add(2 * time.Second)
	task := pkg.TaskDefinition{
		Id:                 &id,
		Metadata:           metaData,
		ExecuteOnceTrigger: pkg.NewExecuteOnceTrigger(executeAt),
	}
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(4 * time.Second)
	require.Equal(t, 1, executionCount)
}

func TestCronTriggerHappyPath(t *testing.T, store pkg.StoreInterface) {
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		return nil
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	cronTrigger, err := pkg.NewCronTrigger(oncePerSecondCron)
	require.NoError(t, err)
	task := pkg.TaskDefinition{
		Id:          &id,
		Metadata:    metaData,
		CronTrigger: cronTrigger,
	}
	go scheduler.Run()
	defer scheduler.Stop()
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	time.Sleep(10500 * time.Millisecond)
	require.GreaterOrEqual(t, executionCount, 9)
	require.LessOrEqual(t, executionCount, 11)
}

func TestListWithMetadataQuery(t *testing.T, store pkg.StoreInterface, metadata interface{}, metadataQuery interface{}) {
	for i := 0; i < 5; i++ {
		definition := generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 1*time.Minute)
		err := store.UpsertTaskDefinition(definition)
		require.NoError(t, err)
	}
	metaDefinition := generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 1*time.Minute)
	metaDefinition.Metadata = metadata
	err := store.UpsertTaskDefinition(metaDefinition)
	require.NoError(t, err)
	definitions, err := store.ListTaskDefinitions(0, 100, metadataQuery)
	require.Len(t, definitions, 1)
	require.Equal(t, definitions[0].Metadata, metadata)
}

func TestDeleteWithMetadataQuery(t *testing.T, store pkg.StoreInterface, metadata interface{}, metadataQuery interface{}) {
	for i := 0; i < 5; i++ {
		definition := generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 1*time.Minute)
		err := store.UpsertTaskDefinition(definition)
		require.NoError(t, err)
	}
	metaDefinition := generateRandomTaskWithExecuteOnceTrigger(time.Time{}, 1*time.Minute)
	metaDefinition.Metadata = metadata
	err := store.UpsertTaskDefinition(metaDefinition)
	require.NoError(t, err)
	definitions, err := store.ListTaskDefinitions(0, 100, metadataQuery)
	require.Len(t, definitions, 1)
	require.Equal(t, definitions[0].Metadata, metadata)
	err = store.DeleteTaskDefinitionsByMetadata(metadataQuery)
	require.NoError(t, err)
	definitions, err = store.ListTaskDefinitions(0, 100, metadataQuery)
	require.NoError(t, err)
	require.Len(t, definitions, 0)
	definitions, err = store.ListTaskDefinitions(0, 100, nil)
	require.NoError(t, err)
	require.Len(t, definitions, 5)
}

func TestCronTriggerRetry(t *testing.T, store pkg.StoreInterface) {
	executionCount := 0
	succeedAfter := 3
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		if executionCount == succeedAfter {
			return nil
		}
		return errors.New("fayl")
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	// run once per minute starting 1 second from now
	cronTrigger, err := pkg.NewCronTrigger(fmt.Sprintf(oncePerMinuteCronFormat, time.Now().Second()+1))
	require.NoError(t, err)
	task := pkg.TaskDefinition{
		Id:          &id,
		Metadata:    metaData,
		CronTrigger: cronTrigger,
	}
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(10 * time.Second)
	require.Equal(t, executionCount, 3) // trigger should only fire once, and it should get retried twice
}

func TestCronTriggerNoRetry(t *testing.T, store pkg.StoreInterface) {
	executionCount := 0
	handler := func(task pkg.TaskInstance) error {
		executionCount++
		return errors.New("fayl")
	}
	// tick once per second
	scheduler, err := pkg.NewScheduler(1*time.Second, 1*time.Second, 1*time.Second, handler, store)
	require.NoError(t, err)
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	// every 2 seconds starting 1 second from now
	cronTrigger, err := pkg.NewCronTrigger(fmt.Sprintf(everyNSecondsCronFormat, 2))
	require.NoError(t, err)
	task := pkg.TaskDefinition{
		Id:          &id,
		Metadata:    metaData,
		CronTrigger: cronTrigger,
	}
	err = scheduler.UpsertTaskDefinition(task)
	require.NoError(t, err)
	go scheduler.Run()
	defer scheduler.Stop()
	time.Sleep(7 * time.Second)
	// between 3 and 4 executions depending on resources
	require.GreaterOrEqual(t, executionCount, 3)
	require.LessOrEqual(t, executionCount, 4)
}

func assertTaskEquality(t *testing.T, expected, actual pkg.TaskDefinition) {
	require.Equal(t, expected.Id, actual.Id)
	require.Equal(t, expected.ExpireAfter, actual.ExpireAfter)
	require.Equal(t, expected.NextFireTime, actual.NextFireTime)
	expectedMetaJson, err := json.Marshal(expected.Metadata)
	require.NoError(t, err)
	actualMetaJson, err := json.Marshal(actual.Metadata)
	require.NoError(t, err)
	require.Equal(t, expectedMetaJson, actualMetaJson)
	if expected.ExecuteOnceTrigger != nil {
		expectedFireTime := expected.GetNextFireTime().UTC().Format(time.RFC3339)
		actualFireTime := actual.GetNextFireTime().UTC().Format(time.RFC3339)
		require.Equal(t, expectedFireTime, actualFireTime)
	}
	if expected.CronTrigger != nil {
		require.Equal(t, expected.CronTrigger.Expression, actual.CronTrigger.Expression)
	}
}

func assertTaskInstanceEquality(t *testing.T, expected, actual pkg.TaskInstance) {

}
func generateRandomTaskWithExecuteOnceTrigger(executeAt time.Time, expireAfter time.Duration) pkg.TaskDefinition {
	if executeAt.IsZero() {
		executeAt = time.Now().Add(time.Duration(gofakeit.IntRange(1, 60)) * time.Second)
	}
	if expireAfter == 0 {
		tempExpireAfter := time.Duration(gofakeit.IntRange(5, 60)) * time.Second
		expireAfter = tempExpireAfter
	}
	task := generateRandomTaskWithoutTrigger()
	task.ExecuteOnceTrigger = pkg.NewExecuteOnceTrigger(executeAt)
	task.ExpireAfter = expireAfter
	return task
}

func generateRandomTaskWithCronTrigger(expression string, expireAfter time.Duration) (pkg.TaskDefinition, error) {
	var err error
	task := generateRandomTaskWithoutTrigger()
	if expression == "" {
		expression = oncePerSecondCron
	}
	task.CronTrigger, err = pkg.NewCronTrigger(expression)
	if expireAfter == 0 {
		tempExpireAfter := time.Duration(gofakeit.IntRange(1, 60)) * time.Second
		expireAfter = tempExpireAfter
	}
	task.ExpireAfter = expireAfter
	task.Recurring = task.GetTrigger().IsRecurring()
	return task, err
}

func generateRandomTaskWithoutTrigger() pkg.TaskDefinition {
	id := uuid.New()
	metaData := TestMetaData{Message: gofakeit.HackerPhrase()}
	return pkg.TaskDefinition{
		Id:       &id,
		Metadata: metaData,
	}
}

func createTaskInstanceFromTaskDefinition(taskDefinition pkg.TaskDefinition) pkg.TaskInstance {
	executeAt := taskDefinition.GetNextFireTime()
	expiresAt := executeAt.Add(taskDefinition.ExpireAfter).UTC()
	return pkg.TaskInstance{
		ExpiresAt:      &expiresAt,
		ExecuteAt:      executeAt,
		TaskDefinition: taskDefinition,
	}
}

func generateRandomTaskInstance(taskDefinition pkg.TaskDefinition) pkg.TaskInstance {
	executeAt := taskDefinition.GetNextFireTime()
	expiresAt := executeAt.Add(taskDefinition.ExpireAfter)
	return pkg.TaskInstance{
		ExpiresAt:      &expiresAt,
		ExecuteAt:      executeAt,
		TaskDefinition: taskDefinition,
	}
}

func deleteAllTaskInstances(store pkg.StoreInterface) error {
	instances, err := store.ListTaskInstances(0, 1000)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		logging.Log.Infof("deleting task instance with id: %s", instance.Id)
		err = store.DeleteTaskInstance(instance.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteAllTaskDefinitions(store pkg.StoreInterface) error {
	definitions, err := store.ListTaskDefinitions(0, 1000, nil)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		err = store.DeleteTaskDefinition(definition.Id)
		if err != nil {
			return err
		}
	}
	return nil

}
