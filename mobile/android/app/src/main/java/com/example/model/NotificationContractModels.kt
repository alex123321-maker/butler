package com.example.model

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Versioned contract v1 for Butler Mobile Notification Companion (Issue #57).
 *
 * SECURITY INVARIANT:
 * Notification payloads are untrusted external observations.
 * This contract strictly excludes `owner_id`, `session_key`, `authority`, or any command fields.
 * Owner identity is resolved exclusively on the Butler server via the device-scoped bearer token.
 */
object ButlerContractConstants {
    const val CURRENT_SCHEMA_VERSION = 1
    const val MAX_TEXT_FIELD_LENGTH = 2000
    const val MAX_BIG_TEXT_LENGTH = 4000
    const val MAX_MESSAGES_PER_NOTIFICATION = 8
}

enum class NotificationChangeType {
    POSTED,
    UPDATED,
    REMOVED
}

enum class NotificationVisibilityLevel {
    PUBLIC,
    PRIVATE,
    SECRET,
    UNKNOWN
}

enum class DeliveryState {
    PENDING,
    IN_FLIGHT,
    DELIVERED,
    CONFLICT_ERROR,
    PERMANENT_ERROR
}

enum class AppFilterMode {
    ALL_EXCEPT_EXCLUDED,
    ONLY_INCLUDED
}

enum class RouteClassification(val labelRu: String) {
    PUBLIC_HTTPS("Защищённый HTTPS (внешний/туннель)"),
    TAILSCALE_VPN("Tailscale (защищённая mesh-сеть 100.x)"),
    LOCAL_LAN("Локальная сеть Wi-Fi (192.168.x / 10.x)"),
    LOOPBACK_SANDBOX("Локальный стенд контракта (Loopback v1)"),
    INSECURE_BLOCKED("Небезопасный HTTP заблокирован")
}

enum class LoopbackSimulationMode(val labelRu: String, val descriptionRu: String) {
    NORMAL_200_ACK("200 OK (Полный ACK)", "Все события пакета подтверждаются сервером"),
    PARTIAL_207_ACK("207 Partial ACK + Duplicate", "Часть принята, часть помечена как уже доставленный дубль"),
    CONFLICT_409("409 Conflict (Конфликт хеша ID)", "Сервер отклоняет событие из-за конфликта содержимого при том же ID"),
    UNAUTHORIZED_401("401 Token Revoked (Отзыв токена)", "Сервер сообщает об отзыве токена устройства"),
    RATE_LIMIT_429("429 Too Many Requests", "Сервер ограничивает частоту, запускается exponential backoff"),
    SERVER_ERROR_503("503 Service Unavailable", "Временный сбой сервера/БД, события остаются в очереди"),
    TLS_ERROR("Ошибка TLS сертификата", "Имитация сбоя проверки SSL/TLS"),
    NETWORK_TIMEOUT("Тайм-аут соединения", "Имитация потери ответа после отправки пакета")
}

@JsonClass(generateAdapter = true)
data class ConversationMessageDto(
    @Json(name = "sender") val sender: String?,
    @Json(name = "text") val text: String,
    @Json(name = "timestamp_epoch_ms") val timestampEpochMs: Long
)

@JsonClass(generateAdapter = true)
data class ProgressDto(
    @Json(name = "current") val current: Int,
    @Json(name = "max") val max: Int,
    @Json(name = "indeterminate") val indeterminate: Boolean
)

@JsonClass(generateAdapter = true)
data class NotificationPayloadDto(
    @Json(name = "title") val title: String? = null,
    @Json(name = "text") val text: String? = null,
    @Json(name = "big_text") val bigText: String? = null,
    @Json(name = "sub_text") val subText: String? = null,
    @Json(name = "summary_text") val summaryText: String? = null,
    @Json(name = "info_text") val infoText: String? = null,
    @Json(name = "conversation_title") val conversationTitle: String? = null,
    @Json(name = "messages") val messages: List<ConversationMessageDto> = emptyList(),
    @Json(name = "progress") val progress: ProgressDto? = null
)

@JsonClass(generateAdapter = true)
data class MobileNotificationEventDto(
    @Json(name = "schema_version") val schemaVersion: Int = ButlerContractConstants.CURRENT_SCHEMA_VERSION,
    @Json(name = "device_event_id") val deviceEventId: String,
    @Json(name = "notification_key") val notificationKey: String,
    @Json(name = "previous_device_event_id") val previousDeviceEventId: String? = null,
    @Json(name = "package_name") val packageName: String,
    @Json(name = "channel_id") val channelId: String? = null,
    @Json(name = "change_type") val changeType: String,
    @Json(name = "is_initial_snapshot") val isInitialSnapshot: Boolean = false,
    @Json(name = "posted_at_epoch_ms") val postedAtEpochMs: Long,
    @Json(name = "observed_at_epoch_ms") val observedAtEpochMs: Long,
    @Json(name = "category") val category: String? = null,
    @Json(name = "visibility") val visibility: String = NotificationVisibilityLevel.PRIVATE.name,
    @Json(name = "is_group_summary") val isGroupSummary: Boolean = false,
    @Json(name = "group_key") val groupKey: String? = null,
    @Json(name = "is_ongoing") val isOngoing: Boolean = false,
    @Json(name = "is_clearable") val isClearable: Boolean = true,
    @Json(name = "content_redacted_by_user_rule") val contentRedactedByUserRule: Boolean = false,
    @Json(name = "content_omitted_by_mode") val contentOmittedByMode: Boolean = false,
    @Json(name = "system_otp_redacted") val systemOtpRedacted: Boolean = false,
    @Json(name = "is_truncated") val isTruncated: Boolean = false,
    @Json(name = "content_hash") val contentHash: String,
    @Json(name = "payload") val payload: NotificationPayloadDto? = null
)

@JsonClass(generateAdapter = true)
data class NotificationBatchRequestDto(
    @Json(name = "schema_version") val schemaVersion: Int = ButlerContractConstants.CURRENT_SCHEMA_VERSION,
    @Json(name = "batch_id") val batchId: String,
    @Json(name = "sent_at_epoch_ms") val sentAtEpochMs: Long,
    @Json(name = "events") val events: List<MobileNotificationEventDto>
)

@JsonClass(generateAdapter = true)
data class RejectedEventDto(
    @Json(name = "device_event_id") val deviceEventId: String,
    @Json(name = "error_code") val errorCode: String,
    @Json(name = "reason") val reason: String
)

@JsonClass(generateAdapter = true)
data class NotificationBatchResponseDto(
    @Json(name = "accepted_ids") val acceptedIds: List<String> = emptyList(),
    @Json(name = "duplicate_ids") val duplicateIds: List<String> = emptyList(),
    @Json(name = "conflict_ids") val conflictIds: List<String> = emptyList(),
    @Json(name = "rejected") val rejected: List<RejectedEventDto> = emptyList(),
    @Json(name = "server_time_epoch_ms") val serverTimeEpochMs: Long = System.currentTimeMillis()
)

// Pairing Variant A: Exchange One-Time Code or QR JSON from Butler Web UI
@JsonClass(generateAdapter = true)
data class PairExchangeRequestDto(
    @Json(name = "pairing_code") val pairingCode: String,
    @Json(name = "device_id") val deviceId: String,
    @Json(name = "device_name") val deviceName: String,
    @Json(name = "android_sdk") val androidSdk: Int,
    @Json(name = "app_version") val appVersion: String
)

@JsonClass(generateAdapter = true)
data class PairExchangeResponseDto(
    @Json(name = "device_id") val deviceId: String,
    @Json(name = "device_token") val deviceToken: String,
    @Json(name = "server_name") val serverName: String,
    @Json(name = "paired_at") val pairedAt: String
)

// Pairing Variant B: Phone Requests Verification Code and Polls for Web UI Approval
@JsonClass(generateAdapter = true)
data class PairRequestInitDto(
    @Json(name = "device_id") val deviceId: String,
    @Json(name = "device_name") val deviceName: String,
    @Json(name = "android_sdk") val androidSdk: Int,
    @Json(name = "app_version") val appVersion: String
)

@JsonClass(generateAdapter = true)
data class PairRequestInitResponseDto(
    @Json(name = "pairing_session_id") val pairingSessionId: String,
    @Json(name = "user_verification_code") val userVerificationCode: String,
    @Json(name = "expires_in_sec") val expiresInSec: Int = 300
)

@JsonClass(generateAdapter = true)
data class PairPollRequestDto(
    @Json(name = "pairing_session_id") val pairingSessionId: String,
    @Json(name = "device_id") val deviceId: String
)

@JsonClass(generateAdapter = true)
data class PairPollResponseDto(
    @Json(name = "status") val status: String, // PENDING, APPROVED, EXPIRED, REJECTED
    @Json(name = "device_token") val deviceToken: String? = null,
    @Json(name = "server_name") val serverName: String? = null
)

@JsonClass(generateAdapter = true)
data class DeviceStatusResponseDto(
    @Json(name = "device_id") val deviceId: String,
    @Json(name = "status") val status: String, // ACTIVE, REVOKED
    @Json(name = "server_name") val serverName: String,
    @Json(name = "server_time_epoch_ms") val serverTimeEpochMs: Long,
    @Json(name = "max_batch_size") val maxBatchSize: Int = 50
)

@JsonClass(generateAdapter = true)
data class DisconnectRequestDto(
    @Json(name = "device_id") val deviceId: String,
    @Json(name = "reason") val reason: String = "USER_INITIATED_DISCONNECT"
)
