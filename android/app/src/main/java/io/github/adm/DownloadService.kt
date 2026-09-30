package io.github.adm

import admmobile.Admmobile
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.PowerManager
import android.os.SystemClock
import android.util.Log
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat

/**
 * Keeps the process alive while downloads run, with a progress notification.
 * Android kills background apps otherwise. It stops a few seconds after the
 * last download finishes (the delay bridges the gap between queued jobs).
 */
class DownloadService : Service() {
    companion object {
        const val ACTION_PAUSE_ALL = "io.github.adm.action.PAUSE_ALL"
        private const val STOP_DELAY_MS = 4000L

        @Volatile
        var instance: DownloadService? = null
            private set

        fun ensureRunning(context: Context) {
            instance?.let {
                it.cancelStop()
                return
            }
            try {
                ContextCompat.startForegroundService(context, Intent(context, DownloadService::class.java))
            } catch (e: Exception) {
                // Android 12+ refuses to start foreground services from the
                // background. Downloads continue while the process lives.
                Log.w("ADM", "could not start download service", e)
            }
        }
    }

    private val handler = Handler(Looper.getMainLooper())
    private var wakeLock: PowerManager.WakeLock? = null
    private var wifiLock: WifiManager.WifiLock? = null
    private var lastUpdate = 0L
    private val stopIfIdle = Runnable {
        if (Admmobile.activeCount() == 0L) {
            ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        instance = this
        val type = if (Build.VERSION.SDK_INT >= 29) ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC else 0
        ServiceCompat.startForeground(this, Notifications.ID_PROGRESS, Notifications.progress(this, 0, 0, 0, 0), type)

        wakeLock = (getSystemService(POWER_SERVICE) as PowerManager)
            .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "adm:downloads")
            .apply {
                setReferenceCounted(false)
                acquire(6 * 60 * 60 * 1000L)
            }
        @Suppress("DEPRECATION")
        wifiLock = (applicationContext.getSystemService(WIFI_SERVICE) as WifiManager)
            .createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "adm:downloads")
            .apply {
                setReferenceCounted(false)
                acquire()
            }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_PAUSE_ALL) Admmobile.pauseAll()
        if (Admmobile.activeCount() == 0L) stopWhenIdle()
        return START_NOT_STICKY
    }

    fun showProgress(active: Long, bytesPerSec: Long, downloaded: Long, total: Long) {
        val now = SystemClock.elapsedRealtime()
        if (now - lastUpdate < 1000) return // Android throttles frequent updates anyway
        lastUpdate = now
        Notifications.notify(this, Notifications.ID_PROGRESS, Notifications.progress(this, active, bytesPerSec, downloaded, total))
    }

    fun stopWhenIdle() {
        handler.removeCallbacks(stopIfIdle)
        handler.postDelayed(stopIfIdle, STOP_DELAY_MS)
    }

    fun cancelStop() = handler.removeCallbacks(stopIfIdle)

    /** Android 15 caps dataSync services at 6h per day; pause cleanly. */
    override fun onTimeout(startId: Int, fgsType: Int) {
        Admmobile.pauseAll()
        Notifications.showTimeout(this)
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        wakeLock?.takeIf { it.isHeld }?.release()
        wifiLock?.takeIf { it.isHeld }?.release()
        instance = null
        super.onDestroy()
    }
}
