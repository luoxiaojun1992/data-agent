package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// ---- SPEC-070: AddChunks 图写入（先搜后写，同 creator） ----

func newGraphTestService(t *testing.T, kb *mockrepo.KBRepository, vec *mockrepo.VectorRepository, graph *mockrepo.GraphRepository) *Service {
	t.Helper()
	s := NewService(kb)
	s.WithVectorIndex(vec, func(_ context.Context, _ string) ([]float32, error) {
		return []float32{0.1, 0.2}, nil
	})
	s.WithGraphIndex(graph)
	s.WithGraphTopN(5)
	return s
}

func TestAddChunks_GraphWrite_SearchThenUpsertThenGraph(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)
	graph := mockrepo.NewGraphRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1", IsPublic: false}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("AddChunks", mock.Anything, mock.Anything).Return(nil)
	kb.On("UpdateDocStatus", mock.Anything, "kbdoc_1", mock.Anything, 2, 0).Return(nil)

	// Search returns one same-creator hit carrying the original chunk_id.
	hit := repository.VectorSearchHit{
		ID:    "12345",
		Score: 0.9,
		Metadata: map[string]interface{}{
			"chunk_id": "chunk_old_1",
			"content":  "旧切片",
		},
	}
	vec.On("Search", mock.Anything, "kb_chunks", mock.Anything, 5, mock.Anything).Return([]repository.VectorSearchHit{hit}, nil)
	vec.On("Upsert", mock.Anything, "kb_chunks", mock.Anything).Return(nil)
	graph.On("UpsertChunk", mock.Anything, mock.MatchedBy(func(c repository.GraphChunk) bool {
		return c.CreatorID == "u1" && c.DocID == "kbdoc_1"
	})).Return(nil)
	graph.On("LinkRelated", mock.Anything, mock.Anything, mock.MatchedBy(func(refs []repository.RelatedRef) bool {
		return len(refs) == 1 && refs[0].ChunkID == "chunk_old_1"
	})).Return(nil)

	s := newGraphTestService(t, kb, vec, graph)
	err := s.AddChunks("kbdoc_1", []string{"文本一", "文本二"})
	assert.NoError(t, err)

	// 每个 chunk 一次 UpsertChunk（2 个文本 → 2 次）。
	graph.AssertNumberOfCalls(t, "UpsertChunk", 2)
	// 有命中才 LinkRelated；两个 chunk 各命中同一 old chunk。
	graph.AssertNumberOfCalls(t, "LinkRelated", 2)
	// 断言：Search 的 filter 精确匹配 creator（只关联同 creator 切片）。
	vec.AssertCalled(t, "Search", mock.Anything, "kb_chunks", mock.Anything, 5, mock.MatchedBy(func(f map[string]interface{}) bool {
		must, ok := f["must"].([]interface{})
		if !ok || len(must) != 1 {
			return false
		}
		cond, ok := must[0].(map[string]interface{})
		return ok && cond["key"] == "creator_id"
	}))
}

