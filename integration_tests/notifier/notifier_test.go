package notifier

import (
	"github.com/golang/mock/gomock"
	"github.com/moira-alert/moira-alert"
	"github.com/moira-alert/moira-alert/database/redis"
	"github.com/moira-alert/moira-alert/metrics/graphite/go-metrics"
	"github.com/moira-alert/moira-alert/mock/moira-alert"
	"github.com/moira-alert/moira-alert/notifier"
	"github.com/moira-alert/moira-alert/notifier/events"
	"github.com/moira-alert/moira-alert/notifier/notifications"
	"github.com/op/go-logging"
	"sync"
	"testing"
	"time"
)

var senderSettings = map[string]string{
	"type": "mega-sender",
}

var notifierConfig = notifier.Config{
	SendingTimeout:   time.Millisecond * 10,
	ResendingTimeout: time.Hour * 24,
}

var shutdown = make(chan bool)
var waitGroup sync.WaitGroup

var notifierMetrics = metrics.ConfigureNotifierMetrics()
var databaseMetrics = metrics.ConfigureDatabaseMetrics()
var logger, _ = logging.GetLogger("Notifier_Test")
var mockCtrl *gomock.Controller

var contact = moira.ContactData{
	ID:    "ContactID-000000000000001",
	Type:  "mega-sender",
	Value: "mail1@example.com",
}

var subscription = moira.SubscriptionData{
	ID:                "subscriptionID-00000000000001",
	Enabled:           true,
	Tags:              []string{"test-tag-1"},
	Contacts:          []string{contact.ID},
	ThrottlingEnabled: true,
}

var trigger = moira.Trigger{
	ID:      "triggerID-0000000000001",
	Name:    "test trigger 1",
	Targets: []string{"test.target.1"},
	Tags:    []string{"test-tag-1"},
}

var triggerData = moira.TriggerData{
	ID:      "triggerID-0000000000001",
	Name:    "test trigger 1",
	Targets: []string{"test.target.1"},
	Tags:    []string{"test-tag-1"},
}

var event = moira.NotificationEvent{
	Metric:    "generate.event.1",
	State:     "OK",
	OldState:  "WARN",
	TriggerID: "triggerID-0000000000001",
}

func TestNotifier(t *testing.T) {
	mockCtrl = gomock.NewController(t)
	defer afterTest()
	database := redis.NewDatabase(logger, redis.Config{Port: "6379", Host: "localhost"}, databaseMetrics)
	database.WriteContact(&contact)
	database.SaveSubscription(&subscription)
	database.SaveTrigger(trigger.ID, &trigger)
	database.PushEvent(&event, true)
	notifier2 := notifier.NewNotifier(database, logger, notifierConfig, notifierMetrics)
	sender := mock_moira_alert.NewMockSender(mockCtrl)
	sender.EXPECT().Init(senderSettings, logger).Return(nil)
	notifier2.RegisterSender(senderSettings, sender)
	sender.EXPECT().SendEvents(gomock.Any(), contact, triggerData, false).Return(nil).Do(func(f ...interface{}) {
		logger.Debugf("SendEvents called. End test")
		close(shutdown)
	})

	fetchEventsWorker := events.FetchEventsWorker{
		Database:  database,
		Logger:    logger,
		Metrics:   notifierMetrics,
		Scheduler: notifier.NewScheduler(database, logger),
	}

	fetchNotificationsWorker := notifications.FetchNotificationsWorker{
		Database: database,
		Logger:   logger,
		Notifier: notifier2,
	}

	fetchEventsWorker.Start()
	fetchNotificationsWorker.Start()

	waitTestEnd()

	fetchEventsWorker.Stop()
	fetchNotificationsWorker.Stop()
}

func waitTestEnd() {
	select {
	case <-shutdown:
		break
	case <-time.After(time.Second * 30):
		logger.Debugf("Test timeout")
		close(shutdown)
		break
	}
}

func afterTest() {
	waitGroup.Wait()
	mockCtrl.Finish()
}
