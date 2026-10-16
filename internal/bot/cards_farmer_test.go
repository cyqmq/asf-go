package bot

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/logging"
)

func TestParseBadgePage(t *testing.T) {
	html := `
<html><body>
<div class="badge_row_inner">
	<div class="badge_title">
		<a>Counter-Strike 2</a>
	</div>
	<div class="badge_title_stats_content">
		<div class="card_drop_info_dialog" id="card_drop_info_dialog_730"></div>
		<span class="progress_info_bold">3 of 5 cards</span>
	</div>
</div>
<div class="badge_row_inner">
	<div class="badge_title">
		<a>Dota 2</a>
	</div>
	<div class="badge_title_stats_content">
		<div class="card_drop_info_dialog" id="card_drop_info_dialog_570"></div>
		<span class="progress_info_bold">No drops remaining</span>
	</div>
</div>
<div class="badge_row_inner">
	<div class="badge_title">
		<a>Half-Life</a>
	</div>
	<div class="badge_title_stats_content">
		<div class="card_drop_info_dialog" id="card_drop_info_dialog_70"></div>
		<span class="progress_info_bold">1 card</span>
	</div>
</div>
</body></html>
`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[uint32]bool)
	games, rowCount := parseBadgePage(doc, seen)
	if rowCount != 3 {
		t.Errorf("行数应为 3，得到 %d", rowCount)
	}
	if len(games) != 2 {
		t.Fatalf("应解析出 2 个有掉落的游戏，得到 %d: %+v", len(games), games)
	}
	if games[0].AppID != 730 {
		t.Errorf("第一个游戏 AppID 应为 730，得到 %d", games[0].AppID)
	}
	if games[0].CardsRemaining != 3 {
		t.Errorf("CS2 剩余卡牌应为 3，得到 %d", games[0].CardsRemaining)
	}
	if games[0].Name != "Counter-Strike 2" {
		t.Errorf("游戏名解析错误: %q", games[0].Name)
	}
	if games[1].AppID != 70 {
		t.Errorf("第二个游戏 AppID 应为 70，得到 %d", games[1].AppID)
	}
}

func TestParseBadgePageDeduplicates(t *testing.T) {
	html := `
<html><body>
<div class="badge_row_inner">
	<div class="card_drop_info_dialog" id="card_drop_info_dialog_730"></div>
	<span class="progress_info_bold">2 cards</span>
</div>
<div class="badge_row_inner">
	<div class="card_drop_info_dialog" id="card_drop_info_dialog_730"></div>
	<span class="progress_info_bold">2 cards</span>
</div>
</body></html>
`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	seen := make(map[uint32]bool)
	games, _ := parseBadgePage(doc, seen)
	if len(games) != 1 {
		t.Errorf("重复 AppID 应去重，得到 %d 个", len(games))
	}
}

func newTestFarmer(t *testing.T, cfg *config.BotConfig) *CardsFarmer {
	t.Helper()
	log, err := logging.New("Bot-test", filepath.Join(t.TempDir(), "test.log"), logging.Info)
	if err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	return &CardsFarmer{
		bot:            &Bot{Config: cfg, Log: log},
		log:            log,
		gameStartTimes: make(map[uint32]time.Time),
	}
}

func TestApplyFarmingOrdersAscending(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.FarmingOrders = []string{"Ascending"}
	cf := newTestFarmer(t, cfg)
	games := []GameInfo{
		{AppID: 730, Name: "CS2", CardsRemaining: 2},
		{AppID: 570, Name: "Dota2", CardsRemaining: 5},
		{AppID: 440, Name: "TF2", CardsRemaining: 1},
	}
	cf.applyFarmingOrders(games)
	if games[0].AppID != 440 || games[1].AppID != 570 || games[2].AppID != 730 {
		t.Errorf("Ascending 排序错误: %v", games)
	}
}

func TestApplyFarmingOrdersDescending(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.FarmingOrders = []string{"Descending"}
	cf := newTestFarmer(t, cfg)
	games := []GameInfo{
		{AppID: 730, Name: "CS2", CardsRemaining: 2},
		{AppID: 570, Name: "Dota2", CardsRemaining: 5},
	}
	cf.applyFarmingOrders(games)
	if games[0].AppID != 730 || games[1].AppID != 570 {
		t.Errorf("Descending 排序错误: %v", games)
	}
}

func TestApplyFarmingOrdersUnorderedKeepsOrder(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.FarmingOrders = []string{"Unordered"}
	cf := newTestFarmer(t, cfg)
	games := []GameInfo{
		{AppID: 730, Name: "CS2", CardsRemaining: 2},
		{AppID: 570, Name: "Dota2", CardsRemaining: 5},
	}
	original := []GameInfo{games[0], games[1]}
	cf.applyFarmingOrders(games)
	if games[0] != original[0] || games[1] != original[1] {
		t.Errorf("Unordered 应保持原顺序: %v", games)
	}
}

func TestApplyHoursLimit(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.HoursUntilCardDrops = 1
	cf := newTestFarmer(t, cfg)
	// 570 两小时前开始（应被过滤），440 刚加入（保留）
	cf.gameStartTimes[570] = time.Now().Add(-2 * time.Hour)
	games := []GameInfo{
		{AppID: 570, Name: "Dota2", CardsRemaining: 5},
		{AppID: 440, Name: "TF2", CardsRemaining: 1},
	}
	games = cf.applyHoursLimit(games)
	if len(games) != 1 || games[0].AppID != 440 {
		t.Errorf("超时游戏应被过滤，实际: %v", games)
	}
	// 再次调用：440 现在有了开始时间，仍然保留
	games = cf.applyHoursLimit(games)
	if len(games) != 1 || games[0].AppID != 440 {
		t.Errorf("新游戏应保留，实际: %v", games)
	}
}

func TestApplyHoursLimitUsesMaxFarmingTime(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.HoursUntilCardDrops = 3
	gc := config.Defaults()
	gc.MaxFarmingTime = 1
	cf := newTestFarmer(t, cfg)
	cf.bot.GlobalConfig = gc
	// 570 两小时前开始：HoursUntilCardDrops=3 未超，但 MaxFarmingTime=1 已超
	cf.gameStartTimes[570] = time.Now().Add(-2 * time.Hour)
	games := []GameInfo{
		{AppID: 570, Name: "Dota2", CardsRemaining: 5},
		{AppID: 440, Name: "TF2", CardsRemaining: 1},
	}
	games = cf.applyHoursLimit(games)
	if len(games) != 1 || games[0].AppID != 440 {
		t.Errorf("MaxFarmingTime=1 应过滤 2 小时前的游戏，实际: %v", games)
	}
}
