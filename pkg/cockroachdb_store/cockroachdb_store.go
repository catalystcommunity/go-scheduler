package cockroachdb_store

import (
	"context"
	"database/sql"
	"embed"
	"time"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/catalystcommunity/go-scheduler/pkg/cockroachdb_store/models"
	"github.com/cockroachdb/cockroach-go/v2/crdb/crdbgorm"
	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const gooseTableName = "goose_catalyst_scheduler"

//go:embed migrations/*.sql
var migrations embed.FS

type CockroachdbStore struct {
	uri    string
	db     *gorm.DB
	config *gorm.Config

	maxIdleConns    *int
	maxOpenConns    *int
	connMaxLifetime *time.Duration
}

func NewCockroachdbStore(uri string, config *gorm.Config, opts ...CockroachdbStoreOpt) pkg.StoreInterface {
	if config == nil {
		config = &gorm.Config{}
	}

	c := &CockroachdbStore{
		uri:    uri,
		config: config,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

type CockroachdbStoreOpt func(*CockroachdbStore)

func WithConnectionSettings(maxIdleConns, maxOpenConns int, connMaxLifetime time.Duration) CockroachdbStoreOpt {
	return func(c *CockroachdbStore) {
		c.maxIdleConns = &maxIdleConns
		c.maxOpenConns = &maxOpenConns
		c.connMaxLifetime = &connMaxLifetime
	}
}

func (c *CockroachdbStore) Initialize() (err error) {
	// connect to db
	c.db, err = gorm.Open(postgres.Open(c.uri), c.config)
	if err != nil {
		logging.Log.WithError(err).Error("error connecting to cockroachdb")
		return err
	}
	// run migrations
	var sqldb *sql.DB
	sqldb, err = c.db.DB()
	if err != nil {
		return err
	}

	// set max idle conns if provided
	if c.maxIdleConns != nil {
		sqldb.SetMaxIdleConns(*c.maxIdleConns)
	} else {
		sqldb.SetMaxIdleConns(10)
	}

	// set max open conns and max conn lifetime
	if c.maxOpenConns != nil {
		sqldb.SetMaxOpenConns(*c.maxOpenConns)
	} else {
		sqldb.SetMaxOpenConns(100)
	}

	if c.connMaxLifetime != nil {
		sqldb.SetConnMaxLifetime(*c.connMaxLifetime)
	} else {
		sqldb.SetConnMaxLifetime(time.Hour)
	}

	// set goose file system to use the embedded migrations
	goose.SetBaseFS(migrations)
	// set goose table name so it doesn't conflict with any other goose tables that the user may be using
	goose.SetTableName(gooseTableName)
	err = goose.Up(sqldb, "migrations")
	if err != nil {
		logging.Log.WithError(err).Error("error running scheduler migrations")
		return err
	}
	return nil
}

func (c *CockroachdbStore) DeleteTaskDefinitionsByMetadata(metadataQuery interface{}) error {
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		// query for task definitions that aren't completed, whose next fire time is less than the limit
		return tx.Where(metadataQuery).Delete(&models.TaskDefinition{}).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error deleting task definitions by metadata query")
	}
	return err
}

func (c *CockroachdbStore) GetTaskDefinitions(ids []*uuid.UUID) ([]pkg.TaskDefinition, error) {
	definitions := []models.TaskDefinition{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		// query for task definitions that aren't completed, whose next fire time is less than the limit
		return tx.Preload(clause.Associations).Find(&definitions, ids).Error
	})
	if err != nil {
		return nil, err
	}
	return models.ToTaskDefinitions(definitions)
}

func (c *CockroachdbStore) DeleteTaskDefinitions(ids []*uuid.UUID) error {
	return crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		// query for task definitions that aren't completed, whose next fire time is less than the limit
		return tx.Delete([]models.TaskDefinition{}, ids).Error
	})
}

func (c *CockroachdbStore) GetTaskDefinitionsToSchedule(limit time.Time) ([]pkg.TaskDefinition, error) {
	limit = limit.UTC()
	taskDefinitionModels := []models.TaskDefinition{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		// query for task definitions that aren't completed, whose next fire time is less than the limit
		return tx.Preload(clause.Associations).Where("completed_at is null and next_fire_time is not null and next_fire_time <= ?", limit).Find(&taskDefinitionModels).Error
	})
	if err != nil {
		return nil, err
	}
	taskDefinitions := []pkg.TaskDefinition{}
	for _, taskDefinitionModel := range taskDefinitionModels {
		taskInstance, err := taskDefinitionModel.ToTaskDefinition()
		if err != nil {
			return nil, err
		}
		taskDefinitions = append(taskDefinitions, taskInstance)
	}
	return taskDefinitions, nil
}

func (c *CockroachdbStore) MarkTaskInstanceComplete(taskInstance pkg.TaskInstance) error {
	completedAt := time.Now().UTC()
	return crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		// if the parent task definition is not recurring, this marks it as completed in a single query
		err := tx.Model(&models.TaskDefinition{}).Where("id = ? and recurring = false", taskInstance.TaskDefinition.Id).Update("completed_at", completedAt).Error
		if err != nil {
			logging.Log.WithError(err).Error("error marking task definition complete")
			return err
		}
		err = tx.Omit("TaskDefinition").Model(&models.TaskInstance{}).Where("id = ?", taskInstance.Id).Update("completed_at", completedAt).Error
		if err != nil {
			logging.Log.WithError(err).Error("error marking task instance complete")
		}
		return err
	})
}

