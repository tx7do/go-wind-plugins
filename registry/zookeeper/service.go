package zookeeper

import (
	"encoding/json"

	wind "github.com/tx7do/go-wind"
)

func marshal(si *wind.Instance) ([]byte, error) {
	return json.Marshal(si)
}

func unmarshal(data []byte) (si *wind.Instance, err error) {
	err = json.Unmarshal(data, &si)
	return
}
