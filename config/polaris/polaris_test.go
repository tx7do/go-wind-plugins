package polaris

import (
	"context"
	"sync"
	"testing"
	"time"

	polaris "github.com/polarismesh/polaris-go"
	polarisapi "github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
)

// ---------------------------------------------------------------------------
// Mocks for the polaris ConfigAPI / ConfigFile interfaces
// ---------------------------------------------------------------------------

// mockConfigFile implements model.ConfigFile with fixed metadata and content.
type mockConfigFile struct {
	namespace string
	fileGroup string
	fileName  string
	content   string

	mu        sync.Mutex
	listeners []model.OnConfigFileChange
}

func (m *mockConfigFile) GetNamespace() string                        { return m.namespace }
func (m *mockConfigFile) GetFileGroup() string                        { return m.fileGroup }
func (m *mockConfigFile) GetFileName() string                         { return m.fileName }
func (m *mockConfigFile) GetFileMode() model.GetConfigFileRequestMode { return 0 }
func (m *mockConfigFile) GetLabels() map[string]string                { return nil }
func (m *mockConfigFile) GetContent() string                          { return m.content }
func (m *mockConfigFile) HasContent() bool                            { return m.content != "" }
func (m *mockConfigFile) AddChangeListenerWithChannel() <-chan model.ConfigFileChangeEvent {
	return nil
}
func (m *mockConfigFile) GetPersistent() model.Persistent { return model.Persistent{} }
func (m *mockConfigFile) GetVersionName() string          { return "" }
func (m *mockConfigFile) GetVersion() uint64              { return 0 }
func (m *mockConfigFile) GetMd5() string                  { return "" }

func (m *mockConfigFile) AddChangeListener(cb model.OnConfigFileChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, cb)
}

// notify dispatches a change event to all registered listeners.
func (m *mockConfigFile) notify(event model.ConfigFileChangeEvent) {
	m.mu.Lock()
	listeners := make([]model.OnConfigFileChange, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.Unlock()

	for _, cb := range listeners {
		cb(event)
	}
}

// mockConfigAPI implements polaris.ConfigAPI, returning a fixed config file
// and error. It records the arguments of the last GetConfigFile call.
type mockConfigAPI struct {
	file *mockConfigFile
	err  error

	mu         sync.Mutex
	lastNS     string
	lastGroup  string
	lastFile   string
	getCalls   int
	fileExists bool
}

func (m *mockConfigAPI) SDKContext() polarisapi.SDKContext { return nil }

func (m *mockConfigAPI) GetConfigFile(namespace, fileGroup, fileName string) (model.ConfigFile, error) {
	m.mu.Lock()
	m.lastNS, m.lastGroup, m.lastFile = namespace, fileGroup, fileName
	m.getCalls++
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}
	if m.fileExists || m.file != nil {
		return m.file, nil
	}
	return nil, nil
}

func (m *mockConfigAPI) FetchConfigFile(_ *polaris.GetConfigFileRequest) (model.ConfigFile, error) {
	return nil, nil
}
func (m *mockConfigAPI) CreateConfigFile(_, _, _, _ string) error { return nil }
func (m *mockConfigAPI) UpdateConfigFile(_, _, _, _ string) error { return nil }
func (m *mockConfigAPI) PublishConfigFile(_, _, _ string) error   { return nil }
func (m *mockConfigAPI) UpsertAndPublishConfigFile(_, _, _, _ string) error {
	return nil
}

func (m *mockConfigAPI) lastGetConfigFileArgs() (ns, group, file string, calls int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastNS, m.lastGroup, m.lastFile, m.getCalls
}

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNew_NilClient(t *testing.T) {
	if _, err := New(nil, WithFileGroup("application"), WithFileName("app.properties")); err == nil {
		t.Fatal("New with a nil client should fail")
	}
}

func TestNew_MissingFileGroup(t *testing.T) {
	if _, err := New(&mockConfigAPI{}, WithFileName("app.properties")); err == nil {
		t.Fatal("New without fileGroup should fail")
	}
}

