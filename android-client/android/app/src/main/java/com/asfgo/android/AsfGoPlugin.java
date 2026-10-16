package com.asfgo.android;

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
import java.io.InputStreamReader;
import java.io.OutputStreamWriter;
import java.io.Writer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

@CapacitorPlugin(name = "AsfGo")
public class AsfGoPlugin extends Plugin {
    private Process process;

    private File binaryFile() {
        return new File(getContext().getApplicationInfo().nativeLibraryDir, "libasfgo.so");
    }

    private File configDir() {
        return new File(getContext().getFilesDir(), "config");
    }

    private File logFile() {
        return new File(getContext().getFilesDir(), "asf-go.log");
    }

    private void ensureBinary() throws IOException {
        File bin = binaryFile();
        if (!bin.exists() || bin.length() == 0) {
            throw new IOException("内置 asf-go 二进制不存在: " + bin.getAbsolutePath());
        }
        if (!bin.canExecute()) {
            throw new IOException("内置 asf-go 二进制不可执行: " + bin.getAbsolutePath());
        }
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

    @PluginMethod
    public void start(PluginCall call) {
        try {
            String password = call.getString("password", "testpass");
            ensureBinary();
            ensureConfig(password);
            if (process != null && process.isAlive()) {
                call.resolve(new JSObject().put("running", true));
                return;
            }
            ProcessBuilder pb = new ProcessBuilder(
                    binaryFile().getAbsolutePath(),
                    "--config", configDir().getAbsolutePath(),
                    "--ipc-addr", "127.0.0.1:1242"
            );
            pb.redirectErrorStream(true);
            pb.redirectOutput(logFile());
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
