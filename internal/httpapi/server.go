package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/pathvar"
	"github.com/zxxf18/kids-poetry-be/internal/audiostore"
	"github.com/zxxf18/kids-poetry-be/internal/sso"
	"github.com/zxxf18/kids-poetry-be/internal/store"
)

type API struct {
	store          *store.MySQL
	audio          *audiostore.Store
	datasetVersion string
	auth           *sso.Service
	facetsMu       sync.RWMutex
	facetsCache    map[string][]store.FacetValue
}

func New(s *store.MySQL, audio *audiostore.Store, datasetVersion string, auth *sso.Service) *API {
	a := &API{store: s, audio: audio, datasetVersion: datasetVersion, auth: auth}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := a.loadFacets(ctx); err != nil {
			logx.Errorf("prewarm poetry facets: %v", err)
		}
	}()
	return a
}

func (a *API) Register(server *rest.Server) {
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/auth/login", Handler: a.auth.Login},
		{Method: http.MethodGet, Path: "/auth/callback", Handler: a.auth.Callback},
		{Method: http.MethodGet, Path: "/api/v1/auth/me", Handler: a.auth.Me},
		{Method: http.MethodPost, Path: "/api/v1/auth/logout", Handler: a.auth.Logout},
	})
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/healthz", Handler: a.health},
		{Method: http.MethodGet, Path: "/api/v1/meta", Handler: a.meta},
		{Method: http.MethodGet, Path: "/api/v1/facets", Handler: a.facets},
		{Method: http.MethodGet, Path: "/api/v1/poems", Handler: a.listPoemsWithAuthPolicy},
		{Method: http.MethodGet, Path: "/api/v1/poems/:id", Handler: a.auth.Require(a.getPoem)},
		{Method: http.MethodGet, Path: "/api/v1/poems/:id/audio", Handler: a.auth.Require(a.getPoemAudio)},
		{Method: http.MethodGet, Path: "/api/v1/featured", Handler: a.featured},
	})
}

func (a *API) listPoemsWithAuthPolicy(w http.ResponseWriter, r *http.Request) {
	for _, key := range []string{"q", "dynasty", "author", "title", "kind", "form", "theme", "cipai", "collection"} {
		if strings.TrimSpace(r.URL.Query().Get(key)) != "" {
			a.auth.Require(a.listPoems)(w, r)
			return
		}
	}
	a.listPoems(w, r)
}

func (a *API) getPoemAudio(w http.ResponseWriter, r *http.Request) {
	if a.audio == nil {
		writeError(w, http.StatusServiceUnavailable, "audio_unavailable", "朗读服务暂时不可用")
		return
	}
	id := strings.TrimSpace(pathvar.Vars(r)["id"])
	meta, err := a.store.Audio(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "audio_not_found", "这首作品暂时还没有朗读")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	object, info, err := a.audio.Open(r.Context(), meta.ObjectKey)
	if err != nil {
		logx.Errorf("open poetry audio %s: %v", id, err)
		writeError(w, http.StatusServiceUnavailable, "audio_unavailable", "朗读暂时无法播放，请稍后再试")
		return
	}
	defer object.Close()
	contentType := meta.MimeType
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, id+".mp3", info.LastModified, object)
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
	result, err := a.loadFacets(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) loadFacets(ctx context.Context) (map[string][]store.FacetValue, error) {
	a.facetsMu.Lock()
	defer a.facetsMu.Unlock()
	if a.facetsCache != nil {
		return a.facetsCache, nil
	}
	result, err := a.store.Facets(ctx)
	if err != nil {
		return nil, err
	}
	a.facetsCache = result
	return result, nil
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
	random := r.URL.Query().Get("random") == "true"
	limit := parseInt(r.URL.Query().Get("limit"), 12, 1, 24)
	candidateLimit := limit
	if random {
		candidateLimit = 300
	}
	items, total, err := a.store.List(r.Context(), store.Query{Collection: collection, Page: 1, PageSize: candidateLimit})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if random && len(items) > 0 {
		items = randomSample(items, limit)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "collection": collection})
}

func randomSample[T any](items []T, limit int) []T {
	for index := len(items) - 1; index > 0; index-- {
		target := rand.IntN(index + 1)
		items[index], items[target] = items[target], items[index]
	}
	if limit >= 0 && len(items) > limit {
		return items[:limit]
	}
	return items
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
