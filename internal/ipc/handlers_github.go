package ipc

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/web"
)

func (s *Server) handleGitHubRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
	prefix := "/api/www/github/release"
	if strings.HasPrefix(r.URL.Path, "/api/releases") {
		prefix = "/api/releases"
	}
	version := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	client := web.NewGitHubClient()

	var release *web.GitHubRelease
	var err error
	if version == "" || version == "latest" {
		// 使用我们自己的 asf-go 仓库（而非上游 ASF），让 ASF-ui 的版本/更新信息与本项目一致
		release, err = web.GetLatestReleaseCached("cyqmq", "asf-go", false)
		if err != nil {
			// GitHub API 限流/不可用时返回合成 release（版本号置空，前端走历史分支仍可加载 wiki）
			release = &web.GitHubRelease{
				TagName:     "",
				PublishedAt: time.Now().UTC().Format(time.RFC3339),
				Body:        "",
				Prerelease:  false,
			}
			err = nil
		}
	} else {
		release, err = client.GetRelease("cyqmq", "asf-go", version)
		if err != nil {
			// ASF-ui 用 /release/{version} 判断是否需要按历史版本加载 wiki；版本不存在（如 asf-go 自己的版本号）
			// 时返回合成 release，避免前端 fetchWiki 卡住（原版 ASF 的版本号在仓库中真实存在）。
			release = &web.GitHubRelease{
				TagName:     version,
				PublishedAt: time.Now().UTC().Format(time.RFC3339),
				Body:        "",
				Prerelease:  false,
			}
			err = nil
		}
	}
	if err != nil || release == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"Success": false, "Message": fmt.Sprintf("获取 GitHub Release 失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Success": true,
		"Result": map[string]any{
			"ChangelogHTML": release.Body,
			"ReleasedAt":    release.PublishedAt,
			"Stable":        !release.Prerelease,
			"Version":       release.TagName,
		},
	})
}

// handleGitHubWiki 提供 ASF-ui 配置字段描述所需的 GitHub wiki 页面/历史
// （对应原版 GitHubController 的 Wiki/Page 与 Wiki/History 端点）。

func (s *Server) handleGitHubWiki(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}

	client := web.NewGitHubClient()
	lower := strings.ToLower(r.URL.Path)

	if strings.HasPrefix(lower, "/api/www/github/wiki/history/") {
		page := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/www/github/wiki/history/"), "/")
		if page == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "缺少页面名"})
			return
		}
		history, err := client.GetWikiHistory(page)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"Success": false, "Message": fmt.Sprintf("获取 wiki 历史失败: %v", err)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": history})
		return
	}

	if strings.HasPrefix(lower, "/api/www/github/wiki/page/") {
		page := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/www/github/wiki/page/"), "/")
		if page == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "缺少页面名"})
			return
		}
		revision := r.URL.Query().Get("revision")
		html, err := client.GetWikiPage(page, revision)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"Success": false, "Message": fmt.Sprintf("获取 wiki 页面失败: %v", err)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": html})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知 wiki 端点"})
}

// handleStorage 提供持久化 KeyValue 存储（对应原版 StorageController），ASF-ui 用它保存界面设置。
