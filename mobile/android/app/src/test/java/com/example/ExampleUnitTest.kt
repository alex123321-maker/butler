package com.example

import com.example.domain.NormalizationOutcome
import com.example.domain.NotificationEventNormalizer
import com.example.domain.PriorNotificationTrackingState
import com.example.domain.PrivacyRedactor
import com.example.domain.RawNotificationSnapshot
import com.example.model.ButlerContractConstants
import com.example.model.ConversationMessageDto
import com.example.model.NotificationChangeType
import com.example.model.NotificationVisibilityLevel
import com.example.model.ProgressDto
import com.example.security.KeystoreCryptoManager
import com.example.security.TransportPolicyValidator
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ExampleUnitTest {

    private val normalizer = NotificationEventNormalizer()

    @Test
    fun `normalizes MessagingStyle, Unicode, progress, and computes deterministic content hash`() {
        val snapshot = RawNotificationSnapshot(
            notificationKey = "0|org.telegram.messenger|42|chat|101",
            packageName = "org.telegram.messenger",
            channelId = "messages",
            postedAtEpochMs = 1790000000000L,
            observedAtEpochMs = 1790000000050L,
            category = "msg",
            visibility = NotificationVisibilityLevel.PRIVATE,
            isGroupSummary = false,
            groupKey = "tg_group_1",
            title = "Алексей 🚀",
            text = "Проверяем кириллицу и эмодзи ✅",
            conversationTitle = "Разработка Butler",
            messages = listOf(
                ConversationMessageDto("Алексей", "Первое сообщение", 1790000000000L),
                ConversationMessageDto("Мария", "Проверяем кириллицу и эмодзи ✅", 1790000000010L)
            ),
            progress = ProgressDto(current = 45, max = 100, indeterminate = false)
        )

        val outcome1 = normalizer.normalizePostedOrUpdated(
            snapshot = snapshot,
            isInitialSnapshot = false,
            priorState = null,
            isContentTransmissionEnabled = true,
            maskOtpAndBankCodes = true
        )
        assertTrue(outcome1 is NormalizationOutcome.Emitted)
        val emitted = (outcome1 as NormalizationOutcome.Emitted).event

        assertEquals(ButlerContractConstants.CURRENT_SCHEMA_VERSION, emitted.schemaVersion)
        assertEquals(NotificationChangeType.POSTED.name, emitted.changeType)
        assertNull(emitted.previousDeviceEventId)
        assertEquals("Алексей 🚀", emitted.payload?.title)
        assertEquals(2, emitted.payload?.messages?.size)
        assertEquals(45, emitted.payload?.progress?.current)

        // Identical second callback must be deduplicated, not emitted again
        val prior = PriorNotificationTrackingState(
            lastDeviceEventId = emitted.deviceEventId,
            lastContentHash = emitted.contentHash
        )
        val duplicateOutcome = normalizer.normalizePostedOrUpdated(
            snapshot = snapshot,
            isInitialSnapshot = false,
            priorState = prior,
            isContentTransmissionEnabled = true,
            maskOtpAndBankCodes = true
        )
        assertTrue(duplicateOutcome is NormalizationOutcome.DeduplicatedUnchanged)

        // Modified text on the same notificationKey must emit UPDATED with previous_device_event_id linked
        val modifiedSnapshot = snapshot.copy(text = "Обновлённый текст карточки!")
        val updatedOutcome = normalizer.normalizePostedOrUpdated(
            snapshot = modifiedSnapshot,
            isInitialSnapshot = false,
            priorState = prior,
            isContentTransmissionEnabled = true,
            maskOtpAndBankCodes = true
        )
        assertTrue(updatedOutcome is NormalizationOutcome.Emitted)
        val updatedEvent = (updatedOutcome as NormalizationOutcome.Emitted).event
        assertEquals(NotificationChangeType.UPDATED.name, updatedEvent.changeType)
        assertEquals(emitted.deviceEventId, updatedEvent.previousDeviceEventId)
        assertNotEquals(emitted.deviceEventId, updatedEvent.deviceEventId)
        assertNotEquals(emitted.contentHash, updatedEvent.contentHash)
    }

    @Test
    fun `truncates oversized notification fields and sets is_truncated flag`() {
        val hugeText = "Б".repeat(ButlerContractConstants.MAX_BIG_TEXT_LENGTH + 500)
        val snapshot = RawNotificationSnapshot(
            notificationKey = "0|com.example.mail|1|null|100",
            packageName = "com.example.mail",
            postedAtEpochMs = 1790000001000L,
            title = "Длинное письмо",
            bigText = hugeText
        )

        val outcome = normalizer.normalizePostedOrUpdated(
            snapshot = snapshot,
            isInitialSnapshot = true,
            priorState = null,
            isContentTransmissionEnabled = true,
            maskOtpAndBankCodes = true
        ) as NormalizationOutcome.Emitted

        assertTrue(outcome.event.isTruncated)
        assertTrue(outcome.event.isInitialSnapshot)
        assertEquals(ButlerContractConstants.MAX_BIG_TEXT_LENGTH, outcome.event.payload?.bigText?.length)
    }

    @Test
    fun `masks OTP codes and bank card numbers and respects content omission mode`() {
        val bankSnapshot = RawNotificationSnapshot(
            notificationKey = "0|ru.sberbankmobile|99|otp|100",
            packageName = "ru.sberbankmobile",
            postedAtEpochMs = 1790000002000L,
            title = "СберБанк",
            text = "Код подтверждения операции: 849201 по карте 4276 3800 1234 5678"
        )

        val maskedOutcome = normalizer.normalizePostedOrUpdated(
            snapshot = bankSnapshot,
            isInitialSnapshot = false,
            priorState = null,
            isContentTransmissionEnabled = true,
            maskOtpAndBankCodes = true
        ) as NormalizationOutcome.Emitted

        assertTrue(maskedOutcome.event.contentRedactedByUserRule)
        val redactedText = maskedOutcome.event.payload?.text.orEmpty()
        assertFalse("OTP must not remain in cleartext", redactedText.contains("849201"))
        assertFalse("Card number must not remain in cleartext", redactedText.contains("4276 3800 1234 5678"))
        assertTrue(redactedText.contains("[OTP-REDACTED]"))
        assertTrue(redactedText.contains("[CARD-REDACTED]"))

        // When content transmission is disabled, payload must be completely omitted
        val omittedOutcome = normalizer.normalizePostedOrUpdated(
            snapshot = bankSnapshot,
            isInitialSnapshot = false,
            priorState = null,
            isContentTransmissionEnabled = false,
            maskOtpAndBankCodes = true
        ) as NormalizationOutcome.Emitted

        assertTrue(omittedOutcome.event.contentOmittedByMode)
        assertNull(omittedOutcome.event.payload)
    }

    @Test
    fun `preserves Android 15 platform OTP redaction indicators`() {
        val snapshot = RawNotificationSnapshot(
            notificationKey = "0|com.google.android.apps.messaging|7|sms|100",
            packageName = "com.google.android.apps.messaging",
            postedAtEpochMs = 1790000003000L,
            platformRedactedOtp = true,
            title = "Сообщения",
            text = "Sensitive notification content hidden"
        )

        val outcome = normalizer.normalizePostedOrUpdated(
            snapshot = snapshot,
            isInitialSnapshot = false,
            priorState = null,
            isContentTransmissionEnabled = true,
            maskOtpAndBankCodes = true
        ) as NormalizationOutcome.Emitted

        assertTrue(outcome.event.systemOtpRedacted)
        assertNotNull(outcome.event.payload)
    }

    @Test
    fun `validates transport security policy for HTTPS, Tailscale, LAN, and public HTTP`() {
        // Public HTTPS is valid
        val httpsCheck = TransportPolicyValidator.validate("https://butler.myhome.dev", allowCleartextLocalHttp = false)
        assertTrue(httpsCheck.isValid)

        // Public cleartext HTTP is strictly blocked even if allowCleartextLocalHttp = true
        val publicHttpCheck = TransportPolicyValidator.validate("http://butler.myhome.dev", allowCleartextLocalHttp = true)
        assertFalse(publicHttpCheck.isValid)

        // Private LAN HTTP is valid only when allowCleartextLocalHttp = true
        val lanBlocked = TransportPolicyValidator.validate("http://192.168.1.40:8080", allowCleartextLocalHttp = false)
        assertFalse(lanBlocked.isValid)
        val lanAllowed = TransportPolicyValidator.validate("http://192.168.1.40:8080", allowCleartextLocalHttp = true)
        assertTrue(lanAllowed.isValid)

        // Tailscale CGNAT 100.64.0.0/10 is recognized as Tailscale VPN
        val tailscaleCheck = TransportPolicyValidator.validate("http://100.101.102.103:8080", allowCleartextLocalHttp = true)
        assertTrue(tailscaleCheck.isValid)
        assertTrue(PrivacyRedactor.isSensitivePackage("ru.sberbankmobile"))
    }
}
