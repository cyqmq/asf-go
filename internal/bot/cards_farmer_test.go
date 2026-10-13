package bot

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
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