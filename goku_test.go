package goku

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestJob struct {
	Foo int
	Bar string
}

func (tj TestJob) Name() string {
	return "test_job"
}

var tjWasCalled bool

func (tj TestJob) Execute(_ TimeoutChan) error {
	tjWasCalled = true
	return nil
}

type TestJobWithTimeout struct {
	Foo int
	Bar string
}

func (tj TestJobWithTimeout) Name() string {
	return "test_job_with_timeout"
}

var tjwtWasCalled bool
var slowJobDone bool

func (tj TestJobWithTimeout) Execute(timeoutChan TimeoutChan) error {
	tjwtWasCalled = true
	slowOperationChan := make(chan struct{})

	go func() {
		time.Sleep(time.Second * 5)
		close(slowOperationChan)
	}()

	select {
	case <-slowOperationChan:
		slowJobDone = true
	case <-timeoutChan:
		slowJobDone = false
	}

	return nil
}

func TestBroker(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	hostport := "127.0.0.1:6379"
	queueName := "goku_test"

	broker, err := NewBroker(BrokerConfig{
		Hostport:     hostport,
		Timeout:      time.Second,
		DefaultQueue: queueName,
	})

	require.NoError(err)

	job := TestJob{
		Foo: 4,
		Bar: "sup",
	}

	err = broker.Run(job)
	assert.NoError(err)

	conn, err := redis.Dial("tcp", hostport)
	require.NoError(err)

	jsn, err := redis.Bytes(conn.Do("LPOP", queueName))
	assert.NoError(err)

	var m map[string]interface{}
	json.Unmarshal(jsn, &m)
	args := m["A"].(map[string]interface{})

	assert.Equal(m["N"], job.Name())
	assert.Equal(args["Foo"], float64(4))
	assert.Equal(args["Bar"], "sup")
}

func TestRun(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	queue := "goku_test"
	hostport := "127.0.0.1:6379"

	config := WorkerConfig{
		NumWorkers: 1,
		Queues:     []string{queue},
		Hostport:   hostport,
		Timeout:    time.Second,
	}

	opts := WorkerPoolOptions{
		Failure: nil,
		Jobs: []Job{
			TestJob{},
		},
	}

	// start the worker
	wp, err := NewWorkerPool(config, opts)
	assert.NoError(err)
	wp.Start()

	tjWasCalled = false

	// schedule the job from the broker
	broker, err := NewBroker(BrokerConfig{
		Hostport:     hostport,
		Timeout:      time.Second,
		DefaultQueue: queue,
	})

	require.NoError(err)

	job := TestJob{
		Foo: 4,
		Bar: "sup",
	}

	err = broker.Run(job)
	assert.NoError(err)
	time.Sleep(time.Second) // give workers some time to pull the job out of the queue
	wp.Stop()

	assert.True(tjWasCalled)
}

func TestRunWithJobTimeout(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	queue := "goku_test"
	hostport := "127.0.0.1:6379"

	config := WorkerConfig{
		NumWorkers: 1,
		Queues:     []string{queue},
		Hostport:   hostport,
		Timeout:    time.Second,
		jobTimeout: time.Second,
	}

	opts := WorkerPoolOptions{
		Failure: nil,
		Jobs: []Job{
			TestJobWithTimeout{},
		},
	}

	// start the worker
	wp, err := NewWorkerPool(config, opts)
	assert.NoError(err)
	wp.Start()

	tjwtWasCalled = false
	tjWasCalled = false

	// schedule the job from the broker
	broker, err := NewBroker(BrokerConfig{
		Hostport:     hostport,
		Timeout:      time.Second,
		DefaultQueue: queue,
	})

	require.NoError(err)

	job := TestJobWithTimeout{
		Foo: 4,
		Bar: "sup",
	}

	err = broker.Run(job)
	assert.NoError(err)
	time.Sleep(time.Second) // give workers some time to pull the job out of the queue
	wp.Stop()

	assert.True(tjwtWasCalled)
	assert.False(slowJobDone)
}

func TestBrokerBadConfig(t *testing.T) {
	_, err := NewBroker(BrokerConfig{})
	assert.Error(t, err)
}

func TestWorkerPoolBadConfig(t *testing.T) {
	_, err := NewWorkerPool(WorkerConfig{}, WorkerPoolOptions{})
	assert.Error(t, err)
}

func TestConfigureBadConfig(t *testing.T) {
	err := Configure(BrokerConfig{})
	assert.Error(t, err)
}

func TestConfigureGoodConfig(t *testing.T) {
	hostport := "127.0.0.1:6379"
	queueName := "goku_test"
	err := Configure(BrokerConfig{
		Hostport:     hostport,
		Timeout:      time.Second,
		DefaultQueue: queueName,
	})
	assert.NoError(t, err)
}

func TestRunWithPtr(t *testing.T) {
	hostport := "127.0.0.1:6379"
	queueName := "goku_test"
	err := Configure(BrokerConfig{
		Hostport:     hostport,
		Timeout:      time.Second,
		DefaultQueue: queueName,
	})
	require.NoError(t, err)

	err = Run(&TestJob{})
	assert.Equal(t, ErrPointer, err)
}

func TestRunAt(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	queue := "goku_test"
	hostport := "127.0.0.1:6379"

	config := WorkerConfig{
		NumWorkers: 1,
		Queues:     []string{queue},
		Hostport:   hostport,
		Timeout:    time.Second,
	}

	opts := WorkerPoolOptions{
		Failure: nil,
		Jobs: []Job{
			TestJob{},
		},
	}

	// start the worker
	wp, err := NewWorkerPool(config, opts)
	assert.NoError(err)
	wp.Start()

	// schedule the job from the broker
	broker, err := NewBroker(BrokerConfig{
		Hostport:     hostport,
		Timeout:      time.Second,
		DefaultQueue: queue,
	})

	require.NoError(err)

	job := TestJob{
		Foo: 4,
		Bar: "sup",
	}

	tjWasCalled = false
	err = broker.RunAt(job, time.Now().Add(3*time.Second))
	assert.NoError(err)

	// give workers some time to pull the job out of the queue
	time.Sleep(2 * time.Second)
	assert.False(tjWasCalled)

	time.Sleep(2 * time.Second)
	wp.Stop()
	assert.True(tjWasCalled)
}
