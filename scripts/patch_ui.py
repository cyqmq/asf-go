#!/usr/bin/env python3
"""对内嵌 ASF-ui 应用 ui_patches.json 中的硬编码链接替换。

用法：python3 scripts/patch_ui.py
运行后：
1. 遍历 internal/ipc/ui/www 下的 .js/.html/.json 文件，替换 ui_patches.json 中的字符串
2. 调用 scripts/update_sri.py 重算所有 SRI 哈希
3. 需要重新构建二进制（go:embed 编译时生效）
"""
import json
import os
import subprocess
import sys

WWW_DIR = "internal/ipc/ui/www"
PATCHES_FILE = "ui_patches.json"
PATCH_SUFFIXES = (".js", ".html", ".json")


def main() -> int:
    with open(PATCHES_FILE, encoding="utf-8") as f:
        patches = json.load(f)
    if "_comment" in patches:
        del patches["_comment"]

    total_replacements = 0
    changed_files = []
    for root, dirs, files in os.walk(WWW_DIR):
        for fname in sorted(files):
            if not fname.endswith(PATCH_SUFFIXES):
                continue
            path = os.path.join(root, fname)
            with open(path, encoding="utf-8", errors="ignore") as f:
                data = f.read()
            original = data
            for old, new in patches.items():
                if old in data:
                    data = data.replace(old, new)
                    total_replacements += 1
            if data != original:
                with open(path, "w", encoding="utf-8") as f:
                    f.write(data)
                changed_files.append(path)

    if changed_files:
        print(f"已补丁 {len(changed_files)} 个文件：")
        for p in changed_files:
            print("  " + p)
        print("重新计算 SRI 哈希...")
        subprocess.run([sys.executable, "scripts/update_sri.py"], check=True)
    else:
        print("没有文件需要补丁（已是最新）")
    print(f"共替换 {total_replacements} 处")
    return 0


if __name__ == "__main__":
    sys.exit(main())