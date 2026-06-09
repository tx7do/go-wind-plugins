package etcd

import (
	"encoding/json"

	wind "github.com/tx7do/go-wind"
)

func marshal(si *wind.Instance) (string, error) {
	data, err := json.Marshal(si)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshal(data []byte) (si *wind.Instance, err error) {
	err = json.Unmarshal(data, &si)
	return
}
