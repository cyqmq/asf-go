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

## 对接 asf-go IPC

- 应用内「设置」页可配置服务器地址、端口、IPC 密码。
- `app.js` 通过 `GET /api/asf` 验证连接并刷新机器人列表；通过 `POST /api/command` 发送命令。
- 请求携带 `Authentication: <IPC 密码>` 头（与 ASF-ui 相同认证方式）。
- Android 允许 cleartext HTTP（`android:usesCleartextTraffic="true"`），`androidScheme` 设为 `http` 以访问局域网内 asf-go 服务。
- asf-go 默认 CORS 允许所有来源；如需限制请在 `GlobalConfig.IPCOrigins` 配置。

## CI 构建

`../.github/workflows/build-apk.yml` 在推送 `android-client/**` 变更时自动构建 debug APK 并上传 artifact（Actions 页面下载）。
