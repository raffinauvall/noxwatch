package runner

import (
	"testing"

	"github.com/raffinauvall/noxwatch/agent/internal/collect"
)

func TestBoundedQueue(t *testing.T) {
	queue := make([]collect.Payload, 0, maxQueue)
	for sequence := int64(1); sequence <= maxQueue+2; sequence++ {
		queue = appendBounded(queue, collect.Payload{Sequence: sequence})
	}
	if len(queue) != maxQueue || queue[0].Sequence != 3 || queue[maxQueue-1].Sequence != maxQueue+2 {
		t.Fatalf("unexpected bounded queue: len=%d first=%d last=%d", len(queue), queue[0].Sequence, queue[maxQueue-1].Sequence)
	}
}
