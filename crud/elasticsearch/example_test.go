package elasticsearch_test

import (
	"github.com/tx7do/go-wind-plugins/crud/elasticsearch"
)

// ExampleNewElasticsearchClient constructs an Elasticsearch client from
// endpoint and credential options. In the data access layer the client's
// document, search, and index-management helpers operate on the configured
// cluster.
func ExampleNewElasticsearchClient() {
	_, err := elasticsearch.NewElasticsearchClient(
		elasticsearch.WithAddresses("http://localhost:9200"),
		elasticsearch.WithUsername("demo-user"),
		elasticsearch.WithPassword("demo-password"),
	)
	if err != nil {
		return
	}
}
