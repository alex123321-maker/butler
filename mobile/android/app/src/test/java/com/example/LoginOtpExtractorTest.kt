package com.example

import com.example.domain.LoginOtpExtractor
import org.junit.Assert.*
import org.junit.Test

class LoginOtpExtractorTest {
    private val now = 1_700_000_000_000L
    @Test fun acceptsOnlyOneFreshLoginCode() {
        val c = LoginOtpExtractor.extract("notification", "Example", "Код для входа: 739284", now, now, false)
        assertNotNull(c)
        assertEquals("739284", c!!.code)
        assertFalse(c.toString().contains("739284"))
        assertEquals(64, c.eventId.length)
        assertNotEquals(c.eventId, c.code)
    }
    @Test fun refusesPaymentsResetAmbiguityAndPlatformRedaction() {
        for (text in listOf("Код оплаты: 739284", "Login payment code: 739284", "Password reset code 739284", "Код входа 739284 или 128493", "Код входа отсутствует")) {
            assertNull(LoginOtpExtractor.extract("notification", "Example", text, now, now, false))
        }
        assertNull(LoginOtpExtractor.extract("notification", "Example", "Login code 739284", now, now, true))
        assertNull(LoginOtpExtractor.extract("notification", "Example", "Login code 739284", now - 91_000, now, false))
    }
    @Test fun refusesPasswordChangeCodesDespiteGenericVerificationWording() {
        for (text in listOf(
            "Your verification code to change your password: 739284",
            "Security code 739284 for password change",
            "Verification code to update your password: 739284",
            "Код для входа и изменения пароля: 739284"
        )) {
            assertNull(text, LoginOtpExtractor.extract("notification", "Example", text, now, now, false))
        }
    }
}