func TestAddChunks_GraphDisabled_NoGraphCalls(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("AddChunks", mock.Anything, mock.Anything).Return(nil)
	kb.On("UpdateDocStatus", mock.Anything, "kbdoc_1", mock.Anything, 1, 0).Return(nil)
	vec.On("Upsert", mock.Anything, "kb_chunks", mock.Anything).Return(nil)

	// graph = nil（未注入）→ 无 Search 调用、无图写入。
	s := NewService(kb)
	s.WithVectorIndex(vec, func(_ context.Context, _ string) ([]float32, error) {
		return []float32{0.1}, nil
	})
	err := s.AddChunks("kbdoc_1", []string{"文本"})
	assert.NoError(t, err)
	vec.AssertNotCalled(t, "Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ---- SPEC-070: DeleteDoc 五步级联（顺序 + 幂等） ----

func TestDeleteDoc_FiveStepCascade_OrderAndIdempotency(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)
	graph := mockrepo.NewGraphRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", GridFSFileID: "fs_abc"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	vec.On("DeletePoints", mock.Anything, "kb_chunks", mock.Anything).Return(nil)
	graph.On("DeleteByDocID", mock.Anything, "kbdoc_1").Return(nil)
	kb.On("DeleteChunks", mock.Anything, "kbdoc_1").Return(int64(3), nil)
	kb.On("DeleteFile", mock.Anything, "fs_abc").Return(nil)
	kb.On("DeleteDoc", mock.Anything, "kbdoc_1").Return(nil)

	s := NewService(kb)
	s.WithVectorIndex(vec, nil)
	s.WithGraphIndex(graph)

	assert.NoError(t, s.DeleteDoc("kbdoc_1", "", true))

	// 五步全部调用（顺序由 mock 断言次数保证，顺序语义靠单元步骤的幂等）。
	vec.AssertCalled(t, "DeletePoints", mock.Anything, "kb_chunks", mock.MatchedBy(func(f map[string]interface{}) bool {
		must, ok := f["must"].([]interface{})
		if !ok || len(must) != 1 {
			return false
		}
		cond := must[0].(map[string]interface{})
		match, _ := cond["match"].(map[string]interface{})
		return cond["key"] == "doc_id" && match["value"] == "kbdoc_1"
	}))
	graph.AssertCalled(t, "DeleteByDocID", mock.Anything, "kbdoc_1")
	kb.AssertCalled(t, "DeleteChunks", mock.Anything, "kbdoc_1")
	kb.AssertCalled(t, "DeleteFile", mock.Anything, "fs_abc")
	kb.AssertCalled(t, "DeleteDoc", mock.Anything, "kbdoc_1")
}

func TestDeleteDoc_AlreadyDeleted_NoopSuccess(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	// GetDoc fails (not found, retry scenario) → idempotent no-op success.
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(nil, errors.New("mongo: no documents in result"))

	s := NewService(kb)
	err := s.DeleteDoc("kbdoc_1", "", true)
	assert.NoError(t, err, "deleting an already-deleted doc must be a no-op success")
}

func TestDeleteDoc_NoGraphNoVector_StillCascadesMongo(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1"} // no GridFS
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("DeleteChunks", mock.Anything, "kbdoc_1").Return(int64(0), nil)
	kb.On("DeleteFile", mock.Anything, "").Return(nil)
	kb.On("DeleteDoc", mock.Anything, "kbdoc_1").Return(nil)

	err := NewService(kb).DeleteDoc("kbdoc_1", "", true)
	assert.NoError(t, err)
	kb.AssertCalled(t, "DeleteChunks", mock.Anything, "kbdoc_1")
	kb.AssertCalled(t, "DeleteFile", mock.Anything, "")
	kb.AssertCalled(t, "DeleteDoc", mock.Anything, "kbdoc_1")
}

// ---- SPEC-091: SetPublicFlag 联动同步图谱 is_public ----

func TestSetPublicFlag_GraphSync_OrderAndParams(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)
	graph := mockrepo.NewGraphRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1", IsPublic: false}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)

	// 用共享切片记录调用顺序，验证「副作用先行，doc 最后」。
	var order []string
	kb.On("UpdateChunkVisibility", mock.Anything, "kbdoc_1", true).Return(nil).Run(func(mock.Arguments) {
		order = append(order, "chunks")
	})
	vec.On("SetPayload", mock.Anything, "kb_chunks", "kbdoc_1", mock.MatchedBy(func(p map[string]interface{}) bool {
		v, _ := p["is_public"].(bool)
		return v
	})).Return(nil).Run(func(mock.Arguments) {
		order = append(order, "qdrant")
	})
	graph.On("SetDocPublic", mock.Anything, "kbdoc_1", true).Return(nil).Run(func(mock.Arguments) {
		order = append(order, "graph")
	})
	kb.On("SetPublicFlag", mock.Anything, "kbdoc_1", true).Return(nil).Run(func(mock.Arguments) {
		order = append(order, "doc")
	})

	s := NewService(kb)
	s.WithVectorIndex(vec, nil)
	s.WithGraphIndex(graph)

	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.NoError(t, err)

	graph.AssertCalled(t, "SetDocPublic", mock.Anything, "kbdoc_1", true)
	assert.Equal(t, []string{"chunks", "qdrant", "graph", "doc"}, order)
}

