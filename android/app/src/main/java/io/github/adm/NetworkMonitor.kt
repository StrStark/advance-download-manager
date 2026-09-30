package io.github.adm

import admmobile.Admmobile
import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.os.Build
import android.util.Log
import org.json.JSONArray
import org.json.JSONObject
import java.util.concurrent.ConcurrentHashMap

/**
 * Tells the Go engine which networks the phone has (Wi-Fi, mobile data,
 * USB/Ethernet…). Go can't enumerate Android networks itself, and binds each
 * download connection to a network by its handle.
 *
 * When "Combine connections" is on, it also asks Android to keep mobile data
 * connected while Wi-Fi is up (Android normally turns it off), so downloads
 * can use both at once.
 */
object NetworkMonitor {
    private lateinit var cm: ConnectivityManager
    private val caps = ConcurrentHashMap<Network, NetworkCapabilities>()
    private val props = ConcurrentHashMap<Network, LinkProperties>()
    private var cellularRequest: ConnectivityManager.NetworkCallback? = null
    @Volatile
    private var started = false

    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onCapabilitiesChanged(network: Network, nc: NetworkCapabilities) {
            caps[network] = nc
            push()
        }

        override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) {
            props[network] = lp
            push()
        }

        override fun onLost(network: Network) {
            caps.remove(network)
            props.remove(network)
            push()
        }
    }

    @Synchronized
    fun start(context: Context) {
        if (started) return
        cm = context.applicationContext.getSystemService(ConnectivityManager::class.java)
        val req = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .removeCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        cm.registerNetworkCallback(req, callback)
        started = true
    }

    /** Keep mobile data up next to Wi-Fi while multi-link is enabled. */
    @Synchronized
    fun setMultiLink(enabled: Boolean) {
        if (!started) return
        if (enabled && cellularRequest == null) {
            val cb = object : ConnectivityManager.NetworkCallback() {}
            try {
                cm.requestNetwork(
                    NetworkRequest.Builder()
                        .addTransportType(NetworkCapabilities.TRANSPORT_CELLULAR)
                        .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                        .build(),
                    cb,
                )
                cellularRequest = cb
            } catch (e: Exception) {
                Log.w("ADM", "could not keep mobile data up", e)
            }
        } else if (!enabled) {
            cellularRequest?.let { runCatching { cm.unregisterNetworkCallback(it) } }
            cellularRequest = null
        }
    }

    private fun push() {
        val arr = JSONArray()
        for ((network, nc) in caps) {
            val lp = props[network]
            val kind = when {
                nc.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> "vpn"
                nc.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
                nc.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
                nc.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
                Build.VERSION.SDK_INT >= 31 && nc.hasTransport(NetworkCapabilities.TRANSPORT_USB) -> "usb"
                else -> "other"
            }
            val iface = lp?.interfaceName ?: ""
            val label = when (kind) {
                "wifi" -> "Wi-Fi"
                "cellular" -> "Mobile data"
                "ethernet" -> "Ethernet"
                "usb" -> "USB"
                "vpn" -> "VPN"
                else -> "Network"
            } + if (iface.isNotEmpty()) " ($iface)" else ""
            val addrs = JSONArray()
            lp?.linkAddresses?.forEach { la ->
                val a = la.address
                if (!a.isLoopbackAddress && !a.isLinkLocalAddress) addrs.put(a.hostAddress)
            }
            arr.put(
                JSONObject()
                    .put("id", "net-${network.networkHandle}")
                    .put("name", iface)
                    .put("label", label)
                    .put("kind", kind)
                    .put("handle", network.networkHandle)
                    .put("addrs", addrs),
            )
        }
        runCatching { Admmobile.setLinks(arr.toString()) }
            .onFailure { Log.w("ADM", "could not update links", it) }
    }
}
