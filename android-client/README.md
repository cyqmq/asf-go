# ASF-Go Android 客户端

基于 [Capacitor](https://capacitorjs.com/) 将 Web 原型封装为 Android APK，通过 asf-go IPC API 控制挂机。

## 目录结构

```
android-client/
├── www/                        # Web 资源（原型页面）
│   ├── index.html
│   ├── style.css
│   └── app.js                   # 已接入真实 IPC API
├── android/                     # Capacitor 生成的 Android 工程
├── capacitor.config.json        # appId / appName / webDir / androidScheme=http
├── package.json                 # @capacitor/core + android + cli
└── download.html                # 原型下载页（可选）
```

## 本地构建 APK

前置：Node.js 18+、Java 17、Android SDK（`ANDROID_HOME`）。

```bash
cd android-client
npm install
npx cap sync android
cd android && ./gradlew assembleDebug
# 产物：android/app/build/outputs/apk/debug/app-debug.apk
```

## 内置服务端 + IPC（已接入）

- **内置 asf-go 服务端**：CI 构建时将 asf-go 交叉编译为 Android arm64 静态 PIE 可执行文件（`CGO_ENABLED=0 GOOS=android GOARCH=arm64 -buildmode=pie`），作为 `jniLibs/arm64-v8a/libasfgo.so` 打包（`useLegacyPackaging true` 解压到可执行的 `nativeLibraryDir`，`keepDebugSymbols` 防止 strip）。
  - Capacitor 原生插件 `AsfGoPlugin`：从 `nativeLibraryDir` 执行 `libasfgo.so`（兜底：老版本从 assets 解压到 filesDir + setExecutable）；启动前先运行 `-version` 自测；设置环境变量 `HOME=filesDir`、`TMPDIR=cacheDir/tmp`、`SSL_CERT_DIR=/system/etc/security/cacerts`；以 `--ipc-addr 127.0.0.1:1242` 启动子进程。asf-go 新增 `-version` 参数供自测。
  - UI「启动 ASF 服务」按钮：调用插件启动内置服务 → 轮询 `GET /api/asf` 直到 IPC 就绪。
- **远程服务器模式**：无原生插件（浏览器预览）或配置了远程地址时，连接外部 asf-go。
- IPC 调用：
  - `GET /api/asf`：连接验证、刷新机器人数量与挂机状态。
  - `POST /api/command`：命令控制台真实执行命令。
  - `Authentication: <IPC 密码>` 头（与 ASF-ui 相同认证方式）。
- 连接失败自动进入**演示模式**。
- 设置页可保存服务器地址/端口/IPC 密码/公共模式（localStorage；内置服务默认 127.0.0.1:1242/testpass）。公共模式开启后 asf-go 以 `--ipc-addr 0.0.0.0:1242` 启动（允许外部访问），地址栏禁用。
- 机器人管理页「添加机器人」弹窗：调用 `POST /api/bot/{name}` 创建新 Bot 配置。
- Android 已允许 cleartext HTTP（`android:usesCleartextTraffic="true"`），`androidScheme` 设为 `http`。

## CI 构建

`../.github/workflows/build-apk.yml` 在推送 `android-client/**` 变更时自动构建 debug APK 并上传 artifact（Actions 页面下载）。
