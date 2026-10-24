package com.example.domain

import com.example.model.ConversationMessageDto
import com.example.model.NotificationPayloadDto

data class RedactionResult(
    val payload: NotificationPayloadDto?,
    val redactedByUserRule: Boolean,
    val omittedByMode: Boolean,
    val systemOtpRedacted: Boolean
)

/**
 * Enforces privacy boundaries before an event ever reaches the local Room queue or network:
 * 1. If `isContentTransmissionEnabled == false`, drops the entire text payload (`payload = null`, `omittedByMode = true`).
 * 2. Preserves Android 15+ platform OTP redaction indicators without attempting to bypass them.
 * 3. If `maskOtpAndBankCodes == true`, redacts 4–8 digit one-time codes, bank card numbers, and CVV
 *    patterns inside notification titles, texts, and MessagingStyle messages.
 */
object PrivacyRedactor {

    private val OTP_CONTEXT_KEYWORDS = Regex(
        pattern = "(?iu)(код|пароль|подтвержд|одноразов|проверочн|вход|списан|карт|банк|otp|code|verification|passcode|pin|2fa|auth|login|sign.?in|token|cvv|cvc)"
    )

    // Matches 4 to 8 digit standalone numeric codes or hyphenated 3-3 codes (e.g., 482-910)
    private val NUMERIC_CODE_REGEX = Regex("""\b(?:\d{4,8}|\d{3}-\d{3})\b""")

    // Matches 13 to 19 digit payment card sequences
    private val CARD_NUMBER_REGEX = Regex("""\b(?:\d[ -]*?){13,19}\b""")

    // Detects Android 15+ system-redacted sensitive notification placeholders
    private val ANDROID_15_SYSTEM_REDACTION_HINTS = listOf(
        "Sensitive notification content hidden",
        "Содержимое уведомления скрыто",
        "Sensitive content hidden",
        "[REDACTED_BY_ANDROID_OS]"
    )

    val SENSITIVE_FINANCE_AND_AUTH_PACKAGES = setOf(
        "ru.sberbankmobile",
        "com.idamob.tinkoff.android",
        "ru.alfabank.mobile.android",
        "ru.vtb24.mobilebanking.android",
        "com.google.android.apps.authenticator2",
        "com.azure.authenticator",
        "com.x8bit.bitwarden",
        "com.agilebits.onepassword"
    )

    fun isSensitivePackage(packageName: String): Boolean {
        val lower = packageName.lowercase()
        return packageName in SENSITIVE_FINANCE_AND_AUTH_PACKAGES ||
            lower.contains("bank") ||
            lower.contains("authenticator") ||
            lower.contains("wallet") ||
            lower.contains("pay") ||
            lower.contains("sber") ||
            lower.contains("tinkoff") ||
            lower.contains("vtb")
    }

    fun applyPrivacyRules(
        rawPayload: NotificationPayloadDto?,
        packageName: String,
        isContentTransmissionEnabled: Boolean,
        maskOtpAndBankCodes: Boolean,
        platformVisibilitySecretOrRedacted: Boolean
    ): RedactionResult {
        if (!isContentTransmissionEnabled) {
            return RedactionResult(
                payload = null,
                redactedByUserRule = false,
                omittedByMode = true,
                systemOtpRedacted = platformVisibilitySecretOrRedacted
            )
        }

        if (rawPayload == null) {
            return RedactionResult(
                payload = null,
                redactedByUserRule = false,
                omittedByMode = false,
                systemOtpRedacted = platformVisibilitySecretOrRedacted
            )
        }

        val combinedText = listOfNotNull(
            rawPayload.title,
            rawPayload.text,
            rawPayload.bigText,
            rawPayload.subText,
            rawPayload.summaryText
        ).joinToString(" ") + " " + rawPayload.messages.joinToString(" ") { it.text }

        val detectedSystemRedaction = platformVisibilitySecretOrRedacted ||
            ANDROID_15_SYSTEM_REDACTION_HINTS.any { combinedText.contains(it, ignoreCase = true) }

        if (!maskOtpAndBankCodes) {
            return RedactionResult(
                payload = rawPayload,
                redactedByUserRule = false,
                omittedByMode = false,
                systemOtpRedacted = detectedSystemRedaction
            )
        }

        val hasOtpContext = isSensitivePackage(packageName) || OTP_CONTEXT_KEYWORDS.containsMatchIn(combinedText)
        var anyModified = false

        fun redactField(input: String?): String? {
            if (input.isNullOrEmpty()) return input
            var out = input
            if (CARD_NUMBER_REGEX.containsMatchIn(out)) {
                val replaced = CARD_NUMBER_REGEX.replace(out, "[CARD-REDACTED]")
                if (replaced != out) {
                    anyModified = true
                    out = replaced
                }
            }
            if (hasOtpContext || OTP_CONTEXT_KEYWORDS.containsMatchIn(out)) {
                val replaced = NUMERIC_CODE_REGEX.replace(out, "[OTP-REDACTED]")
                if (replaced != out) {
                    anyModified = true
                    out = replaced
                }
            }
            return out
        }

        val sanitizedMessages = rawPayload.messages.map { msg ->
            val sanitizedText = redactField(msg.text) ?: msg.text
            ConversationMessageDto(
                sender = msg.sender,
                text = sanitizedText,
                timestampEpochMs = msg.timestampEpochMs
            )
        }

        val sanitizedPayload = rawPayload.copy(
            title = redactField(rawPayload.title),
            text = redactField(rawPayload.text),
            bigText = redactField(rawPayload.bigText),
            subText = redactField(rawPayload.subText),
            summaryText = redactField(rawPayload.summaryText),
            infoText = redactField(rawPayload.infoText),
            conversationTitle = redactField(rawPayload.conversationTitle),
            messages = sanitizedMessages
        )

        return RedactionResult(
            payload = sanitizedPayload,
            redactedByUserRule = anyModified,
            omittedByMode = false,
            systemOtpRedacted = detectedSystemRedaction
        )
    }
}