func TestSetPublicFlag_GraphNil_NoPanic(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("UpdateChunkVisibility", mock.Anything, "kbdoc_1", true).Return(nil)
	vec.On("SetPayload", mock.Anything, "kb_chunks", "kbdoc_1", mock.Anything).Return(nil)
	kb.On("SetPublicFlag", mock.Anything, "kbdoc_1", true).Return(nil)

	// graph 未注入 → 不 panic、主链路不受影响。
	s := NewService(kb)
	s.WithVectorIndex(vec, nil)

	assert.NoError(t, s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false))
	kb.AssertCalled(t, "SetPublicFlag", mock.Anything, "kbdoc_1", true)
}

func TestSetPublicFlag_GraphError_AbortsBeforeDoc(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)
	graph := mockrepo.NewGraphRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("UpdateChunkVisibility", mock.Anything, "kbdoc_1", true).Return(nil)
	vec.On("SetPayload", mock.Anything, "kb_chunks", "kbdoc_1", mock.Anything).Return(nil)
	graph.On("SetDocPublic", mock.Anything, "kbdoc_1", true).Return(errors.New("arcadedb down"))

	s := NewService(kb)
	s.WithVectorIndex(vec, nil)
	s.WithGraphIndex(graph)

	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "graph is_public")
	// 图同步失败 → 提交点（doc is_public）不更新。
	kb.AssertNotCalled(t, "SetPublicFlag", mock.Anything, "kbdoc_1", true)
}

func TestSetPublicFlag_ChunkVisibilityError_Aborts(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("UpdateChunkVisibility", mock.Anything, "kbdoc_1", true).Return(errors.New("mongo down"))

	s := NewService(kb)
	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chunk visibility")
	kb.AssertNotCalled(t, "SetPublicFlag", mock.Anything, "kbdoc_1", true)
}

func TestSetPublicFlag_ForbiddenPrivateDoc(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "owner", IsPublic: false}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)

	s := NewService(kb)
	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "attacker", false)
	assert.ErrorIs(t, err, ErrNotFound)
	kb.AssertNotCalled(t, "SetPublicFlag", mock.Anything, "kbdoc_1", true)
	kb.AssertNotCalled(t, "UpdateChunkVisibility", mock.Anything, "kbdoc_1", true)
}

func TestSetPublicFlag_GetDocError(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(nil, errors.New("mongo down"))

	s := NewService(kb)
	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get doc")
}

func TestSetPublicFlag_DocNotFound(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(nil, nil)

	s := NewService(kb)
	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSetPublicFlag_QdrantError_AbortsBeforeDoc(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)
	vec := mockrepo.NewVectorRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("UpdateChunkVisibility", mock.Anything, "kbdoc_1", true).Return(nil)
	vec.On("SetPayload", mock.Anything, "kb_chunks", "kbdoc_1", mock.Anything).Return(errors.New("qdrant down"))

	s := NewService(kb)
	s.WithVectorIndex(vec, nil)

	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "qdrant payload")
	kb.AssertNotCalled(t, "SetPublicFlag", mock.Anything, "kbdoc_1", true)
}

func TestSetPublicFlag_DocSetError(t *testing.T) {
	kb := mockrepo.NewKBRepository(t)

	doc := &knowledge.KnowledgeDoc{ID: "kbdoc_1", UserID: "u1"}
	kb.On("GetDoc", mock.Anything, "kbdoc_1").Return(doc, nil)
	kb.On("UpdateChunkVisibility", mock.Anything, "kbdoc_1", true).Return(nil)
	kb.On("SetPublicFlag", mock.Anything, "kbdoc_1", true).Return(errors.New("mongo update failed"))

	s := NewService(kb)
	err := s.SetPublicFlag(context.Background(), "kbdoc_1", true, "u1", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "set public flag on doc")
}
