package pkg

import (
	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/google/uuid"
	"github.com/joomcode/errorx"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Scheduler struct {
	ScheduleWindow *time.Duration
	RunnerWindow   *time.Duration
	CleanupWindow  *time.Duration
	Handler        func(taskInstance TaskInstance) error
	store          StoreInterface
	lock           *sync.Mutex
	run            bool
	shutdown       chan bool
}

func NewScheduler(scheduleWindow, runnerWindow, cleanupWindow time.Duration, handler func(taskInstance TaskInstance) error, store StoreInterface) (*Scheduler, error) {
	scheduler := &Scheduler{
		ScheduleWindow: &scheduleWindow,
		RunnerWindow:   &runnerWindow,
		CleanupWindow:  &cleanupWindow,
		Handler:        handler,
		store:          store,
		lock:           new(sync.Mutex),
		shutdown:       make(chan bool, 1),
	}
	err := scheduler.initializeStore()
	return scheduler, err
}

func (s *Scheduler) UpsertTaskDefinition(task TaskDefinition) error {
	err := validateTask(task)
	if err != nil {
		return err
	}
	if task.ExpireAfter == 0 {
		task.ExpireAfter = *s.ScheduleWindow
	}
	task.NextFireTime = task.GetNextFireTime()
	task.Recurring = task.GetTrigger().IsRecurring()
	if task.Id == nil || task.Id == &uuid.Nil {
		id := uuid.New()
		task.Id = &id
	}
	return s.store.UpsertTaskDefinition(task)
}

func (s *Scheduler) GetTaskDefinitions(ids []*uuid.UUID) ([]TaskDefinition, error) {
	return s.store.GetTaskDefinitions(ids)
}

func (s *Scheduler) ListTaskDefinitions(skip, limit int, metadataQuery interface{}) ([]TaskDefinition, error) {
	return s.store.ListTaskDefinitions(skip, limit, metadataQuery)
}

func (s *Scheduler) DeleteTaskDefinition(id *uuid.UUID) error {
	if id == nil {
		return errorx.IllegalArgument.New("an id must be provided")
	}
	return s.store.DeleteTaskDefinition(id)
}

func (s *Scheduler) DeleteTaskDefinitions(ids []*uuid.UUID) error {
	return s.store.DeleteTaskDefinitions(ids)
}

func (s *Scheduler) DeleteTaskDefinitionsByMetadataQuery(metadataQuery interface{}) error {
	return s.store.DeleteTaskDefinitionsByMetadata(metadataQuery)
}

func (s *Scheduler) Run() {
	s.run = true
	// start task instance scheduler, task instance runner, and task instance cleanup, in background
	go s.startTaskInstanceScheduler()
	go s.startTaskInstanceRunner()
	go s.startTaskInstanceCleanup()
	go s.waitForOsSignal()
	// wait for shutdown from caller, or os
	<-s.shutdown
	s.shutDown()
}

func (s *Scheduler) startTaskInstanceScheduler() {
	ticker := time.NewTicker(*s.ScheduleWindow)
	for range ticker.C {
		if s.run {
			s.createTaskInstances()
		} else {
			ticker.Stop()
			break
		}
	}
}

func (s *Scheduler) createTaskInstances() {
	taskDefinitions, err := s.store.GetTaskDefinitionsToSchedule(time.Now().Add(*s.ScheduleWindow))
	if err != nil {
		logging.Log.WithError(err).Error("error getting task definitions to run in window")
		return
	}
	for _, taskDefinition := range taskDefinitions {
		err = s.createTaskInstance(taskDefinition)
		if err != nil {
			logging.Log.WithError(err).Error("error creating task instance")
		}
	}
}

func (s *Scheduler) createTaskInstance(taskDefinition TaskDefinition) error {
	executeAt := taskDefinition.GetNextFireTime()
	expiresAt := executeAt.Add(taskDefinition.ExpireAfter)
	taskInstance := TaskInstance{
		ExpiresAt:      &expiresAt,
		ExecuteAt:      executeAt,
		TaskDefinition: taskDefinition,
	}
	err := s.store.UpsertTaskInstance(taskInstance)
	if err != nil {
		logging.Log.WithError(err).Error("error creating task instance")
		return err
	}
	// update task definition's next fire time, nil for non recurring triggers which will prevent creating more task instances
	if taskDefinition.Recurring {
		nextExecution := *taskDefinition.GetFireTimeFrom(*executeAt)
		taskDefinition.NextFireTime = &nextExecution
	} else {
		taskDefinition.NextFireTime = nil
	}
	err = s.store.UpsertTaskDefinition(taskDefinition)
	if err != nil {
		logging.Log.WithError(err).WithFields(logrus.Fields{"id": taskDefinition.Id}).Error("error setting task definition next execution time")
	}
	return err
}

func (s *Scheduler) startTaskInstanceRunner() {
	ticker := time.NewTicker(*s.RunnerWindow)
	for range ticker.C {
		if s.run {
			s.scheduleTaskInstanceRuns()
		} else {
			ticker.Stop()
			break
		}
	}
}

func (s *Scheduler) scheduleTaskInstanceRuns() {
	// query for task instances that should be run
	taskInstances, err := s.store.GetTaskInstancesToRun(time.Now().Add(*s.ScheduleWindow))
	if err != nil {
		logging.Log.WithError(err).Error("error getting task instances to run")
		return
	}
	// handle each instance in a goroutine, the goroutine will sleep until its scheduled fire time
	for _, taskInstance := range taskInstances {
		go s.handleTaskInstance(taskInstance)
	}
}

func (s *Scheduler) handleTaskInstance(taskInstance TaskInstance) {
	// sleep until the execution time
	time.Sleep(time.Until(*taskInstance.ExecuteAt))
	// mark task in progress
	err := s.markTaskInstanceInProgress(taskInstance)
	if err != nil {
		return
	}
	// call handler
	err = s.Handler(taskInstance)
	if err == nil {
		// no error, mark instance completed
		err = s.store.MarkTaskInstanceComplete(taskInstance)
		if err != nil {
			logging.Log.WithError(err).WithFields(logrus.Fields{"task_instance_id": taskInstance.Id, "task_definition_id": taskInstance.TaskDefinition.Id}).Error("error setting task instance completed_at")
		}
	}
}

func (s *Scheduler) markTaskInstanceInProgress(taskInstance TaskInstance) error {
	startedAt := time.Now().UTC()
	expiresAt := startedAt.Add(taskInstance.TaskDefinition.ExpireAfter)
	taskInstance.StartedAt = &startedAt
	taskInstance.ExpiresAt = &expiresAt
	err := s.store.UpsertTaskInstance(taskInstance)
	if err != nil {
		logging.Log.WithError(err).WithFields(logrus.Fields{"task_instance_id": taskInstance.Id, "task_definition_id": taskInstance.TaskDefinition.Id}).Error("error setting task instance started_at")
	}
	return err
}

func (s *Scheduler) startTaskInstanceCleanup() {
	ticker := time.NewTicker(*s.CleanupWindow)
	for range ticker.C {
		if s.run {
			s.cleanUp()
		} else {
			ticker.Stop()
			break
		}
	}
}

// cleanUp() queries for completed task instances, then deletes the completed task instances, and then deletes
// the associated
func (s *Scheduler) cleanUp() {
	// delete completed task instances
	err := s.store.DeleteCompletedTaskInstances()
	if err != nil {
		logging.Log.WithError(err).Error("error deleting completed task instances")
	}
	// delete task definitions with no task instances
	err = s.store.DeleteCompletedTaskDefinitions()
	if err != nil {
		logging.Log.WithError(err).Error("error deleting completed task definitions")
	}
}

func (s *Scheduler) waitForOsSignal() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-signals
	logging.Log.WithFields(logrus.Fields{"signal": receivedSignal}).Info("scheduler exiting due to os signal")
	s.shutdown <- true
}

func (s *Scheduler) Stop() {
	s.shutdown <- true
}

func (s *Scheduler) shutDown() {
	s.run = false
	logging.Log.Info("scheduler stopped")
}

func validateTask(task TaskDefinition) error {
	if task.Id == nil {
		return errorx.IllegalArgument.New("tasks must have an id")
	}
	if task.Metadata == nil {
		return errorx.IllegalArgument.New("tasks must have metadata")
	}
	if task.GetTrigger() == nil {
		return errorx.IllegalArgument.New("tasks must have a trigger")
	}
	return nil
}

func (s *Scheduler) initializeStore() error {
	return s.store.Initialize()
}
