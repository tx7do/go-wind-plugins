package machinery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/RichardKnop/machinery/v2"
	"github.com/RichardKnop/machinery/v2/config"
	machineryLog "github.com/RichardKnop/machinery/v2/log"
	"github.com/RichardKnop/machinery/v2/tasks"

	amqpBackend "github.com/RichardKnop/machinery/v2/backends/amqp"
	dynamoBackend "github.com/RichardKnop/machinery/v2/backends/dynamodb"
	eagerBackend "github.com/RichardKnop/machinery/v2/backends/eager"
	ifaceBackend "github.com/RichardKnop/machinery/v2/backends/iface"
	memcacheBackend "github.com/RichardKnop/machinery/v2/backends/memcache"
	mongoBackend "github.com/RichardKnop/machinery/v2/backends/mongo"
	redisBackend "github.com/RichardKnop/machinery/v2/backends/redis"

	amqpBroker "github.com/RichardKnop/machinery/v2/brokers/amqp"
	eagerBroker "github.com/RichardKnop/machinery/v2/brokers/eager"
	gcppubsubBroker "github.com/RichardKnop/machinery/v2/brokers/gcppubsub"
	ifaceBroker "github.com/RichardKnop/machinery/v2/brokers/iface"
	redisBroker "github.com/RichardKnop/machinery/v2/brokers/redis"
	sqsBroker "github.com/RichardKnop/machinery/v2/brokers/sqs"

	eagerLock "github.com/RichardKnop/machinery/v2/locks/eager"
	ifaceLock "github.com/RichardKnop/machinery/v2/locks/iface"
	redisLock "github.com/RichardKnop/machinery/v2/locks/redis"

	"github.com/tx7do/go-wind/log"
)

const (
	TracerMessageSystemKey = "machinery"
	SpanNameProducer       = "machinery-producer"
	SpanNameConsumer       = "machinery-consumer"
)

type Server struct {
	sync.RWMutex
	started atomic.Bool

	baseCtx context.Context
	err     error

	machineryServer *machinery.Server
	cfg             *config.Config

	brokerOption   brokerOption
	backendOption  backendOption
	lockOption     lockOption
	consumerOption consumerOption
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		baseCtx: context.Background(),
		started: atomic.Bool{},

		cfg: &config.Config{
			DefaultQueue:    "wind_machinery_queue",
			ResultsExpireIn: 3600,

			AMQP: &config.AMQPConfig{},
			SQS:  &config.SQSConfig{},
			Redis: &config.RedisConfig{
				MaxIdle:                3,
				IdleTimeout:            240,
				ReadTimeout:            15,
				WriteTimeout:           15,
				ConnectTimeout:         15,
				NormalTasksPollPeriod:  1000,
				DelayedTasksPollPeriod: 500,
			},
			GCPPubSub: &config.GCPPubSubConfig{},
			MongoDB:   &config.MongoDBConfig{},
			DynamoDB:  &config.DynamoDBConfig{},
		},
		consumerOption: consumerOption{
			consumerTag: "wind_machinery_worker",
			concurrency: 1,
			queue:       "wind_machinery_queue",
		},
		brokerOption: brokerOption{
			brokerType: BrokerTypeRedis,
			db:         0,
		},
		backendOption: backendOption{
			backendType: BackendTypeRedis,
			db:          0,
		},
		lockOption: lockOption{
			lockType: LockTypeRedis,
			db:       0,
			retries:  1,
		},
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...ServerOption) {
	for _, o := range opts {
		o(s)
	}

	s.installLogger()

	s.createMachineryServer()
}

func (s *Server) Name() string {
	return KindMachinery
}

func (s *Server) HandleFunc(name string, handler any) error {
	if err := s.registerTask(name, handler); err != nil {
		return err
	}
	return nil
}

// NewTask enqueue a new task
func (s *Server) NewTask(ctx context.Context, typeName string, opts ...TaskOption) error {
	return s.newTask(ctx, "", "", typeName, opts...)
}

// NewPeriodicTask 周期性定时任务，不支持秒级任务，最大精度只到分钟。
func (s *Server) NewPeriodicTask(ctx context.Context, cronSpec, typeName string, opts ...TaskOption) error {
	return s.newTask(ctx, cronSpec, typeName, typeName, opts...)
}

