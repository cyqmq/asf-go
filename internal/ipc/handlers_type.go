package ipc

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) handleType(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
	typeName := extractSubPath(r, "/api/type/")
	if typeName == "" {
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{}})
		return
	}
	resp, ok := getTypeInfo(typeName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效类型: " + typeName})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": resp})
}

// handleStructure 返回 ASF-ui 配置编辑器所需的默认配置结构（对应原版 StructureController /api/structure/{type}）。

func (s *Server) handleStructure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
	typeName := extractSubPath(r, "/api/structure/")
	if typeName == "" {
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{}})
		return
	}
	obj, ok := getStructure(typeName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效类型: " + typeName})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": obj})
}

// extractSubPath 从原始请求路径提取 /api/xxx/ 之后的子路径，并做 URL 解码。

func extractSubPath(r *http.Request, prefix string) string {
	origPath, _ := r.Context().Value(originalPathKey{}).(string)
	if origPath == "" {
		origPath = r.URL.Path
	}
	lower := strings.ToLower(origPath)
	idx := strings.Index(lower, prefix)
	var sub string
	if idx >= 0 {
		sub = origPath[idx+len(prefix):]
	} else {
		sub = strings.TrimPrefix(origPath, strings.TrimSuffix(prefix, "/"))
	}
	sub = strings.Trim(sub, "/")
	if decoded, err := url.PathUnescape(sub); err == nil {
		sub = decoded
	}
	return sub
}

// handleAPI404 为未实现的 API 端点返回 JSON 404（而非静态文件服务器的文本 404）。
