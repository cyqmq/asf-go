#!/usr/bin/env python3
"""重新计算内嵌 ASF-ui 的 SRI 哈希。

修改 www 下任何 JS 内容后必须运行本脚本：
1. 对 main bundle 的 o.sriHashes 中所有 chunk 重算 sha256/sha384（基于当前文件内容）
2. 重算 main bundle 自身的哈希并更新 index.html 的 integrity 属性

用法：python3 scripts/update_sri.py
"""
import base64
import hashlib
import os
import re
import sys

SCRIPTS_DIR = "internal/ipc/ui/www/scripts"
INDEX_HTML = "internal/ipc/ui/www/index.html"
MAIN_BUNDLE = os.path.join(SCRIPTS_DIR, "main.141d65d.bundle.js")


def sri(data: bytes) -> str:
    sha256 = base64.b64encode(hashlib.sha256(data).digest()).decode()
    sha384 = base64.b64encode(hashlib.sha384(data).digest()).decode()
    return f"sha256-{sha256} sha384-{sha384}"


def main() -> int:
    data = open(MAIN_BUNDLE, encoding="utf-8", errors="ignore").read()

    m = re.search(r"o\.sriHashes=", data)
    if not m:
        print("错误：main bundle 中找不到 o.sriHashes", file=sys.stderr)
        return 1
    start = m.end()
    depth = 0
    end = start
    for i in range(start, len(data)):
        c = data[i]
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                end = i + 1
                break
    obj_text = data[start:end]
    entries = dict(re.findall(r'(\d+):"(sha256-[^"]*)"', obj_text))
    if not entries:
        print("错误：sriHashes 为空", file=sys.stderr)
        return 1

    updated = 0
    for chunk_id in entries:
        # 文件名形如 <id>.<hash>.chunk.js
        matches = [f for f in os.listdir(SCRIPTS_DIR) if f.startswith(chunk_id + ".") and f.endswith(".chunk.js")]
        if not matches:
            continue
        path = os.path.join(SCRIPTS_DIR, matches[0])
        content = open(path, "rb").read()
        new_hash = sri(content)
        if entries[chunk_id] != new_hash:
            obj_text = re.sub(rf'{re.escape(chunk_id)}:"[^"]*"', f'{chunk_id}:"{new_hash}"', obj_text, count=1)
            updated += 1

    data = data[:start] + obj_text + data[end:]
    open(MAIN_BUNDLE, "w", encoding="utf-8").write(data)

    # 更新 index.html 的 main bundle integrity
    main_sri = sri(open(MAIN_BUNDLE, "rb").read())
    idx = open(INDEX_HTML, encoding="utf-8", errors="ignore").read()
    pattern = re.compile(r'<script defer="defer" src="/scripts/main[^"]*"[^>]*></script>')
    new_tag = f'<script defer="defer" src="/scripts/main.141d65d.bundle.js" integrity="{main_sri}" crossorigin="anonymous"></script>'
    idx_new = pattern.sub(new_tag, idx)
    if idx_new == idx:
        print("警告：index.html 中未找到 main script 标签", file=sys.stderr)
        return 1
    open(INDEX_HTML, "w", encoding="utf-8").write(idx_new)

    print(f"SRI 已更新：{updated} 个 chunk + main bundle")
    return 0


if __name__ == "__main__":
    sys.exit(main())