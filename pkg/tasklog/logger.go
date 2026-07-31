// Package tasklog 提供与传输协议无关的任务日志缓冲器。
package tasklog

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultBufferSize   = 10
	DefaultFlushPeriod  = 3 * time.Second
	DefaultFlushTimeout = 10 * time.Second
)

//go:generate mockgen -source=./logger.go -package=tasklogmocks -destination=./mocks/logger.mock.go -typed Sink

// Sink 接收一批任务日志，Kafka 序号等传输元数据由具体 Sink 管理。
type Sink interface {
	// WriteBatch 发送一个已聚合的日志批次；返回错误表示该批次未被接收，Logger 会保留并在后续刷新时重试。
	WriteBatch(ctx context.Context, logs []string) error
}

// Options 控制日志批量刷新行为。
type Options struct {
	BufferSize   int
	FlushPeriod  time.Duration
	FlushTimeout time.Duration
	OnError      func(error)
}

// Logger 按容量或时间批量刷新任务日志。
type Logger struct {
	ctx    context.Context
	sink   Sink
	config Options

	mu     sync.Mutex
	buffer []string
	closed bool

	ticker    *time.Ticker
	flushCh   chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func New(ctx context.Context, sink Sink, options Options) *Logger {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.BufferSize <= 0 {
		options.BufferSize = DefaultBufferSize
	}
	if options.FlushPeriod <= 0 {
		options.FlushPeriod = DefaultFlushPeriod
	}
	if options.FlushTimeout <= 0 {
		options.FlushTimeout = DefaultFlushTimeout
	}
	l := &Logger{
		ctx: ctx, sink: sink, config: options,
		buffer:  make([]string, 0, options.BufferSize),
		ticker:  time.NewTicker(options.FlushPeriod),
		flushCh: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.loop()
	}()
	return l
}

func (l *Logger) Log(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.buffer = append(l.buffer, message)
	flush := len(l.buffer) >= l.config.BufferSize
	l.mu.Unlock()

	if flush {
		select {
		case l.flushCh <- struct{}{}:
		default:
		}
	}
}

// Close 在最后一次刷新后停止后台协程。
// 发送失败的日志可通过 PendingLogs 获取，由外层协议继续兜底。
func (l *Logger) Close() {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		close(l.done)
		l.wg.Wait()
	})
}

// PendingLogs 返回尚未被 Sink 接收的日志。
func (l *Logger) PendingLogs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.buffer...)
}

func (l *Logger) loop() {
	defer l.ticker.Stop()
	for {
		select {
		case <-l.ticker.C:
			l.flush()
		case <-l.flushCh:
			l.flush()
		case <-l.done:
			l.flush()
			return
		}
	}
}

func (l *Logger) flush() {
	l.mu.Lock()
	if len(l.buffer) == 0 {
		l.mu.Unlock()
		return
	}
	logs := l.buffer
	l.buffer = make([]string, 0, l.config.BufferSize)
	l.mu.Unlock()

	if l.sink == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(l.ctx), l.config.FlushTimeout)
	err := l.sink.WriteBatch(ctx, logs)
	cancel()
	if err == nil {
		return
	}

	l.mu.Lock()
	l.buffer = append(logs, l.buffer...)
	l.mu.Unlock()
	if l.config.OnError != nil {
		l.config.OnError(err)
	}
}
