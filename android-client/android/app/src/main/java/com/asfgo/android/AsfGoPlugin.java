package com.asfgo.android;

import android.content.res.AssetManager;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

import java.io.BufferedReader;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.io.OutputStreamWriter;
import java.io.Writer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 内置 asf-go 服务端。
 *
 * 二进制为 Go 静态链接的 PIE 可执行文件，打包在 jniLibs/arm64-v8a/libasfgo.so。
 * Android 系统会将其解压到 nativeLibraryDir（可执行分区），从那里直接运行；
 * 老版本 Android 若 nativeLibraryDir 不可用，则回退到从 assets 解压到 filesDir。
 */
@CapacitorPlugin(name = "AsfGo")
public class AsfGoPlugin extends Plugin {
    private static final String BINARY_NAME = "libasfgo.so";
    private static final String ASSET_FALLBACK = "asf-go";

    private Process process;

    private File binaryFile() {
        return new File(getContext().getApplicationInfo().nativeLibraryDir, BINARY_NAME);
    }

    private File fallbackBinaryFile() {
        return new File(getContext().getFilesDir(), "asf-go");
    }

    private File configDir() {
        return new File(getContext().getFilesDir(), "config");
    }

    private File logFile() {
        return new File(getContext().getFilesDir(), "asf-go.log");
    }

    /**
     * 优先使用 nativeLibraryDir 中的可执行文件；若不可用（老版本 Android），
     * 从 assets 解压到 filesDir 并设置可执行权限（老版本 filesDir 可执行）。
     */
    private File ensureBinary() throws IOException {
        File bin = binaryFile();
        if (bin.exists() && bin.length() > 0 && bin.canExecute()) {
            return bin;
        }
        // 兜底：从 assets 解压
        File fallback = fallbackBinaryFile();
        AssetManager am = getContext().getAssets();
        try (InputStream in = am.open(ASSET_FALLBACK); OutputStream out = new FileOutputStream(fallback)) {
            byte[] buf = new byte[8192];
            int n;
            while ((n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
            }
        }
        if (!fallback.setExecutable(true)) {
            throw new IOException("无法设置 asf-go 可执行权限: " + fallback.getAbsolutePath());
        }
        return fallback;
    }

    private void ensureConfig(String password) throws IOException {
        File dir = configDir();
        if (!dir.exists() && !dir.mkdirs()) {
            throw new IOException("无法创建配置目录");
        }
        File cfg = new File(dir, "ASF.json");
        if (cfg.exists()) {
            return;
        }
        String json = "{\n" +
                "  \"IPC\": true,\n" +
                "  \"IPCPassword\": \"" + escape(password) + "\",\n" +
                "  \"IPCPasswordFormat\": \"PlainText\",\n" +
                "  \"AutoRestart\": true,\n" +
                "  \"SteamWebApiKey\": \"\",\n" +
                "  \"SteamOwnerID\": 0\n" +
                "}\n";
        try (Writer w = new OutputStreamWriter(new FileOutputStream(cfg), StandardCharsets.UTF_8)) {
            w.write(json);
        }
    }

    private static String escape(String s) {
        if (s == null) {
            return "";
        }
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    /**
     * 启动前自测：运行 "<binary> -version"，确保可执行且能正常运行。
     */
    private void selfTest(File bin) throws IOException {
        Process p = new ProcessBuilder(bin.getAbsolutePath(), "-version")
                .redirectErrorStream(true)
                .start();
        try {
            int code = p.waitFor();
            if (code != 0) {
                throw new IOException("asf-go 自测失败 (exit=" + code + ")，请检查二进制");
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new IOException("asf-go 自测被中断", e);
        }
    }

    /**
     * 构建子进程环境变量：HOME/TMPDIR 指向可写目录，PATH/LANG/SSL_CERT_DIR 指向系统路径。
     */
    private Map<String, String> buildEnvironment() {
        File filesDir = getContext().getFilesDir();
        File tmpDir = new File(getContext().getCacheDir(), "tmp");
        if (!tmpDir.exists()) {
            tmpDir.mkdirs();
        }
        Map<String, String> env = new HashMap<>();
        env.put("HOME", filesDir.getAbsolutePath());
        env.put("TMPDIR", tmpDir.getAbsolutePath());
        env.put("PATH", "/system/bin:/vendor/bin");
        env.put("LANG", "C.UTF-8");
        // Android 系统 CA 证书目录（Go crypto/x509 会读取）
        env.put("SSL_CERT_DIR", "/system/etc/security/cacerts");
        return env;
    }

    @PluginMethod
    public void start(PluginCall call) {
        try {
            String password = call.getString("password", "testpass");
            boolean isPublic = call.getBoolean("public", false);
            File bin = ensureBinary();
            ensureConfig(password);
            if (process != null && process.isAlive()) {
                call.resolve(new JSObject().put("running", true));
                return;
            }
            selfTest(bin);
            String ipcAddr = isPublic ? "0.0.0.0:1242" : "127.0.0.1:1242";
            ProcessBuilder pb = new ProcessBuilder(
                    bin.getAbsolutePath(),
                    "--config", configDir().getAbsolutePath(),
                    "--ipc-addr", ipcAddr
            );
            pb.redirectErrorStream(true);
            pb.redirectOutput(logFile());
            pb.environment().putAll(buildEnvironment());
            process = pb.start();
            call.resolve(new JSObject().put("running", true));
        } catch (Exception e) {
            call.reject("启动 asf-go 失败: " + e.getMessage());
        }
    }

    @PluginMethod
    public void stop(PluginCall call) {
        if (process != null) {
            process.destroy();
            process = null;
        }
        call.resolve();
    }

    @PluginMethod
    public void isRunning(PluginCall call) {
        call.resolve(new JSObject().put("running", process != null && process.isAlive()));
    }

    @PluginMethod
    public void getLog(PluginCall call) {
        try {
            String text = "";
            if (logFile().exists()) {
                text = readTail(logFile(), 200);
            }
            call.resolve(new JSObject().put("log", text));
        } catch (IOException e) {
            call.reject("读取日志失败: " + e.getMessage());
        }
    }

    private static String readTail(File f, int maxLines) throws IOException {
        StringBuilder sb = new StringBuilder();
        List<String> lines = new ArrayList<>();
        try (BufferedReader r = new BufferedReader(new InputStreamReader(new FileInputStream(f), StandardCharsets.UTF_8))) {
            String line;
            while ((line = r.readLine()) != null) {
                lines.add(line);
                if (lines.size() > maxLines) {
                    lines.remove(0);
                }
            }
        }
        for (String l : lines) {
            sb.append(l).append('\n');
        }
        return sb.toString();
    }
}
