package com.example.data.local

import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/**
 * Durable local queue record for a normalized Android notification event.
 *
 * SECURITY AT REST:
 * `encryptedPayloadBlob` holds the Base64-encoded AES-256-GCM IV + Ciphertext of
 * `NotificationPayloadDto` (JSON). Plaintext titles/texts/messages are never written to SQLite.
 */
@Entity(
    tableName = "queued_notification_events",
    indices = [
        Index(value = ["deliveryState", "nextAttemptAtEpochMs"]),
        Index(value = ["notificationKey"]),
        Index(value = ["observedAtEpochMs"])
    ]
)
data class QueuedNotificationEntity(
    @PrimaryKey
    val deviceEventId: String,
    val schemaVersion: Int,
    val notificationKey: String,
    val previousDeviceEventId: String?,
    val packageName: String,
    val channelId: String?,
    val changeType: String,
    val isInitialSnapshot: Boolean,
    val postedAtEpochMs: Long,
    val observedAtEpochMs: Long,
    val category: String?,
    val visibility: String,
    val isGroupSummary: Boolean,
    val groupKey: String?,
    val isOngoing: Boolean,
    val isClearable: Boolean,
    val contentRedactedByUserRule: Boolean,
    val contentOmittedByMode: Boolean,
    val systemOtpRedacted: Boolean,
    val isTruncated: Boolean,
    val contentHash: String,
    val encryptedPayloadBlob: String?,
    val payloadSizeBytes: Int,
    val deliveryState: String, // PENDING, IN_FLIGHT, DELIVERED, CONFLICT_ERROR, PERMANENT_ERROR
    val attemptCount: Int = 0,
    val nextAttemptAtEpochMs: Long = 0L,
    val deliveredAtEpochMs: Long? = null,
    val lastErrorCode: String? = null,
    val lastErrorReason: String? = null
)

@Entity(tableName = "app_filter_rules")
data class AppFilterRuleEntity(
    @PrimaryKey
    val packageName: String,
    val appLabel: String,
    val categoryLabel: String,
    val isSensitiveSource: Boolean,
    val isExcludedInAllMode: Boolean,
    val isIncludedInAllowlistMode: Boolean,
    val lastSeenNotificationEpochMs: Long? = null,
    val totalObservedCount: Int = 0
)

/**
 * Zero-PII diagnostic log entry.
 * Strictly forbids storing notification titles, message bodies, sender names, or bearer tokens.
 */
@Entity(
    tableName = "diagnostic_logs",
    indices = [Index(value = ["timestampEpochMs"])]
)
data class DiagnosticLogEntity(
    @PrimaryKey(autoGenerate = true)
    val id: Long = 0,
    val timestampEpochMs: Long = System.currentTimeMillis(),
    val level: String, // INFO, WARN, ERROR
    val stage: String, // LISTENER, QUEUE, NETWORK_SYNC, PAIRING, SECURITY, POWER
    val code: String,
    val summaryRu: String,
    val httpStatus: Int? = null,
    val batchSize: Int? = null,
    val latencyMs: Long? = null
)
