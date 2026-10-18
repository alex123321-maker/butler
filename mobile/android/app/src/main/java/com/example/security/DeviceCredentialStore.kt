package com.example.security

import android.content.Context
import android.content.SharedPreferences
import android.os.Build
import com.example.model.AppFilterMode
import com.example.model.LoopbackSimulationMode
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.util.UUID

data class DeviceConfigurationState(
    val deviceId: String,
    val deviceName: String,
    val serverUrl: String,
    val serverName: String?,
    val isPaired: Boolean,
    val isTokenRevoked: Boolean,
    val pairedAtEpochMs: Long?,
    val allowCleartextLocalHttp: Boolean,
    val isSyncPaused: Boolean,
    val isContentTransmissionEnabled: Boolean,
    val maskOtpAndBankCodes: Boolean,
    val appFilterMode: AppFilterMode,
    val maxQueueCount: Int,
    val maxQueueBytes: Long,
    val maxQueueAgeDays: Int,
    val droppedOverflowCount: Long,
    val lastSuccessfulSyncEpochMs: Long?,
    val lastSyncErrorText: String?,
    val lastErrorCategory: String?,
    val loopbackSimulationMode: LoopbackSimulationMode,
    val pendingPairingSessionId: String?,
    val pendingVerificationCode: String?
)

/**
 * Manages device-scoped credentials and sync configuration.
 * The `device_token` is never kept in plaintext on disk: it is encrypted with
 * [KeystoreCryptoManager] (AES-256-GCM via Android Keystore) before writing to SharedPreferences.
 */
