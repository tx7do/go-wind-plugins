package polaris

import (
	"fmt"

	"github.com/polarismesh/polaris-go/pkg/model"
)

// eventChan wraps a channel that receives polaris config file change events.
type eventChan struct {
	closed bool
	event  chan model.ConfigFileChangeEvent
}

// eventChanMap maps full paths (namespace/fileGroup/fileName) to their event
// channels. It is used to route change notifications from the global callback
// to the correct watcher.
var eventChanMap = make(map[string]eventChan)

// getFullPath builds a unique key for a config file.
func getFullPath(namespace string, fileGroup string, fileName string) string {
	return fmt.Sprintf("%s/%s/%s", namespace, fileGroup, fileName)
}
