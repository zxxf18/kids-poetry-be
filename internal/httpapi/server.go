package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/pathvar"
	"github.com/zxxf18/kids-poetry-be/internal/store"
)

type API struct {
	store          *store.MySQL
	datasetVersion string
}

func New(s *store.MySQL, datasetVersion string) *API {
	return &API{store: s, datasetVersion: datasetVersion}
}

func (a *API) Register(server *rest.Server) {
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/healthz", Handler: a.health},
		{Method: http.MethodGet, Path: "/api/v1/meta", Handler: a.meta},
		{Method: http.MethodGet, Path: "/api/v1/facets", Handler: a.facets},
		{Method: http.MethodGet, Path: "/api/v1/poems", Handler: a.listPoems},
		{Method: http.MethodGet, Path: "/api/v1/poems/:id", Handler: a.getPoem},
		{Method: http.MethodGet, Path: "/api/v1/featured", Handler: a.featured},
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库暂时不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "datasetVersion": a.datasetVersion})
}
func (a *API) meta(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.Count(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count, "datasetVersion": a.datasetVersion})
}
func (a *API) facets(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.Facets(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) listPoems(w http.ResponseWriter, r *http.Request) {
	q := queryFromRequest(r)
	items, total, err := a.store.List(r.Context(), q)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": q.Page, "pageSize": q.PageSize, "hasMore": q.Page*q.PageSize < total})
}

func (a *API) featured(w http.ResponseWriter, r *http.Request) {
	collection := strings.TrimSpace(r.URL.Query().Get("collection"))
	if collection == "" {
		collection = "widely-known"
	}
	limit := parseInt(r.URL.Query().Get("limit"), 12, 1, 50)
	items, total, err := a.store.List(r.Context(), store.Query{Collection: collection, Page: 1, PageSize: limit})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "collection": collection})
}

func (a *API) getPoem(w http.ResponseWriter, r *http.Request) {
	id := pathvar.Vars(r)["id"]
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_id", "诗词编号不能为空")
		return
	}
	p, err := a.store.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "没有找到这首诗词")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func queryFromRequest(r *http.Request) store.Query {
	v := r.URL.Query()
	return store.Query{Q: strings.TrimSpace(v.Get("q")), Dynasty: strings.TrimSpace(v.Get("dynasty")), Author: strings.TrimSpace(v.Get("author")), Title: strings.TrimSpace(v.Get("title")), Kind: strings.TrimSpace(v.Get("kind")), Form: strings.TrimSpace(v.Get("form")), Theme: strings.TrimSpace(v.Get("theme")), Cipai: strings.TrimSpace(v.Get("cipai")), Collection: strings.TrimSpace(v.Get("collection")), HasTranslation: v.Get("hasTranslation") == "true", Page: parseInt(v.Get("page"), 1, 1, 100000), PageSize: parseInt(v.Get("pageSize"), 24, 1, 60)}
}
func parseInt(raw string, fallback, min, max int) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
func writeStoreError(w http.ResponseWriter, err error) {
	logx.Errorf("poetry API storage error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时开小差了，请稍后再试")
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
