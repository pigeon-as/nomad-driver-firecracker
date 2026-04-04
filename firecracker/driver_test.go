package firecracker

import (
	"regexp"
	"testing"

	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

func TestJailerID(t *testing.T) {
	jailerRegex := regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

	tests := []struct {
		name    string
		allocID string
		task    string
	}{
		{"simple", "abc12345-1234-1234-1234-123456789012", "web"},
		{"long task name", "abc12345-1234-1234-1234-123456789012", "my-very-long-task-name-that-is-still-valid"},
		{"special chars", "abc12345-1234-1234-1234-123456789012", "task_with.special/chars!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &drivers.TaskConfig{AllocID: tt.allocID, Name: tt.task}
			id := jailerID(cfg)

			must.True(t, jailerRegex.MatchString(id), must.Sprintf("jailerID(%q, %q) = %q, does not match %s", tt.allocID, tt.task, id, jailerRegex))
			must.True(t, len(id) <= 64, must.Sprintf("jailerID(%q, %q) = %q, length %d exceeds 64", tt.allocID, tt.task, id, len(id)))
		})
	}
}

func TestJailerID_UniquePerTask(t *testing.T) {
	allocID := "abc12345-1234-1234-1234-123456789012"
	id1 := jailerID(&drivers.TaskConfig{AllocID: allocID, Name: "web"})
	id2 := jailerID(&drivers.TaskConfig{AllocID: allocID, Name: "sidecar"})

	must.NotEq(t, id1, id2)
}

func TestJailerID_UniquePerAlloc(t *testing.T) {
	id1 := jailerID(&drivers.TaskConfig{AllocID: "aaaaaaaa-1111-1111-1111-111111111111", Name: "web"})
	id2 := jailerID(&drivers.TaskConfig{AllocID: "bbbbbbbb-2222-2222-2222-222222222222", Name: "web"})

	must.NotEq(t, id1, id2)
}

func TestJailerID_Deterministic(t *testing.T) {
	cfg := &drivers.TaskConfig{AllocID: "abc12345-1234-1234-1234-123456789012", Name: "web"}
	id1 := jailerID(cfg)
	id2 := jailerID(cfg)

	must.EqOp(t, id1, id2)
}