func TestNew_MissingFileName(t *testing.T) {
	if _, err := New(&mockConfigAPI{}, WithFileGroup("application")); err == nil {
		t.Fatal("New without fileName should fail")
	}
}

func TestNew_SuccessWithDefaults(t *testing.T) {
	s, err := New(&mockConfigAPI{}, WithFileGroup("application"), WithFileName("app.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil source")
	}
	// The namespace defaults to "default".
	if s.options.namespace != "default" {
		t.Errorf("default namespace = %q, want %q", s.options.namespace, "default")
	}
	if s.options.fileGroup != "application" {
		t.Errorf("fileGroup = %q, want %q", s.options.fileGroup, "application")
	}
	if s.options.fileName != "app.properties" {
		t.Errorf("fileName = %q, want %q", s.options.fileName, "app.properties")
	}
}

func TestOptions(t *testing.T) {
	o := &options{}

	WithNamespace("prod")(o)
	if o.namespace != "prod" {
		t.Errorf("WithNamespace set namespace = %q, want %q", o.namespace, "prod")
	}

	WithFileGroup("group")(o)
	if o.fileGroup != "group" {
		t.Errorf("WithFileGroup set fileGroup = %q, want %q", o.fileGroup, "group")
	}

	WithFileName("file")(o)
	if o.fileName != "file" {
		t.Errorf("WithFileName set fileName = %q, want %q", o.fileName, "file")
	}
}

// ---------------------------------------------------------------------------
// resolveFileName
// ---------------------------------------------------------------------------