// NewGroup 执行一组异步任务，任务之间互不影响。
func (s *Server) NewGroup(groupTasks ...TasksOption) error {
	return s.newGroup("", "", 0, groupTasks...)
}

func (s *Server) NewPeriodicGroup(cronSpec string, groupTasks ...TasksOption) error {
	return s.newGroup(cronSpec, "periodic-group", 0, groupTasks...)
}

// NewChord 先执行一组同步任务，执行完成后，再调用最后一个回调函数。
func (s *Server) NewChord(chordTasks ...TasksOption) error {
	return s.newChord("", "", 0, chordTasks...)
}

func (s *Server) NewPeriodicChord(cronSpec string, chordTasks ...TasksOption) error {
	return s.newChord(cronSpec, "periodic-chord", 0, chordTasks...)
}

// NewChain 执行一组同步任务，任务有次序之分，上个任务的出参可作为下个任务的入参。
func (s *Server) NewChain(chainTasks ...TasksOption) error {
	return s.newChain("", "", chainTasks...)
}

func (s *Server) NewPeriodicChain(cronSpec string, chainTasks ...TasksOption) error {
	return s.newChain(cronSpec, "periodic-chain", chainTasks...)
}

func (s *Server) Start(ctx context.Context) error {
	if s.err != nil {
		return s.err
	}

	if s.started.Load() {
		return nil
	}

	if err := s.newWorker(
		s.consumerOption.consumerTag,
		s.consumerOption.concurrency,
		s.consumerOption.queue,
	); err != nil && !errors.Is(err, machinery.ErrWorkerQuitGracefully) {
		return err
	}

	LogInfof("server started")

	s.baseCtx = ctx
	s.started.Store(true)

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	LogInfo("server stopping...")

	s.started.Store(false)

	s.machineryServer = nil
	s.err = nil

	LogInfo("server stopped.")

	return nil
}

// installLogger 安装日志记录器
func (s *Server) installLogger() {
	machineryLog.SetDebug(newLogger(log.LevelDebug))
	machineryLog.SetInfo(newLogger(log.LevelInfo))
	machineryLog.SetWarning(newLogger(log.LevelWarn))
	machineryLog.SetError(newLogger(log.LevelError))
	machineryLog.SetFatal(newLogger(log.LevelError))
}

func (s *Server) createMachineryServer() {
	var broker ifaceBroker.Broker
	var backend ifaceBackend.Backend
	var lock ifaceLock.Lock

	var err error

	if s.cfg.Broker != "" {
		switch s.brokerOption.brokerType {
		case BrokerTypeRedis:
			broker = redisBroker.NewGR(s.cfg, []string{s.cfg.Broker}, s.brokerOption.db)
			break
		case BrokerTypeAmqp:
			broker = amqpBroker.New(s.cfg)
			break
		case BrokerTypeGcpPubSub:
			if broker, err = gcppubsubBroker.New(s.cfg, s.brokerOption.projectID, s.brokerOption.subscriptionName); err != nil {
				LogError("create GCP PubSub broker error:", err)
			}
			break
		case BrokerTypeSQS:
			if broker, err = sqsBroker.New(s.cfg); err != nil {
				LogError("create SQS broker error:", err)
			}
			break
		}
	}

	if s.cfg.ResultBackend != "" {
		switch s.backendOption.backendType {
		case BackendTypeRedis:
			backend = redisBackend.NewGR(s.cfg, []string{s.cfg.ResultBackend}, s.backendOption.db)
			break
		case BackendTypeAmqp:
			backend = amqpBackend.New(s.cfg)
			break
		case BackendTypeMemcache:
			backend = memcacheBackend.New(s.cfg, []string{s.cfg.ResultBackend})
			break
		case BackendTypeMongoDB:
			if backend, err = mongoBackend.New(s.cfg); err != nil {
				LogError("create MongoDB backend error:", err)
			}
			break
		case BackendTypeDynamoDB:
			if backend, err = dynamoBackend.New(s.cfg); err != nil {
				LogError("create DynamoDB backend error:", err)
			}
			break
		}
	}

	if s.cfg.Lock != "" {
		switch s.lockOption.lockType {
		case LockTypeRedis:
			lock = redisLock.New(s.cfg, []string{s.cfg.Lock}, s.lockOption.db, s.lockOption.retries)
			break
		}
	}

	if broker == nil {
		broker = eagerBroker.New()
	}
	if backend == nil {
		backend = eagerBackend.New()
	}
	if lock == nil {
		lock = eagerLock.New()
	}

	s.machineryServer = machinery.NewServer(s.cfg, broker, backend, lock)
}

