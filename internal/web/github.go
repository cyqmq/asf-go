// Package web 移植自 ArchiSteamFarm.Web，提供 GitHub 更新检查等网络功能。
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// GitHubAsset 对应 ASF 的 ReleaseAsset。
type GitHubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// GitHubRelease 对应 ASF 的 ReleaseResponse。
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Prerelease  bool          `json:"prerelease"`
	Body        string        `json:"body"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubClient 是 GitHub API 客户端。
type GitHubClient struct {
	http *http.Client
}

// NewGitHubClient 创建 GitHub API 客户端。
func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// githubAuthHeader 返回 GitHub 请求的 Authorization 头（支持 GITHUB_TOKEN 环境变量以规避限流）。
func githubAuthHeader() string {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

// GetLatestRelease 获取指定仓库的最新 Release。
func (g *GitHubClient) GetLatestRelease(owner, repo string, stable bool) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "asf-go")
	if auth := githubAuthHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回状态码 %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}
	if stable && release.Prerelease {
		return nil, fmt.Errorf("最新 Release 为预发布版")
	}
	return &release, nil
}

// GetRelease 获取指定版本标签的 Release。
func (g *GitHubClient) GetRelease(owner, repo, tag string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "asf-go")
	if auth := githubAuthHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回状态码 %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}
	return &release, nil
}

// CompareVersions 比较两个版本号（vX.Y.Z 或 X.Y.Z）。返回 1 表示 a>b，-1 表示 a<b，0 相等。
func CompareVersions(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := 0; i < maxLen; i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x > y {
			return 1
		}
		if x < y {
			return -1
		}
	}
	return 0
}

// DownloadAsset 将 Release 附件下载到目标文件路径。
func (g *GitHubClient) DownloadAsset(asset *GitHubAsset, destPath string) error {
	if asset == nil || asset.URL == "" {
		return fmt.Errorf("无效的 Release 附件")
	}
	req, err := http.NewRequest(http.MethodGet, asset.URL, nil)
	if err != nil {
		return fmt.Errorf("构造下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "asf-go")
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回状态码 %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}