class DeviceCredentialStore(
    context: Context,
    private val cryptoManager: KeystoreCryptoManager
) {
    private val prefs: SharedPreferences =
        context.applicationContext.getSharedPreferences(PREFS_FILE_NAME, Context.MODE_PRIVATE)

    private val _state = MutableStateFlow(loadState())
    val state: StateFlow<DeviceConfigurationState> = _state.asStateFlow()

    @Synchronized
    fun getDeviceId(): String {
        val existing = prefs.getString(KEY_DEVICE_ID, null)
        if (!existing.isNullOrBlank()) return existing
        val generated = UUID.randomUUID().toString()
        prefs.edit().putString(KEY_DEVICE_ID, generated).apply()
        return generated
    }

    @Synchronized
    fun getDecryptedDeviceToken(): String? {
        val cipherBlob = prefs.getString(KEY_ENCRYPTED_DEVICE_TOKEN, null) ?: return null
        if (cipherBlob.isBlank()) return null
        return try {
            cryptoManager.decryptText(cipherBlob)
        } catch (_: Exception) {
            null
        }
    }

    @Synchronized
    fun savePairingSuccess(
        serverUrl: String,
        serverName: String,
        deviceToken: String,
        pairedAtEpochMs: Long = System.currentTimeMillis()
    ) {
        val encryptedToken = cryptoManager.encryptText(deviceToken)
        prefs.edit()
            .putString(KEY_SERVER_URL, serverUrl)
            .putString(KEY_SERVER_NAME, serverName)
            .putString(KEY_ENCRYPTED_DEVICE_TOKEN, encryptedToken)
            .putLong(KEY_PAIRED_AT_MS, pairedAtEpochMs)
            .putBoolean(KEY_TOKEN_REVOKED, false)
            .remove(KEY_PENDING_PAIRING_SESSION_ID)
            .remove(KEY_PENDING_VERIFICATION_CODE)
            .putString(KEY_LAST_SYNC_ERROR, null)
            .putString(KEY_LAST_ERROR_CATEGORY, null)
            .apply()
        refreshState()
    }

    @Synchronized
    fun savePendingPhonePairingSession(sessionId: String?, verificationCode: String?) {
        prefs.edit()
            .putString(KEY_PENDING_PAIRING_SESSION_ID, sessionId?.let { cryptoManager.encryptText(it) })
            .putString(KEY_PENDING_VERIFICATION_CODE, verificationCode)
            .apply()
        refreshState()
    }

    @Synchronized
    fun markTokenRevoked(reasonRu: String) {
        prefs.edit()
            .remove(KEY_ENCRYPTED_DEVICE_TOKEN)
            .putBoolean(KEY_TOKEN_REVOKED, true)
            .putBoolean(KEY_SYNC_PAUSED, true)
            .putString(KEY_LAST_SYNC_ERROR, reasonRu)
            .putString(KEY_LAST_ERROR_CATEGORY, "AUTH_REVOKED")
            .apply()
        refreshState()
    }

    @Synchronized
    fun revokeAndClearCredentials() {
        cryptoManager.destroyAndRotateKey()
        val preservedDeviceId = UUID.randomUUID().toString()
        prefs.edit()
            .putString(KEY_DEVICE_ID, preservedDeviceId)
            .remove(KEY_ENCRYPTED_DEVICE_TOKEN)
            .remove(KEY_SERVER_NAME)
            .remove(KEY_PAIRED_AT_MS)
            .remove(KEY_PENDING_PAIRING_SESSION_ID)
            .remove(KEY_PENDING_VERIFICATION_CODE)
            .putBoolean(KEY_TOKEN_REVOKED, false)
            .putBoolean(KEY_SYNC_PAUSED, false)
            .putString(KEY_LAST_SYNC_ERROR, null)
            .putString(KEY_LAST_ERROR_CATEGORY, null)
            .apply()
        refreshState()
    }

    @Synchronized
    fun updateServerUrl(serverUrl: String, allowCleartextLocalHttp: Boolean) {
        val current = prefs.getString(KEY_SERVER_URL, "").orEmpty().trimEnd('/')
        if (!getDecryptedDeviceToken().isNullOrBlank() && current != serverUrl.trim().trimEnd('/')) {
            recordSyncError("PAIRING_SERVER_BOUND", "Сначала отключите устройство перед сменой сервера")
            return
        }
        prefs.edit()
            .putString(KEY_SERVER_URL, serverUrl.trim())
            .putBoolean(KEY_ALLOW_CLEARTEXT_HTTP, allowCleartextLocalHttp)
            .apply()
        refreshState()
    }

    @Synchronized
    fun setSyncPaused(paused: Boolean) {
        prefs.edit().putBoolean(KEY_SYNC_PAUSED, paused).apply()
        refreshState()
    }

    @Synchronized
    fun setContentTransmissionEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_CONTENT_TRANSMISSION_ENABLED, enabled).apply()
        refreshState()
    }

    @Synchronized
    fun setMaskOtpAndBankCodes(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_MASK_OTP_CODES, enabled).apply()
        refreshState()
    }

    @Synchronized
    fun setAppFilterMode(mode: AppFilterMode) {
        prefs.edit().putString(KEY_APP_FILTER_MODE, mode.name).apply()
        refreshState()
    }

    @Synchronized
    fun setLoopbackSimulationMode(mode: LoopbackSimulationMode) {
        prefs.edit().putString(KEY_LOOPBACK_SIMULATION_MODE, mode.name).apply()
        refreshState()
    }

    @Synchronized
    fun updateQueueLimits(maxCount: Int, maxBytes: Long, maxAgeDays: Int) {
        prefs.edit()
            .putInt(KEY_MAX_QUEUE_COUNT, maxCount.coerceIn(50, 50_000))
            .putLong(KEY_MAX_QUEUE_BYTES, maxBytes.coerceIn(256 * 1024L, 100 * 1024 * 1024L))
            .putInt(KEY_MAX_QUEUE_AGE_DAYS, maxAgeDays.coerceIn(1, 90))
            .apply()
        refreshState()
    }

    @Synchronized
    fun incrementDroppedOverflowCount(delta: Int): Long {
        if (delta <= 0) return prefs.getLong(KEY_DROPPED_OVERFLOW_COUNT, 0L)
        val updated = prefs.getLong(KEY_DROPPED_OVERFLOW_COUNT, 0L) + delta
        prefs.edit().putLong(KEY_DROPPED_OVERFLOW_COUNT, updated).apply()
        refreshState()
        return updated
    }

    @Synchronized
    fun resetDroppedOverflowCount() {
        prefs.edit().putLong(KEY_DROPPED_OVERFLOW_COUNT, 0L).apply()
        refreshState()
    }

    @Synchronized
    fun recordSyncSuccess(epochMs: Long = System.currentTimeMillis()) {
        prefs.edit()
            .putLong(KEY_LAST_SUCCESS_SYNC_MS, epochMs)
            .putString(KEY_LAST_SYNC_ERROR, null)
            .putString(KEY_LAST_ERROR_CATEGORY, null)
            .apply()
        refreshState()
    }

    @Synchronized
    fun recordSyncError(category: String, messageRu: String) {
        prefs.edit()
            .putString(KEY_LAST_ERROR_CATEGORY, category)
            .putString(KEY_LAST_SYNC_ERROR, messageRu)
            .apply()
        refreshState()
    }

    private fun refreshState() {
        _state.value = loadState()
    }

    private fun loadState(): DeviceConfigurationState {
        val deviceId = getDeviceId()
        val manufacturer = Build.MANUFACTURER?.replaceFirstChar { it.uppercase() } ?: "Android"
        val model = Build.MODEL ?: "Device"
        val defaultDeviceName = if (model.startsWith(manufacturer, ignoreCase = true)) {
            model
        } else {
            "$manufacturer $model"
        }

        val tokenPresent = !getDecryptedDeviceToken().isNullOrBlank()
        val isTokenRevoked = prefs.getBoolean(KEY_TOKEN_REVOKED, false)
        val filterModeName = prefs.getString(KEY_APP_FILTER_MODE, AppFilterMode.ALL_EXCEPT_EXCLUDED.name)
        val filterMode = runCatching { AppFilterMode.valueOf(filterModeName!!) }
            .getOrDefault(AppFilterMode.ALL_EXCEPT_EXCLUDED)

        val simModeName = prefs.getString(
            KEY_LOOPBACK_SIMULATION_MODE,
            LoopbackSimulationMode.NORMAL_200_ACK.name
        )
        val simMode = runCatching { LoopbackSimulationMode.valueOf(simModeName!!) }
            .getOrDefault(LoopbackSimulationMode.NORMAL_200_ACK)

        val pairedAt = prefs.getLong(KEY_PAIRED_AT_MS, -1L).takeIf { it > 0L }
        val lastSync = prefs.getLong(KEY_LAST_SUCCESS_SYNC_MS, -1L).takeIf { it > 0L }

        return DeviceConfigurationState(
            deviceId = deviceId,
            deviceName = defaultDeviceName,
            serverUrl = prefs.getString(KEY_SERVER_URL, "") ?: "",
            serverName = prefs.getString(KEY_SERVER_NAME, null),
            isPaired = tokenPresent && !isTokenRevoked,
            isTokenRevoked = isTokenRevoked,
            pairedAtEpochMs = pairedAt,
            allowCleartextLocalHttp = prefs.getBoolean(KEY_ALLOW_CLEARTEXT_HTTP, false),
            isSyncPaused = prefs.getBoolean(KEY_SYNC_PAUSED, false),
            isContentTransmissionEnabled = prefs.getBoolean(KEY_CONTENT_TRANSMISSION_ENABLED, true),
            maskOtpAndBankCodes = prefs.getBoolean(KEY_MASK_OTP_CODES, true),
            appFilterMode = filterMode,
            maxQueueCount = prefs.getInt(KEY_MAX_QUEUE_COUNT, DEFAULT_MAX_QUEUE_COUNT),
            maxQueueBytes = prefs.getLong(KEY_MAX_QUEUE_BYTES, DEFAULT_MAX_QUEUE_BYTES),
            maxQueueAgeDays = prefs.getInt(KEY_MAX_QUEUE_AGE_DAYS, DEFAULT_MAX_QUEUE_AGE_DAYS),
            droppedOverflowCount = prefs.getLong(KEY_DROPPED_OVERFLOW_COUNT, 0L),
            lastSuccessfulSyncEpochMs = lastSync,
            lastSyncErrorText = prefs.getString(KEY_LAST_SYNC_ERROR, null),
            lastErrorCategory = prefs.getString(KEY_LAST_ERROR_CATEGORY, null),
            loopbackSimulationMode = simMode,
            pendingPairingSessionId = prefs.getString(KEY_PENDING_PAIRING_SESSION_ID, null)?.let { runCatching { cryptoManager.decryptText(it) }.getOrNull() },
            pendingVerificationCode = prefs.getString(KEY_PENDING_VERIFICATION_CODE, null)
        )
    }

    companion object {
        private const val PREFS_FILE_NAME = "butler_device_credentials_prefs"
        private const val KEY_DEVICE_ID = "device_id"
        private const val KEY_SERVER_URL = "server_url"
        private const val KEY_SERVER_NAME = "server_name"
        private const val KEY_ENCRYPTED_DEVICE_TOKEN = "encrypted_device_token"
        private const val KEY_PAIRED_AT_MS = "paired_at_ms"
        private const val KEY_TOKEN_REVOKED = "token_revoked"
        private const val KEY_ALLOW_CLEARTEXT_HTTP = "allow_cleartext_http"
        private const val KEY_SYNC_PAUSED = "sync_paused"
        private const val KEY_CONTENT_TRANSMISSION_ENABLED = "content_transmission_enabled"
        private const val KEY_MASK_OTP_CODES = "mask_otp_codes"
        private const val KEY_APP_FILTER_MODE = "app_filter_mode"
        private const val KEY_MAX_QUEUE_COUNT = "max_queue_count"
        private const val KEY_MAX_QUEUE_BYTES = "max_queue_bytes"
        private const val KEY_MAX_QUEUE_AGE_DAYS = "max_queue_age_days"
        private const val KEY_DROPPED_OVERFLOW_COUNT = "dropped_overflow_count"
        private const val KEY_LAST_SUCCESS_SYNC_MS = "last_success_sync_ms"
        private const val KEY_LAST_SYNC_ERROR = "last_sync_error"
        private const val KEY_LAST_ERROR_CATEGORY = "last_error_category"
        private const val KEY_LOOPBACK_SIMULATION_MODE = "loopback_simulation_mode"
        private const val KEY_PENDING_PAIRING_SESSION_ID = "pending_pairing_session_id"
        private const val KEY_PENDING_VERIFICATION_CODE = "pending_verification_code"

        const val DEFAULT_MAX_QUEUE_COUNT = 5_000
        const val DEFAULT_MAX_QUEUE_BYTES = 25L * 1024L * 1024L // 25 MB
        const val DEFAULT_MAX_QUEUE_AGE_DAYS = 14
    }
}
