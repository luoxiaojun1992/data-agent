package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// enqueueIndex is a best-effort async index enqueue. A nil queue must be a
// no-op (no panic); an enqueue error must be swallowed (logged, not returned).

func TestService_enqueueIndex_NilQueue(t *testing.T) {
	s := NewService(nil)
	s.enqueueIndex(context.Background(), "doc_1", "file_1") // must not panic
}

func TestService_enqueueIndex_Success(t *testing.T) {
	queue := mockrepo.NewQueueRepository(t)
	queue.On("EnqueueRaw", mock.Anything, "kb_index", mock.Anything).Return(nil)

	s := NewService(nil)
	s.queue = queue
	s.enqueueIndex(context.Background(), "doc_1", "file_1")

	queue.AssertCalled(t, "EnqueueRaw", mock.Anything, "kb_index", mock.Anything)
}

func TestService_enqueueIndex_Error(t *testing.T) {
	queue := mockrepo.NewQueueRepository(t)
	queue.On("EnqueueRaw", mock.Anything, "kb_index", mock.Anything).Return(errors.New("redis down"))

	s := NewService(nil)
	s.queue = queue
	s.enqueueIndex(context.Background(), "doc_1", "file_1") // must not panic
}
