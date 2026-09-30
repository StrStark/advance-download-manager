package io.github.adm

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.drawable.Icon
import android.os.Build
import android.text.format.Formatter
import android.webkit.MimeTypeMap
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import java.io.File
import java.util.concurrent.atomic.AtomicInteger

object Notifications {
    const val ID_PROGRESS = 1
    private const val ID_TIMEOUT = 2
    private const val CH_ACTIVE = "active"
    private const val CH_DONE = "done"
    private val nextId = AtomicInteger(100)

    fun createChannels(ctx: Context) {
        val nm = ctx.getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(
            NotificationChannel(CH_ACTIVE, ctx.getString(R.string.channel_active), NotificationManager.IMPORTANCE_LOW)
                .apply { description = ctx.getString(R.string.channel_active_desc) },
        )
        nm.createNotificationChannel(
            NotificationChannel(CH_DONE, ctx.getString(R.string.channel_done), NotificationManager.IMPORTANCE_DEFAULT)
                .apply { description = ctx.getString(R.string.channel_done_desc) },
        )
    }

    fun notify(ctx: Context, id: Int, n: Notification) {
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(ctx, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) return
        ctx.getSystemService(NotificationManager::class.java).notify(id, n)
    }

    private fun openApp(ctx: Context): PendingIntent = PendingIntent.getActivity(
        ctx, 0,
        Intent(ctx, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
        PendingIntent.FLAG_IMMUTABLE,
    )

    fun progress(ctx: Context, active: Long, bytesPerSec: Long, downloaded: Long, total: Long): Notification {
        val b = Notification.Builder(ctx, CH_ACTIVE)
            .setSmallIcon(R.drawable.ic_notification)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .setCategory(Notification.CATEGORY_PROGRESS)
            .setContentIntent(openApp(ctx))
        if (active <= 0) {
            b.setContentTitle(ctx.getString(R.string.notif_preparing)).setProgress(0, 0, true)
        } else {
            val title = if (active == 1L) "1 download" else "$active downloads"
            val speed = Formatter.formatShortFileSize(ctx, bytesPerSec) + "/s"
            if (total > 0) {
                val pct = (downloaded * 100 / total).toInt().coerceIn(0, 100)
                b.setContentTitle(title).setContentText("$speed · $pct%").setProgress(100, pct, false)
            } else {
                b.setContentTitle(title).setContentText(speed).setProgress(0, 0, true)
            }
        }
        val pause = PendingIntent.getService(
            ctx, 1,
            Intent(ctx, DownloadService::class.java).setAction(DownloadService.ACTION_PAUSE_ALL),
            PendingIntent.FLAG_IMMUTABLE,
        )
        b.addAction(Notification.Action.Builder(null as Icon?, ctx.getString(R.string.notif_pause_all), pause).build())
        return b.build()
    }

    fun showComplete(ctx: Context, filename: String, file: File) {
        val open = openFileIntent(ctx, file)?.let {
            PendingIntent.getActivity(ctx, nextId.get(), it, PendingIntent.FLAG_IMMUTABLE)
        } ?: openApp(ctx)
        val n = Notification.Builder(ctx, CH_DONE)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(ctx.getString(R.string.notif_done_title))
            .setContentText(filename)
            .setAutoCancel(true)
            .setContentIntent(open)
            .build()
        notify(ctx, nextId.getAndIncrement(), n)
    }

    fun showTimeout(ctx: Context) {
        val n = Notification.Builder(ctx, CH_DONE)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(ctx.getString(R.string.notif_timeout_title))
            .setContentText(ctx.getString(R.string.notif_timeout_text))
            .setAutoCancel(true)
            .setContentIntent(openApp(ctx))
            .build()
        notify(ctx, ID_TIMEOUT, n)
    }

    /** ACTION_VIEW for a downloaded file, shared through our FileProvider. */
    fun openFileIntent(ctx: Context, file: File): Intent? = try {
        val uri = FileProvider.getUriForFile(ctx, "${ctx.packageName}.files", file)
        val mime = MimeTypeMap.getSingleton()
            .getMimeTypeFromExtension(file.extension.lowercase()) ?: "*/*"
        Intent(Intent.ACTION_VIEW)
            .setDataAndType(uri, mime)
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    } catch (e: IllegalArgumentException) {
        null // outside the shared paths
    }
}
