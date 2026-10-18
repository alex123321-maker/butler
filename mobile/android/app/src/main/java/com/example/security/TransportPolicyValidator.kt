package com.example.security

import com.example.model.RouteClassification
import com.example.BuildConfig
import java.net.URI

data class TransportValidationResult(
    val isValid: Boolean,
    val normalizedBaseUrl: String,
    val classification: RouteClassification,
    val warningOrErrorRu: String? = null
)

/**
 * Validates the configured Butler server URL according to Issue #57 transport security rules:
 * - Public/external networks require HTTPS (`https://`).
 * - Private home Wi-Fi LAN (`192.168.x.x`, `10.x.x.x`, `172.16-31.x.x`, `localhost`) and
 *   Tailscale mesh (`100.64.0.0/10`, `*.ts.net`) allow `http://` ONLY when `allowCleartextLocalHttp`
 *   is explicitly enabled by the owner.
 * - `loopback://butler.local` uses the built-in v1 Contract Loopback Inspector for local verification.
 */
object TransportPolicyValidator {

    const val LOOPBACK_BASE_URL = "loopback://butler.local"

    fun validate(rawUrl: String, allowCleartextLocalHttp: Boolean): TransportValidationResult {
        val trimmed = rawUrl.trim().trimEnd('/')
        if (trimmed.isEmpty()) {
            return TransportValidationResult(
                isValid = false,
                normalizedBaseUrl = "",
                classification = RouteClassification.INSECURE_BLOCKED,
                warningOrErrorRu = "Укажите адрес сервера Butler (HTTPS, Tailscale 100.x или локальный IP)"
            )
        }

        if (trimmed.startsWith("loopback://", ignoreCase = true)) {
            return TransportValidationResult(
                isValid = BuildConfig.DEBUG,
                normalizedBaseUrl = LOOPBACK_BASE_URL,
                classification = RouteClassification.LOOPBACK_SANDBOX,
                warningOrErrorRu = "Активен встроенный инспектор контракта v1 (автономная проверка без внешнего сервера Go)"
            )
        }

        val uri = try {
            URI(if ("://" in trimmed) trimmed else "https://$trimmed")
        } catch (_: Exception) {
            return TransportValidationResult(
                isValid = false,
                normalizedBaseUrl = trimmed,
                classification = RouteClassification.INSECURE_BLOCKED,
                warningOrErrorRu = "Некорректный формат URL сервера"
            )
        }

        val scheme = uri.scheme?.lowercase() ?: ""
        val host = uri.host?.lowercase() ?: ""
        if (uri.userInfo != null || uri.rawQuery != null || uri.fragment != null || (!uri.path.isNullOrEmpty() && uri.path != "/")) {
            return TransportValidationResult(false, "", RouteClassification.INSECURE_BLOCKED, "Укажите адрес без пароля, пути, query и fragment")
        }
        if (host.isEmpty() || (scheme != "https" && scheme != "http")) {
            return TransportValidationResult(
                isValid = false,
                normalizedBaseUrl = trimmed,
                classification = RouteClassification.INSECURE_BLOCKED,
                warningOrErrorRu = "Поддерживаются только схемы https://, http:// (для LAN/Tailscale) или loopback://"
            )
        }

        val normalized = "${scheme}://${uri.authority}"
        val isTailscale = isTailscaleHost(host)
        val isPrivateLan = isPrivateLanHost(host)

        if (scheme == "https") {
            val classification = when {
                isTailscale -> RouteClassification.TAILSCALE_VPN
                isPrivateLan -> RouteClassification.LOCAL_LAN
                else -> RouteClassification.PUBLIC_HTTPS
            }
            return TransportValidationResult(
                isValid = true,
                normalizedBaseUrl = normalized,
                classification = classification,
                warningOrErrorRu = if (isPrivateLan) {
                    "Локальный адрес $host недоступен из мобильной сети без VPN/Tailscale"
                } else null
            )
        }

        // scheme == "http"
        if (!isTailscale && !isPrivateLan) {
            return TransportValidationResult(
                isValid = false,
                normalizedBaseUrl = normalized,
                classification = RouteClassification.INSECURE_BLOCKED,
                warningOrErrorRu = "Незашифрованный HTTP запрещён для внешних доменов ($host). Используйте HTTPS или Tailscale."
            )
        }

        if (!allowCleartextLocalHttp) {
            return TransportValidationResult(
                isValid = false,
                normalizedBaseUrl = normalized,
                classification = RouteClassification.INSECURE_BLOCKED,
                warningOrErrorRu = "Для HTTP в локальной сети/Tailscale ($host) включите тумблер «Разрешить локальный HTTP отладки»."
            )
        }

        val classification = if (isTailscale) {
            RouteClassification.TAILSCALE_VPN
        } else {
            RouteClassification.LOCAL_LAN
        }

        val note = if (isPrivateLan) {
            "Локальный HTTP разрешён. На мобильном интернете (LTE/5G) адрес $host будет недоступен без Tailscale."
        } else {
            "HTTP внутри защищённого туннеля Tailscale ($host)."
        }

        return TransportValidationResult(
            isValid = true,
            normalizedBaseUrl = normalized,
            classification = classification,
            warningOrErrorRu = note
        )
    }

    fun isPrivateLanHost(host: String): Boolean {
        if (host == "localhost" || host == "127.0.0.1" || host == "10.0.2.2" || host.endsWith(".local") || host.endsWith(".lan")) {
            return true
        }
        val parts = host.split(".")
        if (parts.size != 4) return false
        val octets = parts.map { it.toIntOrNull() ?: return false }
        if (octets.any { it !in 0..255 }) return false
        val first = octets[0]
        val second = octets[1]
        return when {
            first == 10 -> true
            first == 192 && second == 168 -> true
            first == 172 && second in 16..31 -> true
            else -> false
        }
    }

    fun isTailscaleHost(host: String): Boolean {
        if (host.endsWith(".ts.net")) return true
        val parts = host.split(".")
        if (parts.size != 4) return false
        val octets = parts.map { it.toIntOrNull() ?: return false }
        if (octets.any { it !in 0..255 }) return false
        // Tailscale CGNAT range: 100.64.0.0/10 (100.64.x.x - 100.127.x.x)
        return octets[0] == 100 && octets[1] in 64..127
    }
}
