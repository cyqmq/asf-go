// Package bot 的 CardsFarmer：移植自 ArchiSteamFarm.Steam.Cards.CardsFarmer。
// 通过解析 steamcommunity.com/my/badges 页面，找出仍有卡牌掉落的游戏并设置游玩状态。
package bot

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/asf-go/asf/internal/logging"
)

// containsUint32 判断切片中是否包含目标值。
func containsUint32(list []uint32, target uint32) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// GameInfo 表示一个待挂机游戏及其剩余卡牌数。
type GameInfo struct {
	AppID          uint32
	Name           string
	CardsRemaining int
}

// CardsFarmer 负责卡牌掉落挂机。
type CardsFarmer struct {
	bot *Bot
	log *logging.Logger

	mu          sync.Mutex
	running     bool
	stopCh      chan struct{}
	gamesToFarm []GameInfo
}

// NewCardsFarmer 创建挂机器。
func NewCardsFarmer(b *Bot) *CardsFarmer {
	return &CardsFarmer{
		bot:    b,
		log:    b.Log,
		stopCh: make(chan struct{}),
	}
}

// IsRunning 返回挂机状态。
func (cf *CardsFarmer) IsRunning() bool {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	return cf.running
}

// GamesToFarm 返回当前待挂机游戏列表。
func (cf *CardsFarmer) GamesToFarm() []GameInfo {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	out := make([]GameInfo, len(cf.gamesToFarm))
	copy(out, cf.gamesToFarm)
	return out
}

// Start 启动挂机循环。
func (cf *CardsFarmer) Start() {
	cf.mu.Lock()
	if cf.running {
		cf.mu.Unlock()
		return
	}
	cf.running = true
	cf.stopCh = make(chan struct{})
	cf.mu.Unlock()

	cf.log.Info("CardsFarmer 启动")
	go cf.loop()
}

// Stop 停止挂机循环。
func (cf *CardsFarmer) Stop() {
	cf.mu.Lock()
	if !cf.running {
		cf.mu.Unlock()
		return
	}
	cf.running = false
	close(cf.stopCh)
	cf.mu.Unlock()
	cf.log.Info("CardsFarmer 已停止")
}

