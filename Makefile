.PHONY: build test race ui-patch ui-upgrade

build:
	go build -o asf ./cmd/asf

test:
	go test ./...

race:
	go test -race ./...

# 对内嵌 ASF-ui 应用链接补丁并重算 SRI（升级 UI 或修改硬编码后运行）
ui-patch:
	python3 scripts/patch_ui.py

# 下载指定版本的原版 ASF release，替换 www 目录，打补丁并重算 SRI。
# 用法：make ui-upgrade VERSION=6.3.10.6
ui-upgrade:
	@test -n "$(VERSION)" || (echo "请指定 VERSION，如 make ui-upgrade VERSION=6.3.10.6"; exit 1)
	rm -rf /tmp/asf-ui-download /tmp/asf-ui-www
	mkdir -p /tmp/asf-ui-download /tmp/asf-ui-www
	curl -fSL "https://github.com/JustArchiNET/ArchiSteamFarm/releases/download/$(VERSION)/ASF-linux-x64.zip" -o /tmp/asf-ui-download/asf.zip
	unzip -o /tmp/asf-ui-download/asf.zip "www/*" -d /tmp/asf-ui-download
	cp -r /tmp/asf-ui-download/www/* internal/ipc/ui/www/
	python3 scripts/patch_ui.py