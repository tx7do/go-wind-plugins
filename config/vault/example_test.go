package vault_test

import (
	"context"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/tx7do/go-wind-plugins/config/vault"
)

// ExampleNew constructs a HashiCorp Vault-backed configuration source. Load
// reads the secret at the configured path and extracts the configured data
// field from it; WatchValue polls the secret and delivers changed values on
// the returned channel.
func ExampleNew() {
	client, err := vaultapi.NewClient(&vaultapi.Config{
		Address: "http://127.0.0.1:8200",
	})
	if err != nil {
		return
	}

	src, err := vault.New(client,
		vault.WithPath("secret/data/myapp/config"),
	)
	if err != nil {
		return
	}

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		return
	}
	_ = ch
}
