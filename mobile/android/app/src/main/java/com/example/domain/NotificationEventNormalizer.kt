package com.example.domain

import com.example.model.ButlerContractConstants
import com.example.model.ConversationMessageDto
import com.example.model.MobileNotificationEventDto
import com.example.model.NotificationChangeType
import com.example.model.NotificationPayloadDto
import com.example.model.NotificationVisibilityLevel
import com.example.model.ProgressDto
import java.security.MessageDigest
import java.util.UUID

/**
 * Platform-independent raw representation of an Android StatusBarNotification
 * so that normalization, truncation, MessagingStyle parsing, and deduplication
 * can be thoroughly unit-tested on the JVM as well as from the live service.
 */
data class RawNotificationSnapshot(
    val notificationKey: String,
    val packageName: String,
    val appLabelHint: String? = null,
    val channelId: String? = null,
    val postedAtEpochMs: Long,
    val observedAtEpochMs: Long = System.currentTimeMillis(),
    val category: String? = null,
    val visibility: NotificationVisibilityLevel = NotificationVisibilityLevel.PRIVATE,
    val isGroupSummary: Boolean = false,
    val groupKey: String? = null,
    val isOngoing: Boolean = false,
    val isClearable: Boolean = true,
    val platformRedactedOtp: Boolean = false,
    val title: String? = null,
    val text: String? = null,
    val bigText: String? = null,
    val subText: String? = null,
    val summaryText: String? = null,
    val infoText: String? = null,
    val conversationTitle: String? = null,
    val messages: List<ConversationMessageDto> = emptyList(),
    val progress: ProgressDto? = null
)

data class PriorNotificationTrackingState(
    val lastDeviceEventId: String,
    val lastContentHash: String
)

sealed class NormalizationOutcome {
    data class Emitted(val event: MobileNotificationEventDto) : NormalizationOutcome()
    data class DeduplicatedUnchanged(val notificationKey: String, val contentHash: String) : NormalizationOutcome()
}

/**
 * Normalizes notifications into versioned [MobileNotificationEventDto] records:
 * - Handles POSTED, UPDATED, and REMOVED transitions.
 * - Truncates oversized strings safely and marks `is_truncated = true`.
 * - Computes a deterministic SHA-256 `content_hash` over normalized fields.
 * - Distinguishes a genuine card modification (`UPDATED` with new `device_event_id` and
 *   `previous_device_event_id`) from a redundant OS callback with identical content.
 */
class NotificationEventNormalizer {

    fun normalizePostedOrUpdated(
        snapshot: RawNotificationSnapshot,
        isInitialSnapshot: Boolean,
        priorState: PriorNotificationTrackingState?,
        isContentTransmissionEnabled: Boolean,
        maskOtpAndBankCodes: Boolean,
        deterministicEventId: String? = null
    ): NormalizationOutcome {
        var wasTruncated = false

        fun BoundString(value: String?, maxLen: Int): String? {
            if (value == null) return null
            val cleaned = value.trim()
            if (cleaned.isEmpty()) return null
            return if (cleaned.length > maxLen) {
                wasTruncated = true
                cleaned.substring(0, if (cleaned[maxLen - 1].isHighSurrogate()) maxLen - 1 else maxLen)
            } else {
                cleaned
            }
        }

        val boundedMessages = if (snapshot.messages.size > ButlerContractConstants.MAX_MESSAGES_PER_NOTIFICATION) {
            wasTruncated = true
            snapshot.messages.takeLast(ButlerContractConstants.MAX_MESSAGES_PER_NOTIFICATION)
        } else {
            snapshot.messages
        }.map { msg ->
            ConversationMessageDto(
                sender = BoundString(msg.sender, 256),
                text = BoundString(msg.text, ButlerContractConstants.MAX_TEXT_FIELD_LENGTH) ?: "",
                timestampEpochMs = msg.timestampEpochMs
            )
        }

        val rawPayload = NotificationPayloadDto(
            title = BoundString(snapshot.title, ButlerContractConstants.MAX_TEXT_FIELD_LENGTH),
            text = BoundString(snapshot.text, ButlerContractConstants.MAX_TEXT_FIELD_LENGTH),
            bigText = BoundString(snapshot.bigText, ButlerContractConstants.MAX_BIG_TEXT_LENGTH),
            subText = BoundString(snapshot.subText, ButlerContractConstants.MAX_TEXT_FIELD_LENGTH),
            summaryText = BoundString(snapshot.summaryText, ButlerContractConstants.MAX_TEXT_FIELD_LENGTH),
            infoText = BoundString(snapshot.infoText, 512),
            conversationTitle = BoundString(snapshot.conversationTitle, 512),
            messages = boundedMessages,
            progress = snapshot.progress
        )

        val redaction = PrivacyRedactor.applyPrivacyRules(
            rawPayload = rawPayload,
            packageName = snapshot.packageName,
            isContentTransmissionEnabled = isContentTransmissionEnabled,
            maskOtpAndBankCodes = maskOtpAndBankCodes,
            platformVisibilitySecretOrRedacted = snapshot.platformRedactedOtp ||
                snapshot.visibility == NotificationVisibilityLevel.SECRET
        )

        val contentHash = computeContentHash(
            notificationKey = snapshot.notificationKey,
            packageName = snapshot.packageName,
            channelId = snapshot.channelId,
            changeType = NotificationChangeType.POSTED.name, // Hash content unabhängig from POSTED vs UPDATED
            category = snapshot.category,
            visibility = snapshot.visibility.name,
            isGroupSummary = snapshot.isGroupSummary,
            groupKey = snapshot.groupKey,
            isOngoing = snapshot.isOngoing,
            contentOmittedByMode = redaction.omittedByMode,
            payload = redaction.payload
        )

        if (priorState != null && priorState.lastContentHash == contentHash) {
            return NormalizationOutcome.DeduplicatedUnchanged(
                notificationKey = snapshot.notificationKey,
                contentHash = contentHash
            )
        }

        val changeType = if (priorState == null) {
            NotificationChangeType.POSTED
        } else {
            NotificationChangeType.UPDATED
        }

        val event = MobileNotificationEventDto(
            schemaVersion = ButlerContractConstants.CURRENT_SCHEMA_VERSION,
            deviceEventId = deterministicEventId ?: UUID.randomUUID().toString(),
            notificationKey = snapshot.notificationKey,
            previousDeviceEventId = priorState?.lastDeviceEventId,
            packageName = snapshot.packageName,
            channelId = snapshot.channelId,
            changeType = changeType.name,
            isInitialSnapshot = isInitialSnapshot,
            postedAtEpochMs = snapshot.postedAtEpochMs,
            observedAtEpochMs = snapshot.observedAtEpochMs,
            category = snapshot.category,
            visibility = snapshot.visibility.name,
            isGroupSummary = snapshot.isGroupSummary,
            groupKey = snapshot.groupKey,
            isOngoing = snapshot.isOngoing,
            isClearable = snapshot.isClearable,
            contentRedactedByUserRule = redaction.redactedByUserRule,
            contentOmittedByMode = redaction.omittedByMode,
            systemOtpRedacted = redaction.systemOtpRedacted,
            isTruncated = wasTruncated,
            contentHash = contentHash,
            payload = redaction.payload
        )

        return NormalizationOutcome.Emitted(event)
    }

