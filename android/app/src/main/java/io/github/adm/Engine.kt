package io.github.adm

import admmobile.Admmobile
import admmobile.Listener
import android.content.Context
import android.media.MediaScannerConnection
import android.os.Environment
import android.os.Handler
import android.os.Looper
import java.io.File

/**
 * Owns the Go engine (compiled from ../mobile with gomobile). It lives as
 * long as the process; the UI and the foreground service both attach to it.
 */
object Engine : Listener {
    @Volatile
    private var url: String? = null
    private lateinit var app: Context
    private val main = Handler(Looper.getMainLooper())

    /** Public Download/ADM, so files show up in the Files app and elsewhere. */
    fun downloadDir(): File =
        File(Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS), "ADM")

    /** Starts the engine (once) and returns the URL the WebView should load. */
    @Synchronized
    fun start(context: Context): String {
        url?.let { return it }
        app = context.applicationContext
        val dataDir = File(app.filesDir, "engine")
        val u = Admmobile.start(dataDir.absolutePath, downloadDir().absolutePath)
        Admmobile.setListener(this)
        url = u
        return u
    }

    // ---- callbacks from Go (arbitrary threads) ----

    override fun onActiveChanged(active: Long) {
        main.post {
            if (active > 0) DownloadService.ensureRunning(app) else DownloadService.instance?.stopWhenIdle()
        }
    }

    override fun onProgress(active: Long, bytesPerSec: Long, downloaded: Long, total: Long) {
        main.post { DownloadService.instance?.showProgress(active, bytesPerSec, downloaded, total) }
    }

    override fun onDownloadComplete(filename: String, path: String, inBatch: Boolean) {
        // Make the file visible to galleries, music players and the Files app.
        MediaScannerConnection.scanFile(app, arrayOf(path), null, null)
        if (!inBatch) Notifications.showComplete(app, filename, File(path))
    }
}
