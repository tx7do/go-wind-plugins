package broker_test

import (
	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewOptionsAndApply builds the option set shared by every broker
// plugin: the address list of the broker to connect to and the codec used
// for message payloads. The configured codec drives Marshal and Unmarshal,
// the encode and decode helpers each plugin runs for every published or
// consumed message, shown here as a payload round-trip.
func ExampleNewOptionsAndApply() {
	opts := broker.NewOptionsAndApply(
		broker.WithAddress("localhost:9092"),
		broker.WithCodec("json"),
	)

	payload := &userEvent{UserID: "1", Action: "created"}

	encoded, err := broker.Marshal(opts.Codec, payload)
	if err != nil {
		return
	}

	var decoded userEvent
	if err := broker.Unmarshal(opts.Codec, encoded, &decoded); err != nil {
		return
	}
	_ = decoded
}