    fun normalizeRemoved(
        notificationKey: String,
        packageName: String,
        channelId: String?,
        postedAtEpochMs: Long,
        observedAtEpochMs: Long = System.currentTimeMillis(),
        priorState: PriorNotificationTrackingState?,
        deterministicEventId: String? = null
    ): MobileNotificationEventDto {
        val contentHash = computeContentHash(
            notificationKey = notificationKey,
            packageName = packageName,
            channelId = channelId,
            changeType = NotificationChangeType.REMOVED.name,
            category = null,
            visibility = NotificationVisibilityLevel.PRIVATE.name,
            isGroupSummary = false,
            groupKey = null,
            isOngoing = false,
            contentOmittedByMode = true,
            payload = null
        )

        return MobileNotificationEventDto(
            schemaVersion = ButlerContractConstants.CURRENT_SCHEMA_VERSION,
            deviceEventId = deterministicEventId ?: UUID.randomUUID().toString(),
            notificationKey = notificationKey,
            previousDeviceEventId = priorState?.lastDeviceEventId,
            packageName = packageName,
            channelId = channelId,
            changeType = NotificationChangeType.REMOVED.name,
            isInitialSnapshot = false,
            postedAtEpochMs = postedAtEpochMs,
            observedAtEpochMs = observedAtEpochMs,
            visibility = NotificationVisibilityLevel.PRIVATE.name,
            contentOmittedByMode = true,
            contentHash = contentHash,
            payload = null
        )
    }

    fun computeContentHash(
        notificationKey: String,
        packageName: String,
        channelId: String?,
        changeType: String,
        category: String?,
        visibility: String,
        isGroupSummary: Boolean,
        groupKey: String?,
        isOngoing: Boolean,
        contentOmittedByMode: Boolean,
        payload: NotificationPayloadDto?
    ): String {
        val canonical = buildString {
            append("v1|")
            append(notificationKey).append('|')
            append(packageName).append('|')
            append(channelId.orEmpty()).append('|')
            append(changeType).append('|')
            append(category.orEmpty()).append('|')
            append(visibility).append('|')
            append(isGroupSummary).append('|')
            append(groupKey.orEmpty()).append('|')
            append(isOngoing).append('|')
            append(contentOmittedByMode).append('|')
            if (payload != null) {
                append(payload.title.orEmpty()).append('|')
                append(payload.text.orEmpty()).append('|')
                append(payload.bigText.orEmpty()).append('|')
                append(payload.subText.orEmpty()).append('|')
                append(payload.summaryText.orEmpty()).append('|')
                append(payload.infoText.orEmpty()).append('|')
                append(payload.conversationTitle.orEmpty()).append('|')
                payload.messages.forEach { msg ->
                    append(msg.sender.orEmpty()).append(':')
                    append(msg.text).append(':')
                    append(msg.timestampEpochMs).append(';')
                }
                append('|')
                payload.progress?.let { p ->
                    append(p.current).append('/').append(p.max).append('/').append(p.indeterminate)
                }
            }
        }
        val digest = MessageDigest.getInstance("SHA-256").digest(canonical.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }
}
