package test

import (
	"fmt"
	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/go-scheduler/pkg"
	"github.com/catalystcommunity/go-scheduler/pkg/cockroachdb_store"
	"github.com/google/uuid"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/cockroachdb"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

var cockroachdbContainer *gnomock.Container
var cockroachdbStore pkg.StoreInterface
var scheduler pkg.Scheduler

const dbName = "test"
const dbPort = 26257

type CockroachdbStoreSuite struct {
	suite.Suite
}

func (s *CockroachdbStoreSuite) SetupSuite() {
	var err error
	namedPorts := gnomock.NamedPorts{
		gnomock.DefaultPort: gnomock.Port{
			Protocol: "tcp",
			Port:     dbPort,
			HostPort: dbPort,
		},
	}
	preset := cockroachdb.Preset(cockroachdb.WithDatabase(dbName))
	cockroachdbContainer, err = gnomock.Start(preset, gnomock.WithCustomNamedPorts(namedPorts))
	require.NoError(s.T(), err)
	uri := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		cockroachdbContainer.Host,
		cockroachdbContainer.DefaultPort(),
		dbName,
		"root",
		"",
	)
	cockroachdbStore = cockroachdb_store.NewCockroachdbStore(uri, nil)
	err = cockroachdbStore.Initialize()
	require.NoError(s.T(), err)
	logging.Log.WithFields(logrus.Fields{"uri": uri}).Info("suite set up")
}

func (s *CockroachdbStoreSuite) TearDownSuite() {
	err := gnomock.Stop(cockroachdbContainer)
	if err != nil {
		logging.Log.WithError(err).Error("error stopping cockroachdb test container")
	}
}

func (s *CockroachdbStoreSuite) SetupTest() {
	// delete all before each test
	require.NoError(s.T(), deleteAllTaskDefinitions(cockroachdbStore))
}

func TestCockroachdbStoreSuite(t *testing.T) {
	suite.Run(t, new(CockroachdbStoreSuite))
}

func (s *CockroachdbStoreSuite) TestCockroachdbStoreHappyPath() {
	TestExecuteOnceTriggerHappyPath(s.T(), cockroachdbStore)
}

func (s *CockroachdbStoreSuite) TestCockroachdbStoreTasksRunInOrder() {
	TestExecuteOnceTriggerTasksRunInOrder(s.T(), cockroachdbStore)
}

func (s *CockroachdbStoreSuite) TestCockroachdbStoreLongRunningTaskExpired() {
	TestExecuteOnceTriggerLongRunningTaskExpired(s.T(), cockroachdbStore)
}

func (s *CockroachdbStoreSuite) TestCockroachdbStoreLongRunningTaskNotExpired() {
	TestExecuteOnceTriggerLongRunningTaskNotExpired(s.T(), cockroachdbStore)
}

func (s *CockroachdbStoreSuite) TestCockroachdbStoreCronTriggerHappyPath() {
	TestCronTriggerHappyPath(s.T(), cockroachdbStore)
}

func (s *CockroachdbStoreSuite) TestListWithMetadataQuery() {
	id := uuid.New().String()
	metadata := map[string]interface{}{"user_id": id}
	metadataQuery := fmt.Sprintf(`metadata @> '{"user_id": "%s"}'`, id)
	TestListWithMetadataQuery(s.T(), cockroachdbStore, metadata, metadataQuery)
}

func (s *CockroachdbStoreSuite) TestDeleteWithMetadataQuery() {
	id := uuid.New().String()
	metadata := map[string]interface{}{"user_id": id}
	metadataQuery := fmt.Sprintf(`metadata @> '{"user_id": "%s"}'`, id)
	TestDeleteWithMetadataQuery(s.T(), cockroachdbStore, metadata, metadataQuery)
}

func (s *CockroachdbStoreSuite) TestSingleTaskDefinitionCreatedForCronTasks() {
	TestCronTriggerHappyPath(s.T(), cockroachdbStore)
	definitions, err := cockroachdbStore.ListTaskDefinitions(0, 1000, nil)
	require.NoError(s.T(), err)
	require.Len(s.T(), definitions, 1)
}
