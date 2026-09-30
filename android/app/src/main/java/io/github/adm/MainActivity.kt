package io.github.adm

import admmobile.Admmobile
import android.Manifest
import android.annotation.SuppressLint
import android.app.DownloadManager
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.Gravity
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.webkit.JavascriptInterface
import android.webkit.RenderProcessGoneDetail
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import android.widget.TextView
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import java.io.File

/**
 * A full-screen WebView showing the same Svelte UI as the desktop app,
 * served by the embedded Go engine on a private loopback port.
 */
class MainActivity : ComponentActivity() {
    private lateinit var web: WebView
    private var insetTop = 0
    private var insetBottom = 0
    private var fileCallback: ValueCallback<Array<Uri>>? = null

    private val pickFile = registerForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        fileCallback?.onReceiveValue(uri?.let { arrayOf(it) })
        fileCallback = null
    }
    private val askPermissions = registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {}

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)

        val url = try {
            Engine.start(this)
        } catch (e: Exception) {
            showFatal(e)
            return
        }

        web = WebView(this)
        val root = FrameLayout(this).apply { addView(web, FrameLayout.LayoutParams(MATCH_PARENT, MATCH_PARENT)) }
        setContentView(root)
        configureWebView()

        // Draw edge-to-edge: the UI pads itself using --inset-top/--inset-bottom.
        // The keyboard is handled natively by shrinking the WebView.
        ViewCompat.setOnApplyWindowInsetsListener(root) { v, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime()).bottom
            v.setPadding(bars.left, 0, bars.right, ime)
            insetTop = bars.top
            insetBottom = if (ime > 0) 0 else bars.bottom
            pushInsets()
            WindowInsetsCompat.CONSUMED
        }

        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                // Let the UI close dialogs/drawers first; otherwise keep running in the background.
                web.evaluateJavascript("window.__admBack ? window.__admBack() : false") { handled ->
                    if (handled != "true") moveTaskToBack(true)
                }
            }
        })

        requestRuntimePermissions()
        web.loadUrl(url)
        handleShare(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleShare(intent)
    }

    override fun onDestroy() {
        if (::web.isInitialized) web.destroy()
        super.onDestroy()
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun configureWebView() {
        web.setBackgroundColor(Color.TRANSPARENT)
        with(web.settings) {
            javaScriptEnabled = true
            domStorageEnabled = true
            allowFileAccess = false
            allowContentAccess = false
            setSupportZoom(false)
        }
        web.addJavascriptInterface(Bridge(), "ADMAndroid")

        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                if (request.url.host == "127.0.0.1") return false
                // Anything else (e.g. a link in the UI) opens in the browser.
                try {
                    startActivity(Intent(Intent.ACTION_VIEW, request.url))
                } catch (_: ActivityNotFoundException) {
                }
                return true
            }

            override fun onPageFinished(view: WebView, url: String) = pushInsets()

            override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                recreate() // the engine keeps running; just rebuild the UI
                return true
            }
        }

        web.webChromeClient = object : WebChromeClient() {
            // "Import file" in the batch dialog uses <input type=file>.
            override fun onShowFileChooser(
                view: WebView,
                callback: ValueCallback<Array<Uri>>,
                params: FileChooserParams,
            ): Boolean {
                fileCallback?.onReceiveValue(null)
                fileCallback = callback
                return try {
                    pickFile.launch("*/*")
                    true
                } catch (_: ActivityNotFoundException) {
                    fileCallback = null
                    false
                }
            }
        }
    }

    private fun pushInsets() {
        if (!::web.isInitialized) return
        val d = resources.displayMetrics.density
        web.evaluateJavascript(
            "(() => { const s = document.documentElement.style;" +
                "s.setProperty('--inset-top', '${insetTop / d}px');" +
                "s.setProperty('--inset-bottom', '${insetBottom / d}px') })()",
            null,
        )
    }

    /** "Share → ADM": hand the shared text to the engine, which extracts links. */
    private fun handleShare(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        intent.getStringExtra(Intent.EXTRA_TEXT)?.let { Admmobile.openURLs(it) }
        intent.action = null // don't re-handle on recreation
    }

    private fun requestRuntimePermissions() {
        val wanted = buildList {
            if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
            if (Build.VERSION.SDK_INT <= 28) add(Manifest.permission.WRITE_EXTERNAL_STORAGE)
        }.filter { ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED }
        if (wanted.isNotEmpty()) askPermissions.launch(wanted.toTypedArray())
    }

    private fun showFatal(e: Exception) {
        setContentView(TextView(this).apply {
            text = getString(R.string.engine_failed, e.message ?: e.toString())
            gravity = Gravity.CENTER
            setPadding(48, 48, 48, 48)
        })
    }

    /** Native helpers the UI calls through window.ADMAndroid. */
    inner class Bridge {
        @JavascriptInterface
        fun openFile(id: String) {
            val path = Admmobile.completedPath(id)
            if (path.isEmpty()) return
            val intent = Notifications.openFileIntent(this@MainActivity, File(path)) ?: return
            runOnUiThread {
                try {
                    startActivity(Intent.createChooser(intent, null))
                } catch (_: ActivityNotFoundException) {
                    Toast.makeText(this@MainActivity, R.string.no_app_to_open, Toast.LENGTH_SHORT).show()
                }
            }
        }

        @JavascriptInterface
        fun showDownloads() {
            runOnUiThread {
                try {
                    startActivity(Intent(DownloadManager.ACTION_VIEW_DOWNLOADS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                } catch (_: ActivityNotFoundException) {
                }
            }
        }
    }
}