func TestResolveFileName(t *testing.T) {
	tests := []struct {
		name           string
		configuredFile string
		key            string
		want           string
	}{
		{"explicit key wins", "default.properties", "other.properties", "other.properties"},
		{"empty key falls back", "default.properties", "", "default.properties"},
		{"both empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &source{options: &options{fileName: tt.configuredFile}}
			if got := s.resolveFileName(tt.key); got != tt.want {
				t.Errorf("resolveFileName(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad_Success(t *testing.T) {
	file := &mockConfigFile{
		namespace: "default",
		fileGroup: "application",
		fileName:  "app.properties",
		content:   "key=value",
	}
	api := &mockConfigAPI{file: file}

	s, err := New(api, WithFileGroup("application"), WithFileName("app.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "key=value" {
		t.Errorf("Load = %q, want %q", string(data), "key=value")
	}

	ns, group, name, calls := api.lastGetConfigFileArgs()
	if calls != 1 {
		t.Fatalf("GetConfigFile calls = %d, want 1", calls)
	}
	if ns != "default" || group != "application" || name != "app.properties" {
		t.Errorf("GetConfigFile args = (%q, %q, %q), want (default, application, app.properties)",
			ns, group, name)
	}

	// Load stores the fetched config file on the source options.
	if s.options.configFile == nil {
		t.Error("Load should store the fetched config file in options")
	}
}

func TestLoad_ExplicitKeyOverridesFileName(t *testing.T) {
	file := &mockConfigFile{content: "content"}
	api := &mockConfigAPI{file: file}

	s, err := New(api, WithFileGroup("application"), WithFileName("app.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := s.Load(context.Background(), "other.properties"); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	_, _, fileArg, _ := api.lastGetConfigFileArgs()
	if fileArg != "other.properties" {
		t.Errorf("GetConfigFile fileName arg = %q, want %q", fileArg, "other.properties")
	}
}

func TestLoad_Error(t *testing.T) {
	api := &mockConfigAPI{err: context.DeadlineExceeded}

	s, err := New(api, WithFileGroup("application"), WithFileName("app.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err == nil {
		t.Fatal("Load should propagate the GetConfigFile error")
	}
	if data != nil {
		t.Errorf("Load = %q, want nil on error", string(data))
	}
}

// ---------------------------------------------------------------------------
// WatchValue
// ---------------------------------------------------------------------------

func TestWatchValue_DeliversChangeEvents(t *testing.T) {
	file := &mockConfigFile{
		namespace: "default",
		fileGroup: "application",
		fileName:  "watch-delivers.properties",
		content:   "v0",
	}
	api := &mockConfigAPI{file: file}

	s, err := New(api, WithFileGroup("application"), WithFileName("watch-delivers.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := s.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue returned error: %v", err)
	}
	if ch == nil {
		t.Fatal("WatchValue returned nil channel")
	}

	// Fire a change event through the registered listener.
	file.notify(model.ConfigFileChangeEvent{
		ConfigFileMetadata: file,
		NewValue:           "v1",
	})

	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatal("watch channel closed before any value was delivered")
		}
		if string(got) != "v1" {
			t.Errorf("watch value = %q, want %q", string(got), "v1")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not deliver the change event within 5s")
	}

	// A second event must be delivered as well.
	file.notify(model.ConfigFileChangeEvent{
		ConfigFileMetadata: file,
		NewValue:           "v2",
	})

	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatal("watch channel closed before the second value was delivered")
		}
		if string(got) != "v2" {
			t.Errorf("watch value = %q, want %q", string(got), "v2")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not deliver the second event within 5s")
	}

	// Cancelling the context must close the channel.
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("watch channel should be closed after ctx cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Error("watch channel was not closed within 5s after ctx cancellation")
	}
}

func TestWatchValue_GetConfigFileError(t *testing.T) {
	api := &mockConfigAPI{err: context.DeadlineExceeded}

	s, err := New(api, WithFileGroup("application"), WithFileName("app.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = s.WatchValue(context.Background(), "")
	if err == nil {
		t.Fatal("WatchValue should propagate the GetConfigFile error")
	}
}

func TestWatchValue_ExplicitKeyOverridesFileName(t *testing.T) {
	file := &mockConfigFile{
		namespace: "default",
		fileGroup: "application",
		fileName:  "watch-explicit.properties",
	}
	api := &mockConfigAPI{file: file}

	s, err := New(api, WithFileGroup("application"), WithFileName("app.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := s.WatchValue(ctx, "watch-explicit.properties"); err != nil {
		t.Fatalf("WatchValue returned error: %v", err)
	}

	_, _, fileArg, _ := api.lastGetConfigFileArgs()
	if fileArg != "watch-explicit.properties" {
		t.Errorf("GetConfigFile fileName arg = %q, want %q", fileArg, "watch-explicit.properties")
	}
}

// Two concurrent watchers on the same file must be independent: cancelling one
// must not stall or close the other.
func TestWatchValue_ConcurrentWatchersIndependent(t *testing.T) {
	file := &mockConfigFile{
		namespace: "default",
		fileGroup: "application",
		fileName:  "watch-concurrent.properties",
		content:   "v0",
	}
	api := &mockConfigAPI{file: file}

	s, err := New(api, WithFileGroup("application"), WithFileName("watch-concurrent.properties"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	ch1, err := s.WatchValue(ctx1, "")
	if err != nil {
		t.Fatalf("first WatchValue returned error: %v", err)
	}
	ch2, err := s.WatchValue(ctx2, "")
	if err != nil {
		t.Fatalf("second WatchValue returned error: %v", err)
	}

	// Both watchers must receive the same event.
	file.notify(model.ConfigFileChangeEvent{ConfigFileMetadata: file, NewValue: "v1"})
	for i, ch := range []<-chan []byte{ch1, ch2} {
		select {
		case got, ok := <-ch:
			if !ok {
				t.Fatalf("watcher %d channel closed before delivery", i)
			}
			if string(got) != "v1" {
				t.Errorf("watcher %d value = %q, want %q", i, string(got), "v1")
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("watcher %d did not receive the event within 5s", i)
		}
	}

	// Cancelling the first watcher must leave the second fully functional.
	cancel1()

	file.notify(model.ConfigFileChangeEvent{ConfigFileMetadata: file, NewValue: "v2"})
	select {
	case got, ok := <-ch2:
		if !ok {
			t.Fatal("second watcher channel was closed by the first watcher's cancellation")
		}
		if string(got) != "v2" {
			t.Errorf("second watcher value = %q, want %q", string(got), "v2")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second watcher stalled after the first watcher was cancelled")
	}
}
