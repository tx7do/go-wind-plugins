package opensearch_test

import (
	"github.com/tx7do/go-wind-plugins/crud/opensearch"
)

// ExampleNewOpenSearchClient constructs an OpenSearch data-access client from
// a list of cluster addresses. In an application's data access layer the
// client owns the search connection; the document, index and search helpers
// built on top of it then issue the actual requests, which requires the live
// connection established here.
func ExampleNewOpenSearchClient() {
	client, err := opensearch.NewOpenSearchClient(
		opensearch.WithAddresses("https://host:port"),
	)
	if err != nil {
		return
	}

	_ = client
}
