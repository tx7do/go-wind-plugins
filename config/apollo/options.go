package apollo

import (
	"github.com/apolloconfig/agollo/v4/constant"
	"github.com/apolloconfig/agollo/v4/extension"
)

// Option is apollo option
type Option func(*options)

type options struct {
	appid          string
	secret         string
	cluster        string
	endpoint       string
	namespace      string
	isBackupConfig bool
	backupPath     string
	originConfig   bool
	mustStart      bool
}

// WithMustStart requires the first config sync to succeed. Without it a
// startup failure is stored in the source and surfaced from Load/WatchValue
// instead of panicking inside NewSource.
func WithMustStart() Option {
	return func(o *options) {
		o.mustStart = true
	}
}

// WithAppID with apollo config app id
func WithAppID(appID string) Option {
	return func(o *options) {
		o.appid = appID
	}
}

// WithCluster with apollo config cluster
func WithCluster(cluster string) Option {
	return func(o *options) {
		o.cluster = cluster
	}
}

// WithEndpoint with apollo config conf server ip
func WithEndpoint(endpoint string) Option {
	return func(o *options) {
		o.endpoint = endpoint
	}
}

// WithEnableBackup with apollo config enable backup config
func WithEnableBackup() Option {
	return func(o *options) {
		o.isBackupConfig = true
	}
}

// WithDisableBackup with apollo config enable backup config
func WithDisableBackup() Option {
	return func(o *options) {
		o.isBackupConfig = false
	}
}

// WithSecret with apollo config app secret
func WithSecret(secret string) Option {
	return func(o *options) {
		o.secret = secret
	}
}

// WithNamespace with apollo config namespace name
func WithNamespace(name string) Option {
	return func(o *options) {
		o.namespace = name
	}
}

// WithBackupPath with apollo config backupPath
func WithBackupPath(backupPath string) Option {
	return func(o *options) {
		o.backupPath = backupPath
	}
}

// WithOriginalConfig use the original configuration file without parse processing
func WithOriginalConfig() Option {
	return func(o *options) {
		extension.AddFormatParser(constant.JSON, &jsonExtParser{})
		extension.AddFormatParser(constant.YAML, &yamlExtParser{})
		extension.AddFormatParser(constant.YML, &yamlExtParser{})
		o.originConfig = true
	}
}
