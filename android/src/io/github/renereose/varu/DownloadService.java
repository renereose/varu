package io.github.renereose.varu;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.IBinder;
import android.os.PowerManager;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.Locale;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * Keeps an install going with the screen off or the app in the background.
 * Android freezes or kills a background app together with its child
 * processes, the installer among them; a foreground service keeps the app
 * running, and a wake lock and a Wi-Fi lock keep the CPU and Wi-Fi awake.
 * It follows the installer's own progress and stops when the job ends or
 * the installer is gone.
 */
public class DownloadService extends Service {
    static volatile boolean running;

    private static final String CHANNEL = "downloads";
    private static final int ONGOING = 1, DONE = 2;

    private PowerManager.WakeLock wake;
    private WifiManager.WifiLock wifi;
    private Thread watcher;

    /** page: the installer's address as it prints it, http://127.0.0.1:port/#token. */
    static void start(Context context, String page) {
        if (running || page == null) return;
        running = true;
        Intent intent = new Intent(context, DownloadService.class).putExtra("page", page);
        try {
            if (Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent);
            else context.startService(intent);
        } catch (IllegalStateException notAllowed) {
            running = false; // Android 12+ refuses from the background; the page asks again
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationChannel channel = new NotificationChannel(CHANNEL,
                    MainActivity.isEstonian() ? "Allalaadimised" : "Downloads", NotificationManager.IMPORTANCE_LOW);
            getSystemService(NotificationManager.class).createNotificationChannel(channel);
        }
        Notification first = progress(0, 0);
        if (Build.VERSION.SDK_INT >= 29) startForeground(ONGOING, first, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
        else startForeground(ONGOING, first);
        if (watcher != null || intent == null) return START_NOT_STICKY;

        // 6 h ceiling on the wake lock; a bigger library on a slow line would need longer.
        wake = getSystemService(PowerManager.class).newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "varu:download");
        wake.acquire(6 * 60 * 60 * 1000L);
        WifiManager wm = (WifiManager) getApplicationContext().getSystemService(Context.WIFI_SERVICE);
        if (wm != null) {
            wifi = wm.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "varu:download");
            wifi.acquire();
        }
        String page = intent.getStringExtra("page");
        watcher = new Thread(() -> watch(page));
        watcher.start();
        return START_NOT_STICKY;
    }

    @Override
    public void onDestroy() {
        running = false;
        if (watcher != null) watcher.interrupt();
        if (wake != null && wake.isHeld()) wake.release();
        if (wifi != null && wifi.isHeld()) wifi.release();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void watch(String page) {
        int hash = page.indexOf('#');
        String state = page.substring(0, hash) + "api/state";
        String token = page.substring(hash + 1);
        NotificationManager nm = getSystemService(NotificationManager.class);
        for (int failures = 0; failures < 3; ) {
            try {
                JSONObject job = fetch(state, token).optJSONObject("job");
                failures = 0;
                if (job == null) break;
                if (job.optBoolean("finished")) {
                    JSONArray errors = job.optJSONArray("errors");
                    nm.notify(DONE, done(errors != null && errors.length() > 0));
                    break;
                }
                nm.notify(ONGOING, progress(job.optLong("done"), job.optLong("total")));
            } catch (IOException | JSONException e) {
                failures++; // three in a row: the installer is gone
            }
            try {
                Thread.sleep(2000);
            } catch (InterruptedException stopped) {
                break;
            }
        }
        stopSelf();
    }

    private static JSONObject fetch(String address, String token) throws IOException, JSONException {
        HttpURLConnection c = (HttpURLConnection) new URL(address).openConnection();
        c.setConnectTimeout(5000);
        c.setReadTimeout(10000);
        c.setRequestProperty("X-Token", token);
        try (InputStream in = c.getInputStream()) {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            for (int n; (n = in.read(buf)) > 0; ) out.write(buf, 0, n);
            return new JSONObject(out.toString("UTF-8"));
        } finally {
            c.disconnect();
        }
    }

    private Notification progress(long done, long total) {
        boolean et = MainActivity.isEstonian();
        Notification.Builder b = builder()
                .setSmallIcon(android.R.drawable.stat_sys_download)
                .setContentTitle(et ? "Varu laadib alla…" : "Varu is downloading…")
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setProgress(1000, total > 0 ? (int) (done * 1000 / total) : 0, total <= 0);
        if (total > 0) b.setContentText(size(done) + " / " + size(total));
        return b.build();
    }

    private Notification done(boolean problems) {
        boolean et = MainActivity.isEstonian();
        return builder()
                .setSmallIcon(android.R.drawable.stat_sys_download_done)
                .setContentTitle(problems ? (et ? "Valmis, kuid tekkis probleeme" : "Finished, with problems")
                        : (et ? "Varu on valmis" : "Varu is ready"))
                .setContentText(problems ? (et ? "Ava Varu, et näha, mis jäi puudu." : "Open Varu to see what is missing.")
                        : (et ? "Varu töötab nüüd ka ilma internetita." : "Varu now works without the internet."))
                .setAutoCancel(true)
                .build();
    }

    /** Tapping either notification opens the app where it was, like its icon does. */
    private Notification.Builder builder() {
        Notification.Builder b = Build.VERSION.SDK_INT >= 26 ? new Notification.Builder(this, CHANNEL) : new Notification.Builder(this);
        Intent open = getPackageManager().getLaunchIntentForPackage(getPackageName());
        return b.setContentIntent(PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE));
    }

    private static String size(long bytes) {
        if (bytes < 1_000_000_000L) return bytes / 1_000_000 + " MB";
        return String.format(Locale.getDefault(), "%.1f GB", bytes / 1e9);
    }
}