func (c *CockroachdbStore) DeleteCompletedTaskInstances() error {
	return crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		err := tx.Where("completed_at is not null").Delete(&models.TaskInstance{}).Error
		if err != nil {
			logging.Log.WithError(err).Error("error deleting completed task instances")
		}
		return err
	})
}

func (c *CockroachdbStore) DeleteCompletedTaskDefinitions() error {
	return crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		err := tx.Where("completed_at is not null").Delete(&models.TaskDefinition{}).Error
		if err != nil {
			logging.Log.WithError(err).Error("error deleting completed task definitions")
		}
		return err
	})
}

func (c *CockroachdbStore) ListCompletedTaskInstances() ([]pkg.TaskInstance, error) {
	taskInstanceModels := []models.TaskInstance{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Where("completed_at is not null").Find(&taskInstanceModels).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error listing completed task instances")
	}
	return models.ToTaskInstances(taskInstanceModels)
}

func (c *CockroachdbStore) ListCompletedTaskDefinitions() ([]pkg.TaskDefinition, error) {
	taskDefinitionModels := []models.TaskDefinition{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Where("completed_at is not null").Find(&taskDefinitionModels).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error listing completed task definitions")
	}
	return models.ToTaskDefinitions(taskDefinitionModels)
}

func (c *CockroachdbStore) GetTaskInstance(id *uuid.UUID) (pkg.TaskInstance, error) {
	taskInstanceModel := models.TaskInstance{Id: id}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.First(&taskInstanceModel).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error getting task definition")
		return pkg.TaskInstance{}, err
	}
	return taskInstanceModel.ToTaskInstance()
}

func (c *CockroachdbStore) ListTaskInstances(offset, limit int) ([]pkg.TaskInstance, error) {
	taskInstanceModels := []models.TaskInstance{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Preload(clause.Associations).Order("created_at").Offset(offset).Limit(limit).Find(&taskInstanceModels).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error listing task instances")
	}
	return models.ToTaskInstances(taskInstanceModels)
}

func (c *CockroachdbStore) ListTaskDefinitions(offset, limit int, metadataQuery interface{}) ([]pkg.TaskDefinition, error) {
	taskDefinitionModels := []models.TaskDefinition{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		tx = tx.Preload(clause.Associations).Order("created_at").Offset(offset).Limit(limit)
		if metadataQuery != nil {
			tx = tx.Where(metadataQuery)
		}
		return tx.Find(&taskDefinitionModels).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error scheduling task with cockroachdb store")
	}
	return models.ToTaskDefinitions(taskDefinitionModels)
}

func (c *CockroachdbStore) GetTaskDefinition(id *uuid.UUID) (pkg.TaskDefinition, error) {
	taskDefinitionModel := models.TaskDefinition{Id: id}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Preload(clause.Associations).First(&taskDefinitionModel).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error getting task definition with cockroachdb store")
		return pkg.TaskDefinition{}, err
	}
	return taskDefinitionModel.ToTaskDefinition()
}

func (c *CockroachdbStore) DeleteTaskDefinition(id *uuid.UUID) error {
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Delete(models.TaskDefinition{Id: id}).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error deleting task with cockroachdb store")
	}
	return err
}

func (c *CockroachdbStore) GetTaskInstancesToRun(limit time.Time) ([]pkg.TaskInstance, error) {
	limit = limit.UTC()
	taskInstanceModels := []models.TaskInstance{}
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		// query for task instances that aren't completed, and either aren't in progress, or are in progress but have expired
		return tx.Preload(clause.Associations).Where("completed_at is null and ((started_at is null and execute_at <= ?) or (started_at is not null and expires_at <= now()))", limit).Find(&taskInstanceModels).Error
	})
	if err != nil {
		return nil, err
	}
	taskInstances := []pkg.TaskInstance{}
	for _, taskInstanceModel := range taskInstanceModels {
		taskInstance, err := taskInstanceModel.ToTaskInstance()
		if err != nil {
			return nil, err
		}
		taskInstances = append(taskInstances, taskInstance)
	}
	return taskInstances, nil
}

func (c *CockroachdbStore) UpsertTaskInstance(taskInstance pkg.TaskInstance) error {
	logging.Log.Info("upserting task instance")
	taskInstanceModel, err := models.GetTaskInstanceModelFromTaskInstance(taskInstance)
	if err != nil {
		return err
	}
	err = crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Omit("TaskDefinition").Clauses(clause.OnConflict{
			UpdateAll: true,
		}).Create(&taskInstanceModel).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error creating task instance")
	}
	return err
}

func (c *CockroachdbStore) DeleteTaskInstance(id *uuid.UUID) error {
	err := crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Delete(models.TaskInstance{Id: id}).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error deleting task instance")
	}
	return err
}

func (c *CockroachdbStore) UpsertTaskDefinition(taskDefinition pkg.TaskDefinition) error {
	logging.Log.Info("upserting task definition")
	taskDefinitionModel, err := models.GetTaskDefinitionModelFromTaskDefinition(taskDefinition)
	taskDefinitionModel.TaskInstances = nil
	if err != nil {
		return err
	}
	err = crdbgorm.ExecuteTx(context.Background(), c.db, nil, func(tx *gorm.DB) error {
		return tx.Omit("TaskInstances").Clauses(clause.OnConflict{
			UpdateAll: true,
		}).Create(&taskDefinitionModel).Error
	})
	if err != nil {
		logging.Log.WithError(err).Error("error upserting task with cockroachdb store")
	}
	return err
}
