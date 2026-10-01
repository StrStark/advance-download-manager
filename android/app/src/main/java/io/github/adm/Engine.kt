package io.github.adm

import admmobile.Admmobile
import admmobile.Listener
import android.content.Context
import android.content.Intent
import android.media.MediaScannerConnection
import android.net.Uri
import android.provider.Settings
import android.widget.Toast
import androidx.core.content.FileProvider
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
        NetworkMonitor.start(app)
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

    /** Opens Android's installer for a downloaded update. */
    override fun onInstallUpdate(apkPath: String) {
        main.post {
            val pm = app.packageManager
            if (!pm.canRequestPackageInstalls()) {
                // Android asks once per app: "Allow installing from ADM".
                Toast.makeText(app, R.string.allow_installs, Toast.LENGTH_LONG).show()
                app.startActivity(
                    Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${app.packageName}"))
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                )
                return@post
            }
            val uri = FileProvider.getUriForFile(app, "${app.packageName}.files", File(apkPath))
            app.startActivity(
                Intent(Intent.ACTION_VIEW)
                    .setDataAndType(uri, "application/vnd.android.package-archive")
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK),
            )
        }
    }

    override fun onMultiLinkChanged(enabled: Boolean) {
        main.post { NetworkMonitor.setMultiLink(enabled) }
    }

    override fun onDownloadComplete(filename: String, path: String, inBatch: Boolean) {
        // Make the file visible to galleries, music players and the Files app.
        MediaScannerConnection.scanFile(app, arrayOf(path), null, null)
        if (!inBatch) Notifications.showComplete(app, filename, File(path))
    }
}
