package apollo

import (
	stdjson "encoding/json"
	"testing"

	"github.com/apolloconfig/agollo/v4/storage"
)

func Test_onChange(t *testing.T) {
	s := map[string]struct {
		Name string `json:"name"`
	}{
		"app": {
			Name: "new",
		},
	}
	expectedJSON, _ := stdjson.Marshal(s)
	c := valueChangeListener{}
	tests := []struct {
		name      string
		namespace string
		changes   map[string]*storage.ConfigChange
		want      []byte
	}{
		{
			"test json onChange",
			"app.yaml",
			map[string]*storage.ConfigChange{
				"name": {
					OldValue:   "old",
					NewValue:   "new",
					ChangeType: storage.MODIFIED,
				},
			},
			expectedJSON,
		},
		{
			"test origin content onChange",
			"app.json",
			map[string]*storage.ConfigChange{
				"content": {
					OldValue:   `{"name":"old"}`,
					NewValue:   `{"name":"new"}`,
					ChangeType: storage.MODIFIED,
				},
			},
			[]byte(`{"name":"new"}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.onChange(tt.namespace, tt.changes)
			if got == nil {
				t.Errorf("onChange() returned nil")
				return
			}
			if string(got) != string(tt.want) {
				t.Errorf("onChange() = %s, want %s", string(got), string(tt.want))
			}
		})
	}
}