func (s *Server) registerTask(name string, handler any) error {
	if err := s.machineryServer.RegisterTask(name, handler); err != nil {
		return err
	}
	return nil
}

func (s *Server) newWorker(consumerTag string, concurrency int, queue string) error {
	worker := s.machineryServer.NewCustomQueueWorker(consumerTag, concurrency, queue)
	if worker == nil {
		return errors.New("create worker failed")
	}

	worker.SetPreTaskHandler(func(signature *tasks.Signature) {

	})

	return worker.Launch()
}

func (s *Server) newTask(ctx context.Context, cronSpec, lockName, typeName string, opts ...TaskOption) error {
	signature := &tasks.Signature{
		Name: typeName,
	}

	for _, o := range opts {
		o(signature)
	}

	var err error

	if len(cronSpec) > 0 {
		err = s.machineryServer.RegisterPeriodicTask(cronSpec, lockName, signature)
		if err != nil {
			return err
		}
	} else {
		_, err = s.machineryServer.SendTask(signature)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) newGroup(cronSpec, lockName string, concurrency int, groupTasks ...TasksOption) error {
	if len(groupTasks) == 0 {
		return errors.New("group task is empty")
	}

	var signatures = make([]*tasks.Signature, 0, len(groupTasks))

	for _, o := range groupTasks {
		o(&signatures)
	}

	if len(signatures) == 0 {
		return errors.New("group task is empty")
	}

	var err error

	if len(cronSpec) > 0 {
		if err := s.machineryServer.RegisterPeriodicGroup(cronSpec, lockName, concurrency, signatures...); err != nil {
			return err
		}
	} else {

		var group *tasks.Group
		group, err = tasks.NewGroup(signatures...)
		if err != nil {
			return err
		}

		_, err = s.machineryServer.SendGroup(group, concurrency)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) newChord(cronSpec, lockName string, concurrency int, groupTasks ...TasksOption) error {
	if len(groupTasks) < 2 {
		return errors.New("chord task is empty")
	}

	var signatures = make([]*tasks.Signature, 0, len(groupTasks))

	for _, o := range groupTasks {
		o(&signatures)
	}

	var finalSignature *tasks.Signature
	finalSignature, signatures = signatures[len(signatures)-1], signatures[:len(signatures)-1]

	var err error

	if len(cronSpec) > 0 {
		if err = s.machineryServer.RegisterPeriodicChord(cronSpec, lockName, concurrency, finalSignature, signatures...); err != nil {
			return err
		}
	} else {
		var group *tasks.Group
		group, err = tasks.NewGroup(signatures...)
		if err != nil {
			return err
		}

		var chord *tasks.Chord
		chord, err = tasks.NewChord(group, finalSignature)
		if err != nil {
			return err
		}

		_, err = s.machineryServer.SendChord(chord, concurrency)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) newChain(cronSpec, lockName string, chainTasks ...TasksOption) error {
	if len(chainTasks) == 0 {
		return errors.New("chain task is empty")
	}

	var signatures = make([]*tasks.Signature, 0, len(chainTasks))

	for _, o := range chainTasks {
		o(&signatures)
	}

	if len(signatures) == 0 {
		return errors.New("chain task is empty")
	}

	var err error

	if len(cronSpec) > 0 {
		if err = s.machineryServer.RegisterPeriodicChain(cronSpec, lockName, signatures...); err != nil {
			return err
		}
	} else {
		var chain *tasks.Chain
		chain, err = tasks.NewChain(signatures...)
		if err != nil {
			return err
		}

		_, err = s.machineryServer.SendChain(chain)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) Endpoint() string {
	return ""
}
