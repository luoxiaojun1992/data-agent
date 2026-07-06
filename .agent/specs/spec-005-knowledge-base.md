# Phase 3 — 知识库系统

> **SPEC-005** | Status: 设计中 | 依赖: SPEC-004

## 目标

实现知识库文档全生命周期：上传解析 → LLM 语义分片 → GridFS 存储 → Milvus 向量索引 → 混合搜索 → 智能解读 → 聚合层管理。

## 背景

Roadmap Phase 3 (Week 5-6)，P3-01 ~ P3-13，总计 ~55h。知识库是 Agent 回答质量的核心依赖。

## 详细设计

### 1. 文档解析引擎
- 支持 PDF（ledongthuc/pdf）、Word、Excel（xuri/excelize）、Markdown、TXT
- 统一输出纯文本

### 2. 文档存储
- 所有知识库文档统一存入 GridFS（MongoDB）
- 小文件和大文件同一路径，GridFS 自动 255KB 分片
- `knowledge_doc_contents`: `gridfs_file_id` + `filename` + `size_bytes`

### 3. LLM 语义分片
- 复用当前 LLM 判断语义段落边界
- 每 chunk 目标 500 字符
- Index Worker 通过 `GridFSDownloadStream`（io.ReadSeeker）分批读取

### 4. Milvus 向量索引
- Collection: `knowledge_chunks`（1536d OpenAI / 768d 其他）
- `doc_id` 强绑定，删除文档时级联清理
- 异步索引任务：复用 Agent Task Queue（Redis Stream）

### 5. MongoDB 全文索引
- `knowledge_docs.title` + `knowledge_doc_contents.content` 建立 text index

### 6. 混合搜索
- Milvus 语义相似度（top-k）
- MongoDB 全文搜索（text score）
- 融合排序（RRF: Reciprocal Rank Fusion）
- 权限过滤：按用户角色裁剪结果

### 7. 智能解读
- KB 检索结果 + 上下文注入 → LLM 生成解读
- 分析结果落库

### 8. 聚合层
- 多维度聚合数据定义：dimensions + metrics
- MongoDB `aggregation_layers` 集合管理

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（knowledge_docs, knowledge_doc_contents, kb_index_tasks, kb_chunks, aggregation_layers） |
| 是否影响现有 API | No |
| 性能影响 | Milvus 向量搜索延迟 < 200ms（top-100） |
| 是否需要新增 Skill | Yes（knowledge_search, save_analysis_report） |
| 是否需要 E2E 测试 | 知识库上传→搜索→结果展示 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `skills/knowledge_search/` | 混合搜索 Skill | 新建 |
| `internal/service/knowledge/` | 知识库服务 | 新建 |
| `github.com/pkoukk/tiktoken-go` | Token 计数（Go 原生, MIT） | 新增依赖 |

## 验证标准

1. 上传 PDF → 自动解析 → GridFS 存储 → 异步索引 → Milvus 可搜索
2. 删除文档 → kb_chunks 级联清理
3. 混合搜索：输入自然语言 → 返回 top-5 相关 chunk
4. 智能解读：分析结果 + KB 上下文 → LLM 生成解读
5. doc_id 隔离：搜索文档 A 不会返回文档 B 的内容