// loop 周期性执行：解析卡牌页面 -> 设置游玩 -> 等待。
func (cf *CardsFarmer) loop() {
	for {
		cf.mu.Lock()
		running := cf.running
		stopCh := cf.stopCh
		cf.mu.Unlock()
		if !running {
			return
		}

		cf.farmOnce()

		select {
		case <-stopCh:
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

// farmOnce 执行一轮挂机：解析页面、设置游玩游戏。
func (cf *CardsFarmer) farmOnce() {
	if cf.bot.steamClient == nil {
		cf.log.Warn("Steam 客户端未就绪，跳过挂机")
		return
	}

	games, err := cf.fetchGamesWithDrops()
	if err != nil {
		cf.log.Error("获取卡牌掉落列表失败: %v", err)
		return
	}

	// 应用挂机黑名单过滤
	if len(cf.bot.Config.FarmingBlacklist) > 0 {
		filtered := make([]GameInfo, 0, len(games))
		for _, g := range games {
			if !containsUint32(cf.bot.Config.FarmingBlacklist, g.AppID) {
				filtered = append(filtered, g)
			}
		}
		games = filtered
	}

	// 挂机队列优先排序（队列中的游戏排前面）
	if len(cf.bot.Config.FarmingQueue) > 0 {
		queueRank := func(appID uint32) int {
			for i, id := range cf.bot.Config.FarmingQueue {
				if id == appID {
					return i
				}
			}
			return len(cf.bot.Config.FarmingQueue)
		}
		sort.SliceStable(games, func(i, j int) bool {
			return queueRank(games[i].AppID) < queueRank(games[j].AppID)
		})
	}

	cf.mu.Lock()
	cf.gamesToFarm = games
	cf.mu.Unlock()

	if len(games) == 0 {
		cf.log.Info("没有发现仍有卡牌掉落的游戏")
		cf.bot.steamClient.StopPlaying()
		return
	}

	appIDs := make([]uint64, 0, len(games))
	for _, g := range games {
		appIDs = append(appIDs, uint64(g.AppID))
		cf.log.Info("挂机游戏 %s (AppID=%d, 剩余卡牌=%d)", g.Name, g.AppID, g.CardsRemaining)
	}
	cf.bot.steamClient.StartPlaying(appIDs)
}

// fetchGamesWithDrops 抓取卡牌页面并解析仍有掉落的游戏。
func (cf *CardsFarmer) fetchGamesWithDrops() ([]GameInfo, error) {
	ws, err := cf.bot.steamClient.WebSession()
	if err != nil {
		return nil, fmt.Errorf("创建 Web 会话失败: %w", err)
	}

	var games []GameInfo
	seen := make(map[uint32]bool)

	// 可选：通过 Web API 获取"徽章未完成"的游戏，作为参考信息
	if cf.bot.SteamAPIKey != "" {
		if candidates, err := cf.bot.BadgeCandidatesFromAPI(); err != nil {
			cf.log.Debug("Web API 徽章查询失败（忽略）: %v", err)
		} else {
			cf.log.Info("Web API 报告 %d 个徽章未完成的游戏（用于交叉参考）", len(candidates))
		}
	}

	// 社区卡牌页面最多约 15 页，逐页解析直到空页。
	for page := uint8(1); page <= 15; page++ {
		doc, err := ws.GetBadgePage(page)
		if err != nil {
			cf.log.Debug("第 %d 页解析失败: %v", page, err)
			break
		}
		pageGames, count := parseBadgePage(doc, seen)
		games = append(games, pageGames...)
		if count == 0 {
			break
		}
	}
	return games, nil
}

// parseBadgePage 解析单页卡牌数据，返回新游戏列表与行数。
func parseBadgePage(doc *goquery.Document, seen map[uint32]bool) ([]GameInfo, int) {
	var games []GameInfo
	rows := doc.Find("div.badge_row_inner")
	rowCount := rows.Length()

	rows.Each(func(_ int, row *goquery.Selection) {
		// 定位包含 appid 的节点：id 形如 card_drop_info_dialog_440
		dialog := row.Find("div[class='card_drop_info_dialog'][id]").First()
		idAttr, exists := dialog.Attr("id")
		if !exists {
			return
		}
		parts := strings.Split(idAttr, "_")
		if len(parts) < 5 {
			return
		}
		appID, err := strconv.ParseUint(parts[4], 10, 32)
		if err != nil || appID == 0 {
			return
		}
		appID32 := uint32(appID)
		if seen[appID32] {
			return
		}

		// 剩余卡牌数：span.progress_info_bold 中的数字
		progress := row.Find("span[class='progress_info_bold']").First().Text()
		cardsRemaining := 0
		for _, ch := range progress {
			if ch >= '0' && ch <= '9' {
				digits := extractDigits(progress)
				cardsRemaining, _ = strconv.Atoi(digits)
				break
			}
		}
		if cardsRemaining == 0 {
			seen[appID32] = true
			return
		}

		// 游戏名：badge_title 或 badge_name
		name := row.Find("div[class='badge_title']").First().Text()
		name = strings.TrimSpace(name)
		if name == "" {
			name = row.Find("div.badge_name").First().Text()
			name = strings.TrimSpace(name)
		}
		if name == "" {
			name = fmt.Sprintf("AppID_%d", appID32)
		}

		seen[appID32] = true
		games = append(games, GameInfo{AppID: appID32, Name: name, CardsRemaining: cardsRemaining})
	})

	return games, rowCount
}

// extractDigits 提取字符串中的第一段连续数字。
func extractDigits(s string) string {
	var b strings.Builder
	inDigits := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			inDigits = true
		} else if inDigits {
			break
		}
	}
	return b.String()
}
