package cloudwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	bLogger "github.com/tx7do/go-wind/log"
)

// Compile-time assertion: cloudwatchLog implements bLogger.Logger.
var _ bLogger.Logger = (*cloudwatchLog)(nil)

// Logger 扩展了项目 Logger 接口，增加 CloudWatch 特有的方法。
type Logger interface {
	bLogger.Logger

	GetClient() *cloudwatchlogs.Client
	Close() error
}

type cloudwatchLog struct {
	client   *cloudwatchlogs.Client
	opts     *options
	extra    []any
	mu       sync.Mutex
	sequence *string // 当前序列 token，用于流式写入
	buffer   []types.InputLogEvent
	flushCtx context.Context
	cancel   context.CancelFunc
}

// GetClient 返回底层 CloudWatch Logs 客户端。
func (c *cloudwatchLog) GetClient() *cloudwatchlogs.Client {
	return c.client
}

// Close 刷新剩余缓冲日志并关闭后台刷新协程。
func (c *cloudwatchLog) Close() error {
	c.cancel()
	c.flush()
	return nil
}

// Debug 输出 DEBUG 级别日志。
func (c *cloudwatchLog) Debug(_ context.Context, msg string, keyvals ...any) {
	c.post("DEBUG", msg, keyvals)
}

// Info 输出 INFO 级别日志。
func (c *cloudwatchLog) Info(_ context.Context, msg string, keyvals ...any) {
	c.post("INFO", msg, keyvals)
}

// Warn 输出 WARN 级别日志。
func (c *cloudwatchLog) Warn(_ context.Context, msg string, keyvals ...any) {
	c.post("WARN", msg, keyvals)
}

// Error 输出 ERROR 级别日志。
func (c *cloudwatchLog) Error(_ context.Context, msg string, keyvals ...any) {
	c.post("ERROR", msg, keyvals)
}

// With 返回附加了指定 key-value 对的新 Logger 实例。
func (c *cloudwatchLog) With(keyvals ...any) bLogger.Logger {
	return &cloudwatchLog{
		client: c.client,
		opts:   c.opts,
		extra:  append(append([]any{}, c.extra...), keyvals...),
	}
}

// Enabled 报告给定级别是否会被输出。云端日志服务默认启用所有级别。
func (c *cloudwatchLog) Enabled(_ bLogger.Level) bool {
	return true
}

// post 将日志条目加入缓冲区，达到阈值后异步刷新到 CloudWatch。
func (c *cloudwatchLog) post(level, msg string, keyvals []any) {
	data := make(map[string]string, 3+len(c.extra)/2+len(keyvals)/2)
	data["level"] = level
	data["msg"] = msg

	all := append(append([]any{}, c.extra...), keyvals...)
	for i := 0; i+1 < len(all); i += 2 {
		data[toString(all[i])] = toString(all[i+1])
	}

	jsonBytes, _ := json.Marshal(data)

	event := types.InputLogEvent{
		Timestamp: aws.Int64(time.Now().UnixMilli()),
		Message:   aws.String(string(jsonBytes)),
	}

	c.mu.Lock()
	c.buffer = append(c.buffer, event)
	shouldFlush := len(c.buffer) >= c.opts.batchSize
	c.mu.Unlock()

	if shouldFlush {
		c.flush()
	}
}

// flush 将缓冲区中的日志批量发送到 CloudWatch Logs。
func (c *cloudwatchLog) flush() {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	events := c.buffer
	c.buffer = nil
	token := c.sequence
	c.mu.Unlock()

	input := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(c.opts.logGroup),
		LogStreamName: aws.String(c.opts.logStream),
		LogEvents:     events,
	}
	if token != nil {
		input.SequenceToken = token
	}

	resp, err := c.client.PutLogEvents(c.flushCtx, input)
	if err != nil {
		// 如果序列 token 不匹配，重新获取并重试一次
		if isSequenceTokenErr(err) {
			if seq, e := c.getSequenceToken(); e == nil && seq != nil {
				input.SequenceToken = seq
				resp, err = c.client.PutLogEvents(c.flushCtx, input)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "cloudwatch: PutLogEvents error: %v\n", err)
			return
		}
	}

	c.mu.Lock()
	c.sequence = resp.NextSequenceToken
	c.mu.Unlock()
}

// getSequenceToken 获取当前日志流的序列 token。
func (c *cloudwatchLog) getSequenceToken() (*string, error) {
	resp, err := c.client.DescribeLogStreams(c.flushCtx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(c.opts.logGroup),
		LogStreamNamePrefix: aws.String(c.opts.logStream),
	})
	if err != nil {
		return nil, err
	}
	for _, s := range resp.LogStreams {
		if s.LogStreamName != nil && *s.LogStreamName == c.opts.logStream {
			return s.UploadSequenceToken, nil
		}
	}
	return nil, nil
}

// isSequenceTokenErr 判断是否为序列 token 错误。
func isSequenceTokenErr(err error) bool {
	var te *types.InvalidSequenceTokenException
	if stdErr(err, &te) {
		return true
	}
	return strings.Contains(err.Error(), "InvalidSequenceTokenException")
}

// stdErr 是 errors.As 的简写。
func stdErr(err error, target any) bool {
	return errors.As(err, target)
}

// NewCloudWatchLogger 创建 AWS CloudWatch Logs 日志记录器。
//
// 在首次写入前会自动创建日志组和日志流（如果不存在）。
func NewCloudWatchLogger(ctx context.Context, opts ...Option) (Logger, error) {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.region),
	)
	if err != nil {
		return nil, fmt.Errorf("cloudwatch: load aws config: %w", err)
	}

	client := cloudwatchlogs.NewFromConfig(awsCfg)

	flushCtx, cancel := context.WithCancel(context.Background())

	l := &cloudwatchLog{
		client:   client,
		opts:     cfg,
		flushCtx: flushCtx,
		cancel:   cancel,
	}

	// 确保日志组和流存在
	if err := l.ensureLogGroup(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("cloudwatch: ensure log group: %w", err)
	}
	if err := l.ensureLogStream(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("cloudwatch: ensure log stream: %w", err)
	}

	// 启动定时刷新协程
	if cfg.flushInterval > 0 {
		go l.autoFlush()
	}

	return l, nil
}

// ensureLogGroup 创建日志组（如不存在）。
func (c *cloudwatchLog) ensureLogGroup(ctx context.Context) error {
	_, err := c.client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(c.opts.logGroup),
	})
	if err != nil {
		if !isAlreadyExists(err) {
			return err
		}
	}
	return nil
}

// ensureLogStream 创建日志流（如不存在）。
func (c *cloudwatchLog) ensureLogStream(ctx context.Context) error {
	_, err := c.client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(c.opts.logGroup),
		LogStreamName: aws.String(c.opts.logStream),
	})
	if err != nil {
		if !isAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func isAlreadyExists(err error) bool {
	var ae *types.ResourceAlreadyExistsException
	if stdErr(err, &ae) {
		return true
	}
	return strings.Contains(err.Error(), "ResourceAlreadyExistsException")
}

// autoFlush 定时刷新缓冲区。
func (c *cloudwatchLog) autoFlush() {
	ticker := time.NewTicker(c.opts.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.flush()
		case <-c.flushCtx.Done():
			return
		}
	}
}

// toString 将任意类型转换为字符串。
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch v := v.(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		return strconv.Itoa(v)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case int8:
		return strconv.Itoa(int(v))
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case int16:
		return strconv.Itoa(int(v))
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case int32:
		return strconv.Itoa(int(v))
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
