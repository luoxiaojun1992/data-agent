package apicollection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

type Service struct {
	repo *mongo.APICollectionRepo
}

func NewService(repo *mongo.APICollectionRepo) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnsureIndexes(ctx context.Context) error {
	return s.repo.EnsureIndexes(ctx)
}

func (s *Service) CreateUpload(ctx context.Context, userID, name, description string, rawSpec []byte, fileID string) (*model.APICollection, error) {
	var rawDoc map[string]interface{}
	if err := json.Unmarshal(rawSpec, &rawDoc); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI JSON: %w", err)
	}
	if _, ok := rawDoc["openapi"]; !ok {
		return nil, fmt.Errorf("missing openapi version")
	}

	apiCount := 0
	if paths, ok := rawDoc["paths"].(map[string]interface{}); ok {
		apiCount = len(paths)
	}

	coll := &model.APICollection{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Status:      model.APICollectionPending,
		OpenAPISpec: json.RawMessage(rawSpec),
		FileID:      fileID,
		UserID:      userID,
		APICount:    apiCount,
	}
	if err := s.repo.Create(ctx, coll); err != nil {
		return nil, err
	}
	return coll, nil
}

func (s *Service) Get(ctx context.Context, id, ownerID string) (*model.APICollection, error) {
	coll, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if ownerID != "" && coll.UserID != ownerID {
		return nil, fmt.Errorf("not found")
	}
	return coll, nil
}

func (s *Service) List(ctx context.Context, ownerID string, page, pageSize int) (*mongo.ListResult, error) {
	return s.repo.List(ctx, mongo.ListParams{UserID: ownerID, Page: page, PageSize: pageSize})
}

func (s *Service) Update(ctx context.Context, id, userID, name, desc string) error {
	return s.repo.UpdateFields(ctx, id, userID, name, desc)
}

func (s *Service) Delete(ctx context.Context, id, userID string) error {
	return s.repo.Delete(ctx, id, userID)
}

func (s *Service) Approve(ctx context.Context, id string, status model.APICollectionStatus) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) SearchApproved(ctx context.Context, query string, limit int) ([]*model.APICollection, error) {
	return s.repo.SearchByDescription(ctx, query, limit)
}

type PathEntry struct {
	Path    string `json:"path"`
	Method  string `json:"method"`
	Summary string `json:"summary"`
}

type APISummaryResult struct {
	Paths    []PathEntry `json:"paths"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Name     string      `json:"name"`
}

func (s *Service) GetAPISummary(ctx context.Context, collectionID string, page, pageSize int) (*APISummaryResult, error) {
	if page <= 0 {
		page = 1
	}
	if page > 100 {
		page = 100
	}
	if pageSize <= 0 || pageSize > 10 {
		pageSize = 10
	}

	coll, err := s.repo.GetByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if coll.Status != model.APICollectionApproved {
		return nil, fmt.Errorf("collection not approved")
	}

	var allPaths []PathEntry
	specMap := parseOpenAPISpec(coll.OpenAPISpec)
	if paths, ok := specMap["paths"].(map[string]interface{}); ok {
		for path, methodsRaw := range paths {
			methods, _ := methodsRaw.(map[string]interface{})
			if methods == nil {
				continue
			}
			for method := range methods {
				if method == "parameters" || method == "servers" || method == "summary" || method == "description" {
					continue
				}
				summary := ""
				if op, ok := methods[method].(map[string]interface{}); ok {
					if s, ok := op["summary"].(string); ok {
						summary = s
					}
				}
				allPaths = append(allPaths, PathEntry{Path: path, Method: method, Summary: summary})
			}
		}
	}

	total := len(allPaths)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return &APISummaryResult{
		Paths:    allPaths[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Name:     coll.Name,
	}, nil
}

type APIMethodDetail struct {
	Path        string      `json:"path"`
	Method      string      `json:"method"`
	Summary     string      `json:"summary"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters,omitempty"`
	RequestBody interface{} `json:"request_body,omitempty"`
	Responses   interface{} `json:"responses,omitempty"`
}

func (s *Service) GetAPIMethod(ctx context.Context, collectionID, path, method string) (*APIMethodDetail, error) {
	coll, err := s.repo.GetByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if coll.Status != model.APICollectionApproved {
		return nil, fmt.Errorf("collection not approved")
	}

	specMap := parseOpenAPISpec(coll.OpenAPISpec)
	pathsRaw, _ := specMap["paths"].(map[string]interface{})
	if pathsRaw == nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}
	methodsRaw, _ := pathsRaw[path].(map[string]interface{})
	if methodsRaw == nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}
	op, _ := methodsRaw[strings.ToLower(method)].(map[string]interface{})
	if op == nil {
		return nil, fmt.Errorf("method not found: %s %s", method, path)
	}

	summary, _ := op["summary"].(string)
	desc, _ := op["description"].(string)
	detail := &APIMethodDetail{
		Path:        path,
		Method:      method,
		Summary:     summary,
		Description: desc,
		Parameters:  op["parameters"],
		RequestBody: op["requestBody"],
		Responses:   op["responses"],
	}
	return detail, nil
}

type APICallResult struct {
	Status  int                 `json:"status"`
	Body    interface{}         `json:"body"`
	Headers map[string][]string `json:"headers"`
}

func (s *Service) CallAPI(ctx context.Context, collectionID, path, method string, params map[string]string, body interface{}, headers map[string]string) (*APICallResult, error) {
	coll, err := s.repo.GetByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if coll.Status != model.APICollectionApproved {
		return nil, fmt.Errorf("collection not approved")
	}

	specMap := parseOpenAPISpec(coll.OpenAPISpec)

	baseURL := ""
	if servers, ok := specMap["servers"].([]interface{}); ok && len(servers) > 0 {
		if srv, ok := servers[0].(map[string]interface{}); ok {
			if url, ok := srv["url"].(string); ok {
				baseURL = strings.TrimRight(url, "/")
			}
		}
	}

	fullURL := baseURL + path
	for k, v := range params {
		fullURL = strings.Replace(fullURL, "{"+k+"}", v, 1)
	}

	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), fullURL, reqBody)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var respJSON interface{}
	json.Unmarshal(respBody, &respJSON)

	return &APICallResult{
		Status:  resp.StatusCode,
		Body:    respJSON,
		Headers: resp.Header,
	}, nil
}

// parseOpenAPISpec unmarshals a json.RawMessage spec into a map for traversal.
// Returns an empty map on parse failure.
func parseOpenAPISpec(spec json.RawMessage) map[string]interface{} {
	if len(spec) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil
	}
	return m
}
