package cron

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// skipWithoutIntegration skips tests that need a live external server.
func skipWithoutIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("KRATOS_IT") == "" {
		t.Skip("skipping integration test: requires a live server; set KRATOS_IT to enable")
	}
}

func TestServer(t *testing.T) {
	skipWithoutIntegration(t)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// 创建 cron 服务
	srv := NewServer()

	// 在 Start 前注册任务（推荐）
	// 每 10 秒执行一次
	_, _ = srv.NewTimerJob("*/10 * * * * *", func() {
		log.Println("task run every 10 seconds")
	})

	// 每分钟执行一次
	_, _ = srv.NewTimerJob("0 */1 * * * *", func() {
		log.Println("task run every minute")
	})

	// 每 5 秒执行一次（描述符写法）
	_, _ = srv.NewTimerJob("@every 5s", func() {
		log.Println("task run every 5 seconds (descriptor)")
	})

	log.Printf("registered %d jobs", srv.GetJobCount())

	defer func() {
		if err := srv.Stop(t.Context()); err != nil {
			t.Errorf("expected nil got %v", err)
		}
	}()

	if err := srv.Start(t.Context()); err != nil {
		panic(err)
	}

	<-interrupt
}

func TestAddRemoveJob(t *testing.T) {
	srv := NewServer()

	// 添加任务
	id1, err := srv.NewTimerJob("*/5 * * * * *", func() {
		log.Println("job 1")
	})
	if err != nil {
		t.Errorf("expected nil got %v", err)
	}

	id2, err := srv.NewTimerJob("*/10 * * * * *", func() {
		log.Println("job 2")
	})
	if err != nil {
		t.Errorf("expected nil got %v", err)
	}

	if srv.GetJobCount() != 2 {
		t.Errorf("expected 2 jobs, got %d", srv.GetJobCount())
	}

	// 移除任务
	srv.RemoveTimerJob(id1)
	if srv.GetJobCount() != 1 {
		t.Errorf("expected 1 job, got %d", srv.GetJobCount())
	}

	// 移除所有
	srv.RemoveAllJobs()
	if srv.GetJobCount() != 0 {
		t.Errorf("expected 0 jobs, got %d", srv.GetJobCount())
	}

	_ = id2
}

func TestServerStartStop(t *testing.T) {
	srv := NewServer()

	_, _ = srv.NewTimerJob("@every 1s", func() {
		// no-op
	})

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()

	if err := srv.Start(ctx); err != nil {
		t.Errorf("expected nil got %v", err)
	}

	log.Println("server started and stopped successfully")
}

func TestEntries(t *testing.T) {
	srv := NewServer()

	_, _ = srv.NewTimerJob("*/5 * * * * *", func() {})
	_, _ = srv.NewTimerJob("*/10 * * * * *", func() {})

	entries := srv.GetEntries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	log.Printf("entries: %+v", entries)
}
