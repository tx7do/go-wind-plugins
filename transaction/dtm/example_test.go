package dtm_test

import (
	"net/url"

	"github.com/tx7do/go-wind-plugins/transaction/dtm"
)

type transferRequest struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

// ExampleNewClient constructs a DTM client against the transaction manager's
// address and assembles a saga on it. NewSaga opens a workflow under a
// globally unique gid; each Add pairs a participating service's forward
// endpoint with the endpoint that undoes it, and Submit ships the finished
// graph to the server, which drives the branches and — when a step fails —
// calls the undo endpoints of the completed ones in reverse order. The
// endpoints are ordinary HTTP handlers in the participating services, each
// running its branch against its own database.
func ExampleNewClient() {
	c := dtm.NewClient(
		dtm.WithServer("http://localhost:36789/api/dtmsvr"),
	)
	defer c.Close()

	payload := transferRequest{OrderID: "order-1", Amount: 100}

	s := c.NewSaga("transfer-001")
	_ = s.Add("/api/transfer/out", "/api/transfer/out/compensate", payload)
	_ = s.Add("/api/transfer/in", "/api/transfer/in/compensate", payload)
	if err := s.Submit(); err != nil {
		return
	}
}

// ExampleBarrierFromQuery is the participant side of the protocol: a branch
// endpoint rebuilds the branch barrier from the callback's query parameters.
// The barrier identifies the global transaction, the branch, and the
// requested operation; the endpoint passes it into its own database
// transaction — the one wrapping the service's gorm/ent repository writes —
// so a branch the server replays applies exactly once.
func ExampleBarrierFromQuery() {
	qs := url.Values{
		"trans_type": {"saga"},
		"gid":        {"transfer-001"},
		"branch_id":  {"br-1"},
		"op":         {"action"},
	}

	barrier, err := dtm.BarrierFromQuery(qs)
	if err != nil {
		return
	}
	_ = barrier
}
