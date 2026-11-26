package io.github.renereose.varu;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.print.PrintAttributes;
import android.print.PrintManager;
import android.view.Menu;
import android.view.MenuItem;
import android.view.WindowManager;
import android.webkit.GeolocationPermissions;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import java.io.BufferedReader;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import org.json.JSONException;
import org.json.JSONObject;

/**
 * Varu on Android: the same Go installer and launcher as on a computer,
 * shipped in the APK as lib*.so, shown in a WebView. With nothing installed
 * yet the app opens the installer wizard; afterwards it opens the map.
 */
public class MainActivity extends Activity {
    private static final Pattern URL = Pattern.compile("http://\\S+");
    private static final int MENU_MAP = 1, MENU_DOWNLOADS = 2;

    // Static: a download keeps the installer running while the app has no
    // screen (DownloadService), and a new screen finds it again.
    private static Process process;
    private static volatile String page; // the address the program printed
    private static boolean askedNotifications;

    private WebView web;
    private boolean installerMode;
    private File viewerRoot;
    private File updating; // the drive the installer was opened to update
    private String geoOrigin;
    private GeolocationPermissions.Callback geoCallback;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        // The screen stays on while Varu is open, for following directions.
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);

        web = new WebView(this);
        WebSettings settings = web.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setGeolocationEnabled(true);
        web.addJavascriptInterface(new Bridge(), "VaruAndroid");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri uri = request.getUrl();
                if (isLocal(uri)) return false;
                // Credits and sources open in the browser.
                try {
                    startActivity(new Intent(Intent.ACTION_VIEW, uri));
                } catch (ActivityNotFoundException none) {
                    Toast.makeText(MainActivity.this, isEstonian() ? "Seda linki ei saa avada." : "No app can open this link.",
                            Toast.LENGTH_LONG).show();
                }
                return true;
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onGeolocationPermissionsShowPrompt(String origin, GeolocationPermissions.Callback callback) {
                if (checkSelfPermission(Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED) {
                    callback.invoke(origin, true, false);
                    return;
                }
                geoOrigin = origin;
                geoCallback = callback;
                requestPermissions(new String[]{
                        Manifest.permission.ACCESS_COARSE_LOCATION, Manifest.permission.ACCESS_FINE_LOCATION}, 1);
            }
        });
        setContentView(web);

        File installed = installedDrive();
        if (downloading()) {
            installerMode = true; // back to the download under way
            web.loadUrl(page);
        } else if (installed != null) {
            startViewer(installed);
            if (olderThanApp(installed)) offerUpdate(installed);
        } else {
            startInstaller(null);
        }
    }

    @Override
    public void onRequestPermissionsResult(int code, String[] permissions, int[] results) {
        if (code != 1 || geoCallback == null) return;
        boolean granted = results.length > 0 && results[0] == PackageManager.PERMISSION_GRANTED;
        geoCallback.invoke(geoOrigin, granted, false);
        geoCallback = null;
    }

    @Override
    protected void onResume() {
        super.onResume();
        // The installer quits when its page goes quiet for a while (as it
        // does in the background); bring it back where the user left off.
        if (process != null && hasExited(process)) {
            if (installerMode) startInstaller(updating);
            else if (viewerRoot != null) startViewer(viewerRoot);
        }
    }

    @Override
    protected void onDestroy() {
        if (!downloading()) stop();
        super.onDestroy();
    }

    @Override
    public void onBackPressed() {
        if (web.canGoBack()) web.goBack();
        else super.onBackPressed();
    }

    @Override
    public boolean onCreateOptionsMenu(Menu menu) {
        boolean et = isEstonian();
        menu.add(0, MENU_MAP, 0, et ? "Kaart" : "Map");
        menu.add(0, MENU_DOWNLOADS, 1, et ? "Allalaadimised" : "Downloads");
        return true;
    }

    @Override
    public boolean onPrepareOptionsMenu(Menu menu) {
        menu.findItem(MENU_MAP).setVisible(installedDrive() != null && !downloading());
        return true;
    }

    @Override
    public boolean onOptionsItemSelected(MenuItem item) {
        if (item.getItemId() == MENU_DOWNLOADS) {
            if (downloading()) web.loadUrl(page);
            else startInstaller(installedDrive());
            return true;
        }
        if (item.getItemId() == MENU_MAP) {
            File drive = installedDrive();
            if (drive != null) startViewer(drive);
            return true;
        }
        return super.onOptionsItemSelected(item);
    }

    // ---------- the Go programs ----------

    /** update: a drive whose earlier choices the wizard starts from, or null. */
    private void startInstaller(File update) {
        List<String> args = new ArrayList<>();
        args.add(program("libvaruinstaller.so"));
        args.add("-no-browser");
        args.add("-phone"); // no desktop launchers
        if (update != null) {
            args.add("-update");
            args.add(update.getAbsolutePath());
        }
        updating = update;
        String dns = dnsServers();
        if (!dns.isEmpty()) {
            args.add("-dns");
            args.add(dns);
        }
        boolean et = isEstonian();
        List<File> places = storage();
        for (int i = 0; i < places.size(); i++) {
            String label = i == 0 ? (et ? "See telefon" : "This phone") : (et ? "Mälukaart" : "Memory card");
            args.add("-volume");
            args.add(label + "=" + places.get(i).getAbsolutePath());
        }
        installerMode = true;
        start(args);
    }

    private void startViewer(File root) {
        List<String> args = new ArrayList<>();
        args.add(program("libvarulauncher.so"));
        args.add("-root");
        args.add(root.getAbsolutePath());
        args.add("-no-browser");
        installerMode = false;
        viewerRoot = root;
        start(args);
        invalidateOptionsMenu();
    }

    private void start(List<String> args) {
        stop();
        try {
            final Process started = new ProcessBuilder(args).redirectErrorStream(true).start();
            process = started;
            new Thread(() -> {
                // Open the address the program prints, and keep draining its output.
                try (BufferedReader out = new BufferedReader(new InputStreamReader(started.getInputStream()))) {
                    boolean opened = false;
                    for (String line; (line = out.readLine()) != null; ) {
                        android.util.Log.i("Varu", line); // adb logcat -s Varu
                        Matcher m = URL.matcher(line);
                        if (!opened && m.find()) {
                            opened = true;
                            String url = m.group();
                            page = url;
                            runOnUiThread(() -> web.loadUrl(url));
                        }
                    }
                } catch (IOException ignored) {
                    // the process ended
                }
            }).start();
        } catch (IOException e) {
            web.loadData("<p style='font:16px sans-serif;padding:1em'>Varu could not start: "
                    + android.text.TextUtils.htmlEncode(e.getMessage()) + "</p>", "text/html", "utf-8");
        }
    }

    private void stop() {
        if (process != null) process.destroy();
        process = null;
    }

    private static boolean downloading() {
        return DownloadService.running && process != null && !hasExited(process);
    }

    private String program(String name) {
        return new File(getApplicationInfo().nativeLibraryDir, name).getAbsolutePath();
    }

    // ---------- storage ----------

    /** App folders on the phone and on a memory card, if there is one. No permission needed. */
    private List<File> storage() {
        List<File> out = new ArrayList<>();
        for (File dir : getExternalFilesDirs(null)) {
            if (dir != null) out.add(dir);
        }
        if (out.isEmpty()) out.add(getFilesDir());
        return out;
    }

    private File installedDrive() {
        for (File dir : storage()) {
            if (new File(dir, "manifest.json").isFile() && new File(dir, "app").isDirectory()) return dir;
        }
        return null;
    }

    private boolean isStorage(File dir) {
        for (File known : storage()) {
            if (known.getAbsolutePath().equals(dir.getAbsolutePath())) return true;
        }
        return false;
    }

    // ---------- updates ----------

    // A new APK brings new programs, but the map pages, road data and
    // library on the phone stay as they were downloaded. Offer to update
    // them; the map keeps working meanwhile, which matters offline.
    private void offerUpdate(File drive) {
        boolean et = isEstonian();
        String version = appVersion();
        new AlertDialog.Builder(this)
                .setTitle(et ? "Uuenda kaarte ja andmeid?" : "Update the maps and data?")
                .setMessage(et
                        ? "See on Varu " + version + ", kuid selle telefoni kaardid, raamatukogu ja muud andmed on varasemast versioonist, nii et osa uusi võimalusi puudub. Uuendamiseks on vaja internetti."
                        : "This is Varu " + version + ", but the maps, library and other data on this phone come from an earlier version, so some new features are missing. Updating needs the internet.")
                .setPositiveButton(et ? "Uuenda" : "Update", (dialog, which) -> startInstaller(drive))
                .setNegativeButton(et ? "Hiljem" : "Later", null)
                .show();
    }

    private String appVersion() {
        try {
            return getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
        } catch (PackageManager.NameNotFoundException e) {
            return "";
        }
    }

    /** Whether the drive's data comes from an earlier release than this app (no release: before 0.1.5). */
    private boolean olderThanApp(File drive) {
        int[] app = parseVersion(appVersion());
        if (app == null) return false; // a development build
        int[] data = parseVersion(dataRelease(drive));
        if (data == null) return true;
        for (int i = 0; i < 3; i++) {
            if (data[i] != app[i]) return data[i] < app[i];
        }
        return false;
    }

    private static String dataRelease(File drive) {
        try (FileInputStream in = new FileInputStream(new File(drive, "manifest.json"))) {
            java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            for (int n; (n = in.read(buf)) > 0; ) out.write(buf, 0, n);
            return new JSONObject(out.toString("UTF-8")).optString("release", "");
        } catch (IOException | JSONException e) {
            return "";
        }
    }

    /** "v1.2.3" or "1.2.3" as {1, 2, 3}; null for anything else. */
    private static int[] parseVersion(String v) {
        Matcher m = Pattern.compile("^v?(\\d+)\\.(\\d+)\\.(\\d+)$").matcher(v == null ? "" : v);
        if (!m.matches()) return null;
        return new int[]{Integer.parseInt(m.group(1)), Integer.parseInt(m.group(2)), Integer.parseInt(m.group(3))};
    }

    // ---------- helpers ----------

    /** Go cannot read Android's DNS settings, so hand it the current network's servers. */
    private String dnsServers() {
        ConnectivityManager cm = getSystemService(ConnectivityManager.class);
        LinkProperties link = cm == null ? null : cm.getLinkProperties(cm.getActiveNetwork());
        StringBuilder out = new StringBuilder();
        if (link != null) {
            for (InetAddress address : link.getDnsServers()) {
                if (out.length() > 0) out.append(',');
                out.append(address.getHostAddress());
            }
        }
        return out.toString();
    }

    /** Process.isAlive() needs Android 8; this works from 7. */
    private static boolean hasExited(Process p) {
        try {
            p.exitValue();
            return true;
        } catch (IllegalThreadStateException running) {
            return false;
        }
    }

    private static boolean isLocal(Uri uri) {
        String host = uri.getHost();
        return "http".equals(uri.getScheme()) && ("127.0.0.1".equals(host) || "localhost".equals(host));
    }

    static boolean isEstonian() {
        return "et".equals(Locale.getDefault().getLanguage());
    }

    /** Called by the wizard page (installer/wizard.html) when it runs inside this app. */
    private class Bridge {
        /** The wizard calls this while a job runs, so it carries on with the screen off. */
        @JavascriptInterface
        public void downloading() {
            runOnUiThread(() -> {
                if (!installerMode || DownloadService.running) return;
                if (Build.VERSION.SDK_INT >= 33 && !askedNotifications
                        && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                    askedNotifications = true; // without it the download still runs, just without its notification
                    requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 2);
                }
                DownloadService.start(MainActivity.this, page);
            });
        }

        @JavascriptInterface
        public void openVaru(String target) {
            runOnUiThread(() -> {
                File root = new File(target);
                if (isStorage(root)) startViewer(root); // only folders this app handed out
            });
        }

        /** WebView ignores window.print(), so pages call this to open the system print dialog. */
        @JavascriptInterface
        public void print() {
            runOnUiThread(() -> {
                PrintManager pm = getSystemService(PrintManager.class);
                if (pm == null) return;
                pm.print("Varu", web.createPrintDocumentAdapter("Varu"), new PrintAttributes.Builder().build());
            });
        }

        @JavascriptInterface
        public void close() {
            runOnUiThread(() -> {
                File drive = installedDrive();
                if (drive != null) startViewer(drive);
                else finish();
            });
        }
    }
}
