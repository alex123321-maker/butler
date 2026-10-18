package com.example.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.os.Build
import android.os.PowerManager
import com.example.security.TransportPolicyValidator
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.net.URI

enum class ActiveNetworkTransport(val labelRu: String) {
    WIFI("Wi-Fi"),
    WIFI_AND_VPN("Wi-Fi + VPN/Tailscale"),
    CELLULAR("Мобильная сеть (LTE/5G)"),
    CELLULAR_AND_VPN("Мобильная сеть + VPN/Tailscale"),
    ETHERNET_OR_OTHER("Проводная/иная сеть"),
    OFFLINE("Нет подключения к сети")
}

data class SystemEnvironmentSnapshot(
    val transport: ActiveNetworkTransport = ActiveNetworkTransport.WIFI,
    val isValidatedInternet: Boolean = true,
    val isVpnOrTailscaleActive: Boolean = false,
    val isDozeIdleMode: Boolean = false,
    val isBatteryOptimizationIgnored: Boolean = false,
    val isSamsungDevice: Boolean = Build.MANUFACTURER.equals("samsung", ignoreCase = true),
    val deviceModelLabel: String = "${Build.MANUFACTURER} ${Build.MODEL} (Android ${Build.VERSION.RELEASE}, SDK ${Build.VERSION.SDK_INT})"
) {
    /**
     * Detects when the phone is on mobile internet (LTE/5G) without VPN/Tailscale while configured
     * to send to a private home Wi-Fi address (`192.168.x.x` / `10.x.x.x`).
     */
    fun isPrivateLanUnreachableOnCurrentNetwork(serverUrl: String): Boolean {
        if (transport != ActiveNetworkTransport.CELLULAR) return false
        val host = runCatching {
            val normalized = if ("://" in serverUrl) serverUrl else "https://$serverUrl"
            URI(normalized).host?.lowercase() ?: ""
        }.getOrDefault("")
        return TransportPolicyValidator.isPrivateLanHost(host) && !TransportPolicyValidator.isTailscaleHost(host)
    }
}

class NetworkConnectivityMonitor(
    private val context: Context,
    private val onNetworkRestored: () -> Unit = {}
) {
    private val connectivityManager =
        context.applicationContext.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
    private val powerManager =
        context.applicationContext.getSystemService(Context.POWER_SERVICE) as? PowerManager

    private val _environmentState = MutableStateFlow(readCurrentSnapshot())
    val environmentState: StateFlow<SystemEnvironmentSnapshot> = _environmentState.asStateFlow()

    private val networkCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) {
            refreshSnapshot()
            onNetworkRestored()
        }

        override fun onCapabilitiesChanged(network: Network, networkCapabilities: NetworkCapabilities) {
            val prevTransport = _environmentState.value.transport
            refreshSnapshot()
            val newTransport = _environmentState.value.transport
            if (prevTransport != newTransport && newTransport != ActiveNetworkTransport.OFFLINE) {
                onNetworkRestored()
            }
        }

        override fun onLost(network: Network) {
            refreshSnapshot()
        }
    }

    init {
        try {
            connectivityManager?.registerDefaultNetworkCallback(networkCallback)
        } catch (_: Exception) {
            // Safe fallback on restricted test environments
        }
    }

    fun refreshSnapshot(): SystemEnvironmentSnapshot {
        val snap = readCurrentSnapshot()
        _environmentState.value = snap
        return snap
    }

    private fun readCurrentSnapshot(): SystemEnvironmentSnapshot {
        val activeNetwork = runCatching { connectivityManager?.activeNetwork }.getOrNull()
        val caps = runCatching {
            if (activeNetwork != null) connectivityManager?.getNetworkCapabilities(activeNetwork) else null
        }.getOrNull()

        val hasWifi = caps?.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) == true
        val hasCellular = caps?.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) == true
        val hasVpn = caps?.hasTransport(NetworkCapabilities.TRANSPORT_VPN) == true
        val hasInternet = caps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) == true

        val transport = when {
            caps == null || !hasInternet -> ActiveNetworkTransport.OFFLINE
            hasWifi && hasVpn -> ActiveNetworkTransport.WIFI_AND_VPN
            hasWifi -> ActiveNetworkTransport.WIFI
            hasCellular && hasVpn -> ActiveNetworkTransport.CELLULAR_AND_VPN
            hasCellular -> ActiveNetworkTransport.CELLULAR
            else -> ActiveNetworkTransport.ETHERNET_OR_OTHER
        }

        val isDoze = runCatching { powerManager?.isDeviceIdleMode == true }.getOrDefault(false)
        val ignoringBatteryOpt = runCatching {
            powerManager?.isIgnoringBatteryOptimizations(context.packageName) == true
        }.getOrDefault(false)

        return SystemEnvironmentSnapshot(
            transport = transport,
            isValidatedInternet = hasInternet,
            isVpnOrTailscaleActive = hasVpn,
            isDozeIdleMode = isDoze,
            isBatteryOptimizationIgnored = ignoringBatteryOpt,
            isSamsungDevice = Build.MANUFACTURER.equals("samsung", ignoreCase = true),
            deviceModelLabel = "${Build.MANUFACTURER} ${Build.MODEL} (Android ${Build.VERSION.RELEASE}, SDK ${Build.VERSION.SDK_INT})"
        )
    }
}
